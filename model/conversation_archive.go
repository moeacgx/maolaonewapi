package model

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	ConversationArchiveConfigID        = 1
	ConversationArchiveDefaultMaxCount = 1000
	ConversationArchiveMaximumMaxCount = 100000
	conversationArchiveTrimBatchSize   = 1000
)

type ConversationArchiveConfig struct {
	Id              int    `json:"id" gorm:"primaryKey"`
	ConfigVersion   int64  `json:"config_version" gorm:"not null;default:1"`
	Enabled         bool   `json:"enabled" gorm:"not null"`
	GroupCodes      string `json:"group_codes" gorm:"type:text"`
	UserIds         string `json:"user_ids" gorm:"type:text"`
	MaxBodyBytes    int64  `json:"max_body_bytes" gorm:"not null;default:2097152"`
	RetentionDays   int    `json:"retention_days" gorm:"not null;default:30"`
	MaxArchiveCount int    `json:"max_archive_count" gorm:"not null;default:1000"`
	UpdatedAt       int64  `json:"updated_at" gorm:"not null;default:0"`
	UpdatedBy       int    `json:"updated_by" gorm:"not null;default:0"`
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
		MaxBodyBytes: 2 * 1024 * 1024, RetentionDays: 30, MaxArchiveCount: ConversationArchiveDefaultMaxCount,
		UpdatedAt: time.Now().Unix(),
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
	if current.MaxArchiveCount <= 0 || current.MaxArchiveCount > ConversationArchiveMaximumMaxCount {
		updates["max_archive_count"] = ConversationArchiveDefaultMaxCount
	}
	if len(updates) > 0 {
		return DB.Model(&current).Updates(updates).Error
	}
	return nil
}

// GetConversationArchiveConfigForUpdate 取得归档配置单例的事务锁，供写入、裁剪和
// 清空操作串行执行，确保多实例下也不会突破会话数上限。
func GetConversationArchiveConfigForUpdate(ctx context.Context, tx *gorm.DB) (*ConversationArchiveConfig, error) {
	if tx == nil {
		return nil, errors.New("数据库事务尚未初始化")
	}
	var config ConversationArchiveConfig
	if err := lockForUpdate(tx.WithContext(ctx)).First(&config, ConversationArchiveConfigID).Error; err != nil {
		return nil, err
	}
	return &config, nil
}

// TrimConversationArchivesWithLimit 在已锁定配置单例的事务中删除超出上限的最旧归档。
// 先查 ID 再删除，避免依赖各数据库不一致的 DELETE ... LIMIT 语法。
func TrimConversationArchivesWithLimit(tx *gorm.DB, maxArchiveCount int) (int64, error) {
	if tx == nil {
		return 0, errors.New("数据库事务尚未初始化")
	}
	if maxArchiveCount <= 0 || maxArchiveCount > ConversationArchiveMaximumMaxCount {
		maxArchiveCount = ConversationArchiveDefaultMaxCount
	}
	var deleted int64
	for {
		var count int64
		if err := tx.Model(&ConversationArchive{}).Count(&count).Error; err != nil {
			return 0, err
		}
		overflow := count - int64(maxArchiveCount)
		if overflow <= 0 {
			return deleted, nil
		}
		batchSize := int64(conversationArchiveTrimBatchSize)
		if overflow < batchSize {
			batchSize = overflow
		}
		ids := make([]int64, 0, batchSize)
		if err := tx.Model(&ConversationArchive{}).
			Order("created_at ASC").Order("id ASC").Limit(int(batchSize)).Pluck("id", &ids).Error; err != nil {
			return 0, err
		}
		if len(ids) == 0 {
			return 0, errors.New("对话归档容量裁剪未找到待删除记录")
		}
		result := tx.Where("id IN ?", ids).Delete(&ConversationArchive{})
		if result.Error != nil {
			return 0, result.Error
		}
		if result.RowsAffected == 0 {
			return 0, errors.New("对话归档容量裁剪期间记录已变化")
		}
		deleted += result.RowsAffected
	}
}

// CreateConversationArchiveWithLimit 写入归档后立即裁剪，以全局配置单例锁保证
// 多实例并发写入时始终保留最新的有限记录。
func CreateConversationArchiveWithLimit(ctx context.Context, archive *ConversationArchive) error {
	if DB == nil {
		return errors.New("数据库尚未初始化")
	}
	if archive == nil {
		return errors.New("对话归档不能为空")
	}
	if err := EnsureConversationArchiveConfig(); err != nil {
		return err
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		config, err := GetConversationArchiveConfigForUpdate(ctx, tx)
		if err != nil {
			return err
		}
		if err := tx.Create(archive).Error; err != nil {
			return err
		}
		_, err = TrimConversationArchivesWithLimit(tx, config.MaxArchiveCount)
		return err
	})
}

// ClearConversationArchives 删除所有已存归档。调用方必须先完成 Root 权限和确认校验。
func ClearConversationArchives(ctx context.Context) (int64, error) {
	if DB == nil {
		return 0, errors.New("数据库尚未初始化")
	}
	if err := EnsureConversationArchiveConfig(); err != nil {
		return 0, err
	}
	var deleted int64
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := GetConversationArchiveConfigForUpdate(ctx, tx); err != nil {
			return err
		}
		result := tx.Where("id > ?", 0).Delete(&ConversationArchive{})
		if result.Error != nil {
			return result.Error
		}
		deleted = result.RowsAffected
		return nil
	})
	return deleted, err
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

// DeleteExpiredConversationArchiveBatch 删除已过期的归档正文，按 ID 分批执行。
// 它与写入、容量裁剪和手动清空持有同一配置锁，避免竞态删除同一批记录。
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
	if err := EnsureConversationArchiveConfig(); err != nil {
		return 0, err
	}
	var deleted int64
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := GetConversationArchiveConfigForUpdate(ctx, tx); err != nil {
			return err
		}
		var ids []int64
		if err := tx.Model(&ConversationArchive{}).
			Where("expires_at > 0 AND expires_at <= ?", now).
			Order("id asc").Limit(limit).Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		result := tx.Where("id IN ?", ids).Delete(&ConversationArchive{})
		if result.Error != nil {
			return result.Error
		}
		deleted = result.RowsAffected
		return nil
	})
	return deleted, err
}
