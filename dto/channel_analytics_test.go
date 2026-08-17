package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func channelAnalyticsContractKeys(t *testing.T, value any) []string {
	t.Helper()
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	decoded := map[string]any{}
	require.NoError(t, common.Unmarshal(encoded, &decoded))
	keys := make([]string, 0, len(decoded))
	for key := range decoded {
		keys = append(keys, key)
	}
	return keys
}

func TestChannelAnalyticsFrontendResponseSchemas(t *testing.T) {
	rate := 0.5
	milliseconds := int64(125)
	meta := ChannelAnalyticsMeta{Backfill: &ChannelAnalyticsBackfillMeta{LastError: "masked"}}
	channel := ChannelAnalyticsChannelItem{Group: "default", TopStatusCodes: []ChannelAnalyticsStatusCode{}}
	model := ChannelAnalyticsModelItem{
		ChannelAnalyticsChannelItem: channel,
		RequestedModel:              "requested",
		UpstreamModel:               "upstream",
		ModelHash:                   "hash",
	}
	stabilityItem := ChannelAnalyticsStabilityItem{
		Key: "key", Group: "default", GroupName: "Default", ChannelId: 1,
		ChannelName: "channel", ChannelType: 1, ChannelTypeName: "OpenAI",
		RequestedModel: "requested", UpstreamModel: "upstream", ModelHash: "hash",
		Windows: []ChannelAnalyticsStabilityWindow{},
	}

	tests := []struct {
		name  string
		value any
		keys  []string
	}{
		{name: "summary response", value: ChannelAnalyticsSummaryResponse{}, keys: []string{"summary", "meta"}},
		{name: "trend response", value: ChannelAnalyticsTrendResponse{}, keys: []string{"points", "meta"}},
		{name: "channels response", value: ChannelAnalyticsChannelsResponse{}, keys: []string{"items", "total", "page", "page_size", "meta"}},
		{name: "models response", value: ChannelAnalyticsModelsResponse{}, keys: []string{"items", "total", "page", "page_size", "meta"}},
		{name: "stability response", value: ChannelAnalyticsStabilityResponse{}, keys: []string{"dimension", "items", "total", "page", "page_size", "meta"}},
		{name: "status response", value: ChannelAnalyticsStatusResponse{}, keys: []string{"items", "error_stages", "meta"}},
		{name: "failures response", value: ChannelAnalyticsFailuresResponse{}, keys: []string{"items", "total", "page", "page_size", "meta"}},
		{name: "filters response", value: ChannelAnalyticsFiltersResponse{}, keys: []string{"channels", "channel_types", "groups", "requested_models", "upstream_models", "requested_model_options", "upstream_model_options", "outcomes", "traffic_sources", "data_origins", "meta"}},
		{name: "filter models response", value: ChannelAnalyticsFilterModelsResponse{}, keys: []string{"model_dimension", "query", "items", "total", "page", "page_size"}},
		{name: "summary", value: ChannelAnalyticsSummary{ClientSuccessRate: &rate, ChannelQualitySuccessRate: &rate, AttemptSuccessRate: &rate, RetryRate: &rate, CacheRequestHitRate: &rate, CacheTokenHitRate: &rate, AvgLatencyMs: &rate, P95LatencyMs: &milliseconds, AvgTtftMs: &rate, P95TtftMs: &milliseconds}, keys: []string{"final_request_count", "channel_attempt_count", "upstream_call_count", "failed_attempt_count", "retry_count", "client_success_rate", "channel_quality_success_rate", "attempt_success_rate", "retry_rate", "usage_sample_count", "input_tokens_total", "uncached_input_tokens", "output_tokens", "total_tokens", "cache_read_tokens", "cache_write_tokens", "cache_request_hit_rate", "cache_token_hit_rate", "charged_quota", "charged_micro_usd", "avg_latency_ms", "p95_latency_ms", "avg_ttft_ms", "p95_ttft_ms"}},
		{name: "trend point", value: ChannelAnalyticsTrendPoint{EventCount: 1, SuccessCount: 1, AvgLatencyMs: &rate}, keys: []string{"bucket_ts", "final_request_count", "channel_attempt_count", "upstream_call_count", "failed_attempt_count", "total_tokens", "event_count", "success_count", "avg_latency_ms"}},
		{name: "status code", value: ChannelAnalyticsStatusCode{}, keys: []string{"status_present", "status_code", "label", "count"}},
		{name: "error stage", value: ChannelAnalyticsErrorStage{}, keys: []string{"error_stage", "count"}},
		{name: "channel item", value: channel, keys: []string{"channel_id", "channel_name", "channel_type", "channel_type_name", "group", "channel_attempt_count", "failure_count", "retry_count", "channel_quality_success_rate", "attempt_success_rate", "usage_sample_count", "input_tokens_total", "uncached_input_tokens", "output_tokens", "cache_read_tokens", "cache_write_tokens", "cache_request_hit_rate", "cache_token_hit_rate", "avg_latency_ms", "p95_latency_ms", "avg_ttft_ms", "p95_ttft_ms", "charged_quota", "charged_micro_usd", "last_failure_at", "top_status_codes"}},
		{name: "model item", value: model, keys: []string{"channel_id", "channel_name", "channel_type", "channel_type_name", "group", "channel_attempt_count", "failure_count", "retry_count", "channel_quality_success_rate", "attempt_success_rate", "usage_sample_count", "input_tokens_total", "uncached_input_tokens", "output_tokens", "cache_read_tokens", "cache_write_tokens", "cache_request_hit_rate", "cache_token_hit_rate", "avg_latency_ms", "p95_latency_ms", "avg_ttft_ms", "p95_ttft_ms", "charged_quota", "charged_micro_usd", "last_failure_at", "top_status_codes", "requested_model", "upstream_model", "model_hash"}},
		{name: "stability item", value: stabilityItem, keys: []string{"key", "group", "group_name", "channel_id", "channel_name", "channel_type", "channel_type_name", "requested_model", "upstream_model", "model_hash", "windows"}},
		{name: "failure item", value: ChannelAnalyticsFailureItem{}, keys: []string{"event_id", "created_at", "request_id", "attempt_seq", "retry_planned", "is_last_started_attempt", "channel_id", "channel_name", "channel_type", "requested_model", "requested_model_hash", "upstream_model", "upstream_model_hash", "group", "traffic_source", "data_origin", "outcome", "failure_owner", "quality_eligible", "partial_response", "error_stage", "stream_end_reason", "upstream_status_present", "upstream_status_code", "normalized_status_present", "normalized_status_code", "client_status_present", "client_status_code", "latency_ms", "ttft_present", "ttft_ms", "retry_reason", "error_summary"}},
		{name: "meta", value: meta, keys: []string{"generated_at", "reliable_from_ts", "data_start_ts", "data_end_ts", "last_flushed_at", "runtime_pending_batch_count", "runtime_flush_failure_count", "runtime_last_flush_error_at", "bucket_level", "bucket_seconds", "retention_days", "partial", "detail_available", "uncovered_channel_types", "invalid_sample_count", "dimension_overflow_count", "dropped_metric_event_count", "dropped_failure_event_count", "dimension_hash_collision_count", "backfill"}},
		{name: "filter model", value: ChannelAnalyticsFilterModel{}, keys: []string{"value", "label", "model", "model_hash"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.ElementsMatch(t, test.keys, channelAnalyticsContractKeys(t, test.value))
		})
	}
}

func TestChannelAnalyticsStabilityWindowFrontendSchema(t *testing.T) {
	assert.ElementsMatch(t, []string{
		"window_seconds", "start_timestamp", "end_timestamp", "channel_attempt_count", "failure_count",
		"quality_eligible_count", "quality_success_count", "quality_success_rate", "attempt_eligible_count",
		"attempt_success_count", "attempt_success_rate", "retry_count", "retry_rate", "partial_response_count",
		"upstream_call_count", "upstream_status_sample_count", "upstream_status_coverage_rate", "upstream_429_count",
		"upstream_4xx_count", "upstream_5xx_count", "http_error_count", "transport_error_count",
		"protocol_error_count", "stream_error_count", "local_error_count", "dispatch_error_count",
		"client_cancelled_count", "live_event_count", "legacy_event_count", "live_event_rate", "legacy_event_rate",
		"minimum_sample_count", "sample_sufficient", "usage_sample_count", "usage_success_coverage_rate",
		"input_tokens_total", "uncached_input_tokens", "output_tokens", "total_tokens", "cache_read_tokens",
		"cache_write_tokens", "cache_request_hit_rate", "cache_token_hit_rate", "latency_sample_count",
		"latency_coverage_rate", "avg_latency_ms", "p95_latency_ms", "ttft_sample_count", "ttft_coverage_rate",
		"avg_ttft_ms", "p95_ttft_ms", "charged_quota", "charged_micro_usd", "last_failure_bucket_ts",
	}, channelAnalyticsContractKeys(t, ChannelAnalyticsStabilityWindow{}))
}
