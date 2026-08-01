package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestNormalizeRequestArchiveEventFilters(t *testing.T) {
	channelIds, err := normalizeRequestArchiveEventChannelIds([]int{31, 7, 31})
	require.NoError(t, err)
	require.Equal(t, []int{7, 31}, channelIds)
	_, err = normalizeRequestArchiveEventChannelIds([]int{0})
	require.EqualError(t, err, "请求归档渠道筛选只能包含正整数 ID")

	groupCodes, err := normalizeRequestArchiveEventGroupCodes([]string{" vip ", "default", "vip"})
	require.NoError(t, err)
	require.Equal(t, []string{"default", "vip"}, groupCodes)
	_, err = normalizeRequestArchiveEventGroupCodes([]string{"auto"})
	require.Error(t, err)

	sources, err := normalizeRequestArchiveEventSources([]string{
		PromptAuditSourceUpstreamPolicy,
		" PROMPT_GUARD ",
		PromptAuditSourceUpstreamPolicy,
	})
	require.NoError(t, err)
	require.Equal(t, []string{PromptAuditSourceGuard, PromptAuditSourceUpstreamPolicy}, sources)
	_, err = normalizeRequestArchiveEventSources([]string{"unknown"})
	require.EqualError(t, err, "请求归档审计来源筛选无效")
}

func TestRequestArchiveEventFiltersFromModel(t *testing.T) {
	filters, err := requestArchiveEventFiltersFromModel(&model.RequestArchiveConfig{
		EventChannelIds: "[9,2,9]",
		EventGroupCodes: `["vip","default","vip"]`,
		EventSources:    `["upstream_policy","sensitive_word"]`,
	})
	require.NoError(t, err)
	require.Equal(t, []int{2, 9}, filters.ChannelIds)
	require.Equal(t, []string{"default", "vip"}, filters.GroupCodes)
	require.Equal(t, []string{PromptAuditSourceSensitiveWord, PromptAuditSourceUpstreamPolicy}, filters.Sources)

	empty, err := requestArchiveEventFiltersFromModel(&model.RequestArchiveConfig{})
	require.NoError(t, err)
	require.Empty(t, empty.ChannelIds)
	require.Empty(t, empty.GroupCodes)
	require.Empty(t, empty.Sources)

	_, err = requestArchiveEventFiltersFromModel(&model.RequestArchiveConfig{EventChannelIds: "{"})
	require.EqualError(t, err, "请求归档渠道筛选配置无效")
}

func TestRequestArchiveEventMatchesFilters(t *testing.T) {
	event := &model.PromptAuditEvent{
		ChannelId: 31,
		GroupCode: "vip",
		Source:    PromptAuditSourceUpstreamPolicy,
	}
	require.True(t, requestArchiveEventMatchesFilters(requestArchiveEventFilters{}, event))
	require.True(t, requestArchiveEventMatchesFilters(requestArchiveEventFilters{
		ChannelIds: []int{7, 31},
		GroupCodes: []string{"default", "vip"},
		Sources:    []string{PromptAuditSourceUpstreamPolicy},
	}, event))
	require.False(t, requestArchiveEventMatchesFilters(requestArchiveEventFilters{
		ChannelIds: []int{7},
	}, event))
	require.False(t, requestArchiveEventMatchesFilters(requestArchiveEventFilters{
		GroupCodes: []string{"default"},
	}, event))
	require.False(t, requestArchiveEventMatchesFilters(requestArchiveEventFilters{
		Sources: []string{PromptAuditSourceSensitiveWord},
	}, event))
	require.False(t, requestArchiveEventMatchesFilters(requestArchiveEventFilters{
		ChannelIds: []int{31},
	}, &model.PromptAuditEvent{Source: PromptAuditSourceGuard}))
	require.False(t, requestArchiveEventMatchesFilters(requestArchiveEventFilters{}, nil))
}
