package relay

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPrepareClaudePassthroughBodyFingerprintContract(t *testing.T) {
	body := []byte("{\n  \"model\": \"claude-sonnet-4-6\",\n  \"messages\": [{\"role\":\"user\",\"content\":\"stable user text\"}],\n  \"system\": \"caller system\",\n  \"metadata\": {\"user_id\":\"caller\"},\n  \"provider_extension\": true\n}\n")
	tests := []struct {
		name        string
		userAgent   string
		fingerprint bool
		wantMutated bool
	}{
		{name: "real Claude Code remains byte exact when enabled", userAgent: "claude-cli/2.8.2 (Claude Code)", fingerprint: true},
		{name: "compatible caller is mutated when enabled", userAgent: "compatible-client/1.0", fingerprint: true, wantMutated: true},
		{name: "compatible caller remains byte exact when disabled", userAgent: "compatible-client/1.0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(string(body)))
			ctx.Request.Header.Set("User-Agent", test.userAgent)

			info := &relaycommon.RelayInfo{
				RelayFormat: types.RelayFormatClaude,
				ChannelMeta: &relaycommon.ChannelMeta{
					ApiType: constant.APITypeAnthropic,
					ChannelSetting: dto.ChannelSettings{
						PassThroughBodyEnabled: true,
					},
					ChannelOtherSettings: dto.ChannelOtherSettings{
						ClaudeCodeFingerprintEnabled: test.fingerprint,
					},
				},
			}
			storage, err := common.CreateBodyStorage(append([]byte(nil), body...))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, storage.Close()) })

			requestBody, closer, err := prepareClaudePassthroughBody(ctx, info, storage)
			require.NoError(t, err)
			if closer != nil {
				t.Cleanup(func() { require.NoError(t, closer.Close()) })
			}
			got, err := io.ReadAll(requestBody)
			require.NoError(t, err)

			if !test.wantMutated {
				require.Equal(t, body, got)
				return
			}
			require.NotEqual(t, body, got)
			var request dto.ClaudeRequest
			require.NoError(t, common.Unmarshal(got, &request))
			require.Len(t, request.ParseSystem(), 3)
			require.Contains(t, request.ParseSystem()[0].GetText(), "Claude Code")
			require.Contains(t, request.ParseSystem()[1].GetText(), "x-anthropic-billing-header:")
			var raw map[string]json.RawMessage
			require.NoError(t, common.Unmarshal(got, &raw))
			require.JSONEq(t, "true", string(raw["provider_extension"]))
		})
	}
}
