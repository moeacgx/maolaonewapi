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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gorilla/websocket"
)

const responsesWebsocketDoneMarker = "[DONE]"

// ResponsesWebsocketSink serializes Responses events onto a client WebSocket.
// WriteSSEData is used by the existing HTTP/SSE adaptor bridge.
type ResponsesWebsocketSink interface {
	WriteEvent([]byte) error
	WriteSSEData(string) error
	WriteError(*relaytypes.NewAPIError) error
	MarkTerminal() bool
}

type responsesWebsocketSink struct {
	conn     *websocket.Conn
	mu       sync.Mutex
	terminal bool
}

// NewResponsesWebsocketSink creates a serialized event sink for one client
// WebSocket connection.
func NewResponsesWebsocketSink(conn *websocket.Conn) ResponsesWebsocketSink {
	return &responsesWebsocketSink{conn: conn}
}

func (s *responsesWebsocketSink) WriteEvent(payload []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeEventLocked(payload)
}

func (s *responsesWebsocketSink) WriteSSEData(data string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn == nil {
		return errors.New("websocket sink connection is nil")
	}
	normalized := strings.ReplaceAll(data, "\r\n", "\n")
	for _, block := range strings.Split(normalized, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" || strings.HasPrefix(block, ":") {
			continue
		}
		payload, ok := responsesWebsocketSSEPayload(block)
		if !ok {
			continue
		}
		if payload == responsesWebsocketDoneMarker {
			if s.terminal {
				continue
			}
			if err := s.writeEventLocked([]byte(`{"type":"response.done"}`)); err != nil {
				return err
			}
			continue
		}
		if err := s.writeEventLocked([]byte(payload)); err != nil {
			return fmt.Errorf("write websocket SSE event: %w", err)
		}
	}
	return nil
}

func (s *responsesWebsocketSink) WriteError(relayErr *relaytypes.NewAPIError) error {
	if relayErr == nil {
		return errors.New("websocket sink error is nil")
	}
	clientError := relayErr.ToOpenAIError()
	payload, err := common.Marshal(struct {
		Type           string `json:"type"`
		SequenceNumber int64  `json:"sequence_number,omitempty"`
		Code           any    `json:"code,omitempty"`
		Message        string `json:"message"`
		Param          string `json:"param,omitempty"`
	}{
		Type:    "error",
		Code:    clientError.Code,
		Message: clientError.Message,
		Param:   clientError.Param,
	})
	if err != nil {
		return err
	}
	return s.WriteEvent(payload)
}

func (s *responsesWebsocketSink) MarkTerminal() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.terminal
}

func (s *responsesWebsocketSink) writeEventLocked(payload []byte) error {
	if s.conn == nil {
		return errors.New("websocket sink connection is nil")
	}
	if s.terminal {
		return nil
	}
	var envelope map[string]json.RawMessage
	if err := common.Unmarshal(payload, &envelope); err != nil || len(envelope) == 0 {
		if err == nil {
			err = errors.New("event must be a JSON object")
		}
		return fmt.Errorf("invalid Responses event JSON: %w", err)
	}
	eventType, ok := responsesWebsocketEventType(envelope)
	if !ok {
		return errors.New("Responses event JSON is missing type")
	}
	if err := s.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		return err
	}
	if responsesWebsocketIsTerminalEvent(eventType) {
		s.terminal = true
	}
	return nil
}

func responsesWebsocketSSEPayload(block string) (string, bool) {
	lines := strings.Split(block, "\n")
	dataLines := make([]string, 0, 1)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if len(dataLines) == 0 {
		return "", false
	}
	return strings.Join(dataLines, "\n"), true
}

func responsesWebsocketEventType(envelope map[string]json.RawMessage) (string, bool) {
	raw, ok := envelope["type"]
	if !ok {
		return "", false
	}
	var eventType string
	if err := common.Unmarshal(raw, &eventType); err != nil || strings.TrimSpace(eventType) == "" {
		return "", false
	}
	return strings.TrimSpace(eventType), true
}

func responsesWebsocketIsTerminalEvent(eventType string) bool {
	switch eventType {
	case "response.completed", "response.done", "response.incomplete", "response.cancelled", "response.canceled", "error":
		return true
	default:
		return false
	}
}
