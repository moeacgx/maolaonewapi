package relay

import (
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

// shouldPassThroughRequestBody keeps the original body only when the selected
// route preserves its protocol. Dynamic bridges can change the target request
// shape, so they must always pass through the adaptor conversion path.
func shouldPassThroughRequestBody(info *relaycommon.RelayInfo) bool {
	if info == nil || info.ForceRequestConversion {
		return false
	}
	return model_setting.GetGlobalSettings().PassThroughRequestEnabled ||
		info.ChannelSetting.PassThroughBodyEnabled
}
