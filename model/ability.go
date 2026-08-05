package model

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"

	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Ability struct {
	Group     string  `json:"group" gorm:"type:varchar(64);primaryKey;autoIncrement:false"`
	GroupId   int     `json:"group_id" gorm:"index"`
	Model     string  `json:"model" gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
	ChannelId int     `json:"channel_id" gorm:"primaryKey;autoIncrement:false;index"`
	Enabled   bool    `json:"enabled"`
	Priority  *int64  `json:"priority" gorm:"bigint;default:0;index"`
	Weight    uint    `json:"weight" gorm:"default:0;index"`
	Tag       *string `json:"tag" gorm:"index"`
}

type AbilityWithChannel struct {
	Ability
	ChannelType int `json:"channel_type"`
}

func GetAllEnableAbilityWithChannels() ([]AbilityWithChannel, error) {
	var abilities []AbilityWithChannel
	err := DB.Table("abilities").
		Select("abilities.*, channels.type as channel_type").
		Joins("left join channels on abilities.channel_id = channels.id").
		Where("abilities.enabled = ?", true).
		Scan(&abilities).Error
	return abilities, err
}

func GetGroupEnabledModels(group string) []string {
	var models []string
	// Find distinct models
	DB.Table("abilities").Where(commonGroupCol+" = ? and enabled = ?", group, true).Distinct("model").Pluck("model", &models)
	return models
}

func GetEnabledModels() []string {
	var models []string
	// Find distinct models
	DB.Table("abilities").Where("enabled = ?", true).Distinct("model").Pluck("model", &models)
	return models
}

func GetAllEnableAbilities() []Ability {
	var abilities []Ability
	DB.Find(&abilities, "enabled = ?", true)
	return abilities
}

func getPriority(group string, model string, retry int) (int, error) {

	var priorities []int
	err := DB.Model(&Ability{}).
		Select("DISTINCT(priority)").
		Where(commonGroupCol+" = ? and model = ? and enabled = ?", group, model, true).
		Order("priority DESC").              // 按优先级降序排序
		Pluck("priority", &priorities).Error // Pluck用于将查询的结果直接扫描到一个切片中

	if err != nil {
		// 处理错误
		return 0, err
	}

	if len(priorities) == 0 {
		// 如果没有查询到优先级，则返回错误
		return 0, errors.New("数据库一致性被破坏")
	}

	// 确定要使用的优先级
	var priorityToUse int
	if retry >= len(priorities) {
		// 如果重试次数大于优先级数，则使用最小的优先级
		priorityToUse = priorities[len(priorities)-1]
	} else {
		priorityToUse = priorities[retry]
	}
	return priorityToUse, nil
}

func getPriorities(group string, model string) ([]int, error) {
	var priorities []int
	err := DB.Model(&Ability{}).
		Select("DISTINCT(priority)").
		Where(commonGroupCol+" = ? and model = ? and enabled = ?", group, model, true).
		Order("priority DESC").
		Pluck("priority", &priorities).Error
	return priorities, err
}

func getChannelQuery(group string, model string, retry int) (*gorm.DB, error) {
	maxPrioritySubQuery := DB.Model(&Ability{}).Select("MAX(priority)").Where(commonGroupCol+" = ? and model = ? and enabled = ?", group, model, true)
	channelQuery := DB.Where(commonGroupCol+" = ? and model = ? and enabled = ? and priority = (?)", group, model, true, maxPrioritySubQuery)
	if retry != 0 {
		priority, err := getPriority(group, model, retry)
		if err != nil {
			return nil, err
		} else {
			channelQuery = DB.Where(commonGroupCol+" = ? and model = ? and enabled = ? and priority = ?", group, model, true, priority)
		}
	}

	return channelQuery, nil
}

func GetChannel(group string, model string, retry int) (*Channel, error) {
	return GetChannelWithExclusions(group, model, retry, nil)
}

func GetChannelWithExclusions(group string, model string, retry int, excludedChannelIDs map[int]struct{}) (*Channel, error) {
	return GetChannelWithSelectionExclusions(group, model, retry, ChannelSelectionExclusions{
		ChannelIDs: excludedChannelIDs,
	})
}

func GetChannelWithSelectionExclusions(group string, model string, retry int, exclusions ChannelSelectionExclusions) (*Channel, error) {
	priorities, err := getPriorities(group, model)
	if err != nil {
		return nil, err
	}
	if len(priorities) == 0 {
		return nil, nil
	}
	if retry >= len(priorities) {
		retry = len(priorities) - 1
	}

	return getChannelWithPriorityFallback(group, model, priorities, retry, exclusions)
}

func getChannelWithPriorityFallback(group string, model string, priorities []int, retry int, exclusions ChannelSelectionExclusions) (*Channel, error) {
	priorityIndexes := buildPrioritySearchOrder(len(priorities), retry, true, exclusions.hasAny())
	for _, priorityIndex := range priorityIndexes {
		candidates, err := getAvailableAbilitiesForPriority(group, model, priorities[priorityIndex], exclusions)
		if err != nil {
			return nil, err
		}
		if len(candidates) > 0 {
			return selectChannelByAbilities(candidates), nil
		}
	}
	return nil, nil
}

type availableChannelCandidate struct {
	channel *Channel
	weight  uint
}

func getAvailableAbilitiesForPriority(group string, model string, priority int, exclusions ChannelSelectionExclusions) ([]availableChannelCandidate, error) {
	var abilities []Ability
	channelQuery := DB.Where(commonGroupCol+" = ? and model = ? and enabled = ? and priority = ?", group, model, true, priority)
	err := channelQuery.Order("weight DESC").Find(&abilities).Error
	if err != nil {
		return nil, err
	}
	channelIDs := make([]int, 0, len(abilities))
	seenChannelIDs := make(map[int]struct{}, len(abilities))
	for _, ability := range abilities {
		if _, seen := seenChannelIDs[ability.ChannelId]; seen {
			continue
		}
		seenChannelIDs[ability.ChannelId] = struct{}{}
		channelIDs = append(channelIDs, ability.ChannelId)
	}
	var candidates []*Channel
	if len(channelIDs) > 0 {
		if err = DB.Where("id IN ?", channelIDs).Find(&candidates).Error; err != nil {
			return nil, err
		}
		if err = HydrateChannelGroupBindings(DB, candidates); err != nil {
			return nil, err
		}
	}
	candidateByID := make(map[int]*Channel, len(candidates))
	for _, candidate := range candidates {
		candidateByID[candidate.Id] = candidate
	}
	availableCandidates := make([]availableChannelCandidate, 0, len(abilities))
	for _, ability := range abilities {
		// MySQL 常见排序规则不区分大小写，SQL 条件可能同时返回
		// 大小写不同的分组；稳定分组编码必须在 Go 中再次精确比较。
		if strings.TrimSpace(ability.Group) != strings.TrimSpace(group) {
			continue
		}
		if exclusions.excludesChannelID(ability.ChannelId) {
			continue
		}
		candidate := candidateByID[ability.ChannelId]
		if candidate == nil {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", ability.ChannelId)
		}
		// abilities 是历史兼容表，分组编辑或迁移异常时可能残留旧的
		// group 记录。最终选渠必须以 channels 当前绑定的分组为准，
		// 避免把已经移出该分组的渠道重新选回来。
		if !channelHasCurrentGroup(candidate, group) {
			continue
		}
		if exclusions.excludesChannelType(candidate.Type) || !IsChannelConcurrencyAvailable(candidate) {
			continue
		}
		availableCandidates = append(availableCandidates, availableChannelCandidate{
			channel: candidate,
			weight:  ability.Weight,
		})
	}
	return availableCandidates, nil
}

func channelHasCurrentGroup(channel *Channel, group string) bool {
	if channel == nil || channel.Id <= 0 {
		return false
	}
	group = strings.TrimSpace(group)
	if group == "" {
		return false
	}
	if channel.GroupsHydrated {
		for _, detail := range channel.GroupDetails {
			if strings.TrimSpace(detail.Code) == group {
				return true
			}
		}
		return false
	}
	for _, current := range channel.GetGroups() {
		if strings.TrimSpace(current) == group {
			return true
		}
	}
	return false
}

func selectChannelByAbilities(candidates []availableChannelCandidate) *Channel {
	weightSum := uint(0)
	for _, candidate := range candidates {
		weightSum += candidate.weight + 10
	}
	weight := common.GetRandomInt(int(weightSum))
	for _, candidate := range candidates {
		weight -= int(candidate.weight) + 10
		if weight <= 0 {
			return candidate.channel
		}
	}
	return candidates[len(candidates)-1].channel
}

func (channel *Channel) AddAbilities(tx *gorm.DB) error {
	useDB := DB
	if tx != nil {
		useDB = tx
	}
	models_ := strings.Split(channel.Models, ",")
	groups_ := strings.Split(channel.Group, ",")
	abilitySet := make(map[string]struct{})
	abilities := make([]Ability, 0, len(models_))
	for _, model := range models_ {
		for _, group := range groups_ {
			key := group + "|" + model
			if _, exists := abilitySet[key]; exists {
				continue
			}
			abilitySet[key] = struct{}{}
			groupID, _ := ResolveGroupIDByCodeWithDB(useDB, group)
			ability := Ability{
				Group:     group,
				GroupId:   groupID,
				Model:     model,
				ChannelId: channel.Id,
				Enabled:   channel.Status == common.ChannelStatusEnabled,
				Priority:  channel.Priority,
				Weight:    uint(channel.GetWeight()),
				Tag:       channel.Tag,
			}
			abilities = append(abilities, ability)
		}
	}
	if len(abilities) == 0 {
		return nil
	}
	for _, chunk := range lo.Chunk(abilities, 50) {
		err := useDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error
		if err != nil {
			return err
		}
	}
	return nil
}

func (channel *Channel) DeleteAbilities() error {
	return DB.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
}

// UpdateAbilities updates abilities of this channel.
// Make sure the channel is completed before calling this function.
func (channel *Channel) UpdateAbilities(tx *gorm.DB) error {
	isNewTx := false
	// 如果没有传入事务，创建新的事务
	if tx == nil {
		tx = DB.Begin()
		if tx.Error != nil {
			return tx.Error
		}
		isNewTx = true
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()
	}

	// First delete all abilities of this channel
	err := tx.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
	if err != nil {
		if isNewTx {
			tx.Rollback()
		}
		return err
	}

	// Then add new abilities
	useDB := tx
	if useDB == nil {
		useDB = DB
	}
	models_ := strings.Split(channel.Models, ",")
	groups_ := strings.Split(channel.Group, ",")
	abilitySet := make(map[string]struct{})
	abilities := make([]Ability, 0, len(models_))
	for _, model := range models_ {
		for _, group := range groups_ {
			key := group + "|" + model
			if _, exists := abilitySet[key]; exists {
				continue
			}
			abilitySet[key] = struct{}{}
			groupID, _ := ResolveGroupIDByCodeWithDB(useDB, group)
			ability := Ability{
				Group:     group,
				GroupId:   groupID,
				Model:     model,
				ChannelId: channel.Id,
				Enabled:   channel.Status == common.ChannelStatusEnabled,
				Priority:  channel.Priority,
				Weight:    uint(channel.GetWeight()),
				Tag:       channel.Tag,
			}
			abilities = append(abilities, ability)
		}
	}

	if len(abilities) > 0 {
		for _, chunk := range lo.Chunk(abilities, 50) {
			err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error
			if err != nil {
				if isNewTx {
					tx.Rollback()
				}
				return err
			}
		}
	}

	// 如果是新创建的事务，需要提交
	if isNewTx {
		return tx.Commit().Error
	}

	return nil
}

func UpdateAbilityStatus(channelId int, status bool) error {
	return DB.Model(&Ability{}).Where("channel_id = ?", channelId).Select("enabled").Update("enabled", status).Error
}

func UpdateAbilityStatusByTag(tag string, status bool) error {
	return DB.Model(&Ability{}).Where("tag = ?", tag).Select("enabled").Update("enabled", status).Error
}

func UpdateAbilityByTag(tag string, newTag *string, priority *int64, weight *uint) error {
	ability := Ability{}
	if newTag != nil {
		ability.Tag = newTag
	}
	if priority != nil {
		ability.Priority = priority
	}
	if weight != nil {
		ability.Weight = *weight
	}
	return DB.Model(&Ability{}).Where("tag = ?", tag).Updates(ability).Error
}

var fixLock = sync.Mutex{}

func FixAbility() (int, int, error) {
	lock := fixLock.TryLock()
	if !lock {
		return 0, 0, errors.New("已经有一个修复任务在运行中，请稍后再试")
	}
	defer fixLock.Unlock()

	// truncate abilities table
	if common.UsingSQLite {
		err := DB.Exec("DELETE FROM abilities").Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Delete abilities failed: %s", err.Error()))
			return 0, 0, err
		}
	} else {
		err := DB.Exec("TRUNCATE TABLE abilities").Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Truncate abilities failed: %s", err.Error()))
			return 0, 0, err
		}
	}
	var channels []*Channel
	// Find all channels
	err := DB.Model(&Channel{}).Find(&channels).Error
	if err != nil {
		return 0, 0, err
	}
	if len(channels) == 0 {
		return 0, 0, nil
	}
	successCount := 0
	failCount := 0
	for _, chunk := range lo.Chunk(channels, 50) {
		ids := lo.Map(chunk, func(c *Channel, _ int) int { return c.Id })
		// Delete all abilities of this channel
		err = DB.Where("channel_id IN ?", ids).Delete(&Ability{}).Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Delete abilities failed: %s", err.Error()))
			failCount += len(chunk)
			continue
		}
		// Then add new abilities
		for _, channel := range chunk {
			err = channel.AddAbilities(nil)
			if err != nil {
				common.SysLog(fmt.Sprintf("Add abilities for channel %d failed: %s", channel.Id, err.Error()))
				failCount++
			} else {
				successCount++
			}
		}
	}
	InitChannelCache()
	return successCount, failCount, nil
}
