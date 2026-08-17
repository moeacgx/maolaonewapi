package model

import (
	"os"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type channelMetricNamedDialector struct {
	gorm.Dialector
	name string
}

func (dialector channelMetricNamedDialector) Name() string {
	return dialector.name
}

func TestChannelMetricPortableDialectsBuildBoundQueries(t *testing.T) {
	tests := []struct {
		name      string
		dialector gorm.Dialector
	}{
		{name: "sqlite", dialector: sqlite.Open(":memory:")},
		{name: "mysql", dialector: mysql.New(mysql.Config{DSN: "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local", SkipInitializeWithVersion: true})},
		{name: "postgres", dialector: postgres.New(postgres.Config{DSN: "host=localhost user=gorm password=gorm dbname=gorm port=9920 sslmode=disable", PreferSimpleProtocol: true})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, err := gorm.Open(test.dialector, &gorm.Config{DryRun: true, DisableAutomaticPing: true})
			require.NoError(t, err)
			assert.True(t, ChannelAnalyticsLogDBSupported(db))

			query, err := applyChannelMetricBucketFilter(db.Model(&ChannelMetricBucket{}), ChannelMetricBucketFilter{
				BucketLevel: "5m", StartTs: 300, EndTs: 600,
				ChannelIds: []int{7}, Groups: []string{"default"},
				UpstreamStatusPresent: boolPointerForChannelMetricDialectTest(true),
				UpstreamStatusCodes:   []int{429},
			})
			require.NoError(t, err)
			var rows []ChannelMetricBucket
			statement := query.Find(&rows).Statement
			assert.NotEmpty(t, statement.SQL.String())
			assert.Len(t, statement.Vars, 7)
		})
	}
}

func TestChannelMetricClickHouseMigrationDegradesWithoutMutation(t *testing.T) {
	db, err := gorm.Open(channelMetricNamedDialector{
		Dialector: sqlite.Open(":memory:"),
		name:      "clickhouse",
	}, &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE logs (id INTEGER PRIMARY KEY, content TEXT)").Error)
	require.NoError(t, db.Exec("INSERT INTO logs (id, content) VALUES (?, ?)", 1, "retained").Error)

	assert.False(t, ChannelAnalyticsLogDBSupported(db))
	require.NoError(t, MigrateChannelAnalyticsLogDB(db))
	assert.False(t, db.Migrator().HasTable(&ChannelMetricBucket{}))
	assert.False(t, ChannelAnalyticsLogDBReady(db))
	var logCount int64
	require.NoError(t, db.Table("logs").Count(&logCount).Error)
	assert.EqualValues(t, 1, logCount)
}

func TestChannelMetricMigrationOnExternalSQLDialects(t *testing.T) {
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
			require.NoError(t, MigrateChannelAnalyticsLogDB(db))
			assert.True(t, db.Migrator().HasTable(&ChannelMetricBucket{}))
			assert.True(t, db.Migrator().HasTable(&ChannelFailureEvent{}))
			assert.True(t, db.Migrator().HasTable(&ChannelMetricFlush{}))
			assert.True(t, db.Migrator().HasTable(&ChannelMetricBackfillJob{}))
		})
	}
}

func boolPointerForChannelMetricDialectTest(value bool) *bool {
	return &value
}
