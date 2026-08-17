package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	channelmetrics "github.com/QuantumNous/new-api/pkg/channel_metrics"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPersistChannelFailureEventsCancelsBlockedWrite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ChannelFailureEvent{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:block_until_cancel", func(tx *gorm.DB) {
		cancel()
		select {
		case <-tx.Statement.Context.Done():
			tx.AddError(tx.Statement.Context.Err())
		case <-time.After(time.Second):
			tx.AddError(context.DeadlineExceeded)
		}
	}))

	startedAt := time.Now()
	err = persistChannelFailureEvents(ctx, db, []model.ChannelFailureEvent{{
		EventId:   "cancelled-write",
		CreatedAt: time.Now().Unix(),
	}})
	require.ErrorIs(t, err, context.Canceled)
	require.Less(t, time.Since(startedAt), 500*time.Millisecond)
}

func TestDeleteChannelMetricBatchesSignalsCatchUpAtLimit(t *testing.T) {
	calls := 0
	needsCatchUp := deleteChannelMetricBatches("test backlog", func() (int64, error) {
		calls++
		return model.ChannelMetricMaxDeleteBatch, nil
	})

	require.True(t, needsCatchUp)
	require.Equal(t, channelMetricCleanupMaxBatches, calls)
}

func TestDeleteChannelMetricBatchesStopsAfterPartialBatch(t *testing.T) {
	calls := 0
	needsCatchUp := deleteChannelMetricBatches("test drained", func() (int64, error) {
		calls++
		if calls < 3 {
			return model.ChannelMetricMaxDeleteBatch, nil
		}
		return model.ChannelMetricMaxDeleteBatch - 1, nil
	})

	require.False(t, needsCatchUp)
	require.Equal(t, 3, calls)
}

type channelMetricRuntimeNamedDialector struct {
	gorm.Dialector
	name string
}

func (dialector channelMetricRuntimeNamedDialector) Name() string {
	return dialector.name
}

func TestInitChannelMetricsWaitsForMasterMigrationWithoutFailing(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	previousDB := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() {
		ShutdownChannelMetrics()
		model.LOG_DB = previousDB
	})

	require.NoError(t, InitChannelMetrics())
	require.NotNil(t, channelMetricCollector())
	require.False(t, ChannelMetricsRuntimeAvailable())
	ShutdownChannelMetrics()
	require.Nil(t, channelMetricCollector())
}

func TestInitChannelMetricsDisablesUnsupportedClickHouse(t *testing.T) {
	db, err := gorm.Open(channelMetricRuntimeNamedDialector{
		Dialector: sqlite.Open(":memory:"),
		name:      "clickhouse",
	}, &gorm.Config{})
	require.NoError(t, err)
	previousDB := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() {
		ShutdownChannelMetrics()
		model.LOG_DB = previousDB
	})

	require.NoError(t, InitChannelMetrics())
	require.Nil(t, channelMetricCollector())
	require.False(t, ChannelMetricsRuntimeAvailable())
}

func TestShutdownChannelMetricsFlushesPendingFacts(t *testing.T) {
	primary, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	logDB, err := gorm.Open(sqlite.Open("file:channel-metric-runtime-shutdown?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := logDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, model.MigrateChannelAnalyticsLogDB(logDB))
	previousPrimary, previousLog := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = primary, logDB
	t.Cleanup(func() {
		ShutdownChannelMetrics()
		model.DB, model.LOG_DB = previousPrimary, previousLog
		_ = sqlDB.Close()
	})

	require.NoError(t, InitChannelMetrics())
	sample := channelmetrics.NewLiveSample(channelmetrics.ScopeFinalRequest, channelmetrics.OutcomeSuccess)
	sample.OccurredAt = time.Now()
	sample.RequestID = "shutdown-flush"
	sample.ClientStatus = channelmetrics.PresentStatus(200)
	recordChannelMetric(sample)
	ShutdownChannelMetrics()

	var count int64
	require.NoError(t, logDB.Model(&model.ChannelMetricBucket{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.False(t, primary.Migrator().HasTable(&model.ChannelMetricBucket{}))
	require.Nil(t, channelMetricCollector())
}
