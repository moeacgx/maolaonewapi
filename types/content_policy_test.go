package types

import (
	"errors"
	"net/http"
	"testing"

	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
)

func TestIsContentPolicyRejectionUsesStableErrorCodes(t *testing.T) {
	tests := []struct {
		name string
		err  *relaytypes.NewAPIError
		want bool
	}{
		{
			name: "local sensitive words",
			err:  relaytypes.NewError(errors.New("blocked"), relaytypes.ErrorCodeSensitiveWordsDetected),
			want: true,
		},
		{
			name: "local prompt guard",
			err:  relaytypes.NewError(errors.New("blocked"), ErrorCodePromptGuardBlocked),
			want: true,
		},
		{
			name: "upstream prompt blocked",
			err:  relaytypes.NewError(errors.New("blocked"), relaytypes.ErrorCodePromptBlocked),
			want: true,
		},
		{
			name: "upstream cyber policy outer code",
			err:  relaytypes.NewOpenAIError(errors.New("blocked"), ErrorCodeCyberPolicy, http.StatusForbidden),
			want: true,
		},
		{
			name: "upstream cyber policy structured relay code",
			err: relaytypes.WithOpenAIError(relaytypes.OpenAIError{
				Message: "blocked", Type: "invalid_request_error", Code: "CYBER_POLICY",
			}, http.StatusForbidden),
			want: true,
		},
		{
			name: "upstream biological risk status 500 message",
			err:  relaytypes.NewOpenAIError(errors.New("This content was flagged for possible biological risk."), relaytypes.ErrorCodeBadResponseStatusCode, http.StatusInternalServerError),
			want: true,
		},
		{
			name: "ordinary bad request remains quality sample",
			err:  relaytypes.NewOpenAIError(errors.New("invalid parameter"), relaytypes.ErrorCodeInvalidRequest, http.StatusBadRequest),
			want: false,
		},
		{
			name: "transport failure remains quality sample",
			err:  relaytypes.NewError(errors.New("connection reset"), relaytypes.ErrorCodeDoRequestFailed),
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, IsContentPolicyRejection(test.err))
		})
	}
}
