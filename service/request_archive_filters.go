package service

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const requestArchiveMaximumEventFilterValues = 1024

type requestArchiveEventFilters struct {
	ChannelIds []int
	GroupCodes []string
	Sources    []string
}

func normalizeRequestArchiveEventChannelIds(values []int) ([]int, error) {
	if len(values) > requestArchiveMaximumEventFilterValues {
		return nil, fmt.Errorf("请求归档渠道筛选数量不能超过 %d 个", requestArchiveMaximumEventFilterValues)
	}
	result := make([]int, 0, len(values))
	seen := make(map[int]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			return nil, errors.New("请求归档渠道筛选只能包含正整数 ID")
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Ints(result)
	return result, nil
}

func normalizeRequestArchiveEventGroupCodes(values []string) ([]string, error) {
	if len(values) > requestArchiveMaximumEventFilterValues {
		return nil, fmt.Errorf("请求归档分组筛选数量不能超过 %d 个", requestArchiveMaximumEventFilterValues)
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		normalized, err := model.NormalizeGroupCode(value)
		if err != nil {
			return nil, fmt.Errorf("请求归档分组筛选编码无效：%w", err)
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	sort.Strings(result)
	return result, nil
}

func normalizeRequestArchiveEventSources(values []string) ([]string, error) {
	if len(values) > requestArchiveMaximumEventFilterValues {
		return nil, fmt.Errorf("请求归档审计来源筛选数量不能超过 %d 个", requestArchiveMaximumEventFilterValues)
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		source := strings.ToLower(strings.TrimSpace(value))
		if source == "" {
			continue
		}
		switch source {
		case PromptAuditSourceGuard, PromptAuditSourceSensitiveWord, PromptAuditSourceUpstreamPolicy:
		default:
			return nil, errors.New("请求归档审计来源筛选无效")
		}
		if _, exists := seen[source]; exists {
			continue
		}
		seen[source] = struct{}{}
		result = append(result, source)
	}
	sort.Strings(result)
	return result, nil
}

func requestArchiveEventFiltersFromModel(config *model.RequestArchiveConfig) (requestArchiveEventFilters, error) {
	filters := requestArchiveEventFilters{
		ChannelIds: make([]int, 0),
		GroupCodes: make([]string, 0),
		Sources:    make([]string, 0),
	}
	if config == nil {
		return filters, nil
	}
	if err := unmarshalRequestArchiveFilter(config.EventChannelIds, &filters.ChannelIds, "渠道"); err != nil {
		return requestArchiveEventFilters{}, err
	}
	if err := unmarshalRequestArchiveFilter(config.EventGroupCodes, &filters.GroupCodes, "分组"); err != nil {
		return requestArchiveEventFilters{}, err
	}
	if err := unmarshalRequestArchiveFilter(config.EventSources, &filters.Sources, "审计来源"); err != nil {
		return requestArchiveEventFilters{}, err
	}
	var err error
	filters.ChannelIds, err = normalizeRequestArchiveEventChannelIds(filters.ChannelIds)
	if err != nil {
		return requestArchiveEventFilters{}, err
	}
	filters.GroupCodes, err = normalizeRequestArchiveEventGroupCodes(filters.GroupCodes)
	if err != nil {
		return requestArchiveEventFilters{}, err
	}
	filters.Sources, err = normalizeRequestArchiveEventSources(filters.Sources)
	if err != nil {
		return requestArchiveEventFilters{}, err
	}
	return filters, nil
}

func unmarshalRequestArchiveFilter(value string, target interface{}, label string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	if err := common.UnmarshalJsonStr(value, target); err != nil {
		return fmt.Errorf("请求归档%s筛选配置无效", label)
	}
	return nil
}

func marshalRequestArchiveFilter(value interface{}, label string) (string, error) {
	payload, err := common.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("请求归档%s筛选无法序列化", label)
	}
	return string(payload), nil
}

func requestArchiveEventMatchesFilters(filters requestArchiveEventFilters, event *model.PromptAuditEvent) bool {
	if event == nil {
		return false
	}
	if len(filters.ChannelIds) > 0 {
		index := sort.SearchInts(filters.ChannelIds, event.ChannelId)
		if event.ChannelId <= 0 || index >= len(filters.ChannelIds) || filters.ChannelIds[index] != event.ChannelId {
			return false
		}
	}
	if len(filters.GroupCodes) > 0 {
		groupCode := strings.TrimSpace(event.GroupCode)
		index := sort.SearchStrings(filters.GroupCodes, groupCode)
		if groupCode == "" || index >= len(filters.GroupCodes) || filters.GroupCodes[index] != groupCode {
			return false
		}
	}
	if len(filters.Sources) > 0 {
		source := strings.ToLower(strings.TrimSpace(event.Source))
		index := sort.SearchStrings(filters.Sources, source)
		if source == "" || index >= len(filters.Sources) || filters.Sources[index] != source {
			return false
		}
	}
	return true
}

func cloneRequestArchiveEventFilters(source requestArchiveEventFilters) requestArchiveEventFilters {
	return requestArchiveEventFilters{
		ChannelIds: append([]int(nil), source.ChannelIds...),
		GroupCodes: append([]string(nil), source.GroupCodes...),
		Sources:    append([]string(nil), source.Sources...),
	}
}
