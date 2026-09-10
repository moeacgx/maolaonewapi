package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

const (
	tokensProOverviewPath                   = "/api/v1/downstream/overview"
	tokensProOverviewDefaultPlatform        = "openai"
	tokensProOverviewDefaultIntervalSeconds = 60
	tokensProOverviewRequestTimeout         = 10 * time.Second
	tokensProOverviewMaxErrorBytes          = 200
	tokensProOverviewZeroDisableReason      = "TokensPro overview concurrency.allowed=0"
	tokensProOverviewMaxResponseBytes       = 1 << 20
)

type tokensProOverviewSyncResult struct {
	Skipped  bool
	Allowed  *int
	Error    string
	Disabled bool
	Enabled  bool
}

type tokensProOverviewSyncSummary struct {
	Total    int `json:"total"`
	Synced   int `json:"synced"`
	Skipped  int `json:"skipped"`
	Failed   int `json:"failed"`
	Disabled int `json:"disabled"`
	Enabled  int `json:"enabled"`
}

type tokensProOverviewPayload struct {
	Concurrency struct {
		Allowed *float64 `json:"allowed"`
	} `json:"concurrency"`
}

var fetchTokensProOverview = fetchTokensProOverviewHTTP

var tokensProOverviewHTTPClient = defaultTokensProOverviewHTTPClient

var disableChannelForTokensProOverview = applyTokensProOverviewDisable

var enableChannelForTokensProOverview = applyTokensProOverviewEnable

func defaultTokensProOverviewHTTPClient(channel *model.Channel) (*http.Client, error) {
	if channel == nil {
		return GetHttpClientWithProxy("")
	}
	return GetHttpClientWithProxySettings(channel.GetSetting().Proxy, channel.GetSetting())
}

func parseTokensProOverviewAllowed(body []byte) (int, error) {
	var payload tokensProOverviewPayload
	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, fmt.Errorf("invalid tokenspro overview JSON: %w", err)
	}
	if payload.Concurrency.Allowed == nil {
		return 0, errors.New("concurrency.allowed is required")
	}
	allowedFloat := *payload.Concurrency.Allowed
	if math.IsNaN(allowedFloat) || math.IsInf(allowedFloat, 0) || allowedFloat < 0 || allowedFloat != math.Trunc(allowedFloat) {
		return 0, errors.New("concurrency.allowed must be a non-negative integer")
	}
	if allowedFloat > float64(math.MaxInt) {
		return 0, errors.New("concurrency.allowed is too large")
	}
	return int(allowedFloat), nil
}

func buildTokensProOverviewURL(baseURL string) (string, error) {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return "", errors.New("tokenspro overview base url is empty")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("tokenspro overview base url is invalid")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if strings.EqualFold(parsed.Path, "/v1") {
		parsed.Path = ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/") + tokensProOverviewPath
	query := parsed.Query()
	query.Set("platform", tokensProOverviewDefaultPlatform)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func shouldSkipTokensProOverviewSync(settings dto.ChannelOtherSettings, now int64) bool {
	if settings.TokensProOverviewSyncEnabled == nil || !*settings.TokensProOverviewSyncEnabled {
		return true
	}
	if settings.TokensProOverviewLastSuccessTime <= 0 {
		return false
	}
	return now-settings.TokensProOverviewLastSuccessTime < tokensProOverviewSyncIntervalSeconds(settings)
}

func tokensProOverviewSyncIntervalSeconds(settings dto.ChannelOtherSettings) int64 {
	interval := tokensProOverviewDefaultIntervalSeconds
	if settings.TokensProOverviewSyncIntervalSeconds != nil && *settings.TokensProOverviewSyncIntervalSeconds > interval {
		interval = *settings.TokensProOverviewSyncIntervalSeconds
	}
	return int64(interval)
}

func decideTokensProOverviewApply(status int, disabledByZero bool, allowed int) (concurrency int, nextDisabledByZero bool, disable bool, enable bool) {
	concurrency = allowed
	if allowed == 0 {
		disable = status == common.ChannelStatusEnabled
		nextDisabledByZero = disabledByZero || disable
		return concurrency, nextDisabledByZero, disable, false
	}
	enable = disabledByZero && status == common.ChannelStatusAutoDisabled
	return concurrency, false, false, enable
}

func tokensProOverviewStatusError(status int, body []byte) error {
	preview := strings.TrimSpace(string(body))
	if len(preview) > tokensProOverviewMaxErrorBytes {
		preview = preview[:tokensProOverviewMaxErrorBytes]
	}
	if preview == "" {
		return fmt.Errorf("tokenspro overview HTTP %d", status)
	}
	return fmt.Errorf("tokenspro overview HTTP %d: %s", status, preview)
}

func fetchTokensProOverviewHTTP(ctx context.Context, channel *model.Channel, apiKey string) (int, []byte, error) {
	requestURL, err := buildTokensProOverviewURL(channel.GetBaseURL())
	if err != nil {
		return 0, nil, err
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, tokensProOverviewRequestTimeout)
		defer cancel()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return 0, nil, err
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Accept", "application/json")
	client, err := tokensProOverviewHTTPClient(channel)
	if err != nil {
		return 0, nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, tokensProOverviewMaxResponseBytes))
	if err != nil {
		return response.StatusCode, nil, err
	}
	return response.StatusCode, body, nil
}

func SyncChannelTokensProOverview(ctx context.Context, channel *model.Channel, now int64) (tokensProOverviewSyncResult, error) {
	if channel == nil {
		return tokensProOverviewSyncResult{}, errors.New("channel is nil")
	}
	if now <= 0 {
		now = common.GetTimestamp()
	}
	settings := channel.GetOtherSettings()
	if shouldSkipTokensProOverviewSync(settings, now) {
		return tokensProOverviewSyncResult{Skipped: true}, nil
	}

	key, keyErr := tokensProOverviewAPIKey(channel)
	if keyErr != nil {
		return persistTokensProOverviewError(channel, settings, keyErr.Error())
	}

	statusCode, body, err := fetchTokensProOverview(ctx, channel, key)
	if err != nil {
		return persistTokensProOverviewError(channel, settings, err.Error())
	}
	if statusCode != http.StatusOK {
		return persistTokensProOverviewError(channel, settings, tokensProOverviewStatusError(statusCode, body).Error())
	}

	allowed, err := parseTokensProOverviewAllowed(body)
	if err != nil {
		return persistTokensProOverviewError(channel, settings, err.Error())
	}

	limit, nextDisabledByZero, disable, enable := decideTokensProOverviewApply(channel.Status, settings.TokensProOverviewDisabledByZero, allowed)
	result := tokensProOverviewSyncResult{}
	if disable {
		if disableChannelForTokensProOverview(channel) {
			channel.Status = common.ChannelStatusAutoDisabled
			result.Disabled = true
		} else if channel.Status == common.ChannelStatusEnabled {
			return persistTokensProOverviewError(channel, settings, "failed to auto-disable for allowed=0")
		}
	}

	allowedCopy := allowed
	settings.TokensProOverviewLastAllowed = &allowedCopy
	settings.TokensProOverviewLastSuccessTime = now
	settings.TokensProOverviewLastError = ""
	if enable {
		settings.TokensProOverviewDisabledByZero = true
	} else {
		settings.TokensProOverviewDisabledByZero = nextDisabledByZero
	}
	if err = persistTokensProOverviewChannel(channel, settings, limit); err != nil {
		return tokensProOverviewSyncResult{}, err
	}

	if enable {
		if enableChannelForTokensProOverview(channel) {
			channel.Status = common.ChannelStatusEnabled
			settings.TokensProOverviewDisabledByZero = false
			if err = persistTokensProOverviewSettings(channel, settings); err != nil {
				return tokensProOverviewSyncResult{}, err
			}
			if common.MemoryCacheEnabled {
				model.InitChannelCache()
			}
			result.Enabled = true
		}
	}

	result.Allowed = &allowedCopy
	return result, nil
}

func persistTokensProOverviewError(channel *model.Channel, settings dto.ChannelOtherSettings, message string) (tokensProOverviewSyncResult, error) {
	settings.TokensProOverviewLastError = truncateTokensProOverviewError(message)
	if err := persistTokensProOverviewSettings(channel, settings); err != nil {
		return tokensProOverviewSyncResult{}, err
	}
	return tokensProOverviewSyncResult{Error: settings.TokensProOverviewLastError}, nil
}

func persistTokensProOverviewSettings(channel *model.Channel, settings dto.ChannelOtherSettings) error {
	channel.SetOtherSettings(settings)
	if err := model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("settings", channel.OtherSettings).Error; err != nil {
		return err
	}
	patchCachedTokensProOverview(channel)
	return nil
}

func persistTokensProOverviewChannel(channel *model.Channel, settings dto.ChannelOtherSettings, concurrency int) error {
	channel.SetOtherSettings(settings)
	limit := concurrency
	channel.ConcurrencyLimit = &limit
	if err := model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{
		"settings":          channel.OtherSettings,
		"concurrency_limit": limit,
	}).Error; err != nil {
		return err
	}
	patchCachedTokensProOverview(channel)
	return nil
}

func patchCachedTokensProOverview(channel *model.Channel) {
	if !common.MemoryCacheEnabled || channel == nil {
		return
	}
	cached, err := model.CacheGetChannel(channel.Id)
	if err != nil || cached == nil {
		return
	}
	cached.ConcurrencyLimit = channel.ConcurrencyLimit
	cached.OtherSettings = channel.OtherSettings
}

func applyTokensProOverviewDisable(channel *model.Channel) bool {
	success := model.UpdateChannelStatus(channel.Id, "", common.ChannelStatusAutoDisabled, tokensProOverviewZeroDisableReason)
	if success {
		enqueueChannelNotification(
			model.NotificationEventTypeChannelDisabled,
			channel.Id,
			channel.Name,
			tokensProOverviewZeroDisableReason,
			tokensProOverviewZeroDisableReason,
			"",
			0,
		)
	}
	return success
}

func applyTokensProOverviewEnable(channel *model.Channel) bool {
	success := model.UpdateChannelStatus(channel.Id, "", common.ChannelStatusEnabled, "")
	if success {
		enqueueChannelNotification(model.NotificationEventTypeChannelEnabled, channel.Id, channel.Name, "", "", "", 0)
	}
	return success
}

func tokensProOverviewAPIKey(channel *model.Channel) (string, error) {
	if channel == nil {
		return "", errors.New("channel is nil")
	}
	if !channel.ChannelInfo.IsMultiKey {
		key := strings.TrimSpace(channel.Key)
		if key == "" {
			return "", errors.New("tokenspro overview key is empty")
		}
		return key, nil
	}
	keys := channel.GetKeys()
	statusList := channel.ChannelInfo.MultiKeyStatusList
	for i, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		status := common.ChannelStatusEnabled
		if statusList != nil {
			if current, ok := statusList[i]; ok {
				status = current
			}
		}
		if status == common.ChannelStatusEnabled {
			return key, nil
		}
	}
	return "", errors.New("no enabled keys")
}

func truncateTokensProOverviewError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= 500 {
		return message
	}
	return message[:500]
}

func ShouldSkipMonitorAutoEnableForTokensProOverview(channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	settings := channel.GetOtherSettings()
	if settings.TokensProOverviewDisabledByZero {
		return true
	}
	if settings.TokensProOverviewSyncEnabled == nil || !*settings.TokensProOverviewSyncEnabled {
		return false
	}
	return settings.TokensProOverviewLastAllowed != nil && *settings.TokensProOverviewLastAllowed == 0
}

func RunTokensProOverviewSyncOnce(ctx context.Context, now int64, report func(processed, total int)) tokensProOverviewSyncSummary {
	if now <= 0 {
		now = common.GetTimestamp()
	}
	channels, err := model.GetAllChannels(0, 0, true, true)
	if err != nil {
		common.SysError(fmt.Sprintf("tokenspro overview sync failed to list channels: %v", err))
		return tokensProOverviewSyncSummary{}
	}
	summary := tokensProOverviewSyncSummary{Total: len(channels)}
	for index, channel := range channels {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		if report != nil {
			report(index, summary.Total)
		}
		if channel == nil {
			continue
		}
		result, syncErr := SyncChannelTokensProOverview(ctx, channel, now)
		if syncErr != nil {
			summary.Failed++
			common.SysError(fmt.Sprintf("tokenspro overview sync failed for channel #%d: %v", channel.Id, syncErr))
			continue
		}
		if result.Skipped {
			summary.Skipped++
			continue
		}
		if result.Error != "" {
			summary.Failed++
			continue
		}
		summary.Synced++
		if result.Disabled {
			summary.Disabled++
		}
		if result.Enabled {
			summary.Enabled++
		}
	}
	if report != nil {
		report(summary.Total, summary.Total)
	}
	return summary
}
