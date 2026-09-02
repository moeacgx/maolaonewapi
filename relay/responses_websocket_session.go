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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const (
	responsesWebsocketCreateType = "response.create"
	responsesWebsocketAppendType = "response.append"
	responsesWebsocketMaxState   = 4 << 20
)

// ResponsesWebsocketState stores the minimum turn state needed to normalize
// incremental Responses WebSocket messages into ordinary HTTP requests.
type ResponsesWebsocketState struct {
	Model              string
	LastRequest        []byte
	LastResponseID     string
	LastResponseOutput []byte
	PendingToolCallIDs []string
}

type responsesWebsocketErrorEnvelope struct {
	Type  string                        `json:"type"`
	Error responsesWebsocketErrorDetail `json:"error"`
}

type responsesWebsocketErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NormalizeResponsesWebsocketRequest converts a client event into the
// stream=true request consumed by the existing Responses relay pipeline.
func NormalizeResponsesWebsocketRequest(raw []byte, state ResponsesWebsocketState) ([]byte, ResponsesWebsocketState, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, state, errors.New("invalid websocket request: empty payload")
	}
	if len(raw) > responsesWebsocketMaxState {
		return nil, state, errors.New("message_too_big: websocket request exceeds the maximum size")
	}

	var envelope map[string]json.RawMessage
	if err := common.Unmarshal(raw, &envelope); err != nil {
		return nil, state, fmt.Errorf("invalid websocket request: %w", err)
	}
	requestType, err := responsesWebsocketStringField(envelope, "type")
	if err != nil {
		return nil, state, errors.New("invalid websocket request: missing event type")
	}

	switch requestType {
	case responsesWebsocketCreateType:
		if len(state.LastRequest) == 0 {
			return normalizeResponsesWebsocketCreate(envelope, state)
		}
		return normalizeResponsesWebsocketSubsequent(envelope, state)
	case responsesWebsocketAppendType:
		if len(state.LastRequest) == 0 {
			return nil, state, errors.New("websocket request received before response.create")
		}
		return normalizeResponsesWebsocketSubsequent(envelope, state)
	default:
		return nil, state, fmt.Errorf("unsupported websocket request type: %s", requestType)
	}
}

func normalizeResponsesWebsocketCreate(envelope map[string]json.RawMessage, state ResponsesWebsocketState) ([]byte, ResponsesWebsocketState, error) {
	model, err := responsesWebsocketStringField(envelope, "model")
	if err != nil || strings.TrimSpace(model) == "" {
		return nil, state, errors.New("model is required in response.create")
	}

	input, err := responsesWebsocketInputField(envelope, false)
	if err != nil {
		return nil, state, err
	}
	delete(envelope, "type")
	delete(envelope, "previous_response_id")
	envelope["model"] = json.RawMessage(mustMarshalString(model))
	envelope["stream"] = json.RawMessage("true")
	inputBytes, err := common.Marshal(input)
	if err != nil {
		return nil, state, fmt.Errorf("marshal websocket input: %w", err)
	}
	envelope["input"] = inputBytes

	normalized, err := common.Marshal(envelope)
	if err != nil {
		return nil, state, fmt.Errorf("marshal websocket request: %w", err)
	}
	if len(normalized) > responsesWebsocketMaxState {
		return nil, state, errors.New("message_too_big: websocket request exceeds the maximum size")
	}
	state.Model = model
	state.LastRequest = append([]byte(nil), normalized...)
	return normalized, state, nil
}

func normalizeResponsesWebsocketSubsequent(envelope map[string]json.RawMessage, state ResponsesWebsocketState) ([]byte, ResponsesWebsocketState, error) {
	if err := validateResponsesWebsocketPreviousResponse(envelope, state); err != nil {
		return nil, state, err
	}
	input, err := responsesWebsocketInputField(envelope, true)
	if err != nil {
		return nil, state, err
	}

	lastRequest := map[string]json.RawMessage{}
	if err := common.Unmarshal(state.LastRequest, &lastRequest); err != nil {
		return nil, state, fmt.Errorf("invalid saved websocket request: %w", err)
	}
	var mergedInput []json.RawMessage
	if responsesWebsocketIsFullTranscript(input) {
		mergedInput = input
	} else {
		priorInput, err := responsesWebsocketArrayField(lastRequest, "input")
		if err != nil {
			return nil, state, fmt.Errorf("invalid saved websocket input: %w", err)
		}
		priorOutput, err := responsesWebsocketArrayBytes(state.LastResponseOutput)
		if err != nil {
			return nil, state, fmt.Errorf("invalid saved websocket output: %w", err)
		}
		mergedInput = make([]json.RawMessage, 0, len(priorInput)+len(priorOutput)+len(input))
		mergedInput = append(mergedInput, priorInput...)
		mergedInput = append(mergedInput, priorOutput...)
		mergedInput = append(mergedInput, input...)
	}

	delete(envelope, "type")
	delete(envelope, "previous_response_id")
	model := state.Model
	if currentModel, ok := envelope["model"]; ok {
		var value string
		if err := common.Unmarshal(currentModel, &value); err == nil && strings.TrimSpace(value) != "" {
			model = value
		}
	}
	if strings.TrimSpace(model) == "" {
		return nil, state, errors.New("model is required in response.create")
	}
	envelope["model"] = json.RawMessage(mustMarshalString(model))
	if _, exists := envelope["instructions"]; !exists {
		if instructions, ok := lastRequest["instructions"]; ok {
			envelope["instructions"] = append(json.RawMessage(nil), instructions...)
		}
	}
	envelope["stream"] = json.RawMessage("true")
	inputBytes, err := common.Marshal(mergedInput)
	if err != nil {
		return nil, state, fmt.Errorf("marshal websocket input: %w", err)
	}
	envelope["input"] = inputBytes

	normalized, err := common.Marshal(envelope)
	if err != nil {
		return nil, state, fmt.Errorf("marshal websocket request: %w", err)
	}
	if len(normalized) > responsesWebsocketMaxState {
		return nil, state, errors.New("message_too_big: websocket request exceeds the maximum size")
	}
	state.Model = model
	state.LastRequest = append([]byte(nil), normalized...)
	return normalized, state, nil
}

func validateResponsesWebsocketPreviousResponse(envelope map[string]json.RawMessage, state ResponsesWebsocketState) error {
	previous, ok := envelope["previous_response_id"]
	if !ok {
		return nil
	}
	var value string
	if err := common.Unmarshal(previous, &value); err != nil {
		return errors.New("previous_response_not_found: invalid previous_response_id")
	}
	value = strings.TrimSpace(value)
	if value != "" && value != state.LastResponseID {
		return errors.New("previous_response_not_found: previous response is not available on this websocket")
	}
	return nil
}

func responsesWebsocketInputField(envelope map[string]json.RawMessage, required bool) ([]json.RawMessage, error) {
	raw, ok := envelope["input"]
	if !ok {
		if required {
			return nil, errors.New("websocket request requires array field: input")
		}
		return []json.RawMessage{}, nil
	}
	return responsesWebsocketArrayBytes(raw)
}

func responsesWebsocketArrayField(envelope map[string]json.RawMessage, key string) ([]json.RawMessage, error) {
	raw, ok := envelope[key]
	if !ok {
		return nil, errors.New("saved websocket request is missing input")
	}
	return responsesWebsocketArrayBytes(raw)
}

func responsesWebsocketArrayBytes(raw []byte) ([]json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return []json.RawMessage{}, nil
	}
	var values []json.RawMessage
	if err := common.Unmarshal(raw, &values); err != nil || values == nil {
		return nil, errors.New("input must be an array")
	}
	return values, nil
}

func responsesWebsocketStringField(envelope map[string]json.RawMessage, key string) (string, error) {
	raw, ok := envelope[key]
	if !ok {
		return "", errors.New("field is missing")
	}
	var value string
	if err := common.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	return value, nil
}

func responsesWebsocketIsFullTranscript(input []json.RawMessage) bool {
	for _, raw := range input {
		var item map[string]json.RawMessage
		if common.Unmarshal(raw, &item) != nil {
			continue
		}
		itemType, _ := responsesWebsocketStringField(item, "type")
		switch strings.TrimSpace(itemType) {
		case "function_call", "custom_tool_call":
			return true
		case "message":
			role, _ := responsesWebsocketStringField(item, "role")
			if strings.EqualFold(strings.TrimSpace(role), "assistant") {
				return true
			}
		}
	}
	return false
}

func mustMarshalString(value string) []byte {
	encoded, err := common.Marshal(value)
	if err != nil {
		return []byte(`""`)
	}
	return encoded
}

// ResponsesWebsocketErrorPayload builds a stable JSON error event for clients.
func ResponsesWebsocketErrorPayload(code, message string) []byte {
	payload, err := common.Marshal(responsesWebsocketErrorEnvelope{
		Type: "error",
		Error: responsesWebsocketErrorDetail{
			Code:    code,
			Message: message,
		},
	})
	if err != nil {
		return []byte(`{"type":"error","error":{"code":"internal_error","message":"request failed"}}`)
	}
	return payload
}
