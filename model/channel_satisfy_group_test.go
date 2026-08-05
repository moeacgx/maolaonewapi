package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestCandidateChannelTagMatchingUsesManagementTags(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	channelSyncLock.RLock()
	oldGroupCache, oldChannelCache := group2model2channels, channelsIDM
	channelSyncLock.RUnlock()
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		channelSyncLock.Lock()
		group2model2channels, channelsIDM = oldGroupCache, oldChannelCache
		channelSyncLock.Unlock()
	})
	require.NoError(t, db.AutoMigrate(&Group{}, &Channel{}, &ChannelGroupBinding{}, &Ability{}))

	routeGroup := Group{Code: "route", Name: "路由分组"}
	otherRouteGroup := Group{Code: "other-route", Name: "其他用户分组"}
	require.NoError(t, db.Create(&routeGroup).Error)
	require.NoError(t, db.Create(&otherRouteGroup).Error)

	channelA := Channel{Name: "candidate-a", Key: "secret", Group: routeGroup.Code, Models: "gpt-test", Status: common.ChannelStatusEnabled, Tag: common.GetPointer("batch-a")}
	channelB := Channel{Name: "candidate-b", Key: "secret", Group: routeGroup.Code, Models: "gpt-test", Status: common.ChannelStatusEnabled, Tag: common.GetPointer("batch-b")}
	channelC := Channel{Name: "other-group-same-tag", Key: "secret", Group: otherRouteGroup.Code, Models: "gpt-test", Status: common.ChannelStatusEnabled, Tag: common.GetPointer("batch-a")}
	require.NoError(t, db.Create(&channelA).Error)
	require.NoError(t, db.Create(&channelB).Error)
	require.NoError(t, db.Create(&channelC).Error)
	require.NoError(t, db.Create([]Ability{
		{
			Group: routeGroup.Code, GroupId: routeGroup.Id, Model: "gpt-test", ChannelId: channelA.Id, Enabled: true,
		},
		{
			Group: routeGroup.Code, GroupId: routeGroup.Id, Model: "gpt-test", ChannelId: channelB.Id, Enabled: true,
		},
		{
			Group: otherRouteGroup.Code, GroupId: otherRouteGroup.Id, Model: "gpt-test", ChannelId: channelC.Id, Enabled: true,
		},
	}).Error)
	require.NoError(t, db.Create(&Ability{
		Group: routeGroup.Code, GroupId: routeGroup.Id, Model: "gpt-test",
		ChannelId: 999999, Enabled: true,
	}).Error)

	tagOptions, err := GetAllChannelTagOptions()
	require.NoError(t, err)
	require.Equal(t, []ChannelTagOption{{Tag: "batch-a", ChannelCount: 2}, {Tag: "batch-b", ChannelCount: 1}}, tagOptions)

	status, tag, exists, err := GetChannelStatusAndTag(channelA.Id)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, common.ChannelStatusEnabled, status)
	require.Equal(t, "batch-a", tag)
	_, _, exists, err = GetChannelStatusAndTag(999999)
	require.NoError(t, err)
	require.False(t, exists)

	matched, err := ChannelBelongsToAnyTag(channelA.Id, []string{"batch-a"})
	require.NoError(t, err)
	require.True(t, matched)

	matched, err = AnyCandidateChannelBelongsToTags(
		[]string{routeGroup.Code}, "gpt-test", []string{"batch-a"},
	)
	require.NoError(t, err)
	require.True(t, matched)

	// 路由和用户分组名称不能被当成渠道管理标签。
	matched, err = AnyCandidateChannelBelongsToTags(
		[]string{routeGroup.Code}, "gpt-test", []string{routeGroup.Code},
	)
	require.NoError(t, err)
	require.False(t, matched)

	matched, err = AnyCandidateChannelBelongsToTags(
		[]string{routeGroup.Code}, "other-model", []string{"batch-a"},
	)
	require.NoError(t, err)
	require.False(t, matched)

	require.NoError(t, db.Model(&Channel{}).Where("id = ?", channelA.Id).
		Update("status", common.ChannelStatusManuallyDisabled).Error)
	matched, err = AnyCandidateChannelBelongsToTags(
		[]string{routeGroup.Code}, "gpt-test", []string{"batch-a"},
	)
	require.NoError(t, err)
	require.False(t, matched)
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", channelA.Id).
		Update("status", common.ChannelStatusEnabled).Error)

	matched, err = AnySpecificChannelIsCandidate(
		[]string{routeGroup.Code}, "gpt-test", []int{channelA.Id},
	)
	require.NoError(t, err)
	require.True(t, matched)
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", channelA.Id).
		Update("status", common.ChannelStatusManuallyDisabled).Error)
	matched, err = AnySpecificChannelIsCandidate(
		[]string{routeGroup.Code}, "gpt-test", []int{channelA.Id},
	)
	require.NoError(t, err)
	require.False(t, matched)
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", channelA.Id).
		Update("status", common.ChannelStatusEnabled).Error)
	matched, err = AnySpecificChannelIsCandidate(
		[]string{routeGroup.Code}, "other-model", []int{channelA.Id},
	)
	require.NoError(t, err)
	require.False(t, matched)

	common.MemoryCacheEnabled = true
	InitChannelCache()
	matched, err = AnyCandidateChannelBelongsToTags(
		[]string{routeGroup.Code}, "gpt-test", []string{"batch-a"},
	)
	require.NoError(t, err)
	require.True(t, matched)
	matched, err = AnySpecificChannelIsCandidate(
		[]string{routeGroup.Code}, "gpt-test", []int{channelA.Id},
	)
	require.NoError(t, err)
	require.True(t, matched)
}
