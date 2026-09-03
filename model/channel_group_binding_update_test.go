package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelPartialUpdatePreservesGroupBindingsAndAbilities(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	channel := &Channel{
		Name:     "partial-update-channel",
		Key:      "partial-update-key",
		Models:   "gpt-4o,gpt-4o-mini",
		GroupIds: []int{vipGroup.Id, defaultGroup.Id},
		Status:   common.ChannelStatusEnabled,
	}
	require.NoError(t, channel.Insert())

	priority := int64(7)
	update := &Channel{Id: channel.Id, Priority: &priority}
	require.NoError(t, update.Update())

	var bindings []ChannelGroupBinding
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Order("position ASC").Find(&bindings).Error)
	require.Len(t, bindings, 2)
	assert.Equal(t, vipGroup.Id, bindings[0].GroupId)
	assert.Equal(t, defaultGroup.Id, bindings[1].GroupId)

	reloaded, err := GetChannelById(channel.Id, false)
	require.NoError(t, err)
	assert.Equal(t, "vip,default", reloaded.Group)
	assert.Equal(t, []int{vipGroup.Id, defaultGroup.Id}, reloaded.GroupIds)
	assert.Equal(t, []string{"vip", "default"}, []string{reloaded.GroupDetails[0].Code, reloaded.GroupDetails[1].Code})

	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Order("group_id ASC, model ASC").Find(&abilities).Error)
	require.Len(t, abilities, 4)
	for _, ability := range abilities {
		assert.Contains(t, []int{vipGroup.Id, defaultGroup.Id}, ability.GroupId)
		assert.Contains(t, []string{"vip", "default"}, ability.Group)
		assert.Equal(t, channel.Id, ability.ChannelId)
	}
}

func TestChannelMultiKeyPartialUpdatePreservesGroupBindings(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	channel := &Channel{
		Name:        "multi-key-partial-update-channel",
		Key:         "key-one\nkey-two",
		Models:      "gpt-4o",
		GroupIds:    []int{vipGroup.Id, defaultGroup.Id},
		Status:      common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{IsMultiKey: true},
	}
	require.NoError(t, channel.Insert())

	weight := uint(9)
	update := &Channel{Id: channel.Id, Weight: &weight, ChannelInfo: channel.ChannelInfo}
	require.NoError(t, update.Update())

	reloaded, err := GetChannelById(channel.Id, false)
	require.NoError(t, err)
	assert.Equal(t, "vip,default", reloaded.Group)
	assert.Equal(t, []int{vipGroup.Id, defaultGroup.Id}, reloaded.GroupIds)
	assert.Equal(t, 2, reloaded.ChannelInfo.MultiKeySize)

	var abilityCount int64
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", channel.Id).Count(&abilityCount).Error)
	assert.Equal(t, int64(2), abilityCount)
	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
	for _, ability := range abilities {
		assert.Contains(t, []int{vipGroup.Id, defaultGroup.Id}, ability.GroupId)
		assert.Contains(t, []string{"vip", "default"}, ability.Group)
	}
}

func TestChannelExplicitGroupReplacementAndEmptyRejection(t *testing.T) {
	defaultGroup, vipGroup := setupGroupBindingsTest(t)
	channel := &Channel{
		Name:     "replace-group-channel",
		Key:      "replace-group-key",
		Models:   "gpt-4o",
		GroupIds: []int{vipGroup.Id},
		Status:   common.ChannelStatusEnabled,
	}
	require.NoError(t, channel.Insert())

	replace := &Channel{Id: channel.Id, GroupIds: []int{defaultGroup.Id}}
	require.NoError(t, replace.Update())
	reloaded, err := GetChannelById(channel.Id, false)
	require.NoError(t, err)
	assert.Equal(t, "default", reloaded.Group)
	assert.Equal(t, []int{defaultGroup.Id}, reloaded.GroupIds)

	empty := &Channel{Id: channel.Id, GroupIds: []int{}}
	require.Error(t, empty.Update())
	reloaded, err = GetChannelById(channel.Id, false)
	require.NoError(t, err)
	assert.Equal(t, "default", reloaded.Group)
	assert.Equal(t, []int{defaultGroup.Id}, reloaded.GroupIds)

	var ability Ability
	require.NoError(t, DB.Where("channel_id = ? AND model = ?", channel.Id, "gpt-4o").First(&ability).Error)
	assert.Equal(t, defaultGroup.Id, ability.GroupId)
	assert.Equal(t, "default", ability.Group)
}
