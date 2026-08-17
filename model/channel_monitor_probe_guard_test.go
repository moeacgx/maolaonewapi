package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func newChannelMonitorProbeGuardTestChannel(id int, status int) *Channel {
	channel := &Channel{
		Id:       id,
		Name:     "monitor-probe-guard",
		Status:   status,
		Key:      "probed-key",
		Group:    "default",
		Models:   "monitor-probe-model",
		AutoBan:  common.GetPointer(1),
		BaseURL:  common.GetPointer("https://probe.example.com"),
		TestTime: 100,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		MonitorLastTestTime:                 10,
		MonitorConsecutiveFailures:          1,
		MonitorConsecutiveSuccesses:         2,
		MonitorTestIntervalMinutes:          common.GetPointer(1.0),
		MonitorResponseTimeThresholdSeconds: common.GetPointer(3.0),
	})
	return channel
}

func TestChannelMonitorProbeGuardRejectsChangedRequestConfiguration(t *testing.T) {
	original := newChannelMonitorProbeGuardTestChannel(0, common.ChannelStatusEnabled)
	guard, err := NewChannelMonitorProbeGuard(original)
	require.NoError(t, err)
	guard = guard.WithTarget(original.Key, 0, false)

	latest := *original
	latest.BaseURL = common.GetPointer("https://changed.example.com")
	require.False(t, guard.MatchesChannel(&latest))
}

func TestChannelMonitorProbeGuardAllowsLatestMonitorRuntimeState(t *testing.T) {
	original := newChannelMonitorProbeGuardTestChannel(0, common.ChannelStatusEnabled)
	guard, err := NewChannelMonitorProbeGuard(original)
	require.NoError(t, err)
	guard = guard.WithTarget(original.Key, 0, false)

	latest := *original
	settings := latest.GetOtherSettings()
	settings.MonitorLastTestTime = 999
	settings.MonitorConsecutiveFailures = 4
	settings.MonitorConsecutiveSuccesses = 0
	latest.SetOtherSettings(settings)

	require.True(t, guard.MatchesChannel(&latest))
}

func TestSaveChannelMonitorSettingsIfProbeUnchangedRejectsNewerState(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	require.NoError(t, db.AutoMigrate(&Channel{}))

	channel := newChannelMonitorProbeGuardTestChannel(991020, common.ChannelStatusEnabled)
	require.NoError(t, db.Create(channel).Error)

	var probed Channel
	require.NoError(t, db.First(&probed, channel.Id).Error)
	guard, err := NewChannelMonitorProbeGuard(&probed)
	require.NoError(t, err)
	guard = guard.WithTarget(probed.Key, 0, false)

	require.NoError(t, db.Model(&Channel{}).
		Where("id = ?", channel.Id).
		Update("status", common.ChannelStatusAutoDisabled).Error)

	settings := probed.GetOtherSettings()
	settings.MonitorConsecutiveFailures = 5
	err = SaveChannelMonitorSettings(&probed, settings, &guard)
	require.ErrorIs(t, err, ErrChannelMonitorProbeStateChanged)

	var reloaded Channel
	require.NoError(t, db.First(&reloaded, channel.Id).Error)
	require.Equal(t, common.ChannelStatusAutoDisabled, reloaded.Status)
	require.Equal(t, 1, reloaded.GetOtherSettings().MonitorConsecutiveFailures)
}

func TestUpdateResponseTimeIfProbeUnchangedRejectsChangedKey(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	require.NoError(t, db.AutoMigrate(&Channel{}))

	channel := newChannelMonitorProbeGuardTestChannel(991021, common.ChannelStatusEnabled)
	require.NoError(t, db.Create(channel).Error)

	var probed Channel
	require.NoError(t, db.First(&probed, channel.Id).Error)
	guard, err := NewChannelMonitorProbeGuard(&probed)
	require.NoError(t, err)
	guard = guard.WithTarget(probed.Key, 0, false)

	require.NoError(t, db.Model(&Channel{}).
		Where("id = ?", channel.Id).
		Update("key", "replacement-key").Error)

	require.False(t, probed.UpdateResponseTimeIfProbeUnchanged(321, guard))

	var reloaded Channel
	require.NoError(t, db.First(&reloaded, channel.Id).Error)
	require.Zero(t, reloaded.ResponseTime)
}

func TestDisableChannelForMonitorIfProbeUnchangedRejectsChangedConfig(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))

	channel := newChannelMonitorProbeGuardTestChannel(991022, common.ChannelStatusEnabled)
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&Ability{
		Group:     "default",
		Model:     channel.Models,
		ChannelId: channel.Id,
		Enabled:   true,
	}).Error)

	var probed Channel
	require.NoError(t, db.First(&probed, channel.Id).Error)
	guard, err := NewChannelMonitorProbeGuard(&probed)
	require.NoError(t, err)
	guard = guard.WithTarget(probed.Key, 0, false)

	require.NoError(t, db.Model(&Channel{}).
		Where("id = ?", channel.Id).
		Update("base_url", "https://changed.example.com").Error)

	require.False(t, DisableChannelForMonitorIfProbeUnchanged(channel.Id, -1, probed.Key, guard, "stale failure"))

	var reloaded Channel
	require.NoError(t, db.First(&reloaded, channel.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, reloaded.Status)
}

func TestEnableAutoDisabledSingleKeyChannelIfProbeUnchangedRejectsChangedConfig(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))

	channel := newChannelMonitorProbeGuardTestChannel(991023, common.ChannelStatusAutoDisabled)
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&Ability{
		Group:     "default",
		Model:     channel.Models,
		ChannelId: channel.Id,
		Enabled:   false,
	}).Error)

	var probed Channel
	require.NoError(t, db.First(&probed, channel.Id).Error)
	guard, err := NewChannelMonitorProbeGuard(&probed)
	require.NoError(t, err)
	guard = guard.WithTarget(probed.Key, 0, false)

	require.NoError(t, db.Model(&Channel{}).
		Where("id = ?", channel.Id).
		Update("header_override", `{"X-Probe":"changed"}`).Error)

	require.False(t, EnableAutoDisabledSingleKeyChannelIfProbeUnchanged(channel.Id, probed.Key, guard))

	var reloaded Channel
	require.NoError(t, db.First(&reloaded, channel.Id).Error)
	require.Equal(t, common.ChannelStatusAutoDisabled, reloaded.Status)
}
