package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestProbePromptAuditEndpointHonorsExplicitTokenAction(t *testing.T) {
	authorizations := make(chan string, 3)
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			authorizations <- r.Header.Get("Authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`))
	}))
	defer guard.Close()
	setupPromptAuditControllerTestDB(t, guard.URL)

	tests := []struct {
		name              string
		tokenAction       string
		token             string
		wantAuthorization string
	}{
		{name: "keep 复用已保存令牌", tokenAction: service.PromptAuditTokenKeep, wantAuthorization: "Bearer stored-token"},
		{name: "clear 明确不发送令牌", tokenAction: service.PromptAuditTokenClear},
		{name: "replace 使用请求令牌", tokenAction: service.PromptAuditTokenReplace, token: "probe-token", wantAuthorization: "Bearer probe-token"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload, err := common.Marshal(map[string]interface{}{
				"endpoint_id": "guard-controller", "token_action": test.tokenAction, "token": test.token,
			})
			require.NoError(t, err)
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.POST("/probe", func(c *gin.Context) {
				c.Set("id", 1)
				c.Set("username", "root")
				ProbePromptAuditEndpoint(c)
			})
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/probe", bytes.NewReader(payload))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
			select {
			case authorization := <-authorizations:
				require.Equal(t, test.wantAuthorization, authorization)
			case <-time.After(2 * time.Second):
				t.Fatal("Guard 节点未收到探测请求")
			}
			require.NotContains(t, recorder.Body.String(), "stored-token")
			require.NotContains(t, recorder.Body.String(), "probe-token")
		})
	}
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Find(&logs).Error)
	serializedLogs, err := common.Marshal(logs)
	require.NoError(t, err)
	require.NotContains(t, string(serializedLogs), "stored-token")
	require.NotContains(t, string(serializedLogs), "probe-token")
	require.NotContains(t, string(serializedLogs), "Bearer ")
}

func TestProbePromptAuditEndpointCanReplaceUnreadableTokenAfterSecretRotation(t *testing.T) {
	authorizations := make(chan string, 2)
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			authorizations <- r.Header.Get("Authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`))
	}))
	defer guard.Close()
	setupPromptAuditControllerTestDB(t, guard.URL)
	common.CryptoSecret = "rotated-controller-test-secret"
	service.InvalidatePromptAuditConfig()

	payload, err := common.Marshal(map[string]interface{}{
		"endpoint_id": "guard-controller", "token_action": service.PromptAuditTokenReplace, "token": "replacement-token",
	})
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/probe", func(c *gin.Context) {
		c.Set("id", 1)
		c.Set("username", "root")
		ProbePromptAuditEndpoint(c)
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/probe", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	select {
	case authorization := <-authorizations:
		require.Equal(t, "Bearer replacement-token", authorization)
	case <-time.After(2 * time.Second):
		t.Fatal("Guard 节点未收到密钥轮换后的探测请求")
	}
}

func TestProbePromptAuditEndpointRetainsReadableTokenWithMixedEndpointSecrets(t *testing.T) {
	authorizations := make(chan string, 1)
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			authorizations <- r.Header.Get("Authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`))
	}))
	defer guard.Close()
	setupPromptAuditControllerTestDB(t, guard.URL)

	readableCiphertext, err := service.EncryptPromptAuditSecret("readable-token")
	require.NoError(t, err)
	cfg, endpoints, err := model.LoadPromptAuditConfig()
	require.NoError(t, err)
	require.Len(t, endpoints, 1)
	require.NoError(t, model.SavePromptAuditConfig(cfg.ConfigVersion, cfg, []model.PromptAuditEndpoint{
		{
			Id: "guard-readable", Name: "Readable Guard", Protocol: "openai_compatible",
			BaseUrl: guard.URL, Model: service.PromptAuditDefaultModel, TokenCiphertext: readableCiphertext,
			TimeoutMs: 1000, InputLimit: service.PromptAuditDefaultInputLimit, Enabled: true,
		},
		{
			Id: "guard-unreadable", Name: "Unreadable Guard", Protocol: "openai_compatible",
			BaseUrl: guard.URL, Model: service.PromptAuditDefaultModel, TokenCiphertext: "v1:not-a-valid-ciphertext",
			TimeoutMs: 1000, InputLimit: service.PromptAuditDefaultInputLimit, Enabled: true,
		},
	}))
	service.InvalidatePromptAuditConfig()

	payload, err := common.Marshal(map[string]interface{}{
		"endpoint_id": "guard-readable", "token_action": service.PromptAuditTokenKeep,
	})
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/probe", func(c *gin.Context) {
		c.Set("id", 1)
		c.Set("username", "root")
		ProbePromptAuditEndpoint(c)
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/probe", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	select {
	case authorization := <-authorizations:
		require.Equal(t, "Bearer readable-token", authorization)
	case <-time.After(2 * time.Second):
		t.Fatal("可解密 Guard 节点未收到探测请求")
	}
}

func TestProbePromptAuditEndpointDoesNotReuseTokenForChangedBaseURL(t *testing.T) {
	requests := make(chan struct{}, 1)
	original := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`))
	}))
	defer original.Close()
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests <- struct{}{}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`))
	}))
	defer attacker.Close()
	setupPromptAuditControllerTestDB(t, original.URL)

	payload, err := common.Marshal(map[string]interface{}{
		"endpoint_id": "guard-controller", "base_url": attacker.URL, "token_action": service.PromptAuditTokenKeep,
	})
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/probe", func(c *gin.Context) {
		c.Set("id", 1)
		c.Set("username", "root")
		ProbePromptAuditEndpoint(c)
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/probe", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "prompt_audit_endpoint_token_action_required")
	select {
	case <-requests:
		t.Fatal("地址变化且 keep 旧令牌时不应向新 Guard 地址发起请求")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestProbePromptAuditEndpointRejectsExplicitOutOfRangeNumbersBeforeNetwork(t *testing.T) {
	var requests atomic.Int64
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer guard.Close()
	setupPromptAuditControllerTestDB(t, guard.URL)

	tests := []struct {
		name  string
		field string
		value int
	}{
		{name: "超时负数", field: "timeout_ms", value: -1},
		{name: "超时显式为零", field: "timeout_ms", value: 0},
		{name: "超时低于下限", field: "timeout_ms", value: 99},
		{name: "超时高于上限", field: "timeout_ms", value: 30001},
		{name: "分片负数", field: "input_limit", value: -1},
		{name: "分片显式为零", field: "input_limit", value: 0},
		{name: "分片低于下限", field: "input_limit", value: 127},
		{name: "分片高于上限", field: "input_limit", value: 100001},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload, err := common.Marshal(map[string]interface{}{
				"endpoint_id": "guard-controller", "token_action": service.PromptAuditTokenKeep,
				test.field: test.value,
			})
			require.NoError(t, err)
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.POST("/probe", func(c *gin.Context) {
				c.Set("id", 1)
				c.Set("username", "root")
				ProbePromptAuditEndpoint(c)
			})
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/probe", bytes.NewReader(payload))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Contains(t, recorder.Body.String(), "prompt_audit_endpoint_invalid")
		})
	}
	require.Zero(t, requests.Load(), "参数校验失败时不应向 Guard 节点发起请求")
}

func TestPromptAuditEventListItemSerializesChannelSnapshot(t *testing.T) {
	item := promptAuditEventListItem{
		PromptAuditEvent: model.PromptAuditEvent{
			Id: 9, GroupCode: "vip", ChannelId: 42, ChannelName: "最终渠道", ChannelGroupDetails: `[{"id":7}]`,
			ChannelGroups: []model.PromptAuditEventChannelGroup{{Id: 7, Code: "vip", Name: "贵宾分组"}},
		},
		Categories:             []string{},
		MatchedScanners:        []string{},
		UnknownCategories:      []string{},
		UserCyberPolicyCount:   6,
		CyberPolicyWindowHours: 720,
	}
	encoded, err := common.Marshal(item)
	require.NoError(t, err)
	var payload map[string]interface{}
	require.NoError(t, common.Unmarshal(encoded, &payload))
	require.EqualValues(t, 42, payload["channel_id"])
	require.Equal(t, "vip", payload["group_code"])
	require.Equal(t, "最终渠道", payload["channel_name"])
	require.EqualValues(t, 6, payload["user_cyber_policy_count"])
	require.EqualValues(t, 720, payload["cyber_policy_window_hours"])
	groups, ok := payload["channel_groups"].([]interface{})
	require.True(t, ok)
	require.Len(t, groups, 1)
	_, exposed := payload["channel_group_details"]
	require.False(t, exposed)
}

func TestPromptAuditFilterFromQueryNormalizesUsername(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodGet, "/events?username=%20Alice.Admin%20&user_id=17", nil)

	filter, err := promptAuditFilterFromQuery(context)
	require.NoError(t, err)
	require.Equal(t, "alice.admin", filter.Username)
	require.Equal(t, 17, filter.UserId)
}

func TestPromptAuditFilterRequestNormalizesUsername(t *testing.T) {
	filter, err := (promptAuditEventFilterRequest{Username: "  ALICE.Admin  "}).toModel()
	require.NoError(t, err)
	require.Equal(t, "alice.admin", filter.Username)

	_, err = (promptAuditEventFilterRequest{Username: strings.Repeat("用", 129)}).toModel()
	require.ErrorContains(t, err, "不能超过 128 个字符")
}

func TestPromptAuditFiltersNormalizeAction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodGet, "/events?action=%20Block%20", nil)

	filter, err := promptAuditFilterFromQuery(context)
	require.NoError(t, err)
	require.Equal(t, "block", filter.Action)

	filter, err = (promptAuditEventFilterRequest{Action: "  Mask  "}).toModel()
	require.NoError(t, err)
	require.Equal(t, "mask", filter.Action)
}

func setupPromptAuditControllerTestDB(t *testing.T, guardURL string) {
	t.Helper()
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldSecret := common.CryptoSecret
	oldRedisEnabled := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "prompt-audit-controller.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Log{}, &model.PromptAuditConfig{}, &model.PromptAuditEndpoint{},
		&model.PromptAuditJob{}, &model.PromptAuditEvent{}, &model.PromptAuditQueueState{},
	))
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled = false
	t.Setenv("CRYPTO_SECRET", "stable-controller-test-secret")
	common.CryptoSecret = "stable-controller-test-secret"
	require.NoError(t, model.EnsurePromptAuditDefaults())
	ciphertext, err := service.EncryptPromptAuditSecret("stored-token")
	require.NoError(t, err)
	cfg, _, err := model.LoadPromptAuditConfig()
	require.NoError(t, err)
	require.NoError(t, model.SavePromptAuditConfig(cfg.ConfigVersion, cfg, []model.PromptAuditEndpoint{{
		Id: "guard-controller", Name: "Guard Controller", Protocol: "openai_compatible",
		BaseUrl: guardURL, Model: service.PromptAuditDefaultModel, TokenCiphertext: ciphertext,
		TimeoutMs: 1000, InputLimit: service.PromptAuditDefaultInputLimit, Enabled: true,
	}}))
	service.InvalidatePromptAuditConfig()
	t.Cleanup(func() {
		service.InvalidatePromptAuditConfig()
		common.CryptoSecret = oldSecret
		common.RedisEnabled = oldRedisEnabled
		model.DB, model.LOG_DB = oldDB, oldLogDB
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
}
