package relay

import (
	"encoding/json"
	"io"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

func TestResponsesHelperDropsPreviousResponseIDForHTTPRelay(t *testing.T) {
	original := &dto.OpenAIResponsesRequest{
		Model:              "gpt-test",
		PreviousResponseID: "resp_previous",
	}
	request := *original
	stripHTTPResponsesContinuation(&request)

	require.Empty(t, request.PreviousResponseID)
	require.Equal(t, "resp_previous", original.PreviousResponseID)
}

func TestNewSanitizedHTTPResponsesBodyPreservesOtherFields(t *testing.T) {
	storage, err := common.CreateBodyStorage([]byte(`{"model":"gpt-test","previous_response_id":"resp_previous","custom_field":true}`))
	require.NoError(t, err)
	defer storage.Close()

	body, closer, err := newSanitizedHTTPResponsesBody(storage)
	require.NoError(t, err)
	if closer != nil {
		defer closer.Close()
	}
	cleaned, err := io.ReadAll(body)
	require.NoError(t, err)

	require.JSONEq(t, `{"model":"gpt-test","custom_field":true}`, string(cleaned))
}

func TestNormalizeHTTPResponsesInputRepairsMissingFunctionCallOutputID(t *testing.T) {
	input := json.RawMessage(`[{"type":"function_call_output","output":"tool result"}]`)

	normalized, err := normalizeHTTPResponsesInput(input)
	require.NoError(t, err)
	require.JSONEq(t, `[{"role":"user","content":"[tool_output_missing_call_id] tool result"}]`, string(normalized))
}

func TestNormalizeHTTPResponsesInputDoesNotUseOutputIDAsCallID(t *testing.T) {
	input := json.RawMessage(`[{"type":"function_call_output","id":"call_from_item","output":"tool result"}]`)

	normalized, err := normalizeHTTPResponsesInput(input)
	require.NoError(t, err)
	require.JSONEq(t, `[{"role":"user","content":"[tool_output_missing_call_id] tool result"}]`, string(normalized))
}

func TestNormalizeHTTPResponsesInputPreservesNonObjectItems(t *testing.T) {
	input := json.RawMessage(`["plain input",{"type":"function_call_output","output":"tool result"}]`)

	normalized, err := normalizeHTTPResponsesInput(input)
	require.NoError(t, err)
	require.JSONEq(t, `["plain input",{"role":"user","content":"[tool_output_missing_call_id] tool result"}]`, string(normalized))
}

func TestNormalizeHTTPResponsesInputPreservesValidCallID(t *testing.T) {
	input := json.RawMessage(`[{"type":"function_call_output","call_id":"call_valid","output":"ok"},{"type":"function_call_output","output":"missing"}]`)

	normalized, err := normalizeHTTPResponsesInput(input)
	require.NoError(t, err)
	require.JSONEq(t, `[{"type":"function_call_output","call_id":"call_valid","output":"ok"},{"role":"user","content":"[tool_output_missing_call_id] missing"}]`, string(normalized))
}

func TestSanitizeHTTPResponsesBodyCleansContinuationAndInput(t *testing.T) {
	body := []byte(`{"model":"gpt-test","previous_response_id":"resp_previous","input":[{"type":"function_call_output","output":"tool result"}],"custom_field":true}`)

	sanitized, err := sanitizeHTTPResponsesBody(body)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"gpt-test","input":[{"role":"user","content":"[tool_output_missing_call_id] tool result"}],"custom_field":true}`, string(sanitized))
}
