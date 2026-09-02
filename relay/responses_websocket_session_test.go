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
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeResponsesWebsocketRequestHandlesCreateAndAppend(t *testing.T) {
	state := ResponsesWebsocketState{
		LastRequest:        []byte(`{"model":"gpt-test","input":[{"role":"user","content":"hi"}],"stream":true}`),
		LastResponseOutput: []byte(`[{"type":"message","role":"assistant","content":"hello"}]`),
		Model:              "gpt-test",
	}

	create, next, err := NormalizeResponsesWebsocketRequest(
		[]byte(`{"type":"response.create","model":"gpt-test","input":[{"role":"user","content":"first"}]}`),
		ResponsesWebsocketState{},
	)
	require.NoError(t, err)
	assertNormalizedResponsesRequest(t, create, "gpt-test", 1)
	assert.Equal(t, "gpt-test", next.Model)

	appendRequest, _, err := NormalizeResponsesWebsocketRequest(
		[]byte(`{"type":"response.append","input":[{"role":"user","content":"next"}]}`),
		state,
	)
	require.NoError(t, err)
	assertNormalizedResponsesRequest(t, appendRequest, "gpt-test", 3)
}

func TestNormalizeResponsesWebsocketRequestReplacesFullTranscript(t *testing.T) {
	state := ResponsesWebsocketState{
		Model:              "gpt-test",
		LastRequest:        []byte(`{"model":"gpt-test","input":[{"role":"user","content":"old"}],"stream":true}`),
		LastResponseOutput: []byte(`[{"type":"message","role":"assistant","content":"old answer"}]`),
	}

	normalized, _, err := NormalizeResponsesWebsocketRequest(
		[]byte(`{"type":"response.create","input":[{"type":"message","role":"user","content":"new"},{"type":"message","role":"assistant","content":"new answer"}]}`),
		state,
	)
	require.NoError(t, err)

	var envelope map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(normalized, &envelope))
	var input []json.RawMessage
	require.NoError(t, common.Unmarshal(envelope["input"], &input))
	assert.Len(t, input, 2)
	assert.NotContains(t, string(normalized), "old answer")
}

func TestNormalizeResponsesWebsocketRequestRejectsMismatchedPreviousResponse(t *testing.T) {
	state := ResponsesWebsocketState{
		Model:          "gpt-test",
		LastResponseID: "resp-current",
		LastRequest:    []byte(`{"model":"gpt-test","input":[],"stream":true}`),
	}

	_, _, err := NormalizeResponsesWebsocketRequest(
		[]byte(`{"type":"response.create","previous_response_id":"resp-old","input":[]}`),
		state,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "previous_response_not_found")

	payload := ResponsesWebsocketErrorPayload("previous_response_not_found", "previous response is not available")
	var envelope map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(payload, &envelope))
	var eventType string
	require.NoError(t, common.Unmarshal(envelope["type"], &eventType))
	assert.Equal(t, "error", eventType)
}

func TestNormalizeResponsesWebsocketRequestRejectsInvalidEvent(t *testing.T) {
	_, _, err := NormalizeResponsesWebsocketRequest(
		[]byte(`{"type":"response.unknown","model":"gpt-test","input":[]}`),
		ResponsesWebsocketState{},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported websocket request type")
}

func TestNormalizeResponsesWebsocketRequestRejectsNonArrayAppendInput(t *testing.T) {
	state := ResponsesWebsocketState{
		Model:       "gpt-test",
		LastRequest: []byte(`{"model":"gpt-test","input":[],"stream":true}`),
	}

	_, _, err := NormalizeResponsesWebsocketRequest(
		[]byte(`{"type":"response.append","input":"next"}`),
		state,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "input must be an array")
	assert.NotContains(t, err.Error(), "next")
}

func TestNormalizeResponsesWebsocketRequestTreatsMissingPreviousOutputAsEmpty(t *testing.T) {
	state := ResponsesWebsocketState{
		Model:       "gpt-test",
		LastRequest: []byte(`{"model":"gpt-test","input":[{"role":"user","content":"first"}],"stream":true}`),
	}

	normalized, _, err := NormalizeResponsesWebsocketRequest(
		[]byte(`{"type":"response.append","input":[{"role":"user","content":"next"}]}`),
		state,
	)
	require.NoError(t, err)
	assertNormalizedResponsesRequest(t, normalized, "gpt-test", 2)
}

func assertNormalizedResponsesRequest(t *testing.T, payload []byte, model string, inputCount int) {
	t.Helper()
	var envelope map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(payload, &envelope))
	assert.NotContains(t, envelope, "type")
	var gotModel string
	require.NoError(t, common.Unmarshal(envelope["model"], &gotModel))
	assert.Equal(t, model, gotModel)
	assert.Equal(t, "true", string(envelope["stream"]))
	var input []json.RawMessage
	require.NoError(t, common.Unmarshal(envelope["input"], &input))
	assert.Len(t, input, inputCount)
}
