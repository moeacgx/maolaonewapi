package perfmetrics

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRecordRelayFailureExcludesContentPolicyAndConfiguredRules(t *testing.T) {
	info := &relaycommon.RelayInfo{OriginModelName: "filter-test", UsingGroup: "default"}
	require.False(t, shouldRecordRelayFailure(info, types.NewError(errors.New("blocked"), hosttypes.ErrorCodeCyberPolicy)))

	relayErr := types.NewErrorWithStatusCode(errors.New("connection reset by peer"), types.ErrorCodeDoRequestFailed, 502)
	rules := []perf_metrics_setting.FailureFilterRule{{ID: "network", Name: "network", Enabled: true, Field: "message", Mode: "contains", Values: []string{"not present", "connection reset"}}}
	registeredSetting := config.GlobalConfig.Get("perf_metrics_setting")
	require.NotNil(t, registeredSetting)
	originalRules, err := json.Marshal(perf_metrics_setting.GetSetting().FailureFilterRules)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(registeredSetting, map[string]string{"failure_filter_rules": string(originalRules)}))
	})
	configuredRules, err := json.Marshal(rules)
	require.NoError(t, err)
	require.NoError(t, config.UpdateConfigFromMap(registeredSetting, map[string]string{"failure_filter_rules": string(configuredRules)}))
	require.True(t, matchesFailureFilterRule(relayErr, rules))
	require.False(t, shouldRecordRelayFailure(info, relayErr), "configured failure must be isolated from performance samples")
	require.True(t, shouldRecordRelayFailure(info, types.NewErrorWithStatusCode(errors.New("different failure"), types.ErrorCodeDoRequestFailed, 502)))
}

func TestMatchesFailureFilterRuleSupportsFieldsModesAndInvalidRegex(t *testing.T) {
	relayErr := types.NewErrorWithStatusCode(errors.New("upstream policy copy"), types.ErrorCodeBadResponseStatusCode, 400)
	tests := []perf_metrics_setting.FailureFilterRule{
		{Enabled: true, Field: " status_code ", Mode: " exact ", Value: "400"},
		{Enabled: true, Field: "error_code", Mode: "contains", Value: "bad_response"},
		{Enabled: true, Field: "message", Mode: "contains", Value: "policy copy"},
		{Enabled: true, Field: "full_error", Mode: "regex", Value: `status_code=400, .*policy`},
	}
	for _, rule := range tests {
		require.True(t, matchesFailureFilterRule(relayErr, []perf_metrics_setting.FailureFilterRule{rule}))
	}
	require.False(t, matchesFailureFilterRule(relayErr, []perf_metrics_setting.FailureFilterRule{{Enabled: true, Field: "message", Mode: "regex", Value: "["}}))
}

func TestFailureFilterRegexCacheStoresValidAndInvalidPatterns(t *testing.T) {
	failureFilterRegexCache.Lock()
	failureFilterRegexCache.entries = make(map[string]failureFilterRegexCacheEntry)
	failureFilterRegexCache.Unlock()
	first, valid := getFailureFilterRegex(`policy_\d+`)
	require.True(t, valid)
	second, valid := getFailureFilterRegex(`policy_\d+`)
	require.True(t, valid)
	require.Same(t, first, second)
	invalid, valid := getFailureFilterRegex("[")
	require.False(t, valid)
	require.Nil(t, invalid)
}

func TestBuildModelSummariesStatusRateUsesBestCanonicalGroup(t *testing.T) {
	models := buildModelSummariesWithGroupStatus(
		map[string]map[int64]counters{
			"gpt-status": {
				100: {requestCount: 13, successCount: 8, totalLatencyMs: 1300},
				200: {requestCount: 12, successCount: 10, totalLatencyMs: 1200},
			},
		},
		map[string]map[int64]map[string]counters{
			"gpt-status": {
				100: {
					"default": {requestCount: 10, successCount: 5},
					"vip":     {requestCount: 3, successCount: 3},
				},
				200: {
					"default": {requestCount: 2, successCount: 1},
					"vip":     {requestCount: 10, successCount: 9},
				},
			},
		},
	)

	require.Len(t, models, 1)
	require.Equal(t, "gpt-status", models[0].ModelName)
	require.Equal(t, 72.0, models[0].SuccessRate)
	require.NotNil(t, models[0].StatusRate)
	require.Equal(t, 92.31, *models[0].StatusRate)
	require.Len(t, models[0].Series, 2)
	require.NotNil(t, models[0].Series[0].StatusRate)
	require.Equal(t, 100.0, *models[0].Series[0].StatusRate)
	require.NotNil(t, models[0].Series[1].StatusRate)
	require.Equal(t, 90.0, *models[0].Series[1].StatusRate)
	require.Equal(t, []float64{61.54, 83.33}, models[0].RecentSuccessRates)
}

func openPerfMetricsAliasTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	model.InitDBColumns()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		model.InitDBColumns()
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestQueryAggregatesHistoricalGroupAlias(t *testing.T) {
	db := openPerfMetricsAliasTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Group{}, &model.GroupAlias{}, &model.PerfMetric{}))
	group := model.Group{Code: "2", Name: "特价", Ratio: 1, Status: model.GroupStatusActive}
	require.NoError(t, db.Create(&group).Error)
	require.NoError(t, db.Create(&model.GroupAlias{Alias: "group_2", GroupId: group.Id}).Error)
	bucketTs := time.Now().Unix() - 10
	require.NoError(t, db.Create(&[]model.PerfMetric{
		{ModelName: "gpt-alias-query", Group: "group_2", BucketTs: bucketTs, RequestCount: 1, SuccessCount: 0, TotalLatencyMs: 100},
		{ModelName: "gpt-alias-query", Group: group.Code, BucketTs: bucketTs, RequestCount: 3, SuccessCount: 3, TotalLatencyMs: 300},
	}).Error)

	result, err := Query(QueryParams{Model: "gpt-alias-query", Group: group.Code, Hours: 1})
	require.NoError(t, err)
	require.Len(t, result.Groups, 1)
	require.Equal(t, group.Code, result.Groups[0].Group)
	require.Equal(t, 75.0, result.Groups[0].SuccessRate)
	require.Equal(t, int64(100), result.Groups[0].AvgLatencyMs)
}
