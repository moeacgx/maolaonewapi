package relay

import (
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

func TestNewHTTPResponsesBodyWithoutContinuationPreservesOtherFields(t *testing.T) {
	storage, err := common.CreateBodyStorage([]byte(`{"model":"gpt-test","previous_response_id":"resp_previous","custom_field":true}`))
	require.NoError(t, err)
	defer storage.Close()

	body, closer, err := newHTTPResponsesBodyWithoutContinuation(storage)
	require.NoError(t, err)
	if closer != nil {
		defer closer.Close()
	}
	cleaned, err := io.ReadAll(body)
	require.NoError(t, err)

	require.JSONEq(t, `{"model":"gpt-test","custom_field":true}`, string(cleaned))
}
