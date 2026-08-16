package claude

import (
	"encoding/json"
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

func claudeFingerprintContext(userAgent string) *gin.Context {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{}`))
	ctx.Request.Header.Set("User-Agent", userAgent)
	return ctx
}

func claudeFingerprintInfo(enabled bool) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType: constant.APITypeAnthropic,
			ApiKey:  "channel-secret",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ClaudeCodeFingerprintEnabled:          enabled,
				ClaudeCodeTransportFingerprintEnabled: enabled,
			},
		},
	}
}

func TestClaudeCodeFingerprintAddsAttributionAndPreservesSystemCache(t *testing.T) {
	t.Parallel()

	req := &dto.ClaudeRequest{
		Model: "claude-sonnet-4-20250514",
		System: []dto.ClaudeMediaMessage{{
			Type:         dto.ContentTypeText,
			Text:         stringPtr("keep this system prompt"),
			CacheControl: json.RawMessage(`{"type":"ephemeral"}`),
		}},
		Messages: []dto.ClaudeMessage{{Role: "user", Content: "stable user text"}},
		Metadata: json.RawMessage(`{"trace":"keep"}`),
	}

	converted, err := (&Adaptor{}).ConvertClaudeRequest(claudeFingerprintContext("compatible-client/1.0"), claudeFingerprintInfo(true), req)
	require.NoError(t, err)
	got := converted.(*dto.ClaudeRequest)
	system := got.ParseSystem()
	require.Len(t, system, 3)
	require.Contains(t, system[0].GetText(), "x-anthropic-billing-header:")
	require.Contains(t, system[1].GetText(), "Claude Code")
	require.Equal(t, "keep this system prompt", system[2].GetText())
	require.JSONEq(t, `{"type":"ephemeral"}`, string(system[2].CacheControl))
	require.Contains(t, string(got.Metadata), "trace")
	require.Contains(t, string(got.Metadata), "user_id")
}

func TestClaudeCodeFingerprintLeavesOrdinaryClientUnchangedWhenDisabled(t *testing.T) {
	t.Parallel()

	req := &dto.ClaudeRequest{Model: "claude-sonnet-4-20250514", System: "user system", Metadata: json.RawMessage(`{"user_id":"caller"}`)}
	before, err := common.Marshal(req)
	require.NoError(t, err)
	converted, err := (&Adaptor{}).ConvertClaudeRequest(claudeFingerprintContext("OpenAI-compatible/1.0"), claudeFingerprintInfo(false), req)
	require.NoError(t, err)
	after, err := common.Marshal(converted)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
}

func TestClaudeCodeFingerprintAddsSyntheticHeadersOnlyWhenEnabled(t *testing.T) {
	t.Parallel()

	ctx := claudeFingerprintContext("OpenAI-compatible/1.0")
	headers := http.Header{}
	err := (&Adaptor{}).SetupRequestHeader(ctx, &headers, claudeFingerprintInfo(true))
	require.NoError(t, err)
	require.Equal(t, "cli", headers.Get("X-App"))
	require.Contains(t, headers.Get("User-Agent"), "claude-cli/2.8.2")
	require.Contains(t, headers.Get("Anthropic-Beta"), "claude-code-20250219")
	require.Empty(t, headers.Get("Authorization"))
	require.Empty(t, headers.Get("X-Internal-Routing"))
}

func TestClaudeCodeFingerprintRealClientPassesOnlyCompatibilityHeaders(t *testing.T) {
	t.Parallel()

	ctx := claudeFingerprintContext("claude-cli/2.8.2 (Claude Code)")
	ctx.Request.Header.Set("X-App", "claude-code")
	ctx.Request.Header.Set("X-Stainless-Lang", "js")
	ctx.Request.Header.Set("X-Stainless-Package-Version", "0.94.0")
	ctx.Request.Header.Set("Anthropic-Beta", "claude-code-20250219")
	ctx.Request.Header.Set("X-Client-Request-Id", "request-id")
	ctx.Request.Header.Set("X-Internal-Routing", "do-not-forward")
	ctx.Request.Header.Set("Authorization", "Bearer user-secret")
	ctx.Request.Header.Set("X-Api-Key", "user-secret")
	ctx.Request.Header.Set("Host", "private.example")

	headers := http.Header{}
	info := claudeFingerprintInfo(false)
	info.ChannelSetting.PassThroughBodyEnabled = true
	err := (&Adaptor{}).SetupRequestHeader(ctx, &headers, info)
	require.NoError(t, err)
	require.Equal(t, "claude-cli/2.8.2 (Claude Code)", headers.Get("User-Agent"))
	require.Equal(t, "claude-code", headers.Get("X-App"))
	require.Equal(t, "claude-code-20250219", headers.Get("Anthropic-Beta"))
	require.Equal(t, "request-id", headers.Get("X-Client-Request-Id"))
	require.Empty(t, headers.Get("Authorization"))
	require.Equal(t, "channel-secret", headers.Get("X-Api-Key"))
	require.Empty(t, headers.Get("X-Internal-Routing"))
	require.NotEqual(t, "private.example", headers.Get("Host"))
}

func TestClaudeCodeRealClientLeavesBodyAndMetadataUntouched(t *testing.T) {
	t.Parallel()

	ctx := claudeFingerprintContext("claude-cli/2.8.2 (Claude Code)")
	ctx.Request.Header.Set("X-App", "claude-code")
	info := claudeFingerprintInfo(true)
	req := &dto.ClaudeRequest{
		Model:    "claude-sonnet-4-20250514",
		System:   "caller system",
		Metadata: json.RawMessage(`{"user_id":"caller","trace":"keep"}`),
	}
	before, err := common.Marshal(req)
	require.NoError(t, err)
	converted, err := (&Adaptor{}).ConvertClaudeRequest(ctx, info, req)
	require.NoError(t, err)
	after, err := common.Marshal(converted)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
}

func TestClaudeCodeFingerprintFinalBodyRecomputesAttributionAfterMutation(t *testing.T) {
	t.Parallel()

	info := claudeFingerprintInfo(true)
	body, err := common.Marshal(&dto.ClaudeRequest{
		Model:    "claude-sonnet-4-20250514",
		Messages: []dto.ClaudeMessage{{Role: "user", Content: "mutated user text"}},
	})
	require.NoError(t, err)
	finalBody, err := ApplyClaudeCodeFinalBodyFingerprint(claudeFingerprintContext("compatible-client/1.0"), info, body)
	require.NoError(t, err)
	var got dto.ClaudeRequest
	require.NoError(t, common.Unmarshal(finalBody, &got))
	require.Contains(t, got.ParseSystem()[0].GetText(), "cc_version=2.8.2.")
	require.Contains(t, got.ParseSystem()[0].GetText(), "cc_entrypoint=cli;")
	require.Contains(t, got.ParseSystem()[1].GetText(), "Claude Code")
}
func TestClaudeCodeFinalFingerprintLeavesRealClientBodyUntouched(t *testing.T) {
	ctx := claudeFingerprintContext("claude-cli/2.8.2 (Claude Code)")
	ctx.Request.Header.Set("X-App", "claude-code")
	info := claudeFingerprintInfo(true)
	body := []byte(`{"model":"claude-sonnet-4-20250514","system":"caller system","metadata":{"user_id":"caller"},"provider_extension":true}`)

	got, err := ApplyClaudeCodeFinalBodyFingerprint(ctx, info, body)
	require.NoError(t, err)
	require.JSONEq(t, string(body), string(got))
}

func stringPtr(value string) *string { return &value }
