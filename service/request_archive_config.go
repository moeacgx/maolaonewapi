package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/google/uuid"
)

// 请求归档位于每条 Relay 请求的热路径。缓存只保存已加密的配置快照，不保存
// 解密后的对象存储密钥；保存接口会立即失效本节点缓存，短 TTL 负责多节点收敛。
type requestArchiveConfigCacheState struct {
	mu       sync.RWMutex
	config   *requestArchivePrivateConfig
	loadedAt time.Time
	err      error
}

var requestArchiveConfigCache requestArchiveConfigCacheState

const requestArchiveConfigCacheTTL = 2 * time.Second

const requestArchiveMaxCredentialBytes = 4096

// ErrRequestArchivePersistence 标记不可安全回显的数据库或持久化故障。
// 对外接口应返回通用 500 文案，具体 cause 仅供 errors.Is 和内部诊断使用。
var ErrRequestArchivePersistence = errors.New("完整请求归档持久化暂时不可用")

type requestArchivePersistenceError struct {
	cause error
}

func (err *requestArchivePersistenceError) Error() string {
	return ErrRequestArchivePersistence.Error()
}

func (err *requestArchivePersistenceError) Unwrap() error {
	return err.cause
}

func (err *requestArchivePersistenceError) Is(target error) bool {
	return target == ErrRequestArchivePersistence
}

func wrapRequestArchivePersistenceError(err error) error {
	if err == nil || errors.Is(err, ErrRequestArchivePersistence) {
		return err
	}
	return &requestArchivePersistenceError{cause: err}
}

func InvalidateRequestArchiveConfig() {
	requestArchiveConfigCache.mu.Lock()
	requestArchiveConfigCache.config = nil
	requestArchiveConfigCache.loadedAt = time.Time{}
	requestArchiveConfigCache.err = nil
	requestArchiveConfigCache.mu.Unlock()
}

// GetRequestArchiveConfig 只返回脱敏存储配置；调用者无法从此接口取得任何
// 访问密钥密文或明文。
func GetRequestArchiveConfig(ctx context.Context) (*RequestArchiveConfig, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	config, targets, err := model.LoadRequestArchiveConfig(ctx)
	if err != nil {
		return nil, wrapRequestArchivePersistenceError(err)
	}
	return publicRequestArchiveConfig(config, targets), nil
}

// RequestArchiveEnabled 供 Relay 热路径在读取正文前判断是否需要归档。配置
// 读取异常时返回 false，保证可选归档故障不会让用户请求多读正文或改变行为。
func RequestArchiveEnabled(ctx context.Context) (bool, error) {
	enabled, _, err := RequestArchiveBodyLimit(ctx)
	return enabled, err
}

// RequestArchiveBodyLimit 让 HTTP BodyStorage 在物化大正文前先做大小判断，
// 避免超过配置上限的磁盘正文被无意义地整体读回内存。
func RequestArchiveBodyLimit(ctx context.Context) (bool, int64, error) {
	if err := ctx.Err(); err != nil {
		return false, 0, err
	}
	config, err := loadRequestArchivePrivateConfig(ctx)
	if err != nil || config == nil || config.Config == nil {
		return false, 0, err
	}
	return config.Config.Enabled, config.Config.MaxBodyBytes, nil
}

// SaveRequestArchiveConfig 校验并保存多目标配置。active_target_id 只被写入
// 新任务；老任务通过它们持久化的 target_id 继续访问原目标，即使该目标已禁用。
func SaveRequestArchiveConfig(ctx context.Context, request RequestArchiveUpdateRequest, actorId int) (*RequestArchiveConfig, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request.ActiveTargetId = strings.ToLower(strings.TrimSpace(request.ActiveTargetId))
	if err := validateRequestArchiveUpdate(request); err != nil {
		return nil, err
	}
	current, currentTargets, err := model.LoadRequestArchiveConfig(ctx)
	if err != nil {
		return nil, wrapRequestArchivePersistenceError(err)
	}
	if request.ExpectedConfigVersion != current.ConfigVersion {
		return nil, model.ErrRequestArchiveConfigConflict
	}
	byId := make(map[string]model.RequestArchiveTarget, len(currentTargets))
	for _, target := range currentTargets {
		byId[target.Id] = target
	}

	targets := make([]model.RequestArchiveTarget, 0, len(request.Targets))
	seen := make(map[string]struct{}, len(request.Targets))
	for _, input := range request.Targets {
		target, err := buildRequestArchiveTarget(input, byId)
		if err != nil {
			return nil, err
		}
		if _, duplicated := seen[target.Id]; duplicated {
			return nil, errors.New("请求归档存储目标 ID 重复")
		}
		seen[target.Id] = struct{}{}
		targets = append(targets, target)
	}
	if request.Enabled {
		if !RequestArchiveCryptoReady() {
			return nil, errors.New("启用完整请求归档前必须配置稳定的 CRYPTO_SECRET")
		}
		if request.ActiveTargetId == "" {
			return nil, errors.New("启用完整请求归档时必须选择活动存储目标")
		}
		foundEnabled := false
		for _, target := range targets {
			if target.Id == request.ActiveTargetId && target.Enabled {
				foundEnabled = true
				break
			}
		}
		if !foundEnabled {
			return nil, errors.New("活动请求归档存储目标不存在或已禁用")
		}
	}

	next := &model.RequestArchiveConfig{
		Id: current.Id, Enabled: request.Enabled, ActiveTargetId: request.ActiveTargetId,
		RetentionDays: request.RetentionDays, WorkerCount: request.WorkerCount,
		QueueCapacity: request.QueueCapacity, MaxBodyBytes: request.MaxBodyBytes,
		QueueMaxBytes: request.QueueMaxBytes, UpdatedBy: actorId,
	}
	if err := model.SaveRequestArchiveConfig(ctx, request.ExpectedConfigVersion, next, targets); err != nil {
		if errors.Is(err, model.ErrRequestArchiveConfigConflict) || errors.Is(err, model.ErrRequestArchiveTargetInUse) {
			return nil, err
		}
		return nil, wrapRequestArchivePersistenceError(err)
	}
	InvalidateRequestArchiveConfig()
	return GetRequestArchiveConfig(ctx)
}

func validateRequestArchiveUpdate(request RequestArchiveUpdateRequest) error {
	if request.ExpectedConfigVersion < 1 {
		return errors.New("请求归档配置版本无效")
	}
	if request.RetentionDays < 1 || request.RetentionDays > 3650 {
		return fmt.Errorf("请求归档保留天数必须在 1 到 3650 之间")
	}
	if request.WorkerCount < 1 || request.WorkerCount > 32 {
		return fmt.Errorf("请求归档工作线程数必须在 1 到 32 之间")
	}
	if request.QueueCapacity < 1 || request.QueueCapacity > 1048576 {
		return fmt.Errorf("请求归档队列容量必须在 1 到 1048576 之间")
	}
	if request.MaxBodyBytes < 1<<10 || request.MaxBodyBytes > model.RequestArchiveMaximumBodyBytes {
		return fmt.Errorf("请求归档单请求上限必须在 1 KiB 到 128 MiB 之间")
	}
	if request.QueueMaxBytes < request.MaxBodyBytes || request.QueueMaxBytes > 64<<30 {
		return fmt.Errorf("请求归档队列字节上限必须不小于单请求上限且不超过 64 GiB")
	}
	if len(request.Targets) > 64 {
		return errors.New("请求归档存储目标数量不能超过 64")
	}
	if request.ActiveTargetId != "" && !validRequestArchiveTargetID(request.ActiveTargetId) {
		return errors.New("请求归档活动存储目标 ID 无效")
	}
	return nil
}

func buildRequestArchiveTarget(input RequestArchiveUpdateTarget, existing map[string]model.RequestArchiveTarget) (model.RequestArchiveTarget, error) {
	id := strings.ToLower(strings.TrimSpace(input.Id))
	if id == "" {
		id = uuid.NewString()
	}
	if !validRequestArchiveTargetID(id) {
		return model.RequestArchiveTarget{}, errors.New("请求归档存储目标 ID 无效")
	}
	prior, exists := existing[id]
	target := model.RequestArchiveTarget{
		Id: id, Name: trimRequestArchiveValue(input.Name, 128),
		Type: strings.ToLower(strings.TrimSpace(input.Type)), Enabled: input.Enabled,
		LocalPath: strings.TrimSpace(input.LocalPath), Endpoint: strings.TrimSpace(input.Endpoint),
		Bucket: strings.TrimSpace(input.Bucket), Region: strings.TrimSpace(input.Region),
		Prefix: strings.TrimSpace(input.Prefix), PathStyle: input.PathStyle,
	}
	if target.Name == "" {
		return model.RequestArchiveTarget{}, errors.New("请求归档存储目标名称不能为空")
	}
	if exists {
		target.CreatedAt = prior.CreatedAt
	}
	if err := NormalizeRequestArchiveTarget(&target); err != nil {
		return model.RequestArchiveTarget{}, err
	}
	if target.Type == model.RequestArchiveTargetLocal {
		// 本地目标没有凭据，即便 UI 误传密钥也不能落库。
		return target, nil
	}
	if exists && (requestArchiveSecretActionKeepsValue(input.AccessKeyAction) || requestArchiveSecretActionKeepsValue(input.SecretKeyAction)) &&
		(target.Endpoint != prior.Endpoint || target.Bucket != prior.Bucket) {
		return model.RequestArchiveTarget{}, errors.New("保留原 S3 兼容存储密钥时不能修改 endpoint 或 bucket")
	}
	accessCiphertext, err := requestArchiveSecretForAction(
		input.AccessKeyAction, input.AccessKey, prior.AccessKeyCiphertext, exists,
		target.Id, requestArchiveAccessKeyPurpose,
	)
	if err != nil {
		return model.RequestArchiveTarget{}, err
	}
	secretCiphertext, err := requestArchiveSecretForAction(
		input.SecretKeyAction, input.SecretKey, prior.SecretKeyCiphertext, exists,
		target.Id, requestArchiveSecretKeyPurpose,
	)
	if err != nil {
		return model.RequestArchiveTarget{}, err
	}
	if (accessCiphertext == "") != (secretCiphertext == "") {
		return model.RequestArchiveTarget{}, errors.New("S3 兼容存储访问密钥和密钥必须同时配置或同时清除")
	}
	if target.Enabled && accessCiphertext == "" {
		return model.RequestArchiveTarget{}, errors.New("启用 S3 兼容存储目标前必须配置访问密钥和密钥")
	}
	target.AccessKeyCiphertext = accessCiphertext
	target.SecretKeyCiphertext = secretCiphertext
	return target, nil
}

func requestArchiveSecretActionKeepsValue(action string) bool {
	action = strings.ToLower(strings.TrimSpace(action))
	return action == "" || action == RequestArchiveSecretKeep
}

func requestArchiveSecretForAction(action, value, existing string, targetExists bool, targetID, purpose string) (string, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		action = RequestArchiveSecretKeep
	}
	switch action {
	case RequestArchiveSecretKeep:
		if value != "" {
			return "", errors.New("保留请求归档存储密钥时不能同时提交新密钥")
		}
		if !targetExists {
			return "", errors.New("新增 S3 兼容存储目标时必须提供密钥")
		}
		return existing, nil
	case RequestArchiveSecretClear:
		if value != "" {
			return "", errors.New("清除请求归档存储密钥时不能同时提交新密钥")
		}
		return "", nil
	case RequestArchiveSecretReplace:
		if strings.TrimSpace(value) == "" {
			return "", errors.New("替换请求归档存储密钥时不能为空")
		}
		if len(value) > requestArchiveMaxCredentialBytes || strings.ContainsRune(value, 0) {
			return "", errors.New("请求归档存储密钥长度或内容无效")
		}
		if !RequestArchiveCryptoReady() {
			return "", errors.New("保存请求归档存储密钥前必须配置稳定的 CRYPTO_SECRET")
		}
		return EncryptRequestArchiveSecret(value, targetID, purpose)
	default:
		return "", errors.New("请求归档存储密钥操作无效")
	}
}

func publicRequestArchiveConfig(config *model.RequestArchiveConfig, targets []model.RequestArchiveTarget) *RequestArchiveConfig {
	result := &RequestArchiveConfig{Targets: make([]RequestArchiveTarget, 0, len(targets))}
	if config != nil {
		result.ConfigVersion = config.ConfigVersion
		result.Enabled = config.Enabled
		result.ActiveTargetId = config.ActiveTargetId
		result.RetentionDays = config.RetentionDays
		result.WorkerCount = config.WorkerCount
		result.QueueCapacity = config.QueueCapacity
		result.MaxBodyBytes = config.MaxBodyBytes
		result.QueueMaxBytes = config.QueueMaxBytes
	}
	for _, target := range targets {
		result.Targets = append(result.Targets, RequestArchiveTarget{
			Id: target.Id, Name: target.Name, Type: target.Type, Enabled: target.Enabled,
			LocalPath: target.LocalPath, Endpoint: target.Endpoint, Bucket: target.Bucket,
			Region: target.Region, Prefix: target.Prefix, PathStyle: target.PathStyle,
			AccessKeyConfigured: target.AccessKeyCiphertext != "",
			SecretKeyConfigured: target.SecretKeyCiphertext != "",
			CreatedAt:           target.CreatedAt, UpdatedAt: target.UpdatedAt,
		})
	}
	return result
}

func loadRequestArchivePrivateConfig(ctx context.Context) (*requestArchivePrivateConfig, error) {
	requestArchiveConfigCache.mu.RLock()
	if !requestArchiveConfigCache.loadedAt.IsZero() && time.Since(requestArchiveConfigCache.loadedAt) < requestArchiveConfigCacheTTL {
		config := cloneRequestArchivePrivateConfig(requestArchiveConfigCache.config)
		err := requestArchiveConfigCache.err
		requestArchiveConfigCache.mu.RUnlock()
		return config, err
	}
	requestArchiveConfigCache.mu.RUnlock()

	requestArchiveConfigCache.mu.Lock()
	defer requestArchiveConfigCache.mu.Unlock()
	if !requestArchiveConfigCache.loadedAt.IsZero() && time.Since(requestArchiveConfigCache.loadedAt) < requestArchiveConfigCacheTTL {
		return cloneRequestArchivePrivateConfig(requestArchiveConfigCache.config), requestArchiveConfigCache.err
	}
	config, err := loadRequestArchivePrivateConfigFromDB(ctx)
	if err != nil {
		stale := cloneRequestArchivePrivateConfig(requestArchiveConfigCache.config)
		requestArchiveConfigCache.loadedAt = time.Now()
		requestArchiveConfigCache.err = err
		return stale, err
	}
	requestArchiveConfigCache.config = config
	requestArchiveConfigCache.loadedAt = time.Now()
	requestArchiveConfigCache.err = nil
	return cloneRequestArchivePrivateConfig(config), nil
}

func loadRequestArchivePrivateConfigFromDB(ctx context.Context) (*requestArchivePrivateConfig, error) {
	config, targets, err := model.LoadRequestArchiveConfig(ctx)
	if err != nil {
		return nil, err
	}
	byId := make(map[string]model.RequestArchiveTarget, len(targets))
	for _, target := range targets {
		byId[target.Id] = target
	}
	return &requestArchivePrivateConfig{Config: config, Targets: byId}, nil
}

func cloneRequestArchivePrivateConfig(source *requestArchivePrivateConfig) *requestArchivePrivateConfig {
	if source == nil {
		return nil
	}
	result := &requestArchivePrivateConfig{Targets: make(map[string]model.RequestArchiveTarget, len(source.Targets))}
	if source.Config != nil {
		config := *source.Config
		result.Config = &config
	}
	for id, target := range source.Targets {
		result.Targets[id] = target
	}
	return result
}

func trimRequestArchiveValue(value string, maximum int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > maximum {
		runes = runes[:maximum]
	}
	return string(runes)
}

func validRequestArchiveTargetID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 64 {
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
