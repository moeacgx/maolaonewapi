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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type responsesWebsocketSinkTestDouble struct {
	events   [][]byte
	sseData  []string
	errors   []*relaytypes.NewAPIError
	terminal bool
}

func (s *responsesWebsocketSinkTestDouble) WriteEvent(payload []byte) error {
	s.events = append(s.events, append([]byte(nil), payload...))
	return nil
}

func (s *responsesWebsocketSinkTestDouble) WriteSSEData(data string) error {
	s.sseData = append(s.sseData, data)
	return nil
}

func (s *responsesWebsocketSinkTestDouble) WriteError(err *relaytypes.NewAPIError) error {
	s.errors = append(s.errors, err)
	return nil
}

func (s *responsesWebsocketSinkTestDouble) MarkTerminal() bool { return s.terminal }

func TestResponsesWebsocketDisabledReturnsNotFoundBeforeUpgrade(t *testing.T) {
	previous := common.ResponsesWebsocketEnabled
	common.ResponsesWebsocketEnabled = false
	t.Cleanup(func() { common.ResponsesWebsocketEnabled = previous })

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)

	ResponsesWebsocket(context)

	assert.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestBuildResponsesWebsocketTurnRequestUsesPostAndJSONBody(t *testing.T) {
	base := httptest.NewRequest(http.MethodGet, "/v1/responses?client=codex", nil)
	base.Header.Set("Authorization", "Bearer sk-test")
	base.Header.Set("Sec-WebSocket-Protocol", "responses")
	base.Header.Set("Sec-WebSocket-Key", "handshake-key")
	body := []byte(`{"model":"gpt-test","input":[],"stream":true}`)

	request := buildResponsesWebsocketTurnRequest(base, body)

	require.Equal(t, http.MethodPost, request.Method)
	assert.Equal(t, "/v1/responses", request.URL.Path)
	assert.Equal(t, "client=codex", request.URL.RawQuery)
	assert.Equal(t, "application/json", request.Header.Get("Content-Type"))
	assert.Equal(t, "text/event-stream", request.Header.Get("Accept"))
	assert.Equal(t, "Bearer sk-test", request.Header.Get("Authorization"))
	assert.Empty(t, request.Header.Get("Sec-WebSocket-Protocol"))
	assert.Empty(t, request.Header.Get("Sec-WebSocket-Key"))
	assert.Equal(t, int64(len(body)), request.ContentLength)
}

func TestResponsesWebsocketRequestIDUsesExplicitFrameID(t *testing.T) {
	requestID := responsesWebsocketRequestID([]byte(`{"request_id":"turn-123"}`), "connection-1")

	assert.Equal(t, "turn-123", requestID)
}

func TestResponsesWebsocketRequestIDPrefixesGeneratedIDWithConnectionID(t *testing.T) {
	requestID := responsesWebsocketRequestID([]byte(`{"type":"response.create"}`), "connection-1")

	assert.NotEmpty(t, requestID)
	assert.Contains(t, requestID, "connection-1")
}

func TestResponsesWebsocketResponseWriterSendsSSEDataToSink(t *testing.T) {
	sink := &responsesWebsocketSinkTestDouble{}
	writer := newResponsesWebsocketResponseWriter(nil, sink)
	payload := []byte("data: {\"type\":\"response.output_text.delta\"}\n\n")

	written, err := writer.Write(payload)

	require.NoError(t, err)
	assert.Equal(t, len(payload), written)
	require.Len(t, sink.sseData, 1)
	assert.Equal(t, string(payload), sink.sseData[0])
}

func TestResponsesWebsocketResponseWriterAcceptsSSEEventLine(t *testing.T) {
	sink := &responsesWebsocketSinkTestDouble{}
	writer := newResponsesWebsocketResponseWriter(nil, sink)

	_, err := writer.Write([]byte("event: response.created\n"))

	require.NoError(t, err)
	assert.Empty(t, sink.events)
	assert.Len(t, sink.sseData, 1)
}

func TestResponsesWebsocketResponseWriterWrapsRawErrorJSON(t *testing.T) {
	sink := &responsesWebsocketSinkTestDouble{}
	writer := newResponsesWebsocketResponseWriter(nil, sink)
	payload := []byte(`{"error":{"message":"upstream failed","type":"server_error"}}`)

	_, err := writer.Write(payload)

	require.NoError(t, err)
	require.Len(t, sink.events, 1)
	assert.Contains(t, string(sink.events[0]), `"type":"error"`)
	assert.Contains(t, string(sink.events[0]), "upstream failed")
}

func TestResponsesWebsocketResponseWriterRejectsNilSink(t *testing.T) {
	writer := newResponsesWebsocketResponseWriter(nil, nil)
	_, err := writer.Write([]byte("data: {}\n\n"))

	assert.EqualError(t, err, "websocket sink is nil")
}
