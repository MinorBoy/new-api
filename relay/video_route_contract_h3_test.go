package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiniMaxH3RouteRequiresMatchingCostVariant(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeNewAPIVideo}
	min, max := 4, 15
	maxReferences := 15
	base := modelrouting.Target{
		UpstreamModel:  modelrouting.MiniMaxH3VIP,
		CostVariantKey: "720p",
		Constraints: modelrouting.Constraints{
			OutputResolutions: []string{"720p"},
			Durations:         modelrouting.DurationConstraint{Min: &min, Max: &max},
			AspectRatios:      []string{"16:9"},
			ReferenceLimits:   modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
			ReferenceTotalMax: &maxReferences,
		},
	}
	require.NoError(t, ValidateVideoRouteTargetContract(channel, modelrouting.MiniMaxH3, base))

	base.CostVariantKey = "2k"
	err := ValidateVideoRouteTargetContract(channel, modelrouting.MiniMaxH3, base)
	require.Error(t, err)
	var contractErr *VideoRouteContractError
	require.ErrorAs(t, err, &contractErr)
	assert.Equal(t, "route_contract_cost_variant", contractErr.Code)
}
