package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
)

func normalizeChannelTag(tag *string) string {
	if tag == nil {
		return ""
	}
	return strings.TrimSpace(*tag)
}

func channelHasAnyTag(channel *Channel, tags map[string]struct{}) bool {
	if channel == nil || len(tags) == 0 {
		return false
	}
	tag := normalizeChannelTag(channel.Tag)
	if tag == "" {
		return false
	}
	_, ok := tags[tag]
	return ok
}

func normalizedChannelTagSet(tags []string) map[string]struct{} {
	result := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			result[tag] = struct{}{}
		}
	}
	return result
}

// GetChannelTag 返回渠道管理用于聚合渠道的 Tag，而不是用户可访问分组。
func GetChannelTag(channelID int) (string, error) {
	if channelID <= 0 {
		return "", nil
	}
	if common.MemoryCacheEnabled {
		channelSyncLock.RLock()
		defer channelSyncLock.RUnlock()
		if channelsIDM == nil {
			return "", errors.New("渠道缓存尚未初始化")
		}
		channel := channelsIDM[channelID]
		if channel == nil {
			return "", fmt.Errorf("渠道 %d 不存在", channelID)
		}
		return normalizeChannelTag(channel.Tag), nil
	}
	channel, err := GetChannelById(channelID, false)
	if err != nil {
		return "", err
	}
	return normalizeChannelTag(channel.Tag), nil
}

// GetChannelStatusAndTag 只读取固定渠道预检需要的状态和管理标签。
// 不存在是可确认的业务状态；数据库故障则单独返回错误，由调用方 fail-safe 处理。
func GetChannelStatusAndTag(channelID int) (status int, tag string, exists bool, err error) {
	if channelID <= 0 {
		return 0, "", false, nil
	}
	if DB == nil {
		return 0, "", false, errors.New("database is nil")
	}
	var channel struct {
		Status int
		Tag    *string
	}
	err = DB.Model(&Channel{}).
		Select([]string{"status", "tag"}).
		Where("id = ?", channelID).
		Take(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, err
	}
	return channel.Status, normalizeChannelTag(channel.Tag), true, nil
}

// ChannelBelongsToAnyTag 按渠道管理标签判断，不读取用户或令牌分组。
func ChannelBelongsToAnyTag(channelID int, tags []string) (bool, error) {
	targets := normalizedChannelTagSet(tags)
	if len(targets) == 0 {
		return false, nil
	}
	tag, err := GetChannelTag(channelID)
	if err != nil {
		return false, err
	}
	_, ok := targets[tag]
	return ok, nil
}

func sensitiveCandidateModelNames(modelName string) []string {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil
	}
	result := []string{modelName}
	if normalized := ratio_setting.FormatMatchingModelName(modelName); normalized != "" && normalized != modelName {
		result = append(result, normalized)
	}
	return result
}

// AnySpecificChannelIsCandidate 判断指定渠道是否位于当前路由分组和模型的候选集中。
func AnySpecificChannelIsCandidate(routeGroups []string, modelName string, channelIDs []int) (bool, error) {
	models := sensitiveCandidateModelNames(modelName)
	if len(routeGroups) == 0 || len(models) == 0 || len(channelIDs) == 0 {
		return false, nil
	}
	targets := make(map[int]struct{}, len(channelIDs))
	for _, channelID := range channelIDs {
		if channelID > 0 {
			targets[channelID] = struct{}{}
		}
	}
	if len(targets) == 0 {
		return false, nil
	}
	if common.MemoryCacheEnabled {
		channelSyncLock.RLock()
		defer channelSyncLock.RUnlock()
		if group2model2channels == nil || channelsIDM == nil {
			return false, errors.New("渠道缓存尚未初始化")
		}
		for _, routeGroup := range routeGroups {
			for _, candidateModel := range models {
				for _, channelID := range group2model2channels[routeGroup][candidateModel] {
					if _, ok := channelsIDM[channelID]; !ok {
						return false, fmt.Errorf("候选渠道 %d 不存在", channelID)
					}
					if _, ok := targets[channelID]; ok {
						return true, nil
					}
				}
			}
		}
		return false, nil
	}

	var count int64
	err := DB.Model(&Ability{}).
		Joins("JOIN channels ON channels.id = abilities.channel_id").
		Where("abilities."+commonGroupCol+" IN ?", routeGroups).
		Where("abilities.model IN ?", models).
		Where("abilities.channel_id IN ?", channelIDs).
		Where("abilities.enabled = ?", true).
		Where("channels.status = ?", common.ChannelStatusEnabled).
		Count(&count).Error
	return count > 0, err
}

// AnyCandidateChannelBelongsToTags 判断当前路由候选渠道中，是否有渠道属于目标管理标签。
// routeGroups 只用于计算候选渠道，绝不与标签或用户分组混用。
func AnyCandidateChannelBelongsToTags(routeGroups []string, modelName string, targetTags []string) (bool, error) {
	models := sensitiveCandidateModelNames(modelName)
	tags := normalizedChannelTagSet(targetTags)
	if len(routeGroups) == 0 || len(models) == 0 || len(tags) == 0 {
		return false, nil
	}
	if common.MemoryCacheEnabled {
		channelSyncLock.RLock()
		defer channelSyncLock.RUnlock()
		if group2model2channels == nil || channelsIDM == nil {
			return false, errors.New("渠道缓存尚未初始化")
		}
		seen := make(map[int]struct{})
		for _, routeGroup := range routeGroups {
			for _, candidateModel := range models {
				for _, channelID := range group2model2channels[routeGroup][candidateModel] {
					if _, ok := seen[channelID]; ok {
						continue
					}
					seen[channelID] = struct{}{}
					channel := channelsIDM[channelID]
					if channel == nil {
						return false, fmt.Errorf("候选渠道 %d 不存在", channelID)
					}
					if channel.Status == common.ChannelStatusEnabled && channelHasAnyTag(channel, tags) {
						return true, nil
					}
				}
			}
		}
		return false, nil
	}

	type candidateChannel struct {
		Id  int
		Tag *string
	}
	var candidates []candidateChannel
	err := DB.Model(&Ability{}).
		Select("DISTINCT channels.id, channels.tag").
		Joins("JOIN channels ON channels.id = abilities.channel_id").
		Where("abilities."+commonGroupCol+" IN ?", routeGroups).
		Where("abilities.model IN ?", models).
		Where("abilities.enabled = ?", true).
		Where("channels.status = ?", common.ChannelStatusEnabled).
		Scan(&candidates).Error
	if err != nil {
		return false, err
	}
	for _, candidate := range candidates {
		if channelHasAnyTag(&Channel{Tag: candidate.Tag}, tags) {
			return true, nil
		}
	}
	return false, nil
}

func IsChannelEnabledForGroupModel(group string, modelName string, channelID int) bool {
	if group == "" || modelName == "" || channelID <= 0 {
		return false
	}
	if !common.MemoryCacheEnabled {
		return isChannelEnabledForGroupModelDB(group, modelName, channelID)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	if group2model2channels == nil {
		return false
	}

	if isChannelIDInList(group2model2channels[group][modelName], channelID) {
		return true
	}
	normalized := ratio_setting.FormatMatchingModelName(modelName)
	if normalized != "" && normalized != modelName {
		return isChannelIDInList(group2model2channels[group][normalized], channelID)
	}
	return false
}

func IsChannelEnabledForAnyGroupModel(groups []string, modelName string, channelID int) bool {
	if len(groups) == 0 {
		return false
	}
	for _, g := range groups {
		if IsChannelEnabledForGroupModel(g, modelName, channelID) {
			return true
		}
	}
	return false
}

func isChannelEnabledForGroupModelDB(group string, modelName string, channelID int) bool {
	if DB == nil {
		return false
	}
	group = strings.TrimSpace(group)
	models := sensitiveCandidateModelNames(modelName)
	if group == "" || len(models) == 0 {
		return false
	}
	var abilities []Ability
	if err := DB.Where(commonGroupCol+" = ? AND model IN ? AND channel_id = ? AND enabled = ?", group, models, channelID, true).
		Find(&abilities).Error; err != nil {
		return false
	}
	hasExactAbility := false
	for _, ability := range abilities {
		if strings.TrimSpace(ability.Group) != group {
			continue
		}
		for _, candidateModel := range models {
			if ability.Model == candidateModel {
				hasExactAbility = true
				break
			}
		}
		if hasExactAbility {
			break
		}
	}
	if !hasExactAbility {
		return false
	}
	var channel Channel
	if err := DB.Select("id", "status", groupBindingGroupColumn()).First(&channel, "id = ?", channelID).Error; err != nil {
		return false
	}
	if channel.Status != common.ChannelStatusEnabled {
		return false
	}
	if err := HydrateChannelGroupBindings(DB, []*Channel{&channel}); err != nil {
		return false
	}
	return channelHasCurrentGroup(&channel, group)
}

func isChannelIDInList(list []int, channelID int) bool {
	for _, id := range list {
		if id == channelID {
			return true
		}
	}
	return false
}

// GroupHasModelChannels 判断分组是否有给定模型的可用渠道。
// 用于多分组令牌路由时过滤出有目标模型的候选分组。
func GroupHasModelChannels(group string, modelName string) bool {
	if group == "" || modelName == "" {
		return false
	}
	if !common.MemoryCacheEnabled {
		return groupHasModelChannelsDB(group, modelName)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	if group2model2channels == nil {
		return false
	}
	if len(group2model2channels[group][modelName]) > 0 {
		return true
	}
	normalized := ratio_setting.FormatMatchingModelName(modelName)
	if normalized != "" && normalized != modelName {
		return len(group2model2channels[group][normalized]) > 0
	}
	return false
}

func groupHasModelChannelsDB(group string, modelName string) bool {
	var count int64
	err := DB.Model(&Ability{}).
		Where(commonGroupCol+" = ? AND model = ? AND enabled = ?", group, modelName, true).
		Count(&count).Error
	if err == nil && count > 0 {
		return true
	}
	normalized := ratio_setting.FormatMatchingModelName(modelName)
	if normalized == "" || normalized == modelName {
		return false
	}
	count = 0
	err = DB.Model(&Ability{}).
		Where(commonGroupCol+" = ? AND model = ? AND enabled = ?", group, normalized, true).
		Count(&count).Error
	return err == nil && count > 0
}
