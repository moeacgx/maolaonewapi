package openai

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOpenAIRealtimeAuditsEachTextFrameBeforeUpstreamWrite(t *testing.T) {
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"Safety: Unsafe\nCategories: Jailbreak"}}]}`)
	}))
	defer guard.Close()
	setupOpenAIRealtimePromptAuditDB(t, guard.URL)

	clientPeer, relayClient, closeClientPair := newRealtimeWebSocketPair(t)
	defer closeClientPair()
	relayTarget, upstreamPeer, closeTargetPair := newRealtimeWebSocketPair(t)
	defer closeTargetPair()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: "/v1/realtime", RawQuery: "model=gpt-realtime"},
		Header: make(http.Header),
	}
	c.Set(common.RequestIdKey, "realtime-frame-order")
	common.SetContextKey(c, constant.ContextKeyUserId, 10)
	common.SetContextKey(c, constant.ContextKeyUserGroupId, 1)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenId, 20)
	common.SetContextKey(c, constant.ContextKeyPromptAuditRealtimeActive, true)

	info := &relaycommon.RelayInfo{
		ClientWs: clientPeer, TargetWs: relayTarget, OriginModelName: "gpt-realtime",
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-realtime"},
	}
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		relayErr, _ := OpenaiRealtimeHandler(c, info)
		require.Nil(t, relayErr)
	}()

	// 音频二进制帧不属于文本 Guard 范围，应先按原顺序写入上游。
	binaryAudio := []byte{0x01, 0x7f, 0x00, 0xa5}
	require.NoError(t, relayClient.WriteMessage(websocket.BinaryMessage, binaryAudio))
	require.NoError(t, upstreamPeer.SetReadDeadline(time.Now().Add(3*time.Second)))
	firstType, firstPayload, err := upstreamPeer.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.BinaryMessage, firstType)
	require.Equal(t, binaryAudio, firstPayload)

	// 第二帧包含风险文本；客户端收到错误和 4403，上游不得收到该帧。
	require.NoError(t, relayClient.WriteJSON(map[string]interface{}{
		"type":    "session.update",
		"session": map[string]string{"instructions": "blocked realtime prompt"},
	}))
	require.NoError(t, relayClient.SetReadDeadline(time.Now().Add(3*time.Second)))
	_, errorPayload, err := relayClient.ReadMessage()
	require.NoError(t, err)
	require.Contains(t, string(errorPayload), service.PromptGuardBlockedCode)
	_, _, err = relayClient.ReadMessage()
	var clientClose *websocket.CloseError
	require.ErrorAs(t, err, &clientClose)
	require.Equal(t, 4403, clientClose.Code)

	require.NoError(t, upstreamPeer.SetReadDeadline(time.Now().Add(3*time.Second)))
	_, unexpectedPayload, err := upstreamPeer.ReadMessage()
	require.Error(t, err)
	require.NotContains(t, string(unexpectedPayload), "blocked realtime prompt")

	select {
	case <-handlerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("Realtime handler did not stop after prompt audit rejection")
	}
}

func TestPromptAuditRealtimeRequestCapturesSelectedChannelAndGroups(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = &http.Request{URL: &url.URL{Path: "/v1/realtime"}}
	common.SetContextKey(c, constant.ContextKeySelectedChannel, &model.Channel{
		Id: 42, Name: "Realtime 渠道",
		GroupDetails: []model.GroupReference{{Id: 7, Code: "vip", Name: "贵宾分组"}},
	})

	request := promptAuditRealtimeRequest(c, &relaycommon.RelayInfo{OriginModelName: "gpt-realtime"}, []byte(`{"type":"response.create"}`))
	require.Equal(t, 42, request.ChannelId)
	require.Equal(t, "Realtime 渠道", request.ChannelName)
	require.Equal(t, []model.PromptAuditEventChannelGroup{{Id: 7, Code: "vip", Name: "贵宾分组"}}, request.ChannelGroups)
}

func TestOpenAIRealtimeAuditsBinaryJSONBeforeUpstreamWrite(t *testing.T) {
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"Safety: Unsafe\nCategories: Jailbreak"}}]}`)
	}))
	defer guard.Close()
	setupOpenAIRealtimePromptAuditDB(t, guard.URL)

	clientPeer, relayClient, closeClientPair := newRealtimeWebSocketPair(t)
	defer closeClientPair()
	relayTarget, upstreamPeer, closeTargetPair := newRealtimeWebSocketPair(t)
	defer closeTargetPair()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: "/v1/realtime", RawQuery: "model=gpt-realtime"},
		Header: make(http.Header),
	}
	c.Set(common.RequestIdKey, "realtime-binary-json")
	common.SetContextKey(c, constant.ContextKeyUserId, 10)
	common.SetContextKey(c, constant.ContextKeyUserGroupId, 1)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenId, 20)
	common.SetContextKey(c, constant.ContextKeyPromptAuditRealtimeActive, true)

	info := &relaycommon.RelayInfo{
		ClientWs: clientPeer, TargetWs: relayTarget, OriginModelName: "gpt-realtime",
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-realtime"},
	}
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		_, _ = OpenaiRealtimeHandler(c, info)
	}()

	riskPayload := []byte(`{"type":"response.create","response":{"instructions":"binary JSON risk"}}`)
	require.NoError(t, relayClient.WriteMessage(websocket.BinaryMessage, riskPayload))
	require.NoError(t, relayClient.SetReadDeadline(time.Now().Add(3*time.Second)))
	_, errorPayload, err := relayClient.ReadMessage()
	require.NoError(t, err)
	require.Contains(t, string(errorPayload), service.PromptGuardBlockedCode)
	_, _, err = relayClient.ReadMessage()
	var clientClose *websocket.CloseError
	require.ErrorAs(t, err, &clientClose)
	require.Equal(t, 4403, clientClose.Code)

	require.NoError(t, upstreamPeer.SetReadDeadline(time.Now().Add(3*time.Second)))
	_, unexpectedPayload, err := upstreamPeer.ReadMessage()
	require.Error(t, err)
	require.NotContains(t, string(unexpectedPayload), "binary JSON risk")

	select {
	case <-handlerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("Realtime handler did not stop after binary JSON audit rejection")
	}
}

func TestOpenAIRealtimeRejectsMalformedClientFrameWithProtocolError(t *testing.T) {
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`)
	}))
	defer guard.Close()
	setupOpenAIRealtimePromptAuditDB(t, guard.URL)

	clientPeer, relayClient, closeClientPair := newRealtimeWebSocketPair(t)
	defer closeClientPair()
	relayTarget, upstreamPeer, closeTargetPair := newRealtimeWebSocketPair(t)
	defer closeTargetPair()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: "/v1/realtime", RawQuery: "model=gpt-realtime"},
		Header: make(http.Header),
	}
	common.SetContextKey(c, constant.ContextKeyPromptAuditRealtimeActive, true)
	info := &relaycommon.RelayInfo{
		ClientWs: clientPeer, TargetWs: relayTarget, OriginModelName: "gpt-realtime",
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-realtime"},
	}
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		_, _ = OpenaiRealtimeHandler(c, info)
	}()

	require.NoError(t, relayClient.WriteMessage(websocket.TextMessage, []byte("not-json")))
	require.NoError(t, relayClient.SetReadDeadline(time.Now().Add(3*time.Second)))
	_, errorPayload, err := relayClient.ReadMessage()
	require.NoError(t, err)
	require.Contains(t, string(errorPayload), string(types.ErrorCodeInvalidRequest))
	_, _, err = relayClient.ReadMessage()
	var clientClose *websocket.CloseError
	require.ErrorAs(t, err, &clientClose)
	require.Equal(t, websocket.CloseInvalidFramePayloadData, clientClose.Code)

	require.NoError(t, upstreamPeer.SetReadDeadline(time.Now().Add(500*time.Millisecond)))
	_, _, err = upstreamPeer.ReadMessage()
	require.Error(t, err)

	select {
	case <-handlerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("Realtime handler did not stop after malformed client frame")
	}
}

func TestOpenAIRealtimeForwardsPreAuditBufferInOrder(t *testing.T) {
	var guardCalls atomic.Int64
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		guardCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"Safety: Unsafe\nCategories: Jailbreak"}}]}`)
	}))
	defer guard.Close()
	setupOpenAIRealtimePromptAuditDB(t, guard.URL)

	clientPeer, relayClient, closeClientPair := newRealtimeWebSocketPair(t)
	defer closeClientPair()
	relayTarget, upstreamPeer, closeTargetPair := newRealtimeWebSocketPair(t)
	defer closeTargetPair()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: "/v1/realtime", RawQuery: "model=gpt-realtime"},
		Header: make(http.Header),
	}
	common.SetContextKey(c, constant.ContextKeyPromptAuditRealtimeActive, true)
	rawAudio := []byte{0x01, 0x7f, 0x00, 0xa5}
	safeControl := []byte(`{"type":"session.update","session":{"instructions":"already audited safe prompt"}}`)
	common.SetContextKey(c, constant.ContextKeyPromptAuditRealtimeBufferedFrames, service.PromptAuditRealtimeFrames{
		{MessageType: websocket.BinaryMessage, Payload: rawAudio},
		{MessageType: websocket.TextMessage, Payload: safeControl},
	})

	info := &relaycommon.RelayInfo{
		ClientWs: clientPeer, TargetWs: relayTarget, OriginModelName: "gpt-realtime",
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-realtime"},
	}
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		_, _ = OpenaiRealtimeHandler(c, info)
	}()

	require.NoError(t, upstreamPeer.SetReadDeadline(time.Now().Add(3*time.Second)))
	firstType, firstPayload, err := upstreamPeer.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.BinaryMessage, firstType)
	require.Equal(t, rawAudio, firstPayload)
	secondType, secondPayload, err := upstreamPeer.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.TextMessage, secondType)
	require.Equal(t, safeControl, secondPayload)
	require.Zero(t, guardCalls.Load(), "渠道前已审计的缓冲帧不得重复调用 Guard")

	_ = relayClient.Close()
	select {
	case <-handlerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("Realtime handler did not stop after client close")
	}
}

func TestOpenAIRealtimeSkipsAuditWhenPreallocationScopeDidNotEnableIt(t *testing.T) {
	var guardCalls atomic.Int64
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		guardCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"Safety: Unsafe\nCategories: Jailbreak"}}]}`)
	}))
	defer guard.Close()
	setupOpenAIRealtimePromptAuditDB(t, guard.URL)

	clientPeer, relayClient, closeClientPair := newRealtimeWebSocketPair(t)
	defer closeClientPair()
	relayTarget, upstreamPeer, closeTargetPair := newRealtimeWebSocketPair(t)
	defer closeTargetPair()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: "/v1/realtime", RawQuery: "model=gpt-realtime"},
		Header: make(http.Header),
	}
	info := &relaycommon.RelayInfo{
		ClientWs: clientPeer, TargetWs: relayTarget, OriginModelName: "gpt-realtime",
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-realtime"},
	}
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		_, _ = OpenaiRealtimeHandler(c, info)
	}()

	// 未设置 ContextKeyPromptAuditRealtimeActive 表示连接在渠道分配前未命中审计范围。
	// 即使全局配置为 blocking，后续处理也不能在渠道分配后重新启用审计。
	restrictedFrame := map[string]interface{}{
		"type":     "response.create",
		"response": map[string]string{"instructions": "blocked only when audit scope is active"},
	}
	require.NoError(t, relayClient.WriteJSON(restrictedFrame))
	require.NoError(t, upstreamPeer.SetReadDeadline(time.Now().Add(3*time.Second)))
	_, payload, err := upstreamPeer.ReadMessage()
	require.NoError(t, err)
	require.Contains(t, string(payload), "blocked only when audit scope is active")
	require.Zero(t, guardCalls.Load())

	_ = relayClient.Close()
	select {
	case <-handlerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("Realtime handler did not stop after client close")
	}
}

func TestOpenAIRealtimeBlocksSubsequentFrameAfterCyberPolicy(t *testing.T) {
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`)
	}))
	defer guard.Close()
	setupOpenAIRealtimePromptAuditDB(t, guard.URL)

	clientPeer, relayClient, closeClientPair := newRealtimeWebSocketPair(t)
	defer closeClientPair()
	relayTarget, upstreamPeer, closeTargetPair := newRealtimeWebSocketPair(t)
	defer closeTargetPair()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: "/v1/realtime", RawQuery: "model=gpt-realtime"},
		Header: make(http.Header),
	}
	c.Set(common.RequestIdKey, "realtime-cyber-policy-block")
	common.SetContextKey(c, constant.ContextKeyUserId, 10)

	info := &relaycommon.RelayInfo{
		ClientWs: clientPeer, TargetWs: relayTarget, OriginModelName: "gpt-realtime",
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-realtime"},
	}
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		_, _ = OpenaiRealtimeHandler(c, info)
	}()

	cyberPolicyFrame := []byte(`{"type":"error","error":{"code":"cyber_policy","message":"blocked"}}`)
	require.NoError(t, upstreamPeer.WriteMessage(websocket.TextMessage, cyberPolicyFrame))
	require.NoError(t, relayClient.SetReadDeadline(time.Now().Add(3*time.Second)))
	_, upstreamError, err := relayClient.ReadMessage()
	require.NoError(t, err)
	require.JSONEq(t, string(cyberPolicyFrame), string(upstreamError))

	nextFrame := []byte(`{"type":"response.create","response":{"instructions":"must not reach upstream"}}`)
	require.NoError(t, relayClient.WriteMessage(websocket.TextMessage, nextFrame))
	_, blockPayload, err := relayClient.ReadMessage()
	require.NoError(t, err)
	require.Contains(t, string(blockPayload), string(types.ErrorCodePromptBlocked))
	_, _, err = relayClient.ReadMessage()
	var clientClose *websocket.CloseError
	require.ErrorAs(t, err, &clientClose)
	require.Equal(t, 4403, clientClose.Code)

	require.NoError(t, upstreamPeer.SetReadDeadline(time.Now().Add(500*time.Millisecond)))
	_, _, err = upstreamPeer.ReadMessage()
	require.Error(t, err)

	select {
	case <-handlerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("Realtime handler did not stop after cyber_policy conversation block")
	}
}

func TestOpenAIRealtimeAllowsSubsequentFrameWhenConversationBlockDisabled(t *testing.T) {
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`)
	}))
	defer guard.Close()
	setupOpenAIRealtimePromptAuditDB(t, guard.URL)
	cfg, endpoints, err := model.LoadPromptAuditConfig()
	require.NoError(t, err)
	cfg.CyberPolicyConversationBlockEnabled = false
	require.NoError(t, model.SavePromptAuditConfig(cfg.ConfigVersion, cfg, endpoints))
	service.InvalidatePromptAuditConfig()

	clientPeer, relayClient, closeClientPair := newRealtimeWebSocketPair(t)
	defer closeClientPair()
	relayTarget, upstreamPeer, closeTargetPair := newRealtimeWebSocketPair(t)
	defer closeTargetPair()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: "/v1/realtime", RawQuery: "model=gpt-realtime"},
		Header: make(http.Header),
	}
	c.Set(common.RequestIdKey, "realtime-cyber-policy-disabled")
	common.SetContextKey(c, constant.ContextKeyUserId, 10)

	info := &relaycommon.RelayInfo{
		ClientWs: clientPeer, TargetWs: relayTarget, OriginModelName: "gpt-realtime",
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-realtime"},
	}
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		_, _ = OpenaiRealtimeHandler(c, info)
	}()

	cyberPolicyFrame := []byte(`{"type":"error","error":{"code":"cyber_policy","message":"blocked"}}`)
	require.NoError(t, upstreamPeer.WriteMessage(websocket.TextMessage, cyberPolicyFrame))
	require.NoError(t, relayClient.SetReadDeadline(time.Now().Add(3*time.Second)))
	_, upstreamError, err := relayClient.ReadMessage()
	require.NoError(t, err)
	require.JSONEq(t, string(cyberPolicyFrame), string(upstreamError))

	nextFrame := []byte(`{"type":"response.create","response":{"instructions":"allowed when disabled"}}`)
	require.NoError(t, relayClient.WriteMessage(websocket.TextMessage, nextFrame))
	require.NoError(t, upstreamPeer.SetReadDeadline(time.Now().Add(3*time.Second)))
	_, forwarded, err := upstreamPeer.ReadMessage()
	require.NoError(t, err)
	require.JSONEq(t, string(nextFrame), string(forwarded))

	_ = relayClient.Close()
	select {
	case <-handlerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("Realtime handler did not stop after client close")
	}
}

func TestOpenAIRealtimeAllowsSubsequentFrameOutsideOfficialRiskScope(t *testing.T) {
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`)
	}))
	defer guard.Close()
	setupOpenAIRealtimePromptAuditDB(t, guard.URL)
	cfg, endpoints, err := model.LoadPromptAuditConfig()
	require.NoError(t, err)
	cfg.UpstreamPolicyTargetType = service.PromptAuditUpstreamPolicyTargetGroups
	cfg.UpstreamPolicyGroupCodes = `["official"]`
	require.NoError(t, model.SavePromptAuditConfig(cfg.ConfigVersion, cfg, endpoints))
	service.InvalidatePromptAuditConfig()

	clientPeer, relayClient, closeClientPair := newRealtimeWebSocketPair(t)
	defer closeClientPair()
	relayTarget, upstreamPeer, closeTargetPair := newRealtimeWebSocketPair(t)
	defer closeTargetPair()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: "/v1/realtime", RawQuery: "model=gpt-realtime"},
		Header: make(http.Header),
	}
	c.Set(common.RequestIdKey, "realtime-cyber-policy-out-of-scope")
	common.SetContextKey(c, constant.ContextKeyUserId, 10)
	common.SetContextKey(c, constant.ContextKeySelectedChannelGroup, "hack")

	info := &relaycommon.RelayInfo{
		ClientWs: clientPeer, TargetWs: relayTarget, OriginModelName: "gpt-realtime",
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-realtime"},
	}
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		_, _ = OpenaiRealtimeHandler(c, info)
	}()

	cyberPolicyFrame := []byte(`{"type":"error","error":{"code":"cyber_policy","message":"blocked"}}`)
	require.NoError(t, upstreamPeer.WriteMessage(websocket.TextMessage, cyberPolicyFrame))
	require.NoError(t, relayClient.SetReadDeadline(time.Now().Add(3*time.Second)))
	_, upstreamError, err := relayClient.ReadMessage()
	require.NoError(t, err)
	require.JSONEq(t, string(cyberPolicyFrame), string(upstreamError))

	nextFrame := []byte(`{"type":"response.create","response":{"instructions":"allowed outside scope"}}`)
	require.NoError(t, relayClient.WriteMessage(websocket.TextMessage, nextFrame))
	require.NoError(t, upstreamPeer.SetReadDeadline(time.Now().Add(3*time.Second)))
	_, forwarded, err := upstreamPeer.ReadMessage()
	require.NoError(t, err)
	require.JSONEq(t, string(nextFrame), string(forwarded))

	_ = relayClient.Close()
	select {
	case <-handlerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("Realtime handler did not stop after client close")
	}
}

func newRealtimeWebSocketPair(t *testing.T) (serverConn, clientConn *websocket.Conn, cleanup func()) {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	serverConnCh := make(chan *websocket.Conn, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConnCh <- conn
		<-release
	}))
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	select {
	case serverConn = <-serverConnCh:
	case <-time.After(3 * time.Second):
		server.Close()
		t.Fatal("WebSocket test pair setup timed out")
	}
	var released bool
	cleanup = func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		if !released {
			close(release)
			released = true
		}
		server.Close()
	}
	return serverConn, clientConn, cleanup
}

func setupOpenAIRealtimePromptAuditDB(t *testing.T, guardURL string) {
	t.Helper()
	service.InitTokenEncoders()
	oldDB := model.DB
	oldSecret := common.CryptoSecret
	oldRedisEnabled := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "prompt-audit-openai-realtime.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.PromptAuditConfig{}, &model.PromptAuditEndpoint{}, &model.PromptAuditJob{},
		&model.PromptAuditEvent{}, &model.PromptAuditQueueState{},
	))
	model.DB = db
	common.RedisEnabled = false
	t.Setenv("CRYPTO_SECRET", "stable-openai-realtime-test-secret")
	common.CryptoSecret = "stable-openai-realtime-test-secret"
	require.NoError(t, model.EnsurePromptAuditDefaults())
	cfg, _, err := model.LoadPromptAuditConfig()
	require.NoError(t, err)
	cfg.Enabled = true
	cfg.BlockingEnabled = true
	require.NoError(t, model.SavePromptAuditConfig(cfg.ConfigVersion, cfg, []model.PromptAuditEndpoint{{
		Id: "guard-openai-realtime", Name: "Guard OpenAI Realtime", Protocol: "openai_compatible",
		BaseUrl: guardURL, Model: service.PromptAuditDefaultModel,
		TimeoutMs: 1000, InputLimit: service.PromptAuditDefaultInputLimit, Enabled: true,
	}}))
	service.InvalidatePromptAuditConfig()
	t.Cleanup(func() {
		service.InvalidatePromptAuditConfig()
		common.CryptoSecret = oldSecret
		common.RedisEnabled = oldRedisEnabled
		model.DB = oldDB
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
}
