package service

import "github.com/QuantumNous/new-api/model"

const (
	RequestArchiveSecretKeep    = "keep"
	RequestArchiveSecretReplace = "replace"
	RequestArchiveSecretClear   = "clear"

	RequestArchiveDefaultRetentionDays = 30
	RequestArchiveDefaultWorkerCount   = 4
	RequestArchiveDefaultQueueCapacity = 32768
)

// RequestArchiveTarget 是管理接口的脱敏目标视图，绝不包含存储访问密钥。
type RequestArchiveTarget struct {
	Id                  string `json:"id"`
	Name                string `json:"name"`
	Type                string `json:"type"`
	Enabled             bool   `json:"enabled"`
	LocalPath           string `json:"local_path,omitempty"`
	Endpoint            string `json:"endpoint,omitempty"`
	Bucket              string `json:"bucket,omitempty"`
	Region              string `json:"region,omitempty"`
	Prefix              string `json:"prefix,omitempty"`
	PathStyle           bool   `json:"path_style"`
	AccessKeyConfigured bool   `json:"access_key_configured"`
	SecretKeyConfigured bool   `json:"secret_key_configured"`
	CreatedAt           int64  `json:"created_at"`
	UpdatedAt           int64  `json:"updated_at"`
}

// RequestArchiveConfig 是独立安全审计页面使用的公开配置视图。
type RequestArchiveConfig struct {
	ConfigVersion   int64                  `json:"config_version"`
	Enabled         bool                   `json:"enabled"`
	ArchiveScope    string                 `json:"archive_scope"`
	EventChannelIds []int                  `json:"event_channel_ids"`
	EventGroupCodes []string               `json:"event_group_codes"`
	EventSources    []string               `json:"event_sources"`
	ActiveTargetId  string                 `json:"active_target_id"`
	RetentionDays   int                    `json:"retention_days"`
	WorkerCount     int                    `json:"worker_count"`
	QueueCapacity   int                    `json:"queue_capacity"`
	MaxBodyBytes    int64                  `json:"max_body_bytes"`
	QueueMaxBytes   int64                  `json:"queue_max_bytes"`
	Targets         []RequestArchiveTarget `json:"targets"`
}

// RequestArchiveUpdateTarget 的密钥仅能作为请求输入。服务层按 keep/replace/
// clear 执行，不会把输入内容附着到任何返回值或日志。
type RequestArchiveUpdateTarget struct {
	Id              string `json:"id"`
	Name            string `json:"name"`
	Type            string `json:"type"`
	Enabled         bool   `json:"enabled"`
	LocalPath       string `json:"local_path"`
	Endpoint        string `json:"endpoint"`
	Bucket          string `json:"bucket"`
	Region          string `json:"region"`
	Prefix          string `json:"prefix"`
	PathStyle       bool   `json:"path_style"`
	AccessKeyAction string `json:"access_key_action"`
	AccessKey       string `json:"access_key"`
	SecretKeyAction string `json:"secret_key_action"`
	SecretKey       string `json:"secret_key"`
}

type RequestArchiveUpdateRequest struct {
	ExpectedConfigVersion int64                        `json:"expected_version"`
	Enabled               bool                         `json:"enabled"`
	ArchiveScope          string                       `json:"archive_scope"`
	EventChannelIds       []int                        `json:"event_channel_ids"`
	EventGroupCodes       []string                     `json:"event_group_codes"`
	EventSources          []string                     `json:"event_sources"`
	ActiveTargetId        string                       `json:"active_target_id"`
	RetentionDays         int                          `json:"retention_days"`
	WorkerCount           int                          `json:"worker_count"`
	QueueCapacity         int                          `json:"queue_capacity"`
	MaxBodyBytes          int64                        `json:"max_body_bytes"`
	QueueMaxBytes         int64                        `json:"queue_max_bytes"`
	Targets               []RequestArchiveUpdateTarget `json:"targets"`
}

// RequestArchiveRequest 是 Relay 在认证和正文快照后提交的最小输入。请求头
// 不属于该契约，特别是 Authorization 永远不会进入归档表或对象存储。
type RequestArchiveRequest struct {
	Body         []byte `json:"-"`
	ArchiveId    string `json:"-"`
	DedupeKey    string `json:"-"`
	AuditEventId int64  `json:"-"`
	ContentType  string `json:"content_type"`
	Method       string `json:"method"`
	Path         string `json:"path"`
	RequestId    string `json:"request_id"`
	UserId       int    `json:"user_id"`
	Username     string `json:"username"`
	UserEmail    string `json:"user_email"`
	TokenId      int    `json:"token_id"`
	TokenName    string `json:"token_name"`
	GroupId      int    `json:"group_id"`
	GroupName    string `json:"group_name"`
}

type RequestArchiveEnqueueResult struct {
	Enqueued bool                        `json:"enqueued"`
	JobId    int64                       `json:"job_id,omitempty"`
	Status   RequestArchiveEnqueueStatus `json:"-"`
}

type RequestArchiveEnqueueStatus string

const (
	RequestArchiveEnqueueStatusNoop          RequestArchiveEnqueueStatus = "noop"
	RequestArchiveEnqueueStatusEnqueued      RequestArchiveEnqueueStatus = "enqueued"
	RequestArchiveEnqueueStatusAlreadyQueued RequestArchiveEnqueueStatus = "already_queued"
)

func (result RequestArchiveEnqueueResult) accepted() bool {
	return result.Status == RequestArchiveEnqueueStatusEnqueued ||
		result.Status == RequestArchiveEnqueueStatusAlreadyQueued
}

// RequestArchiveProbeResult 是存储连通性探测的脱敏结果。错误信息只使用
// 稳定代码和通用文案，不能回显 endpoint、bucket、路径或底层 SDK 错误。
type RequestArchiveProbeResult struct {
	Healthy   bool   `json:"healthy"`
	LatencyMs int64  `json:"latency_ms"`
	Status    string `json:"status"`
	ErrorCode string `json:"error_code,omitempty"`
	Message   string `json:"message"`
}

type RequestArchiveRuntimeSnapshot struct {
	Enabled         bool                              `json:"enabled"`
	ConfigVersion   int64                             `json:"config_version"`
	WorkerRunning   bool                              `json:"worker_running"`
	WorkerCount     int                               `json:"worker_count"`
	WorkerActive    int64                             `json:"worker_active"`
	HeartbeatAt     int64                             `json:"heartbeat_at"`
	LastProcessedAt int64                             `json:"last_processed_at"`
	LastErrorCode   string                            `json:"last_error_code,omitempty"`
	Queue           model.RequestArchiveRuntimeCounts `json:"queue"`
	QueueDelayMs    int64                             `json:"queue_delay_ms"`
	Enqueued        int64                             `json:"enqueued"`
	Dropped         int64                             `json:"dropped"`
	LastEnqueueCode string                            `json:"last_enqueue_code,omitempty"`
}

type requestArchivePrivateConfig struct {
	Config       *model.RequestArchiveConfig
	Targets      map[string]model.RequestArchiveTarget
	EventFilters requestArchiveEventFilters
}
