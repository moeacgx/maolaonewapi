package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func TestOfficialCustomChannelTypeMappings(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
		wantAPIType int
	}{
		{"advanced custom", constant.ChannelTypeAdvancedCustom, constant.APITypeAdvancedCustom},
		{"sub2api", constant.ChannelTypeSub2API, constant.APITypeSub2API},
		{"newapi", constant.ChannelTypeNewAPI, constant.APITypeNewAPI},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ChannelType2APIType(tt.channelType)
			if !ok {
				t.Fatalf("ChannelType2APIType(%d) ok = false", tt.channelType)
			}
			if got != tt.wantAPIType {
				t.Fatalf("ChannelType2APIType(%d) = %d, want %d", tt.channelType, got, tt.wantAPIType)
			}
			if !IsResponsesCompactAPIType(got) {
				t.Fatalf("IsResponsesCompactAPIType(%d) = false", got)
			}
		})
	}
}
