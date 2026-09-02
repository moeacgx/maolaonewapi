/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var responsesWebsocketUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(*http.Request) bool {
		return true
	},
}

// ResponsesWebsocket handles the downstream Responses WebSocket session.
// Each incoming frame is run through the existing Relay pipeline as one
// temporary POST request, preserving normal billing and retry behavior.
func ResponsesWebsocket(c *gin.Context) {
	if !common.ResponsesWebsocketEnabled {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	conn, err := responsesWebsocketUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	state := relay.ResponsesWebsocketState{}
	connectionID := strings.TrimSpace(c.Request.Header.Get("X-Client-Request-ID"))
	if connectionID == "" {
		connectionID = common.NewRequestId()
	}
	seenRequestIDs := make(map[string]struct{})
	c.Set(responsesWebsocketConnectionKey, conn)
	var writeMu sync.Mutex
	baseRequest := c.Request.Clone(context.Background())
	baseKeys := cloneGinKeys(c)
	readResults := make(chan responsesWebsocketReadResult, 1)
	startRead := func() {
		go func() {
			messageType, payload, readErr := conn.ReadMessage()
			readResults <- responsesWebsocketReadResult{messageType: messageType, payload: payload, err: readErr}
		}()
	}
	startRead()
	var turnResults <-chan responsesWebsocketTurnResult

	for {
		select {
		case read := <-readResults:
			if read.err != nil {
				cancel()
				if turnResults != nil {
					<-turnResults
				}
				return
			}
			if read.messageType != websocket.TextMessage && read.messageType != websocket.BinaryMessage {
				startRead()
				continue
			}
			if turnResults != nil {
				writeResponsesWebsocketEvent(&writeMu, conn, relay.ResponsesWebsocketErrorPayload("turn_in_progress", "wait for the current response to complete"))
				startRead()
				continue
			}
			normalized, nextState, normalizeErr := relay.NormalizeResponsesWebsocketRequest(read.payload, state)
			if normalizeErr != nil {
				writeResponsesWebsocketEvent(&writeMu, conn, relay.ResponsesWebsocketErrorPayload("invalid_request", normalizeErr.Error()))
				startRead()
				continue
			}
			requestID := responsesWebsocketRequestID(read.payload, connectionID)
			if _, seen := seenRequestIDs[requestID]; seen {
				writeResponsesWebsocketEvent(&writeMu, conn, relay.ResponsesWebsocketErrorPayload("duplicate_request_id", "request has already been processed"))
				startRead()
				continue
			}
			seenRequestIDs[requestID] = struct{}{}
			resultChannel := make(chan responsesWebsocketTurnResult, 1)
			turnResults = resultChannel
			go func() {
				resultChannel <- runResponsesWebsocketTurn(baseRequest, baseKeys, conn, normalized, nextState, requestID, ctx, &writeMu)
			}()
			startRead()
		case result := <-turnResults:
			if result.err != nil {
				cancel()
				return
			}
			state = result.state
			turnResults = nil
		}
	}
}

const responsesWebsocketConnectionKey = "responses_websocket_connection"

type responsesWebsocketReadResult struct {
	messageType int
	payload     []byte
	err         error
}

type responsesWebsocketTurnResult struct {
	state relay.ResponsesWebsocketState
	err   error
}

func runResponsesWebsocketTurn(baseRequest *http.Request, baseKeys map[string]any, conn *websocket.Conn, frame []byte, state relay.ResponsesWebsocketState, requestID string, ctx context.Context, writeMu *sync.Mutex) responsesWebsocketTurnResult {
	sink := relay.NewResponsesWebsocketSinkWithWriteLock(conn, writeMu)
	turnRequest := buildResponsesWebsocketTurnRequest(baseRequest, frame)
	turnRequest = turnRequest.WithContext(ctx)
	turnWriter := newResponsesWebsocketHTTPWriter(sink)
	turnEngine := gin.New()
	turnEngine.Use(func(turnContext *gin.Context) {
		for key, value := range baseKeys {
			turnContext.Set(key, value)
		}
		turnContext.Set(common.RequestIdKey, requestID)
		turnContext.Set("downstream_transport", "websocket")
		turnContext.Set("upstream_transport", "http")
		turnContext.Next()
	})
	turnEngine.Use(middleware.ModelRequestRateLimit(), middleware.PromptAudit(), middleware.Distribute())
	turnEngine.POST("/v1/responses", func(turnContext *gin.Context) {
		Relay(turnContext, types.RelayFormatOpenAIResponses)
	})
	turnEngine.ServeHTTP(turnWriter, turnRequest)
	if turnWriter.writeErr != nil {
		return responsesWebsocketTurnResult{state: state, err: turnWriter.writeErr}
	}
	if snapshotter, ok := sink.(interface {
		Snapshot() relay.ResponsesWebsocketSnapshot
	}); ok {
		snapshot := snapshotter.Snapshot()
		state.LastResponseID = snapshot.LastResponseID
		state.LastResponseOutput = snapshot.LastResponseOutput
	}
	return responsesWebsocketTurnResult{state: state}
}

func cloneGinKeys(c *gin.Context) map[string]any {
	keys := make(map[string]any)
	if c == nil {
		return keys
	}
	for key, value := range c.Keys {
		keys[key] = value
	}
	return keys
}

func writeResponsesWebsocketEvent(writeMu *sync.Mutex, conn *websocket.Conn, payload []byte) {
	if conn == nil {
		return
	}
	if writeMu != nil {
		writeMu.Lock()
		defer writeMu.Unlock()
	}
	_ = conn.WriteMessage(websocket.TextMessage, payload)
}

func responsesWebsocketRequestID(frame []byte, connectionID string) string {
	var envelope map[string]json.RawMessage
	if common.Unmarshal(frame, &envelope) == nil {
		if raw, ok := envelope["request_id"]; ok {
			var explicit string
			if common.Unmarshal(raw, &explicit) == nil {
				explicit = strings.TrimSpace(explicit)
				if explicit != "" && len(explicit) <= 128 {
					return explicit
				}
			}
		}
	}
	connectionID = strings.TrimSpace(connectionID)
	if connectionID == "" {
		connectionID = "responses-websocket"
	}
	return connectionID + "-" + common.NewRequestId()
}

func responsesWebsocketConnection(c *gin.Context) *websocket.Conn {
	if c == nil {
		return nil
	}
	if conn, ok := common.GetContextKeyType[*websocket.Conn](c, responsesWebsocketConnectionKey); ok {
		return conn
	}
	return nil
}

func buildResponsesWebsocketTurnRequest(base *http.Request, body []byte) *http.Request {
	if base == nil {
		base = &http.Request{}
	}
	request := base.Clone(base.Context())
	request.Method = http.MethodPost
	if request.URL == nil {
		request.URL = &url.URL{Path: "/v1/responses"}
	} else {
		urlCopy := *request.URL
		urlCopy.Path = "/v1/responses"
		request.URL = &urlCopy
	}
	request.Header = request.Header.Clone()
	for key := range request.Header {
		if strings.HasPrefix(strings.ToLower(key), "sec-websocket-") {
			request.Header.Del(key)
		}
	}
	request.Header.Del("Upgrade")
	request.Header.Del("Connection")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	request.ContentLength = int64(len(body))
	return request
}

type responsesWebsocketHTTPWriter struct {
	sink     relay.ResponsesWebsocketSink
	header   http.Header
	status   int
	size     int
	written  bool
	writeErr error
}

func newResponsesWebsocketHTTPWriter(sink relay.ResponsesWebsocketSink) *responsesWebsocketHTTPWriter {
	return &responsesWebsocketHTTPWriter{sink: sink, header: make(http.Header)}
}

// newResponsesWebsocketResponseWriter is retained for focused controller
// tests and callers in this package; the bridge now implements the raw
// http.ResponseWriter contract consumed by an isolated Gin turn engine.
func newResponsesWebsocketResponseWriter(_ gin.ResponseWriter, sink relay.ResponsesWebsocketSink) *responsesWebsocketHTTPWriter {
	return newResponsesWebsocketHTTPWriter(sink)
}

func (w *responsesWebsocketHTTPWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *responsesWebsocketHTTPWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
}

func (w *responsesWebsocketHTTPWriter) WriteHeaderNow() {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.written = true
}

func (w *responsesWebsocketHTTPWriter) Write(p []byte) (int, error) {
	if w.sink == nil {
		return 0, errors.New("websocket sink is nil")
	}
	w.WriteHeaderNow()
	var err error
	trimmed := bytes.TrimSpace(p)
	if bytes.HasPrefix(trimmed, []byte("data:")) {
		err = w.sink.WriteSSEData(string(p))
	} else {
		err = w.writeRawJSON(trimmed)
	}
	if err != nil {
		w.writeErr = err
		return len(p), err
	}
	w.size += len(p)
	return len(p), nil
}

func (w *responsesWebsocketHTTPWriter) WriteString(value string) (int, error) {
	return w.Write([]byte(value))
}

func (w *responsesWebsocketHTTPWriter) Status() int { return w.status }

func (w *responsesWebsocketHTTPWriter) Size() int { return w.size }

func (w *responsesWebsocketHTTPWriter) Written() bool { return w.written }

func (w *responsesWebsocketHTTPWriter) Flush() { w.WriteHeaderNow() }

func (w *responsesWebsocketHTTPWriter) writeRawJSON(payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	var envelope map[string]json.RawMessage
	if err := common.Unmarshal(payload, &envelope); err != nil {
		return fmt.Errorf("invalid websocket response JSON: %w", err)
	}
	if _, hasType := envelope["type"]; hasType {
		return w.sink.WriteEvent(payload)
	}
	if rawError, hasError := envelope["error"]; hasError {
		wrapped, err := common.Marshal(struct {
			Type  string          `json:"type"`
			Error json.RawMessage `json:"error"`
		}{Type: "error", Error: rawError})
		if err != nil {
			return err
		}
		return w.sink.WriteEvent(wrapped)
	}
	return errors.New("websocket response JSON is missing type")
}
