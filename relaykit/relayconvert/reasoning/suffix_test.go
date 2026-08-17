package reasoning

import "testing"

func TestParseGeminiReasoningEffortFromModelSuffix(t *testing.T) {
	tests := []struct {
		name       string
		model      string
		wantModel  string
		wantEffort string
		wantOK     bool
	}{
		{name: "Gemini 2.5 effort", model: "gemini-2.5-pro-high", wantModel: "gemini-2.5-pro", wantEffort: "high", wantOK: true},
		{name: "Gemini 3 effort", model: "gemini-3-flash-preview-ultra", wantModel: "gemini-3-flash-preview", wantEffort: "ultra", wantOK: true},
		{name: "Gemini latest alias", model: "gemini-pro-latest-max", wantModel: "gemini-pro-latest", wantEffort: "max", wantOK: true},
		{name: "Qwen max collision", model: "qwen3-max", wantModel: "qwen3-max"},
		{name: "Qwen thinking budget collision", model: "qwen3-thinking-1024", wantModel: "qwen3-thinking-1024"},
		{name: "Arbitrary thinking collision", model: "custom-thinking-high", wantModel: "custom-thinking-high"},
		{name: "Non reasoning ultra collision", model: "custom-vision-ultra", wantModel: "custom-vision-ultra"},
		{name: "Gemini image collision", model: "gemini-3-pro-image-ultra", wantModel: "gemini-3-pro-image-ultra"},
		{name: "Gemini embedding collision", model: "gemini-embedding-001-max", wantModel: "gemini-embedding-001-max"},
		{name: "Gemini family boundary collision", model: "gemini-2.5-flashlike-max", wantModel: "gemini-2.5-flashlike-max"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model, effort, ok := ParseGeminiReasoningEffortFromModelSuffix(test.model)
			if model != test.wantModel {
				t.Fatalf("model = %q, want %q", model, test.wantModel)
			}
			if effort != test.wantEffort {
				t.Fatalf("effort = %q, want %q", effort, test.wantEffort)
			}
			if ok != test.wantOK {
				t.Fatalf("ok = %t, want %t", ok, test.wantOK)
			}
		})
	}
}

func TestClaudeOpusReasoningFamilyBoundaries(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{model: "claude-opus-4-6", want: true},
		{model: "claude-opus-4-6-20260201", want: true},
		{model: "claude-opus-4-60", want: false},
		{model: "claude-opus-4-60-max", want: false},
		{model: "qwen3-thinking-1024", want: false},
	}
	for _, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			if got := IsClaudeOpusReasoningModel(test.model); got != test.want {
				t.Fatalf("IsClaudeOpusReasoningModel(%q) = %t, want %t", test.model, got, test.want)
			}
		})
	}
}

func TestParseOpenAIReasoningEffortFromModelSuffix(t *testing.T) {
	tests := []struct {
		model      string
		wantEffort string
		wantModel  string
	}{
		{model: "o3-high", wantEffort: "high", wantModel: "o3"},
		{model: "o3-medium", wantEffort: "medium", wantModel: "o3"},
		{model: "o3-low", wantEffort: "low", wantModel: "o3"},
		{model: "o3-minimal", wantEffort: "minimal", wantModel: "o3"},
		{model: "o3-none", wantEffort: "none", wantModel: "o3"},
		{model: "gpt-5.1-xhigh", wantEffort: "xhigh", wantModel: "gpt-5.1"},
		{model: "gpt-5.1-codex-max", wantModel: "gpt-5.1-codex-max"},
		{model: "qwen3-max", wantModel: "qwen3-max"},
		{model: "custom-vision-ultra", wantModel: "custom-vision-ultra"},
	}

	for _, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			effort, model := ParseOpenAIReasoningEffortFromModelSuffix(test.model)
			if effort != test.wantEffort {
				t.Fatalf("effort = %q, want %q", effort, test.wantEffort)
			}
			if model != test.wantModel {
				t.Fatalf("model = %q, want %q", model, test.wantModel)
			}
		})
	}
}

func TestIsClaudeThinkingModel(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{model: "claude-sonnet-4-6", want: true},
		{model: "claude-sonnet-4-6-20260101", want: true},
		{model: "claude-sonnet-4-5", want: true},
		{model: "claude-sonnet-4-5-20250929", want: true},
		{model: "claude-3-7-sonnet", want: true},
		{model: "claude-3-7-sonnet-20250219", want: true},
		{model: "claude-opus-4-6", want: true},
		{model: "claude-opus-4-7-20270101", want: true},
		{model: "claude-opus-4-8", want: true},
		{model: "claude-opus-4-60", want: false},
		{model: "claude-opus-4-70", want: false},
		{model: "claude-sonnet-4-6-custom", want: false},
		{model: "claude-sonnet-4-6-20260101-extra", want: false},
		{model: "claude-sonnet-4-6-2026010x", want: false},
		{model: "vendor/claude-sonnet-4-6", want: false},
		{model: "custom-claude-opus-4-7", want: false},
		{model: "claude-haiku-4-5", want: false},
	}

	for _, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			if got := IsClaudeThinkingModel(test.model); got != test.want {
				t.Fatalf("IsClaudeThinkingModel(%q) = %t, want %t", test.model, got, test.want)
			}
		})
	}
}
