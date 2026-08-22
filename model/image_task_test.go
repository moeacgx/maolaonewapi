package model

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestImageTaskStorageOwnershipCleanupAndReconciliation(t *testing.T) {
	truncateTables(t)
	now := time.Now().Unix()
	tasks := []*Task{
		{TaskID: "expired-success", UserId: 1, Platform: constant.TaskPlatformCanvasImage, Status: TaskStatusSuccess, FinishTime: now - 7200, Data: []byte(`{"data":[1]}`)},
		{TaskID: "expired-failure", UserId: 1, Platform: constant.TaskPlatformImage, Status: TaskStatusFailure, FinishTime: now - 7200, Data: []byte(`{"error":true}`)},
		{TaskID: "recent-success", UserId: 1, Platform: constant.TaskPlatformCanvasImage, Status: TaskStatusSuccess, FinishTime: now - 60, Data: []byte(`{"data":[2]}`)},
		{TaskID: "stale-pending", UserId: 1, Platform: constant.TaskPlatformCanvasImage, Status: TaskStatusQueued, SubmitTime: now - 7200, Data: []byte(`{"transient":true}`)},
		{TaskID: "recent-pending", UserId: 1, Platform: constant.TaskPlatformImage, Status: TaskStatusInProgress, SubmitTime: now - 60},
		{TaskID: "other-platform", UserId: 1, Platform: constant.TaskPlatform("video"), Status: TaskStatusSuccess, FinishTime: now - 7200, Data: []byte(`{"video":true}`)},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert())
	}

	owned, exists, err := GetImageTaskByOwnerPlatform(1, "expired-success", constant.TaskPlatformCanvasImage)
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, "expired-success", owned.TaskID)
	for _, lookup := range []struct {
		userID   int
		platform constant.TaskPlatform
	}{{2, constant.TaskPlatformCanvasImage}, {1, constant.TaskPlatformImage}} {
		_, exists, err = GetImageTaskByOwnerPlatform(lookup.userID, "expired-success", lookup.platform)
		require.NoError(t, err)
		assert.False(t, exists)
	}

	cleared, err := ClearExpiredImageTaskData(context.Background(), now-3600, 50)
	require.NoError(t, err)
	assert.EqualValues(t, 2, cleared)

	reconciled, err := ReconcileStaleImageTasks(context.Background(), now-3600, 50, "image generation was interrupted before completion")
	require.NoError(t, err)
	assert.EqualValues(t, 1, reconciled)

	var stale Task
	require.NoError(t, DB.Where("task_id = ?", "stale-pending").First(&stale).Error)
	assert.EqualValues(t, TaskStatusFailure, stale.Status)
	assert.Equal(t, "100%", stale.Progress)
	assert.Empty(t, stale.Data)
	assert.Equal(t, "image generation was interrupted before completion", stale.FailReason)

	var recent Task
	require.NoError(t, DB.Where("task_id = ?", "recent-pending").First(&recent).Error)
	assert.EqualValues(t, TaskStatusInProgress, recent.Status)
	var other Task
	require.NoError(t, DB.Where("task_id = ?", "other-platform").First(&other).Error)
	assert.NotEmpty(t, other.Data)
}

func TestImageTaskAdmissionTransactionCountsCommittedTask(t *testing.T) {
	truncateTables(t)
	user := &User{Id: 701, Username: "image-admission", AffCode: "image-admission-aff", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(user).Error)

	tx, admitted, err := BeginImageTaskAdmission(context.Background(), user.Id, 801, 1, 1)
	require.NoError(t, err)
	require.True(t, admitted)
	task := &Task{
		TaskID: "transactional-admission", UserId: user.Id, Platform: constant.TaskPlatformImage,
		Status: TaskStatusQueued, PrivateData: TaskPrivateData{TokenId: 801},
	}
	require.NoError(t, tx.Create(task).Error)
	require.NoError(t, tx.Commit().Error)

	blockedTx, admitted, err := BeginImageTaskAdmission(context.Background(), user.Id, 801, 1, 1)
	require.NoError(t, err)
	assert.False(t, admitted)
	assert.Nil(t, blockedTx)
}

func TestGenericTaskPollingExcludesLocalImageWrappers(t *testing.T) {
	truncateTables(t)
	now := time.Now().Unix()
	for _, task := range []*Task{
		{TaskID: "canvas-local", Platform: constant.TaskPlatformCanvasImage, Status: TaskStatusInProgress, Progress: "10%", SubmitTime: now - 7200},
		{TaskID: "image-local", Platform: constant.TaskPlatformImage, Status: TaskStatusQueued, Progress: "0%", SubmitTime: now - 7200},
		{TaskID: "video-upstream", Platform: constant.TaskPlatform("sora"), Status: TaskStatusInProgress, Progress: "10%", SubmitTime: now - 7200},
	} {
		require.NoError(t, task.Insert())
	}
	unfinished := GetAllUnFinishSyncTasks(20)
	require.Len(t, unfinished, 1)
	assert.Equal(t, "video-upstream", unfinished[0].TaskID)
	timedOut := GetTimedOutUnfinishedTasks(now-3600, 20)
	require.Len(t, timedOut, 1)
	assert.Equal(t, "video-upstream", timedOut[0].TaskID)
}

func TestImageTaskWalletCacheRepairFencesStaleFillAndRepeatsAfterCrash(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	user := &User{Id: 751, Username: "refund-cache-user", AffCode: "refund-cache-aff", Quota: 900, UsedQuota: 100, Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, DB.First(user, user.Id).Error)
	stale := user.ToBaseUser()
	require.NoError(t, writeUserCache(stale, true))
	staleFillRelease := make(chan struct{})
	staleFillResult := make(chan error, 1)
	go func(snapshot *UserBase) {
		<-staleFillRelease
		staleFillResult <- writeUserCache(snapshot, true)
	}(stale)
	task := &Task{
		TaskID: "wallet-cache-generation", UserId: user.Id, Platform: constant.TaskPlatformImage,
		Status: TaskStatusFailure, Quota: 100, UpdatedAt: time.Now().Unix(),
		PrivateData: TaskPrivateData{BillingSource: "wallet"},
	}
	require.NoError(t, task.Insert())

	persisted, claimed, err := RefundImageTaskMoney(context.Background(), task.ID, 100, "provider failed")
	require.NoError(t, err)
	require.True(t, claimed)
	marker := persisted.PrivateData.RefundReconciliation
	require.NotNil(t, marker)
	assert.Greater(t, marker.WalletQuotaVersion, stale.QuotaVersion)
	_, accountingDone, err := ReconcileImageTaskRefundAccounting(context.Background(), task.ID)
	require.NoError(t, err)
	require.True(t, accountingDone)

	require.NoError(t, RepairUserQuotaCache(user.Id, marker.WalletQuotaVersion, marker.WalletQuota, int64(marker.Amount)))
	// Simulate a process crash after Redis committed but before the durable bit.
	require.NoError(t, RepairUserQuotaCache(user.Id, marker.WalletQuotaVersion, marker.WalletQuota, int64(marker.Amount)))
	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	close(staleFillRelease)
	assert.ErrorIs(t, <-staleFillResult, ErrUserQuotaCachePending)
	assert.EqualValues(t, 1000, cached.Quota)
	assert.Equal(t, marker.WalletQuotaVersion, cached.QuotaVersion)

	_, claimed, err = RefundImageTaskMoney(context.Background(), task.ID, 100, "retry")
	require.NoError(t, err)
	assert.False(t, claimed)
	var reloaded User
	require.NoError(t, DB.First(&reloaded, user.Id).Error)
	assert.EqualValues(t, 1000, reloaded.Quota, "cache recovery retry must never re-credit money")
	_, err = MarkImageTaskRefundCacheRepaired(context.Background(), task.ID)
	require.NoError(t, err)
	var repaired Task
	require.NoError(t, DB.First(&repaired, task.ID).Error)
	require.True(t, repaired.PrivateData.RefundReconciliation.CacheRepairDone)
}

func TestImageTaskWalletCacheRepairDoesNotCreditPostRefundHydration(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	user := &User{Id: 752, Username: "refund-cache-hydrated", AffCode: "refund-cache-hydrated-aff", Quota: 900, Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, DB.First(user, user.Id).Error)
	stale := user.ToBaseUser()
	task := &Task{
		TaskID: "wallet-cache-post-refund-hydration", UserId: user.Id, Platform: constant.TaskPlatformImage,
		Status: TaskStatusFailure, Quota: 100, UpdatedAt: time.Now().Unix(),
		PrivateData: TaskPrivateData{BillingSource: "wallet"},
	}
	require.NoError(t, task.Insert())
	persisted, claimed, err := RefundImageTaskMoney(context.Background(), task.ID, task.Quota, "provider failed")
	require.NoError(t, err)
	require.True(t, claimed)
	marker := persisted.PrivateData.RefundReconciliation
	require.NotNil(t, marker)
	assert.EqualValues(t, 1000, marker.WalletQuota)

	var hydrated User
	require.NoError(t, DB.First(&hydrated, user.Id).Error)
	require.NoError(t, writeUserCache(hydrated.ToBaseUser(), true))
	require.NoError(t, RepairUserQuotaCache(user.Id, marker.WalletQuotaVersion, marker.WalletQuota, int64(marker.Amount)))
	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.EqualValues(t, marker.WalletQuota, cached.Quota)
	assert.Equal(t, marker.WalletQuotaVersion, cached.QuotaVersion)

	// The repair published the floor even though it did not apply a delta.
	assert.ErrorIs(t, writeUserCache(stale, true), ErrUserQuotaCachePending)
	cached, err = cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.EqualValues(t, marker.WalletQuota, cached.Quota)
}

func TestRepairUserQuotaCacheInvalidatesPostSnapshotSpend(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	const userID = 753
	user := &User{
		Id: userID, Username: "refund-cache-spend", AffCode: "refund-cache-spend-aff",
		Quota: 400, QuotaVersion: 4, AuthVersion: 1, Status: common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(user).Error)
	// Keep a complete cache snapshot from before the committed refund.
	var olderSnapshot = user.ToBaseUser()
	require.NoError(t, writeUserCache(olderSnapshot, true))

	tx := DB.Begin()
	require.NoError(t, tx.Error)
	refundVersion, refundQuota, err := IncreaseUserQuotaWithTx(tx, userID, 100)
	require.NoError(t, err)
	require.NoError(t, tx.Commit().Error)
	assert.EqualValues(t, 5, refundVersion)
	assert.EqualValues(t, 500, refundQuota)

	// A spend after that snapshot changes the durable quota but deliberately
	// does not advance quota_version. Mirror the spend in the old cache hash.
	require.NoError(t, decreaseUserQuota(userID, 200))
	require.NoError(t, cacheDecrUserQuota(userID, 200))
	var spent User
	require.NoError(t, DB.First(&spent, userID).Error)
	assert.EqualValues(t, 300, spent.Quota)
	assert.EqualValues(t, refundVersion, spent.QuotaVersion)

	delayedFillRelease := make(chan struct{})
	delayedFillResult := make(chan error, 1)
	go func(snapshot *UserBase) {
		<-delayedFillRelease
		delayedFillResult <- writeUserCache(snapshot, true)
	}(olderSnapshot)
	require.NoError(t, RepairUserQuotaCache(userID, refundVersion, refundQuota, 100))
	assert.Zero(t, common.RDB.Exists(t.Context(), getUserCacheKey(userID)).Val())

	// Repair must invalidate the mismatched hash; the next read hydrates the
	// actual spent DB quota while retaining the committed refund floor.
	hydrated, err := GetUserCache(userID)
	require.NoError(t, err)
	assert.EqualValues(t, spent.Quota, hydrated.Quota)
	assert.EqualValues(t, refundVersion, hydrated.QuotaVersion)

	close(delayedFillRelease)
	assert.ErrorIs(t, <-delayedFillResult, ErrUserQuotaCachePending)
	cached, err := cacheGetUserBase(userID)
	require.NoError(t, err)
	assert.EqualValues(t, spent.Quota, cached.Quota)
	assert.EqualValues(t, refundVersion, cached.QuotaVersion)
}

func TestRefundLogIdempotencyConcurrentSeparateSQLite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "refund-logs.db")
	logDB, err := gorm.Open(sqlite.Open(dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, logDB.AutoMigrate(&Log{}))
	sqlDB, err := logDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })

	params := RecordTaskBillingLogParams{UserId: 1, LogType: LogTypeRefund, RequestId: "concurrent-separate-log", Quota: 100}
	start := make(chan struct{})
	errs := make([]error, 2)
	var wait sync.WaitGroup
	for i := range errs {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			errs[index] = recordTaskBillingLogOnceWithDB(logDB, buildTaskBillingLog(params))
		}(i)
	}
	close(start)
	wait.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	var count int64
	require.NoError(t, logDB.Model(&Log{}).Where("request_id = ? AND type = ?", params.RequestId, LogTypeRefund).Count(&count).Error)
	assert.EqualValues(t, 1, count)

	legacy := &Log{RequestId: "legacy-refund-log", Type: LogTypeRefund, CreatedAt: time.Now().Unix()}
	require.NoError(t, logDB.Create(legacy).Error)
	legacyParams := params
	legacyParams.RequestId = legacy.RequestId
	require.NoError(t, recordTaskBillingLogOnceWithDB(logDB, buildTaskBillingLog(legacyParams)))
	require.NoError(t, logDB.Model(&Log{}).Where("request_id = ? AND type = ?", legacy.RequestId, LogTypeRefund).Count(&count).Error)
	assert.EqualValues(t, 1, count, "pre-migration refund logs must be recognized")

	require.NoError(t, logDB.Create(&Log{RequestId: "normal-duplicate", Type: LogTypeConsume}).Error)
	require.NoError(t, logDB.Create(&Log{RequestId: "normal-duplicate", Type: LogTypeConsume}).Error)
}

func TestClickHouseRefundLogCreateHasNoOnConflict(t *testing.T) {
	recorder := &migrationSQLRecorder{}
	db, err := gorm.Open(channelMetricNamedDialector{
		Dialector: sqlite.Open(":memory:"), name: "clickhouse",
	}, &gorm.Config{DryRun: true, DisableAutomaticPing: true, Logger: recorder})
	require.NoError(t, err)
	params := RecordTaskBillingLogParams{UserId: 1, LogType: LogTypeRefund, RequestId: "clickhouse-dry-run", Quota: 100}
	require.NoError(t, recordTaskBillingLogOnceWithDB(db, buildTaskBillingLog(params)))
	sql := strings.ToUpper(strings.Join(recorder.statements, "\n"))
	assert.Contains(t, sql, "INSERT INTO")
	assert.NotContains(t, sql, "ON CONFLICT")
}

func setupRefundLogSink(t *testing.T, clickHouse bool) *gorm.DB {
	t.Helper()
	sinkPath := filepath.Join(t.TempDir(), "refund-logs.db")
	dialector := gorm.Dialector(sqlite.Open(sinkPath + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"))
	if clickHouse {
		dialector = channelMetricNamedDialector{Dialector: dialector, name: "clickhouse"}
	}
	sink, err := gorm.Open(dialector, &gorm.Config{SkipDefaultTransaction: clickHouse})
	require.NoError(t, err)
	require.NoError(t, sink.AutoMigrate(&Log{}))
	if clickHouse {
		require.NoError(t, sink.Migrator().DropIndex(&Log{}, "uidx_logs_idempotency_key"))
	}
	previousLogDB := LOG_DB
	LOG_DB = sink
	t.Cleanup(func() {
		LOG_DB = previousLogDB
		if sqlDB, err := sink.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return sink
}

func newPendingRefundLogTask(t *testing.T, taskID string) *Task {
	t.Helper()
	task := &Task{
		TaskID: taskID, Platform: constant.TaskPlatformImage, Status: TaskStatusFailure,
		PrivateData: TaskPrivateData{RefundReconciliation: &TaskRefundReconciliation{
			Amount: 100, UserId: 1, AccountingDone: true, CacheRepairDone: true,
		}},
	}
	require.NoError(t, task.Insert())
	return task
}

func refundLogParams(taskID string) RecordTaskBillingLogParams {
	return RecordTaskBillingLogParams{UserId: 1, LogType: LogTypeRefund, RequestId: taskID, Quota: 100}
}

func TestClickHouseRefundLogStalledClaimantNeverOverlaps(t *testing.T) {
	truncateTables(t)
	sink := setupRefundLogSink(t, true)
	task := newPendingRefundLogTask(t, "clickhouse-stalled-claimant")
	insertStarted := make(chan struct{})
	releaseInsert := make(chan struct{})
	var once sync.Once
	require.NoError(t, sink.Callback().Create().Before("gorm:create").Register("test:block_clickhouse_refund", func(tx *gorm.DB) {
		if log, ok := tx.Statement.Dest.(*Log); ok && log.RequestId == task.TaskID {
			once.Do(func() { close(insertStarted) })
			<-releaseInsert
		}
	}))
	firstResult := make(chan error, 1)
	go func() {
		firstResult <- FinalizeImageTaskRefundReconciliation(context.Background(), task.ID, refundLogParams(task.TaskID))
	}()
	select {
	case <-insertStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("first ClickHouse insert did not start")
	}
	var fenced Task
	require.NoError(t, DB.First(&fenced, task.ID).Error)
	require.NotNil(t, fenced.PrivateData.RefundReconciliation)
	marker := fenced.PrivateData.RefundReconciliation
	require.True(t, marker.LogWriteAttempted)
	require.True(t, marker.ManualReconciliationRequired)
	marker.LogWriteAttemptedAt = time.Now().Add(-imageTaskRefundLogClaimLease - time.Second).Unix()
	fenced.PrivateData.RefundReconciliation = marker
	require.NoError(t, DB.Model(&Task{}).Where("id = ?", fenced.ID).Update("private_data", fenced.PrivateData).Error)
	require.NoError(t, FinalizeImageTaskRefundReconciliation(context.Background(), task.ID, refundLogParams(task.TaskID)))
	close(releaseInsert)
	require.NoError(t, <-firstResult)
	var count int64
	require.NoError(t, sink.Model(&Log{}).Where("request_id = ?", task.TaskID).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	var completed Task
	require.NoError(t, DB.First(&completed, task.ID).Error)
	assert.Nil(t, completed.PrivateData.RefundReconciliation)
}

func TestClickHouseRefundLogCrashBeforeInsertLeavesManualSignal(t *testing.T) {
	truncateTables(t)
	sink := setupRefundLogSink(t, true)
	task := newPendingRefundLogTask(t, "clickhouse-crash-before-insert")
	_, token, claimed, err := claimImageTaskRefundLog(context.Background(), task.ID, task.TaskID)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NotEmpty(t, token)

	var crashed Task
	require.NoError(t, DB.First(&crashed, task.ID).Error)
	marker := crashed.PrivateData.RefundReconciliation
	require.NotNil(t, marker)
	assert.True(t, marker.LogWriteAttempted)
	assert.True(t, marker.ManualReconciliationRequired)
	assert.Equal(t, "task-refund:"+task.TaskID, marker.LogIdempotencyKey)
	assert.NotEmpty(t, marker.ManualReconciliationReason)
	assert.Zero(t, crashed.Quota)

	require.NoError(t, FinalizeImageTaskRefundReconciliation(context.Background(), task.ID, refundLogParams(task.TaskID)))
	var count int64
	require.NoError(t, sink.Model(&Log{}).Where("request_id = ?", task.TaskID).Count(&count).Error)
	assert.Zero(t, count, "a crashed attempted state must never be inserted automatically")
	pending, err := ClaimPendingImageTaskRefundReconciliations(context.Background(), time.Now().Add(time.Minute).Unix(), 10)
	require.NoError(t, err)
	assert.Empty(t, pending)
	manual, err := FindUnreportedImageTaskRefundManualReconciliations(context.Background(), time.Now().Add(time.Minute).Unix(), 10)
	require.NoError(t, err)
	require.Len(t, manual, 1)
	assert.Equal(t, task.TaskID, manual[0].TaskID)
}

func TestClickHouseRefundLogAmbiguousErrorIsNeverRetried(t *testing.T) {
	truncateTables(t)
	sink := setupRefundLogSink(t, true)
	task := newPendingRefundLogTask(t, "clickhouse-ambiguous-error")
	attempts := 0
	require.NoError(t, sink.Callback().Create().After("gorm:create").Register("test:ambiguous_clickhouse_refund", func(tx *gorm.DB) {
		if log, ok := tx.Statement.Dest.(*Log); ok && log.RequestId == task.TaskID {
			attempts++
			tx.AddError(errors.New("ambiguous ClickHouse insert result"))
		}
	}))
	err := FinalizeImageTaskRefundReconciliation(context.Background(), task.ID, refundLogParams(task.TaskID))
	require.ErrorContains(t, err, "ambiguous ClickHouse insert result")
	require.ErrorIs(t, err, ErrImageTaskRefundManualReconciliationRequired)
	require.NoError(t, FinalizeImageTaskRefundReconciliation(context.Background(), task.ID, refundLogParams(task.TaskID)))
	assert.Equal(t, 1, attempts)
	var count int64
	require.NoError(t, sink.Model(&Log{}).Where("request_id = ?", task.TaskID).Count(&count).Error)
	assert.EqualValues(t, 1, count, "the accepted but errored insert must not be duplicated")
	var retained Task
	require.NoError(t, DB.First(&retained, task.ID).Error)
	require.NotNil(t, retained.PrivateData.RefundReconciliation)
	assert.True(t, retained.PrivateData.RefundReconciliation.ManualReconciliationRequired)
	assert.Zero(t, retained.Quota)
}

func TestClickHouseRefundLogSuccessfulClaimantFinalizesOnce(t *testing.T) {
	truncateTables(t)
	sink := setupRefundLogSink(t, true)
	task := newPendingRefundLogTask(t, "clickhouse-successful-claimant")
	require.NoError(t, FinalizeImageTaskRefundReconciliation(context.Background(), task.ID, refundLogParams(task.TaskID)))
	require.NoError(t, FinalizeImageTaskRefundReconciliation(context.Background(), task.ID, refundLogParams(task.TaskID)))
	var count int64
	require.NoError(t, sink.Model(&Log{}).Where("request_id = ?", task.TaskID).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	var completed Task
	require.NoError(t, DB.First(&completed, task.ID).Error)
	assert.Nil(t, completed.PrivateData.RefundReconciliation)
}

func TestRelationalRefundLogCrashRetryRemainsExactlyOnce(t *testing.T) {
	truncateTables(t)
	sink := setupRefundLogSink(t, false)
	task := newPendingRefundLogTask(t, "relational-crash-retry")
	_, token, claimed, err := claimImageTaskRefundLog(context.Background(), task.ID, task.TaskID)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NotEmpty(t, token)
	require.NoError(t, recordTaskBillingLogOnceWithDB(sink, buildTaskBillingLog(refundLogParams(task.TaskID))))

	var leased Task
	require.NoError(t, DB.First(&leased, task.ID).Error)
	leased.PrivateData.RefundReconciliation.LogClaimUntil = time.Now().Add(-time.Second).Unix()
	require.NoError(t, DB.Model(&Task{}).Where("id = ?", leased.ID).Update("private_data", leased.PrivateData).Error)
	require.NoError(t, FinalizeImageTaskRefundReconciliation(context.Background(), task.ID, refundLogParams(task.TaskID)))
	var count int64
	require.NoError(t, sink.Model(&Log{}).Where("request_id = ?", task.TaskID).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	var completed Task
	require.NoError(t, DB.First(&completed, task.ID).Error)
	assert.Nil(t, completed.PrivateData.RefundReconciliation)
}

func TestRefundLogIdempotencyOnConfiguredSQLDialects(t *testing.T) {
	tests := []struct {
		name      string
		env       string
		dialector func(string) gorm.Dialector
	}{
		{name: "mysql", env: "TEST_MYSQL_DSN", dialector: func(dsn string) gorm.Dialector {
			return mysql.New(mysql.Config{DSN: dsn, SkipInitializeWithVersion: true})
		}},
		{name: "postgres", env: "TEST_POSTGRES_DSN", dialector: func(dsn string) gorm.Dialector {
			return postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn := os.Getenv(test.env)
			if dsn == "" {
				t.Skip(test.env + " is not configured")
			}
			db, err := gorm.Open(test.dialector(dsn), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&Log{}))
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(4)
			t.Cleanup(func() { _ = sqlDB.Close() })

			requestID := fmt.Sprintf("refund-idempotency-%s-%d", test.name, time.Now().UnixNano())
			t.Cleanup(func() { _ = db.Where("request_id = ?", requestID).Delete(&Log{}).Error })
			params := RecordTaskBillingLogParams{UserId: 1, LogType: LogTypeRefund, RequestId: requestID, Quota: 100}
			start := make(chan struct{})
			errs := make([]error, 2)
			var wait sync.WaitGroup
			for i := range errs {
				wait.Add(1)
				go func(index int) {
					defer wait.Done()
					<-start
					errs[index] = recordTaskBillingLogOnceWithDB(db, buildTaskBillingLog(params))
				}(i)
			}
			close(start)
			wait.Wait()
			for _, err := range errs {
				require.NoError(t, err)
			}
			var count int64
			require.NoError(t, db.Model(&Log{}).Where("request_id = ? AND type = ?", requestID, LogTypeRefund).Count(&count).Error)
			assert.EqualValues(t, 1, count)
		})
	}
}
