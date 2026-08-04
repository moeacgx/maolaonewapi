package ali

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestRequestOpenAI2AliFiltersThinkingBudgetByUpstreamModel(t *testing.T) {
	tests := []struct {
		name          string
		requestModel  string
		upstreamModel string
		wantBudget    bool
	}{
		{name: "qwen", requestModel: "qwen-plus", upstreamModel: "qwen-plus", wantBudget: true},
		{name: "mapped qwen explicit zero", requestModel: "custom-model", upstreamModel: "Qwen/Qwen3-235B", wantBudget: true},
		{name: "non qwen upstream", requestModel: "qwen-plus", upstreamModel: "deepseek-r1", wantBudget: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converted := requestOpenAI2Ali(dto.GeneralOpenAIRequest{
				Model:          tt.requestModel,
				ThinkingBudget: json.RawMessage(`0`),
			}, tt.upstreamModel)
			require.Equal(t, tt.wantBudget, converted.ThinkingBudget != nil)
			if tt.wantBudget {
				require.JSONEq(t, `0`, string(converted.ThinkingBudget))
			}
		})
	}
}
