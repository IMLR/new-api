package helper

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
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

	modelMap := make(map[string]string)
	modelMapping := common.GetContextKeyString(c, constant.ContextKeyChannelModelMapping)
	if modelMapping != "" && modelMapping != "{}" {
		if err := common.UnmarshalJsonStr(modelMapping, &modelMap); err != nil {
			return fmt.Errorf("unmarshal_model_mapping_failed")
		}
	}
	currentModel := mappingModelName
	// An explicit mapping for the client name wins. Otherwise matching supplies
	// the channel's native name, which can itself have an explicit mapping.
	if modelMap[currentModel] == "" {
		if matched := common.GetContextKeyString(c, constant.ContextKeyChannelMatchedModel); matched != "" {
			currentModel = matched
		}
	}
	visitedModels := map[string]bool{currentModel: true}
	for {
		mappedModel := modelMap[currentModel]
		if mappedModel == "" || mappedModel == currentModel {
			break
		}
		if visitedModels[mappedModel] {
			return errors.New("model_mapping_contains_cycle")
		}
		visitedModels[mappedModel] = true
		currentModel = mappedModel
	}
	info.UpstreamModelName = currentModel
	info.IsModelMapped = currentModel != mappingModelName

	if isResponsesCompact {
		finalUpstreamModelName := mappingModelName
		if info.IsModelMapped && info.UpstreamModelName != "" {
			finalUpstreamModelName = info.UpstreamModelName
		}
		info.UpstreamModelName = finalUpstreamModelName
		billingModelName := finalUpstreamModelName
		if common.GetContextKeyString(c, constant.ContextKeyChannelMatchedModel) != "" && modelMap[mappingModelName] == "" {
			billingModelName = mappingModelName
		}
		info.OriginModelName = ratio_setting.WithCompactModelSuffix(billingModelName)
	}
	if request != nil {
		request.SetModelName(info.UpstreamModelName)
	}
	return nil
}
