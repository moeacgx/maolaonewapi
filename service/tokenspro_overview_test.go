package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestParseTokensProOverviewAllowed(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		want      int
		wantError string
	}{
		{
			name: "reads allowed and ignores available",
			body: `{"platform":"openai","generated_at":"2026-09-10T00:00:00Z","concurrency":{"allowed":4,"in_flight":1,"available":3},"balance":{"unused":true}}`,
			want: 4,
		},
		{
			name: "allows zero",
			body: `{"concurrency":{"allowed":0,"available":0}}`,
			want: 0,
		},
		{
			name:      "rejects missing concurrency",
			body:      `{"platform":"openai"}`,
			wantError: "concurrency.allowed",
		},
		{
			name:      "rejects missing allowed",
			body:      `{"concurrency":{"available":3}}`,
			wantError: "concurrency.allowed",
		},
		{
			name:      "rejects fractional allowed",
			body:      `{"concurrency":{"allowed":4.5}}`,
			wantError: "concurrency.allowed",
		},
		{
			name:      "rejects negative allowed",
			body:      `{"concurrency":{"allowed":-1}}`,
			wantError: "concurrency.allowed",
		},
		{
			name:      "rejects invalid json",
			body:      `{"concurrency":`,
			wantError: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, err := parseTokensProOverviewAllowed([]byte(tt.body))
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, allowed)
		})
	}
}

func TestBuildTokensProOverviewURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
		wantErr bool
	}{
		{
			name:    "origin without slash",
			baseURL: "https://tokens.example.com",
			want:    "https://tokens.example.com/api/v1/downstream/overview?platform=openai",
		},
		{
			name:    "origin with slash",
			baseURL: "https://tokens.example.com/",
			want:    "https://tokens.example.com/api/v1/downstream/overview?platform=openai",
		},
		{
			name:    "strips trailing openai v1 prefix",
			baseURL: "https://tokens.example.com/v1",
			want:    "https://tokens.example.com/api/v1/downstream/overview?platform=openai",
		},
		{
			name:    "strips trailing openai v1 prefix with slash",
			baseURL: "https://tokens.example.com/v1/",
			want:    "https://tokens.example.com/api/v1/downstream/overview?platform=openai",
		},
		{
			name:    "empty base url",
			baseURL: "  ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildTokensProOverviewURL(tt.baseURL)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestShouldSkipTokensProOverviewSync(t *testing.T) {
	enabled := true
	interval := 120
	now := int64(1_000_000)

	assert.True(t, shouldSkipTokensProOverviewSync(dto.ChannelOtherSettings{}, now))
	assert.True(t, shouldSkipTokensProOverviewSync(dto.ChannelOtherSettings{TokensProOverviewSyncEnabled: new(bool)}, now))
	assert.False(t, shouldSkipTokensProOverviewSync(dto.ChannelOtherSettings{
		TokensProOverviewSyncEnabled: &enabled,
	}, now))
	assert.True(t, shouldSkipTokensProOverviewSync(dto.ChannelOtherSettings{
		TokensProOverviewSyncEnabled:     &enabled,
		TokensProOverviewLastSuccessTime: now - 30,
	}, now))
	assert.False(t, shouldSkipTokensProOverviewSync(dto.ChannelOtherSettings{
		TokensProOverviewSyncEnabled:     &enabled,
		TokensProOverviewLastSuccessTime: now - 90,
	}, now))
	assert.True(t, shouldSkipTokensProOverviewSync(dto.ChannelOtherSettings{
		TokensProOverviewSyncEnabled:         &enabled,
		TokensProOverviewSyncIntervalSeconds: &interval,
		TokensProOverviewLastSuccessTime:     now - 90,
	}, now))
	assert.False(t, shouldSkipTokensProOverviewSync(dto.ChannelOtherSettings{
		TokensProOverviewSyncEnabled:         &enabled,
		TokensProOverviewSyncIntervalSeconds: &interval,
		TokensProOverviewLastSuccessTime:     now - 180,
	}, now))
}

func TestDecideTokensProOverviewApply(t *testing.T) {
	tests := []struct {
		name           string
		status         int
		disabledByZero bool
		allowed        int
		wantLimit      int
		wantMarker     bool
		wantDisable    bool
		wantEnable     bool
	}{
		{
			name:       "writes allowed on enabled channel",
			status:     common.ChannelStatusEnabled,
			allowed:    4,
			wantLimit:  4,
			wantMarker: false,
		},
		{
			name:        "zero disables enabled channel",
			status:      common.ChannelStatusEnabled,
			allowed:     0,
			wantLimit:   0,
			wantMarker:  true,
			wantDisable: true,
		},
		{
			name:           "restores only overview auto-disabled channels",
			status:         common.ChannelStatusAutoDisabled,
			disabledByZero: true,
			allowed:        6,
			wantLimit:      6,
			wantEnable:     true,
		},
		{
			name:       "does not enable monitor auto-disabled channels",
			status:     common.ChannelStatusAutoDisabled,
			allowed:    6,
			wantLimit:  6,
			wantEnable: false,
		},
		{
			name:       "does not enable manually disabled channels",
			status:     common.ChannelStatusManuallyDisabled,
			allowed:    6,
			wantLimit:  6,
			wantEnable: false,
		},
		{
			name:           "clears marker on manual disable without enabling",
			status:         common.ChannelStatusManuallyDisabled,
			disabledByZero: true,
			allowed:        3,
			wantLimit:      3,
			wantEnable:     false,
		},
		{
			name:       "zero on already auto-disabled does not claim the marker",
			status:     common.ChannelStatusAutoDisabled,
			allowed:    0,
			wantLimit:  0,
			wantMarker: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, marker, disable, enable := decideTokensProOverviewApply(tt.status, tt.disabledByZero, tt.allowed)
			assert.Equal(t, tt.wantLimit, limit)
			assert.Equal(t, tt.wantMarker, marker)
			assert.Equal(t, tt.wantDisable, disable)
			assert.Equal(t, tt.wantEnable, enable)
		})
	}
}

func TestSyncChannelTokensProOverviewWritesAllowedAndKeepsFailuresUnchanged(t *testing.T) {
	channel := setupTokensProOverviewTestDB(t)
	enabled := true
	limit := 8
	channel.ConcurrencyLimit = &limit
	channel.Status = common.ChannelStatusEnabled
	channel.SetOtherSettings(dto.ChannelOtherSettings{TokensProOverviewSyncEnabled: &enabled})
	require.NoError(t, model.DB.Save(channel).Error)

	originalFetch := fetchTokensProOverview
	t.Cleanup(func() { fetchTokensProOverview = originalFetch })

	t.Run("success writes allowed not available", func(t *testing.T) {
		fetchTokensProOverview = func(context.Context, *model.Channel, string) (int, []byte, error) {
			return http.StatusOK, []byte(`{"concurrency":{"allowed":4,"available":1}}`), nil
		}
		result, err := SyncChannelTokensProOverview(context.Background(), reloadTokensProOverviewChannel(t, channel.Id), 1_000)
		require.NoError(t, err)
		assert.False(t, result.Skipped)
		require.NotNil(t, result.Allowed)
		assert.Equal(t, 4, *result.Allowed)

		stored := reloadTokensProOverviewChannel(t, channel.Id)
		assert.Equal(t, 4, stored.GetConcurrencyLimit())
		assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
		settings := stored.GetOtherSettings()
		require.NotNil(t, settings.TokensProOverviewLastAllowed)
		assert.Equal(t, 4, *settings.TokensProOverviewLastAllowed)
		assert.Equal(t, int64(1_000), settings.TokensProOverviewLastSuccessTime)
		assert.Empty(t, settings.TokensProOverviewLastError)
	})

	t.Run("non-200 does not change concurrency or status", func(t *testing.T) {
		before := reloadTokensProOverviewChannel(t, channel.Id)
		fetchTokensProOverview = func(context.Context, *model.Channel, string) (int, []byte, error) {
			return http.StatusForbidden, []byte(`{"error":"missing downstream:read"}`), nil
		}
		result, err := SyncChannelTokensProOverview(context.Background(), before, 1_100)
		require.NoError(t, err)
		assert.NotEmpty(t, result.Error)

		stored := reloadTokensProOverviewChannel(t, channel.Id)
		assert.Equal(t, 4, stored.GetConcurrencyLimit())
		assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
		settings := stored.GetOtherSettings()
		require.NotNil(t, settings.TokensProOverviewLastAllowed)
		assert.Equal(t, 4, *settings.TokensProOverviewLastAllowed)
		assert.Equal(t, int64(1_000), settings.TokensProOverviewLastSuccessTime)
		assert.Contains(t, settings.TokensProOverviewLastError, "403")
	})

	t.Run("allowed zero auto-disables and later restores", func(t *testing.T) {
		fetchTokensProOverview = func(context.Context, *model.Channel, string) (int, []byte, error) {
			return http.StatusOK, []byte(`{"concurrency":{"allowed":0,"available":0}}`), nil
		}
		_, err := SyncChannelTokensProOverview(context.Background(), reloadTokensProOverviewChannel(t, channel.Id), 1_200)
		require.NoError(t, err)
		stored := reloadTokensProOverviewChannel(t, channel.Id)
		assert.Equal(t, 0, stored.GetConcurrencyLimit())
		assert.Equal(t, common.ChannelStatusAutoDisabled, stored.Status)
		assert.True(t, stored.GetOtherSettings().TokensProOverviewDisabledByZero)

		fetchTokensProOverview = func(context.Context, *model.Channel, string) (int, []byte, error) {
			return http.StatusOK, []byte(`{"concurrency":{"allowed":5}}`), nil
		}
		_, err = SyncChannelTokensProOverview(context.Background(), reloadTokensProOverviewChannel(t, channel.Id), 1_300)
		require.NoError(t, err)
		stored = reloadTokensProOverviewChannel(t, channel.Id)
		assert.Equal(t, 5, stored.GetConcurrencyLimit())
		assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
		assert.False(t, stored.GetOtherSettings().TokensProOverviewDisabledByZero)
	})

	t.Run("does not enable manually disabled channels", func(t *testing.T) {
		require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("status", common.ChannelStatusManuallyDisabled).Error)
		settings := reloadTokensProOverviewChannel(t, channel.Id).GetOtherSettings()
		settings.TokensProOverviewDisabledByZero = true
		reload := reloadTokensProOverviewChannel(t, channel.Id)
		reload.SetOtherSettings(settings)
		require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("settings", reload.OtherSettings).Error)

		fetchTokensProOverview = func(context.Context, *model.Channel, string) (int, []byte, error) {
			return http.StatusOK, []byte(`{"concurrency":{"allowed":7}}`), nil
		}
		_, err := SyncChannelTokensProOverview(context.Background(), reloadTokensProOverviewChannel(t, channel.Id), 1_400)
		require.NoError(t, err)
		stored := reloadTokensProOverviewChannel(t, channel.Id)
		assert.Equal(t, 7, stored.GetConcurrencyLimit())
		assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
		assert.False(t, stored.GetOtherSettings().TokensProOverviewDisabledByZero)
	})
}

func TestFetchTokensProOverviewHTTPUsesBearerAndOverviewPath(t *testing.T) {
	var gotAuth string
	var gotPath string
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_, _ = io.WriteString(w, `{"concurrency":{"allowed":2}}`)
	}))
	t.Cleanup(server.Close)

	originalClient := tokensProOverviewHTTPClient
	t.Cleanup(func() { tokensProOverviewHTTPClient = originalClient })
	tokensProOverviewHTTPClient = func(*model.Channel) (*http.Client, error) {
		return server.Client(), nil
	}

	baseURL := server.URL
	channel := &model.Channel{Key: "tp-key", BaseURL: &baseURL}
	status, body, err := fetchTokensProOverviewHTTP(context.Background(), channel, "tp-key")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, string(body), `"allowed":2`)
	assert.Equal(t, "Bearer tp-key", gotAuth)
	assert.Equal(t, "/api/v1/downstream/overview", gotPath)
	assert.Equal(t, "platform=openai", gotQuery)
}

func TestShouldSkipMonitorAutoEnableForTokensProOverview(t *testing.T) {
	assert.False(t, ShouldSkipMonitorAutoEnableForTokensProOverview(nil))
	channel := &model.Channel{}
	assert.False(t, ShouldSkipMonitorAutoEnableForTokensProOverview(channel))
	channel.SetOtherSettings(dto.ChannelOtherSettings{TokensProOverviewDisabledByZero: true})
	assert.True(t, ShouldSkipMonitorAutoEnableForTokensProOverview(channel))
}

func setupTokensProOverviewTestDB(t *testing.T) *model.Channel {
	t.Helper()

	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMainType := common.MainDatabaseType()
	oldLogType := common.LogDatabaseType()
	oldMemoryCache := common.MemoryCacheEnabled
	oldRedisEnabled := common.RedisEnabled

	dsn := "file:" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	model.DB = db
	model.LOG_DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	model.InitDBColumns()
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false

	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		model.InitDBColumns()
		common.MemoryCacheEnabled = oldMemoryCache
		common.RedisEnabled = oldRedisEnabled
		_ = sqlDB.Close()
	})

	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}, &model.Log{}))
	baseURL := "https://tokens.example.com"
	limit := 8
	channel := &model.Channel{
		Name:             "tokenspro-channel",
		Key:              "tokenspro-key",
		Type:             constant.ChannelTypeOpenAI,
		Models:           "gpt-4o",
		Group:            "default",
		Status:           common.ChannelStatusEnabled,
		BaseURL:          &baseURL,
		ConcurrencyLimit: &limit,
	}
	require.NoError(t, db.Create(channel).Error)
	return channel
}

func reloadTokensProOverviewChannel(t *testing.T, id int) *model.Channel {
	t.Helper()
	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, id).Error)
	return &stored
}

func TestTokensProOverviewRequestErrorMessageIncludesStatus(t *testing.T) {
	err := tokensProOverviewStatusError(http.StatusUnauthorized, []byte(`nope`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}
