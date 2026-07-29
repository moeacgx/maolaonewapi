package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestParseQwen3GuardResponse(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		scanners   []string
		action     string
		decision   string
		risk       string
		wantErr    string
		unknownLen int
	}{
		{name: "安全", content: "Safety: Safe\nCategories: None", scanners: PromptAuditScannerIDs, action: "Allow", decision: "pass", risk: "low"},
		{name: "命中暴力", content: "Safety: Unsafe\nCategories: Violence", scanners: []string{"violent"}, action: "Block", decision: "critical", risk: "critical"},
		{name: "未启用类别只警告", content: "Categories: Violent\nSafety: Unsafe", scanners: []string{"pii"}, action: "Warn", decision: "flag", risk: "high"},
		{name: "争议高危类别阻断", content: "Safety: Controversial\nCategories: PII", scanners: []string{"pii"}, action: "Block", decision: "critical", risk: "critical"},
		{name: "未知类别安全阻断", content: "Safety: Unsafe\nCategories: Future Risk", scanners: PromptAuditScannerIDs, action: "Block", decision: "critical", risk: "critical", unknownLen: 1},
		{name: "多余说明非法", content: "Safety: Safe\nCategories: None\nExplanation: ok", scanners: PromptAuditScannerIDs, wantErr: PromptGuardInvalidResponseCode},
		{name: "重复字段非法", content: "Safety:\nSafety: Safe", scanners: PromptAuditScannerIDs, wantErr: PromptGuardInvalidResponseCode},
		{name: "重复分类字段非法", content: "Categories:\nCategories: None", scanners: PromptAuditScannerIDs, wantErr: PromptGuardInvalidResponseCode},
		{name: "空分类列表非法", content: "Safety: Safe\nCategories: ,", scanners: PromptAuditScannerIDs, wantErr: PromptGuardInvalidResponseCode},
		{name: "非法安全值", content: "Safety: Maybe\nCategories: None", scanners: PromptAuditScannerIDs, wantErr: PromptGuardInvalidResponseCode},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ParseQwen3GuardResponse(test.content, test.scanners)
			if test.wantErr != "" {
				var guardErr *PromptGuardError
				require.ErrorAs(t, err, &guardErr)
				require.Equal(t, test.wantErr, guardErr.Code)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.action, result.Action)
			require.Equal(t, test.decision, result.Decision)
			require.Equal(t, test.risk, result.RiskLevel)
			require.Len(t, result.UnknownCategories, test.unknownLen)
			for _, category := range result.UnknownCategories {
				require.NotContains(t, category, "future_risk")
			}
		})
	}
}

func TestExtractPromptAuditSnapshotLatestUserFirst(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5",
		"messages":[
			{"role":"system","content":"系统说明"},
			{"role":"user","content":"较早问题"},
			{"role":"assistant","content":"历史回答"},
			{"role":"user","content":[{"type":"text","text":"最新用户输入🙂"}]}
		]
	}`)
	snapshot, err := ExtractPromptAuditSnapshot(PromptAuditRequest{Body: body, Protocol: "openai_chat_completions"})
	require.NoError(t, err)
	require.Equal(t, 4, snapshot.MessageCount)
	require.True(t, strings.HasPrefix(snapshot.ScanText, "最新用户输入🙂"+promptAuditPrioritySeparator))
	require.Len(t, snapshot.ContextSegments, 4)
	require.Equal(t, "client", snapshot.ContextSegments[0].Kind)
	require.Equal(t, "system", snapshot.ContextSegments[0].Role)
	require.Equal(t, "llm", snapshot.ContextSegments[2].Kind)
	require.Equal(t, "assistant", snapshot.ContextSegments[2].Role)
	require.Equal(t, "client", snapshot.ContextSegments[3].Kind)
	require.Equal(t, "user", snapshot.ContextSegments[3].Role)
	require.True(t, strings.HasPrefix(snapshot.FullPrompt, "系统说明\n\n较早问题\n\n历史回答\n\n最新用户输入🙂"))
	require.Len(t, snapshot.PromptHash, 64)
	require.False(t, snapshot.PromptTruncated)
}

func TestExtractPromptAuditSnapshotProtocols(t *testing.T) {
	tests := []struct {
		protocol string
		body     string
		want     string
	}{
		{protocol: "anthropic_messages", body: `{"system":"规则","messages":[{"role":"user","content":[{"type":"text","text":"问题"}]}]}`, want: "问题"},
		{protocol: "gemini", body: `{"systemInstruction":{"parts":[{"text":"规则"}]},"contents":[{"role":"user","parts":[{"text":"问题"}]}]}`, want: "问题"},
		{protocol: "openai_responses", body: `{"instructions":"规则","input":[{"role":"user","content":[{"type":"input_text","text":"问题"}]}]}`, want: "问题"},
		{protocol: "openai_realtime", body: `{"type":"session.update","session":{"instructions":"实时规则"}}`, want: "实时规则"},
		{protocol: "openai_realtime", body: `{"type":"conversation.item.create","item":{"role":"user","content":[{"type":"input_text","text":"实时问题"}]}}`, want: "实时问题"},
		{protocol: "openai_chat_completions", body: `{"messages":[{"role":"assistant","tool_calls":[{"type":"function","function":{"name":"lookup","arguments":"{\"query\":\"工具参数风险文本\"}"}}]}]}`, want: "工具参数风险文本"},
		{protocol: "openai_responses", body: `{"input":[{"type":"function_call_output","call_id":"call_1","output":"工具输出风险文本"}]}`, want: "工具输出风险文本"},
		{protocol: "openai_responses", body: `{"input":[{"role":"assistant","tool_calls":[{"type":"function","function":{"arguments":"{\"query\":\"响应工具调用风险文本\"}"}}]}]}`, want: "响应工具调用风险文本"},
		{protocol: "openai_realtime", body: `{"type":"conversation.item.create","item":{"type":"function_call_output","call_id":"call_1","output":"实时工具输出风险文本"}}`, want: "实时工具输出风险文本"},
		{protocol: "openai_realtime", body: `{"type":"conversation.item.create","item":{"role":"assistant","tool_calls":[{"type":"function","function":{"arguments":"{\"query\":\"实时工具调用风险文本\"}"}}]}}`, want: "实时工具调用风险文本"},
		{protocol: "openai_realtime", body: `{"type":"session.update","session":{"tools":[{"type":"function","name":"lookup","description":"实时工具说明风险文本"}]}}`, want: "实时工具说明风险文本"},
		{protocol: "openai_realtime", body: `{"type":"session.update","session":{"audio":{"input":{"transcription":{"prompt":"实时转写提示风险文本"}}}}}`, want: "实时转写提示风险文本"},
		{protocol: "openai_realtime", body: `{"type":"conversation.item.create","item":{"role":"assistant","content":[{"type":"input_text","text":"客户端助手角色风险文本"}]}}`, want: "客户端助手角色风险文本"},
		{protocol: "openai_realtime", body: `{"type":"conversation.item.create","item":{"role":"future_role","content":[{"type":"input_text","text":"未来角色风险文本"}]}}`, want: "未来角色风险文本"},
		{protocol: "openai_realtime", body: `{"type":"transcription_session.update","session":{"input_audio_transcription":{"prompt":"转写会话风险文本"}}}`, want: "转写会话风险文本"},
		{protocol: "openai_realtime", body: `{"type":"response.create","response":{"input":"实时响应问题","prompt":{"variables":{"topic":"实时模板变量风险文本"}},"tools":[{"type":"function","description":"实时响应工具说明风险文本"}]}}`, want: "实时模板变量风险文本"},
		{protocol: "openai_realtime", body: `{"type":"response.create","response":{"tools":[{"type":"function","description":"实时响应工具说明风险文本"}]}}`, want: "实时响应工具说明风险文本"},
		{protocol: "openai_realtime", body: `{"type":"response.create","model":"gpt-realtime","input":"根级实时响应风险文本"}`, want: "根级实时响应风险文本"},
		{protocol: "openai_realtime", body: `{"type":"future.realtime.event","text":"未知 Realtime 事件风险文本"}`, want: "未知 Realtime 事件风险文本"},
		{protocol: "openai_chat_completions", body: `{"messages":[{"role":"user","content":"问题"}],"tools":[{"type":"function","function":{"name":"lookup","description":"聊天工具说明风险文本"}}]}`, want: "聊天工具说明风险文本"},
		{protocol: "openai_chat_completions", body: `{"messages":[{"role":"user","content":[{"type":"future_text_part","text":"未知内容类型风险文本"}]}]}`, want: "未知内容类型风险文本"},
		{protocol: "openai_chat_completions", body: `{"messages":[{"role":"user","content":[{"type":"future_value_part","value":"未知内容值风险文本"}]}]}`, want: "未知内容值风险文本"},
		{protocol: "openai_chat_completions", body: `{"messages":[{"role":"assistant","content":"历史回答","reasoning_content":"客户端推理上下文风险文本"}]}`, want: "客户端推理上下文风险文本"},
		{protocol: "anthropic_messages", body: `{"messages":[{"role":"user","content":"问题"}],"tools":[{"name":"lookup","description":"Claude 工具说明风险文本","input_schema":{"type":"object"}}]}`, want: "Claude 工具说明风险文本"},
		{protocol: "anthropic_messages", body: `{"messages":[{"role":"user","content":[{"type":"future_content_part","content":"Claude 未知内容类型风险文本"}]}]}`, want: "Claude 未知内容类型风险文本"},
		{protocol: "anthropic_messages", body: `{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"Claude 思考上下文风险文本","signature":"opaque"}]}]}`, want: "Claude 思考上下文风险文本"},
		{protocol: "openai_responses", body: `{"input":"问题","prompt":{"id":"pmpt_1","variables":{"topic":"响应模板变量风险文本"}}}`, want: "响应模板变量风险文本"},
		{protocol: "openai_responses", body: `{"input":"问题","prompt":"兼容客户端直接 prompt 文本"}`, want: "兼容客户端直接 prompt 文本"},
		{protocol: "openai_responses", body: `{"input":"问题","tools":[{"type":"function","name":"lookup","description":"响应工具说明风险文本"}]}`, want: "响应工具说明风险文本"},
		{protocol: "openai_responses", body: `{"type":"future_http_extension","input":"HTTP 根级 type 不能绕过风险文本"}`, want: "HTTP 根级 type 不能绕过风险文本"},
		{protocol: "gemini", body: `{"contents":[{"role":"user","parts":[{"text":"问题"}]}],"tools":[{"functionDeclarations":[{"name":"lookup","description":"Gemini 工具说明风险文本"}]}]}`, want: "Gemini 工具说明风险文本"},
		{protocol: "gemini", body: `{"contents":[{"role":"user","parts":[{"functionResponse":{"name":"lookup","response":{"text":"Gemini 工具输出风险文本"}}}]}]}`, want: "Gemini 工具输出风险文本"},
		{protocol: "gemini", body: `{"contents":[{"role":"model","parts":[{"executableCode":{"language":"PYTHON","code":"Gemini 可执行代码风险文本"}},{"codeExecutionResult":{"outcome":"OUTCOME_OK","output":"Gemini 执行输出风险文本"}}]}]}`, want: "Gemini 可执行代码风险文本"},
		{protocol: "gemini", body: `{"instances":[{"content":"Vertex embedding 风险文本"}]}`, want: "Vertex embedding 风险文本"},
		{protocol: "gemini", body: `{"instances":[{"content":{"parts":[{"text":"Vertex parts 风险文本"}]}}]}`, want: "Vertex parts 风险文本"},
		{protocol: "images", body: `{"prompt":"画一只猫","image":"data:image/png;base64,AAAAAAAA"}`, want: "画一只猫"},
		{protocol: "images", body: `{"caption":"图片说明风险文本","style":"艺术风格风险文本","image":"data:image/png;base64,AAAAAAAA"}`, want: "图片说明风险文本"},
		{protocol: "images", body: `{"prompt":"画一只猫","logo_info":{"logo_text_content":"图片水印风险文本"}}`, want: "图片水印风险文本"},
		{protocol: "audio", body: `{"model":"tts-test","ref_text":"音色参考风险文本"}`, want: "音色参考风险文本"},
		{protocol: "task", body: `{"gpt_description_prompt":"创作歌曲","tags":"歌曲风格标签风险文本"}`, want: "歌曲风格标签风险文本"},
		{protocol: "embedding", body: `{"input":["文本一","文本二"]}`, want: "文本二"},
	}
	for _, test := range tests {
		t.Run(test.protocol+test.want, func(t *testing.T) {
			snapshot, err := ExtractPromptAuditSnapshot(PromptAuditRequest{Protocol: test.protocol, Body: []byte(test.body)})
			require.NoError(t, err)
			require.Contains(t, snapshot.ScanText, test.want)
		})
	}
}

func TestExtractPromptAuditFormValueSlices(t *testing.T) {
	// urlencoded/multipart 表单在解析后会把重复字段表示为 []string，
	// 与 JSON 解码得到的 []interface{} 不同；两者都必须进入文本提取器。
	root := map[string]interface{}{
		"input":     []string{"表单输入一", "表单输入二"},
		"prompt":    []string{"表单提示词"},
		"documents": []string{"文档一", "文档二"},
	}
	texts := extractPromptAuditMediaPrompts(root)
	require.ElementsMatch(t, []string{"表单输入一", "表单输入二", "表单提示词", "文档一", "文档二"}, texts)

	structured := promptAuditStructuredTexts([]string{"结构化参数一", "结构化参数二"})
	require.Equal(t, []string{"结构化参数一", "结构化参数二"}, structured)
}

func TestExtractPromptAuditRealtimeUnknownDataSkipsMediaPayload(t *testing.T) {
	media := `{"type":"future.realtime.event","data":"data:audio/wav;base64,` + strings.Repeat("A", 512) + `"}`
	_, err := ExtractPromptAuditSnapshot(PromptAuditRequest{
		Protocol: "openai_realtime",
		Body:     []byte(media),
	})
	require.ErrorIs(t, err, ErrPromptAuditNoText)
}

func TestExtractPromptAuditSnapshotAuditsUnknownHTTPRoles(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		body     string
		want     string
	}{
		{
			name:     "Chat 未知角色",
			protocol: "openai_chat_completions",
			body:     `{"messages":[{"role":"user","content":"较早输入"},{"role":"future_role","content":"聊天未知角色风险文本"}]}`,
			want:     "聊天未知角色风险文本",
		},
		{
			name:     "Claude 未知角色",
			protocol: "anthropic_messages",
			body:     `{"messages":[{"role":"user","content":"较早输入"},{"role":"future_role","content":"Claude 未知角色风险文本"}]}`,
			want:     "Claude 未知角色风险文本",
		},
		{
			name:     "Responses 未知角色",
			protocol: "openai_responses",
			body:     `{"input":[{"role":"user","content":[{"type":"input_text","text":"较早输入"}]},{"role":"future_role","content":[{"type":"input_text","text":"Responses 未知角色风险文本"}]}]}`,
			want:     "Responses 未知角色风险文本",
		},
		{
			name:     "Gemini 未知角色",
			protocol: "gemini",
			body:     `{"contents":[{"role":"user","parts":[{"text":"较早输入"}]},{"role":"future_role","parts":[{"text":"Gemini 未知角色风险文本"}]}]}`,
			want:     "Gemini 未知角色风险文本",
		},
		{
			name:     "Chat 缺失角色",
			protocol: "openai_chat_completions",
			body:     `{"messages":[{"role":"user","content":"较早输入"},{"content":"缺失角色风险文本"}]}`,
			want:     "缺失角色风险文本",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot, err := ExtractPromptAuditSnapshot(PromptAuditRequest{
				Protocol: test.protocol,
				Body:     []byte(test.body),
			})
			require.NoError(t, err)
			require.True(t, strings.HasPrefix(snapshot.ScanText, test.want+promptAuditPrioritySeparator))
			require.Contains(t, snapshot.FullPrompt, test.want)
		})
	}
}

func TestPromptAuditSnapshotTruncationAndPreview(t *testing.T) {
	text := strings.Repeat("界", PromptAuditMaxFullPromptRunes+7)
	snapshot, err := ExtractPromptAuditSnapshot(PromptAuditRequest{
		Protocol: "images", Body: []byte(`{"prompt":"` + text + `"}`),
	})
	require.NoError(t, err)
	require.Equal(t, PromptAuditMaxFullPromptRunes+7, snapshot.PromptLength)
	require.Len(t, []rune(snapshot.FullPrompt), PromptAuditMaxFullPromptRunes)
	require.Len(t, []rune(snapshot.ScanText), PromptAuditMaxFullPromptRunes)
	require.True(t, snapshot.PromptTruncated)

	preview := BuildPromptAuditPreview("contact me@example.com Authorization: Bearer abcdefghijklmnopqrstuvwxyz password=supersecretvalue")
	require.Contains(t, preview, "me@example.com")
	require.Contains(t, preview, "abcdefghijklmnopqrstuvwxyz")
	require.Contains(t, preview, "supersecretvalu")
	require.Contains(t, preview, "Bearer")
	require.LessOrEqual(t, len([]rune(preview)), PromptAuditPreviewRunes+1)

	credentialPreview := BuildPromptAuditPreview("Authorization: eyJhbGciOiJIUzI1NiJ9.verysecretpayload.signaturevalue")
	require.Contains(t, credentialPreview, "eyJhbGci")
	require.Contains(t, credentialPreview, "verysecretpayload")
}

func TestSplitPromptAuditRunesPreservesPriorityAndUnicode(t *testing.T) {
	chunks := SplitPromptAuditRunes("优先🙂文本"+promptAuditPrioritySeparator+"普通上下文abcdef", 3)
	require.Equal(t, []string{"优先🙂", "文本", "普通上", "下文a", "bcd", "ef"}, chunks)
}

func TestPromptAuditCrypto(t *testing.T) {
	originalSecret := common.CryptoSecret
	t.Cleanup(func() { common.CryptoSecret = originalSecret })
	t.Setenv("CRYPTO_SECRET", "stable-test-secret")
	common.CryptoSecret = "stable-test-secret"

	ciphertext, err := EncryptPromptAuditSecret("top-secret-prompt")
	require.NoError(t, err)
	require.NotContains(t, ciphertext, "top-secret-prompt")
	plain, err := DecryptPromptAuditSecret(ciphertext)
	require.NoError(t, err)
	require.Equal(t, "top-secret-prompt", plain)

	replacement := "A"
	if strings.HasSuffix(ciphertext, replacement) {
		replacement = "B"
	}
	tampered := ciphertext[:len(ciphertext)-1] + replacement
	_, err = DecryptPromptAuditSecret(tampered)
	require.Error(t, err)

	require.True(t, PromptAuditCryptoReady())
	require.NoError(t, os.Unsetenv("CRYPTO_SECRET"))
	require.False(t, PromptAuditCryptoReady())
}

type promptAuditMockScanner struct {
	mu    sync.Mutex
	calls []string
	scan  func(PromptAuditEndpoint) (*PromptAuditResult, error)
}

type promptAuditScannerFunc func(context.Context, PromptAuditEndpoint, string, []string) (*PromptAuditResult, error)

func (f promptAuditScannerFunc) Scan(ctx context.Context, endpoint PromptAuditEndpoint, chunk string, scanners []string) (*PromptAuditResult, error) {
	return f(ctx, endpoint, chunk, scanners)
}

func (s *promptAuditMockScanner) Scan(_ context.Context, endpoint PromptAuditEndpoint, _ string, _ []string) (*PromptAuditResult, error) {
	s.mu.Lock()
	s.calls = append(s.calls, endpoint.Id)
	s.mu.Unlock()
	return s.scan(endpoint)
}

func TestPromptAuditEvaluatorEndpointFailover(t *testing.T) {
	scanner := &promptAuditMockScanner{}
	scanner.scan = func(endpoint PromptAuditEndpoint) (*PromptAuditResult, error) {
		if endpoint.Id == "primary" {
			return nil, &PromptGuardError{Code: PromptGuardUnavailableCode, Retryable: true, Cause: errors.New("temporary")}
		}
		return ParseQwen3GuardResponse("Safety: Safe\nCategories: None", PromptAuditScannerIDs)
	}
	evaluator := NewPromptAuditGuardEvaluator(scanner, 64, 16)
	result, err := evaluator.Evaluate(context.Background(), &PromptAuditConfig{
		Scanners: PromptAuditScannerIDs,
		Endpoints: []PromptAuditEndpoint{
			{Id: "primary", Enabled: true, InputLimit: 4000},
			{Id: "secondary", Enabled: true, InputLimit: 4000},
		},
	}, PromptAuditSnapshot{ScanText: "hello"})
	require.NoError(t, err)
	require.Equal(t, "Allow", result.Action)
	require.Equal(t, []string{"primary", "secondary"}, scanner.calls)
}

func TestPromptAuditEvaluatorUsesSharedEvaluationDeadline(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	scanner := promptAuditScannerFunc(func(ctx context.Context, _ PromptAuditEndpoint, _ string, _ []string) (*PromptAuditResult, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		select {
		case <-time.After(35 * time.Millisecond):
			return ParseQwen3GuardResponse("Safety: Safe\nCategories: None", PromptAuditScannerIDs)
		case <-ctx.Done():
			return nil, &PromptGuardError{Code: PromptGuardUnavailableCode, Retryable: true,
				Timeout: errors.Is(ctx.Err(), context.DeadlineExceeded), Cause: ctx.Err()}
		}
	})
	evaluator := NewPromptAuditGuardEvaluator(scanner, 64, 16)
	started := time.Now()
	_, err := evaluator.Evaluate(context.Background(), &PromptAuditConfig{
		Scanners: PromptAuditScannerIDs,
		Endpoints: []PromptAuditEndpoint{
			{Id: "primary", Enabled: true, TimeoutMs: 80, InputLimit: 1},
		},
	}, PromptAuditSnapshot{ScanText: "三个分片"})
	require.Error(t, err)
	require.Less(t, time.Since(started), 250*time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 3, calls)
}

func TestPromptAuditEvaluatorDoesNotFailoverInvalidResponse(t *testing.T) {
	scanner := &promptAuditMockScanner{scan: func(PromptAuditEndpoint) (*PromptAuditResult, error) {
		return nil, &PromptGuardError{Code: PromptGuardInvalidResponseCode}
	}}
	evaluator := NewPromptAuditGuardEvaluator(scanner, 64, 16)
	_, err := evaluator.Evaluate(context.Background(), &PromptAuditConfig{
		Scanners:  PromptAuditScannerIDs,
		Endpoints: []PromptAuditEndpoint{{Id: "primary", Enabled: true}, {Id: "secondary", Enabled: true}},
	}, PromptAuditSnapshot{ScanText: "hello"})
	require.Error(t, err)
	require.Equal(t, []string{"primary"}, scanner.calls)
}

func TestNormalizePromptAuditBaseURLSecurity(t *testing.T) {
	valid := []string{"http://127.0.0.1:8080/v1", "https://10.0.0.2/guard", "https://guard.example.com"}
	for _, value := range valid {
		_, err := NormalizePromptAuditBaseURL(value)
		require.NoError(t, err, value)
	}
	invalid := []string{
		"ftp://guard.example.com", "https://user:pass@guard.example.com", "https://guard.example.com?q=1",
		"http://169.254.169.254", "http://100.100.100.200", "http://metadata.google.internal",
	}
	for _, value := range invalid {
		_, err := NormalizePromptAuditBaseURL(value)
		require.Error(t, err, value)
	}
	url, err := PromptAuditChatCompletionsURL("https://guard.example.com/v1")
	require.NoError(t, err)
	require.Equal(t, "https://guard.example.com/v1/chat/completions", url)
	encodedPath, err := NormalizePromptAuditBaseURL("https://guard.example.com/guard%20node/")
	require.NoError(t, err)
	require.Equal(t, "https://guard.example.com/guard%20node", encodedPath)
}

func TestProbePromptAuditEndpointChatFallbackReportsHTTPStatus(t *testing.T) {
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"}}]}`))
	}))
	defer guard.Close()

	result := ProbePromptAuditEndpoint(context.Background(), PromptAuditEndpoint{
		Id: "fallback-probe", BaseUrl: guard.URL, Model: PromptAuditDefaultModel,
		TimeoutMs: 1000, InputLimit: PromptAuditDefaultInputLimit, Enabled: true,
	})
	require.True(t, result.Ok)
	require.True(t, result.Healthy)
	require.Equal(t, http.StatusOK, result.HTTPStatus)
	require.Empty(t, result.ErrorCode)
}
