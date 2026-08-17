package service

import (
	"errors"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"strings"
)

type RetryParam struct {
	Ctx                     *gin.Context
	TokenGroup              string
	ModelName               string
	RequestPath             string
	Retry                   *int
	ExcludedChannelIDs      map[int]struct{}
	RetryFallbackChannelIDs map[int]struct{}
}

func (p *RetryParam) GetRetry() int {
	if p == nil || p.Retry == nil {
		return 0
	}
	return *p.Retry
}
func (p *RetryParam) SetRetry(retry int) {
	if p != nil {
		p.Retry = &retry
	}
}
func (p *RetryParam) IncreaseRetry() {
	if p == nil {
		return
	}
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

// ExcludeChannelID records an attempted channel. Fallback is only allowed for
// a regular single-group request after all untried candidates are exhausted.
func (p *RetryParam) ExcludeChannelID(channelID int, allowFallback bool) {
	if p == nil || channelID <= 0 {
		return
	}
	if p.ExcludedChannelIDs == nil {
		p.ExcludedChannelIDs = make(map[int]struct{})
	}
	p.ExcludedChannelIDs[channelID] = struct{}{}
	if allowFallback {
		if p.RetryFallbackChannelIDs == nil {
			p.RetryFallbackChannelIDs = make(map[int]struct{})
		}
		p.RetryFallbackChannelIDs[channelID] = struct{}{}
	}
}

// ParseTokenGroupList parses an explicitly authorized ordered token group list.
// It rejects empty entries, duplicates, and mixing auto with named groups.
func ParseTokenGroupList(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "auto" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	groups := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		group := strings.TrimSpace(part)
		if group == "" {
			return nil, fmt.Errorf("token group list contains an empty group")
		}
		if group == "auto" {
			return nil, fmt.Errorf("token group list cannot mix auto with named groups")
		}
		if _, ok := seen[group]; ok {
			return nil, fmt.Errorf("token group list contains duplicate group %q", group)
		}
		seen[group] = struct{}{}
		groups = append(groups, group)
	}
	return groups, nil
}

func GetRequestTokenGroups(ctx *gin.Context, tokenGroup string) []string {
	requested := strings.TrimSpace(tokenGroup)
	if ctx == nil || requested == "" || requested == "auto" {
		return nil
	}
	value, ok := common.GetContextKey(ctx, constant.ContextKeyTokenGroups)
	groups, ok := value.([]string)
	if !ok || len(groups) == 0 {
		return nil
	}
	original := strings.TrimSpace(common.GetContextKeyString(ctx, constant.ContextKeyTokenGroup))
	if original == "" || requested == original {
		return append([]string(nil), groups...)
	}
	for _, group := range groups {
		if group == requested {
			return []string{requested}
		}
	}
	return nil
}

func retryGroupIndex(ctx *gin.Context) int {
	if ctx == nil {
		return 0
	}
	if value, ok := common.GetContextKey(ctx, constant.ContextKeyAutoGroupIndex); ok {
		if index, ok := value.(int); ok && index >= 0 {
			return index
		}
	}
	return 0
}
func retryGroupStartIndex(ctx *gin.Context) int {
	if ctx == nil {
		return 0
	}
	if value, ok := common.GetContextKey(ctx, constant.ContextKeyAutoGroupRetryIndex); ok {
		if index, ok := value.(int); ok && index >= 0 {
			return index
		}
	}
	return 0
}

// RelayMaxRetries preserves the site-wide retry budget. Ordered group
// selection consumes that global budget while tracking its own group-local
// priority index.
func RelayMaxRetries(_ *RetryParam) int {
	return common.RetryTimes
}

func retryFallbackExclusions(param *RetryParam) map[int]struct{} {
	if param == nil || param.GetRetry() <= 0 || len(param.RetryFallbackChannelIDs) == 0 ||
		param.TokenGroup == "auto" || len(GetRequestTokenGroups(param.Ctx, param.TokenGroup)) > 1 {
		return nil
	}
	ids := make(map[int]struct{}, len(param.ExcludedChannelIDs))
	for id := range param.ExcludedChannelIDs {
		if _, relax := param.RetryFallbackChannelIDs[id]; !relax {
			ids[id] = struct{}{}
		}
	}
	return ids
}

func selectFromGroup(param *RetryParam, group string, retry int) (*model.Channel, error) {
	exclusions := model.ChannelSelectionExclusions{ChannelIDs: param.ExcludedChannelIDs}
	channel, err := model.GetRandomSatisfiedChannelWithSelectionExclusions(group, param.ModelName, retry, param.RequestPath, exclusions)
	if err != nil || channel != nil {
		return channel, err
	}
	if fallback := retryFallbackExclusions(param); fallback != nil {
		return model.GetRandomSatisfiedChannelWithSelectionExclusions(group, param.ModelName, retry, param.RequestPath, model.ChannelSelectionExclusions{ChannelIDs: fallback})
	}
	return nil, nil
}
func selectFromOrderedGroups(param *RetryParam, groups []string) (*model.Channel, string, error) {
	selectGroup := param.TokenGroup
	startGroup := retryGroupIndex(param.Ctx)
	globalRetry := param.GetRetry()
	groupStartRetry := retryGroupStartIndex(param.Ctx)
	if groupStartRetry > globalRetry {
		groupStartRetry = globalRetry
	}

	for i := startGroup; i < len(groups); i++ {
		if i > startGroup {
			groupStartRetry = globalRetry
		}
		priorityRetry := globalRetry - groupStartRetry
		channel, err := selectFromGroup(param, groups[i], priorityRetry)
		if err != nil {
			return nil, groups[i], err
		}
		if channel == nil {
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, globalRetry)
			continue
		}

		selectGroup = groups[i]
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, selectGroup)
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
		common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, groupStartRetry)
		return channel, selectGroup, nil
	}
	return nil, selectGroup, nil
}

// CacheGetRandomSatisfiedChannel selects from the current ordered group until
// its priority tiers are exhausted, then advances to the next authorized group.
// It updates actual selected-group context without changing the original
// TokenGroup.
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	if param == nil {
		return nil, "", errors.New("retry parameter is nil")
	}
	if param.TokenGroup == "auto" {
		userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)
		groups := GetRequestAutoGroups(param.Ctx, userGroup)
		if len(groups) == 0 {
			return nil, param.TokenGroup, errors.New("auto groups is not enabled")
		}
		return selectFromOrderedGroups(param, groups)
	}
	if groups := GetRequestTokenGroups(param.Ctx, param.TokenGroup); len(groups) > 1 {
		return selectFromOrderedGroups(param, groups)
	}
	channel, err := selectFromGroup(param, param.TokenGroup, param.GetRetry())
	if err != nil {
		return nil, param.TokenGroup, err
	}
	return channel, param.TokenGroup, nil
}
