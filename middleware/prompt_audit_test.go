package middleware

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPromptAuditHTTPStopsRelaySideEffectsBeforeNextMiddleware(t *testing.T) {
	tests := []struct {
		name          string
		guardStatus   int
		guardContent  string
		wantStatus    int
		wantCode      string
		wantNextCalls int64
	}{
		{
			name: "安全请求继续", guardStatus: http.StatusOK,
			guardContent: "Safety: Safe\nCategories: None", wantStatus: http.StatusNoContent,
			wantNextCalls: 1,
		},
		{
			name: "风险请求在渠道前阻断", guardStatus: http.StatusOK,
			guardContent: "Safety: Unsafe\nCategories: Jailbreak", wantStatus: http.StatusForbidden,
			wantCode: service.PromptGuardBlockedCode,
		},
		{
			name: "Guard 不可用在渠道前失败", guardStatus: http.StatusInternalServerError,
			wantStatus: http.StatusServiceUnavailable, wantCode: service.PromptGuardUnavailableCode,
		},
		{
			name: "Guard 非法响应在渠道前失败", guardStatus: http.StatusOK,
			guardContent: "invalid guard response", wantStatus: http.StatusServiceUnavailable,
			wantCode: service.PromptGuardInvalidResponseCode,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if test.guardStatus != http.StatusOK {
					http.Error(w, "guard unavailable", test.guardStatus)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, test.guardContent)
			}))
			defer guard.Close()
			setupPromptAuditHTTPTestDB(t, guard.URL)

			var nextCalls atomic.Int64
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.POST("/v1/chat/completions",
				func(c *gin.Context) {
					common.SetContextKey(c, constant.ContextKeyUserId, 10)
					common.SetContextKey(c, constant.ContextKeyUserGroupId, 1)
					common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
					common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
					common.SetContextKey(c, constant.ContextKeyTokenGroupMode, "inherit")
					common.SetContextKey(c, constant.ContextKeyTokenId, 20)
					c.Next()
				},
				PromptAudit(),
				func(c *gin.Context) {
					nextCalls.Add(1)
					c.Status(http.StatusNoContent)
				},
			)
			body := []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"audit this prompt"}]}`)
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			require.Equal(t, test.wantStatus, recorder.Code)
			require.Equal(t, test.wantNextCalls, nextCalls.Load())
			if test.wantCode != "" {
				require.Contains(t, recorder.Body.String(), test.wantCode)
			}
		})
	}
}

func TestPromptAuditBinaryBodyIsNotTreatedAsJSON(t *testing.T) {
	var guardCalls atomic.Int64
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		guardCalls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer guard.Close()
	setupPromptAuditHTTPTestDB(t, guard.URL)

	// 音频/图片等原始二进制正文不属于文本 Guard 范围；即使内容不是
	// JSON，也必须原样交给后续渠道，而不是在门禁阶段返回 400。
	body := []byte{0xff, 0xd8, 0x00, 0x01, 0x7f, 0x80, 0x10}
	var downstreamBody []byte
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/v1/audio/transcriptions",
		func(c *gin.Context) {
			common.SetContextKey(c, constant.ContextKeyUserId, 10)
			common.SetContextKey(c, constant.ContextKeyUserGroupId, 1)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
			common.SetContextKey(c, constant.ContextKeyTokenGroupMode, "inherit")
			c.Next()
		},
		PromptAudit(),
		func(c *gin.Context) {
			var err error
			downstreamBody, err = io.ReadAll(c.Request.Body)
			require.NoError(t, err)
			c.Status(http.StatusNoContent)
		},
	)
	request := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", bytes.NewReader(body))
	request.Header.Set("Content-Type", "audio/mpeg")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusNoContent, response.Code)
	require.Equal(t, body, downstreamBody)
	require.Zero(t, guardCalls.Load())
}

func TestPromptAuditMultipartRefTextIsAudited(t *testing.T) {
	var guardCalls atomic.Int64
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		guardCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"Safety: Unsafe\nCategories: Jailbreak"}}]}`)
	}))
	defer guard.Close()
	setupPromptAuditHTTPTestDB(t, guard.URL)

	var downstreamCalls atomic.Int64
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/v1/audio/speech",
		func(c *gin.Context) {
			common.SetContextKey(c, constant.ContextKeyUserId, 10)
			common.SetContextKey(c, constant.ContextKeyUserGroupId, 1)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
			common.SetContextKey(c, constant.ContextKeyTokenGroupMode, "inherit")
			c.Next()
		},
		PromptAudit(),
		func(c *gin.Context) {
			downstreamCalls.Add(1)
			c.Status(http.StatusNoContent)
		},
	)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "tts-test"))
	require.NoError(t, writer.WriteField("ref_text", "音色参考风险文本"))
	filePart, err := writer.CreateFormFile("file", "reference.wav")
	require.NoError(t, err)
	_, err = filePart.Write([]byte{0x52, 0x49, 0x46, 0x46, 0x00, 0xff})
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	request := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusForbidden, response.Code)
	require.Contains(t, response.Body.String(), service.PromptGuardBlockedCode)
	require.EqualValues(t, 1, guardCalls.Load())
	require.Zero(t, downstreamCalls.Load())
}

func setupPromptAuditHTTPTestDB(t *testing.T, guardURL string) {
	t.Helper()
	oldDB := model.DB
	oldSecret := common.CryptoSecret
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "prompt-audit-http.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.PromptAuditConfig{}, &model.PromptAuditEndpoint{}, &model.PromptAuditJob{},
		&model.PromptAuditEvent{}, &model.PromptAuditQueueState{},
		&model.RequestArchiveConfig{}, &model.RequestArchiveTarget{}, &model.RequestArchiveJob{},
		&model.RequestArchiveQueueState{},
	))
	model.DB = db
	t.Setenv("CRYPTO_SECRET", "stable-http-test-secret")
	common.CryptoSecret = "stable-http-test-secret"
	require.NoError(t, model.EnsurePromptAuditDefaults())
	require.NoError(t, model.EnsureRequestArchiveDefaults())
	cfg, _, err := model.LoadPromptAuditConfig()
	require.NoError(t, err)
	cfg.Enabled = true
	cfg.BlockingEnabled = true
	require.NoError(t, model.SavePromptAuditConfig(cfg.ConfigVersion, cfg, []model.PromptAuditEndpoint{{
		Id: "guard-http", Name: "Guard HTTP", Protocol: "openai_compatible",
		BaseUrl: guardURL, Model: service.PromptAuditDefaultModel,
		TimeoutMs: 1000, InputLimit: service.PromptAuditDefaultInputLimit, Enabled: true,
	}}))
	service.InvalidatePromptAuditConfig()
	service.InvalidateRequestArchiveConfig()
	t.Cleanup(func() {
		service.InvalidatePromptAuditConfig()
		service.InvalidateRequestArchiveConfig()
		common.CryptoSecret = oldSecret
		model.DB = oldDB
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
}

func TestPromptAuditQueuesRawRequestArchiveBeforeGuardBlock(t *testing.T) {
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"Safety: Unsafe\nCategories: Jailbreak"}}]}`)
	}))
	defer guard.Close()
	setupPromptAuditHTTPTestDB(t, guard.URL)

	archiveConfig, err := service.GetRequestArchiveConfig(context.Background())
	require.NoError(t, err)
	_, err = service.SaveRequestArchiveConfig(context.Background(), service.RequestArchiveUpdateRequest{
		ExpectedConfigVersion: archiveConfig.ConfigVersion,
		Enabled:               true,
		ActiveTargetId:        "archive-test",
		RetentionDays:         30,
		WorkerCount:           1,
		QueueCapacity:         16,
		MaxBodyBytes:          model.RequestArchiveDefaultMaxBodyBytes,
		QueueMaxBytes:         model.RequestArchiveDefaultQueueMaxBytes,
		Targets: []service.RequestArchiveUpdateTarget{{
			Id: "archive-test", Name: "测试归档", Type: model.RequestArchiveTargetLocal,
			Enabled: true, LocalPath: requestArchiveMiddlewareTestLocalPath(t, "archive"),
		}},
	}, 1)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/v1/chat/completions",
		func(c *gin.Context) {
			common.SetContextKey(c, constant.ContextKeyUserId, 10)
			common.SetContextKey(c, constant.ContextKeyUserGroupId, 1)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
			common.SetContextKey(c, constant.ContextKeyTokenGroupMode, "inherit")
			c.Next()
		},
		PromptAudit(),
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)
	body := []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"this raw request must be archived"}]}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?secret=not-stored", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusForbidden, response.Code)

	var job model.RequestArchiveJob
	require.NoError(t, model.DB.First(&job).Error)
	plain, err := service.DecryptRequestArchivePayload(&job)
	require.NoError(t, err)
	require.Equal(t, body, plain)
	require.NotContains(t, job.Path, "secret")
	var event model.PromptAuditEvent
	require.NoError(t, model.DB.First(&event).Error)
	require.Equal(t, "gpt-test", event.Model)
}
