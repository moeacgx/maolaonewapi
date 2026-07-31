package model

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

const (
	RequestArchiveConfigID = 1

	RequestArchiveTargetLocal = "local"
	RequestArchiveTargetS3    = "s3"

	RequestArchiveScopeAllRequests = "all_requests"
	RequestArchiveScopeAuditEvents = "audit_events"

	RequestArchiveJobQueued     = "queued"
	RequestArchiveJobProcessing = "processing"
	RequestArchiveJobRetry      = "retry"
	RequestArchiveJobDone       = "done"
	RequestArchiveJobFailed     = "failed"

	RequestArchiveObjectVersionUnknown     = ""
	RequestArchiveObjectVersionExact       = "exact"
	RequestArchiveObjectVersionUnversioned = "unversioned"
	RequestArchiveObjectVersionAbsent      = "absent"

	RequestArchiveJobMaxAttempts = 3
	// RequestArchiveMinimumDeliveryWindow 必须覆盖单次对象写入的最大客户端
	// 等待时间，避免任务在保留期结束前一刻才开始不可逆的外部写入。
	RequestArchiveMinimumDeliveryWindow = 5 * time.Minute

	RequestArchiveDefaultMaxBodyBytes  int64 = 64 * 1024 * 1024
	RequestArchiveDefaultQueueMaxBytes int64 = 1024 * 1024 * 1024
	RequestArchiveMaximumBodyBytes     int64 = 128 * 1024 * 1024

	requestArchiveBatchSize = 500
)

// RequestArchiveLargeText 让归档密文在 MySQL 中使用 LONGTEXT，避免超过
// TEXT 的 64 KiB 上限；SQLite 和 PostgreSQL 继续使用标准 TEXT。
type RequestArchiveLargeText string

func (RequestArchiveLargeText) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	if db != nil && db.Dialector != nil && db.Dialector.Name() == "mysql" {
		return "LONGTEXT"
	}
	return "TEXT"
}

// RequestArchiveConfig 保存 HTTP 正文与 Realtime 客户端帧归档的单例配置。存储目标独立建表，
// active_target_id 仅作用于后续新建任务，已入队任务始终保留自己的 target_id。
type RequestArchiveConfig struct {
	Id             int    `json:"id" gorm:"primaryKey"`
	ConfigVersion  int64  `json:"config_version" gorm:"not null;default:1"`
	Enabled        bool   `json:"enabled" gorm:"not null;default:false"`
	ArchiveScope   string `json:"archive_scope" gorm:"type:varchar(32);not null;default:'all_requests'"`
	ActiveTargetId string `json:"active_target_id" gorm:"type:varchar(64);not null;default:'';index"`
	RetentionDays  int    `json:"retention_days" gorm:"not null;default:30"`
	WorkerCount    int    `json:"worker_count" gorm:"not null;default:4"`
	QueueCapacity  int    `json:"queue_capacity" gorm:"not null;default:32768"`
	MaxBodyBytes   int64  `json:"max_body_bytes" gorm:"not null;default:67108864"`
	QueueMaxBytes  int64  `json:"queue_max_bytes" gorm:"not null;default:1073741824"`
	UpdatedAt      int64  `json:"updated_at" gorm:"not null;default:0"`
	UpdatedBy      int    `json:"updated_by" gorm:"not null;default:0"`
}

func (RequestArchiveConfig) TableName() string { return "request_archive_configs" }

// RequestArchiveTarget 只保存密钥的版本化密文。访问密钥字段绝不能从 API
// 回传，service 层会用显式的公开视图替代本模型。
type RequestArchiveTarget struct {
	Id                  string `json:"id" gorm:"type:varchar(64);primaryKey"`
	Name                string `json:"name" gorm:"type:varchar(128);not null"`
	Type                string `json:"type" gorm:"type:varchar(16);not null;index"`
	Enabled             bool   `json:"enabled" gorm:"not null;default:true;index"`
	LocalPath           string `json:"local_path" gorm:"type:text;not null;default:''"`
	Endpoint            string `json:"endpoint" gorm:"type:text;not null;default:''"`
	Bucket              string `json:"bucket" gorm:"type:varchar(255);not null;default:''"`
	Region              string `json:"region" gorm:"type:varchar(128);not null;default:''"`
	Prefix              string `json:"prefix" gorm:"type:varchar(512);not null;default:''"`
	PathStyle           bool   `json:"path_style" gorm:"not null;default:false"`
	AccessKeyCiphertext string `json:"-" gorm:"type:text;not null;default:''"`
	SecretKeyCiphertext string `json:"-" gorm:"type:text;not null;default:''"`
	CreatedAt           int64  `json:"created_at" gorm:"not null;default:0"`
	UpdatedAt           int64  `json:"updated_at" gorm:"not null;default:0"`
}

func (RequestArchiveTarget) TableName() string { return "request_archive_targets" }

// RequestArchiveJob 是数据库持久队列。request_ciphertext 默认是 AES-GCM 信封；
// 未配置 CRYPTO_SECRET 时的新本地任务使用明确的 plain_ra1 前缀保存明文，
// 不得写入 Authorization 或任意原始请求头。
type RequestArchiveJob struct {
	Id                int64                   `json:"id" gorm:"primaryKey;index:idx_request_archive_expiry,priority:3"`
	ArchiveId         string                  `json:"archive_id" gorm:"type:varchar(36);not null;default:'';index"`
	DedupeKey         *string                 `json:"-" gorm:"type:varchar(36);uniqueIndex"`
	AuditEventId      int64                   `json:"audit_event_id" gorm:"not null;default:0;index"`
	Status            string                  `json:"status" gorm:"type:varchar(24);not null;index:idx_request_archive_claim,priority:1;index:idx_request_archive_expiry,priority:1"`
	ClaimVersion      int64                   `json:"claim_version" gorm:"not null;default:0"`
	WorkerId          string                  `json:"worker_id" gorm:"type:varchar(128);not null;default:'';index"`
	LeaseUntil        int64                   `json:"lease_until" gorm:"not null;default:0;index:idx_request_archive_claim,priority:3"`
	Attempts          int                     `json:"attempts" gorm:"not null;default:0"`
	NextAttemptAt     int64                   `json:"next_attempt_at" gorm:"not null;default:0;index:idx_request_archive_claim,priority:2"`
	TargetId          string                  `json:"target_id" gorm:"type:varchar(64);not null;index"`
	ConfigVersion     int64                   `json:"config_version" gorm:"not null;default:0;index"`
	RequestCiphertext RequestArchiveLargeText `json:"-" gorm:"not null"`
	// RequestCipherFormat 标记正文是 ra3 加密信封还是无密钥兼容明文。
	// 空值按历史 ra3 处理，避免旧任务在迁移后改变语义。
	RequestCipherFormat string `json:"-" gorm:"type:varchar(16);not null;default:'ra3'"`
	// SHA256 为兼容列：ra1/ra2 保存原 SHA-256，ra3 保存任务密钥计算的 HMAC-SHA256；
	// 明文兼容任务保存普通 SHA-256，仅用于完整性校验。
	SHA256          string `json:"sha256" gorm:"type:char(64);not null;index"`
	ByteSize        int64  `json:"byte_size" gorm:"not null;default:0"`
	ContentType     string `json:"content_type" gorm:"type:varchar(255);not null;default:''"`
	Method          string `json:"method" gorm:"type:varchar(16);not null;default:''"`
	Path            string `json:"path" gorm:"type:text;not null;default:''"`
	RequestId       string `json:"request_id" gorm:"type:varchar(128);not null;default:'';index"`
	UserId          int    `json:"user_id" gorm:"not null;default:0;index"`
	Username        string `json:"username" gorm:"type:varchar(128);not null;default:''"`
	UserEmail       string `json:"user_email" gorm:"type:varchar(255);not null;default:''"`
	TokenId         int    `json:"token_id" gorm:"not null;default:0;index"`
	TokenName       string `json:"token_name" gorm:"type:varchar(128);not null;default:''"`
	GroupId         int    `json:"group_id" gorm:"not null;default:0;index"`
	GroupName       string `json:"group_name" gorm:"type:varchar(128);not null;default:''"`
	ObjectKey       string `json:"object_key" gorm:"type:varchar(768);not null;default:''"`
	ObjectVersionId string `json:"object_version_id" gorm:"type:text;not null;default:''"`
	// ObjectVersionMode 区分精确版本、明确无版本和确认不存在。空值只表示
	// 尚未协调，清理流程必须先等待静默期，且不得据此发送删除请求。
	ObjectVersionMode         string `json:"object_version_mode" gorm:"type:varchar(16);not null;default:''"`
	CleanupReconcileStartedAt int64  `json:"cleanup_reconcile_started_at" gorm:"not null;default:0"`
	LastErrorCode             string `json:"last_error_code" gorm:"type:varchar(64);not null;default:'';index"`
	LastErrorMessage          string `json:"last_error_message" gorm:"type:varchar(512);not null;default:''"`
	CreatedAt                 int64  `json:"created_at" gorm:"not null;default:0;index"`
	UpdatedAt                 int64  `json:"updated_at" gorm:"not null;default:0"`
	FinishedAt                int64  `json:"finished_at" gorm:"not null;default:0;index"`
	ExpiresAt                 int64  `json:"expires_at" gorm:"not null;default:0;index;index:idx_request_archive_expiry,priority:2"`
}

func (RequestArchiveJob) TableName() string { return "request_archive_jobs" }

// RequestArchiveQueueState 统计仍占用数据库密文容量的任务。queued、
// processing、retry 以及保留密文的 failed 都占用容量；成功写出并清空密文
// 或过期删除后才释放。所有变更与任务位于同一事务，保证多进程正确性。
type RequestArchiveQueueState struct {
	Id          int   `json:"id" gorm:"primaryKey"`
	ActiveCount int64 `json:"active_count" gorm:"not null;default:0"`
	ActiveBytes int64 `json:"active_bytes" gorm:"not null;default:0"`
	Version     int64 `json:"version" gorm:"not null;default:0"`
	UpdatedAt   int64 `json:"updated_at" gorm:"not null;default:0"`
}

func (RequestArchiveQueueState) TableName() string { return "request_archive_queue_states" }

var (
	ErrRequestArchiveConfigConflict    = errors.New("request archive config conflict")
	ErrRequestArchiveQueueFull         = errors.New("request archive queue is full")
	ErrRequestArchiveTargetInUse       = errors.New("request archive target still has retained jobs")
	ErrRequestArchiveConfigChanged     = errors.New("request archive config changed")
	ErrRequestArchiveTargetUnavailable = errors.New("request archive target unavailable")
	ErrRequestArchiveBodyTooLarge      = errors.New("request archive body too large")
	ErrRequestArchiveAlreadyQueued     = errors.New("request archive candidate already queued")
)

// MigrateRequestArchive 由应用启动迁移流程调用。保留独立函数让后续接线
// 不必在主迁移列表里复制表清单。
func MigrateRequestArchive() error {
	if DB == nil {
		return errors.New("请求归档数据库尚未初始化")
	}
	return DB.AutoMigrate(
		&RequestArchiveConfig{}, &RequestArchiveTarget{},
		&RequestArchiveJob{}, &RequestArchiveQueueState{},
	)
}

func AutoMigrateRequestArchive() error { return MigrateRequestArchive() }

func defaultRequestArchiveConfig() RequestArchiveConfig {
	return RequestArchiveConfig{
		Id: RequestArchiveConfigID, ConfigVersion: 1, ArchiveScope: RequestArchiveScopeAllRequests, RetentionDays: 30,
		WorkerCount: 4, QueueCapacity: 32768,
		MaxBodyBytes: RequestArchiveDefaultMaxBodyBytes, QueueMaxBytes: RequestArchiveDefaultQueueMaxBytes,
	}
}

func EnsureRequestArchiveDefaults() error {
	return EnsureRequestArchiveDefaultsContext(context.Background())
}

func EnsureRequestArchiveDefaultsContext(ctx context.Context) error {
	if DB == nil {
		return errors.New("请求归档数据库尚未初始化")
	}
	db := DB.WithContext(ctx)
	config := defaultRequestArchiveConfig()
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&config).Error; err != nil {
		return err
	}
	state := RequestArchiveQueueState{Id: RequestArchiveConfigID, UpdatedAt: time.Now().Unix()}
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error
}

func LoadRequestArchiveConfig(ctx context.Context) (*RequestArchiveConfig, []RequestArchiveTarget, error) {
	db := DB.WithContext(ctx)
	var config RequestArchiveConfig
	if err := db.First(&config, "id = ?", RequestArchiveConfigID).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, err
		}
		// 正常读取不再反复执行 INSERT ON CONFLICT；仅配置行确实缺失时
		// 才补齐单例配置和队列状态，兼顾热路径开销与并发首次启动。
		if err := EnsureRequestArchiveDefaultsContext(ctx); err != nil {
			return nil, nil, err
		}
		if err := db.First(&config, "id = ?", RequestArchiveConfigID).Error; err != nil {
			return nil, nil, err
		}
	}
	if strings.TrimSpace(config.ArchiveScope) == "" {
		config.ArchiveScope = RequestArchiveScopeAllRequests
	}
	var targets []RequestArchiveTarget
	if err := db.Order("created_at ASC, id ASC").Find(&targets).Error; err != nil {
		return nil, nil, err
	}
	return &config, targets, nil
}

func GetRequestArchiveTarget(ctx context.Context, id string) (*RequestArchiveTarget, error) {
	if strings.TrimSpace(id) == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var target RequestArchiveTarget
	if err := DB.WithContext(ctx).First(&target, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &target, nil
}

// SaveRequestArchiveConfig 使用 config_version 作为 CAS。不存在于 targets
// 的旧目标只会在没有活动任务引用时才删除，避免切换主目标后丢失待重试任务。
func SaveRequestArchiveConfig(ctx context.Context, expectedVersion int64, config *RequestArchiveConfig, targets []RequestArchiveTarget) error {
	if config == nil {
		return errors.New("request archive config is nil")
	}
	if err := EnsureRequestArchiveDefaultsContext(ctx); err != nil {
		return err
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().Unix()
		config.Id = RequestArchiveConfigID
		config.ConfigVersion = expectedVersion + 1
		config.UpdatedAt = now

		seen := make(map[string]struct{}, len(targets))
		for index := range targets {
			target := &targets[index]
			if !isCanonicalRequestArchiveTargetID(target.Id) {
				return errors.New("请求归档存储目标 ID 必须是小写 ASCII 标识")
			}
			if _, duplicated := seen[target.Id]; duplicated {
				return errors.New("请求归档存储目标 ID 重复")
			}
			seen[target.Id] = struct{}{}
			if target.CreatedAt == 0 {
				target.CreatedAt = now
			}
			target.UpdatedAt = now
		}
		if config.ActiveTargetId != "" {
			if !isCanonicalRequestArchiveTargetID(config.ActiveTargetId) {
				return errors.New("请求归档活动存储目标 ID 必须是小写 ASCII 标识")
			}
			if _, ok := seen[config.ActiveTargetId]; !ok {
				return errors.New("请求归档活动存储目标不存在")
			}
		}

		result := tx.Model(&RequestArchiveConfig{}).
			Where("id = ? AND config_version = ?", RequestArchiveConfigID, expectedVersion).
			Updates(map[string]interface{}{
				"config_version":   config.ConfigVersion,
				"enabled":          config.Enabled,
				"archive_scope":    config.ArchiveScope,
				"active_target_id": config.ActiveTargetId,
				"retention_days":   config.RetentionDays,
				"worker_count":     config.WorkerCount,
				"queue_capacity":   config.QueueCapacity,
				"max_body_bytes":   config.MaxBodyBytes,
				"queue_max_bytes":  config.QueueMaxBytes,
				"updated_at":       now,
				"updated_by":       config.UpdatedBy,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrRequestArchiveConfigConflict
		}

		var existing []RequestArchiveTarget
		if err := tx.Find(&existing).Error; err != nil {
			return err
		}
		existingById := make(map[string]RequestArchiveTarget, len(existing))
		for _, target := range existing {
			existingById[target.Id] = target
		}
		for index := range targets {
			if prior, exists := existingById[targets[index].Id]; exists && requestArchiveTargetDeliveryChanged(prior, targets[index]) {
				inUse, err := requestArchiveTargetHasRetainedJobs(tx, targets[index].Id)
				if err != nil {
					return err
				}
				if inUse {
					return ErrRequestArchiveTargetInUse
				}
			}
			if err := tx.Save(&targets[index]).Error; err != nil {
				return err
			}
		}
		for _, target := range existing {
			if _, keep := seen[target.Id]; keep {
				continue
			}
			inUse, err := requestArchiveTargetHasRetainedJobs(tx, target.Id)
			if err != nil {
				return err
			}
			if inUse {
				return ErrRequestArchiveTargetInUse
			}
			if err := tx.Delete(&RequestArchiveTarget{}, "id = ?", target.Id).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// requestArchiveTargetDeliveryChanged 禁止在有待投递或仍在保留期对象时原地
// 改写位置或凭据。旧任务只持久化 target_id，切换存储必须新增目标后再切换，
// 否则保留期清理可能删除错误位置或遗留旧对象。
func requestArchiveTargetDeliveryChanged(before, after RequestArchiveTarget) bool {
	return before.Type != after.Type || before.LocalPath != after.LocalPath || before.Endpoint != after.Endpoint ||
		before.Bucket != after.Bucket || before.Region != after.Region || before.Prefix != after.Prefix ||
		before.PathStyle != after.PathStyle || before.AccessKeyCiphertext != after.AccessKeyCiphertext ||
		before.SecretKeyCiphertext != after.SecretKeyCiphertext
}

func requestArchiveTargetHasRetainedJobs(tx *gorm.DB, targetID string) (bool, error) {
	var count int64
	err := tx.Model(&RequestArchiveJob{}).
		Where("target_id = ? AND (status IN ? OR (status IN ? AND object_key <> ''))", targetID,
			[]string{RequestArchiveJobQueued, RequestArchiveJobProcessing, RequestArchiveJobRetry},
			[]string{RequestArchiveJobDone, RequestArchiveJobFailed}).
		Count(&count).Error
	return count > 0, err
}

func EnqueueRequestArchiveJob(ctx context.Context, job *RequestArchiveJob, _ int) error {
	if job == nil {
		return errors.New("request archive job is nil")
	}
	if strings.TrimSpace(job.TargetId) == "" {
		return errors.New("请求归档任务缺少存储目标")
	}
	if !isCanonicalRequestArchiveTargetID(job.TargetId) {
		return errors.New("请求归档任务存储目标 ID 无效")
	}
	if job.ConfigVersion < 1 {
		return ErrRequestArchiveConfigChanged
	}
	if job.ByteSize < 0 || job.ByteSize > RequestArchiveMaximumBodyBytes {
		return ErrRequestArchiveBodyTooLarge
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().Unix()
		var config RequestArchiveConfig
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&config, "id = ?", RequestArchiveConfigID).Error; err != nil {
			return err
		}
		if !config.Enabled || config.ConfigVersion != job.ConfigVersion || config.ActiveTargetId != job.TargetId {
			return ErrRequestArchiveConfigChanged
		}
		if config.MaxBodyBytes < 1 || config.MaxBodyBytes > RequestArchiveMaximumBodyBytes || job.ByteSize > config.MaxBodyBytes {
			return ErrRequestArchiveBodyTooLarge
		}
		var target RequestArchiveTarget
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&target, "id = ? AND enabled = ?", job.TargetId, true).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRequestArchiveTargetUnavailable
			}
			return err
		}
		if config.QueueCapacity < 1 || config.QueueMaxBytes < 1 || job.ByteSize > config.QueueMaxBytes {
			return ErrRequestArchiveQueueFull
		}
		state := RequestArchiveQueueState{Id: RequestArchiveConfigID, UpdatedAt: now}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
			return err
		}
		remainingBytes := config.QueueMaxBytes - job.ByteSize
		result := tx.Model(&RequestArchiveQueueState{}).
			Where("id = ? AND active_count < ? AND active_bytes <= ?", RequestArchiveConfigID, config.QueueCapacity, remainingBytes).
			Updates(map[string]interface{}{
				"active_count": gorm.Expr("active_count + ?", 1),
				"active_bytes": gorm.Expr("active_bytes + ?", job.ByteSize),
				"version":      gorm.Expr("version + ?", 1),
				"updated_at":   now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrRequestArchiveQueueFull
		}
		job.Status = RequestArchiveJobQueued
		job.NextAttemptAt = now
		// ra2/ra3 的 AAD 已绑定 service 层生成的 created_at。大正文加密可能
		// 跨越一秒，入队事务不得重写该值，否则持久化后的密文将无法解密。
		if job.CreatedAt <= 0 {
			job.CreatedAt = now
		}
		job.UpdatedAt = now
		if job.DedupeKey != nil {
			create := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "dedupe_key"}},
				DoNothing: true,
			}).Create(job)
			if create.Error == nil && create.RowsAffected == 0 {
				return ErrRequestArchiveAlreadyQueued
			}
			return create.Error
		}
		return tx.Create(job).Error
	})
}

func isCanonicalRequestArchiveTargetID(value string) bool {
	if value == "" || len(value) > 64 || value != strings.TrimSpace(value) || value != strings.ToLower(value) {
		return false
	}
	for _, runeValue := range value {
		if (runeValue >= 'a' && runeValue <= 'z') || (runeValue >= '0' && runeValue <= '9') ||
			runeValue == '-' || runeValue == '_' {
			continue
		}
		return false
	}
	return true
}

// RequestArchiveQueueHasCapacity 是加密前的低成本预检，用来避免队列已满时
// 仍构造大密文。它只提供快速拒绝提示，最终正确性仍由入队事务的条件更新保证。
func RequestArchiveQueueHasCapacity(ctx context.Context, capacity int, capacityBytes, bodyBytes int64) (bool, error) {
	if capacity < 1 || capacityBytes < 1 || bodyBytes < 0 ||
		bodyBytes > RequestArchiveMaximumBodyBytes || bodyBytes > capacityBytes {
		return false, nil
	}
	var state RequestArchiveQueueState
	err := DB.WithContext(ctx).First(&state, "id = ?", RequestArchiveConfigID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return state.ActiveCount < int64(capacity) && state.ActiveBytes <= capacityBytes-bodyBytes, nil
}

// RequestArchiveJobCandidate 只包含领取前评估内存预算所需的字段，绝不把
// 大密文带入候选扫描。Worker 取得进程内预算后才调用条件领取并加载完整行。
type RequestArchiveJobCandidate struct {
	Id                  int64
	ClaimVersion        int64
	ByteSize            int64
	ArchiveId           string
	RequestCipherFormat string
}

func ListRequestArchiveJobCandidates(ctx context.Context, limit int) ([]RequestArchiveJobCandidate, error) {
	if limit < 1 || limit > 16 {
		limit = 16
	}
	nowTime := time.Now()
	now := nowTime.Unix()
	latestSafeExpiry := nowTime.Add(RequestArchiveMinimumDeliveryWindow).Unix()
	var candidates []RequestArchiveJobCandidate
	err := DB.WithContext(ctx).Model(&RequestArchiveJob{}).
		Select("id", "claim_version", "byte_size", "archive_id", "request_cipher_format").
		Where("status IN ? AND next_attempt_at <= ? AND (lease_until = 0 OR lease_until < ?) AND expires_at > ? AND byte_size >= 0 AND byte_size <= ?",
			[]string{RequestArchiveJobQueued, RequestArchiveJobRetry}, now, now, latestSafeExpiry, RequestArchiveMaximumBodyBytes).
		Order("id ASC").Limit(limit).Scan(&candidates).Error
	return candidates, err
}

// ClaimRequestArchiveJobCandidate 对候选版本做条件更新。调用方必须在进入
// 本函数前取得与 byte_size 对应的内存预算，因为成功后会加载完整密文行。
func ClaimRequestArchiveJobCandidate(ctx context.Context, workerId string, lease time.Duration, candidate RequestArchiveJobCandidate) (*RequestArchiveJob, error) {
	if strings.TrimSpace(workerId) == "" || lease <= 0 || candidate.Id <= 0 {
		return nil, errors.New("请求归档任务领取参数无效")
	}
	nowTime := time.Now()
	now := nowTime.Unix()
	latestSafeExpiry := nowTime.Add(RequestArchiveMinimumDeliveryWindow).Unix()
	leaseUntil := nowTime.Add(lease).Unix()
	db := DB.WithContext(ctx)
	result := db.Model(&RequestArchiveJob{}).
		Where("id = ? AND status IN ? AND claim_version = ? AND next_attempt_at <= ? AND (lease_until = 0 OR lease_until < ?) AND expires_at > ?",
			candidate.Id, []string{RequestArchiveJobQueued, RequestArchiveJobRetry}, candidate.ClaimVersion, now, now, latestSafeExpiry).
		Updates(map[string]interface{}{
			"status":        RequestArchiveJobProcessing,
			"worker_id":     workerId,
			"lease_until":   leaseUntil,
			"claim_version": gorm.Expr("claim_version + ?", 1),
			"attempts":      gorm.Expr("attempts + ?", 1),
			"updated_at":    now,
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, gorm.ErrRecordNotFound
	}
	var claimed RequestArchiveJob
	if err := db.First(&claimed, "id = ?", candidate.Id).Error; err != nil {
		return nil, err
	}
	return &claimed, nil
}

// ClaimRequestArchiveJob 使用条件更新领取候选任务。它不依赖 SKIP LOCKED，
// claim_version 与 worker_id 共同形成 fencing，防止过期 Worker 覆盖新状态。
func ClaimRequestArchiveJob(ctx context.Context, workerId string, lease time.Duration) (*RequestArchiveJob, error) {
	if strings.TrimSpace(workerId) == "" || lease <= 0 {
		return nil, errors.New("请求归档任务领取参数无效")
	}
	candidates, err := ListRequestArchiveJobCandidates(ctx, 16)
	if err != nil {
		return nil, err
	}
	for _, candidate := range candidates {
		claimed, claimErr := ClaimRequestArchiveJobCandidate(ctx, workerId, lease, candidate)
		if errors.Is(claimErr, gorm.ErrRecordNotFound) {
			continue
		}
		if claimErr != nil {
			return nil, claimErr
		}
		return claimed, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func RenewRequestArchiveJobLease(ctx context.Context, job *RequestArchiveJob, lease time.Duration) error {
	if job == nil || lease <= 0 {
		return errors.New("请求归档任务租约参数无效")
	}
	now := time.Now()
	leaseUntil := now.Add(lease).Unix()
	result := DB.WithContext(ctx).Model(&RequestArchiveJob{}).
		Where("id = ? AND status = ? AND claim_version = ? AND worker_id = ?",
			job.Id, RequestArchiveJobProcessing, job.ClaimVersion, job.WorkerId).
		Updates(map[string]interface{}{"lease_until": leaseUntil, "updated_at": now.Unix()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("request archive lease claim lost")
	}
	job.LeaseUntil = leaseUntil
	return nil
}

func RetryRequestArchiveJob(ctx context.Context, job *RequestArchiveJob, code, message string, next time.Time) error {
	if job == nil {
		return errors.New("request archive job is nil")
	}
	result := DB.WithContext(ctx).Model(&RequestArchiveJob{}).
		Where("id = ? AND status = ? AND claim_version = ? AND worker_id = ?",
			job.Id, RequestArchiveJobProcessing, job.ClaimVersion, job.WorkerId).
		Updates(map[string]interface{}{
			"status":             RequestArchiveJobRetry,
			"worker_id":          "",
			"lease_until":        0,
			"next_attempt_at":    next.Unix(),
			"last_error_code":    truncateRequestArchiveError(code),
			"last_error_message": truncateRequestArchiveError(message),
			"updated_at":         time.Now().Unix(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("request archive retry claim lost")
	}
	return nil
}

// FinishRequestArchiveJob 成功时清空数据库中仅用于可靠投递的密文副本；
// 已经原子写入外部存储的对象仍是版本化密文，避免数据库长期重复保存大正文。
func FinishRequestArchiveJob(ctx context.Context, job *RequestArchiveJob, objectKey, objectVersionID string) error {
	if job == nil || strings.TrimSpace(objectKey) == "" {
		return errors.New("请求归档任务完成参数无效")
	}
	if err := validateRequestArchiveObjectVersion(objectVersionID); err != nil {
		return err
	}
	versionMode := requestArchiveObjectVersionMode(objectVersionID)
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().Unix()
		result := tx.Model(&RequestArchiveJob{}).
			Where("id = ? AND status = ? AND claim_version = ? AND worker_id = ?",
				job.Id, RequestArchiveJobProcessing, job.ClaimVersion, job.WorkerId).
			Updates(map[string]interface{}{
				"status":              RequestArchiveJobDone,
				"worker_id":           "",
				"lease_until":         0,
				"object_key":          objectKey,
				"object_version_id":   objectVersionID,
				"object_version_mode": versionMode,
				"request_ciphertext":  "",
				"finished_at":         now,
				"updated_at":          now,
				"last_error_code":     "",
				"last_error_message":  "",
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("request archive completion claim lost")
		}
		return releaseRequestArchiveQueueCapacity(tx, now, 1, job.ByteSize)
	})
}

// FailRequestArchiveJob 保留失败任务的密文，方便在保留期内人工处理或后续
// 增加精确对象清理；失败消息必须由调用方传入稳定代码而不是底层错误正文。
func FailRequestArchiveJob(ctx context.Context, job *RequestArchiveJob, code, message string) error {
	if job == nil {
		return errors.New("request archive job is nil")
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().Unix()
		result := tx.Model(&RequestArchiveJob{}).
			Where("id = ? AND status = ? AND claim_version = ? AND worker_id = ?",
				job.Id, RequestArchiveJobProcessing, job.ClaimVersion, job.WorkerId).
			Updates(map[string]interface{}{
				"status":             RequestArchiveJobFailed,
				"worker_id":          "",
				"lease_until":        0,
				"finished_at":        now,
				"last_error_code":    truncateRequestArchiveError(code),
				"last_error_message": truncateRequestArchiveError(message),
				"updated_at":         now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("request archive failure claim lost")
		}
		// failed 任务仍保留密文，必须继续占用任务数和字节容量，直到
		// 保留期清理真正删除该行。
		return nil
	})
}

// MarkRequestArchiveJobObjectLocation 先持久化确定性对象键，再开始外部写入。
// 即使对象写成功后完成事务暂时失败，恢复 Worker 也能探测并精确清理该对象。
func MarkRequestArchiveJobObjectLocation(ctx context.Context, job *RequestArchiveJob, objectKey string) error {
	if job == nil || strings.TrimSpace(objectKey) == "" {
		return errors.New("请求归档对象键无效")
	}
	result := DB.WithContext(ctx).Model(&RequestArchiveJob{}).
		Where("id = ? AND status = ? AND claim_version = ? AND worker_id = ?", job.Id, RequestArchiveJobProcessing, job.ClaimVersion, job.WorkerId).
		Updates(map[string]interface{}{
			"object_key": objectKey,
			"updated_at": time.Now().Unix(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("request archive object location claim lost")
	}
	job.ObjectKey = objectKey
	return nil
}

func MarkRequestArchiveJobObjectVersion(ctx context.Context, job *RequestArchiveJob, objectVersionID string) error {
	if job == nil {
		return errors.New("请求归档任务为空")
	}
	if err := validateRequestArchiveObjectVersion(objectVersionID); err != nil {
		return err
	}
	versionMode := requestArchiveObjectVersionMode(objectVersionID)
	result := DB.WithContext(ctx).Model(&RequestArchiveJob{}).
		Where("id = ? AND status = ? AND claim_version = ? AND worker_id = ?", job.Id, RequestArchiveJobProcessing, job.ClaimVersion, job.WorkerId).
		Updates(map[string]interface{}{
			"object_version_id":   objectVersionID,
			"object_version_mode": versionMode,
			"updated_at":          time.Now().Unix(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("request archive object version claim lost")
	}
	job.ObjectVersionId = objectVersionID
	job.ObjectVersionMode = versionMode
	return nil
}

// MarkRequestArchiveCleanupObjectVersion 在保留期清理前补齐崩溃窗口中丢失的
// 版本信息。对象键和终态共同参与条件更新，避免把探测结果写入已经变化的任务。
func MarkRequestArchiveCleanupObjectVersion(ctx context.Context, id int64, objectKey, objectVersionID string) error {
	if id <= 0 || strings.TrimSpace(objectKey) == "" {
		return errors.New("请求归档清理对象版本参数无效")
	}
	if err := validateRequestArchiveObjectVersion(objectVersionID); err != nil {
		return err
	}
	versionMode := requestArchiveObjectVersionMode(objectVersionID)
	db := DB.WithContext(ctx)
	result := db.Model(&RequestArchiveJob{}).
		Where("id = ? AND object_key = ? AND status IN ? AND object_version_mode = ?", id, objectKey,
			[]string{RequestArchiveJobDone, RequestArchiveJobFailed}, RequestArchiveObjectVersionUnknown).
		Updates(map[string]interface{}{
			"object_version_id":            objectVersionID,
			"object_version_mode":          versionMode,
			"cleanup_reconcile_started_at": 0,
			"updated_at":                   time.Now().Unix(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	// 多实例维护线程可能同时得到相同结论。已由另一实例写入相同版本时
	// 视为成功；不同版本或任务状态变化都必须停止删除。
	var stored RequestArchiveJob
	if err := db.Select("id", "status", "object_key", "object_version_id", "object_version_mode", "cleanup_reconcile_started_at").
		First(&stored, "id = ?", id).Error; err != nil {
		return err
	}
	if stored.ObjectKey != objectKey || stored.ObjectVersionMode != versionMode || stored.ObjectVersionId != objectVersionID ||
		stored.CleanupReconcileStartedAt != 0 ||
		(stored.Status != RequestArchiveJobDone && stored.Status != RequestArchiveJobFailed) {
		return errors.New("request archive cleanup object version changed")
	}
	return nil
}

func requestArchiveObjectVersionMode(objectVersionID string) string {
	if objectVersionID == "" {
		return RequestArchiveObjectVersionUnversioned
	}
	return RequestArchiveObjectVersionExact
}

// BeginRequestArchiveCleanupReconciliation 在任何外部探测或删除前持久化静默期。
// unknown 任务可能对应仍在对象存储服务端执行的晚到写入，因此对象当前是否存在
// 都不能缩短静默期。
func BeginRequestArchiveCleanupReconciliation(ctx context.Context, id int64, objectKey string, now int64, quietPeriod time.Duration) (bool, error) {
	if id <= 0 || strings.TrimSpace(objectKey) == "" || now <= 0 || quietPeriod < time.Second {
		return false, errors.New("请求归档对象清理协调参数无效")
	}
	quietSeconds := int64(quietPeriod / time.Second)
	db := DB.WithContext(ctx)
	result := db.Model(&RequestArchiveJob{}).
		Where("id = ? AND object_key = ? AND status IN ? AND object_version_mode = ? AND cleanup_reconcile_started_at = 0", id, objectKey,
			[]string{RequestArchiveJobDone, RequestArchiveJobFailed}, RequestArchiveObjectVersionUnknown).
		Updates(map[string]interface{}{
			"cleanup_reconcile_started_at": now,
			"updated_at":                   now,
		})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 1 {
		return false, nil
	}
	var stored RequestArchiveJob
	if err := db.Select("id", "status", "object_key", "object_version_mode", "cleanup_reconcile_started_at").
		First(&stored, "id = ?", id).Error; err != nil {
		return false, err
	}
	if stored.ObjectKey != objectKey || stored.ObjectVersionMode != RequestArchiveObjectVersionUnknown ||
		stored.CleanupReconcileStartedAt <= 0 ||
		(stored.Status != RequestArchiveJobDone && stored.Status != RequestArchiveJobFailed) {
		return false, errors.New("request archive cleanup reconciliation changed")
	}
	return stored.CleanupReconcileStartedAt <= now-quietSeconds, nil
}

// ConfirmRequestArchiveCleanupAbsent 把静默期后的“不存在”结论固化为独立状态。
// 该 CAS 与 unknown -> exact/unversioned 的版本固化互斥，避免多实例按旧快照删行。
func ConfirmRequestArchiveCleanupAbsent(
	ctx context.Context,
	id int64,
	objectKey string,
	reconcileStartedAt int64,
	now int64,
	quietPeriod time.Duration,
) error {
	if id <= 0 || strings.TrimSpace(objectKey) == "" || reconcileStartedAt <= 0 || now <= 0 || quietPeriod < time.Second {
		return errors.New("请求归档对象不存在确认参数无效")
	}
	quietSeconds := int64(quietPeriod / time.Second)
	if reconcileStartedAt > now-quietSeconds {
		return errors.New("request archive cleanup quiet period not elapsed")
	}
	db := DB.WithContext(ctx)
	result := db.Model(&RequestArchiveJob{}).
		Where("id = ? AND object_key = ? AND status IN ? AND object_version_mode = ? AND cleanup_reconcile_started_at = ?", id, objectKey,
			[]string{RequestArchiveJobDone, RequestArchiveJobFailed}, RequestArchiveObjectVersionUnknown, reconcileStartedAt).
		Updates(map[string]interface{}{
			"object_version_mode": RequestArchiveObjectVersionAbsent,
			"updated_at":          now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var stored RequestArchiveJob
	if err := db.Select("id", "status", "object_key", "object_version_id", "object_version_mode", "cleanup_reconcile_started_at").
		First(&stored, "id = ?", id).Error; err != nil {
		return err
	}
	if stored.ObjectKey != objectKey || stored.ObjectVersionId != "" ||
		stored.ObjectVersionMode != RequestArchiveObjectVersionAbsent ||
		stored.CleanupReconcileStartedAt != reconcileStartedAt ||
		(stored.Status != RequestArchiveJobDone && stored.Status != RequestArchiveJobFailed) {
		return errors.New("request archive cleanup absence confirmation changed")
	}
	return nil
}

func releaseRequestArchiveQueueCapacity(tx *gorm.DB, now, count, bytes int64) error {
	if count < 0 {
		count = 0
	}
	if bytes < 0 {
		bytes = 0
	}
	return tx.Model(&RequestArchiveQueueState{}).
		Where("id = ?", RequestArchiveConfigID).
		Updates(map[string]interface{}{
			"active_count": gorm.Expr("CASE WHEN active_count >= ? THEN active_count - ? ELSE 0 END", count, count),
			"active_bytes": gorm.Expr("CASE WHEN active_bytes >= ? THEN active_bytes - ? ELSE 0 END", bytes, bytes),
			"version":      gorm.Expr("version + ?", 1),
			"updated_at":   now,
		}).Error
}

// RecoverExpiredRequestArchiveJobs 只用条件更新回收过期租约，避免旧 Worker
// 在恢复后覆盖已被其他进程领取的任务。
func RecoverExpiredRequestArchiveJobs(ctx context.Context, now int64) (int64, error) {
	if now <= 0 {
		return 0, errors.New("请求归档租约回收时间无效")
	}
	db := DB.WithContext(ctx)
	var candidates []RequestArchiveJob
	if err := db.Where("status = ? AND lease_until > 0 AND lease_until < ?", RequestArchiveJobProcessing, now).
		Select("id", "claim_version", "worker_id", "attempts").
		Order("id ASC").Limit(requestArchiveBatchSize).Find(&candidates).Error; err != nil {
		return 0, err
	}
	if len(candidates) == 0 {
		return 0, nil
	}
	var recovered int64
	err := db.Transaction(func(tx *gorm.DB) error {
		for _, candidate := range candidates {
			status, nextAttemptAt, finishedAt := RequestArchiveJobRetry, now, int64(0)
			message := "任务租约已过期，等待重新领取"
			if candidate.Attempts >= RequestArchiveJobMaxAttempts {
				status, nextAttemptAt, finishedAt = RequestArchiveJobFailed, 0, now
				message = "任务租约已过期且已达到最大重试次数"
			}
			result := tx.Model(&RequestArchiveJob{}).
				Where("id = ? AND status = ? AND claim_version = ? AND worker_id = ? AND lease_until > 0 AND lease_until < ?",
					candidate.Id, RequestArchiveJobProcessing, candidate.ClaimVersion, candidate.WorkerId, now).
				Updates(map[string]interface{}{
					"status":             status,
					"worker_id":          "",
					"lease_until":        0,
					"next_attempt_at":    nextAttemptAt,
					"last_error_code":    "request_archive_lease_expired",
					"last_error_message": message,
					"finished_at":        finishedAt,
					"updated_at":         now,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				continue
			}
			recovered++
		}
		return nil
	})
	return recovered, err
}

// ExpirePendingRequestArchiveJobs 将保留期内尚未成功投递的 queued/retry
// 任务直接终止。否则过期正文会在存储故障恢复后重新上传，违背保留期语义。
func ExpirePendingRequestArchiveJobs(ctx context.Context, now int64) (int64, error) {
	if now <= 0 {
		return 0, errors.New("请求归档过期处理时间无效")
	}
	db := DB.WithContext(ctx)
	var candidates []RequestArchiveJob
	if err := db.Where("status IN ? AND expires_at > 0 AND expires_at <= ?",
		[]string{RequestArchiveJobQueued, RequestArchiveJobRetry}, now).
		Select("id").Order("id ASC").Limit(requestArchiveBatchSize).Find(&candidates).Error; err != nil {
		return 0, err
	}
	if len(candidates) == 0 {
		return 0, nil
	}
	var expired int64
	err := db.Transaction(func(tx *gorm.DB) error {
		for _, candidate := range candidates {
			result := tx.Model(&RequestArchiveJob{}).
				Where("id = ? AND status IN ? AND expires_at > 0 AND expires_at <= ?", candidate.Id,
					[]string{RequestArchiveJobQueued, RequestArchiveJobRetry}, now).
				Updates(map[string]interface{}{
					"status":             RequestArchiveJobFailed,
					"worker_id":          "",
					"lease_until":        0,
					"next_attempt_at":    0,
					"last_error_code":    "request_archive_expired",
					"last_error_message": "归档任务已超过保留期，未写入外部存储",
					"finished_at":        now,
					"updated_at":         now,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 1 {
				expired++
			}
		}
		return nil
	})
	return expired, err
}

// ListExpiredRequestArchiveJobs 供 service 层按任务记录精确删除外部对象。
// 该查询绝不返回工作中任务，也不会枚举本地目录或 bucket。
func ListExpiredRequestArchiveJobs(ctx context.Context, now int64, batch int) ([]RequestArchiveJob, error) {
	return ListExpiredRequestArchiveJobsAfter(ctx, now, batch, 0)
}

// ListExpiredRequestArchiveJobsAfter 只读取精确清理所需的字段。不得把失败
// 任务中仍待重试的密文整体读入内存，避免清理线程放大大请求内存占用。
func ListExpiredRequestArchiveJobsAfter(ctx context.Context, now int64, batch int, afterID int64) ([]RequestArchiveJob, error) {
	if now <= 0 {
		return nil, errors.New("请求归档清理截止时间无效")
	}
	if batch < 1 || batch > requestArchiveBatchSize {
		batch = requestArchiveBatchSize
	}
	var jobs []RequestArchiveJob
	err := DB.WithContext(ctx).Select("id", "status", "target_id", "byte_size", "object_key", "object_version_id", "object_version_mode", "cleanup_reconcile_started_at").
		Where("id > ? AND status IN ? AND expires_at > 0 AND expires_at <= ?", afterID,
			[]string{RequestArchiveJobDone, RequestArchiveJobFailed}, now).
		Order("id ASC").Limit(batch).Find(&jobs).Error
	return jobs, err
}

// RequestArchiveObjectCleanupMatch 是外部对象删除完成时的持久状态快照。
// 最终删行必须逐字段匹配，不能只依赖任务 ID。
type RequestArchiveObjectCleanupMatch struct {
	Id                        int64
	Status                    string
	ByteSize                  int64
	ObjectKey                 string
	ObjectVersionId           string
	ObjectVersionMode         string
	CleanupReconcileStartedAt int64
}

// DeleteExpiredRequestArchiveObjectJobs 只删除已按相同版本状态完成外部清理的任务。
// exact、unversioned 与 absent 都是由条件迁移固化的状态；unknown 永不允许删行。
func DeleteExpiredRequestArchiveObjectJobs(
	ctx context.Context,
	matches []RequestArchiveObjectCleanupMatch,
	now int64,
) (int64, error) {
	if len(matches) == 0 {
		return 0, nil
	}
	if now <= 0 {
		return 0, errors.New("请求归档清理截止时间无效")
	}
	seen := make(map[int64]struct{}, len(matches))
	for _, match := range matches {
		if match.Id <= 0 || strings.TrimSpace(match.ObjectKey) == "" || match.ByteSize < 0 ||
			(match.Status != RequestArchiveJobDone && match.Status != RequestArchiveJobFailed) {
			return 0, errors.New("请求归档对象清理快照无效")
		}
		if _, exists := seen[match.Id]; exists {
			return 0, errors.New("请求归档对象清理快照重复")
		}
		seen[match.Id] = struct{}{}
		switch match.ObjectVersionMode {
		case RequestArchiveObjectVersionExact:
			if match.ObjectVersionId == "" || match.CleanupReconcileStartedAt != 0 {
				return 0, errors.New("请求归档精确版本清理快照无效")
			}
		case RequestArchiveObjectVersionUnversioned:
			if match.ObjectVersionId != "" || match.CleanupReconcileStartedAt != 0 {
				return 0, errors.New("请求归档无版本清理快照无效")
			}
		case RequestArchiveObjectVersionAbsent:
			if match.ObjectVersionId != "" || match.CleanupReconcileStartedAt <= 0 {
				return 0, errors.New("请求归档不存在清理快照无效")
			}
		default:
			return 0, errors.New("请求归档对象版本状态未确认")
		}
	}

	var removed int64
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var retainedCount, retainedBytes int64
		for _, match := range matches {
			result := tx.Where(
				"id = ? AND status = ? AND byte_size = ? AND expires_at > 0 AND expires_at <= ? AND object_key = ? AND object_version_id = ? AND object_version_mode = ? AND cleanup_reconcile_started_at = ?",
				match.Id, match.Status, match.ByteSize, now, match.ObjectKey, match.ObjectVersionId,
				match.ObjectVersionMode, match.CleanupReconcileStartedAt,
			).Delete(&RequestArchiveJob{})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				continue
			}
			removed++
			if match.Status == RequestArchiveJobFailed {
				retainedCount++
				retainedBytes += match.ByteSize
			}
		}
		if retainedCount == 0 {
			return nil
		}
		return releaseRequestArchiveQueueCapacity(tx, time.Now().Unix(), retainedCount, retainedBytes)
	})
	return removed, err
}

// DeleteExpiredRequestArchiveJobs 只清理从未产生外部对象键的过期失败任务。
// 带对象键的任务必须使用 DeleteExpiredRequestArchiveObjectJobs 提交条件快照。
func DeleteExpiredRequestArchiveJobs(ctx context.Context, ids []int64, now int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	if now <= 0 {
		return 0, errors.New("请求归档清理截止时间无效")
	}
	var removed int64
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var jobs []RequestArchiveJob
		if err := tx.Select("id", "status", "byte_size").
			Where("id IN ? AND status = ? AND object_key = '' AND expires_at > 0 AND expires_at <= ?", ids,
				RequestArchiveJobFailed, now).
			Find(&jobs).Error; err != nil {
			return err
		}
		var retainedCount, retainedBytes int64
		for _, job := range jobs {
			result := tx.Where("id = ? AND status = ? AND object_key = '' AND expires_at > 0 AND expires_at <= ?", job.Id, job.Status, now).
				Delete(&RequestArchiveJob{})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				continue
			}
			removed++
			if job.Status == RequestArchiveJobFailed {
				retainedCount++
				retainedBytes += job.ByteSize
			}
		}
		if retainedCount == 0 {
			return nil
		}
		return releaseRequestArchiveQueueCapacity(tx, time.Now().Unix(), retainedCount, retainedBytes)
	})
	return removed, err
}

// RecordRequestArchiveCleanupError 只记录固定错误代码，不接受底层 endpoint
// 或凭据错误文本，避免管理日志意外暴露存储配置。
func RecordRequestArchiveCleanupError(ctx context.Context, id int64, code string) error {
	result := DB.WithContext(ctx).Model(&RequestArchiveJob{}).Where("id = ? AND status IN ?", id,
		[]string{RequestArchiveJobDone, RequestArchiveJobFailed}).
		Updates(map[string]interface{}{
			"last_error_code":    truncateRequestArchiveError(code),
			"last_error_message": "归档对象清理失败，等待下次精确重试",
			"updated_at":         time.Now().Unix(),
		})
	return result.Error
}

// CleanupFinishedRequestArchiveJobs 仅作为不含外部对象的失败任务清理兜底。
// 成功任务必须经 service 的精确对象删除流程清理，不能直接在模型层删行。
func CleanupFinishedRequestArchiveJobs(ctx context.Context, now int64, batch int) (int64, error) {
	if now <= 0 {
		return 0, errors.New("请求归档清理截止时间无效")
	}
	if batch < 1 || batch > requestArchiveBatchSize {
		batch = requestArchiveBatchSize
	}
	db := DB.WithContext(ctx)
	var jobs []RequestArchiveJob
	if err := db.Where("status = ? AND object_key = '' AND expires_at > 0 AND expires_at <= ?", RequestArchiveJobFailed, now).
		Order("id ASC").Limit(batch).Select("id").Find(&jobs).Error; err != nil {
		return 0, err
	}
	if len(jobs) == 0 {
		return 0, nil
	}
	ids := make([]int64, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.Id)
	}
	return DeleteExpiredRequestArchiveJobs(ctx, ids, now)
}

func GetRequestArchiveJob(ctx context.Context, id int64) (*RequestArchiveJob, error) {
	var job RequestArchiveJob
	if err := DB.WithContext(ctx).First(&job, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

type RequestArchiveRuntimeCounts struct {
	Queued         int64 `json:"queued"`
	Processing     int64 `json:"processing"`
	Retry          int64 `json:"retry"`
	Done           int64 `json:"done"`
	Failed         int64 `json:"failed"`
	Active         int64 `json:"active"`
	Capacity       int64 `json:"capacity"`
	ActiveBytes    int64 `json:"active_bytes"`
	CapacityBytes  int64 `json:"capacity_bytes"`
	OldestQueuedAt int64 `json:"oldest_queued_at"`
}

func GetRequestArchiveRuntimeCounts(ctx context.Context, capacity int, capacityBytes ...int64) (RequestArchiveRuntimeCounts, error) {
	db := DB.WithContext(ctx)
	result := RequestArchiveRuntimeCounts{Capacity: int64(capacity)}
	if len(capacityBytes) > 0 && capacityBytes[0] > 0 {
		result.CapacityBytes = capacityBytes[0]
	}
	type countRow struct {
		Status string
		Count  int64
	}
	var rows []countRow
	if err := db.Model(&RequestArchiveJob{}).Select("status, COUNT(*) AS count").Group("status").Scan(&rows).Error; err != nil {
		return result, err
	}
	for _, row := range rows {
		switch row.Status {
		case RequestArchiveJobQueued:
			result.Queued = row.Count
		case RequestArchiveJobProcessing:
			result.Processing = row.Count
		case RequestArchiveJobRetry:
			result.Retry = row.Count
		case RequestArchiveJobDone:
			result.Done = row.Count
		case RequestArchiveJobFailed:
			result.Failed = row.Count
		}
	}
	result.Active = result.Queued + result.Processing + result.Retry
	var state RequestArchiveQueueState
	if err := db.First(&state, "id = ?", RequestArchiveConfigID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return result, err
	} else if err == nil {
		result.Active = state.ActiveCount
		result.ActiveBytes = state.ActiveBytes
	}
	if err := db.Model(&RequestArchiveJob{}).
		Where("status IN ?", []string{RequestArchiveJobQueued, RequestArchiveJobRetry}).
		Select("COALESCE(MIN(created_at), 0)").Scan(&result.OldestQueuedAt).Error; err != nil {
		return result, err
	}
	return result, nil
}

func truncateRequestArchiveError(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > 500 {
		runes = runes[:500]
	}
	return string(runes)
}

func validateRequestArchiveObjectVersion(value string) error {
	if len(value) > 4096 || strings.ContainsRune(value, 0) {
		return errors.New("请求归档对象版本标识无效")
	}
	return nil
}
