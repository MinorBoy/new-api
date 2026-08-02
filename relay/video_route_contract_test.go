package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
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
			name: "cangyuan rejects a route that advertises media", channelType: constant.ChannelTypeCangyuan,
			target:   videoContractTarget("seedance-2.0", []string{"720p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
			wantCode: "route_contract_input_mode",
		},
		{
			name: "cangyuan accepts a text-only route", channelType: constant.ChannelTypeCangyuan,
			target: videoContractTarget("seedance-2.0", []string{"720p"}, 4, 15, []modelrouting.InputMode{modelrouting.InputModeText}, modelrouting.ReferenceLimits{}),
		},
		{
			name: "paipu rejects a route that advertises media", channelType: constant.ChannelTypePaipu,
			target:   videoContractTarget("lec-seedance-2-0", []string{"720p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1}),
			wantCode: "route_contract_input_mode",
		},
		{
			name: "paipu enforces mapped resolution suffix", channelType: constant.ChannelTypePaipu,
			target:   videoContractTarget("lec-feituo-seedance-2-0-my-upscaled-1080p", []string{"720p"}, 4, 15, []modelrouting.InputMode{modelrouting.InputModeText}, modelrouting.ReferenceLimits{}),
			wantCode: "route_contract_resolution",
		},
		{
			name: "clmm rejects audio declaration", channelType: constant.ChannelTypeClmmMall,
			target:   videoContractTarget("op-video-720p", []string{"720p"}, 5, 15, nil, modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1}),
			wantCode: "route_contract_references",
		},
		{
			name: "clmm rejects 1080p", channelType: constant.ChannelTypeClmmMall,
			target:   videoContractTarget("op-video-720p", []string{"1080p"}, 5, 15, nil, modelrouting.ReferenceLimits{Images: 4, Videos: 3}),
			wantCode: "route_contract_resolution",
		},
		{
			name: "clmm rejects unknown mapped model grammar", channelType: constant.ChannelTypeClmmMall,
			target:   videoContractTarget("mg-seedance2.0 -720p pro", []string{"720p"}, 5, 15, nil, modelrouting.ReferenceLimits{Images: 4, Videos: 3}),
			wantCode: "route_contract_model",
		},
		{
			name: "clmm rejects declared four-second duration", channelType: constant.ChannelTypeClmmMall,
			target:   videoContractTarget("op-video-720p", []string{"720p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 4, Videos: 3}),
			wantCode: "route_contract_duration",
		},
		{
			name: "dimensio rejects combined maxima above twelve", channelType: constant.ChannelTypeDimensio,
			target:   videoContractTarget("jimeng-video-seedance-2.0-vip", []string{"720p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}),
			wantCode: "route_contract_references",
		},
		{
			name: "dimensio rejects unknown imported model", channelType: constant.ChannelTypeDimensio,
			target:   videoContractTarget("dimensio-seedance-2.0", []string{"720p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1}),
			wantCode: "route_contract_model",
		},
		{
			name: "dimensio limits 1080p to vip", channelType: constant.ChannelTypeDimensio,
			target:   videoContractTarget("jimeng-video-seedance-2.0-fast-vip", []string{"1080p"}, 4, 15, nil, modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1}),
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
	return modelrouting.Target{
		UpstreamModel: modelName,
		Constraints: modelrouting.Constraints{
			OutputResolutions: resolutions,
			Durations: modelrouting.DurationConstraint{
				Min: common.GetPointer(minDuration), Max: common.GetPointer(maxDuration),
			},
			InputModes: inputModes, ReferenceLimits: limits,
		},
	}
}
