package model

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupPromptAuditTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "prompt-audit.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&PromptAuditConfig{}, &PromptAuditEndpoint{}, &PromptAuditJob{},
		&PromptAuditEvent{}, &PromptAuditQueueState{},
	))
	DB = db
	t.Cleanup(func() {
		DB = oldDB
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, EnsurePromptAuditDefaults())
	return db
}

func TestCountCyberPolicyEventsByUsersUsesAutoBanScope(t *testing.T) {
	db := setupPromptAuditTestDB(t)
	until := time.Now().Unix()
	since := until - int64(time.Hour/time.Second)

	events := []PromptAuditEvent{
		{UserId: 11, Source: promptAuditUpstreamPolicySource, ErrorCode: promptAuditCyberPolicyCode, CreatedAt: since, Categories: "[]", MatchedScanners: "[]", UnknownCategories: "[]"},
		{UserId: 11, Source: promptAuditUpstreamPolicySource, ErrorCode: promptAuditCyberPolicyCode, CreatedAt: until, Categories: "[]", MatchedScanners: "[]", UnknownCategories: "[]"},
		{UserId: 11, Source: promptAuditUpstreamPolicySource, ErrorCode: promptAuditCyberPolicyCode, CreatedAt: since - 1, Categories: "[]", MatchedScanners: "[]", UnknownCategories: "[]"},
		{UserId: 11, Source: "prompt_guard", ErrorCode: promptAuditCyberPolicyCode, CreatedAt: until, Categories: "[]", MatchedScanners: "[]", UnknownCategories: "[]"},
		{UserId: 11, Source: promptAuditUpstreamPolicySource, ErrorCode: "other", CreatedAt: until, Categories: "[]", MatchedScanners: "[]", UnknownCategories: "[]"},
		{UserId: 22, Source: promptAuditUpstreamPolicySource, ErrorCode: promptAuditCyberPolicyCode, CreatedAt: until, Categories: "[]", MatchedScanners: "[]", UnknownCategories: "[]"},
	}
	require.NoError(t, db.Create(&events).Error)

	counts, err := CountCyberPolicyEventsByUsers([]int{11, 22, 11, 0, -1}, since, until)
	require.NoError(t, err)
	require.EqualValues(t, 2, counts[11])
	require.EqualValues(t, 1, counts[22])
	require.NotContains(t, counts, 0)

	empty, err := CountCyberPolicyEventsByUsers(nil, since, until)
	require.NoError(t, err)
	require.Empty(t, empty)
	_, err = CountCyberPolicyEventsByUsers([]int{11}, until, since)
	require.Error(t, err)
}

func TestPromptAuditConfigCAS(t *testing.T) {
	setupPromptAuditTestDB(t)
	cfg, _, err := LoadPromptAuditConfig()
	require.NoError(t, err)
	require.EqualValues(t, 1, cfg.ConfigVersion)

	cfg.Enabled = false
	cfg.WorkerCount = 6
	require.NoError(t, SavePromptAuditConfig(1, cfg, []PromptAuditEndpoint{}))
	updated, _, err := LoadPromptAuditConfig()
	require.NoError(t, err)
	require.EqualValues(t, 2, updated.ConfigVersion)
	require.Equal(t, 6, updated.WorkerCount)

	stale := *cfg
	stale.WorkerCount = 8
	err = SavePromptAuditConfig(1, &stale, nil)
	require.ErrorIs(t, err, ErrPromptAuditConfigConflict)
	current, _, err := LoadPromptAuditConfig()
	require.NoError(t, err)
	require.Equal(t, 6, current.WorkerCount)
}

func TestSavePromptAuditConfigPreservesDisabledEndpoint(t *testing.T) {
	setupPromptAuditTestDB(t)
	cfg, _, err := LoadPromptAuditConfig()
	require.NoError(t, err)
	require.NoError(t, SavePromptAuditConfig(cfg.ConfigVersion, cfg, []PromptAuditEndpoint{{
		Id: "disabled-guard", Name: "Disabled Guard", Protocol: "openai_compatible",
		BaseUrl: "http://127.0.0.1:8080", Model: "Qwen3Guard-Gen-8B",
		TimeoutMs: 3000, InputLimit: 4000, Enabled: false,
	}}))
	_, endpoints, err := LoadPromptAuditConfig()
	require.NoError(t, err)
	require.Len(t, endpoints, 1)
	require.False(t, endpoints[0].Enabled)
}

func TestPromptAuditCiphertextUsesCrossDatabaseLargeText(t *testing.T) {
	tests := []struct {
		name      string
		dialector gorm.Dialector
		wantType  string
	}{
		{
			name: "mysql",
			dialector: mysql.New(mysql.Config{
				DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local",
				SkipInitializeWithVersion: true,
			}),
			wantType: "LONGTEXT",
		},
		{
			name:      "postgres",
			dialector: postgres.Open("host=localhost port=9911 user=gorm dbname=gorm sslmode=disable"),
			wantType:  "TEXT",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, err := gorm.Open(test.dialector, &gorm.Config{
				DisableAutomaticPing: true,
				DryRun:               true,
			})
			require.NoError(t, err)
			statement := &gorm.Statement{DB: db}
			require.NoError(t, statement.Parse(&PromptAuditJob{}))
			field := statement.Schema.LookUpField("PromptCiphertext")
			require.NotNil(t, field)
			dataType := strings.ToUpper(db.Migrator().FullDataTypeOf(field).SQL)
			require.Contains(t, dataType, test.wantType)
		})
	}
}

func TestPromptAuditCiphertextRoundTripExceedsMySQLTextLimit(t *testing.T) {
	db := setupPromptAuditTestDB(t)
	largeCiphertext := PromptAuditLargeText(strings.Repeat("密", 100_000))
	job := &PromptAuditJob{PromptCiphertext: largeCiphertext, Snapshot: "{}", ConfigVersion: 1}
	require.NoError(t, EnqueuePromptAuditJob(job, 10))

	var stored PromptAuditJob
	require.NoError(t, db.First(&stored, job.Id).Error)
	require.Equal(t, largeCiphertext, stored.PromptCiphertext)
}

func TestPromptAuditQueueCapacityClaimAndFencing(t *testing.T) {
	db := setupPromptAuditTestDB(t)
	first := &PromptAuditJob{PromptCiphertext: "cipher-1", Snapshot: "{}", ConfigVersion: 1}
	require.NoError(t, EnqueuePromptAuditJob(first, 1))
	err := EnqueuePromptAuditJob(&PromptAuditJob{PromptCiphertext: "cipher-2", Snapshot: "{}", ConfigVersion: 1}, 1)
	require.Error(t, err)

	claimed, err := ClaimPromptAuditJob("worker-a", time.Minute)
	require.NoError(t, err)
	require.Equal(t, "worker-a", claimed.WorkerId)
	require.EqualValues(t, 1, claimed.ClaimVersion)
	require.Equal(t, 1, claimed.Attempts)

	stale := *claimed
	stale.ClaimVersion--
	err = RetryPromptAuditJob(&stale, "temporary", "safe message", time.Now())
	require.Error(t, err)
	require.NoError(t, RetryPromptAuditJob(claimed, "temporary", "safe message", time.Now()))

	reclaimed, err := ClaimPromptAuditJob("worker-b", time.Minute)
	require.NoError(t, err)
	require.EqualValues(t, 2, reclaimed.ClaimVersion)
	require.Equal(t, 2, reclaimed.Attempts)
	err = FinishPromptAuditJob(claimed, nil, false)
	require.Error(t, err)
	require.NoError(t, FinishPromptAuditJob(reclaimed, nil, false))

	var state PromptAuditQueueState
	require.NoError(t, db.First(&state, "id = ?", PromptAuditConfigID).Error)
	require.Zero(t, state.ActiveCount)
	var stored PromptAuditJob
	require.NoError(t, db.First(&stored, first.Id).Error)
	require.Equal(t, PromptAuditJobDone, stored.Status)
	require.Empty(t, stored.PromptCiphertext)
}

func TestPromptAuditQueueConcurrentAdmissionDoesNotExceedCapacity(t *testing.T) {
	db := setupPromptAuditTestDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// 单连接让 SQLite 稳定串行提交；并发调用仍会竞争同一条条件更新，
	// 用于验证容量计数与任务写入始终处于同一事务。
	sqlDB.SetMaxOpenConns(1)

	const capacity = 5
	const candidates = 24
	var admitted atomic.Int64
	var workers sync.WaitGroup
	workers.Add(candidates)
	for index := 0; index < candidates; index++ {
		go func(jobIndex int) {
			defer workers.Done()
			job := &PromptAuditJob{
				PromptCiphertext: PromptAuditLargeText(fmt.Sprintf("cipher-%d", jobIndex)),
				Snapshot:         "{}",
				ConfigVersion:    1,
			}
			if EnqueuePromptAuditJob(job, capacity) == nil {
				admitted.Add(1)
			}
		}(index)
	}
	workers.Wait()

	require.EqualValues(t, capacity, admitted.Load())
	var state PromptAuditQueueState
	require.NoError(t, db.First(&state, "id = ?", PromptAuditConfigID).Error)
	require.EqualValues(t, capacity, state.ActiveCount)
	var jobCount int64
	require.NoError(t, db.Model(&PromptAuditJob{}).Count(&jobCount).Error)
	require.EqualValues(t, capacity, jobCount)
}

func TestRecoverExpiredPromptAuditJobLease(t *testing.T) {
	setupPromptAuditTestDB(t)
	job := &PromptAuditJob{PromptCiphertext: "cipher", Snapshot: "{}", ConfigVersion: 1}
	require.NoError(t, EnqueuePromptAuditJob(job, 10))
	claimed, err := ClaimPromptAuditJob("worker-a", time.Second)
	require.NoError(t, err)

	recovered, err := RecoverExpiredPromptAuditJobs(claimed.LeaseUntil + 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, recovered)
	var recoveredJob PromptAuditJob
	require.NoError(t, DB.First(&recoveredJob, claimed.Id).Error)
	require.Equal(t, PromptAuditJobRetry, recoveredJob.Status)
	require.Empty(t, recoveredJob.WorkerId)
	require.Zero(t, recoveredJob.LeaseUntil)
}

func TestRecoverExpiredPromptAuditJobStopsAfterMaxAttempts(t *testing.T) {
	setupPromptAuditTestDB(t)
	const encryptedPrompt = "v1.encrypted.prompt"
	job := &PromptAuditJob{
		PromptCiphertext: encryptedPrompt,
		Snapshot: `{"request_id":"lease-terminal","user_id":12,"api_key_id":34,"prompt_hash":"hash-value",` +
			`"redacted_preview":"safe preview","prompt_length":321,"prompt_truncated":true,"message_count":4}`,
		ConfigVersion: 1,
	}
	require.NoError(t, EnqueuePromptAuditJob(job, 10))
	claimed, err := ClaimPromptAuditJob("worker-terminal", time.Second)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&PromptAuditJob{}).Where("id = ?", claimed.Id).Updates(map[string]interface{}{
		"attempts": PromptAuditJobMaxAttempts, "lease_until": time.Now().Unix() - 1,
	}).Error)

	recovered, err := RecoverExpiredPromptAuditJobs(time.Now().Unix())
	require.NoError(t, err)
	require.EqualValues(t, 1, recovered)
	var recoveredJob PromptAuditJob
	require.NoError(t, DB.First(&recoveredJob, claimed.Id).Error)
	require.Equal(t, PromptAuditJobFailed, recoveredJob.Status)
	require.NotZero(t, recoveredJob.FinishedAt)
	require.Zero(t, recoveredJob.LeaseUntil)
	var event PromptAuditEvent
	require.NoError(t, DB.First(&event, "job_id = ?", claimed.Id).Error)
	require.Equal(t, "lease-terminal", event.RequestId)
	require.Equal(t, "prompt_audit_lease_expired", event.ErrorCode)
	require.Equal(t, "error", event.Decision)
	require.Equal(t, encryptedPrompt, string(event.PromptCiphertext))
	require.Equal(t, PromptAuditCipherKindJobPayload, event.PromptCipherKind)
	require.Equal(t, "safe preview", event.RedactedPreview)
	require.Equal(t, 321, event.PromptLength)
	require.True(t, event.PromptTruncated)
	require.Equal(t, 4, event.MessageCount)
	require.Greater(t, event.ExpiresAt, event.CreatedAt)
	var state PromptAuditQueueState
	require.NoError(t, DB.First(&state, "id = ?", PromptAuditConfigID).Error)
	require.Zero(t, state.ActiveCount)
}

func TestRenewPromptAuditJobLeaseUsesFencing(t *testing.T) {
	setupPromptAuditTestDB(t)
	job := &PromptAuditJob{PromptCiphertext: "cipher", Snapshot: "{}", ConfigVersion: 1}
	require.NoError(t, EnqueuePromptAuditJob(job, 10))
	claimed, err := ClaimPromptAuditJob("worker-a", time.Second)
	require.NoError(t, err)
	originalLease := claimed.LeaseUntil
	require.NoError(t, RenewPromptAuditJobLease(claimed, time.Minute))
	require.Greater(t, claimed.LeaseUntil, originalLease)

	stale := *claimed
	stale.ClaimVersion--
	require.Error(t, RenewPromptAuditJobLease(&stale, time.Minute))
	recovered, err := RecoverExpiredPromptAuditJobs(originalLease + 1)
	require.NoError(t, err)
	require.Zero(t, recovered)
}

func TestCleanupPromptAuditDataDeletesOnlyExpiredIDs(t *testing.T) {
	db := setupPromptAuditTestDB(t)
	now := time.Now().Unix()
	expired := promptAuditTestEvent("expired", now-10)
	future := promptAuditTestEvent("future", now+3600)
	require.NoError(t, db.Create(&expired).Error)
	require.NoError(t, db.Create(&future).Error)

	deleted, _, err := CleanupPromptAuditData(now, 500)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)
	var events []PromptAuditEvent
	require.NoError(t, db.Order("id ASC").Find(&events).Error)
	require.Len(t, events, 1)
	require.Equal(t, "future", events[0].RequestId)
}

func TestCleanupFinishedPromptAuditJobsKeepsActiveTasks(t *testing.T) {
	db := setupPromptAuditTestDB(t)
	now := time.Now().Unix()
	jobs := []PromptAuditJob{
		{Status: PromptAuditJobDone, PromptCiphertext: "", Snapshot: "{}", ConfigVersion: 1, CreatedAt: now - 200, UpdatedAt: now - 100, FinishedAt: now - 100},
		{Status: PromptAuditJobQueued, PromptCiphertext: "queued-cipher", Snapshot: "{}", ConfigVersion: 1, CreatedAt: now - 200, UpdatedAt: now - 100, FinishedAt: 0},
		{Status: PromptAuditJobProcessing, PromptCiphertext: "processing-cipher", Snapshot: "{}", ConfigVersion: 1, CreatedAt: now - 200, UpdatedAt: now - 100, FinishedAt: now - 100},
	}
	require.NoError(t, db.Create(&jobs).Error)

	deleted, err := CleanupFinishedPromptAuditJobs(now-50, 500)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)
	var remaining []PromptAuditJob
	require.NoError(t, db.Order("id ASC").Find(&remaining).Error)
	require.Len(t, remaining, 2)
	require.Equal(t, PromptAuditJobQueued, remaining[0].Status)
	require.Equal(t, PromptAuditJobProcessing, remaining[1].Status)
}

func TestCleanupExpiredPromptAuditJobsRemovesCiphertextAndRepairsCapacity(t *testing.T) {
	db := setupPromptAuditTestDB(t)
	now := time.Now().Unix()
	jobs := []*PromptAuditJob{
		{PromptCiphertext: "expired-queued", Snapshot: "{}", ConfigVersion: 1},
		{PromptCiphertext: "expired-retry", Snapshot: "{}", ConfigVersion: 1},
		{PromptCiphertext: "expired-processing", Snapshot: "{}", ConfigVersion: 1},
		{PromptCiphertext: "fresh-queued", Snapshot: "{}", ConfigVersion: 1},
	}
	for _, job := range jobs {
		require.NoError(t, EnqueuePromptAuditJob(job, 10))
	}
	require.NoError(t, db.Model(&PromptAuditJob{}).Where("id IN ?", []int64{jobs[0].Id, jobs[1].Id, jobs[2].Id}).
		Update("created_at", now-200).Error)
	require.NoError(t, db.Model(&PromptAuditJob{}).Where("id = ?", jobs[1].Id).
		Update("status", PromptAuditJobRetry).Error)
	require.NoError(t, db.Model(&PromptAuditJob{}).Where("id = ?", jobs[2].Id).
		Updates(map[string]interface{}{
			"status": PromptAuditJobProcessing, "worker_id": "active-worker", "lease_until": now + 60,
		}).Error)

	deleted, err := CleanupExpiredPromptAuditJobs(now-100, 500)
	require.NoError(t, err)
	require.EqualValues(t, 2, deleted)

	var remaining []PromptAuditJob
	require.NoError(t, db.Order("id ASC").Find(&remaining).Error)
	require.Len(t, remaining, 2)
	require.Equal(t, PromptAuditJobProcessing, remaining[0].Status)
	require.Equal(t, PromptAuditJobQueued, remaining[1].Status)
	var state PromptAuditQueueState
	require.NoError(t, db.First(&state, "id = ?", PromptAuditConfigID).Error)
	require.EqualValues(t, 2, state.ActiveCount)
}

func TestDeletePromptAuditEventMissing(t *testing.T) {
	setupPromptAuditTestDB(t)
	_, _, err := DeletePromptAuditEvent(999)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound))
}

func TestPromptAuditEventKeywordTreatsWildcardsLiterally(t *testing.T) {
	db := setupPromptAuditTestDB(t)
	withWildcard := promptAuditTestEvent("wildcard", time.Now().Add(time.Hour).Unix())
	withWildcard.RedactedPreview = "cost 100%_done"
	ordinary := promptAuditTestEvent("ordinary", time.Now().Add(time.Hour).Unix())
	ordinary.RedactedPreview = "ordinary preview"
	require.NoError(t, db.Create(&withWildcard).Error)
	require.NoError(t, db.Create(&ordinary).Error)

	for _, keyword := range []string{"%", "_", "%_"} {
		events, total, err := ListPromptAuditEvents(PromptAuditEventFilter{Keyword: keyword}, 1, 20)
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		require.Len(t, events, 1)
		require.Equal(t, "wildcard", events[0].RequestId)
	}
}

func TestDeletePromptAuditEventsByFilterBatchesBeyondSQLiteParameterLimit(t *testing.T) {
	db := setupPromptAuditTestDB(t)
	events := make([]PromptAuditEvent, 1205)
	for index := range events {
		events[index] = promptAuditTestEvent(fmt.Sprintf("batch-%d", index), time.Now().Add(time.Hour).Unix())
	}
	require.NoError(t, db.CreateInBatches(events, 100).Error)

	deletedEvents, deletedJobs, err := DeletePromptAuditEventsByFilter(PromptAuditEventFilter{Decision: "flag"})
	require.NoError(t, err)
	require.EqualValues(t, len(events), deletedEvents)
	require.Zero(t, deletedJobs)

	var remaining int64
	require.NoError(t, db.Model(&PromptAuditEvent{}).Count(&remaining).Error)
	require.Zero(t, remaining)
}

func TestDeletePromptAuditEventsByFilterRejectsUnboundedCriteria(t *testing.T) {
	setupPromptAuditTestDB(t)
	_, _, err := DeletePromptAuditEventsByFilter(PromptAuditEventFilter{})
	require.ErrorContains(t, err, "至少需要一个筛选条件")
	_, _, err = DeletePromptAuditEventsByFilter(PromptAuditEventFilter{UserId: -1})
	require.ErrorContains(t, err, "不能为负数")
}

func TestDeletePromptAuditEventsByIdsKeepsSharedFinishedJob(t *testing.T) {
	db := setupPromptAuditTestDB(t)
	now := time.Now().Unix()
	job := PromptAuditJob{Status: PromptAuditJobDone, CreatedAt: now, UpdatedAt: now, FinishedAt: now}
	require.NoError(t, db.Create(&job).Error)
	first := PromptAuditEvent{JobId: job.Id, RequestId: "shared-1", CreatedAt: now, ExpiresAt: now + 3600}
	second := PromptAuditEvent{JobId: job.Id, RequestId: "shared-2", CreatedAt: now, ExpiresAt: now + 3600}
	require.NoError(t, db.Create(&first).Error)
	require.NoError(t, db.Create(&second).Error)
	deletedEvents, deletedJobs, err := DeletePromptAuditEventsByIds([]int64{first.Id})
	require.NoError(t, err)
	require.EqualValues(t, 1, deletedEvents)
	require.Zero(t, deletedJobs)
	var persisted PromptAuditJob
	require.NoError(t, db.First(&persisted, job.Id).Error)
	var remaining PromptAuditEvent
	require.NoError(t, db.First(&remaining, second.Id).Error)
}

func TestPromptAuditIntegrationMySQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("设置 TEST_MYSQL_DSN 后运行 MySQL 安全审计集成测试")
	}
	runPromptAuditExternalDatabaseIntegration(t, "mysql", dsn)
}

func TestPromptAuditIntegrationPostgreSQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("设置 TEST_POSTGRES_DSN 后运行 PostgreSQL 安全审计集成测试")
	}
	runPromptAuditExternalDatabaseIntegration(t, "postgres", dsn)
}

func runPromptAuditExternalDatabaseIntegration(t *testing.T, dialect, dsn string) {
	t.Helper()

	var (
		db  *gorm.DB
		err error
	)
	switch dialect {
	case "mysql":
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	case "postgres":
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	default:
		t.Fatalf("不支持的安全审计集成测试数据库：%s", dialect)
	}
	require.NoError(t, err)

	tables := []interface{}{
		&PromptAuditConfig{}, &PromptAuditEndpoint{}, &PromptAuditJob{},
		&PromptAuditEvent{}, &PromptAuditQueueState{},
	}
	for _, table := range tables {
		if db.Migrator().HasTable(table) {
			t.Skipf("拒绝在已有安全审计表的 %s 数据库上运行集成测试", dialect)
		}
	}

	oldDB := DB
	DB = db
	migrated := false
	t.Cleanup(func() {
		DB = oldDB
		if migrated {
			_ = db.Migrator().DropTable(
				&PromptAuditEvent{}, &PromptAuditJob{}, &PromptAuditEndpoint{},
				&PromptAuditQueueState{}, &PromptAuditConfig{},
			)
		}
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.AutoMigrate(tables...))
	migrated = true
	require.NoError(t, EnsurePromptAuditDefaults())

	cfg, _, err := LoadPromptAuditConfig()
	require.NoError(t, err)
	cfg.WorkerCount = 6
	require.NoError(t, SavePromptAuditConfig(cfg.ConfigVersion, cfg, nil))
	updated, _, err := LoadPromptAuditConfig()
	require.NoError(t, err)
	require.Equal(t, 6, updated.WorkerCount)

	job := &PromptAuditJob{PromptCiphertext: "encrypted-job", Snapshot: "{}", ConfigVersion: updated.ConfigVersion}
	require.NoError(t, EnqueuePromptAuditJob(job, 8))
	claimed, err := ClaimPromptAuditJob("integration-worker", time.Minute)
	require.NoError(t, err)
	jobEvent := promptAuditTestEvent("cross-db-job", time.Now().Add(time.Hour).Unix())
	require.NoError(t, FinishPromptAuditJob(claimed, &jobEvent, false))

	directEvent := promptAuditTestEvent("cross-db-direct", time.Now().Add(time.Hour).Unix())
	require.NoError(t, db.Create(&directEvent).Error)
	events, total, err := ListPromptAuditEvents(PromptAuditEventFilter{Decision: "flag"}, 1, 1)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.EqualValues(t, 2, total)

	deletedEvents, deletedJobs, err := DeletePromptAuditEventsByFilter(PromptAuditEventFilter{RequestId: directEvent.RequestId})
	require.NoError(t, err)
	require.EqualValues(t, 1, deletedEvents)
	require.Zero(t, deletedJobs)

	deletedEvents, deletedJobs, err = DeletePromptAuditEventsByIds([]int64{jobEvent.Id})
	require.NoError(t, err)
	require.EqualValues(t, 1, deletedEvents)
	require.EqualValues(t, 1, deletedJobs)

	runtime, err := GetPromptAuditRuntimeCounts(8)
	require.NoError(t, err)
	require.Zero(t, runtime.Active)
}

func promptAuditTestEvent(requestId string, expiresAt int64) PromptAuditEvent {
	return PromptAuditEvent{
		RequestId: requestId, PromptHash: requestId, RedactedPreview: "***",
		PromptCiphertext: "cipher", Decision: "flag", RiskLevel: "high", Action: "Warn",
		Safety: "Unsafe", Categories: "[]", MatchedScanners: "[]",
		CreatedAt: time.Now().Add(-24 * time.Hour).Unix(), ExpiresAt: expiresAt,
	}
}
