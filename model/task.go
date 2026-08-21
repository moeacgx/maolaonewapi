package model

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	commonRelay "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"gorm.io/gorm"
)

type TaskStatus string

type TaskRefundReconciliationState string

func (t TaskStatus) ToVideoStatus() string {
	var status string
	switch t {
	case TaskStatusQueued, TaskStatusSubmitted:
		status = dto.VideoStatusQueued
	case TaskStatusInProgress:
		status = dto.VideoStatusInProgress
	case TaskStatusSuccess:
		status = dto.VideoStatusCompleted
	case TaskStatusFailure:
		status = dto.VideoStatusFailed
	default:
		status = dto.VideoStatusUnknown // Default fallback
	}
	return status
}

const (
	TaskStatusNotStart   TaskStatus = "NOT_START"
	TaskStatusSubmitted             = "SUBMITTED"
	TaskStatusQueued                = "QUEUED"
	TaskStatusInProgress            = "IN_PROGRESS"
	TaskStatusFailure               = "FAILURE"
	TaskStatusSuccess               = "SUCCESS"
	TaskStatusUnknown               = "UNKNOWN"
)

const (
	TaskRefundReconciliationStateNone             TaskRefundReconciliationState = ""
	TaskRefundReconciliationStatePending          TaskRefundReconciliationState = "pending"
	TaskRefundReconciliationStateManualUnreported TaskRefundReconciliationState = "manual_unreported"
	TaskRefundReconciliationStateManualReported   TaskRefundReconciliationState = "manual_reported"
)

// TaskRefundLegacyCutoff separates tasks created before timeout refunds were
// introduced. Those legacy tasks are failed without an automatic refund.
const TaskRefundLegacyCutoff int64 = 1771718400 // 2026-02-22 00:00:00 UTC

type Task struct {
	ID         int64                 `json:"id" gorm:"primary_key;AUTO_INCREMENT;index:idx_tasks_refund_reconciliation_eligibility,priority:6"`
	CreatedAt  int64                 `json:"created_at" gorm:"index"`
	UpdatedAt  int64                 `json:"updated_at" gorm:"index:idx_tasks_refund_reconciliation_eligibility,priority:5"`
	TaskID     string                `json:"task_id" gorm:"type:varchar(191);index"`                                                              // 第三方id，不一定有/ song id\ Task id
	Platform   constant.TaskPlatform `json:"platform" gorm:"type:varchar(30);index;index:idx_tasks_refund_reconciliation_eligibility,priority:3"` // 平台
	UserId     int                   `json:"user_id" gorm:"index"`
	Group      string                `json:"group" gorm:"type:varchar(50)"` // 修正计费用
	ChannelId  int                   `json:"channel_id" gorm:"index"`
	Quota      int                   `json:"quota" gorm:"index:idx_tasks_refund_reconciliation_eligibility,priority:4"`
	Action     string                `json:"action" gorm:"type:varchar(40);index"`                                                              // 任务类型, song, lyrics, description-mode
	Status     TaskStatus            `json:"status" gorm:"type:varchar(20);index;index:idx_tasks_refund_reconciliation_eligibility,priority:2"` // 任务状态
	FailReason string                `json:"fail_reason"`
	SubmitTime int64                 `json:"submit_time" gorm:"index"`
	StartTime  int64                 `json:"start_time" gorm:"index"`
	FinishTime int64                 `json:"finish_time" gorm:"index"`
	Progress   string                `json:"progress" gorm:"type:varchar(20);index"`
	Properties Properties            `json:"properties" gorm:"type:json"`
	Username   string                `json:"username,omitempty" gorm:"-"`
	// 禁止返回给用户，内部可能包含key等隐私信息
	PrivateData               TaskPrivateData               `json:"-" gorm:"column:private_data;type:json"`
	RefundReconciliationState TaskRefundReconciliationState `json:"-" gorm:"type:varchar(20);not null;default:'';index:idx_tasks_refund_reconciliation_eligibility,priority:1"`
	Data                      json.RawMessage               `json:"data" gorm:"type:json"`
}

func (t *Task) SetData(data any) {
	b, _ := common.Marshal(data)
	t.Data = json.RawMessage(b)
}

func (t *Task) GetData(v any) error {
	return common.Unmarshal(t.Data, &v)
}

type Properties struct {
	Input             string `json:"input"`
	UpstreamModelName string `json:"upstream_model_name,omitempty"`
	OriginModelName   string `json:"origin_model_name,omitempty"`
}

func (m *Properties) Scan(val interface{}) error {
	bytesValue, _ := val.([]byte)
	if len(bytesValue) == 0 {
		*m = Properties{}
		return nil
	}
	return common.Unmarshal(bytesValue, m)
}

func (m Properties) Value() (driver.Value, error) {
	if m == (Properties{}) {
		return nil, nil
	}
	return common.Marshal(m)
}

type TaskPrivateData struct {
	Key            string `json:"key,omitempty"`
	UpstreamTaskID string `json:"upstream_task_id,omitempty"` // 上游真实 task ID
	ResultURL      string `json:"result_url,omitempty"`       // 任务成功后的结果 URL（视频地址等）
	// 计费上下文：用于异步退款/差额结算（轮询阶段读取）
	BillingSource        string                    `json:"billing_source,omitempty"`  // "wallet" 或 "subscription"
	SubscriptionId       int                       `json:"subscription_id,omitempty"` // 订阅 ID，用于订阅退款
	TokenId              int                       `json:"token_id,omitempty"`        // 令牌 ID，用于令牌额度退款
	NodeName             string                    `json:"node_name,omitempty"`       // 发起任务的节点名，轮询结算阶段据此归属日志而非最后查询节点
	BillingContext       *TaskBillingContext       `json:"billing_context,omitempty"` // 计费参数快照（用于轮询阶段重新计算）
	RefundReconciliation *TaskRefundReconciliation `json:"refund_reconciliation,omitempty"`
}

// TaskRefundReconciliation durably records post-commit accounting that remains
// after an image task's money refund has committed. Quota is cleared in the
// same transaction as the wallet/subscription refund, so this marker can be
// retried without making the money refund eligible again.
type TaskRefundReconciliation struct {
	Amount                       int                 `json:"amount"`
	Reason                       string              `json:"reason,omitempty"`
	UserId                       int                 `json:"user_id"`
	ChannelId                    int                 `json:"channel_id,omitempty"`
	TokenId                      int                 `json:"token_id,omitempty"`
	BillingSource                string              `json:"billing_source,omitempty"`
	SubscriptionId               int                 `json:"subscription_id,omitempty"`
	Group                        string              `json:"group,omitempty"`
	ModelName                    string              `json:"model_name,omitempty"`
	NodeName                     string              `json:"node_name,omitempty"`
	BillingContext               *TaskBillingContext `json:"billing_context,omitempty"`
	OriginModelName              string              `json:"origin_model_name,omitempty"`
	UpstreamModelName            string              `json:"upstream_model_name,omitempty"`
	AccountingDone               bool                `json:"accounting_done,omitempty"`
	WalletQuotaVersion           int64               `json:"wallet_quota_version,omitempty"`
	WalletQuota                  int                 `json:"wallet_quota,omitempty"`
	CacheRepairDone              bool                `json:"cache_repair_done,omitempty"`
	LogClaimToken                string              `json:"log_claim_token,omitempty"`
	LogClaimUntil                int64               `json:"log_claim_until,omitempty"`
	LogWriteAttempted            bool                `json:"log_write_attempted,omitempty"`
	LogWriteAttemptedAt          int64               `json:"log_write_attempted_at,omitempty"`
	LogIdempotencyKey            string              `json:"log_idempotency_key,omitempty"`
	ManualReconciliationRequired bool                `json:"manual_reconciliation_required,omitempty"`
	ManualReconciliationReason   string              `json:"manual_reconciliation_reason,omitempty"`
	ManualReconciliationReported bool                `json:"manual_reconciliation_reported,omitempty"`
}

// TaskBillingContext 记录任务提交时的计费参数，以便轮询阶段可以重新计算额度。
type TaskBillingContext struct {
	ModelPrice      float64            `json:"model_price,omitempty"`       // 模型单价
	GroupRatio      float64            `json:"group_ratio,omitempty"`       // 分组倍率
	ModelRatio      float64            `json:"model_ratio,omitempty"`       // 模型倍率
	OtherRatios     map[string]float64 `json:"other_ratios,omitempty"`      // 附加倍率（时长、分辨率等）
	OriginModelName string             `json:"origin_model_name,omitempty"` // 模型名称，必须为OriginModelName
	PerCallBilling  bool               `json:"per_call_billing,omitempty"`  // 按次计费：跳过轮询阶段的差额结算
}

// GetUpstreamTaskID 获取上游真实 task ID（用于与 provider 通信）
// 旧数据没有 UpstreamTaskID 时，TaskID 本身就是上游 ID
func (t *Task) GetUpstreamTaskID() string {
	if t.PrivateData.UpstreamTaskID != "" {
		return t.PrivateData.UpstreamTaskID
	}
	return t.TaskID
}

// GetResultURL 获取任务结果 URL（视频地址等）
// 新数据存在 PrivateData.ResultURL 中；旧数据回退到 FailReason（历史兼容）
func (t *Task) GetResultURL() string {
	if t.PrivateData.ResultURL != "" {
		return t.PrivateData.ResultURL
	}
	return t.FailReason
}

// GenerateTaskID 生成对外暴露的 task_xxxx 格式 ID
func GenerateTaskID() string {
	key, _ := common.GenerateRandomCharsKey(32)
	return "task_" + key
}

func (p *TaskPrivateData) Scan(val interface{}) error {
	bytesValue, _ := val.([]byte)
	if len(bytesValue) == 0 {
		return nil
	}
	return common.Unmarshal(bytesValue, p)
}

func (p TaskPrivateData) Value() (driver.Value, error) {
	if (p == TaskPrivateData{}) {
		return nil, nil
	}
	return common.Marshal(p)
}

// SyncTaskQueryParams 用于包含所有搜索条件的结构体，可以根据需求添加更多字段
type SyncTaskQueryParams struct {
	Platform       constant.TaskPlatform
	ChannelID      string
	TaskID         string
	UserID         string
	Action         string
	Status         string
	StartTimestamp int64
	EndTimestamp   int64
	Username       string
	ModelName      string
	UserIDs        []int
}

func applyTaskTextFilter(query *gorm.DB, column string, value string) *gorm.DB {
	value = strings.TrimSpace(value)
	if value == "" {
		return query
	}
	if strings.Contains(value, "%") {
		pattern, err := sanitizeLikePattern(value)
		if err != nil {
			return query.Where("1 = 0")
		}
		return query.Where(column+" LIKE ? ESCAPE '!'", pattern)
	}
	return query.Where(column+" = ?", value)
}

func taskJSONValueExpr(field string) string {
	dialect := ""
	if DB != nil && DB.Dialector != nil {
		dialect = DB.Dialector.Name()
	}
	switch dialect {
	case "postgres":
		return "tasks.properties->>'" + field + "'"
	case "mysql":
		return "JSON_UNQUOTE(JSON_EXTRACT(tasks.properties, '$." + field + "'))"
	default:
		return "json_extract(tasks.properties, '$." + field + "')"
	}
}

func taskVisibleModelExpr() string {
	origin := taskJSONValueExpr("origin_model_name")
	upstream := taskJSONValueExpr("upstream_model_name")
	return "COALESCE(NULLIF(" + origin + ", ''), " + upstream + ")"
}

func applyTaskModelNameFilter(query *gorm.DB, modelName string, admin bool) *gorm.DB {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return query
	}
	if admin {
		origin := taskJSONValueExpr("origin_model_name")
		upstream := taskJSONValueExpr("upstream_model_name")
		if strings.Contains(modelName, "%") {
			pattern, err := sanitizeLikePattern(modelName)
			if err != nil {
				return query.Where("1 = 0")
			}
			return query.Where("("+origin+" LIKE ? ESCAPE '!' OR "+upstream+" LIKE ? ESCAPE '!')", pattern, pattern)
		}
		return query.Where("("+origin+" = ? OR "+upstream+" = ?)", modelName, modelName)
	}
	visible := taskVisibleModelExpr()
	if strings.Contains(modelName, "%") {
		pattern, err := sanitizeLikePattern(modelName)
		if err != nil {
			return query.Where("1 = 0")
		}
		return query.Where(visible+" LIKE ? ESCAPE '!'", pattern)
	}
	return query.Where(visible+" = ?", modelName)
}

func applyTaskQueryFilters(query *gorm.DB, queryParams SyncTaskQueryParams, admin bool) *gorm.DB {
	if queryParams.ChannelID != "" && admin {
		query = query.Where("tasks.channel_id = ?", queryParams.ChannelID)
	}
	if queryParams.Platform != "" {
		query = query.Where("tasks.platform = ?", queryParams.Platform)
	}
	if queryParams.UserID != "" && admin {
		query = query.Where("tasks.user_id = ?", queryParams.UserID)
	}
	if len(queryParams.UserIDs) != 0 && admin {
		query = query.Where("tasks.user_id in (?)", queryParams.UserIDs)
	}
	if queryParams.Username != "" && admin {
		query = query.Joins("JOIN users ON users.id = tasks.user_id")
		query = applyTaskTextFilter(query, "users.username", queryParams.Username)
	}
	if queryParams.ModelName != "" {
		query = applyTaskModelNameFilter(query, queryParams.ModelName, admin)
	}
	if queryParams.TaskID != "" {
		query = query.Where("tasks.task_id = ?", queryParams.TaskID)
	}
	if queryParams.Action != "" {
		query = query.Where("tasks.action = ?", queryParams.Action)
	}
	if queryParams.Status != "" {
		query = query.Where("tasks.status = ?", queryParams.Status)
	}
	if queryParams.StartTimestamp != 0 {
		query = query.Where("tasks.submit_time >= ?", queryParams.StartTimestamp)
	}
	if queryParams.EndTimestamp != 0 {
		query = query.Where("tasks.submit_time <= ?", queryParams.EndTimestamp)
	}
	return query
}

func InitTask(platform constant.TaskPlatform, relayInfo *commonRelay.RelayInfo) *Task {
	properties := Properties{}
	privateData := TaskPrivateData{}
	if relayInfo != nil && relayInfo.ChannelMeta != nil {
		if relayInfo.ChannelMeta.ChannelType == constant.ChannelTypeGemini ||
			relayInfo.ChannelMeta.ChannelType == constant.ChannelTypeVertexAi {
			privateData.Key = relayInfo.ChannelMeta.ApiKey
		}
		if relayInfo.UpstreamModelName != "" {
			properties.UpstreamModelName = relayInfo.UpstreamModelName
		}
		if relayInfo.OriginModelName != "" {
			properties.OriginModelName = relayInfo.OriginModelName
		}
	}

	// 使用预生成的公开 ID（如果有），否则新生成
	taskID := ""
	if relayInfo.TaskRelayInfo != nil && relayInfo.TaskRelayInfo.PublicTaskID != "" {
		taskID = relayInfo.TaskRelayInfo.PublicTaskID
	} else {
		taskID = GenerateTaskID()
	}

	t := &Task{
		TaskID:      taskID,
		UserId:      relayInfo.UserId,
		Group:       relayInfo.UsingGroup,
		SubmitTime:  time.Now().Unix(),
		Status:      TaskStatusNotStart,
		Progress:    "0%",
		ChannelId:   relayInfo.ChannelId,
		Platform:    platform,
		Properties:  properties,
		PrivateData: privateData,
	}
	return t
}

func TaskGetAllUserTask(userId int, startIdx int, num int, queryParams SyncTaskQueryParams) []*Task {
	return taskGetAllUserTask(userId, startIdx, num, queryParams, false)
}

func TaskGetAllUserTaskForLog(userId int, startIdx int, num int, queryParams SyncTaskQueryParams) []*Task {
	tasks := taskGetAllUserTask(userId, startIdx, num, queryParams, true)
	hydrateTaskLogData(tasks)
	return tasks
}

func taskGetAllUserTask(userId int, startIdx int, num int, queryParams SyncTaskQueryParams, omitData bool) []*Task {
	var tasks []*Task
	var err error

	query := DB.Model(&Task{}).Where("tasks.user_id = ?", userId)
	query = applyTaskQueryFilters(query, queryParams, false)

	err = taskLogSelect(query, omitData, true).Order("tasks.id desc").Limit(num).Offset(startIdx).Find(&tasks).Error
	if err != nil {
		return nil
	}

	return tasks
}

func TaskGetAllTasks(startIdx int, num int, queryParams SyncTaskQueryParams) []*Task {
	return taskGetAllTasks(startIdx, num, queryParams, false)
}

func TaskGetAllTasksForLog(startIdx int, num int, queryParams SyncTaskQueryParams) []*Task {
	tasks := taskGetAllTasks(startIdx, num, queryParams, true)
	hydrateTaskLogData(tasks)
	return tasks
}

func taskGetAllTasks(startIdx int, num int, queryParams SyncTaskQueryParams, omitData bool) []*Task {
	var tasks []*Task
	var err error

	query := DB.Model(&Task{})
	query = applyTaskQueryFilters(query, queryParams, true)

	err = taskLogSelect(query, omitData, false).Order("tasks.id desc").Limit(num).Offset(startIdx).Find(&tasks).Error
	if err != nil {
		return nil
	}

	return tasks
}

func taskLogSelect(query *gorm.DB, omitData bool, omitChannel bool) *gorm.DB {
	columns := make([]string, 0, 2)
	if omitData {
		columns = append(columns, "data")
	}
	if omitChannel {
		columns = append(columns, "channel_id")
	}
	if len(columns) == 0 {
		return query
	}
	return query.Omit(columns...)
}

func hydrateTaskLogData(tasks []*Task) {
	ids := make([]int64, 0, len(tasks))
	now := time.Now().Unix()
	for _, task := range tasks {
		if shouldHydrateTaskLogData(task, now) {
			ids = append(ids, task.ID)
		}
	}
	if len(ids) == 0 {
		return
	}

	var rows []struct {
		ID   int64
		Data json.RawMessage
	}
	if err := DB.Model(&Task{}).
		Select("id", "data").
		Where("id IN ?", ids).
		Find(&rows).Error; err != nil {
		return
	}

	dataByID := make(map[int64]json.RawMessage, len(rows))
	for _, row := range rows {
		dataByID[row.ID] = row.Data
	}
	for _, task := range tasks {
		if task != nil {
			task.Data = dataByID[task.ID]
		}
	}
}

func shouldHydrateTaskLogData(task *Task, now int64) bool {
	if task == nil {
		return false
	}
	if !constant.IsImageTaskPlatform(task.Platform) {
		return true
	}
	if task.Status != TaskStatusSuccess {
		return false
	}
	return !imageTaskLogDataExpired(task, now)
}

func imageTaskLogDataExpired(task *Task, now int64) bool {
	retentionHours := common.GetImageTaskDataRetentionHours()
	if retentionHours <= 0 || task.FinishTime <= 0 {
		return false
	}
	return now >= task.FinishTime+int64(retentionHours)*int64(time.Hour/time.Second)
}

func GetTimedOutUnfinishedTasks(cutoffUnix int64, limit int) []*Task {
	var tasks []*Task
	err := DB.Where("progress != ?", "100%").
		Where("status NOT IN ?", []string{TaskStatusFailure, TaskStatusSuccess}).
		Where("platform NOT IN ?", constant.ImageTaskPlatforms()).
		Where("submit_time < ?", cutoffUnix).
		Order("submit_time").
		Limit(limit).
		Find(&tasks).Error
	if err != nil {
		return nil
	}
	return tasks
}

// GetUnrefundedFailedTasks returns failed tasks whose non-zero quota marks a
// pending refund. Legacy tasks are excluded before LIMIT is applied so they
// cannot starve refundable tasks from the reconciliation sweep.
func GetUnrefundedFailedTasks(updatedBefore int64, limit int) []*Task {
	if limit <= 0 {
		return nil
	}

	var tasks []*Task
	err := DB.Where("status = ?", TaskStatusFailure).
		Where("quota != ?", 0).
		Where("updated_at <= ?", updatedBefore).
		Where("(submit_time <= ? OR submit_time >= ?)", 0, TaskRefundLegacyCutoff).
		Order("id").
		Limit(limit).
		Find(&tasks).Error
	if err != nil {
		return nil
	}
	return tasks
}

func GetAllUnFinishSyncTasks(limit int) []*Task {
	var tasks []*Task
	var err error
	// get all tasks progress is not 100%
	err = DB.Where("progress != ?", "100%").Where("status != ?", TaskStatusFailure).Where("status != ?", TaskStatusSuccess).Where("platform NOT IN ?", constant.ImageTaskPlatforms()).Limit(limit).Order("id").Find(&tasks).Error
	if err != nil {
		return nil
	}
	return tasks
}

// HasUnfinishedSyncTasks reports whether the async-task scheduler has polling
// or refund-reconciliation work. The legacy name is retained because it is the
// scheduler's existing work-detection hook.
func HasUnfinishedSyncTasks() bool {
	return HasTaskPollingWork()
}

func hasUnfinishedSyncTasks() bool {
	var id int64
	err := DB.Model(&Task{}).
		Where("progress != ?", "100%").
		Where("status != ?", TaskStatusFailure).
		Where("status != ?", TaskStatusSuccess).
		Where("platform NOT IN ?", constant.ImageTaskPlatforms()).
		Limit(1).
		Pluck("id", &id).Error
	return err == nil && id != 0
}

// HasTaskPollingWork reports whether polling has unfinished work or failed
// tasks with a pending, non-legacy refund marker.
func HasTaskPollingWork() bool {
	if hasUnfinishedSyncTasks() {
		return true
	}

	var id int64
	err := DB.Model(&Task{}).
		Where("status = ?", TaskStatusFailure).
		Where("quota != ?", 0).
		Where("(submit_time <= ? OR submit_time >= ?)", 0, TaskRefundLegacyCutoff).
		Limit(1).
		Pluck("id", &id).Error
	return err == nil && id != 0
}

func GetByTaskId(userId int, taskId string) (*Task, bool, error) {
	if taskId == "" {
		return nil, false, nil
	}
	var task *Task
	var err error
	err = DB.Where("user_id = ? and task_id = ?", userId, taskId).
		First(&task).Error
	exist, err := RecordExist(err)
	if err != nil {
		return nil, false, err
	}
	return task, exist, err
}

func GetByTaskIds(userId int, taskIds []any) ([]*Task, error) {
	if len(taskIds) == 0 {
		return nil, nil
	}
	var task []*Task
	var err error
	err = DB.Where("user_id = ? and task_id in (?)", userId, taskIds).
		Find(&task).Error
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (Task *Task) Insert() error {
	var err error
	err = DB.Create(Task).Error
	return err
}

type taskSnapshot struct {
	Status     TaskStatus
	Progress   string
	StartTime  int64
	FinishTime int64
	FailReason string
	ResultURL  string
	Data       json.RawMessage
}

func (s taskSnapshot) Equal(other taskSnapshot) bool {
	return s.Status == other.Status &&
		s.Progress == other.Progress &&
		s.StartTime == other.StartTime &&
		s.FinishTime == other.FinishTime &&
		s.FailReason == other.FailReason &&
		s.ResultURL == other.ResultURL &&
		bytes.Equal(s.Data, other.Data)
}

func (t *Task) Snapshot() taskSnapshot {
	return taskSnapshot{
		Status:     t.Status,
		Progress:   t.Progress,
		StartTime:  t.StartTime,
		FinishTime: t.FinishTime,
		FailReason: t.FailReason,
		ResultURL:  t.PrivateData.ResultURL,
		Data:       t.Data,
	}
}

func (Task *Task) Update() error {
	var err error
	err = DB.Save(Task).Error
	return err
}

func (t *Task) UpdateQuota() error {
	return DB.Model(t).Update("quota", t.Quota).Error
}

// ClaimQuotaForRefund atomically clears an expected non-zero quota. A true
// result grants the caller ownership of the corresponding refund attempt.
func ClaimQuotaForRefund(id int64, expectedQuota int) (bool, error) {
	if expectedQuota == 0 {
		return false, nil
	}

	result := DB.Model(&Task{}).
		Where("id = ? AND quota = ?", id, expectedQuota).
		Update("quota", 0)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// RestoreQuotaAfterFailedRefund restores a claimed quota marker only while it
// is still zero, so a later reconciliation pass can retry the refund.
func RestoreQuotaAfterFailedRefund(id int64, quota int) (bool, error) {
	if quota == 0 {
		return false, nil
	}

	result := DB.Model(&Task{}).
		Where("id = ? AND quota = ?", id, 0).
		Update("quota", quota)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// UpdateWithStatus performs a conditional UPDATE guarded by fromStatus (CAS).
// Returns (true, nil) if this caller won the update, (false, nil) if
// another process already moved the task out of fromStatus. MySQL commonly
// reports changed rows rather than matched rows, so a same-value no-op update
// can also return false even when the status predicate still matched.
//
// Uses Model().Select("*").Updates() instead of Save() because GORM's Save
// falls back to INSERT ON CONFLICT when the WHERE-guarded UPDATE matches
// zero rows, which silently bypasses the CAS guard.
func (t *Task) UpdateWithStatus(fromStatus TaskStatus) (bool, error) {
	result := DB.Model(t).Where("status = ?", fromStatus).Select("*").Updates(t)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// TaskBulkUpdateByID performs an unconditional bulk UPDATE by primary key IDs.
// WARNING: This function has NO CAS (Compare-And-Swap) guard — it will overwrite
// any concurrent status changes. DO NOT use in billing/quota lifecycle flows
// (e.g., timeout, success, failure transitions that trigger refunds or settlements).
// For status transitions that involve billing, use Task.UpdateWithStatus() instead.
func TaskBulkUpdateByID(ids []int64, params map[string]any) error {
	if len(ids) == 0 {
		return nil
	}
	return DB.Model(&Task{}).
		Where("id in (?)", ids).
		Updates(params).Error
}

type TaskQuotaUsage struct {
	Mode  string  `json:"mode"`
	Count float64 `json:"count"`
}

// TaskCountAllTasks returns total tasks that match the given query params (admin usage)
func TaskCountAllTasks(queryParams SyncTaskQueryParams) int64 {
	var total int64
	query := DB.Model(&Task{})
	query = applyTaskQueryFilters(query, queryParams, true)
	_ = query.Count(&total).Error
	return total
}

// TaskCountAllUserTask returns total tasks for given user
func TaskCountAllUserTask(userId int, queryParams SyncTaskQueryParams) int64 {
	var total int64
	query := DB.Model(&Task{}).Where("tasks.user_id = ?", userId)
	query = applyTaskQueryFilters(query, queryParams, false)
	_ = query.Count(&total).Error
	return total
}
func (t *Task) ToOpenAIVideo() *dto.OpenAIVideo {
	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = t.TaskID
	openAIVideo.Status = t.Status.ToVideoStatus()
	openAIVideo.Model = t.Properties.OriginModelName
	openAIVideo.SetProgressStr(t.Progress)
	openAIVideo.CreatedAt = t.CreatedAt
	openAIVideo.CompletedAt = t.UpdatedAt
	openAIVideo.SetMetadata("url", t.GetResultURL())
	return openAIVideo
}
