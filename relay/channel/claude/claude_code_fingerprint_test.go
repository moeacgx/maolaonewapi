package claude

import (
	"bytes"
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
	"github.com/QuantumNous/new-api/setting/model_setting"
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
	require.Contains(t, system[0].GetText(), "Claude Code")
	require.Contains(t, system[1].GetText(), "x-anthropic-billing-header:")
	require.Equal(t, "keep this system prompt", system[2].GetText())
	require.JSONEq(t, `{"type":"ephemeral"}`, string(system[2].CacheControl))
	require.Contains(t, string(got.Metadata), "trace")
	require.Contains(t, string(got.Metadata), "user_id")
}

func TestClaudeCodeFingerprintCapsCacheControlBreakpointsInWireOrder(t *testing.T) {
	t.Parallel()

	cacheControl := json.RawMessage(`{"type":"ephemeral"}`)
	req := &dto.ClaudeRequest{
		Model: "claude-sonnet-4-20250514",
		Tools: []any{
			map[string]any{"name": "tool-one", "cache_control": cacheControl},
			map[string]any{"name": "tool-two", "cache_control": cacheControl},
		},
		System: []dto.ClaudeMediaMessage{{
			Type:         dto.ContentTypeText,
			Text:         stringPtr("system cached"),
			CacheControl: cacheControl,
		}},
		Messages: []dto.ClaudeMessage{{
			Role: "user",
			Content: []dto.ClaudeMediaMessage{{
				Type:         dto.ContentTypeText,
				Text:         stringPtr("message cached"),
				CacheControl: cacheControl,
			}},
		}},
	}

	converted, err := (&Adaptor{}).ConvertClaudeRequest(
		claudeFingerprintContext("compatible-client/1.0"),
		claudeFingerprintInfo(true),
		req,
	)
	require.NoError(t, err)
	got := converted.(*dto.ClaudeRequest)

	tools := got.GetTools()
	require.Len(t, tools, 2)
	require.NotNil(t, tools[0].(map[string]any)["cache_control"])
	require.NotNil(t, tools[1].(map[string]any)["cache_control"])

	system := got.ParseSystem()
	require.Len(t, system, 3)
	require.Equal(t, claudeCodeSystemText, system[0].GetText())
	require.NotNil(t, system[0].CacheControl)
	require.Equal(t, "system cached", system[2].GetText())
	require.NotNil(t, system[2].CacheControl)

	messageContent, err := got.Messages[0].ParseContent()
	require.NoError(t, err)
	require.Len(t, messageContent, 1)
	require.Empty(t, messageContent[0].CacheControl)

	// Two tools, the stable marker, and the source system breakpoint are the
	// four retained breakpoints in Anthropic's tools/system/messages order.
	require.Equal(t, []string{"tool-one", "tool-two", claudeCodeSystemText, "system cached"}, []string{
		tools[0].(map[string]any)["name"].(string),
		tools[1].(map[string]any)["name"].(string),
		system[0].GetText(),
		system[2].GetText(),
	})
}

func TestClaudeCodeFingerprintKeepsFirstCachePrefixStableAcrossUserContent(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		content             string
		selectedBytes       string
		expectedFingerprint string
	}{
		{content: "0000A00B000000000000C", selectedBytes: "ABC", expectedFingerprint: "f29"},
		{content: "0000X00Y000000000000Z", selectedBytes: "XYZ", expectedFingerprint: "d77"},
	}

	prefixes := make([][]byte, 0, len(testCases))
	billings := make([]string, 0, len(testCases))
	for _, testCase := range testCases {
		require.Equal(t, testCase.selectedBytes, string([]byte{
			testCase.content[4],
			testCase.content[7],
			testCase.content[20],
		}))

		request := &dto.ClaudeRequest{
			Model:    "claude-sonnet-4-20250514",
			System:   "caller system",
			Messages: []dto.ClaudeMessage{{Role: "user", Content: testCase.content}},
		}
		converted, err := (&Adaptor{}).ConvertClaudeRequest(
			claudeFingerprintContext("compatible-client/1.0"),
			claudeFingerprintInfo(true),
			request,
		)
		require.NoError(t, err)
		got := converted.(*dto.ClaudeRequest)

		system := got.ParseSystem()
		markerCount := 0
		billingCount := 0
		for _, block := range system {
			switch {
			case block.GetText() == claudeCodeSystemText:
				markerCount++
			case strings.HasPrefix(block.GetText(), claudeCodeBillingPrefix):
				billingCount++
			}
		}
		require.Equal(t, 1, markerCount)
		require.Equal(t, 1, billingCount)
		require.Equal(t, claudeCodeSystemText, system[0].GetText())
		require.JSONEq(t, `{"type":"ephemeral"}`, string(system[0].CacheControl))
		expectedBilling := "x-anthropic-billing-header: cc_version=2.8.2." + testCase.expectedFingerprint + "; cc_entrypoint=cli;"
		require.Equal(t, expectedBilling, system[1].GetText())
		require.Empty(t, system[1].CacheControl)
		billings = append(billings, system[1].GetText())

		serialized, err := common.Marshal(got)
		require.NoError(t, err)
		breakpoint := []byte(`"cache_control":{"type":"ephemeral"}`)
		breakpointIndex := bytes.Index(serialized, breakpoint)
		require.NotEqual(t, -1, breakpointIndex)
		prefix := serialized[:breakpointIndex+len(breakpoint)]
		require.NotContains(t, string(prefix), claudeCodeBillingPrefix)
		require.NotContains(t, string(prefix), testCase.content)
		prefixes = append(prefixes, append([]byte(nil), prefix...))

		reapplied, err := (&Adaptor{}).ConvertClaudeRequest(
			claudeFingerprintContext("compatible-client/1.0"),
			claudeFingerprintInfo(true),
			got,
		)
		require.NoError(t, err)
		reappliedSerialized, err := common.Marshal(reapplied)
		require.NoError(t, err)
		require.Equal(t, serialized, reappliedSerialized)
	}

	require.Equal(t, prefixes[0], prefixes[1])
	require.NotEqual(t, billings[0], billings[1])
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
	require.Contains(t, got.ParseSystem()[0].GetText(), "Claude Code")
	require.Contains(t, got.ParseSystem()[1].GetText(), "cc_version=2.8.2.")
	require.Contains(t, got.ParseSystem()[1].GetText(), "cc_entrypoint=cli;")
}
func TestClaudeCodeFinalFingerprintLeavesRealClientBodyUntouched(t *testing.T) {
	ctx := claudeFingerprintContext("claude-cli/2.8.2 (Claude Code)")
	ctx.Request.Header.Set("X-App", "claude-code")
	info := claudeFingerprintInfo(true)
	body := []byte(`{"model":"claude-sonnet-4-20250514","system":"caller system","metadata":{"user_id":"caller"},"provider_extension":true}`)

	got, err := ApplyClaudeCodeFinalBodyFingerprint(ctx, info, body)
	require.NoError(t, err)
	require.Equal(t, body, got)
}

func TestClaudeCodeOriginalPassthroughRequiresAdminOptIn(t *testing.T) {
	ctx := claudeFingerprintContext("claude-cli/2.8.2 (Claude Code)")
	info := claudeFingerprintInfo(true)

	require.False(t, shouldUseClaudeCodeOriginalPassThrough(ctx, info), "Claude Code headers alone must not authorize passthrough")
	body := []byte(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"stable user text"}]}`)
	compatibleCtx := claudeFingerprintContext("compatible-client/1.0")
	got, err := ApplyClaudeCodePassthroughBodyFingerprint(compatibleCtx, info, body)
	require.NoError(t, err)
	require.Equal(t, body, got, "fingerprinting must not manufacture passthrough authority")
	info.ChannelSetting.PassThroughBodyEnabled = true
	require.True(t, shouldUseClaudeCodeOriginalPassThrough(ctx, info))
}

func TestClaudeCodePassthroughPreservesAdminHeadersAndIncomingCompatibilityPrecedence(t *testing.T) {
	ctx := claudeFingerprintContext("claude-cli/2.9.0 (Claude Code)")
	ctx.Request.Header.Set("X-App", "caller-cli")
	ctx.Request.Header.Set("Anthropic-Version", "caller-version")
	ctx.Request.Header.Set("Anthropic-Beta", "caller-beta")
	ctx.Request.Header.Set("X-Client-Request-Id", "caller-request")
	ctx.Request.Header.Set("Authorization", "Bearer caller-secret")
	ctx.Request.Header.Set("X-Api-Key", "caller-key")
	ctx.Request.Header.Set("Host", "caller.example")
	ctx.Request.Header.Set("Cookie", "session=caller")
	ctx.Request.Header.Set("X-Internal-Routing", "caller-route")

	info := claudeFingerprintInfo(false)
	info.OriginModelName = "claude-test-model"
	info.ChannelSetting.PassThroughBodyEnabled = true

	settings := model_setting.GetClaudeSettings()
	originalHeaders := settings.HeadersSettings
	settings.HeadersSettings = map[string]map[string][]string{
		info.OriginModelName: {
			"User-Agent":        {"admin-agent"},
			"X-App":             {"admin-app"},
			"Anthropic-Version": {"admin-version"},
			"Anthropic-Beta":    {"admin-beta"},
			"X-Admin-Trace":     {"admin-trace"},
		},
	}
	t.Cleanup(func() { settings.HeadersSettings = originalHeaders })

	headers := http.Header{}
	require.NoError(t, (&Adaptor{}).SetupRequestHeader(ctx, &headers, info))

	require.Equal(t, "claude-cli/2.9.0 (Claude Code)", headers.Get("User-Agent"))
	require.Equal(t, "caller-cli", headers.Get("X-App"))
	require.Equal(t, "caller-version", headers.Get("Anthropic-Version"))
	require.Equal(t, "caller-beta", headers.Get("Anthropic-Beta"))
	require.Equal(t, "caller-request", headers.Get("X-Client-Request-Id"))
	require.Equal(t, "admin-trace", headers.Get("X-Admin-Trace"))
	require.Empty(t, headers.Get("Authorization"))
	require.Equal(t, "channel-secret", headers.Get("X-Api-Key"))
	require.Empty(t, headers.Get("Host"))
	require.Empty(t, headers.Get("Cookie"))
	require.Empty(t, headers.Get("X-Internal-Routing"))
}

func stringPtr(value string) *string { return &value }
