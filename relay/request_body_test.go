package relay

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
)

func TestShouldPassThroughRequestBodyHonorsForcedConversion(t *testing.T) {
	globalSettings := model_setting.GetGlobalSettings()
	previousGlobal := globalSettings.PassThroughRequestEnabled
	globalSettings.PassThroughRequestEnabled = true
	t.Cleanup(func() {
		globalSettings.PassThroughRequestEnabled = previousGlobal
	})

	forced := &relaycommon.RelayInfo{
		ForceRequestConversion: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{PassThroughBodyEnabled: true},
		},
	}
	assert.False(t, shouldPassThroughRequestBody(forced))

	normal := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{PassThroughBodyEnabled: true},
		},
	}
	assert.True(t, shouldPassThroughRequestBody(normal))
}
