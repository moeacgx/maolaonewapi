package helper

import (
	"errors"
	"fmt"
	"strings"

	appcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func ModelMappedHelper(c *gin.Context, info *relaycommon.RelayInfo, request dto.Request) error {
	if info.ChannelMeta == nil {
		info.ChannelMeta = &relaycommon.ChannelMeta{}
	}

	isResponsesCompact := info.RelayMode == relayconstant.RelayModeResponsesCompact
	originModelName := info.OriginModelName
	mappingModelName := originModelName
	if isResponsesCompact && strings.HasSuffix(originModelName, ratio_setting.CompactModelSuffix) {
		mappingModelName = strings.TrimSuffix(originModelName, ratio_setting.CompactModelSuffix)
	}

	// map model name
	modelMapping := c.GetString("model_mapping")
	exactCompactAliasMapped := false
	if modelMapping != "" && modelMapping != "{}" {
		modelMap := make(map[string]string)
		err := appcommon.Unmarshal([]byte(modelMapping), &modelMap)
		if err != nil {
			return fmt.Errorf("unmarshal_model_mapping_failed")
		}

		// 支持链式模型重定向，最终使用链尾的模型。
		// Responses compact 先尝试精确匹配原始别名；没有显式 compact 别名映射时，
		// 保留旧行为：去掉 -openai-compact 后缀再查基础模型映射。
		currentModel := mappingModelName
		if isResponsesCompact {
			if mappedModel, exists := modelMap[originModelName]; exists && strings.TrimSpace(mappedModel) != "" {
				currentModel = originModelName
				exactCompactAliasMapped = true
			}
		}
		visitedModels := map[string]bool{
			currentModel: true,
		}
		for {
			if mappedModel, exists := modelMap[currentModel]; exists && strings.TrimSpace(mappedModel) != "" {
				mappedModel = strings.TrimSpace(mappedModel)
				// 模型重定向循环检测，避免无限循环
				if visitedModels[mappedModel] {
					if mappedModel == currentModel {
						if currentModel == info.OriginModelName {
							info.IsModelMapped = false
							return nil
						} else {
							info.IsModelMapped = true
							break
						}
					}
					return errors.New("model_mapping_contains_cycle")
				}
				visitedModels[mappedModel] = true
				currentModel = mappedModel
				info.IsModelMapped = true
			} else {
				break
			}
		}
		if info.IsModelMapped {
			info.UpstreamModelName = currentModel
		}
	}

	if isResponsesCompact {
		finalUpstreamModelName := mappingModelName
		if info.IsModelMapped && info.UpstreamModelName != "" {
			finalUpstreamModelName = info.UpstreamModelName
		}
		info.UpstreamModelName = finalUpstreamModelName
		if exactCompactAliasMapped && !strings.HasSuffix(finalUpstreamModelName, ratio_setting.CompactModelSuffix) {
			info.RelayMode = relayconstant.RelayModeResponses
			info.RequestURLPath = "/v1/responses"
		} else {
			info.OriginModelName = ratio_setting.WithCompactModelSuffix(finalUpstreamModelName)
		}
	}
	if request != nil {
		request.SetModelName(info.UpstreamModelName)
	}
	return nil
}
