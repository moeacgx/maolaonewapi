package middleware

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPromptAuditRealtimeBlocksBeforeNextMiddleware(t *testing.T) {
	tests := []struct {
		name             string
		guardContent     string
		wantNext         int64
		wantCloseCode    int
		wantErrorCode    string
		wantAcknowledged bool
	}{
		{
			name: "安全首帧继续进入渠道链路", guardContent: "Safety: Safe\nCategories: None",
			wantNext: 1, wantAcknowledged: true,
		},
		{
			name: "风险首帧在渠道前阻断", guardContent: "Safety: Unsafe\nCategories: Jailbreak",
			wantCloseCode: 4403, wantErrorCode: service.PromptGuardBlockedCode,
		},
		{
			name: "非法 Guard 响应在渠道前失败", guardContent: "not-a-valid-guard-response",
			wantCloseCode: websocket.CloseTryAgainLater, wantErrorCode: service.PromptGuardInvalidResponseCode,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, test.guardContent)
			}))
			defer guard.Close()
			setupPromptAuditRealtimeTestDB(t, guard.URL)

			var nextCalls atomic.Int64
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.GET("/v1/realtime",
				func(c *gin.Context) {
					common.SetContextKey(c, constant.ContextKeyUserId, 10)
					common.SetContextKey(c, constant.ContextKeyUserGroupId, 1)
					common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
					common.SetContextKey(c, constant.ContextKeyTokenId, 20)
					c.Next()
				},
				PromptAuditRealtime(),
				func(c *gin.Context) {
					nextCalls.Add(1)
					realtimeActive := common.GetContextKeyBool(c, constant.ContextKeyPromptAuditRealtimeActive)
					require.True(t, realtimeActive)
					conn, ok := common.GetContextKeyType[*websocket.Conn](c, constant.ContextKeyPromptAuditRealtimeClientWs)
					require.True(t, ok)
					frames, ok := common.GetContextKeyType[service.PromptAuditRealtimeFrames](c, constant.ContextKeyPromptAuditRealtimeBufferedFrames)
					require.True(t, ok)
					require.Len(t, frames, 1)
					require.Contains(t, string(frames[0].Payload), "first realtime prompt")
					_ = conn.WriteJSON(map[string]string{"type": "audit.ack"})
				},
			)
			server := httptest.NewServer(router)
			defer server.Close()

			dialer := websocket.Dialer{Subprotocols: []string{"realtime"}}
			conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/realtime?model=gpt-realtime", nil)
			require.NoError(t, err)
			defer conn.Close()
			require.NoError(t, conn.WriteJSON(map[string]interface{}{
				"type":    "session.update",
				"session": map[string]string{"instructions": "first realtime prompt"},
			}))

			_, payload, readErr := conn.ReadMessage()
			if test.wantAcknowledged {
				require.NoError(t, readErr)
				require.Contains(t, string(payload), "audit.ack")
				require.Equal(t, test.wantNext, nextCalls.Load())
				return
			}
			require.NoError(t, readErr)
			require.Contains(t, string(payload), test.wantErrorCode)
			_, _, readErr = conn.ReadMessage()
			var closeErr *websocket.CloseError
			require.ErrorAs(t, readErr, &closeErr)
			require.Equal(t, test.wantCloseCode, closeErr.Code)
			require.Zero(t, nextCalls.Load())
		})
	}
}

func TestPromptAuditRealtimeGuardOffStillRunsSensitiveRuleBeforeDistribution(t *testing.T) {
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer guard.Close()
	setupPromptAuditRealtimeTestDB(t, guard.URL)
	cfg, endpoints, err := model.LoadPromptAuditConfig()
	require.NoError(t, err)
	cfg.Enabled = false
	cfg.BlockingEnabled = false
	require.NoError(t, model.SavePromptAuditConfig(cfg.ConfigVersion, cfg, endpoints))
	service.InvalidatePromptAuditConfig()

	oldEnabled, oldPromptEnabled := setting.CheckSensitiveEnabled, setting.CheckSensitiveOnPromptEnabled
	oldRules, oldConfigured := setting.SensitiveRules, setting.SensitiveRulesConfigured
	oldChannelIDs := setting.SensitiveRuleChannelIds
	setting.CheckSensitiveEnabled = true
	setting.CheckSensitiveOnPromptEnabled = true
	setting.SensitiveRulesConfigured = true
	setting.SensitiveRules = []setting.SensitiveRule{{
		ID: "realtime-sensitive", Name: "Realtime Sensitive", Enabled: true,
		Action: setting.SensitiveRuleActionBlock, Scope: setting.SensitiveRuleScopeRequest,
		Keywords: []string{"realtime-secret"},
	}}
	setting.SensitiveRuleChannelIds = []int{99}
	t.Cleanup(func() {
		setting.CheckSensitiveEnabled, setting.CheckSensitiveOnPromptEnabled = oldEnabled, oldPromptEnabled
		setting.SensitiveRules, setting.SensitiveRulesConfigured = oldRules, oldConfigured
		setting.SensitiveRuleChannelIds = oldChannelIDs
	})

	var nextCalls atomic.Int64
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/v1/realtime",
		func(c *gin.Context) {
			common.SetContextKey(c, constant.ContextKeyUserId, 10)
			common.SetContextKey(c, constant.ContextKeyUserGroupId, 1)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyTokenId, 20)
			c.Next()
		},
		PromptAuditRealtime(),
		func(c *gin.Context) { nextCalls.Add(1) },
	)
	server := httptest.NewServer(router)
	defer server.Close()

	dialer := websocket.Dialer{Subprotocols: []string{"realtime"}}
	conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/realtime", nil)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.WriteJSON(map[string]interface{}{
		"type": "session.update", "session": map[string]string{"instructions": "realtime-secret"},
	}))
	_, payload, readErr := conn.ReadMessage()
	require.NoError(t, readErr)
	require.Contains(t, string(payload), "内容审计命中风险规则")
	require.NotContains(t, string(payload), string(types.ErrorCodeSensitiveWordsDetected))
	_, _, readErr = conn.ReadMessage()
	var closeErr *websocket.CloseError
	require.ErrorAs(t, readErr, &closeErr)
	require.Equal(t, 4403, closeErr.Code)
	require.Equal(t, service.SensitiveFilterRealtimeCloseReason, closeErr.Text)
	require.Zero(t, nextCalls.Load())

	var event model.PromptAuditEvent
	require.NoError(t, model.DB.First(&event, "source = ?", service.PromptAuditSourceSensitiveWord).Error)
	require.Equal(t, "realtime_request", event.Stage)
}

func TestPromptAuditRealtimeMaskForwardsRewrittenFrameButGuardsOriginal(t *testing.T) {
	guardBodies := make(chan string, 1)
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		guardBodies <- string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`)
	}))
	defer guard.Close()
	setupPromptAuditRealtimeTestDB(t, guard.URL)

	oldEnabled, oldPromptEnabled := setting.CheckSensitiveEnabled, setting.CheckSensitiveOnPromptEnabled
	oldRules, oldConfigured := setting.SensitiveRules, setting.SensitiveRulesConfigured
	oldChannelIDs, oldWords := setting.SensitiveRuleChannelIds, setting.SensitiveWords
	setting.CheckSensitiveEnabled = true
	setting.CheckSensitiveOnPromptEnabled = true
	setting.SensitiveRulesConfigured = true
	setting.SensitiveRules = []setting.SensitiveRule{{
		ID: "realtime-mask", Name: "Realtime Mask", Enabled: true,
		Action: setting.SensitiveRuleActionMask, Scope: setting.SensitiveRuleScopeRequest,
		Keywords: []string{"original-secret"}, Replacement: "[masked]",
	}}
	setting.SensitiveRuleChannelIds = []int{99}
	setting.SensitiveWords = nil
	t.Cleanup(func() {
		setting.CheckSensitiveEnabled, setting.CheckSensitiveOnPromptEnabled = oldEnabled, oldPromptEnabled
		setting.SensitiveRules, setting.SensitiveRulesConfigured = oldRules, oldConfigured
		setting.SensitiveRuleChannelIds, setting.SensitiveWords = oldChannelIDs, oldWords
	})

	forwardedFrames := make(chan string, 1)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/v1/realtime",
		func(c *gin.Context) {
			common.SetContextKey(c, constant.ContextKeyUserId, 10)
			common.SetContextKey(c, constant.ContextKeyUserGroupId, 1)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyTokenId, 20)
			c.Next()
		},
		PromptAuditRealtime(),
		func(c *gin.Context) {
			frames, _ := common.GetContextKeyType[service.PromptAuditRealtimeFrames](c, constant.ContextKeyPromptAuditRealtimeBufferedFrames)
			forwardedFrames <- string(frames[0].Payload)
			conn, _ := common.GetContextKeyType[*websocket.Conn](c, constant.ContextKeyPromptAuditRealtimeClientWs)
			_ = conn.WriteJSON(map[string]string{"type": "audit.ack"})
		},
	)
	server := httptest.NewServer(router)
	defer server.Close()

	dialer := websocket.Dialer{Subprotocols: []string{"realtime"}}
	conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/realtime", nil)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.WriteJSON(map[string]interface{}{
		"type": "session.update", "session": map[string]string{"instructions": "original-secret"},
	}))
	_, _, err = conn.ReadMessage()
	require.NoError(t, err)

	require.Contains(t, <-guardBodies, "original-secret")
	forwarded := <-forwardedFrames
	require.Contains(t, forwarded, "[masked]")
	require.NotContains(t, forwarded, "original-secret")
}

func TestPromptAuditRealtimeBuffersRawBinaryBeforeRiskJSON(t *testing.T) {
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"Safety: Unsafe\nCategories: Jailbreak"}}]}`)
	}))
	defer guard.Close()
	setupPromptAuditRealtimeTestDB(t, guard.URL)

	var nextCalls atomic.Int64
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/v1/realtime",
		func(c *gin.Context) {
			common.SetContextKey(c, constant.ContextKeyUserId, 10)
			common.SetContextKey(c, constant.ContextKeyUserGroupId, 1)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyTokenId, 20)
			c.Next()
		},
		PromptAuditRealtime(),
		func(c *gin.Context) {
			nextCalls.Add(1)
			conn, _ := common.GetContextKeyType[*websocket.Conn](c, constant.ContextKeyPromptAuditRealtimeClientWs)
			_ = conn.WriteJSON(map[string]string{"type": "audit.ack"})
		},
	)
	server := httptest.NewServer(router)
	defer server.Close()

	dialer := websocket.Dialer{Subprotocols: []string{"realtime"}}
	conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/realtime?model=gpt-realtime", nil)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, []byte{0x01, 0x7f, 0x00, 0xa5}))
	require.NoError(t, conn.WriteJSON(map[string]interface{}{
		"type":    "session.update",
		"session": map[string]string{"instructions": "blocked after binary audio"},
	}))

	_, payload, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Contains(t, string(payload), service.PromptGuardBlockedCode)
	_, _, err = conn.ReadMessage()
	var closeErr *websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, 4403, closeErr.Code)
	require.Zero(t, nextCalls.Load())
}

func TestPromptAuditRealtimeAuditsBinaryJSON(t *testing.T) {
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"Safety: Unsafe\nCategories: Jailbreak"}}]}`)
	}))
	defer guard.Close()
	setupPromptAuditRealtimeTestDB(t, guard.URL)

	var nextCalls atomic.Int64
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/v1/realtime",
		func(c *gin.Context) {
			common.SetContextKey(c, constant.ContextKeyUserId, 10)
			common.SetContextKey(c, constant.ContextKeyUserGroupId, 1)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyTokenId, 20)
			c.Next()
		},
		PromptAuditRealtime(),
		func(c *gin.Context) {
			nextCalls.Add(1)
			conn, _ := common.GetContextKeyType[*websocket.Conn](c, constant.ContextKeyPromptAuditRealtimeClientWs)
			_ = conn.WriteJSON(map[string]string{"type": "audit.ack"})
		},
	)
	server := httptest.NewServer(router)
	defer server.Close()

	dialer := websocket.Dialer{Subprotocols: []string{"realtime"}}
	conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/realtime?model=gpt-realtime", nil)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, []byte(
		`{"type":"response.create","response":{"instructions":"binary JSON risk"}}`,
	)))

	_, payload, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Contains(t, string(payload), service.PromptGuardBlockedCode)
	_, _, err = conn.ReadMessage()
	var closeErr *websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, 4403, closeErr.Code)
	require.Zero(t, nextCalls.Load())
}

func TestPromptAuditRealtimeArchivesInitialAndSubsequentFramesExactlyOnceInOrder(t *testing.T) {
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`)
	}))
	defer guard.Close()
	setupPromptAuditRealtimeTestDB(t, guard.URL)
	enablePromptAuditRealtimeRequestArchive(t)
	service.InitTokenEncoders()

	handlerResult := make(chan error, 1)
	handoffFrame := make(chan service.PromptAuditRealtimeFrame, 1)
	initialAudio := []byte{0x01, 0x7f, 0x00, 0xa5}
	initialControl := []byte(`{"type":"response.create","response":{"instructions":"safe initial control"}}`)
	postHandoff := service.PromptAuditRealtimeFrame{
		MessageType: websocket.BinaryMessage,
		Payload:     []byte(`{"type":"response.create","response":{"instructions":"relay-owned frame"}}`),
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/v1/realtime",
		func(c *gin.Context) {
			c.Set(common.RequestIdKey, "realtime-archive-order")
			common.SetContextKey(c, constant.ContextKeyUserId, 10)
			common.SetContextKey(c, constant.ContextKeyUserGroupId, 1)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
			common.SetContextKey(c, constant.ContextKeyTokenId, 20)
			c.Next()
		},
		PromptAuditRealtime(),
		func(c *gin.Context) {
			clientConn, ok := common.GetContextKeyType[*websocket.Conn](c, constant.ContextKeyPromptAuditRealtimeClientWs)
			if !ok || clientConn == nil {
				handlerResult <- fmt.Errorf("Realtime 客户端连接未写入上下文")
				return
			}
			frames, ok := common.GetContextKeyType[service.PromptAuditRealtimeFrames](c, constant.ContextKeyPromptAuditRealtimeBufferedFrames)
			if !ok || len(frames) != 2 {
				handlerResult <- fmt.Errorf("渠道分配前缓冲帧数量不一致: %d", len(frames))
				return
			}
			if frames[0].MessageType != websocket.BinaryMessage || !bytes.Equal(frames[0].Payload, initialAudio) ||
				frames[1].MessageType != websocket.TextMessage || !bytes.Equal(frames[1].Payload, initialControl) {
				handlerResult <- fmt.Errorf("渠道分配前缓冲帧顺序或内容不一致")
				return
			}
			var jobs []model.RequestArchiveJob
			if dbErr := model.DB.Order("id ASC").Find(&jobs).Error; dbErr != nil {
				handlerResult <- fmt.Errorf("读取 Realtime 归档失败: %w", dbErr)
				return
			}
			if len(jobs) != 2 {
				handlerResult <- fmt.Errorf("渠道分配前归档数量不一致: %d", len(jobs))
				return
			}
			for index, expected := range [][]byte{initialAudio, initialControl} {
				plaintext, decryptErr := service.DecryptRequestArchivePayload(&jobs[index])
				if decryptErr != nil || !bytes.Equal(plaintext, expected) {
					handlerResult <- fmt.Errorf("第 %d 个渠道分配前归档内容不一致: %v", index, decryptErr)
					return
				}
			}
			_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
			messageType, payload, readErr := clientConn.ReadMessage()
			if readErr != nil {
				handlerResult <- fmt.Errorf("下游未接收到交接后的客户端帧: %w", readErr)
				return
			}
			handoffFrame <- service.PromptAuditRealtimeFrame{MessageType: messageType, Payload: payload}
			handlerResult <- nil
		},
	)
	server := httptest.NewServer(router)
	defer server.Close()

	dialer := websocket.Dialer{Subprotocols: []string{"realtime"}}
	conn, _, err := dialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"/v1/realtime?model=gpt-realtime&credential=must-not-be-stored",
		nil,
	)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, initialAudio))
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, initialControl))
	require.NoError(t, conn.WriteMessage(postHandoff.MessageType, postHandoff.Payload))

	select {
	case handlerErr := <-handlerResult:
		require.NoError(t, handlerErr)
	case <-time.After(3 * time.Second):
		t.Fatal("Realtime middleware did not transfer socket ownership")
	}
	transferred := <-handoffFrame
	require.Equal(t, postHandoff.MessageType, transferred.MessageType)
	require.Equal(t, postHandoff.Payload, transferred.Payload)

	var jobs []model.RequestArchiveJob
	require.NoError(t, model.DB.Order("id ASC").Find(&jobs).Error)
	require.Len(t, jobs, 2, "Middleware 只归档渠道分配前由其读取的帧，后续帧由 Relay 阶段消费")
	require.Equal(t, []string{"WS_BINARY", "WS_TEXT"}, []string{jobs[0].Method, jobs[1].Method})
	require.Equal(t, "realtime-archive-order", jobs[0].RequestId)
	require.Equal(t, "realtime-archive-order", jobs[1].RequestId)
}

func TestPromptAuditRealtimeArchiveOnlyQueuesFirstFrameBeforeDistribution(t *testing.T) {
	var guardCalls atomic.Int64
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		guardCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`)
	}))
	defer guard.Close()
	setupPromptAuditRealtimeTestDB(t, guard.URL)
	enablePromptAuditRealtimeRequestArchive(t)

	cfg, endpoints, err := model.LoadPromptAuditConfig()
	require.NoError(t, err)
	cfg.Enabled = false
	cfg.BlockingEnabled = false
	require.NoError(t, model.SavePromptAuditConfig(cfg.ConfigVersion, cfg, endpoints))
	service.InvalidatePromptAuditConfig()

	oldSensitiveEnabled := setting.CheckSensitiveEnabled
	oldSensitivePromptEnabled := setting.CheckSensitiveOnPromptEnabled
	setting.CheckSensitiveEnabled = false
	setting.CheckSensitiveOnPromptEnabled = false
	t.Cleanup(func() {
		setting.CheckSensitiveEnabled = oldSensitiveEnabled
		setting.CheckSensitiveOnPromptEnabled = oldSensitivePromptEnabled
	})

	handlerResult := make(chan error, 1)
	handoffFrame := make(chan service.PromptAuditRealtimeFrame, 1)
	var distributionCalls atomic.Int64
	var archivedBeforeDistribution atomic.Bool
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/v1/realtime",
		func(c *gin.Context) {
			c.Set(common.RequestIdKey, "realtime-archive-only")
			common.SetContextKey(c, constant.ContextKeyUserId, 10)
			common.SetContextKey(c, constant.ContextKeyUserGroupId, 1)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
			common.SetContextKey(c, constant.ContextKeyTokenId, 20)
			c.Next()
		},
		PromptAuditRealtime(),
		func(c *gin.Context) {
			distributionCalls.Add(1)
			var job model.RequestArchiveJob
			if dbErr := model.DB.First(&job, "request_id = ?", "realtime-archive-only").Error; dbErr != nil {
				handlerResult <- fmt.Errorf("渠道分配前未找到首帧归档: %w", dbErr)
				return
			}
			plaintext, decryptErr := service.DecryptRequestArchivePayload(&job)
			if decryptErr != nil || !bytes.Equal(plaintext, []byte{0x01, 0x7f, 0x00, 0xa5}) {
				handlerResult <- fmt.Errorf("渠道分配前首帧归档内容不一致: %v", decryptErr)
				return
			}
			frames, ok := common.GetContextKeyType[service.PromptAuditRealtimeFrames](c, constant.ContextKeyPromptAuditRealtimeBufferedFrames)
			if !ok || len(frames) != 1 || !bytes.Equal(frames[0].Payload, plaintext) {
				handlerResult <- fmt.Errorf("首帧缓冲交接内容不一致")
				return
			}
			archivedBeforeDistribution.Store(true)
			clientConn, ok := common.GetContextKeyType[*websocket.Conn](c, constant.ContextKeyPromptAuditRealtimeClientWs)
			if !ok || clientConn == nil {
				handlerResult <- fmt.Errorf("Realtime 客户端连接未写入上下文")
				return
			}
			_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
			messageType, payload, readErr := clientConn.ReadMessage()
			if readErr != nil {
				handlerResult <- fmt.Errorf("归档-only Middleware 读取了应交给 Relay 的后续帧: %w", readErr)
				return
			}
			handoffFrame <- service.PromptAuditRealtimeFrame{MessageType: messageType, Payload: payload}
			handlerResult <- nil
		},
	)
	server := httptest.NewServer(router)
	defer server.Close()

	dialer := websocket.Dialer{Subprotocols: []string{"realtime"}}
	conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/realtime?model=gpt-realtime", nil)
	require.NoError(t, err)
	defer conn.Close()
	payload := []byte{0x01, 0x7f, 0x00, 0xa5}
	postHandoff := service.PromptAuditRealtimeFrame{
		MessageType: websocket.TextMessage,
		Payload:     []byte(`{"type":"response.create","response":{"instructions":"relay-owned"}}`),
	}
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, payload))
	require.NoError(t, conn.WriteMessage(postHandoff.MessageType, postHandoff.Payload))

	select {
	case handlerErr := <-handlerResult:
		require.NoError(t, handlerErr)
	case <-time.After(3 * time.Second):
		t.Fatal("Archive-only middleware did not transfer socket ownership")
	}
	transferred := <-handoffFrame
	require.Equal(t, postHandoff.MessageType, transferred.MessageType)
	require.Equal(t, postHandoff.Payload, transferred.Payload)
	require.Zero(t, guardCalls.Load(), "仅开启请求归档时不得调用 Guard")
	require.Equal(t, int64(1), distributionCalls.Load())
	require.True(t, archivedBeforeDistribution.Load(), "首帧必须在进入渠道分配前完成归档入队")

	var jobs []model.RequestArchiveJob
	require.NoError(t, model.DB.Order("id ASC").Find(&jobs).Error)
	require.Len(t, jobs, 1, "归档-only Middleware 不得读取或归档交接后的帧")
	plaintext, decryptErr := service.DecryptRequestArchivePayload(&jobs[0])
	require.NoError(t, decryptErr)
	require.Equal(t, payload, plaintext)
}

func TestPromptAuditRealtimeRejectsInvalidFirstControlFrameBeforeDistribution(t *testing.T) {
	var guardCalls atomic.Int64
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		guardCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`)
	}))
	defer guard.Close()
	setupPromptAuditRealtimeTestDB(t, guard.URL)

	tests := []struct {
		name    string
		payload []byte
	}{
		{name: "空对象", payload: []byte(`{}`)},
		{name: "缺少类型", payload: []byte(`{"session":{"instructions":"text"}}`)},
		{name: "类型为空", payload: []byte(`{"type":"  "}`)},
		{name: "JSON 数组", payload: []byte(`[]`)},
		{name: "非 JSON 文本", payload: []byte(`not-json`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var nextCalls atomic.Int64
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.GET("/v1/realtime",
				func(c *gin.Context) {
					common.SetContextKey(c, constant.ContextKeyUserId, 10)
					common.SetContextKey(c, constant.ContextKeyUserGroupId, 1)
					common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
					c.Next()
				},
				PromptAuditRealtime(),
				func(c *gin.Context) { nextCalls.Add(1) },
			)
			server := httptest.NewServer(router)
			defer server.Close()

			dialer := websocket.Dialer{Subprotocols: []string{"realtime"}}
			conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/realtime", nil)
			require.NoError(t, err)
			defer conn.Close()
			require.NoError(t, conn.WriteMessage(websocket.TextMessage, test.payload))

			_, payload, err := conn.ReadMessage()
			require.NoError(t, err)
			require.Contains(t, string(payload), string(types.ErrorCodeInvalidRequest))
			_, _, err = conn.ReadMessage()
			var closeErr *websocket.CloseError
			require.ErrorAs(t, err, &closeErr)
			require.Equal(t, websocket.CloseInvalidFramePayloadData, closeErr.Code)
			require.Zero(t, nextCalls.Load())
		})
	}
	require.Zero(t, guardCalls.Load(), "非法首帧不应调用 Guard 或进入渠道分配")
}

func TestPromptAuditRealtimeRejectsTooManyBufferedFrames(t *testing.T) {
	var guardCalls atomic.Int64
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		guardCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`)
	}))
	defer guard.Close()
	setupPromptAuditRealtimeTestDB(t, guard.URL)

	var nextCalls atomic.Int64
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/v1/realtime",
		func(c *gin.Context) {
			common.SetContextKey(c, constant.ContextKeyUserId, 10)
			common.SetContextKey(c, constant.ContextKeyUserGroupId, 1)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyTokenId, 20)
			c.Next()
		},
		PromptAuditRealtime(),
		func(c *gin.Context) {
			nextCalls.Add(1)
		},
	)
	server := httptest.NewServer(router)
	defer server.Close()

	dialer := websocket.Dialer{Subprotocols: []string{"realtime"}}
	conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/realtime?model=gpt-realtime", nil)
	require.NoError(t, err)
	defer conn.Close()
	for index := 0; index <= promptAuditRealtimeMaxBufferedFrames; index++ {
		require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, []byte{0x00}))
	}

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
	_, payload, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Contains(t, string(payload), string(types.ErrorCodeInvalidRequest))
	_, _, err = conn.ReadMessage()
	var closeErr *websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, websocket.CloseMessageTooBig, closeErr.Code)
	require.Zero(t, nextCalls.Load())
	require.Zero(t, guardCalls.Load())
}

func TestPromptAuditRealtimeReturnsDistributionFailureAsWebSocketEvent(t *testing.T) {
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`)
	}))
	defer guard.Close()
	setupPromptAuditRealtimeTestDB(t, guard.URL)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/v1/realtime",
		func(c *gin.Context) {
			common.SetContextKey(c, constant.ContextKeyUserId, 10)
			common.SetContextKey(c, constant.ContextKeyUserGroupId, 1)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyTokenId, 20)
			c.Next()
		},
		PromptAuditRealtime(),
		func(c *gin.Context) {
			abortWithOpenAiMessage(c, http.StatusServiceUnavailable, "当前没有可用渠道", types.ErrorCodeModelNotFound)
		},
	)
	server := httptest.NewServer(router)
	defer server.Close()

	dialer := websocket.Dialer{Subprotocols: []string{"realtime"}}
	conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/realtime?model=gpt-realtime", nil)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.WriteJSON(map[string]interface{}{
		"type":    "session.update",
		"session": map[string]string{"instructions": "safe realtime prompt"},
	}))

	_, payload, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Contains(t, string(payload), string(types.ErrorCodeModelNotFound))
	require.Contains(t, string(payload), "当前没有可用渠道")
	_, _, err = conn.ReadMessage()
	var closeErr *websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, websocket.CloseTryAgainLater, closeErr.Code)
}

func setupPromptAuditRealtimeTestDB(t *testing.T, guardURL string) {
	t.Helper()
	oldDB := model.DB
	oldSecret := common.CryptoSecret
	oldRedisEnabled := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "prompt-audit-realtime.db")), &gorm.Config{
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
	common.RedisEnabled = false
	t.Setenv("CRYPTO_SECRET", "stable-realtime-test-secret")
	common.CryptoSecret = "stable-realtime-test-secret"
	require.NoError(t, model.EnsurePromptAuditDefaults())
	require.NoError(t, model.EnsureRequestArchiveDefaults())
	cfg, _, err := model.LoadPromptAuditConfig()
	require.NoError(t, err)
	cfg.Enabled = true
	cfg.BlockingEnabled = true
	require.NoError(t, model.SavePromptAuditConfig(cfg.ConfigVersion, cfg, []model.PromptAuditEndpoint{{
		Id: "guard-realtime", Name: "Guard Realtime", Protocol: "openai_compatible",
		BaseUrl: guardURL, Model: service.PromptAuditDefaultModel,
		TimeoutMs: 1000, InputLimit: service.PromptAuditDefaultInputLimit, Enabled: true,
	}}))
	service.InvalidatePromptAuditConfig()
	service.InvalidateRequestArchiveConfig()
	t.Cleanup(func() {
		service.InvalidatePromptAuditConfig()
		service.InvalidateRequestArchiveConfig()
		common.CryptoSecret = oldSecret
		common.RedisEnabled = oldRedisEnabled
		model.DB = oldDB
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
}

func enablePromptAuditRealtimeRequestArchive(t *testing.T) {
	t.Helper()
	config, err := service.GetRequestArchiveConfig(context.Background())
	require.NoError(t, err)
	_, err = service.SaveRequestArchiveConfig(context.Background(), service.RequestArchiveUpdateRequest{
		ExpectedConfigVersion: config.ConfigVersion,
		Enabled:               true,
		ActiveTargetId:        "realtime-archive",
		RetentionDays:         30,
		WorkerCount:           1,
		QueueCapacity:         16,
		MaxBodyBytes:          model.RequestArchiveDefaultMaxBodyBytes,
		QueueMaxBytes:         model.RequestArchiveDefaultQueueMaxBytes,
		Targets: []service.RequestArchiveUpdateTarget{{
			Id: "realtime-archive", Name: "Realtime 归档", Type: model.RequestArchiveTargetLocal,
			Enabled: true, LocalPath: requestArchiveMiddlewareTestLocalPath(t, "archive"),
		}},
	}, 1)
	require.NoError(t, err)
}

func TestWritePromptAuditRealtimeDecisionFinalClientView(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, common.UpdateErrorMessageReplacementRules(`[]`)) })
	tests := []struct {
		name          string
		requestID     string
		rules         string
		decision      service.PromptAuditDecision
		wantMessage   string
		wantCloseCode int
	}{
		{
			name:      "blocked frame ignores HTTP status rule and decorates request id",
			requestID: "realtime-audit-request",
			rules: `[
				{"status_code":403,"match":"internal realtime blocked","mode":"exact","replace":"wrong status-conditioned message"},
				{"match":"internal realtime blocked","mode":"exact","replace":"public realtime blocked","replace_status_code":429}
			]`,
			decision: service.PromptAuditDecision{
				Allow: false, ErrorCode: service.PromptGuardBlockedCode,
				HTTPStatus: http.StatusForbidden, Message: "internal realtime blocked",
			},
			wantMessage:   "public realtime blocked (request id: realtime-audit-request)",
			wantCloseCode: 4403,
		},
		{
			name: "fail closed frame leaves empty request id undecorated",
			rules: `[
				{"status_code":503,"match":"internal realtime unavailable","mode":"exact","replace":"wrong status-conditioned message"},
				{"match":"internal realtime unavailable","mode":"exact","replace":"public realtime unavailable"}
			]`,
			decision: service.PromptAuditDecision{
				Allow: false, ErrorCode: service.PromptGuardUnavailableCode,
				HTTPStatus: http.StatusServiceUnavailable, Message: "internal realtime unavailable",
			},
			wantMessage:   "public realtime unavailable",
			wantCloseCode: websocket.CloseTryAgainLater,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, common.UpdateErrorMessageReplacementRules(test.rules))
			original := test.decision
			type serverResult struct {
				decision service.PromptAuditDecision
				err      error
			}
			result := make(chan serverResult, 1)
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.GET("/v1/realtime", func(c *gin.Context) {
				c.Set(common.RequestIdKey, test.requestID)
				serverConn, err := promptAuditRealtimeUpgrader.Upgrade(c.Writer, c.Request, nil)
				if err != nil {
					result <- serverResult{err: err}
					return
				}
				defer serverConn.Close()
				writePromptAuditRealtimeDecision(c, serverConn, test.decision)
				result <- serverResult{decision: test.decision}
			})
			server := httptest.NewServer(router)
			defer server.Close()

			dialer := websocket.Dialer{Subprotocols: []string{"realtime"}}
			clientConn, _, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/realtime", nil)
			require.NoError(t, err)
			defer clientConn.Close()
			_, frame, err := clientConn.ReadMessage()
			require.NoError(t, err)
			var event struct {
				Error *types.OpenAIError `json:"error"`
			}
			require.NoError(t, common.Unmarshal(frame, &event))
			require.NotNil(t, event.Error)
			require.Equal(t, test.wantMessage, event.Error.Message)
			require.Equal(t, test.decision.ErrorCode, event.Error.Code)

			_, _, err = clientConn.ReadMessage()
			var closeErr *websocket.CloseError
			require.ErrorAs(t, err, &closeErr)
			require.Equal(t, test.wantCloseCode, closeErr.Code)
			written := <-result
			require.NoError(t, written.err)
			require.Equal(t, original, written.decision)
		})
	}
}

func TestPromptAuditRealtimeCyberBlockFinalizesClientMessageOnce(t *testing.T) {
	guardCalls := atomic.Int64{}
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		guardCalls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer guard.Close()
	setupPromptAuditRealtimeTestDB(t, guard.URL)
	require.NoError(t, common.UpdateErrorMessageReplacementRules(`[{"match":"当前会话因上游安全策略拒绝已被本地屏蔽，请开启新会话后重试","mode":"exact","replace":"public cyber block"}]`))
	t.Cleanup(func() { require.NoError(t, common.UpdateErrorMessageReplacementRules(`[]`)) })

	cfg, endpoints, err := model.LoadPromptAuditConfig()
	require.NoError(t, err)
	cfg.UpstreamPolicyEnabled = true
	cfg.CyberSessionBlockEnabled = true
	cfg.CyberSessionBlockTTLSeconds = 60
	require.NoError(t, model.SavePromptAuditConfig(cfg.ConfigVersion, cfg, endpoints))
	service.InvalidatePromptAuditConfig()
	runtimeCfg, err := service.GetPromptAuditConfig(context.Background())
	require.NoError(t, err)

	const sessionID = "blocked-realtime-session"
	seed, _ := gin.CreateTestContext(httptest.NewRecorder())
	seed.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)
	seed.Request.Header.Set("X-Session-Id", sessionID)
	common.SetContextKey(seed, constant.ContextKeyTokenId, 909)
	require.NotEmpty(t, service.CacheCyberSessionBlockKey(seed, nil))
	require.True(t, service.MarkCyberSessionBlocked(seed, runtimeCfg))

	var nextCalls atomic.Int64
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/v1/realtime",
		func(c *gin.Context) {
			c.Set(common.RequestIdKey, "cyber-realtime-request")
			common.SetContextKey(c, constant.ContextKeyTokenId, 909)
			c.Next()
		},
		PromptAuditRealtime(),
		func(c *gin.Context) { nextCalls.Add(1) },
	)
	server := httptest.NewServer(router)
	defer server.Close()

	headers := http.Header{}
	headers.Set("X-Session-Id", sessionID)
	dialer := websocket.Dialer{Subprotocols: []string{"realtime"}}
	conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/realtime", headers)
	require.NoError(t, err)
	defer conn.Close()
	_, frame, err := conn.ReadMessage()
	require.NoError(t, err)
	var event struct {
		Error *types.OpenAIError `json:"error"`
	}
	require.NoError(t, common.Unmarshal(frame, &event))
	require.NotNil(t, event.Error)
	require.Equal(t, "public cyber block (request id: cyber-realtime-request)", event.Error.Message)
	require.Equal(t, 1, strings.Count(event.Error.Message, "public cyber block"))
	require.Equal(t, 1, strings.Count(event.Error.Message, "(request id: cyber-realtime-request)"))
	require.Equal(t, service.CyberSessionBlockedCode, event.Error.Code)

	_, _, err = conn.ReadMessage()
	var closeErr *websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, 4403, closeErr.Code)
	require.Zero(t, nextCalls.Load())
	require.Zero(t, guardCalls.Load())
}
