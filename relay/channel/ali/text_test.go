package ali

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

func TestRequestOpenAI2AliTopP(t *testing.T) {
	tests := []struct {
		name string
		topP *float64
		want *float64
	}{
		{
			name: "omitted top_p is not injected",
			topP: nil,
			want: nil,
		},
		{
			name: "in-range top_p is preserved",
			topP: lo.ToPtr(0.8),
			want: lo.ToPtr(0.8),
		},
		{
			name: "top_p of 1 is clamped to two decimals",
			topP: lo.ToPtr(1.0),
			want: lo.ToPtr(0.99),
		},
		{
			name: "top_p above 1 is clamped to two decimals",
			topP: lo.ToPtr(1.5),
			want: lo.ToPtr(0.99),
		},
		{
			name: "top_p of 0 is clamped to two decimals",
			topP: lo.ToPtr(0.0),
			want: lo.ToPtr(0.01),
		},
		{
			name: "negative top_p is clamped to two decimals",
			topP: lo.ToPtr(-0.3),
			want: lo.ToPtr(0.01),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := requestOpenAI2Ali(dto.GeneralOpenAIRequest{
				Model: "qwen-plus",
				TopP:  tt.topP,
			}, "qwen-plus")

			require.Equal(t, tt.want, got.TopP)
		})
	}
}

func TestRequestOpenAI2AliThinkingBudget(t *testing.T) {
	tests := []struct {
		name          string
		requestModel  string
		upstreamModel string
		wantBudget    bool
	}{
		{
			name:          "qwen upstream preserves explicit zero budget",
			requestModel:  "qwen-plus",
			upstreamModel: "qwen-plus",
			wantBudget:    true,
		},
		{
			name:          "qwq upstream preserves explicit zero budget",
			requestModel:  "customer-model",
			upstreamModel: "Qwen/QwQ-32B",
			wantBudget:    true,
		},
		{
			name:          "non-qwen upstream drops budget",
			requestModel:  "qwen-plus",
			upstreamModel: "deepseek-r1",
			wantBudget:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := requestOpenAI2Ali(dto.GeneralOpenAIRequest{
				Model:          tt.requestModel,
				ThinkingBudget: json.RawMessage(`0`),
			}, tt.upstreamModel)

			if tt.wantBudget {
				require.Equal(t, json.RawMessage(`0`), got.ThinkingBudget)
			} else {
				require.Nil(t, got.ThinkingBudget)
			}
		})
	}
}
