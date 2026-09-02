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

package relay

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesWebsocketSinkWritesSSEJSONAsOneTextFrame(t *testing.T) {
	server, client, sinkErr := openResponsesWebsocketSink(t, func(sink ResponsesWebsocketSink) error {
		return sink.WriteSSEData(`data: {"type":"response.output_text.delta","delta":"hello"}` + "\n\n")
	})
	defer server.Close()
	defer client.Close()

	require.NoError(t, <-sinkErr)
	_, payload, err := client.ReadMessage()
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"response.output_text.delta","delta":"hello"}`, string(payload))
}

func TestResponsesWebsocketSinkDoesNotDuplicateTerminalEventForDoneMarker(t *testing.T) {
	server, client, sinkErr := openResponsesWebsocketSink(t, func(sink ResponsesWebsocketSink) error {
		if err := sink.WriteSSEData(`data: {"type":"response.completed","response":{"id":"resp-1"}}` + "\n\n"); err != nil {
			return err
		}
		return sink.WriteSSEData("data: [DONE]\n\n")
	})
	defer server.Close()
	defer client.Close()

	require.NoError(t, <-sinkErr)
	client.SetReadDeadline(time.Now().Add(time.Second))
	_, first, err := client.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(first), `"type":"response.completed"`)
	_, _, err = client.ReadMessage()
	assert.Error(t, err)
}

func TestResponsesWebsocketSinkMapsDoneMarkerWhenNoTerminalEventExists(t *testing.T) {
	server, client, sinkErr := openResponsesWebsocketSink(t, func(sink ResponsesWebsocketSink) error {
		return sink.WriteSSEData("data: [DONE]\n\n")
	})
	defer server.Close()
	defer client.Close()

	require.NoError(t, <-sinkErr)
	_, payload, err := client.ReadMessage()
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"response.done"}`, string(payload))
}

func TestResponsesWebsocketSinkRejectsNonJSONSSEData(t *testing.T) {
	server, client, sinkErr := openResponsesWebsocketSink(t, func(sink ResponsesWebsocketSink) error {
		return sink.WriteSSEData("data: not-json\n\n")
	})
	defer server.Close()
	defer client.Close()

	err := <-sinkErr
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "JSON"))
	client.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, _, readErr := client.ReadMessage()
	assert.Error(t, readErr)
}

func TestResponsesWebsocketSinkWritesStableErrorEvent(t *testing.T) {
	server, client, sinkErr := openResponsesWebsocketSink(t, func(sink ResponsesWebsocketSink) error {
		return sink.WriteError(relaytypes.NewError(errors.New("upstream failed"), relaytypes.ErrorCodeBadResponse))
	})
	defer server.Close()
	defer client.Close()

	require.NoError(t, <-sinkErr)
	_, payload, err := client.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(payload), `"type":"error"`)
	assert.Contains(t, string(payload), "upstream failed")
}

func openResponsesWebsocketSink(t *testing.T, write func(ResponsesWebsocketSink) error) (*httptest.Server, *websocket.Conn, <-chan error) {
	t.Helper()
	result := make(chan error, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			result <- err
			return
		}
		defer conn.Close()
		sink := NewResponsesWebsocketSink(conn)
		result <- write(sink)
	}))

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	return server, client, result
}
