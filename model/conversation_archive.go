package model

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

const ConversationArchiveConfigID = 1

type ConversationArchiveConfig struct {
	Id            int    `json:"id" gorm:"primaryKey"`
	ConfigVersion int64  `json:"config_version" gorm:"not null;default:1"`
	Enabled       bool   `json:"enabled" gorm:"not null"`
	GroupCodes    string `json:"group_codes" gorm:"type:text"`
	UserIds       string `json:"user_ids" gorm:"type:text"`
	MaxBodyBytes  int64  `json:"max_body_bytes" gorm:"not null;default:2097152"`
	RetentionDays int    `json:"retention_days" gorm:"not null;default:30"`
	UpdatedAt     int64  `json:"updated_at" gorm:"not null;default:0"`
	UpdatedBy     int    `json:"updated_by" gorm:"not null;default:0"`
}

func (ConversationArchiveConfig) TableName() string { return "conversation_archive_configs" }

// ConversationArchive 保存经过清洗的对话内容，不包含请求头、鉴权信息或媒体数据。
// Content 使用跨数据库大文本类型，以支持较长的多轮对话。
type ConversationArchive struct {
	Id                int64                   `json:"id" gorm:"primaryKey;index"`
	RequestId         string                  `json:"request_id" gorm:"type:varchar(128);index"`
	UserId            int                     `json:"user_id" gorm:"index"`
	Username          string                  `json:"username" gorm:"type:varchar(128)"`
	GroupId           int                     `json:"group_id" gorm:"index"`
	GroupCode         string                  `json:"group_code" gorm:"type:varchar(128);index"`
	GroupName         string                  `json:"group_name" gorm:"type:varchar(128)"`
	Model             string                  `json:"model" gorm:"type:varchar(255);index"`
	Protocol          string                  `json:"protocol" gorm:"type:varchar(32)"`
	MessageCount      int                     `json:"message_count"`
	ByteSize          int                     `json:"byte_size"`
	Content           RequestArchiveLargeText `json:"content" gorm:"not null"`
	ContentCipherKind string                  `json:"-" gorm:"type:varchar(16);not null;default:'plaintext_v1'"`
	CreatedAt         int64                   `json:"created_at" gorm:"index"`
	ExpiresAt         int64                   `json:"expires_at" gorm:"index"`
}

func (ConversationArchive) TableName() string { return "conversation_archives" }

func MigrateConversationArchive() error {
	if DB == nil {
		return errors.New("数据库尚未初始化")
	}
	if err := DB.AutoMigrate(&ConversationArchiveConfig{}, &ConversationArchive{}); err != nil {
		return err
	}
	return EnsureConversationArchiveConfig()
}

// EnsureConversationArchiveConfig 补齐归档配置单例。它可在迁移和首次保存前
// 幂等调用，避免管理员直接 PUT 配置时因尚未访问 GET 而缺少 ID=1 记录。
func EnsureConversationArchiveConfig() error {
	if DB == nil {
		return errors.New("数据库尚未初始化")
	}
	var current ConversationArchiveConfig
	defaults := ConversationArchiveConfig{
		Id: ConversationArchiveConfigID, ConfigVersion: 1, GroupCodes: "[]", UserIds: "[]",
		MaxBodyBytes: 2 * 1024 * 1024, RetentionDays: 30, UpdatedAt: time.Now().Unix(),
	}
	if err := DB.First(&current, ConversationArchiveConfigID).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if createErr := DB.Create(&defaults).Error; createErr != nil {
			// 并发启动时另一个实例可能刚插入 ID=1；重新读取即可继续。
			if loadErr := DB.First(&current, ConversationArchiveConfigID).Error; loadErr != nil {
				return createErr
			}
		} else {
			current = defaults
		}
	}
	updates := map[string]interface{}{}
	if current.GroupCodes == "" {
		updates["group_codes"] = "[]"
	}
	if current.UserIds == "" {
		updates["user_ids"] = "[]"
	}
	if current.MaxBodyBytes == 0 {
		updates["max_body_bytes"] = 2 * 1024 * 1024
	}
	if current.RetentionDays == 0 {
		updates["retention_days"] = 30
	}
	if len(updates) > 0 {
		return DB.Model(&current).Updates(updates).Error
	}
	return nil
}

func NormalizeConversationArchiveGroupCode(code string) string {
	// 分组标识沿用主模型的大小写语义；这里只去除首尾空白。
	return strings.TrimSpace(code)
}

func NewConversationArchiveExpiry(retentionDays int) int64 {
	if retentionDays <= 0 {
		retentionDays = 30
	}
	return time.Now().Add(time.Duration(retentionDays) * 24 * time.Hour).Unix()
}

// DeleteExpiredConversationArchiveBatch 删除已过期的归档正文，按 ID 分批执行，
// 避免在 PostgreSQL 等不支持 DELETE ... LIMIT 的数据库上使用方言 SQL。
func DeleteExpiredConversationArchiveBatch(ctx context.Context, now int64, limit int) (int64, error) {
	if DB == nil {
		return 0, errors.New("数据库尚未初始化")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	if limit <= 0 {
		limit = 500
	}
	var ids []int64
	if err := DB.WithContext(ctx).Model(&ConversationArchive{}).
		Where("expires_at > 0 AND expires_at <= ?", now).
		Order("id asc").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := DB.WithContext(ctx).Where("id IN ?", ids).Delete(&ConversationArchive{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}
