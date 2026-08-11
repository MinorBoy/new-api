package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateVideoRouteTargetContract(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
		settings    relaydto.ChannelOtherSettings
		target      modelrouting.Target
		wantCode    string
	}{
		{
			name: "4stoken keeps imported ARK media limits", channelType: constant.ChannelTypeFourSToken,
			target: videoContractTarget("4sdance933", []string{"720p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
		},
		{
			name: "8yes accepts resolution encoded by mapped model", channelType: constant.ChannelTypeEightYes,
			target: videoContractTarget("videos-mini-480p", []string{"480p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
		},
		{
			name: "8yes rejects resolution conflicting with mapped model", channelType: constant.ChannelTypeEightYes,
			target:   videoContractTarget("videos-mini-480p", []string{"720p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
			wantCode: "route_contract_resolution",
		},
		{
			name: "megabyai accepts 480p", channelType: constant.ChannelTypeMegaByAI,
			target: videoContractTarget("videos-mini", []string{"480p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
		},
		{
			name: "megabyai accepts 720p", channelType: constant.ChannelTypeMegaByAI,
			target: videoContractTarget("videos-standard", []string{"720p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
		},
		{
			name: "megabyai accepts 1080p", channelType: constant.ChannelTypeMegaByAI,
			target: videoContractTarget("videos-standard", []string{"1080p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
		},
		{
			name: "megabyai accepts 4k", channelType: constant.ChannelTypeMegaByAI,
			target: videoContractTarget("videos-standard", []string{"4k"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
		},
		{
			name: "megabyai rejects 1440p", channelType: constant.ChannelTypeMegaByAI,
			target:   videoContractTarget("videos-standard", []string{"1440p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
			wantCode: "route_contract_resolution",
		},
		{
			name: "omegaai accepts imported db image model", channelType: constant.ChannelTypeOmegaAI,
			target: func() modelrouting.Target {
				target := videoContractTargetWithMinimums("db-ai-video-v1", []string{"720p"}, 5, 10,
					[]modelrouting.InputMode{modelrouting.InputModeFirstFrame, modelrouting.InputModeOmniReference},
					modelrouting.ReferenceLimits{Images: 9}, modelrouting.ReferenceLimits{Images: 1})
				target.Constraints.ReferenceTotalMax = common.GetPointer(9)
				return target
			}(),
		},
		{
			name: "omegaai rejects unknown mapped model", channelType: constant.ChannelTypeOmegaAI,
			target:   videoContractTarget("unknown-omega-model", []string{"720p"}, 5, 10, nil, modelrouting.ReferenceLimits{Images: 9}),
			wantCode: "route_contract_model",
		},
		{
			name: "omegaai rejects unsupported resolution", channelType: constant.ChannelTypeOmegaAI,
			target:   videoContractTarget("db-ai-video-v1", []string{"1080p"}, 5, 10, nil, modelrouting.ReferenceLimits{Images: 9}),
			wantCode: "route_contract_resolution",
		},
		{
			name: "omegaai rejects imported image model video capability", channelType: constant.ChannelTypeOmegaAI,
			target:   videoContractTarget("db-ai-video-v1", []string{"720p"}, 5, 10, nil, modelrouting.ReferenceLimits{Images: 9, Videos: 1}),
			wantCode: "route_contract_references",
		},
		{
			name: "omegaai rejects duration above task limit", channelType: constant.ChannelTypeOmegaAI,
			target:   videoContractTarget("db-ai-video-v1", []string{"720p"}, 5, relaycommon.MaxTaskDurationSeconds+1, nil, modelrouting.ReferenceLimits{Images: 9}),
			wantCode: "route_contract_duration",
		},
		{
			name: "cangyuan accepts documented media limits", channelType: constant.ChannelTypeCangyuan,
			target: videoContractTargetWithCangyuanLimits("seedance-2.0", []string{"480p", "720p"}, 4, 15,
				[]modelrouting.InputMode{modelrouting.InputModeText, modelrouting.InputModeFirstFrame, modelrouting.InputModeFirstLastFrames, modelrouting.InputModeOmniReference},
				modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1}, 8, 15),
		},
		{
			name: "cangyuan rejects five images", channelType: constant.ChannelTypeCangyuan,
			target: videoContractTargetWithCangyuanLimits("seedance-2.0", []string{"720p"}, 4, 15, nil,
				modelrouting.ReferenceLimits{Images: 5, Videos: 3, Audios: 1}, 9, 15),
			wantCode: "route_contract_references",
		},
		{
			name: "cangyuan sd5 accepts documented shared media limits", channelType: constant.ChannelTypeCangyuan,
			target: func() modelrouting.Target {
				target := videoContractTargetWithCangyuanLimits("sd5-seedance-2.0-fast", []string{"720p"}, 4, 15,
					[]modelrouting.InputMode{modelrouting.InputModeText, modelrouting.InputModeFirstLastFrames, modelrouting.InputModeOmniReference},
					modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, 12, 15)
				target.Constraints.ReferenceVideoAudioTotalMax = common.GetPointer(3)
				return target
			}(),
		},
		{
			name: "cangyuan rejects unsupported resolution", channelType: constant.ChannelTypeCangyuan,
			target: videoContractTargetWithCangyuanLimits("seedance-2.0", []string{"1080p"}, 4, 15, nil,
				modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1}, 8, 15),
			wantCode: "route_contract_resolution",
		},
		{
			name: "cangyuan rejects four-second-below duration", channelType: constant.ChannelTypeCangyuan,
			target: videoContractTargetWithCangyuanLimits("seedance-2.0", []string{"720p"}, 3, 15, nil,
				modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1}, 8, 15),
			wantCode: "route_contract_duration",
		},
		{
			name: "paipu accepts an imported reference route", channelType: constant.ChannelTypePaipu,
			target: videoContractTargetWithMinimums("vendor-model-from-import-v9", []string{"720p"}, 4, 15,
				[]modelrouting.InputMode{modelrouting.InputModeOmniReference},
				modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, modelrouting.ReferenceLimits{}),
		},
		{
			name: "paipu accepts an imported text-only route", channelType: constant.ChannelTypePaipu,
			target: videoContractTarget("vendor-model-from-import-v9", []string{"720p"}, 4, 15,
				[]modelrouting.InputMode{modelrouting.InputModeText}, modelrouting.ReferenceLimits{}),
		},
		{
			name: "paipu rejects ten images", channelType: constant.ChannelTypePaipu,
			target: videoContractTargetWithMinimums("vendor-model-from-import-v9", []string{"720p"}, 4, 15,
				[]modelrouting.InputMode{modelrouting.InputModeOmniReference},
				modelrouting.ReferenceLimits{Images: 10, Videos: 3, Audios: 3}, modelrouting.ReferenceLimits{}),
			wantCode: "route_contract_references",
		},
		{
			name: "paipu rejects four videos", channelType: constant.ChannelTypePaipu,
			target: videoContractTargetWithMinimums("vendor-model-from-import-v9", []string{"720p"}, 4, 15,
				[]modelrouting.InputMode{modelrouting.InputModeOmniReference},
				modelrouting.ReferenceLimits{Images: 9, Videos: 4, Audios: 3}, modelrouting.ReferenceLimits{}),
			wantCode: "route_contract_references",
		},
		{
			name: "paipu rejects four audios", channelType: constant.ChannelTypePaipu,
			target: videoContractTargetWithMinimums("vendor-model-from-import-v9", []string{"720p"}, 4, 15,
				[]modelrouting.InputMode{modelrouting.InputModeOmniReference},
				modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 4}, modelrouting.ReferenceLimits{}),
			wantCode: "route_contract_references",
		},
		{
			name: "paipu rejects an empty upstream model", channelType: constant.ChannelTypePaipu,
			target: videoContractTargetWithMinimums("  ", []string{"720p"}, 4, 15,
				[]modelrouting.InputMode{modelrouting.InputModeOmniReference},
				modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, modelrouting.ReferenceLimits{}),
			wantCode: "route_contract_model",
		},
		{
			name: "paipu rejects an oversized duration", channelType: constant.ChannelTypePaipu,
			target: videoContractTargetWithMinimums("vendor-model-from-import-v9", []string{"720p"}, 4, relaycommon.MaxTaskDurationSeconds+1,
				[]modelrouting.InputMode{modelrouting.InputModeOmniReference},
				modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, modelrouting.ReferenceLimits{}),
			wantCode: "route_contract_duration",
		},
		{
			name: "paipu rejects minimums above limits", channelType: constant.ChannelTypePaipu,
			target: videoContractTargetWithMinimums("vendor-model-from-import-v9", []string{"720p"}, 4, 15,
				[]modelrouting.InputMode{modelrouting.InputModeOmniReference},
				modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 3}, modelrouting.ReferenceLimits{Images: 5}),
			wantCode: "route_contract_references",
		},
		{
			name: "z5api accepts imported omni reference route", channelType: constant.ChannelTypeZ5API,
			target: videoContractTargetWithMinimums("sd-2-c6-imported", []string{"720p"}, 1, 15,
				[]modelrouting.InputMode{modelrouting.InputModeOmniReference},
				modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, modelrouting.ReferenceLimits{}),
		},
		{
			name: "z5api accepts imported text-only route", channelType: constant.ChannelTypeZ5API,
			target: videoContractTarget("sd-2-c6-imported", []string{"720p"}, 1, 15,
				[]modelrouting.InputMode{modelrouting.InputModeText}, modelrouting.ReferenceLimits{}),
		},
		{
			name: "z5api rejects ten images", channelType: constant.ChannelTypeZ5API,
			target: videoContractTarget("sd-2-c6-imported", []string{"720p"}, 1, 15, nil,
				modelrouting.ReferenceLimits{Images: 10, Videos: 3, Audios: 3}),
			wantCode: "route_contract_references",
		},
		{
			name: "z5api rejects four videos", channelType: constant.ChannelTypeZ5API,
			target: videoContractTarget("sd-2-c6-imported", []string{"720p"}, 1, 15, nil,
				modelrouting.ReferenceLimits{Images: 9, Videos: 4, Audios: 3}),
			wantCode: "route_contract_references",
		},
		{
			name: "z5api rejects four audios", channelType: constant.ChannelTypeZ5API,
			target: videoContractTarget("sd-2-c6-imported", []string{"720p"}, 1, 15, nil,
				modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 4}),
			wantCode: "route_contract_references",
		},
		{
			name: "z5api rejects an empty upstream model", channelType: constant.ChannelTypeZ5API,
			target: videoContractTarget("  ", []string{"720p"}, 1, 15, nil,
				modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
			wantCode: "route_contract_model",
		},
		{
			name: "z5api rejects an oversized duration", channelType: constant.ChannelTypeZ5API,
			target: videoContractTarget("sd-2-c6-imported", []string{"720p"}, 1, relaycommon.MaxTaskDurationSeconds+1, nil,
				modelrouting.ReferenceLimits{}),
			wantCode: "route_contract_duration",
		},
		{
			name: "z5api rejects minimums above limits", channelType: constant.ChannelTypeZ5API,
			target: videoContractTargetWithMinimums("sd-2-c6-imported", []string{"720p"}, 1, 15, nil,
				modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 3}, modelrouting.ReferenceLimits{Images: 5}),
			wantCode: "route_contract_references",
		},
		{
			name: "zzone accepts documented references", channelType: constant.ChannelTypeZZone,
			target: func() modelrouting.Target {
				target := videoContractTargetWithMinimums("imported-zzone-model", []string{"720p"}, 1, 15,
					[]modelrouting.InputMode{modelrouting.InputModeText, modelrouting.InputModeOmniReference},
					modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1}, modelrouting.ReferenceLimits{})
				target.Constraints.AspectRatios = []string{"16:9", "9:16", "1:1"}
				return target
			}(),
		},
		{
			name: "zzone accepts imported text-only route", channelType: constant.ChannelTypeZZone,
			target: videoContractTarget("imported-zzone-model", []string{"720p"}, 1, 15,
				[]modelrouting.InputMode{modelrouting.InputModeText}, modelrouting.ReferenceLimits{}),
		},
		{
			name: "zzone rejects five images", channelType: constant.ChannelTypeZZone,
			target: videoContractTarget("imported-zzone-model", []string{"720p"}, 1, 15, nil,
				modelrouting.ReferenceLimits{Images: 5, Videos: 3, Audios: 1}),
			wantCode: "route_contract_references",
		},
		{
			name: "zzone rejects four videos", channelType: constant.ChannelTypeZZone,
			target: videoContractTarget("imported-zzone-model", []string{"720p"}, 1, 15, nil,
				modelrouting.ReferenceLimits{Images: 4, Videos: 4, Audios: 1}),
			wantCode: "route_contract_references",
		},
		{
			name: "zzone rejects two audios", channelType: constant.ChannelTypeZZone,
			target: videoContractTarget("imported-zzone-model", []string{"720p"}, 1, 15, nil,
				modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 2}),
			wantCode: "route_contract_references",
		},
		{
			name: "zzone rejects empty upstream model", channelType: constant.ChannelTypeZZone,
			target:   videoContractTarget("  ", []string{"720p"}, 1, 15, nil, modelrouting.ReferenceLimits{}),
			wantCode: "route_contract_model",
		},
		{
			name: "zzone rejects oversized duration", channelType: constant.ChannelTypeZZone,
			target: videoContractTarget("imported-zzone-model", []string{"720p"}, 1, relaycommon.MaxTaskDurationSeconds+1, nil,
				modelrouting.ReferenceLimits{}),
			wantCode: "route_contract_duration",
		},
		{
			name: "zzone rejects undocumented aspect ratio", channelType: constant.ChannelTypeZZone,
			target: func() modelrouting.Target {
				target := videoContractTarget("imported-zzone-model", []string{"720p"}, 1, 15, nil, modelrouting.ReferenceLimits{})
				target.Constraints.AspectRatios = []string{"16:9", "21:9"}
				return target
			}(),
			wantCode: "route_contract_ratio",
		},
		{
			name: "zzone rejects minimums above limits", channelType: constant.ChannelTypeZZone,
			target: videoContractTargetWithMinimums("imported-zzone-model", []string{"720p"}, 1, 15, nil,
				modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1}, modelrouting.ReferenceLimits{Images: 5}),
			wantCode: "route_contract_references",
		},
		{
			name: "Mikoto accepts documented Sora limits", channelType: constant.ChannelTypeMikoto,
			target: videoContractTargetWithTotal("sora-v3-pro", []string{"720p"}, 4, 15,
				[]modelrouting.InputMode{modelrouting.InputModeText, modelrouting.InputModeFirstFrame, modelrouting.InputModeFirstLastFrames, modelrouting.InputModeOmniReference},
				modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, 12),
		},
		{
			name: "Mikoto rejects Sora 1080p", channelType: constant.ChannelTypeMikoto,
			target:   videoContractTarget("sora-v3-pro", []string{"1080p"}, 4, 15, nil, modelrouting.ReferenceLimits{}),
			wantCode: "route_contract_resolution",
		},
		{
			name: "Mikoto accepts documented Seedance 1080p model", channelType: constant.ChannelTypeMikoto,
			target: videoContractTarget("seedance-2.0-1080p", []string{"1080p"}, 4, 15,
				[]modelrouting.InputMode{modelrouting.InputModeText, modelrouting.InputModeFirstFrame, modelrouting.InputModeFirstLastFrames, modelrouting.InputModeOmniReference},
				modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
		},
		{
			name: "Mikoto rejects Seedance model resolution mismatch", channelType: constant.ChannelTypeMikoto,
			target:   videoContractTarget("seedance-fast-480p", []string{"720p"}, 4, 15, nil, modelrouting.ReferenceLimits{}),
			wantCode: "route_contract_resolution",
		},
		{
			name: "Mikoto rejects unknown model", channelType: constant.ChannelTypeMikoto,
			target:   videoContractTarget("unverified-model", []string{"720p"}, 4, 15, nil, modelrouting.ReferenceLimits{}),
			wantCode: "route_contract_model",
		},
		{
			name: "Mikoto rejects duration below four", channelType: constant.ChannelTypeMikoto,
			target:   videoContractTarget("seedance-2.0-720p", []string{"720p"}, 3, 15, nil, modelrouting.ReferenceLimits{}),
			wantCode: "route_contract_duration",
		},
		{
			name: "Mikoto rejects ten images", channelType: constant.ChannelTypeMikoto,
			target:   videoContractTarget("seedance-2.0-720p", []string{"720p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 10}),
			wantCode: "route_contract_references",
		},
		{
			name: "clmm accepts imported audio capability", channelType: constant.ChannelTypeClmmMall,
			target: videoContractTarget("op-video-720p", []string{"720p"}, 5, 15, nil, modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1}),
		},
		{
			name: "clmm rejects more than three audios", channelType: constant.ChannelTypeClmmMall,
			target:   videoContractTarget("op-video-720p", []string{"720p"}, 5, 15, nil, modelrouting.ReferenceLimits{Audios: 4}),
			wantCode: "route_contract_references",
		},
		{
			name: "clmm rejects aggregate maximum above fifteen", channelType: constant.ChannelTypeClmmMall,
			target:   videoContractTargetWithTotal("op-video-720p", []string{"720p"}, 5, 15, nil, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, 16),
			wantCode: "route_contract_references",
		},
		{
			name: "clmm rejects 1080p", channelType: constant.ChannelTypeClmmMall,
			target:   videoContractTarget("op-video-720p", []string{"1080p"}, 5, 15, nil, modelrouting.ReferenceLimits{Images: 4, Videos: 3}),
			wantCode: "route_contract_resolution",
		},
		{
			name: "clmm accepts configured model id", channelType: constant.ChannelTypeClmmMall,
			target: videoContractTarget("mg-seedance2.0 -720p pro", []string{"720p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 4, Videos: 3}),
		},
		{
			name: "clmm accepts declared four-second duration", channelType: constant.ChannelTypeClmmMall,
			target: videoContractTarget("op-video-720p", []string{"720p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 4, Videos: 3}),
		},
		{
			name: "dimensio rejects combined maxima above twelve", channelType: constant.ChannelTypeDimensio,
			target:   videoContractTarget("jmg-video-seedance-2.0-vip", []string{"720p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
			wantCode: "route_contract_references",
		},
		{
			name: "dimensio accepts the real jmg model id", channelType: constant.ChannelTypeDimensio,
			target: videoContractTargetWithTotal("jmg-video-seedance-2.0-vip", []string{"1080p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, 12),
		},
		{
			name: "dimensio accepts the real pxv 2160p model id", channelType: constant.ChannelTypeDimensio,
			target: videoContractTarget("pxv-seedance-2.0-standard", []string{"4k"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
		},
		{
			name: "dimensio accepts imported hgf model", channelType: constant.ChannelTypeDimensio,
			target: videoContractTargetWithTotal("hgf-seedance-2.0", []string{"4k"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, 15),
		},
		{
			name: "dimensio accepts imported dvc model", channelType: constant.ChannelTypeDimensio,
			target: videoContractTargetWithTotal("dvc-seedance-2.0", []string{"1080p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 9}, 9),
		},
		{
			name: "dimensio rejects empty configured model", channelType: constant.ChannelTypeDimensio,
			target:   videoContractTarget("  ", []string{"720p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 9}),
			wantCode: "route_contract_model",
		},
		{
			name: "dimensio limits 1080p to vip", channelType: constant.ChannelTypeDimensio,
			target:   videoContractTarget("jmg-video-seedance-2.0-fast-vip", []string{"1080p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1}),
			wantCode: "route_contract_resolution",
		},
		{
			name: "secure discount requires an image", channelType: constant.ChannelTypeSecure,
			settings: relaydto.ChannelOtherSettings{SecureVideoGroup: relaydto.SecureVideoGroupDiscount},
			target:   videoContractTarget("video-2.0-pro", []string{"720p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
			wantCode: "route_contract_references",
		},
		{
			name: "secure overseas rejects combined maxima above twelve", channelType: constant.ChannelTypeSecure,
			settings: relaydto.ChannelOtherSettings{SecureVideoGroup: relaydto.SecureVideoGroupOverseas},
			target:   videoContractTarget("video-2.0-pro", []string{"720p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
			wantCode: "route_contract_references",
		},
		{
			name: "secure enterprise rejects four-second minimum", channelType: constant.ChannelTypeSecure,
			settings: relaydto.ChannelOtherSettings{SecureVideoGroup: relaydto.SecureVideoGroupEnterprise},
			target:   videoContractTarget("video-2.0-pro", []string{"720p"}, 4, 15, []modelrouting.InputMode{modelrouting.InputModeText}, modelrouting.ReferenceLimits{}),
			wantCode: "route_contract_duration",
		},
		{
			name: "secure enterprise rejects reference video capability", channelType: constant.ChannelTypeSecure,
			settings: relaydto.ChannelOtherSettings{SecureVideoGroup: relaydto.SecureVideoGroupEnterprise},
			target:   videoContractTarget("video-2.0-pro", []string{"720p"}, 5, 15, nil, modelrouting.ReferenceLimits{Images: 9, Videos: 1, Audios: 3}),
			wantCode: "route_contract_references",
		},
		{
			name: "secure fast rejects 1080p", channelType: constant.ChannelTypeSecure,
			settings: relaydto.ChannelOtherSettings{SecureVideoGroup: relaydto.SecureVideoGroupOverseas},
			target:   videoContractTarget("video-2.0-fast", []string{"1080p"}, 4, 15, []modelrouting.InputMode{modelrouting.InputModeText}, modelrouting.ReferenceLimits{}),
			wantCode: "route_contract_resolution",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &model.Channel{Type: tt.channelType}
			channel.SetOtherSettings(tt.settings)

			err := ValidateVideoRouteTargetContract(channel, tt.target)
			if tt.wantCode == "" {
				require.NoError(t, err)
				return
			}
			var contractErr *VideoRouteContractError
			require.ErrorAs(t, err, &contractErr)
			assert.Equal(t, tt.wantCode, contractErr.Code)
		})
	}
}

func videoContractTarget(modelName string, resolutions []string, minDuration, maxDuration int, inputModes []modelrouting.InputMode, limits modelrouting.ReferenceLimits) modelrouting.Target {
	return videoContractTargetWithMinimums(modelName, resolutions, minDuration, maxDuration, inputModes, limits, modelrouting.ReferenceLimits{})
}

func videoContractTargetWithTotal(modelName string, resolutions []string, minDuration, maxDuration int, inputModes []modelrouting.InputMode, limits modelrouting.ReferenceLimits, total int) modelrouting.Target {
	target := videoContractTarget(modelName, resolutions, minDuration, maxDuration, inputModes, limits)
	target.Constraints.ReferenceTotalMax = common.GetPointer(total)
	return target
}

func videoContractTargetWithCangyuanLimits(modelName string, resolutions []string, minDuration, maxDuration int, inputModes []modelrouting.InputMode, limits modelrouting.ReferenceLimits, total, videoDuration int) modelrouting.Target {
	target := videoContractTargetWithTotal(modelName, resolutions, minDuration, maxDuration, inputModes, limits, total)
	target.Constraints.ReferenceVideoTotalDurationSeconds = common.GetPointer(videoDuration)
	return target
}

func videoContractTargetWithMinimums(modelName string, resolutions []string, minDuration, maxDuration int, inputModes []modelrouting.InputMode, limits, minimums modelrouting.ReferenceLimits) modelrouting.Target {
	return modelrouting.Target{
		UpstreamModel: modelName,
		Constraints: modelrouting.Constraints{
			OutputResolutions: resolutions,
			Durations: modelrouting.DurationConstraint{
				Min: common.GetPointer(minDuration), Max: common.GetPointer(maxDuration),
			},
			InputModes: inputModes, ReferenceLimits: limits, ReferenceMinimums: minimums,
		},
	}
}
