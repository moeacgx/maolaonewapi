package tencent

import (
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

const tokenHubBaseURL = "https://tokenhub.tencentmaas.com"

// DispatchAdaptor routes Tencent Hunyuan credentials by format:
// AppId|SecretId|SecretKey uses the native TC3 adaptor, while a single
// TokenHub API key uses Tencent's OpenAI-compatible endpoint.
type DispatchAdaptor struct {
	channel.Adaptor
}

func (a *DispatchAdaptor) Init(info *relaycommon.RelayInfo) {
	apiKey := strings.TrimPrefix(info.ApiKey, "Bearer ")
	if strings.Contains(apiKey, "|") {
		a.Adaptor = &Adaptor{}
	} else {
		a.Adaptor = &openai.Adaptor{}
		if info.ChannelBaseUrl == "" || info.ChannelBaseUrl == constant.ChannelBaseURLs[constant.ChannelTypeTencent] {
			info.ChannelBaseUrl = tokenHubBaseURL
		}
	}
	a.Adaptor.Init(info)
}

func (a *DispatchAdaptor) GetModelList() []string {
	return ModelList
}

func (a *DispatchAdaptor) GetChannelName() string {
	return ChannelName
}
