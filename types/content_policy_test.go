package types

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsContentPolicyRejectionUsesStableErrorCodes(t *testing.T) {
	tests := []struct {
		name string
		err  *NewAPIError
		want bool
	}{
		{
			name: "local sensitive words",
			err:  NewError(errors.New("blocked"), ErrorCodeSensitiveWordsDetected),
			want: true,
		},
		{
			name: "local prompt guard",
			err:  NewError(errors.New("blocked"), ErrorCodePromptGuardBlocked),
			want: true,
		},
		{
			name: "upstream prompt blocked",
			err:  NewError(errors.New("blocked"), ErrorCodePromptBlocked),
			want: true,
		},
		{
			name: "upstream cyber policy outer code",
			err:  NewOpenAIError(errors.New("blocked"), ErrorCodeCyberPolicy, http.StatusForbidden),
			want: true,
		},
		{
			name: "upstream cyber policy structured relay code",
			err: WithOpenAIError(OpenAIError{
				Message: "blocked", Type: "invalid_request_error", Code: "CYBER_POLICY",
			}, http.StatusForbidden),
			want: true,
		},
		{
			name: "ordinary bad request remains quality sample",
			err:  NewOpenAIError(errors.New("invalid parameter"), ErrorCodeInvalidRequest, http.StatusBadRequest),
			want: false,
		},
		{
			name: "transport failure remains quality sample",
			err:  NewError(errors.New("connection reset"), ErrorCodeDoRequestFailed),
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, IsContentPolicyRejection(test.err))
		})
	}
}
