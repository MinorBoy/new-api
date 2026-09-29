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

// Each channel maps the canonical MiniMax-H3 model onto its own provider model
// ID, so the route contract must not require one fixed upstream spelling.
func TestMiniMaxH3RouteAcceptsProviderUpstreamModelIDs(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeNewAPIVideo}
	min, max := 4, 15
	maxReferences := 15
	target := func(upstreamModel, variant string, resolutions []string) modelrouting.Target {
		return modelrouting.Target{
			UpstreamModel:  upstreamModel,
			CostVariantKey: variant,
			Constraints: modelrouting.Constraints{
				OutputResolutions: resolutions,
				Durations:         modelrouting.DurationConstraint{Min: &min, Max: &max},
				AspectRatios:      []string{"auto", "16:9"},
				ReferenceLimits:   modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
				ReferenceTotalMax: &maxReferences,
			},
		}
	}

	for _, test := range []struct {
		name          string
		upstreamModel string
		variant       string
		resolutions   []string
	}{
		{name: "legacy vip alias", upstreamModel: modelrouting.MiniMaxH3VIP, variant: "720p", resolutions: []string{"720p"}},
		{name: "vendor 768p model", upstreamModel: "vendor-h3-768p", variant: "768p", resolutions: []string{"768p"}},
		{name: "vendor 2k model", upstreamModel: "lec-h3video-2k", variant: "2k", resolutions: []string{"2k"}},
		{name: "arbitrary vendor id", upstreamModel: "minimax_h3_image_audio_to_video_v2", variant: "2k", resolutions: []string{"2k"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, ValidateVideoRouteTargetContract(channel, modelrouting.MiniMaxH3, target(test.upstreamModel, test.variant, test.resolutions)))
		})
	}

	// One route target carries exactly one cost variant, so a mixed-resolution
	// target must be split into per-resolution targets instead.
	err := ValidateVideoRouteTargetContract(channel, modelrouting.MiniMaxH3, target("vendor-h3", "2k", []string{"720p", "768p", "2k"}))
	require.Error(t, err)
	var contractErr *VideoRouteContractError
	require.ErrorAs(t, err, &contractErr)
	assert.Equal(t, "route_contract_cost_variant", contractErr.Code)
}

func TestMiniMaxH3RouteRejectsCrossFamilyAndInvalidConstraints(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeNewAPIVideo}
	min, max := 4, 15
	maxReferences := 15
	valid := func() modelrouting.Target {
		return modelrouting.Target{
			UpstreamModel:  "vendor-h3-2k",
			CostVariantKey: "2k",
			Constraints: modelrouting.Constraints{
				OutputResolutions: []string{"2k"},
				Durations:         modelrouting.DurationConstraint{Min: &min, Max: &max},
				AspectRatios:      []string{"16:9"},
				ReferenceLimits:   modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
				ReferenceTotalMax: &maxReferences,
			},
		}
	}
	require.NoError(t, ValidateVideoRouteTargetContract(channel, modelrouting.MiniMaxH3, valid()))

	for _, test := range []struct {
		name   string
		mutate func(*modelrouting.Target)
		code   string
	}{
		{name: "Seedance upstream model", mutate: func(t *modelrouting.Target) { t.UpstreamModel = modelrouting.Seedance20 }, code: "route_contract_model"},
		{name: "empty upstream model", mutate: func(t *modelrouting.Target) { t.UpstreamModel = "  " }, code: "route_contract_model"},
		{name: "unsupported resolution", mutate: func(t *modelrouting.Target) {
			t.Constraints.OutputResolutions = []string{"4k"}
		}, code: "route_contract_resolution"},
		{name: "duration above maximum", mutate: func(t *modelrouting.Target) {
			upper := 16
			t.Constraints.Durations = modelrouting.DurationConstraint{Min: &min, Max: &upper}
			t.CostVariantKey = "2k"
		}, code: "route_contract_duration"},
		{name: "reference overflow", mutate: func(t *modelrouting.Target) {
			t.Constraints.ReferenceLimits.Videos = 4
		}, code: "route_contract_references"},
		{name: "unknown cost variant", mutate: func(t *modelrouting.Target) { t.CostVariantKey = "1080p" }, code: "route_contract_cost_variant"},
	} {
		t.Run(test.name, func(t *testing.T) {
			target := valid()
			test.mutate(&target)
			err := ValidateVideoRouteTargetContract(channel, modelrouting.MiniMaxH3, target)
			require.Error(t, err)
			var contractErr *VideoRouteContractError
			require.ErrorAs(t, err, &contractErr)
			assert.Equal(t, test.code, contractErr.Code)
		})
	}

	// The provider ID is not the client identity: an upstream-only H3 name may
	// still be validated, but a non-H3 canonical model must not use H3 rules.
	err := ValidateVideoRouteTargetContract(channel, modelrouting.Seedance20, valid())
	require.NoError(t, err)
}
