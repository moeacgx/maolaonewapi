package controller

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

// convertChannelToMultiKey 仅准备显式单密钥转换，持久化和缓存刷新仍由渠道更新负责。
func convertChannelToMultiKey(channel *PatchChannel, origin *model.Channel) error {
	for _, channelType := range []int{origin.Type, channel.Type} {
		if channelType == constant.ChannelTypeCodex || channelType == constant.ChannelTypeVertexAi {
			return fmt.Errorf("该渠道类型不支持在编辑时转换为密钥聚合模式")
		}
	}
	if origin.GetOtherInfo()["source"] == "ionet" {
		return fmt.Errorf("部署托管渠道不支持转换为密钥聚合模式")
	}
	mode := constant.MultiKeyModeRandom
	if channel.MultiKeyMode != nil {
		mode = constant.MultiKeyMode(*channel.MultiKeyMode)
	}
	if mode != constant.MultiKeyModeRandom && mode != constant.MultiKeyModePolling {
		return fmt.Errorf("多密钥使用策略必须为 random 或 polling")
	}
	keyMode := "append"
	if channel.KeyMode != nil {
		keyMode = *channel.KeyMode
	}
	if keyMode != "append" && keyMode != "replace" {
		return fmt.Errorf("密钥更新模式必须为 append 或 replace")
	}
	if keyMode == "replace" && strings.TrimSpace(channel.Key) == "" {
		return fmt.Errorf("覆盖模式必须提供至少一个有效密钥")
	}
	keyText := channel.Key
	if keyMode == "append" {
		keyText = origin.Key + "\n" + keyText
	}
	keys := make([]string, 0)
	seen := make(map[string]struct{})
	for _, line := range strings.Split(keyText, "\n") {
		key := strings.TrimSpace(line)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return fmt.Errorf("密钥聚合模式必须包含至少一个有效密钥")
	}
	channel.Key = strings.Join(keys, "\n")
	channel.ChannelInfo = model.ChannelInfo{
		IsMultiKey:   true,
		MultiKeyMode: mode,
		MultiKeySize: len(keys),
	}
	return nil
}
