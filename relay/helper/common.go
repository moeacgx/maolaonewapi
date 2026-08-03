package helper

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const timingDiagnosticsRelayInfoKey = "timing_diagnostics_relay_info"

var eventStreamHeaderNames = []string{
	"Content-Type",
	"Cache-Control",
	"Connection",
	"Transfer-Encoding",
	"X-Accel-Buffering",
}

func SetTimingDiagnosticsRelayInfo(c *gin.Context, info *relaycommon.RelayInfo) {
	if c == nil || info == nil {
		return
	}
	c.Set(timingDiagnosticsRelayInfoKey, info)
}

func markFirstDownstreamWrite(c *gin.Context) {
	if c == nil {
		return
	}
	if info, ok := c.Get(timingDiagnosticsRelayInfoKey); ok {
		if relayInfo, ok := info.(*relaycommon.RelayInfo); ok && relayInfo != nil {
			relayInfo.MarkTimingFirstDownstreamWrite()
		}
	}
}

func FlushWriter(c *gin.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("flush panic recovered: %v", r)
		}
	}()

	if c == nil || c.Writer == nil {
		return nil
	}

	if requestContextDone(c) {
		return fmt.Errorf("request context done: %w", c.Request.Context().Err())
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return errors.New("streaming error: flusher not found")
	}

	flusher.Flush()
	return nil
}

func requestContextDone(c *gin.Context) bool {
	return c != nil && c.Request != nil && c.Request.Context().Err() != nil
}

func SetEventStreamHeaders(c *gin.Context) {
	// 检查是否已经设置过头部
	if c == nil || c.Writer == nil || c.GetBool("event_stream_headers_set") {
		return
	}

	// 设置标志，表示头部已经设置过
	c.Set("event_stream_headers_set", true)

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
}

// ResetEventStreamHeadersForRetry 清理尚未提交的 SSE 头，使下一渠道可以按
// 实际响应类型重新设置响应头。已提交响应不能安全重置，因此保持原样。
func ResetEventStreamHeadersForRetry(c *gin.Context) {
	if c == nil || c.Writer == nil || c.Writer.Written() {
		return
	}
	for _, header := range eventStreamHeaderNames {
		c.Writer.Header().Del(header)
	}
	c.Set("event_stream_headers_set", false)
}

func ClaudeData(c *gin.Context, resp dto.ClaudeResponse) error {
	if requestContextDone(c) {
		return nil
	}

	if c.GetBool("sensitive_response_stream_blocked") {
		return nil
	}
	jsonData, err := common.Marshal(resp)
	if err != nil {
		common.SysError("error marshalling stream response: " + err.Error())
	} else {
		blocked, wrote, err := writeFilteredEventData(c, fmt.Sprintf("event: %s\n", resp.Type), string(jsonData))
		if blocked || err != nil {
			return err
		}
		if !wrote {
			return nil
		}
	}
	return FlushWriter(c)
}

func ClaudeChunkData(c *gin.Context, resp dto.ClaudeResponse, data string) {
	if requestContextDone(c) {
		return
	}

	if c.GetBool("sensitive_response_stream_blocked") {
		return
	}
	blocked, wrote, err := writeFilteredEventData(c, fmt.Sprintf("event: %s\n", resp.Type), data)
	if blocked || err != nil || !wrote {
		return
	}
	_ = FlushWriter(c)
}

func ResponseChunkData(c *gin.Context, resp dto.ResponsesStreamResponse, data string) error {
	return ResponseChunkDataBatch(c, []ResponseChunkDataItem{{Response: resp, Data: data}})
}

type ResponseChunkDataItem struct {
	Response dto.ResponsesStreamResponse
	Data     string
}

func ResponseChunkDataBatch(c *gin.Context, chunks []ResponseChunkDataItem) error {
	if c == nil || c.Writer == nil {
		return errors.New("context or writer is nil")
	}
	if requestContextDone(c) {
		return fmt.Errorf("request context done: %w", c.Request.Context().Err())
	}

	if c.GetBool("sensitive_response_stream_blocked") {
		return nil
	}
	items := make([]service.SensitiveStreamDataItem, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk.Data == "" {
			continue
		}
		items = append(items, service.SensitiveStreamDataItem{
			Data: chunk.Data, EventLine: fmt.Sprintf("event: %s\n", chunk.Response.Type),
		})
	}
	blocked, wrote, err := writeFilteredStreamDataBatch(c, items)
	if blocked || err != nil {
		return err
	}
	if !wrote {
		return nil
	}
	return FlushWriter(c)
}

func StringData(c *gin.Context, str string) error {
	return StringDataBatch(c, []string{str})
}

func StringDataBatch(c *gin.Context, data []string) error {
	if c == nil || c.Writer == nil {
		return errors.New("context or writer is nil")
	}
	if requestContextDone(c) {
		return fmt.Errorf("request context done: %w", c.Request.Context().Err())
	}
	if c.GetBool("sensitive_response_stream_blocked") {
		return nil
	}

	items := make([]service.SensitiveStreamDataItem, 0, len(data))
	for _, item := range data {
		if item != "" {
			items = append(items, service.SensitiveStreamDataItem{Data: item})
		}
	}
	blocked, wrote, err := writeFilteredStreamDataBatch(c, items)
	if blocked || err != nil {
		return err
	}
	if !wrote {
		return nil
	}
	return FlushWriter(c)
}

func PingData(c *gin.Context) error {
	if c == nil || c.Writer == nil {
		return errors.New("context or writer is nil")
	}

	if requestContextDone(c) {
		return fmt.Errorf("request context done: %w", c.Request.Context().Err())
	}

	if _, err := c.Writer.Write([]byte(": PING\n\n")); err != nil {
		return fmt.Errorf("write ping data failed: %w", err)
	}
	return FlushWriter(c)
}

func ObjectData(c *gin.Context, object interface{}) error {
	if object == nil {
		return errors.New("object is nil")
	}
	jsonData, err := common.Marshal(object)
	if err != nil {
		return fmt.Errorf("error marshalling object: %w", err)
	}
	return StringData(c, string(jsonData))
}

func writeFilteredEventData(c *gin.Context, eventLine string, data string) (blocked bool, wrote bool, err error) {
	return writeFilteredStreamDataBatch(c, []service.SensitiveStreamDataItem{{
		Data: data, EventLine: eventLine,
	}})
}

func writeFilteredStreamDataBatch(c *gin.Context, items []service.SensitiveStreamDataItem) (blocked bool, wrote bool, err error) {
	for _, item := range items {
		service.RecordUpstreamPolicyPayload(c, []byte(item.Data), "response_stream")
	}
	result, err := service.ApplySensitiveFilterToStreamDataBatchForSend(c, items)
	if err != nil {
		return false, false, err
	}
	if result.Blocked {
		logger.LogWarn(c, fmt.Sprintf("upstream stream response blocked by sensitive rules: %s", service.FormatSensitiveFilterMatches(result.Matches)))
		c.Set("sensitive_response_stream_blocked", true)
		writeErr := writeSensitiveStreamErrorEvent(c)
		flushErr := FlushWriter(c)
		if writeErr != nil {
			return true, false, writeErr
		}
		return true, true, flushErr
	}
	if err := writeSensitiveStreamDataItems(c, result.Items); err != nil {
		return false, false, err
	}
	return false, len(result.Items) > 0, nil
}

func writeSensitiveStreamEventData(c *gin.Context, data string) error {
	if c == nil || c.Writer == nil {
		return errors.New("context or writer is nil")
	}

	common.CustomEvent{}.WriteContentType(c.Writer)
	encoded := strings.ReplaceAll(data, "\r", "\\r")
	if err := writeSensitiveStreamString(c.Writer, encoded); err != nil {
		return fmt.Errorf("write event stream data failed: %w", err)
	}
	if strings.HasPrefix(data, "data") {
		if err := writeSensitiveStreamString(c.Writer, "\n\n"); err != nil {
			return fmt.Errorf("write event stream delimiter failed: %w", err)
		}
	}
	return nil
}

func writeSensitiveStreamString(writer gin.ResponseWriter, data string) error {
	written, err := writer.Write([]byte(data))
	if err != nil {
		return err
	}
	if written != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

func writeSensitiveStreamDataItems(c *gin.Context, items []service.SensitiveStreamDataItem) error {
	for _, item := range items {
		markFirstDownstreamWrite(c)
		if item.EventLine != "" {
			if err := writeSensitiveStreamEventData(c, item.EventLine); err != nil {
				return err
			}
		}
		if err := writeSensitiveStreamEventData(c, "data: "+item.Data); err != nil {
			return err
		}
	}
	return nil
}

func writeSensitiveStreamErrorEvent(c *gin.Context) error {
	if err := writeSensitiveStreamEventData(c, "event: error\n"); err != nil {
		return err
	}
	return writeSensitiveStreamEventData(c, "data: "+string(service.SensitiveFilterSSEOpenAIErrorBody(c)))
}

// FlushSensitiveStreamData 只刷新敏感词过滤器暂存的流数据，不追加 [DONE]。
// 调用方可据此区分正常结束、客户端取消和内容审计拦截。
func FlushSensitiveStreamData(c *gin.Context) error {
	if c == nil || c.Writer == nil {
		return errors.New("context or writer is nil")
	}
	if requestContextDone(c) {
		return fmt.Errorf("request context done: %w", c.Request.Context().Err())
	}
	if c.GetBool("sensitive_response_stream_blocked") {
		return service.ErrSensitiveResponseBlocked
	}

	result, err := service.FlushSensitiveStreamDataForSend(c)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("flush sensitive stream buffer failed: %s", err.Error()))
		c.Set("sensitive_response_stream_blocked", true)
		writeErr := writeSensitiveStreamErrorEvent(c)
		flushErr := FlushWriter(c)
		if writeErr != nil || flushErr != nil {
			return errors.Join(err, writeErr, flushErr)
		}
		return err
	}
	if result.Blocked {
		logger.LogWarn(c, fmt.Sprintf("upstream stream response blocked by sensitive rules: %s", service.FormatSensitiveFilterMatches(result.Matches)))
		c.Set("sensitive_response_stream_blocked", true)
		writeErr := writeSensitiveStreamErrorEvent(c)
		flushErr := FlushWriter(c)
		if writeErr != nil || flushErr != nil {
			return errors.Join(service.ErrSensitiveResponseBlocked, writeErr, flushErr)
		}
		return service.ErrSensitiveResponseBlocked
	}
	if err := writeSensitiveStreamDataItems(c, result.Items); err != nil {
		return err
	}
	if len(result.Items) == 0 {
		return nil
	}
	return FlushWriter(c)
}

func Done(c *gin.Context) {
	if err := FlushSensitiveStreamData(c); err != nil {
		return
	}
	_ = StringData(c, "[DONE]")
}

func WssString(c *gin.Context, ws *websocket.Conn, str string) error {
	if ws == nil {
		logger.LogError(c, "websocket connection is nil")
		return errors.New("websocket connection is nil")
	}
	//common.LogInfo(c, fmt.Sprintf("sending message: %s", str))
	return ws.WriteMessage(1, []byte(str))
}

func WssObject(c *gin.Context, ws *websocket.Conn, object interface{}) error {
	jsonData, err := common.Marshal(object)
	if err != nil {
		return fmt.Errorf("error marshalling object: %w", err)
	}
	if ws == nil {
		logger.LogError(c, "websocket connection is nil")
		return errors.New("websocket connection is nil")
	}
	//common.LogInfo(c, fmt.Sprintf("sending message: %s", jsonData))
	return ws.WriteMessage(1, jsonData)
}

func WssError(c *gin.Context, ws *websocket.Conn, openaiError types.OpenAIError) {
	if ws == nil {
		return
	}
	errorObj := &dto.RealtimeEvent{
		Type:    "error",
		EventId: GetLocalRealtimeID(c),
		Error:   &openaiError,
	}
	_ = WssObject(c, ws, errorObj)
}

func GetResponseID(c *gin.Context) string {
	logID := c.GetString(common.RequestIdKey)
	return fmt.Sprintf("chatcmpl-%s", logID)
}

func GetLocalRealtimeID(c *gin.Context) string {
	logID := c.GetString(common.RequestIdKey)
	return fmt.Sprintf("evt_%s", logID)
}

func GenerateStartEmptyResponse(id string, createAt int64, model string, systemFingerprint *string) *dto.ChatCompletionsStreamResponse {
	return &dto.ChatCompletionsStreamResponse{
		Id:                id,
		Object:            "chat.completion.chunk",
		Created:           createAt,
		Model:             model,
		SystemFingerprint: systemFingerprint,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					Role:    "assistant",
					Content: common.GetPointer(""),
				},
			},
		},
	}
}

func GenerateStopResponse(id string, createAt int64, model string, finishReason string) *dto.ChatCompletionsStreamResponse {
	return &dto.ChatCompletionsStreamResponse{
		Id:                id,
		Object:            "chat.completion.chunk",
		Created:           createAt,
		Model:             model,
		SystemFingerprint: nil,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				FinishReason: &finishReason,
			},
		},
	}
}

func GenerateFinalUsageResponse(id string, createAt int64, model string, usage dto.Usage) *dto.ChatCompletionsStreamResponse {
	return &dto.ChatCompletionsStreamResponse{
		Id:                id,
		Object:            "chat.completion.chunk",
		Created:           createAt,
		Model:             model,
		SystemFingerprint: nil,
		Choices:           make([]dto.ChatCompletionsStreamResponseChoice, 0),
		Usage:             &usage,
	}
}
