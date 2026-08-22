package model

import (
	"context"
	"fmt"
	"strings"
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

const taskRefundReconciliationEligibilityIndex = "idx_tasks_refund_reconciliation_eligibility"

func testRefundReconciliationMarker(manual bool) *TaskRefundReconciliation {
	marker := &TaskRefundReconciliation{
		Amount: 100, UserId: 1, AccountingDone: true, CacheRepairDone: true,
	}
	if manual {
		marker.LogWriteAttempted = true
		marker.ManualReconciliationRequired = true
		marker.ManualReconciliationReason = clickHouseRefundManualReason
	}
	return marker
}

func TestImageTaskRefundEligibilityQueriesSkipEarlierNoneligibleRows(t *testing.T) {
	for _, test := range []struct {
		name  string
		state TaskRefundReconciliationState
		find  func(context.Context, int64, int) ([]Task, error)
	}{
		{name: "pending", state: TaskRefundReconciliationStatePending, find: ClaimPendingImageTaskRefundReconciliations},
		{name: "manual", state: TaskRefundReconciliationStateManualUnreported, find: FindUnreportedImageTaskRefundManualReconciliations},
	} {
		t.Run(test.name, func(t *testing.T) {
			truncateTables(t)
			const cutoff int64 = 10_000
			manual := test.state == TaskRefundReconciliationStateManualUnreported
			for i := 0; i < 40; i++ {
				task := &Task{
					TaskID:   fmt.Sprintf("refund-query-noneligible-%s-%02d", test.name, i),
					Platform: constant.TaskPlatformImage, Status: TaskStatusFailure,
					UpdatedAt:   int64(i + 1),
					PrivateData: TaskPrivateData{RefundReconciliation: testRefundReconciliationMarker(manual)},
				}
				require.NoError(t, task.Insert())
			}
			eligible := &Task{
				TaskID:   "refund-query-eligible-" + test.name,
				Platform: constant.TaskPlatformCanvasImage, Status: TaskStatusFailure,
				UpdatedAt: 9_000, RefundReconciliationState: test.state,
				PrivateData: TaskPrivateData{RefundReconciliation: testRefundReconciliationMarker(manual)},
			}
			require.NoError(t, eligible.Insert())

			found, err := test.find(context.Background(), cutoff, 1)
			require.NoError(t, err)
			require.Len(t, found, 1)
			assert.Equal(t, eligible.ID, found[0].ID)
		})
	}
}

func TestImageTaskRefundClaimCASRemainsIdempotent(t *testing.T) {
	truncateTables(t)
	task := &Task{
		TaskID: "refund-claim-cas", Platform: constant.TaskPlatformImage,
		Status: TaskStatusFailure, UpdatedAt: time.Now().Add(-time.Minute).Unix(),
		RefundReconciliationState: TaskRefundReconciliationStatePending,
		PrivateData:               TaskPrivateData{RefundReconciliation: testRefundReconciliationMarker(false)},
	}
	require.NoError(t, task.Insert())

	_, token, claimed, err := claimImageTaskRefundLog(context.Background(), task.ID, task.TaskID)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NotEmpty(t, token)
	_, _, claimed, err = claimImageTaskRefundLog(context.Background(), task.ID, task.TaskID)
	require.NoError(t, err)
	assert.False(t, claimed)

	var persisted Task
	require.NoError(t, DB.First(&persisted, task.ID).Error)
	assert.Equal(t, TaskRefundReconciliationStatePending, persisted.RefundReconciliationState)
	require.NoError(t, releaseImageTaskRefundLogClaim(context.Background(), task.ID, token))
	_, retryToken, claimed, err := claimImageTaskRefundLog(context.Background(), task.ID, task.TaskID)
	require.NoError(t, err)
	assert.True(t, claimed)
	assert.NotEmpty(t, retryToken)
}

func TestImageTaskRefundStateTransitionsStayWithPrivateMarker(t *testing.T) {
	truncateTables(t)
	user := &User{
		Id: 8801, Username: "refund-state-user", AffCode: "refund-state-aff",
		Quota: 900, UsedQuota: 100, Status: common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(user).Error)
	task := &Task{
		TaskID: "refund-state-transitions", UserId: user.Id,
		Platform: constant.TaskPlatformImage, Status: TaskStatusFailure,
		Quota: 100, UpdatedAt: time.Now().Unix(),
		PrivateData: TaskPrivateData{BillingSource: "wallet"},
	}
	require.NoError(t, task.Insert())

	persisted, claimed, err := RefundImageTaskMoney(context.Background(), task.ID, task.Quota, "provider failed")
	require.NoError(t, err)
	require.True(t, claimed)
	require.NotNil(t, persisted.PrivateData.RefundReconciliation)
	assert.Equal(t, TaskRefundReconciliationStatePending, persisted.RefundReconciliationState)

	persisted, completed, err := ReconcileImageTaskRefundAccounting(context.Background(), task.ID)
	require.NoError(t, err)
	require.True(t, completed)
	assert.Equal(t, TaskRefundReconciliationStatePending, persisted.RefundReconciliationState)
	persisted, err = MarkImageTaskRefundCacheRepaired(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, TaskRefundReconciliationStatePending, persisted.RefundReconciliationState)

	require.NoError(t, FinalizeImageTaskRefundReconciliation(context.Background(), task.ID, RecordTaskBillingLogParams{
		UserId: user.Id, LogType: LogTypeRefund, Quota: 100, RequestId: task.TaskID,
	}))
	require.NoError(t, DB.First(&persisted, task.ID).Error)
	assert.Equal(t, TaskRefundReconciliationStateNone, persisted.RefundReconciliationState)
	assert.Nil(t, persisted.PrivateData.RefundReconciliation)
}

func TestImageTaskRefundManualStateTransitions(t *testing.T) {
	truncateTables(t)
	task := &Task{
		TaskID: "refund-manual-state", Platform: constant.TaskPlatformImage,
		Status: TaskStatusFailure, UpdatedAt: time.Now().Add(-time.Minute).Unix(),
		RefundReconciliationState: TaskRefundReconciliationStatePending,
		PrivateData:               TaskPrivateData{RefundReconciliation: testRefundReconciliationMarker(false)},
	}
	require.NoError(t, task.Insert())

	originalLogDB := LOG_DB
	clickHouse, err := gorm.Open(channelMetricNamedDialector{
		Dialector: sqlite.Open(":memory:"), name: "clickhouse",
	}, &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	require.NoError(t, err)
	LOG_DB = clickHouse
	t.Cleanup(func() { LOG_DB = originalLogDB })

	_, _, claimed, err := claimImageTaskRefundLog(context.Background(), task.ID, task.TaskID)
	require.NoError(t, err)
	require.True(t, claimed)
	var persisted Task
	require.NoError(t, DB.First(&persisted, task.ID).Error)
	require.NotNil(t, persisted.PrivateData.RefundReconciliation)
	assert.True(t, persisted.PrivateData.RefundReconciliation.ManualReconciliationRequired)
	assert.Equal(t, TaskRefundReconciliationStateManualUnreported, persisted.RefundReconciliationState)

	require.NoError(t, MarkImageTaskRefundManualReconciliationReported(context.Background(), task.ID))
	require.NoError(t, DB.First(&persisted, task.ID).Error)
	assert.True(t, persisted.PrivateData.RefundReconciliation.ManualReconciliationReported)
	assert.Equal(t, TaskRefundReconciliationStateManualReported, persisted.RefundReconciliationState)
}

func TestImageTaskRefundTransitionRollbackCannotDesyncStateAndMarker(t *testing.T) {
	truncateTables(t)
	user := &User{
		Id: 8802, Username: "refund-state-rollback", AffCode: "refund-state-rollback-aff",
		Quota: 900, UsedQuota: 100, Status: common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(user).Error)
	task := &Task{
		TaskID: "refund-state-rollback", UserId: user.Id,
		Platform: constant.TaskPlatformImage, Status: TaskStatusFailure,
		Quota: 100, UpdatedAt: time.Now().Unix(),
		PrivateData: TaskPrivateData{BillingSource: "wallet"},
	}
	require.NoError(t, task.Insert())
	require.NoError(t, DB.Exec(fmt.Sprintf(`CREATE TRIGGER fail_refund_state_transition BEFORE UPDATE OF refund_reconciliation_state ON tasks WHEN NEW.id = %d BEGIN SELECT RAISE(ABORT, 'state write failed'); END`, task.ID)).Error)
	t.Cleanup(func() { _ = DB.Exec("DROP TRIGGER IF EXISTS fail_refund_state_transition").Error })

	_, claimed, err := RefundImageTaskMoney(context.Background(), task.ID, task.Quota, "provider failed")
	require.Error(t, err)
	assert.False(t, claimed)
	var persisted Task
	require.NoError(t, DB.First(&persisted, task.ID).Error)
	assert.Equal(t, 100, persisted.Quota)
	assert.Equal(t, TaskRefundReconciliationStateNone, persisted.RefundReconciliationState)
	assert.Nil(t, persisted.PrivateData.RefundReconciliation)
	var persistedUser User
	require.NoError(t, DB.First(&persistedUser, user.Id).Error)
	assert.EqualValues(t, 900, persistedUser.Quota)
}

func TestHasImageTaskMaintenanceWorkUsesOneBoundedQueryAtSteadyState(t *testing.T) {
	truncateTables(t)
	callbackName := "test:count_image_task_maintenance_queries"
	queries := 0
	require.NoError(t, DB.Callback().Query().Before("gorm:query").Register(callbackName, func(*gorm.DB) {
		queries++
	}))
	t.Cleanup(func() { _ = DB.Callback().Query().Remove(callbackName) })

	assert.False(t, HasImageTaskMaintenanceWork(time.Now().Unix(), 60, 3_600))
	assert.Equal(t, 1, queries)
}

func TestHasImageTaskMaintenanceWorkKeepsTimeoutAndRetentionEligibility(t *testing.T) {
	for _, test := range []struct {
		name             string
		task             Task
		timeoutSeconds   int64
		retentionSeconds int64
	}{
		{
			name: "timeout",
			task: Task{
				TaskID: "maintenance-timeout", Platform: constant.TaskPlatformImage,
				Status: TaskStatusInProgress, SubmitTime: 800,
			},
			timeoutSeconds: 100,
		},
		{
			name: "retention",
			task: Task{
				TaskID: "maintenance-retention", Platform: constant.TaskPlatformCanvasImage,
				Status: TaskStatusSuccess, FinishTime: 800, Data: []byte(`{"result":true}`),
			},
			retentionSeconds: 100,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			truncateTables(t)
			require.NoError(t, test.task.Insert())
			assert.True(t, HasImageTaskMaintenanceWork(1_000, test.timeoutSeconds, test.retentionSeconds))
		})
	}
}

func TestTaskRefundReconciliationEligibilityMigrationAndIndex(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE tasks (id integer primary key autoincrement)").Error)
	require.NoError(t, db.Exec("INSERT INTO tasks DEFAULT VALUES").Error)

	require.NoError(t, db.AutoMigrate(&Task{}))
	assert.True(t, db.Migrator().HasColumn(&Task{}, "RefundReconciliationState"))
	assert.True(t, db.Migrator().HasIndex(&Task{}, taskRefundReconciliationEligibilityIndex))
	var state TaskRefundReconciliationState
	require.NoError(t, db.Table("tasks").Select("refund_reconciliation_state").Where("id = ?", 1).Scan(&state).Error)
	assert.Equal(t, TaskRefundReconciliationStateNone, state)

	require.NoError(t, db.AutoMigrate(&Task{}))
	assert.True(t, db.Migrator().HasIndex(&Task{}, taskRefundReconciliationEligibilityIndex))

	statement := &gorm.Statement{DB: db}
	require.NoError(t, statement.Parse(&Task{}))
	var fields []string
	for _, index := range statement.Schema.ParseIndexes() {
		if index.Name != taskRefundReconciliationEligibilityIndex {
			continue
		}
		for _, field := range index.Fields {
			fields = append(fields, field.Field.DBName)
		}
	}
	assert.Equal(t, []string{
		"refund_reconciliation_state", "status", "platform", "quota", "updated_at", "id",
	}, fields)
}

func TestImageTaskRefundEligibilityDryRunSQLIsPortable(t *testing.T) {
	dialectors := []struct {
		name      string
		dialector gorm.Dialector
	}{
		{name: "sqlite", dialector: sqlite.Open(":memory:")},
		{name: "mysql", dialector: mysql.New(mysql.Config{
			DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local",
			SkipInitializeWithVersion: true,
		})},
		{name: "postgres", dialector: postgres.New(postgres.Config{
			DSN:                  "host=localhost user=gorm password=gorm dbname=gorm port=9920 sslmode=disable",
			PreferSimpleProtocol: true,
		})},
	}
	for _, test := range dialectors {
		t.Run(test.name, func(t *testing.T) {
			recorder := &migrationSQLRecorder{}
			db, err := gorm.Open(test.dialector, &gorm.Config{
				DryRun: true, DisableAutomaticPing: true, Logger: recorder,
			})
			require.NoError(t, err)
			require.NoError(t, db.Migrator().CreateTable(&Task{}))
			ddl := strings.ToLower(strings.Join(recorder.statements, ";"))
			assert.Contains(t, ddl, "refund_reconciliation_state")
			assert.Contains(t, ddl, taskRefundReconciliationEligibilityIndex)
			var tasks []Task
			result := imageTaskRefundReconciliationsQuery(db, TaskRefundReconciliationStatePending, 100, 1).Find(&tasks)
			require.NoError(t, result.Error)
			sql := strings.ToLower(result.Statement.SQL.String())
			assert.Contains(t, sql, "refund_reconciliation_state")
			assert.Contains(t, sql, "status")
			assert.Contains(t, sql, "platform")
			assert.Contains(t, sql, "quota")
			assert.Contains(t, sql, "updated_at")
			assert.Contains(t, sql, "order by")
			assert.Contains(t, sql, "limit")
			assert.NotContains(t, sql, "json")

			var id int64
			result = imageTaskMaintenanceWorkQuery(db, 100, 10, 20).Pluck("id", &id)
			require.NoError(t, result.Error)
			sql = strings.ToLower(result.Statement.SQL.String())
			assert.Contains(t, sql, "refund_reconciliation_state")
			assert.Contains(t, sql, "limit")
			assert.NotContains(t, sql, "json")
		})
	}
}
