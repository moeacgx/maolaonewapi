package dto

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dynamicRoutingTestRule() DynamicRoutingRule {
	return DynamicRoutingRule{
		ID:           "route-high",
		Enabled:      true,
		SourceModel:  "gemini-3.7-flash",
		TargetModel:  "gemini-3.7-flash-high",
		ChannelTypes: []int{1},
		RequestPaths: []string{"/v1/chat/completions"},
		Conditions: []DynamicRoutingCondition{
			{Field: DynamicRoutingConditionReasoningEffort, Value: "high"},
			{Field: "request.response_format.type", Value: "json_object"},
		},
	}
}

func TestValidateDynamicRoutingRules(t *testing.T) {
	require.NoError(t, ValidateDynamicRoutingRules([]DynamicRoutingRule{dynamicRoutingTestRule()}))

	tests := []struct {
		name  string
		rules []DynamicRoutingRule
		want  string
	}{
		{
			name: "enabled rule requires id",
			rules: []DynamicRoutingRule{{
				Enabled: true, SourceModel: "public", TargetModel: "upstream",
			}},
			want: "id is required",
		},
		{
			name: "enabled rule ids are unique",
			rules: []DynamicRoutingRule{
				{ID: "duplicate", Enabled: true, SourceModel: "public-a", TargetModel: "upstream-a"},
				{ID: "duplicate", Enabled: true, SourceModel: "public-b", TargetModel: "upstream-b"},
			},
			want: "duplicates another enabled rule",
		},
		{
			name: "unsupported action rejected",
			rules: []DynamicRoutingRule{{
				ID: "cross-endpoint", Enabled: true, Action: "image_generate", SourceModel: "public", TargetModel: "upstream",
			}},
			want: "action is not supported",
		},
		{
			name: "image bridge only accepts responses path",
			rules: []DynamicRoutingRule{{
				ID: "image-bridge", Enabled: true,
				Action:      DynamicRoutingActionResponsesImageToolBridge,
				SourceModel: "gpt-5.6-sol", TargetModel: "gpt-image-2",
				RequestPaths: []string{"/v1/chat/completions"},
			}},
			want: "must be exactly /v1/responses",
		},
		{
			name: "image bridge requires responses path",
			rules: []DynamicRoutingRule{{
				ID: "image-bridge-empty", Enabled: true,
				Action:      DynamicRoutingActionResponsesImageToolBridge,
				SourceModel: "gpt-5.6-sol", TargetModel: "gpt-image-2",
			}},
			want: "must be exactly /v1/responses",
		},
		{
			name: "image bridge rejects unsupported target path",
			rules: []DynamicRoutingRule{{
				ID: "image-bridge-chat-target", Enabled: true,
				Action:      DynamicRoutingActionResponsesImageToolBridge,
				SourceModel: "gpt-5.6-sol", TargetModel: "gpt-image-2",
				TargetPath: "/v1/chat/completions", RequestPaths: []string{"/v1/responses"},
			}},
			want: "target_path is unsupported",
		},
		{
			name: "channel type must be positive and unique",
			rules: []DynamicRoutingRule{{
				ID: "invalid-channel-type", Enabled: true, SourceModel: "public", TargetModel: "upstream", ChannelTypes: []int{1, 1},
			}},
			want: "channel_types contains duplicates",
		},
		{
			name: "request path must be path only",
			rules: []DynamicRoutingRule{{
				ID: "invalid-path", Enabled: true, SourceModel: "public", TargetModel: "upstream", RequestPaths: []string{"v1/chat/completions"},
			}},
			want: "request_paths contains an invalid path",
		},
		{
			name: "source groups reject auto",
			rules: []DynamicRoutingRule{{
				ID: "invalid-source-auto", Enabled: true, SourceModel: "public", TargetModel: "upstream", SourceGroups: []string{"auto"},
			}},
			want: "source_groups contains an invalid group code",
		},
		{
			name: "source groups reject comma list",
			rules: []DynamicRoutingRule{{
				ID: "invalid-source-list", Enabled: true, SourceModel: "public", TargetModel: "upstream", SourceGroups: []string{"default,image"},
			}},
			want: "source_groups contains an invalid group code",
		},
		{
			name: "source groups reject duplicates",
			rules: []DynamicRoutingRule{{
				ID: "duplicate-source-group", Enabled: true, SourceModel: "public", TargetModel: "upstream", SourceGroups: []string{"default", "default"},
			}},
			want: "source_groups contains duplicates",
		},
		{
			name: "target group rejects auto",
			rules: []DynamicRoutingRule{{
				ID: "invalid-target-auto", Enabled: true, SourceModel: "public", TargetModel: "upstream", TargetGroup: "auto",
			}},
			want: "target_group must be one group code",
		},
		{
			name: "request condition requires simple json path",
			rules: []DynamicRoutingRule{{
				ID: "invalid-condition", Enabled: true, SourceModel: "public", TargetModel: "upstream",
				Conditions: []DynamicRoutingCondition{{Field: "request.response-format.type", Value: "json_object"}},
			}},
			want: "must use a simple request JSON path",
		},
		{
			name: "unsupported condition operator rejected",
			rules: []DynamicRoutingRule{{
				ID: "invalid-operator", Enabled: true, SourceModel: "public", TargetModel: "upstream",
				Conditions: []DynamicRoutingCondition{{Field: DynamicRoutingConditionReasoningEffort, Operator: "contains", Value: "high"}},
			}},
			want: "operator is unsupported",
		},
		{
			name: "priority has bounded range",
			rules: []DynamicRoutingRule{{
				ID: "invalid-priority", Enabled: true, SourceModel: "public", TargetModel: "upstream", Priority: MaxDynamicRoutingPriority + 1,
			}},
			want: "priority must be between",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDynamicRoutingRules(tt.rules)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}

	require.NoError(t, ValidateDynamicRoutingRules([]DynamicRoutingRule{{
		ID: "image-bridge-responses", Enabled: true,
		Action:      DynamicRoutingActionResponsesImageToolBridge,
		SourceModel: "gpt-5.6-sol", TargetModel: "gpt-image-2",
		TargetPath: "/v1/responses", RequestPaths: []string{"/v1/responses"},
	}}))
}

func TestEffectiveDynamicRoutingTargetPathDefaultsImageBridgeToImagesAPI(t *testing.T) {
	rule := DynamicRoutingRule{
		Action:     DynamicRoutingActionResponsesImageToolBridge,
		TargetPath: "",
	}
	assert.Equal(t, DynamicRoutingImageGenerationPath, EffectiveDynamicRoutingTargetPath(rule))

	rule.TargetPath = DynamicRoutingImageGenerationPath
	assert.Equal(t, DynamicRoutingImageGenerationPath, EffectiveDynamicRoutingTargetPath(rule))
}

func TestDynamicRoutingChannelConfigJSONAndValidation(t *testing.T) {
	disabled := false
	settings := ChannelSettings{
		DynamicRouting: &DynamicRoutingChannelConfig{
			Enabled: &disabled,
			Rules:   []DynamicRoutingRule{dynamicRoutingTestRule()},
		},
	}

	encoded, err := json.Marshal(settings)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"dynamic_routing"`)
	assert.Contains(t, string(encoded), `"enabled":false`)

	var decoded ChannelSettings
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.NotNil(t, decoded.DynamicRouting)
	require.NotNil(t, decoded.DynamicRouting.Enabled)
	assert.False(t, *decoded.DynamicRouting.Enabled)
	require.NoError(t, decoded.Validate())

	decoded.DynamicRouting.Rules[0].TargetModel = ""
	err = decoded.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target_model is required")
}
