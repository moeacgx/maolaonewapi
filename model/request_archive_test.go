package model

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupRequestArchiveTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "request-archive.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	DB = db
	require.NoError(t, MigrateRequestArchive())
	require.NoError(t, EnsureRequestArchiveDefaults())
	t.Cleanup(func() {
		DB = oldDB
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestMigrateRequestArchiveAddsSQLiteDedupeKeyBeforeUniqueIndex(t *testing.T) {
	oldDB := DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "request-archive-upgrade.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() {
		DB = oldDB
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.Migrator().CreateTable(&RequestArchiveJob{}))
	require.NoError(t, db.Migrator().DropColumn(&RequestArchiveJob{}, "DedupeKey"))
	require.False(t, db.Migrator().HasColumn(&RequestArchiveJob{}, "DedupeKey"))

	require.NoError(t, migrateSQLiteRequestArchiveDedupeKey())
	require.NoError(t, db.AutoMigrate(&RequestArchiveJob{}))
	require.True(t, db.Migrator().HasColumn(&RequestArchiveJob{}, "DedupeKey"))
	require.True(t, db.Migrator().HasIndex(&RequestArchiveJob{}, "DedupeKey"))

	dedupeKey := "00000000-0000-0000-0000-000000000001"
	first := RequestArchiveJob{
		DedupeKey: &dedupeKey, Status: RequestArchiveJobQueued, TargetId: "local",
		RequestCiphertext: "payload", SHA256: strings.Repeat("a", 64),
	}
	second := RequestArchiveJob{
		DedupeKey: &dedupeKey, Status: RequestArchiveJobQueued, TargetId: "local",
		RequestCiphertext: "payload", SHA256: strings.Repeat("b", 64),
	}
	require.NoError(t, db.Create(&first).Error)
	require.Error(t, db.Create(&second).Error)
}

func configureRequestArchiveTestTarget(t *testing.T, targetID string, capacity int) *RequestArchiveConfig {
	t.Helper()
	config, _, err := LoadRequestArchiveConfig(context.Background())
	require.NoError(t, err)
	config.Enabled = true
	config.ActiveTargetId = targetID
	config.QueueCapacity = capacity
	target := RequestArchiveTarget{
		Id: targetID, Name: "测试归档", Type: RequestArchiveTargetLocal,
		Enabled: true, LocalPath: t.TempDir(),
	}
	require.NoError(t, SaveRequestArchiveConfig(context.Background(), config.ConfigVersion, config, []RequestArchiveTarget{target}))
	updated, _, err := LoadRequestArchiveConfig(context.Background())
	require.NoError(t, err)
	return updated
}

func TestRequestArchiveConfigCASAndActiveTargetJobs(t *testing.T) {
	setupRequestArchiveTestDB(t)
	config, _, err := LoadRequestArchiveConfig(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 1, config.ConfigVersion)

	target := RequestArchiveTarget{
		Id: "archive-local", Name: "本地归档", Type: RequestArchiveTargetLocal,
		Enabled: true, LocalPath: `D:\archive`,
	}
	config.Enabled = true
	config.ActiveTargetId = target.Id
	require.NoError(t, SaveRequestArchiveConfig(context.Background(), config.ConfigVersion, config, []RequestArchiveTarget{target}))

	updated, targets, err := LoadRequestArchiveConfig(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 2, updated.ConfigVersion)
	require.Len(t, targets, 1)
	require.Equal(t, target.Id, updated.ActiveTargetId)

	stale := *updated
	stale.WorkerCount = 8
	require.ErrorIs(t, SaveRequestArchiveConfig(context.Background(), 1, &stale, targets), ErrRequestArchiveConfigConflict)

	job := &RequestArchiveJob{TargetId: target.Id, ConfigVersion: updated.ConfigVersion, RequestCiphertext: "v1.test.test", SHA256: strings.Repeat("a", 64), ExpiresAt: time.Now().Add(time.Hour).Unix()}
	require.NoError(t, EnqueueRequestArchiveJob(context.Background(), job, 4))
	relocated := target
	relocated.LocalPath = `D:\another-archive`
	relocationConfig := *updated
	require.ErrorIs(t, SaveRequestArchiveConfig(context.Background(), updated.ConfigVersion, &relocationConfig, []RequestArchiveTarget{relocated}), ErrRequestArchiveTargetInUse)
	removal := *updated
	removal.Enabled = false
	removal.ActiveTargetId = ""
	require.ErrorIs(t, SaveRequestArchiveConfig(context.Background(), updated.ConfigVersion, &removal, nil), ErrRequestArchiveTargetInUse)
}

func TestRequestArchiveQueueClaimFencingAndFinish(t *testing.T) {
	db := setupRequestArchiveTestDB(t)
	config := configureRequestArchiveTestTarget(t, "target-1", 1)
	job := &RequestArchiveJob{
		TargetId:          "target-1",
		ConfigVersion:     config.ConfigVersion,
		RequestCiphertext: "v1.ciphertext.payload",
		SHA256:            strings.Repeat("b", 64),
		ExpiresAt:         time.Now().Add(time.Hour).Unix(),
	}
	require.NoError(t, EnqueueRequestArchiveJob(context.Background(), job, 1))
	require.ErrorIs(t, EnqueueRequestArchiveJob(context.Background(), &RequestArchiveJob{
		TargetId: "target-1", ConfigVersion: config.ConfigVersion, RequestCiphertext: "v1.other.payload", SHA256: strings.Repeat("c", 64), ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}, 1), ErrRequestArchiveQueueFull)

	claimed, err := ClaimRequestArchiveJob(context.Background(), "worker-a", time.Minute)
	require.NoError(t, err)
	require.Equal(t, "worker-a", claimed.WorkerId)
	require.EqualValues(t, 1, claimed.ClaimVersion)
	require.Equal(t, 1, claimed.Attempts)

	stale := *claimed
	stale.ClaimVersion--
	require.Error(t, RetryRequestArchiveJob(context.Background(), &stale, "temporary", "固定错误", time.Now()))
	require.NoError(t, RetryRequestArchiveJob(context.Background(), claimed, "temporary", "固定错误", time.Now()))

	reclaimed, err := ClaimRequestArchiveJob(context.Background(), "worker-b", time.Minute)
	require.NoError(t, err)
	require.EqualValues(t, 2, reclaimed.ClaimVersion)
	require.Error(t, FinishRequestArchiveJob(context.Background(), claimed, "requests/2026/01/01/1-a.enc", ""))
	require.NoError(t, FinishRequestArchiveJob(context.Background(), reclaimed, "requests/2026/01/01/1-b.enc", ""))

	var state RequestArchiveQueueState
	require.NoError(t, db.First(&state, "id = ?", RequestArchiveConfigID).Error)
	require.Zero(t, state.ActiveCount)
	var stored RequestArchiveJob
	require.NoError(t, db.First(&stored, job.Id).Error)
	require.Equal(t, RequestArchiveJobDone, stored.Status)
	require.Empty(t, stored.RequestCiphertext)
	require.Equal(t, RequestArchiveObjectVersionUnversioned, stored.ObjectVersionMode)
}

func TestRequestArchiveCleanupVersionRecoveryIsFencedByObjectKey(t *testing.T) {
	db := setupRequestArchiveTestDB(t)
	job := RequestArchiveJob{
		Status: RequestArchiveJobDone, TargetId: "target-version-recovery",
		ObjectKey: "requests/2026/07/28/1-version.enc", SHA256: strings.Repeat("a", 64),
		ExpiresAt: time.Now().Add(-time.Minute).Unix(),
	}
	require.NoError(t, db.Create(&job).Error)
	require.NoError(t, MarkRequestArchiveCleanupObjectVersion(
		context.Background(), job.Id, job.ObjectKey, "opaque-version-1",
	))
	require.NoError(t, MarkRequestArchiveCleanupObjectVersion(
		context.Background(), job.Id, job.ObjectKey, "opaque-version-1",
	), "多实例写入相同恢复结果应保持幂等")
	require.Error(t, MarkRequestArchiveCleanupObjectVersion(
		context.Background(), job.Id, "requests/changed.enc", "opaque-version-2",
	))

	var stored RequestArchiveJob
	require.NoError(t, db.First(&stored, job.Id).Error)
	require.Equal(t, RequestArchiveObjectVersionExact, stored.ObjectVersionMode)
	require.Equal(t, "opaque-version-1", stored.ObjectVersionId)
}

func TestRequestArchiveCleanupReconciliationRequiresPersistedQuietPeriod(t *testing.T) {
	db := setupRequestArchiveTestDB(t)
	now := time.Now().Unix()
	job := RequestArchiveJob{
		Status: RequestArchiveJobDone, TargetId: "target-absence-confirmation",
		ObjectKey: "requests/2026/07/28/2-absence.enc", SHA256: strings.Repeat("b", 64),
		ExpiresAt: now - 1,
	}
	require.NoError(t, db.Create(&job).Error)

	ready, err := BeginRequestArchiveCleanupReconciliation(context.Background(), job.Id, job.ObjectKey, now, 10*time.Minute)
	require.NoError(t, err)
	require.False(t, ready)
	ready, err = BeginRequestArchiveCleanupReconciliation(context.Background(), job.Id, job.ObjectKey, now+599, 10*time.Minute)
	require.NoError(t, err)
	require.False(t, ready)
	require.Error(t, ConfirmRequestArchiveCleanupAbsent(
		context.Background(), job.Id, job.ObjectKey, now, now+599, 10*time.Minute,
	))
	ready, err = BeginRequestArchiveCleanupReconciliation(context.Background(), job.Id, job.ObjectKey, now+600, 10*time.Minute)
	require.NoError(t, err)
	require.True(t, ready)
	require.NoError(t, ConfirmRequestArchiveCleanupAbsent(
		context.Background(), job.Id, job.ObjectKey, now, now+600, 10*time.Minute,
	))
	require.NoError(t, ConfirmRequestArchiveCleanupAbsent(
		context.Background(), job.Id, job.ObjectKey, now, now+600, 10*time.Minute,
	), "多实例写入相同不存在结论应保持幂等")
	require.Error(t, MarkRequestArchiveCleanupObjectVersion(
		context.Background(), job.Id, job.ObjectKey, "late-version",
	), "不存在确认必须与版本固化互斥")
	_, err = BeginRequestArchiveCleanupReconciliation(
		context.Background(), job.Id, "requests/changed.enc", now+600, 10*time.Minute,
	)
	require.Error(t, err)

	var stored RequestArchiveJob
	require.NoError(t, db.First(&stored, job.Id).Error)
	require.Equal(t, RequestArchiveObjectVersionAbsent, stored.ObjectVersionMode)
	require.Equal(t, now, stored.CleanupReconcileStartedAt)
	removed, err := DeleteExpiredRequestArchiveObjectJobs(context.Background(), []RequestArchiveObjectCleanupMatch{{
		Id: job.Id, Status: job.Status, ByteSize: job.ByteSize, ObjectKey: job.ObjectKey,
		ObjectVersionMode: RequestArchiveObjectVersionAbsent, CleanupReconcileStartedAt: now - 1,
	}}, now+600)
	require.NoError(t, err)
	require.Zero(t, removed, "旧实例的不存在确认快照不能删除任务")
	removed, err = DeleteExpiredRequestArchiveObjectJobs(context.Background(), []RequestArchiveObjectCleanupMatch{{
		Id: job.Id, Status: job.Status, ByteSize: job.ByteSize, ObjectKey: job.ObjectKey,
		ObjectVersionMode: RequestArchiveObjectVersionAbsent, CleanupReconcileStartedAt: now,
	}}, now+600)
	require.NoError(t, err)
	require.EqualValues(t, 1, removed)
}

func TestRequestArchiveObjectCleanupDeleteUsesVersionStateCAS(t *testing.T) {
	db := setupRequestArchiveTestDB(t)
	now := time.Now().Unix()
	job := RequestArchiveJob{
		Status: RequestArchiveJobDone, TargetId: "target-cleanup-cas", ByteSize: 7,
		ObjectKey: "requests/2026/07/28/3-cas.enc", ObjectVersionId: "version-2",
		ObjectVersionMode: RequestArchiveObjectVersionExact, SHA256: strings.Repeat("c", 64),
		ExpiresAt: now - 1,
	}
	require.NoError(t, db.Create(&job).Error)

	removed, err := DeleteExpiredRequestArchiveObjectJobs(context.Background(), []RequestArchiveObjectCleanupMatch{{
		Id: job.Id, Status: job.Status, ByteSize: job.ByteSize, ObjectKey: job.ObjectKey,
		ObjectVersionId: "version-1", ObjectVersionMode: RequestArchiveObjectVersionExact,
	}}, now)
	require.NoError(t, err)
	require.Zero(t, removed, "旧实例的版本快照不能删除已变化任务")
	require.NoError(t, db.First(&RequestArchiveJob{}, job.Id).Error)

	removed, err = DeleteExpiredRequestArchiveObjectJobs(context.Background(), []RequestArchiveObjectCleanupMatch{{
		Id: job.Id, Status: job.Status, ByteSize: job.ByteSize, ObjectKey: job.ObjectKey,
		ObjectVersionId: job.ObjectVersionId, ObjectVersionMode: job.ObjectVersionMode,
	}}, now)
	require.NoError(t, err)
	require.EqualValues(t, 1, removed)
}

func TestRequestArchiveDoesNotClaimInsideDeliveryQuietWindow(t *testing.T) {
	db := setupRequestArchiveTestDB(t)
	now := time.Now()
	job := RequestArchiveJob{
		Status: RequestArchiveJobQueued, TargetId: "target-quiet-window", ConfigVersion: 1,
		RequestCiphertext: "cipher", SHA256: strings.Repeat("c", 64), ByteSize: 6,
		NextAttemptAt: now.Add(-time.Minute).Unix(), ExpiresAt: now.Add(RequestArchiveMinimumDeliveryWindow / 2).Unix(),
	}
	require.NoError(t, db.Create(&job).Error)
	candidates, err := ListRequestArchiveJobCandidates(context.Background(), 16)
	require.NoError(t, err)
	require.Empty(t, candidates)

	require.NoError(t, db.Model(&RequestArchiveJob{}).Where("id = ?", job.Id).
		Update("expires_at", now.Add(RequestArchiveMinimumDeliveryWindow+time.Minute).Unix()).Error)
	candidates, err = ListRequestArchiveJobCandidates(context.Background(), 16)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	claimed, err := ClaimRequestArchiveJobCandidate(context.Background(), "quiet-window-worker", time.Minute, candidates[0])
	require.NoError(t, err)
	require.Equal(t, job.Id, claimed.Id)
}

func TestRequestArchiveEnqueueRejectsConfiguredAndAbsoluteBodyLimits(t *testing.T) {
	db := setupRequestArchiveTestDB(t)
	config := configureRequestArchiveTestTarget(t, "target-limits", 4)
	job := func(byteSize int64) *RequestArchiveJob {
		return &RequestArchiveJob{
			TargetId: "target-limits", ConfigVersion: config.ConfigVersion,
			RequestCiphertext: "ra2.payload", SHA256: strings.Repeat("a", 64),
			ByteSize: byteSize, ExpiresAt: time.Now().Add(time.Hour).Unix(),
		}
	}
	require.ErrorIs(t, EnqueueRequestArchiveJob(context.Background(), job(config.MaxBodyBytes+1), 4), ErrRequestArchiveBodyTooLarge)
	require.ErrorIs(t, EnqueueRequestArchiveJob(context.Background(), job(RequestArchiveMaximumBodyBytes+1), 4), ErrRequestArchiveBodyTooLarge)

	corrupt := job(RequestArchiveMaximumBodyBytes + 1)
	corrupt.Status = RequestArchiveJobQueued
	corrupt.NextAttemptAt = time.Now().Unix()
	require.NoError(t, db.Create(corrupt).Error)
	candidates, err := ListRequestArchiveJobCandidates(context.Background(), 16)
	require.NoError(t, err)
	require.Empty(t, candidates)
}

func TestRequestArchiveModelRejectsNonCanonicalTargetIDs(t *testing.T) {
	setupRequestArchiveTestDB(t)
	config, _, err := LoadRequestArchiveConfig(context.Background())
	require.NoError(t, err)
	target := RequestArchiveTarget{Id: "Archive-A", Name: "invalid", Type: RequestArchiveTargetLocal, LocalPath: t.TempDir()}
	require.Error(t, SaveRequestArchiveConfig(context.Background(), config.ConfigVersion, config, []RequestArchiveTarget{target}))

	job := &RequestArchiveJob{TargetId: "Archive-A", ConfigVersion: config.ConfigVersion, ByteSize: 1}
	require.Error(t, EnqueueRequestArchiveJob(context.Background(), job, 1))
}

func TestRequestArchiveLeaseRecoveryAndCleanupSkipsWorkingTasks(t *testing.T) {
	db := setupRequestArchiveTestDB(t)
	config := configureRequestArchiveTestTarget(t, "target-1", 4)
	job := &RequestArchiveJob{
		TargetId: "target-1", ConfigVersion: config.ConfigVersion, RequestCiphertext: "v1.ciphertext.payload", SHA256: strings.Repeat("d", 64),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	require.NoError(t, EnqueueRequestArchiveJob(context.Background(), job, 4))
	claimed, err := ClaimRequestArchiveJob(context.Background(), "worker-a", time.Second)
	require.NoError(t, err)

	recovered, err := RecoverExpiredRequestArchiveJobs(context.Background(), claimed.LeaseUntil+1)
	require.NoError(t, err)
	require.EqualValues(t, 1, recovered)
	var recoveredJob RequestArchiveJob
	require.NoError(t, db.First(&recoveredJob, job.Id).Error)
	require.Equal(t, RequestArchiveJobRetry, recoveredJob.Status)

	removed, err := CleanupFinishedRequestArchiveJobs(context.Background(), time.Now().Unix(), 500)
	require.NoError(t, err)
	require.Zero(t, removed)
	require.NoError(t, db.First(&RequestArchiveJob{}, job.Id).Error)
}

func TestRequestArchiveExpiresQueuedJobsBeforeStorageWrite(t *testing.T) {
	db := setupRequestArchiveTestDB(t)
	config := configureRequestArchiveTestTarget(t, "target-expiry", 4)
	job := &RequestArchiveJob{
		TargetId: "target-expiry", ConfigVersion: config.ConfigVersion,
		RequestCiphertext: "ra1.ciphertext.payload", SHA256: strings.Repeat("f", 64), ByteSize: 42,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	require.NoError(t, EnqueueRequestArchiveJob(context.Background(), job, 4))
	require.NoError(t, db.Model(&RequestArchiveJob{}).Where("id = ?", job.Id).Update("expires_at", time.Now().Add(-time.Minute).Unix()).Error)
	expired, err := ExpirePendingRequestArchiveJobs(context.Background(), time.Now().Unix())
	require.NoError(t, err)
	require.EqualValues(t, 1, expired)
	var stored RequestArchiveJob
	require.NoError(t, db.First(&stored, job.Id).Error)
	require.Equal(t, RequestArchiveJobFailed, stored.Status)
	var state RequestArchiveQueueState
	require.NoError(t, db.First(&state, "id = ?", RequestArchiveConfigID).Error)
	require.EqualValues(t, 1, state.ActiveCount)
	require.EqualValues(t, 42, state.ActiveBytes)
	removed, err := DeleteExpiredRequestArchiveJobs(context.Background(), []int64{job.Id}, time.Now().Unix())
	require.NoError(t, err)
	require.EqualValues(t, 1, removed)
	require.NoError(t, db.First(&state, "id = ?", RequestArchiveConfigID).Error)
	require.Zero(t, state.ActiveCount)
	require.Zero(t, state.ActiveBytes)
}

func TestRequestArchiveEnqueueRejectsChangedConfig(t *testing.T) {
	setupRequestArchiveTestDB(t)
	config := configureRequestArchiveTestTarget(t, "target-version", 4)
	config.ConfigVersion++
	require.NoError(t, DB.Model(&RequestArchiveConfig{}).Where("id = ?", RequestArchiveConfigID).
		Update("config_version", config.ConfigVersion).Error)
	err := EnqueueRequestArchiveJob(context.Background(), &RequestArchiveJob{
		TargetId: "target-version", ConfigVersion: config.ConfigVersion - 1,
		RequestCiphertext: "ra1.ciphertext.payload", SHA256: strings.Repeat("1", 64), ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}, 4)
	require.ErrorIs(t, err, ErrRequestArchiveConfigChanged)
}

func TestRequestArchiveLargeCiphertextUsesCrossDatabaseType(t *testing.T) {
	tests := []struct {
		name      string
		dialector gorm.Dialector
		wantType  string
	}{
		{
			name: "mysql",
			dialector: mysql.New(mysql.Config{
				DSN: "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local", SkipInitializeWithVersion: true,
			}),
			wantType: "LONGTEXT",
		},
		{
			name: "postgres", dialector: postgres.Open("host=localhost port=9911 user=gorm dbname=gorm sslmode=disable"), wantType: "TEXT",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, err := gorm.Open(test.dialector, &gorm.Config{DisableAutomaticPing: true, DryRun: true})
			require.NoError(t, err)
			statement := &gorm.Statement{DB: db}
			require.NoError(t, statement.Parse(&RequestArchiveJob{}))
			field := statement.Schema.LookUpField("RequestCiphertext")
			require.NotNil(t, field)
			typeName := strings.ToUpper(db.Migrator().FullDataTypeOf(field).SQL)
			require.Contains(t, typeName, test.wantType)
		})
	}
}

func TestRequestArchiveQueueStateKeepsFailedPayloadBoundedUntilCleanup(t *testing.T) {
	db := setupRequestArchiveTestDB(t)
	config := configureRequestArchiveTestTarget(t, "target-1", 2)
	job := &RequestArchiveJob{TargetId: "target-1", ConfigVersion: config.ConfigVersion, RequestCiphertext: "cipher", SHA256: strings.Repeat("e", 64), ByteSize: 7, ExpiresAt: time.Now().Add(time.Hour).Unix()}
	require.NoError(t, EnqueueRequestArchiveJob(context.Background(), job, 2))
	claimed, err := ClaimRequestArchiveJob(context.Background(), "worker", time.Minute)
	require.NoError(t, err)
	require.NoError(t, FailRequestArchiveJob(context.Background(), claimed, "storage", "固定错误"))
	counts, err := GetRequestArchiveRuntimeCounts(context.Background(), 2)
	require.NoError(t, err)
	require.EqualValues(t, 1, counts.Active)
	require.EqualValues(t, 7, counts.ActiveBytes)
	require.EqualValues(t, 1, counts.Failed)
	require.NoError(t, db.Model(&RequestArchiveJob{}).Where("id = ?", job.Id).Update("expires_at", time.Now().Add(-time.Minute).Unix()).Error)
	removed, err := DeleteExpiredRequestArchiveJobs(context.Background(), []int64{job.Id}, time.Now().Unix())
	require.NoError(t, err)
	require.EqualValues(t, 1, removed)
	counts, err = GetRequestArchiveRuntimeCounts(context.Background(), 2)
	require.NoError(t, err)
	require.Zero(t, counts.Active)
	require.Zero(t, counts.ActiveBytes)

	_, err = GetRequestArchiveJob(context.Background(), 999999)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound))
}

func TestRequestArchiveExpiryIndexAndOpaqueVersionValidation(t *testing.T) {
	db := setupRequestArchiveTestDB(t)
	require.True(t, db.Migrator().HasIndex(&RequestArchiveJob{}, "idx_request_archive_expiry"))
	require.NoError(t, validateRequestArchiveObjectVersion("opaque version id with spaces"))
	require.Error(t, validateRequestArchiveObjectVersion(strings.Repeat("x", 4097)))
	require.Error(t, validateRequestArchiveObjectVersion("invalid\x00version"))
}

func TestLoadRequestArchiveConfigOnlyUpsertsMissingSingleton(t *testing.T) {
	db := setupRequestArchiveTestDB(t)
	createCalls := 0
	const callbackName = "request_archive_test_count_config_create"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == (RequestArchiveConfig{}).TableName() {
			createCalls++
		}
	}))
	t.Cleanup(func() { db.Callback().Create().Remove(callbackName) })

	_, _, err := LoadRequestArchiveConfig(context.Background())
	require.NoError(t, err)
	require.Zero(t, createCalls, "配置已存在时不应重复执行默认 upsert")

	require.NoError(t, db.Delete(&RequestArchiveConfig{}, RequestArchiveConfigID).Error)
	_, _, err = LoadRequestArchiveConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, createCalls, "仅缺失配置单例时执行一次默认 upsert")
}

func TestRequestArchiveDatabaseOperationsHonorContextCancellation(t *testing.T) {
	setupRequestArchiveTestDB(t)
	config := configureRequestArchiveTestTarget(t, "target-context", 4)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := LoadRequestArchiveConfig(ctx)
	require.ErrorIs(t, err, context.Canceled)
	err = EnqueueRequestArchiveJob(ctx, &RequestArchiveJob{
		TargetId: "target-context", ConfigVersion: config.ConfigVersion,
		RequestCiphertext: "ra1.ciphertext.payload", SHA256: strings.Repeat("a", 64),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}, 4)
	require.ErrorIs(t, err, context.Canceled)
}

func TestCleanupFinishedRequestArchiveJobsSkipsStoredObjects(t *testing.T) {
	db := setupRequestArchiveTestDB(t)
	now := time.Now().Unix()
	jobs := []RequestArchiveJob{
		{
			Status: RequestArchiveJobFailed, TargetId: "target-cleanup", ObjectKey: "",
			RequestCiphertext: "cipher-a", SHA256: strings.Repeat("a", 64), ExpiresAt: now - 1,
		},
		{
			Status: RequestArchiveJobFailed, TargetId: "target-cleanup", ObjectKey: "requests/2026/01/01/2.enc",
			RequestCiphertext: "cipher-b", SHA256: strings.Repeat("b", 64), ExpiresAt: now - 1,
		},
	}
	require.NoError(t, db.Create(&jobs).Error)

	removed, err := CleanupFinishedRequestArchiveJobs(context.Background(), now, 500)
	require.NoError(t, err)
	require.EqualValues(t, 1, removed)
	var remaining []RequestArchiveJob
	require.NoError(t, db.Order("id ASC").Find(&remaining).Error)
	require.Len(t, remaining, 1)
	require.Equal(t, jobs[1].ObjectKey, remaining[0].ObjectKey)
}
