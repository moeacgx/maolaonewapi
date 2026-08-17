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
