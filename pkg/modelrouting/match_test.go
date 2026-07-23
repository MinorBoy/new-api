package modelrouting_test

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveFactsPrefersExplicitValues(t *testing.T) {
	resolution := " 1080P "
	duration := 10
	ratio := " 16:9 "
	input := modelrouting.FactsInput{
		CanonicalModel:    modelrouting.Seedance20,
		OutputResolution:  &resolution,
		DurationSeconds:   &duration,
		AspectRatio:       &ratio,
		ReferenceImages:   9,
		ReferenceVideos:   3,
		ReferenceAudios:   3,
		RequireRealPerson: true,
	}

	facts, err := modelrouting.ResolveFacts(" 分组A ", input, modelrouting.Defaults{
		OutputResolution: "720p",
		DurationSeconds:  5,
		AspectRatio:      "9:16",
	})
	require.NoError(t, err)
	assert.Equal(t, "分组A", facts.GroupName)
	assert.Equal(t, "1080p", facts.OutputResolution)
	assert.Equal(t, 10, facts.DurationSeconds)
	assert.Equal(t, "16:9", facts.AspectRatio)
	assert.Equal(t, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, facts.References)
	assert.True(t, facts.RequireRealPerson)
}

func TestEvaluateTargetsUsesOutputResolutionForUpscale(t *testing.T) {
	supportsRealPerson := true
	snapshot := modelrouting.PolicySnapshot{
		ID:             7,
		GroupName:      "分组A",
		CanonicalModel: modelrouting.Seedance20,
		Enabled:        true,
		TargetsByChannel: map[int][]modelrouting.Target{
			11: {{
				ID:            21,
				ChannelID:     11,
				Name:          "720p generation to 1080p",
				UpstreamModel: "lec-feituo-seedance-2-0-my-upscaled-1080p",
				Priority:      50,
				Enabled:       true,
				Constraints: modelrouting.Constraints{
					OutputResolutions:    []string{"1080p"},
					GenerationResolution: "720p",
					Upscaled:             true,
					Durations:            modelrouting.DurationConstraint{Min: intPtr(4), Max: intPtr(15)},
					AspectRatios:         []string{"16:9", "9:16"},
					ReferenceLimits:      modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1},
					SupportsRealPerson:   &supportsRealPerson,
				},
			}},
		},
	}

	matching := modelrouting.Evaluate(snapshot, modelrouting.Facts{
		GroupName:         "分组A",
		CanonicalModel:    modelrouting.Seedance20,
		OutputResolution:  "1080p",
		DurationSeconds:   10,
		AspectRatio:       "16:9",
		References:        modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1},
		RequireRealPerson: true,
	})
	require.Contains(t, matching.CompatibleByChannel, 11)
	assert.Equal(t, 21, matching.CompatibleByChannel[11].ID)

	notMatching := modelrouting.Evaluate(snapshot, modelrouting.Facts{
		GroupName:        "分组A",
		CanonicalModel:   modelrouting.Seedance20,
		OutputResolution: "720p",
		DurationSeconds:  10,
		AspectRatio:      "16:9",
	})
	assert.Empty(t, notMatching.CompatibleByChannel)
	assert.Equal(t, 1, notMatching.MismatchCounts[modelrouting.MismatchResolution])
}

func TestEvaluateTargetsSupportsInclusiveAndDiscreteDurationsAndAnyRatio(t *testing.T) {
	tests := []struct {
		name        string
		durations   modelrouting.DurationConstraint
		duration    int
		ratio       string
		shouldMatch bool
	}{
		{name: "range lower bound", durations: modelrouting.DurationConstraint{Min: intPtr(4), Max: intPtr(15)}, duration: 4, ratio: "21:9", shouldMatch: true},
		{name: "range upper bound", durations: modelrouting.DurationConstraint{Min: intPtr(4), Max: intPtr(15)}, duration: 15, ratio: "adaptive", shouldMatch: true},
		{name: "outside range", durations: modelrouting.DurationConstraint{Min: intPtr(4), Max: intPtr(15)}, duration: 16, ratio: "16:9", shouldMatch: false},
		{name: "discrete match", durations: modelrouting.DurationConstraint{Values: []int{5, 10, 15}}, duration: 10, ratio: "4:3", shouldMatch: true},
		{name: "discrete miss", durations: modelrouting.DurationConstraint{Values: []int{5, 10, 15}}, duration: 12, ratio: "4:3", shouldMatch: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := targetWithLimits(1, 11, 10, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3})
			target.Constraints.Durations = tt.durations
			target.Constraints.AspectRatios = nil
			result := modelrouting.Evaluate(modelrouting.PolicySnapshot{
				TargetsByChannel: map[int][]modelrouting.Target{11: {target}},
			}, modelrouting.Facts{
				OutputResolution: "720p",
				DurationSeconds:  tt.duration,
				AspectRatio:      tt.ratio,
			})
			if tt.shouldMatch {
				assert.Contains(t, result.CompatibleByChannel, 11)
				return
			}
			assert.NotContains(t, result.CompatibleByChannel, 11)
			assert.Equal(t, 1, result.MismatchCounts[modelrouting.MismatchDuration])
		})
	}
}

func TestMaterialPresetMatching(t *testing.T) {
	tests := []struct {
		name   string
		limits modelrouting.ReferenceLimits
	}{
		{name: "933", limits: modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}},
		{name: "431", limits: modelrouting.ReferenceLimits{Images: 4, Videos: 3, Audios: 1}},
		{name: "9", limits: modelrouting.ReferenceLimits{Images: 9, Videos: 0, Audios: 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := targetWithLimits(1, 11, 10, tt.limits)
			atLimit := matchingFacts(tt.limits)
			assert.Empty(t, modelrouting.Match(target.Constraints, atLimit))

			overImages := atLimit
			overImages.References.Images++
			assert.Equal(t, []modelrouting.MismatchReason{modelrouting.MismatchReferenceImages}, modelrouting.Match(target.Constraints, overImages))

			overVideos := atLimit
			overVideos.References.Videos++
			assert.Equal(t, []modelrouting.MismatchReason{modelrouting.MismatchReferenceVideos}, modelrouting.Match(target.Constraints, overVideos))

			overAudios := atLimit
			overAudios.References.Audios++
			assert.Equal(t, []modelrouting.MismatchReason{modelrouting.MismatchReferenceAudios}, modelrouting.Match(target.Constraints, overAudios))
		})
	}
}

func TestRealPersonMatchingRequiresExplicitSupportOnlyWhenRequested(t *testing.T) {
	supported := true
	unsupported := false
	tests := []struct {
		name              string
		targetSupport     *bool
		requireRealPerson bool
		shouldMatch       bool
	}{
		{name: "unrequired matches supported", targetSupport: &supported, shouldMatch: true},
		{name: "unrequired matches unsupported", targetSupport: &unsupported, shouldMatch: true},
		{name: "unrequired matches unknown", targetSupport: nil, shouldMatch: true},
		{name: "required matches supported", targetSupport: &supported, requireRealPerson: true, shouldMatch: true},
		{name: "required rejects unsupported", targetSupport: &unsupported, requireRealPerson: true, shouldMatch: false},
		{name: "required rejects unknown", targetSupport: nil, requireRealPerson: true, shouldMatch: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := targetWithLimits(1, 11, 10, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3})
			target.Constraints.SupportsRealPerson = tt.targetSupport
			facts := matchingFacts(modelrouting.ReferenceLimits{})
			facts.RequireRealPerson = tt.requireRealPerson
			reasons := modelrouting.Match(target.Constraints, facts)
			if tt.shouldMatch {
				assert.NotContains(t, reasons, modelrouting.MismatchRealPerson)
				return
			}
			assert.Contains(t, reasons, modelrouting.MismatchRealPerson)
		})
	}
}

func TestEvaluateTargetsSelectsHighestPriorityThenLowestID(t *testing.T) {
	lowPriority := targetWithLimits(30, 11, 10, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3})
	highID := targetWithLimits(22, 11, 50, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3})
	lowID := targetWithLimits(21, 11, 50, modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3})

	result := modelrouting.Evaluate(modelrouting.PolicySnapshot{
		TargetsByChannel: map[int][]modelrouting.Target{11: {lowPriority, highID, lowID}},
	}, matchingFacts(modelrouting.ReferenceLimits{}))

	require.Contains(t, result.CompatibleByChannel, 11)
	assert.Equal(t, 21, result.CompatibleByChannel[11].ID)
}

func targetWithLimits(id, channelID, priority int, limits modelrouting.ReferenceLimits) modelrouting.Target {
	return modelrouting.Target{
		ID:        id,
		ChannelID: channelID,
		Priority:  priority,
		Enabled:   true,
		Constraints: modelrouting.Constraints{
			OutputResolutions: []string{"720p"},
			Durations:         modelrouting.DurationConstraint{Min: intPtr(4), Max: intPtr(15)},
			ReferenceLimits:   limits,
		},
	}
}

func matchingFacts(references modelrouting.ReferenceLimits) modelrouting.Facts {
	return modelrouting.Facts{
		OutputResolution: "720p",
		DurationSeconds:  10,
		AspectRatio:      "16:9",
		References:       references,
	}
}

func intPtr(value int) *int {
	return &value
}
