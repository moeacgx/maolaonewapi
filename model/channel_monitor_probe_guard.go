package model

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
)

var ErrChannelMonitorProbeStateChanged = errors.New("channel monitor probe state changed")

var errChannelMonitorSettingsCASConflict = errors.New("channel monitor settings CAS conflict")

type ChannelMonitorProbeGuard struct {
	status             int
	key                string
	isMultiKey         bool
	multiKeyStatusList map[int]int
	configFingerprint  [sha256.Size]byte
	targetKey          string
	targetKeyIndex     int
	hasTargetKeyIndex  bool
	targetBound        bool
	initialized        bool
}

type channelMonitorProbeConfiguration struct {
	Type               int                      `json:"type"`
	ConcurrencyLimit   *int                     `json:"concurrency_limit"`
	OpenAIOrganization *string                  `json:"openai_organization"`
	TestModel          *string                  `json:"test_model"`
	BaseURL            *string                  `json:"base_url"`
	Other              string                   `json:"other"`
	Models             string                   `json:"models"`
	ModelMapping       *string                  `json:"model_mapping"`
	StatusCodeMapping  *string                  `json:"status_code_mapping"`
	Setting            *string                  `json:"setting"`
	ParamOverride      *string                  `json:"param_override"`
	HeaderOverride     *string                  `json:"header_override"`
	OtherSettings      dto.ChannelOtherSettings `json:"other_settings"`
}

func channelMonitorRequestSettings(settings dto.ChannelOtherSettings) dto.ChannelOtherSettings {
	settings.UpstreamModelUpdateCheckEnabled = false
	settings.UpstreamModelUpdateAutoSyncEnabled = false
	settings.UpstreamModelUpdateLastCheckTime = 0
	settings.UpstreamModelUpdateLastDetectedModels = nil
	settings.UpstreamModelUpdateLastRemovedModels = nil
	settings.UpstreamModelUpdateIgnoredModels = nil
	settings.MonitorEnabled = nil
	settings.MonitorTestIntervalMinutes = nil
	settings.MonitorResponseTimeThresholdSeconds = nil
	settings.MonitorAutoDisableEnabled = nil
	settings.MonitorAutoEnableEnabled = nil
	settings.MonitorDisableThreshold = nil
	settings.MonitorEnableThreshold = nil
	settings.MonitorLastTestTime = 0
	settings.MonitorConsecutiveFailures = 0
	settings.MonitorConsecutiveSuccesses = 0
	return settings
}

func channelMonitorProbeConfigurationFingerprint(channel *Channel) ([sha256.Size]byte, error) {
	var fingerprint [sha256.Size]byte
	if channel == nil {
		return fingerprint, errors.New("channel is required")
	}

	otherSettings := dto.ChannelOtherSettings{}
	if channel.OtherSettings != "" {
		if err := common.UnmarshalJsonStr(channel.OtherSettings, &otherSettings); err != nil {
			return fingerprint, fmt.Errorf("failed to parse channel probe settings: %w", err)
		}
	}
	configuration := channelMonitorProbeConfiguration{
		Type:               channel.Type,
		ConcurrencyLimit:   channel.ConcurrencyLimit,
		OpenAIOrganization: channel.OpenAIOrganization,
		TestModel:          channel.TestModel,
		BaseURL:            channel.BaseURL,
		Other:              channel.Other,
		Models:             channel.Models,
		ModelMapping:       channel.ModelMapping,
		StatusCodeMapping:  channel.StatusCodeMapping,
		Setting:            channel.Setting,
		ParamOverride:      channel.ParamOverride,
		HeaderOverride:     channel.HeaderOverride,
		OtherSettings:      channelMonitorRequestSettings(otherSettings),
	}
	payload, err := common.Marshal(configuration)
	if err != nil {
		return fingerprint, fmt.Errorf("failed to marshal channel probe configuration: %w", err)
	}
	return sha256.Sum256(payload), nil
}

func NewChannelMonitorProbeGuard(channel *Channel) (ChannelMonitorProbeGuard, error) {
	guard := ChannelMonitorProbeGuard{}
	fingerprint, err := channelMonitorProbeConfigurationFingerprint(channel)
	if err != nil {
		return guard, err
	}
	guard.status = channel.Status
	guard.key = channel.Key
	guard.isMultiKey = channel.ChannelInfo.IsMultiKey
	guard.configFingerprint = fingerprint
	guard.initialized = true
	if channel.ChannelInfo.MultiKeyStatusList != nil {
		guard.multiKeyStatusList = make(map[int]int, len(channel.ChannelInfo.MultiKeyStatusList))
		for index, status := range channel.ChannelInfo.MultiKeyStatusList {
			guard.multiKeyStatusList[index] = status
		}
	}
	return guard, nil
}

func (guard ChannelMonitorProbeGuard) WithTarget(usingKey string, keyIndex int, hasKeyIndex bool) ChannelMonitorProbeGuard {
	guard.targetKey = usingKey
	guard.targetKeyIndex = keyIndex
	guard.hasTargetKeyIndex = hasKeyIndex
	guard.targetBound = true
	return guard
}

func (guard ChannelMonitorProbeGuard) ExpectedStatus() int {
	return guard.status
}

func channelMonitorProbeKeyStatus(statusList map[int]int, index int) int {
	if status, ok := statusList[index]; ok {
		return status
	}
	return common.ChannelStatusEnabled
}

func channelMonitorProbeKeyStatusesEqual(left map[int]int, right map[int]int) bool {
	if len(left) != len(right) {
		return false
	}
	for index, status := range left {
		if rightStatus, ok := right[index]; !ok || rightStatus != status {
			return false
		}
	}
	return true
}

func (guard ChannelMonitorProbeGuard) MatchesChannel(channel *Channel) bool {
	if !guard.initialized || !guard.targetBound || channel == nil ||
		channel.Status != guard.status || channel.Key != guard.key ||
		channel.ChannelInfo.IsMultiKey != guard.isMultiKey {
		return false
	}
	fingerprint, err := channelMonitorProbeConfigurationFingerprint(channel)
	if err != nil || fingerprint != guard.configFingerprint {
		return false
	}
	if guard.targetKey == "" {
		return !guard.isMultiKey ||
			channelMonitorProbeKeyStatusesEqual(guard.multiKeyStatusList, channel.ChannelInfo.MultiKeyStatusList)
	}
	if !guard.isMultiKey {
		return guard.targetKey == guard.key
	}
	if !guard.hasTargetKeyIndex {
		return false
	}
	currentKeys := channel.GetKeys()
	snapshotChannel := &Channel{Key: guard.key}
	snapshotKeys := snapshotChannel.GetKeys()
	if guard.targetKeyIndex < 0 || guard.targetKeyIndex >= len(snapshotKeys) ||
		guard.targetKeyIndex >= len(currentKeys) ||
		snapshotKeys[guard.targetKeyIndex] != guard.targetKey ||
		currentKeys[guard.targetKeyIndex] != guard.targetKey {
		return false
	}
	return channelMonitorProbeKeyStatus(guard.multiKeyStatusList, guard.targetKeyIndex) ==
		channelMonitorProbeKeyStatus(channel.ChannelInfo.MultiKeyStatusList, guard.targetKeyIndex)
}

func channelMonitorNullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func channelMonitorNullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func channelMonitorProbeCASValues(channel *Channel) map[string]any {
	return map[string]any{
		"status":               channel.Status,
		"key":                  channel.Key,
		"type":                 channel.Type,
		"concurrency_limit":    channelMonitorNullableInt(channel.ConcurrencyLimit),
		"open_ai_organization": channelMonitorNullableString(channel.OpenAIOrganization),
		"test_model":           channelMonitorNullableString(channel.TestModel),
		"base_url":             channelMonitorNullableString(channel.BaseURL),
		"other":                channel.Other,
		"models":               channel.Models,
		"model_mapping":        channelMonitorNullableString(channel.ModelMapping),
		"status_code_mapping":  channelMonitorNullableString(channel.StatusCodeMapping),
		"setting":              channelMonitorNullableString(channel.Setting),
		"param_override":       channelMonitorNullableString(channel.ParamOverride),
		"header_override":      channelMonitorNullableString(channel.HeaderOverride),
	}
}

func applyChannelMonitorSettingsCAS(query *gorm.DB, settings string) *gorm.DB {
	if settings == "" {
		return query.Where("(settings = ? OR settings IS NULL)", "")
	}
	return query.Where("settings = ?", settings)
}

func applyChannelMonitorProbeCAS(query *gorm.DB, channel *Channel, rawChannelInfo []byte) *gorm.DB {
	query = query.Where(channelMonitorProbeCASValues(channel))
	query = applyChannelMonitorSettingsCAS(query, channel.OtherSettings)
	if common.UsingSQLite {
		query = query.Where("channel_info = ?", rawChannelInfo)
	}
	return query
}

func loadChannelMonitorProbeState(tx *gorm.DB, channelID int) (Channel, []byte, error) {
	var channel Channel
	if err := lockForUpdate(tx).Where("id = ?", channelID).First(&channel).Error; err != nil {
		return channel, nil, err
	}
	var rawChannelInfo []byte
	if common.UsingSQLite {
		if err := tx.Model(&Channel{}).
			Select("channel_info").
			Where("id = ?", channelID).
			Row().
			Scan(&rawChannelInfo); err != nil {
			return channel, nil, err
		}
	}
	return channel, rawChannelInfo, nil
}

func (channel *Channel) UpdateResponseTimeIfProbeUnchanged(responseTime int64, guard ChannelMonitorProbeGuard) bool {
	if channel == nil || channel.Id <= 0 {
		return false
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		current, rawChannelInfo, err := loadChannelMonitorProbeState(tx, channel.Id)
		if err != nil {
			return err
		}
		if !guard.MatchesChannel(&current) {
			return ErrChannelMonitorProbeStateChanged
		}
		result := applyChannelMonitorProbeCAS(
			tx.Model(&Channel{}).Where("id = ?", channel.Id),
			&current,
			rawChannelInfo,
		).Select("response_time", "test_time").Updates(Channel{
			TestTime:     common.GetTimestamp(),
			ResponseTime: int(responseTime),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrChannelMonitorProbeStateChanged
		}
		return nil
	})
	if errors.Is(err, ErrChannelMonitorProbeStateChanged) {
		return false
	}
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to conditionally update monitor response time: channel_id=%d, error=%v", channel.Id, err))
		return false
	}
	return true
}

func SaveChannelMonitorSettings(channel *Channel, settings dto.ChannelOtherSettings, guard *ChannelMonitorProbeGuard) error {
	if channel == nil || channel.Id <= 0 {
		return errors.New("channel is required")
	}

	const maxAttempts = 5
	for attempt := 0; attempt < maxAttempts; attempt++ {
		var savedSettings string
		err := DB.Transaction(func(tx *gorm.DB) error {
			current, rawChannelInfo, err := loadChannelMonitorProbeState(tx, channel.Id)
			if err != nil {
				return err
			}
			if guard != nil && !guard.MatchesChannel(&current) {
				return ErrChannelMonitorProbeStateChanged
			}

			latest := dto.ChannelOtherSettings{}
			if current.OtherSettings != "" {
				if err := common.UnmarshalJsonStr(current.OtherSettings, &latest); err != nil {
					return fmt.Errorf("failed to parse channel monitor settings: %w", err)
				}
			}
			latest.MonitorLastTestTime = settings.MonitorLastTestTime
			latest.MonitorConsecutiveFailures = settings.MonitorConsecutiveFailures
			latest.MonitorConsecutiveSuccesses = settings.MonitorConsecutiveSuccesses
			payload, err := common.Marshal(latest)
			if err != nil {
				return fmt.Errorf("failed to marshal channel monitor settings: %w", err)
			}
			savedSettings = string(payload)
			if savedSettings == current.OtherSettings {
				return nil
			}

			query := tx.Model(&Channel{}).Where("id = ?", channel.Id)
			if guard != nil {
				query = applyChannelMonitorProbeCAS(query, &current, rawChannelInfo)
			} else {
				query = applyChannelMonitorSettingsCAS(query, current.OtherSettings)
			}
			result := query.Update("settings", savedSettings)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errChannelMonitorSettingsCASConflict
			}
			return nil
		})
		if err == nil {
			channel.OtherSettings = savedSettings
			return nil
		}
		if errors.Is(err, ErrChannelMonitorProbeStateChanged) {
			return err
		}
		if !errors.Is(err, errChannelMonitorSettingsCASConflict) {
			return err
		}
	}

	return errors.New("channel monitor settings changed too frequently")
}
