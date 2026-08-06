package openai

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestOpenAIRealtimeQuotaSyncErrorIsVisibleAndKeepsCommittedUsage(t *testing.T) {
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
	c.Set(common.RequestIdKey, "realtime-quota-sync")

	info := &relaycommon.RelayInfo{
		ClientWs: clientPeer, TargetWs: relayTarget, OriginModelName: "gpt-realtime",
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-realtime"},
	}

	consumeCalls := 0
	consumedTotals := make([]int, 0, 2)
	consumeQuota := func(_ *gin.Context, _ *relaycommon.RelayInfo, usage *dto.RealtimeUsage) error {
		consumeCalls++
		consumedTotals = append(consumedTotals, usage.TotalTokens)
		if consumeCalls == 1 {
			return nil
		}
		return fmt.Errorf("%w: test persistence window", model.ErrUserQuotaCacheSync)
	}

	type handlerResult struct {
		err   *types.NewAPIError
		usage *dto.RealtimeUsage
	}
	handlerDone := make(chan handlerResult, 1)
	go func() {
		relayErr, usage := openaiRealtimeHandlerWithQuotaConsumer(c, info, consumeQuota)
		handlerDone <- handlerResult{err: relayErr, usage: usage}
	}()

	firstDone := []byte(`{"type":"response.done","response":{"usage":{"total_tokens":12,"input_tokens":7,"output_tokens":5,"input_token_details":{"text_tokens":7},"output_token_details":{"text_tokens":5}}}}`)
	require.NoError(t, upstreamPeer.WriteMessage(websocket.TextMessage, firstDone))
	require.NoError(t, relayClient.SetReadDeadline(time.Now().Add(3*time.Second)))
	_, firstPayload, err := relayClient.ReadMessage()
	require.NoError(t, err)
	require.JSONEq(t, string(firstDone), string(firstPayload))

	failedDone := []byte(`{"type":"response.done","response":{"usage":{"total_tokens":30,"input_tokens":20,"output_tokens":10,"input_token_details":{"text_tokens":20},"output_token_details":{"text_tokens":10}}}}`)
	require.NoError(t, upstreamPeer.WriteMessage(websocket.TextMessage, failedDone))

	_, errorPayload, err := relayClient.ReadMessage()
	require.NoError(t, err)
	var errorEvent dto.RealtimeEvent
	require.NoError(t, common.Unmarshal(errorPayload, &errorEvent))
	require.Equal(t, dto.RealtimeEventTypeError, errorEvent.Type)
	require.NotNil(t, errorEvent.Error)
	require.Equal(t, string(types.ErrorTypeNewAPIError), errorEvent.Error.Type)
	require.Equal(t, string(types.ErrorCodeQueryDataError), errorEvent.Error.Code)

	_, _, err = relayClient.ReadMessage()
	var closeErr *websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, websocket.CloseTryAgainLater, closeErr.Code)

	select {
	case result := <-handlerDone:
		require.NotNil(t, result.err)
		require.ErrorIs(t, result.err, model.ErrUserQuotaCacheSync)
		require.Equal(t, http.StatusServiceUnavailable, result.err.StatusCode)
		require.True(t, types.IsSkipRetryError(result.err))
		require.NotNil(t, result.usage)
		require.Equal(t, 12, result.usage.TotalTokens)
		require.Equal(t, 7, result.usage.InputTokens)
		require.Equal(t, 5, result.usage.OutputTokens)
		require.Equal(t, 2, consumeCalls)
		require.Equal(t, []int{12, 42}, consumedTotals)
	case <-time.After(3 * time.Second):
		t.Fatal("Realtime handler did not stop after quota sync failure")
	}
}

func TestOpenAIRealtimeTailQuotaSyncErrorIsNotSilentlyIgnored(t *testing.T) {
	clientPeer, relayClient, closeClientPair := newRealtimeWebSocketPair(t)
	defer closeClientPair()
	relayTarget, upstreamPeer, closeTargetPair := newRealtimeWebSocketPair(t)
	defer closeTargetPair()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: "/v1/realtime", RawQuery: "model=custom-realtime"},
		Header: make(http.Header),
	}
	c.Set(common.RequestIdKey, "realtime-tail-quota-sync")

	info := &relaycommon.RelayInfo{
		ClientWs: clientPeer, TargetWs: relayTarget, OriginModelName: "custom-realtime",
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "custom-realtime"},
	}

	consumeCalls := 0
	consumeQuota := func(_ *gin.Context, _ *relaycommon.RelayInfo, _ *dto.RealtimeUsage) error {
		consumeCalls++
		return fmt.Errorf("%w: test tail persistence window", model.ErrUserQuotaCacheSync)
	}

	type handlerResult struct {
		err   *types.NewAPIError
		usage *dto.RealtimeUsage
	}
	handlerDone := make(chan handlerResult, 1)
	go func() {
		relayErr, usage := openaiRealtimeHandlerWithQuotaConsumer(c, info, consumeQuota)
		handlerDone <- handlerResult{err: relayErr, usage: usage}
	}()

	delta := []byte(`{"type":"response.audio_transcript.delta","delta":"tail usage must be charged"}`)
	require.NoError(t, upstreamPeer.WriteMessage(websocket.TextMessage, delta))
	require.NoError(t, relayClient.SetReadDeadline(time.Now().Add(3*time.Second)))
	_, deltaPayload, err := relayClient.ReadMessage()
	require.NoError(t, err)
	require.JSONEq(t, string(delta), string(deltaPayload))

	require.NoError(t, upstreamPeer.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"),
		time.Now().Add(time.Second),
	))

	_, errorPayload, err := relayClient.ReadMessage()
	require.NoError(t, err)
	var errorEvent dto.RealtimeEvent
	require.NoError(t, common.Unmarshal(errorPayload, &errorEvent))
	require.Equal(t, dto.RealtimeEventTypeError, errorEvent.Type)
	require.NotNil(t, errorEvent.Error)
	require.Equal(t, string(types.ErrorCodeQueryDataError), errorEvent.Error.Code)

	_, _, err = relayClient.ReadMessage()
	var closeErr *websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, websocket.CloseTryAgainLater, closeErr.Code)

	select {
	case result := <-handlerDone:
		require.NotNil(t, result.err)
		require.ErrorIs(t, result.err, model.ErrUserQuotaCacheSync)
		require.Equal(t, http.StatusServiceUnavailable, result.err.StatusCode)
		require.NotNil(t, result.usage)
		require.Zero(t, result.usage.TotalTokens)
		require.Equal(t, 1, consumeCalls)
	case <-time.After(3 * time.Second):
		t.Fatal("Realtime handler did not stop after tail quota sync failure")
	}
}

func TestOpenAIRealtimeReservationErrorIsNotSilentlyIgnored(t *testing.T) {
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
	c.Set(common.RequestIdKey, "realtime-reservation-error")
	info := &relaycommon.RelayInfo{
		ClientWs: clientPeer, TargetWs: relayTarget, OriginModelName: "gpt-realtime",
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-realtime"},
	}

	consumeCalls := 0
	consumeQuota := func(_ *gin.Context, _ *relaycommon.RelayInfo, _ *dto.RealtimeUsage) error {
		consumeCalls++
		return errors.New("forced realtime reservation failure")
	}
	type handlerResult struct {
		err   *types.NewAPIError
		usage *dto.RealtimeUsage
	}
	handlerDone := make(chan handlerResult, 1)
	go func() {
		relayErr, usage := openaiRealtimeHandlerWithQuotaConsumer(c, info, consumeQuota)
		handlerDone <- handlerResult{err: relayErr, usage: usage}
	}()

	responseDone := []byte(`{"type":"response.done","response":{"usage":{"total_tokens":12,"input_tokens":7,"output_tokens":5,"input_token_details":{"text_tokens":7},"output_token_details":{"text_tokens":5}}}}`)
	require.NoError(t, upstreamPeer.WriteMessage(websocket.TextMessage, responseDone))
	require.NoError(t, relayClient.SetReadDeadline(time.Now().Add(3*time.Second)))
	_, errorPayload, err := relayClient.ReadMessage()
	require.NoError(t, err)
	var errorEvent dto.RealtimeEvent
	require.NoError(t, common.Unmarshal(errorPayload, &errorEvent))
	require.Equal(t, dto.RealtimeEventTypeError, errorEvent.Type)
	require.NotNil(t, errorEvent.Error)
	require.Equal(t, string(types.ErrorCodeUpdateDataError), errorEvent.Error.Code)

	_, _, err = relayClient.ReadMessage()
	var closeErr *websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, websocket.CloseTryAgainLater, closeErr.Code)

	select {
	case result := <-handlerDone:
		require.NotNil(t, result.err)
		require.Equal(t, types.ErrorCodeUpdateDataError, result.err.GetErrorCode())
		require.Equal(t, http.StatusServiceUnavailable, result.err.StatusCode)
		require.True(t, types.IsSkipRetryError(result.err))
		require.NotNil(t, result.usage)
		require.Zero(t, result.usage.TotalTokens)
		require.Equal(t, 1, consumeCalls)
	case <-time.After(3 * time.Second):
		t.Fatal("Realtime handler did not stop after reservation failure")
	}
}

func TestOpenAIRealtimeWaitsForReadersBeforeTailSettlement(t *testing.T) {
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

	consumeStarted := make(chan struct{})
	releaseConsume := make(chan struct{})
	var consumeCalls atomic.Int32
	consumeQuota := func(_ *gin.Context, _ *relaycommon.RelayInfo, _ *dto.RealtimeUsage) error {
		if consumeCalls.Add(1) == 1 {
			close(consumeStarted)
		}
		<-releaseConsume
		return nil
	}
	type handlerResult struct {
		err   *types.NewAPIError
		usage *dto.RealtimeUsage
	}
	handlerDone := make(chan handlerResult, 1)
	go func() {
		relayErr, usage := openaiRealtimeHandlerWithQuotaConsumer(c, info, consumeQuota)
		handlerDone <- handlerResult{err: relayErr, usage: usage}
	}()

	responseDone := []byte(`{"type":"response.done","response":{"usage":{"total_tokens":12,"input_tokens":7,"output_tokens":5,"input_token_details":{"text_tokens":7},"output_token_details":{"text_tokens":5}}}}`)
	require.NoError(t, upstreamPeer.WriteMessage(websocket.TextMessage, responseDone))
	select {
	case <-consumeStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("Realtime quota consumer did not start")
	}
	require.NoError(t, relayClient.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "client done"),
		time.Now().Add(time.Second),
	))

	select {
	case <-handlerDone:
		t.Fatal("Realtime handler returned before the target reader stopped")
	case <-time.After(150 * time.Millisecond):
	}
	close(releaseConsume)

	select {
	case result := <-handlerDone:
		require.Nil(t, result.err)
		require.NotNil(t, result.usage)
		require.Equal(t, 12, result.usage.TotalTokens)
		require.EqualValues(t, 1, consumeCalls.Load())
	case <-time.After(3 * time.Second):
		t.Fatal("Realtime handler did not stop after both readers exited")
	}
}
