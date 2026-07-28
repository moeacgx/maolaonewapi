package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/google/uuid"
	"golang.org/x/sync/semaphore"
	"gorm.io/gorm"
)

const (
	requestArchiveLease                       = 45 * time.Second
	requestArchiveLeaseRenewTimeout           = 5 * time.Second
	requestArchiveEnqueueBaseTimeout          = 250 * time.Millisecond
	requestArchiveEnqueueTimeoutPerMiB        = 50 * time.Millisecond
	requestArchiveEnqueueMaxTimeout           = 8 * time.Second
	requestArchiveStorageBaseTimeout          = 20 * time.Second
	requestArchiveStorageTimeoutPerMiB        = 2 * time.Second
	requestArchiveStorageMaxTimeout           = 5 * time.Minute
	requestArchiveCleanupReconcileQuietPeriod = 2 * requestArchiveStorageMaxTimeout
	requestArchiveWorkerReconcileInterval     = 250 * time.Millisecond
	requestArchiveEncryptionMemoryBudget      = int64(384 * 1024 * 1024)
)

var (
	errRequestArchiveEncryptionBusy = errors.New("请求归档加密容量暂时繁忙")
	requestArchiveEncryptionBudget  = semaphore.NewWeighted(requestArchiveEncryptionMemoryBudget)
)

// QueueRequestArchive 在 Relay 取得原始正文后的第一时间调用。它只把正文
// 加密信封写入持久队列，外部文件或对象存储由后台 Worker 异步处理，调用方
// 可将错误作为运行指标记录而不影响主请求。
func QueueRequestArchive(ctx context.Context, request RequestArchiveRequest) (RequestArchiveEnqueueResult, error) {
	return queueRequestArchiveWithBody(ctx, request, int64(len(request.Body)), func() ([]byte, error) {
		return request.Body, nil
	})
}

// QueueRequestArchiveFromReader 在取得全局内存预算后才读取正文。磁盘型
// BodyStorage 使用此入口，避免并发大请求先各自物化完整 []byte 再被预算拒绝。
func QueueRequestArchiveFromReader(ctx context.Context, request RequestArchiveRequest, reader io.Reader, bodySize int64) (RequestArchiveEnqueueResult, error) {
	if reader == nil {
		return RequestArchiveEnqueueResult{}, errors.New("请求归档正文读取器为空")
	}
	return queueRequestArchiveWithBody(ctx, request, bodySize, func() ([]byte, error) {
		if bodySize < 0 || bodySize > int64(^uint(0)>>1) {
			return nil, model.ErrRequestArchiveBodyTooLarge
		}
		body := make([]byte, int(bodySize))
		if _, err := io.ReadFull(reader, body); err != nil {
			return nil, err
		}
		return body, nil
	})
}

func queueRequestArchiveWithBody(
	ctx context.Context,
	request RequestArchiveRequest,
	bodySize int64,
	loadBody func() ([]byte, error),
) (RequestArchiveEnqueueResult, error) {
	if err := ctx.Err(); err != nil {
		return RequestArchiveEnqueueResult{}, err
	}
	privateConfig, err := loadRequestArchivePrivateConfig(ctx)
	if err != nil {
		return RequestArchiveEnqueueResult{}, err
	}
	if privateConfig.Config == nil || !privateConfig.Config.Enabled {
		return RequestArchiveEnqueueResult{}, nil
	}
	if !RequestArchiveCryptoReady() {
		return RequestArchiveEnqueueResult{}, errors.New("请求归档加密密钥不可用")
	}
	if bodySize < 0 || bodySize > model.RequestArchiveMaximumBodyBytes ||
		privateConfig.Config.MaxBodyBytes < 1 ||
		privateConfig.Config.MaxBodyBytes > model.RequestArchiveMaximumBodyBytes ||
		bodySize > privateConfig.Config.MaxBodyBytes {
		return RequestArchiveEnqueueResult{}, model.ErrRequestArchiveBodyTooLarge
	}
	target, ok := privateConfig.Targets[privateConfig.Config.ActiveTargetId]
	if !ok || !target.Enabled {
		return RequestArchiveEnqueueResult{}, errors.New("请求归档活动存储目标不可用")
	}
	capacityContext, cancelCapacity := context.WithTimeout(ctx, requestArchiveEnqueueTimeoutForSize(bodySize))
	hasCapacity, capacityErr := model.RequestArchiveQueueHasCapacity(
		capacityContext, privateConfig.Config.QueueCapacity, privateConfig.Config.QueueMaxBytes, bodySize,
	)
	cancelCapacity()
	if capacityErr != nil {
		return RequestArchiveEnqueueResult{}, capacityErr
	}
	if !hasCapacity {
		return RequestArchiveEnqueueResult{}, model.ErrRequestArchiveQueueFull
	}
	envelopeSize, err := requestArchiveChunkedEnvelopeSize(bodySize, requestArchiveGCMTagSize)
	if err != nil {
		return RequestArchiveEnqueueResult{}, err
	}
	encryptionWeight := bodySize + int64(envelopeSize) + requestArchiveChunkSize + 64*1024
	if encryptionWeight > requestArchiveEncryptionMemoryBudget || !requestArchiveEncryptionBudget.TryAcquire(encryptionWeight) {
		return RequestArchiveEnqueueResult{}, errRequestArchiveEncryptionBusy
	}
	defer requestArchiveEncryptionBudget.Release(encryptionWeight)
	body, err := loadBody()
	if err != nil {
		return RequestArchiveEnqueueResult{}, err
	}
	if int64(len(body)) != bodySize {
		return RequestArchiveEnqueueResult{}, errors.New("请求归档正文长度发生变化")
	}
	now := time.Now().Unix()
	retentionDays := privateConfig.Config.RetentionDays
	if retentionDays < 1 {
		retentionDays = RequestArchiveDefaultRetentionDays
	}
	job := &model.RequestArchiveJob{
		ArchiveId: uuid.NewString(), TargetId: target.Id, ConfigVersion: privateConfig.Config.ConfigVersion,
		ByteSize:    bodySize,
		ContentType: trimRequestArchiveHeaderValue(request.ContentType, 255),
		Method:      trimRequestArchiveValue(strings.ToUpper(request.Method), 16),
		Path:        sanitizeRequestArchivePath(request.Path), RequestId: trimRequestArchiveValue(request.RequestId, 128),
		UserId: request.UserId, Username: trimRequestArchiveValue(request.Username, 128),
		UserEmail: trimRequestArchiveValue(request.UserEmail, 255),
		TokenId:   request.TokenId, TokenName: trimRequestArchiveValue(request.TokenName, 128),
		GroupId: request.GroupId, GroupName: trimRequestArchiveValue(request.GroupName, 128),
		CreatedAt: now, ExpiresAt: now + int64(retentionDays)*24*60*60,
	}
	job.SHA256, err = requestArchivePlaintextDigest(job, requestArchiveCipherVersion, body)
	if err != nil {
		return RequestArchiveEnqueueResult{}, err
	}
	ciphertext, err := EncryptRequestArchiveJobPayload(body, job)
	if err != nil {
		return RequestArchiveEnqueueResult{}, err
	}
	job.RequestCiphertext = model.RequestArchiveLargeText(ciphertext)
	enqueueContext, cancelEnqueue := context.WithTimeout(ctx, requestArchiveEnqueueTimeoutForSize(bodySize))
	defer cancelEnqueue()
	if err := model.EnqueueRequestArchiveJob(enqueueContext, job, privateConfig.Config.QueueCapacity); err != nil {
		return RequestArchiveEnqueueResult{}, err
	}
	requestArchiveEnqueued.Add(1)
	requestArchiveLastEnqueue.Store("")
	return RequestArchiveEnqueueResult{Enqueued: true, JobId: job.Id}, nil
}

func requestArchiveEnqueueTimeoutForSize(bodySize int64) time.Duration {
	if bodySize < 0 {
		bodySize = 0
	}
	mebibytes := (bodySize + (1 << 20) - 1) >> 20
	timeout := requestArchiveEnqueueBaseTimeout + time.Duration(mebibytes)*requestArchiveEnqueueTimeoutPerMiB
	if timeout > requestArchiveEnqueueMaxTimeout {
		return requestArchiveEnqueueMaxTimeout
	}
	return timeout
}

func trimRequestArchiveHeaderValue(value string, maximum int) string {
	value = strings.NewReplacer("\r", "", "\n", "").Replace(value)
	return trimRequestArchiveValue(value, maximum)
}

// sanitizeRequestArchivePath 故意去除 query 和 fragment，避免 API key 等
// 可能被客户端放在 URL 查询参数中的凭据成为“元数据”泄露面。
func sanitizeRequestArchivePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := url.ParseRequestURI(value); err == nil && parsed != nil {
		if parsed.EscapedPath() != "" {
			return trimRequestArchiveValue(parsed.EscapedPath(), 2048)
		}
	}
	value = strings.SplitN(value, "?", 2)[0]
	value = strings.SplitN(value, "#", 2)[0]
	return trimRequestArchiveValue(value, 2048)
}

// ProcessNextRequestArchiveJob 领取并投递一个任务。返回 false 表示当前没有
// 可领取任务；已禁用的目标仍可为旧任务投递，保证切换活动目标不会中断重试。
func ProcessNextRequestArchiveJob(ctx context.Context, workerId string) (bool, error) {
	// 缺少稳定密钥时保持任务为 queued/retry，等待运维修复配置。此处不能
	// 领取后把无法解密的健康任务标成 failed。
	if !RequestArchiveCryptoReady() {
		return false, nil
	}
	candidates, err := model.ListRequestArchiveJobCandidates(ctx, 16)
	if err != nil {
		return false, err
	}
	for _, candidate := range candidates {
		memoryWeight, weightErr := requestArchiveWorkerMemoryWeight(candidate)
		if weightErr != nil {
			return false, weightErr
		}
		if !requestArchiveEncryptionBudget.TryAcquire(memoryWeight) {
			// 保持 FIFO：最早任务暂时拿不到预算时不越过它领取后续任务。
			return false, nil
		}
		job, claimErr := model.ClaimRequestArchiveJobCandidate(ctx, workerId, requestArchiveLease, candidate)
		if errors.Is(claimErr, gorm.ErrRecordNotFound) {
			requestArchiveEncryptionBudget.Release(memoryWeight)
			continue
		}
		if claimErr != nil {
			requestArchiveEncryptionBudget.Release(memoryWeight)
			return false, claimErr
		}
		processed, processErr := processClaimedRequestArchiveJob(ctx, job)
		requestArchiveEncryptionBudget.Release(memoryWeight)
		return processed, processErr
	}
	return false, nil
}

func requestArchiveWorkerMemoryWeight(candidate model.RequestArchiveJobCandidate) (int64, error) {
	envelopeSize, err := requestArchiveChunkedEnvelopeSize(candidate.ByteSize, requestArchiveGCMTagSize)
	if err != nil {
		return 0, err
	}
	weight := int64(envelopeSize) + requestArchiveChunkSize + 64*1024
	if strings.TrimSpace(candidate.ArchiveId) == "" {
		// ra1 的数据库信封长度与分片信封不同，且兼容解密会把单块
		// Base64 密文原地变成明文，只需额外预留一份正文。
		legacyEnvelope := int64(base64.RawURLEncoding.EncodedLen(int(candidate.ByteSize)+requestArchiveGCMTagSize) + 64)
		weight = legacyEnvelope + candidate.ByteSize + 64*1024
	}
	if weight < 1 {
		weight = 1
	}
	if weight > requestArchiveEncryptionMemoryBudget {
		weight = requestArchiveEncryptionMemoryBudget
	}
	return weight, nil
}

func processClaimedRequestArchiveJob(ctx context.Context, job *model.RequestArchiveJob) (bool, error) {
	if job.ExpiresAt > 0 && job.ExpiresAt <= time.Now().Unix() {
		return true, model.FailRequestArchiveJob(ctx, job, "request_archive_expired", "归档任务已超过保留期")
	}
	if job.ByteSize > 0 && job.RequestCiphertext == "" {
		return true, model.FailRequestArchiveJob(ctx, job, "request_archive_ciphertext_invalid", "归档密文无效")
	}
	if job.RequestCiphertext != "" {
		// ra2/ra3 按分片校验并立即丢弃短暂明文，避免 Worker 为大请求再次
		// 组装整块正文。真正写出的仍是原始版本化密文信封。
		if validateErr := ValidateRequestArchivePayload(job); validateErr != nil {
			return true, model.FailRequestArchiveJob(ctx, job, "request_archive_ciphertext_invalid", "归档密文无效")
		}
	}
	target, err := model.GetRequestArchiveTarget(ctx, job.TargetId)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, model.FailRequestArchiveJob(ctx, job, "request_archive_target_unavailable", "归档存储目标不存在")
	}
	if err != nil {
		return true, retryOrFailRequestArchiveJob(ctx, job, "request_archive_target_unavailable")
	}
	objectKey := job.ObjectKey
	if objectKey == "" {
		objectKey, err = requestArchiveObjectKey(*target, job)
		if err != nil {
			return true, model.FailRequestArchiveJob(ctx, job, "request_archive_object_key_invalid", "归档对象键无效")
		}
		if err := model.MarkRequestArchiveJobObjectLocation(ctx, job, objectKey); err != nil {
			return true, err
		}
	}
	writeParent := ctx
	var cancelExpiry context.CancelFunc
	if job.ExpiresAt > 0 {
		writeParent, cancelExpiry = context.WithDeadline(ctx, time.Unix(job.ExpiresAt, 0))
		defer cancelExpiry()
	}
	if err := writeParent.Err(); err != nil {
		return true, model.FailRequestArchiveJob(ctx, job, "request_archive_expired", "归档任务已超过保留期")
	}
	storageContext, cancelStorage := context.WithTimeout(
		writeParent, requestArchiveStorageTimeoutForSize(int64(len(job.RequestCiphertext))),
	)
	defer cancelStorage()
	writeContext, stopHeartbeat := startRequestArchiveLeaseHeartbeat(storageContext, job)
	writtenKey, objectVersionID, writeErr := writeRequestArchiveObject(writeContext, *target, job)
	leaseErr := stopHeartbeat()
	if leaseErr != nil {
		return true, leaseErr
	}
	if writeErr != nil {
		if job.ExpiresAt > 0 && job.ExpiresAt <= time.Now().Unix() {
			return true, model.FailRequestArchiveJob(ctx, job, "request_archive_expired", "归档任务已超过保留期")
		}
		return true, retryOrFailRequestArchiveJob(ctx, job, "request_archive_storage_unavailable")
	}
	if err := model.MarkRequestArchiveJobObjectVersion(ctx, job, objectVersionID); err != nil {
		return true, err
	}
	if err := model.FinishRequestArchiveJob(ctx, job, writtenKey, objectVersionID); err != nil {
		return true, err
	}
	requestArchiveLastProcessed.Store(time.Now().Unix())
	requestArchiveLastError.Store("")
	return true, nil
}

func requestArchiveStorageTimeoutForSize(ciphertextBytes int64) time.Duration {
	if ciphertextBytes < 0 {
		ciphertextBytes = 0
	}
	mebibytes := (ciphertextBytes + (1 << 20) - 1) >> 20
	timeout := requestArchiveStorageBaseTimeout + time.Duration(mebibytes)*requestArchiveStorageTimeoutPerMiB
	if timeout > requestArchiveStorageMaxTimeout {
		return requestArchiveStorageMaxTimeout
	}
	return timeout
}

func retryOrFailRequestArchiveJob(ctx context.Context, job *model.RequestArchiveJob, code string) error {
	if job != nil && job.Attempts < model.RequestArchiveJobMaxAttempts {
		backoff := []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute}
		index := job.Attempts - 1
		if index < 0 {
			index = 0
		}
		if index >= len(backoff) {
			index = len(backoff) - 1
		}
		return model.RetryRequestArchiveJob(ctx, job, code, "归档存储暂时不可用", time.Now().Add(backoff[index]))
	}
	return model.FailRequestArchiveJob(ctx, job, code, "归档存储不可用")
}

func startRequestArchiveLeaseHeartbeat(parent context.Context, job *model.RequestArchiveJob) (context.Context, func() error) {
	workContext, cancelWork := context.WithCancel(parent)
	heartbeatContext, cancelHeartbeat := context.WithCancel(parent)
	done := make(chan error, 1)
	go func() {
		interval := requestArchiveLease / 3
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				leaseContext, cancelLease := context.WithTimeout(parent, requestArchiveLeaseRenewTimeout)
				err := model.RenewRequestArchiveJobLease(leaseContext, job, requestArchiveLease)
				cancelLease()
				if err != nil {
					cancelWork()
					done <- err
					return
				}
			case <-heartbeatContext.Done():
				done <- nil
				return
			}
		}
	}()
	return workContext, func() error {
		cancelHeartbeat()
		err := <-done
		cancelWork()
		return err
	}
}

type requestArchiveRuntime struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type requestArchiveWorkerHandle struct {
	stop     chan struct{}
	done     chan struct{}
	stopping bool
}

var (
	requestArchiveRuntimeMu      sync.RWMutex
	requestArchiveRuntimeCurrent *requestArchiveRuntime
	requestArchiveWorkerSequence atomic.Int64
	requestArchiveWorkerRunning  atomic.Int64
	requestArchiveWorkerActive   atomic.Int64
	requestArchiveHeartbeatAt    atomic.Int64
	requestArchiveLastProcessed  atomic.Int64
	requestArchiveLastError      atomic.Value
	requestArchiveEnqueued       atomic.Int64
	requestArchiveDropped        atomic.Int64
	requestArchiveLastEnqueue    atomic.Value
)

// RecordRequestArchiveDropped 只记录稳定错误码。归档是异步旁路，任何入队
// 失败都不得写入用户正文、Authorization 或底层存储错误。
func RecordRequestArchiveDropped(err error) {
	requestArchiveDropped.Add(1)
	requestArchiveLastEnqueue.Store(requestArchiveEnqueueErrorCode(err))
}

func requestArchiveEnqueueErrorCode(err error) string {
	switch {
	case errors.Is(err, model.ErrRequestArchiveQueueFull):
		return "request_archive_queue_full"
	case errors.Is(err, model.ErrRequestArchiveBodyTooLarge):
		return "request_archive_body_too_large"
	case errors.Is(err, model.ErrRequestArchiveConfigChanged):
		return "request_archive_config_changed"
	case errors.Is(err, model.ErrRequestArchiveTargetUnavailable):
		return "request_archive_target_unavailable"
	case errors.Is(err, errRequestArchiveEncryptionBusy):
		return "request_archive_encryption_busy"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "request_archive_enqueue_canceled"
	default:
		return "request_archive_enqueue_failed"
	}
}

// InitRequestArchiveRuntime 启动可由主程序接线的独立工作队列。归档关闭后
// 仍保留一个 drain Worker 处理关闭前已入队的任务，不再接收新请求。
func InitRequestArchiveRuntime() error {
	requestArchiveRuntimeMu.Lock()
	defer requestArchiveRuntimeMu.Unlock()
	if requestArchiveRuntimeCurrent != nil {
		return nil
	}
	if model.DB == nil {
		return errors.New("请求归档数据库尚未初始化")
	}
	if err := model.EnsureRequestArchiveDefaultsContext(context.Background()); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	runtime := &requestArchiveRuntime{cancel: cancel, done: make(chan struct{})}
	requestArchiveRuntimeCurrent = runtime
	go runRequestArchiveRuntime(ctx, runtime)
	return nil
}

func ShutdownRequestArchiveRuntime() {
	requestArchiveRuntimeMu.Lock()
	runtime := requestArchiveRuntimeCurrent
	requestArchiveRuntimeCurrent = nil
	requestArchiveRuntimeMu.Unlock()
	if runtime == nil {
		return
	}
	runtime.cancel()
	select {
	case <-runtime.done:
	case <-time.After(6 * time.Second):
		common.SysError("request archive runtime shutdown timed out")
	}
}

func runRequestArchiveRuntime(ctx context.Context, runtime *requestArchiveRuntime) {
	defer close(runtime.done)
	var goroutines sync.WaitGroup
	workers := make(map[int64]*requestArchiveWorkerHandle)
	startWorker := func() {
		workerNumber := requestArchiveWorkerSequence.Add(1)
		handle := &requestArchiveWorkerHandle{stop: make(chan struct{}), done: make(chan struct{})}
		workers[workerNumber] = handle
		goroutines.Add(1)
		go func() {
			defer goroutines.Done()
			defer close(handle.done)
			requestArchiveWorkerRunning.Add(1)
			defer requestArchiveWorkerRunning.Add(-1)
			runRequestArchiveWorker(ctx, handle.stop, workerNumber)
		}()
	}
	stopWorker := func(handle *requestArchiveWorkerHandle) {
		if handle == nil || handle.stopping {
			return
		}
		handle.stopping = true
		close(handle.stop)
	}
	reconcileWorkers := func(desired int) {
		if desired < 1 {
			desired = 1
		}
		for workerNumber, handle := range workers {
			select {
			case <-handle.done:
				delete(workers, workerNumber)
			default:
			}
		}
		running := 0
		for _, handle := range workers {
			if !handle.stopping {
				running++
			}
		}
		// 缩容中的 Worker 会完成当前任务后退出。在它们尚未退出时不补新
		// Worker，避免配置快速反复变更导致实际并发短暂超过目标值。
		alive := len(workers)
		for running < desired && alive < desired {
			startWorker()
			running++
			alive++
		}
		for running > desired {
			var newestNumber int64
			var newest *requestArchiveWorkerHandle
			for workerNumber, handle := range workers {
				if !handle.stopping && workerNumber > newestNumber {
					newestNumber = workerNumber
					newest = handle
				}
			}
			stopWorker(newest)
			running--
		}
	}

	// 即使归档关闭也保留一个 drain Worker，处理关闭前已经持久化的任务。
	reconcileWorkers(1)
	goroutines.Add(1)
	go func() {
		defer goroutines.Done()
		runRequestArchiveMaintenance(ctx)
	}()
	reconcileTicker := time.NewTicker(requestArchiveWorkerReconcileInterval)
	defer reconcileTicker.Stop()
	for {
		if privateConfig, err := loadRequestArchivePrivateConfig(ctx); err == nil && privateConfig != nil {
			reconcileWorkers(requestArchiveDesiredWorkerCount(privateConfig.Config))
		}
		select {
		case <-reconcileTicker.C:
			continue
		case <-ctx.Done():
			for _, handle := range workers {
				stopWorker(handle)
			}
			goroutines.Wait()
			return
		}
	}
}

func requestArchiveDesiredWorkerCount(config *model.RequestArchiveConfig) int {
	if config == nil || !config.Enabled {
		return 1
	}
	if config.WorkerCount < 1 {
		return 1
	}
	if config.WorkerCount > 32 {
		return 32
	}
	return config.WorkerCount
}

func runRequestArchiveWorker(ctx context.Context, stop <-chan struct{}, workerNumber int64) {
	workerId := fmt.Sprintf("request-archive-%d", workerNumber)
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		default:
		}
		requestArchiveWorkerActive.Add(1)
		requestArchiveHeartbeatAt.Store(time.Now().Unix())
		processed, err := ProcessNextRequestArchiveJob(ctx, workerId)
		requestArchiveWorkerActive.Add(-1)
		if err != nil && ctx.Err() == nil {
			requestArchiveLastError.Store(requestArchiveErrorCode(err))
			common.SysError("request archive worker failed: " + requestArchiveErrorCode(err))
		}
		delay := 100 * time.Millisecond
		if !processed {
			delay = 750 * time.Millisecond
		}
		if !waitRequestArchiveWorker(ctx, stop, delay) {
			return
		}
	}
}

func waitRequestArchiveWorker(ctx context.Context, stop <-chan struct{}, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-stop:
		return false
	case <-ctx.Done():
		return false
	}
}

func runRequestArchiveMaintenance(ctx context.Context) {
	recoverTicker := time.NewTicker(30 * time.Second)
	cleanupTicker := time.NewTicker(time.Minute)
	defer recoverTicker.Stop()
	defer cleanupTicker.Stop()
	for {
		select {
		case <-recoverTicker.C:
			if _, err := model.RecoverExpiredRequestArchiveJobs(ctx, time.Now().Unix()); err != nil {
				common.SysError("request archive lease recovery failed")
			}
			for batch := 0; batch < 20; batch++ {
				expired, err := model.ExpirePendingRequestArchiveJobs(ctx, time.Now().Unix())
				if err != nil {
					common.SysError("request archive pending expiry cleanup failed")
					break
				}
				if expired < 500 {
					break
				}
			}
		case <-cleanupTicker.C:
			if _, err := drainExpiredRequestArchiveObjects(ctx, time.Now().Unix(), 500, 45*time.Second); err != nil {
				common.SysError("request archive retention cleanup failed")
			}
		case <-ctx.Done():
			return
		}
	}
}

// drainExpiredRequestArchiveObjects 按时间预算持续清理，不设置固定批次数上限。
// 高吞吐实例因此不会因为每小时硬上限而永久积压；失败对象仍通过游标跳过，
// 下一轮会从头重试它们。
func drainExpiredRequestArchiveObjects(ctx context.Context, now int64, batch int, budget time.Duration) (int64, error) {
	if batch < 1 || batch > 500 {
		batch = 500
	}
	if budget <= 0 {
		budget = time.Second
	}
	deadline := time.Now().Add(budget)
	budgetContext, cancelBudget := context.WithDeadline(ctx, deadline)
	defer cancelBudget()
	cursor := int64(0)
	var total int64
	for time.Now().Before(deadline) {
		removed, scanned, nextCursor, err := cleanupExpiredRequestArchiveObjects(budgetContext, now, batch, cursor)
		total += removed
		if err != nil {
			return total, err
		}
		if scanned == 0 || scanned < batch || nextCursor <= cursor {
			return total, nil
		}
		cursor = nextCursor
	}
	return total, nil
}

// CleanupExpiredRequestArchiveObjects 按数据库任务的精确 object_key 删除，
// 成功（或对象已不存在）后才删对应任务行。仅在版本号丢失时按精确 key
// 查询对象版本，绝不做无界 bucket 枚举，也不递归删本地目录；删除失败只
// 保留任务并记录稳定错误，供下一轮重试。
func CleanupExpiredRequestArchiveObjects(ctx context.Context, now int64, batch int) (int64, error) {
	removed, _, _, err := cleanupExpiredRequestArchiveObjects(ctx, now, batch, 0)
	return removed, err
}

func cleanupExpiredRequestArchiveObjects(ctx context.Context, now int64, batch int, afterID int64) (int64, int, int64, error) {
	jobs, err := model.ListExpiredRequestArchiveJobsAfter(ctx, now, batch, afterID)
	if err != nil || len(jobs) == 0 {
		return 0, len(jobs), afterID, err
	}
	objectRemovable := make([]model.RequestArchiveObjectCleanupMatch, 0, len(jobs))
	withoutObjectRemovable := make([]int64, 0, len(jobs))
	lastID := afterID
	for _, job := range jobs {
		lastID = job.Id
		if err := ctx.Err(); err != nil {
			return 0, len(jobs), lastID, err
		}
		if job.ObjectKey == "" {
			withoutObjectRemovable = append(withoutObjectRemovable, job.Id)
			continue
		}
		target, targetErr := model.GetRequestArchiveTarget(ctx, job.TargetId)
		if targetErr != nil {
			_ = model.RecordRequestArchiveCleanupError(ctx, job.Id, "request_archive_cleanup_target_unavailable")
			continue
		}
		objectVersionID := job.ObjectVersionId
		objectVersionMode := job.ObjectVersionMode
		reconcileStartedAt := job.CleanupReconcileStartedAt
		if objectVersionMode == model.RequestArchiveObjectVersionUnknown {
			ready, reconcileErr := model.BeginRequestArchiveCleanupReconciliation(
				ctx, job.Id, job.ObjectKey, now, requestArchiveCleanupReconcileQuietPeriod,
			)
			if reconcileErr != nil {
				_ = model.RecordRequestArchiveCleanupError(ctx, job.Id, "request_archive_cleanup_reconcile_persist_failed")
				continue
			}
			if !ready {
				continue
			}
			if target.Type == model.RequestArchiveTargetS3 && objectVersionID == "" {
				resolvedVersionID, objectExists, resolveErr := resolveRequestArchiveS3ObjectVersion(ctx, *target, job.ObjectKey)
				if resolveErr != nil {
					_ = model.RecordRequestArchiveCleanupError(ctx, job.Id, "request_archive_object_version_unconfirmed")
					continue
				}
				if !objectExists {
					confirmErr := model.ConfirmRequestArchiveCleanupAbsent(
						ctx, job.Id, job.ObjectKey, reconcileStartedAt, now,
						requestArchiveCleanupReconcileQuietPeriod,
					)
					if confirmErr != nil {
						_ = model.RecordRequestArchiveCleanupError(ctx, job.Id, "request_archive_object_absence_persist_failed")
						continue
					}
					objectRemovable = append(objectRemovable, model.RequestArchiveObjectCleanupMatch{
						Id: job.Id, Status: job.Status, ByteSize: job.ByteSize, ObjectKey: job.ObjectKey,
						ObjectVersionMode:         model.RequestArchiveObjectVersionAbsent,
						CleanupReconcileStartedAt: reconcileStartedAt,
					})
					continue
				}
				objectVersionID = requestArchiveVersionIDForTarget(*target, resolvedVersionID)
			}
			if persistErr := model.MarkRequestArchiveCleanupObjectVersion(
				ctx, job.Id, job.ObjectKey, objectVersionID,
			); persistErr != nil {
				_ = model.RecordRequestArchiveCleanupError(ctx, job.Id, "request_archive_object_version_persist_failed")
				continue
			}
			objectVersionMode = requestArchiveObjectVersionModeForID(objectVersionID)
			reconcileStartedAt = 0
		}
		if objectVersionMode == model.RequestArchiveObjectVersionAbsent {
			objectRemovable = append(objectRemovable, model.RequestArchiveObjectCleanupMatch{
				Id: job.Id, Status: job.Status, ByteSize: job.ByteSize, ObjectKey: job.ObjectKey,
				ObjectVersionMode:         objectVersionMode,
				CleanupReconcileStartedAt: reconcileStartedAt,
			})
			continue
		}
		deleteErr := deleteRequestArchiveObject(ctx, *target, job.ObjectKey, objectVersionID, objectVersionMode)
		if deleteErr != nil && !errors.Is(deleteErr, errRequestArchiveStoredObjectNotFound) {
			_ = model.RecordRequestArchiveCleanupError(ctx, job.Id, "request_archive_object_cleanup_failed")
			continue
		}
		objectRemovable = append(objectRemovable, model.RequestArchiveObjectCleanupMatch{
			Id: job.Id, Status: job.Status, ByteSize: job.ByteSize, ObjectKey: job.ObjectKey,
			ObjectVersionId: objectVersionID, ObjectVersionMode: objectVersionMode,
			CleanupReconcileStartedAt: reconcileStartedAt,
		})
	}
	objectRemoved, err := model.DeleteExpiredRequestArchiveObjectJobs(ctx, objectRemovable, now)
	if err != nil {
		return objectRemoved, len(jobs), lastID, err
	}
	withoutObjectRemoved, err := model.DeleteExpiredRequestArchiveJobs(ctx, withoutObjectRemovable, now)
	return objectRemoved + withoutObjectRemoved, len(jobs), lastID, err
}

func waitRequestArchiveContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func requestArchiveErrorCode(err error) string {
	if err == nil {
		return ""
	}
	return "request_archive_worker_error"
}

func GetRequestArchiveRuntimeSnapshot(ctx context.Context) (RequestArchiveRuntimeSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return RequestArchiveRuntimeSnapshot{}, err
	}
	config, _, err := model.LoadRequestArchiveConfig(ctx)
	if err != nil {
		return RequestArchiveRuntimeSnapshot{}, err
	}
	queue, err := model.GetRequestArchiveRuntimeCounts(ctx, config.QueueCapacity, config.QueueMaxBytes)
	if err != nil {
		return RequestArchiveRuntimeSnapshot{}, err
	}
	requestArchiveRuntimeMu.RLock()
	running := requestArchiveRuntimeCurrent != nil
	requestArchiveRuntimeMu.RUnlock()
	snapshot := RequestArchiveRuntimeSnapshot{
		Enabled: config.Enabled, ConfigVersion: config.ConfigVersion,
		WorkerRunning: running && requestArchiveWorkerRunning.Load() > 0,
		WorkerCount:   config.WorkerCount, WorkerActive: requestArchiveWorkerActive.Load(),
		HeartbeatAt: requestArchiveHeartbeatAt.Load(), LastProcessedAt: requestArchiveLastProcessed.Load(),
		Queue: queue, Enqueued: requestArchiveEnqueued.Load(), Dropped: requestArchiveDropped.Load(),
	}
	if queue.OldestQueuedAt > 0 {
		snapshot.QueueDelayMs = (time.Now().Unix() - queue.OldestQueuedAt) * 1000
	}
	if value := requestArchiveLastError.Load(); value != nil {
		snapshot.LastErrorCode, _ = value.(string)
	}
	if value := requestArchiveLastEnqueue.Load(); value != nil {
		snapshot.LastEnqueueCode, _ = value.(string)
	}
	return snapshot, nil
}
