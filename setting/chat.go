package setting

import (
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
)

var ccSwitchAPIAddress atomic.Pointer[string]

func GetCCSwitchAPIAddress() string {
	address := ccSwitchAPIAddress.Load()
	if address == nil {
		return ""
	}
	return *address
}

func SetCCSwitchAPIAddress(address string) {
	ccSwitchAPIAddress.Store(&address)
}

var Chats = []map[string]string{
	//{
	//	"ChatGPT Next Web 官方示例": "https://app.nextchat.dev/#/?settings={\"key\":\"{key}\",\"url\":\"{address}\"}",
	//},
	{
		"Cherry Studio": "cherrystudio://providers/api-keys?v=1&data={cherryConfig}",
	},
	{
		"AionUI": "aionui://provider/add?v=1&data={aionuiConfig}",
	},
	{
		"流畅阅读": "fluentread",
	},
	{
		"CC Switch": "ccswitch",
	},
	{
		"DeepChat": "deepchat://provider/install?v=1&data={deepchatConfig}",
	},
	{
		"Lobe Chat 官方示例": "https://chat-preview.lobehub.com/?settings={\"keyVaults\":{\"openai\":{\"apiKey\":\"{key}\",\"baseURL\":\"{address}/v1\"}}}",
	},
	{
		"AI as Workspace": "https://aiaw.app/set-provider?provider={\"type\":\"openai\",\"settings\":{\"apiKey\":\"{key}\",\"baseURL\":\"{address}/v1\",\"compatibility\":\"strict\"}}",
	},
	{
		"AMA 问天": "ama://set-api-key?server={address}&key={key}",
	},
	{
		"OpenCat": "opencat://team/join?domain={address}&token={key}",
	},
}

func UpdateChatsByJsonString(jsonString string) error {
	next := make([]map[string]string, 0)
	if err := common.UnmarshalJsonStr(jsonString, &next); err != nil {
		return err
	}
	Chats = next
	return nil
}

func Chats2JsonString() string {
	jsonBytes, err := common.Marshal(Chats)
	if err != nil {
		common.SysLog("error marshalling chats: " + err.Error())
		return "[]"
	}
	return string(jsonBytes)
}

func NormalizeCCSwitchAPIAddress(value string) (string, error) {
	normalized := strings.TrimRight(strings.TrimSpace(value), "/")
	if normalized == "" {
		return "", nil
	}
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("CC Switch API 地址必须是以 http:// 或 https:// 开头的绝对地址")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("CC Switch API 地址不能包含用户名或密码")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("CC Switch API 地址不能包含查询参数或锚点")
	}
	return normalized, nil
}
