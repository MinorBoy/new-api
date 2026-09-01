package helper

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func ModelMappedHelper(c *gin.Context, info *relaycommon.RelayInfo, request dto.Request) error {
	defer func() {
		info.PredictedUpstreamModel = info.UpstreamModelName
		if info.PredictedUpstreamModel == "" {
			info.PredictedUpstreamModel = info.OriginModelName
		}
	}()
	if info.ChannelMeta == nil {
		info.ChannelMeta = &relaycommon.ChannelMeta{}
	}
	// Routing may select an upstream image model without creating a capability
	// audit (image profiles use the catalog/SKU decision directly). Treat the
	// published routing model as authoritative before applying the channel's
	// ordinary model_mapping, so the selected provider identity reaches both
	// the adaptor and cost accounting.
	if routeUpstreamModel := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyRoutingUpstreamModel)); routeUpstreamModel != "" {
		info.UpstreamModelName = routeUpstreamModel
		info.IsModelMapped = info.UpstreamModelName != info.OriginModelName
		if request != nil {
			request.SetModelName(info.UpstreamModelName)
		}
		return nil
	}
	if info.Routing != nil && strings.TrimSpace(info.Routing.UpstreamModel) != "" {
		info.UpstreamModelName = info.Routing.UpstreamModel
		info.IsModelMapped = info.UpstreamModelName != info.OriginModelName
		if request != nil {
			request.SetModelName(info.UpstreamModelName)
		}
		return nil
	}

	isResponsesCompact := info.RelayMode == relayconstant.RelayModeResponsesCompact
	originModelName := info.OriginModelName
	mappingModelName := originModelName
	if isResponsesCompact && strings.HasSuffix(originModelName, ratio_setting.CompactModelSuffix) {
		mappingModelName = strings.TrimSuffix(originModelName, ratio_setting.CompactModelSuffix)
	}

	// map model name
	modelMapping := c.GetString("model_mapping")
	if modelMapping != "" && modelMapping != "{}" {
		mappedModel, changed, err := service.ResolveMappedModel(mappingModelName, modelMapping)
		if err != nil {
			if err.Error() == "model_mapping_contains_cycle" {
				return err
			}
			return fmt.Errorf("unmarshal_model_mapping_failed")
		}
		info.IsModelMapped = changed
		if changed {
			info.UpstreamModelName = mappedModel
		}
	}

	if isResponsesCompact {
		finalUpstreamModelName := mappingModelName
		if info.IsModelMapped && info.UpstreamModelName != "" {
			finalUpstreamModelName = info.UpstreamModelName
		}
		info.UpstreamModelName = finalUpstreamModelName
		info.OriginModelName = ratio_setting.WithCompactModelSuffix(finalUpstreamModelName)
	}
	if request != nil {
		request.SetModelName(info.UpstreamModelName)
	}
	return nil
}
