package modelrouting_test

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/modelrouting"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePolicyRejectsAmbiguousSamePriorityTargets(t *testing.T) {
	policy := validPolicySnapshot()
	policy.TargetsByChannel[11] = []modelrouting.Target{
		validTarget(21, 11, 50, []string{"720p"}, modelrouting.DurationConstraint{Min: intPtr(4), Max: intPtr(15)}, []string{"16:9"}),
		validTarget(22, 11, 50, []string{"720p", "1080p"}, modelrouting.DurationConstraint{Values: []int{5, 10, 15}}, []string{"16:9", "9:16"}),
	}

	err := modelrouting.ValidatePolicy(policy, relaycommon.MaxTaskDurationSeconds)
	require.Error(t, err)
	var validationErr *modelrouting.ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, modelrouting.ValidationTargetOverlap, validationErr.Code)
	assert.Equal(t, []int{21, 22}, validationErr.TargetIDs)
}

func TestValidatePolicyAcceptsDisjointSamePriorityTargets(t *testing.T) {
	policy := validPolicySnapshot()
	policy.TargetsByChannel[11] = []modelrouting.Target{
		validTarget(21, 11, 50, []string{"720p"}, modelrouting.DurationConstraint{Min: intPtr(4), Max: intPtr(9)}, nil),
		validTarget(22, 11, 50, []string{"720p"}, modelrouting.DurationConstraint{Min: intPtr(10), Max: intPtr(15)}, nil),
	}

	require.NoError(t, modelrouting.ValidatePolicy(policy, relaycommon.MaxTaskDurationSeconds))
}

func TestValidatePolicyMinimumExpectedMarginBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		value   *int
		wantErr bool
	}{
		{name: "inherit", value: nil},
		{name: "explicit zero", value: intPtr(0)},
		{name: "maximum", value: intPtr(10_000)},
		{name: "negative", value: intPtr(-1), wantErr: true},
		{name: "over maximum", value: intPtr(10_001), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := validPolicySnapshot()
			policy.TargetsByChannel[11][0].MinimumExpectedMarginBPS = tt.value

			err := modelrouting.ValidatePolicy(policy, relaycommon.MaxTaskDurationSeconds)
			if !tt.wantErr {
				require.NoError(t, err)
				assert.Equal(t, tt.value, policy.TargetsByChannel[11][0].MinimumExpectedMarginBPS)
				return
			}
			var validationErr *modelrouting.ValidationError
			require.ErrorAs(t, err, &validationErr)
			assert.Equal(t, modelrouting.ValidationInvalidMinimumExpectedMargin, validationErr.Code)
		})
	}
}

func TestValidatePolicyRejectsInvalidContracts(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*modelrouting.PolicySnapshot)
		expected modelrouting.ValidationCode
	}{
		{
			name: "unknown canonical model",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.CanonicalModel = "seedance-custom"
			},
			expected: modelrouting.ValidationInvalidModel,
		},
		{
			name: "empty group",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.GroupName = " "
			},
			expected: modelrouting.ValidationInvalidGroup,
		},
		{
			name: "auto group",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.GroupName = "auto"
			},
			expected: modelrouting.ValidationInvalidGroup,
		},
		{
			name: "empty output resolutions",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.TargetsByChannel[11][0].Constraints.OutputResolutions = nil
			},
			expected: modelrouting.ValidationInvalidOutputResolution,
		},
		{
			name: "unsupported output resolution",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.TargetsByChannel[11][0].Constraints.OutputResolutions = []string{"1440p"}
			},
			expected: modelrouting.ValidationInvalidOutputResolution,
		},
		{
			name: "unsupported default resolution",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.Defaults.OutputResolution = "1440p"
			},
			expected: modelrouting.ValidationInvalidOutputResolution,
		},
		{
			name: "duration values and range",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.TargetsByChannel[11][0].Constraints.Durations = modelrouting.DurationConstraint{
					Values: []int{5}, Min: intPtr(4), Max: intPtr(15),
				}
			},
			expected: modelrouting.ValidationInvalidDuration,
		},
		{
			name: "zero duration",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.TargetsByChannel[11][0].Constraints.Durations = modelrouting.DurationConstraint{Values: []int{0}}
			},
			expected: modelrouting.ValidationInvalidDuration,
		},
		{
			name: "duration over global maximum",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.TargetsByChannel[11][0].Constraints.Durations = modelrouting.DurationConstraint{Values: []int{relaycommon.MaxTaskDurationSeconds + 1}}
			},
			expected: modelrouting.ValidationInvalidDuration,
		},
		{
			name: "invalid default duration",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.Defaults.DurationSeconds = 0
			},
			expected: modelrouting.ValidationInvalidDuration,
		},
		{
			name: "unsupported aspect ratio",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.TargetsByChannel[11][0].Constraints.AspectRatios = []string{"2:1"}
			},
			expected: modelrouting.ValidationInvalidAspectRatio,
		},
		{
			name: "unsupported default aspect ratio",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.Defaults.AspectRatio = "2:1"
			},
			expected: modelrouting.ValidationInvalidAspectRatio,
		},
		{
			name: "too many images",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.TargetsByChannel[11][0].Constraints.ReferenceLimits.Images = 10
			},
			expected: modelrouting.ValidationInvalidReferenceLimit,
		},
		{
			name: "too many videos",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.TargetsByChannel[11][0].Constraints.ReferenceLimits.Videos = 4
			},
			expected: modelrouting.ValidationInvalidReferenceLimit,
		},
		{
			name: "too many audios",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.TargetsByChannel[11][0].Constraints.ReferenceLimits.Audios = 4
			},
			expected: modelrouting.ValidationInvalidReferenceLimit,
		},
		{
			name: "negative reference limit",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.TargetsByChannel[11][0].Constraints.ReferenceLimits.Audios = -1
			},
			expected: modelrouting.ValidationInvalidReferenceLimit,
		},
		{
			name: "unknown input mode",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.TargetsByChannel[11][0].Constraints.InputModes = []modelrouting.InputMode{"unknown"}
			},
			expected: modelrouting.ValidationInvalidInputMode,
		},
		{
			name: "negative reference minimum",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.TargetsByChannel[11][0].Constraints.ReferenceMinimums.Images = -1
			},
			expected: modelrouting.ValidationInvalidReferenceLimit,
		},
		{
			name: "reference minimum exceeds maximum",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.TargetsByChannel[11][0].Constraints.ReferenceMinimums.Images = 3
				policy.TargetsByChannel[11][0].Constraints.ReferenceLimits.Images = 2
			},
			expected: modelrouting.ValidationInvalidReferenceLimit,
		},
		{
			name: "reference total exceeds individual maxima",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.TargetsByChannel[11][0].Constraints.ReferenceTotalMax = intPtr(16)
			},
			expected: modelrouting.ValidationInvalidReferenceLimit,
		},
		{
			name: "video audio total exceeds individual maxima",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.TargetsByChannel[11][0].Constraints.ReferenceVideoAudioTotalMax = intPtr(7)
			},
			expected: modelrouting.ValidationInvalidReferenceLimit,
		},
		{
			name: "enabled policy defaults are unsupported",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.Defaults.OutputResolution = "1080p"
			},
			expected: modelrouting.ValidationDefaultRouteUnavailable,
		},
		{
			name: "enabled policy has no enabled target",
			mutate: func(policy *modelrouting.PolicySnapshot) {
				policy.TargetsByChannel[11][0].Enabled = false
			},
			expected: modelrouting.ValidationDefaultRouteUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := validPolicySnapshot()
			tt.mutate(&policy)
			err := modelrouting.ValidatePolicy(policy, relaycommon.MaxTaskDurationSeconds)
			require.Error(t, err)
			var validationErr *modelrouting.ValidationError
			require.ErrorAs(t, err, &validationErr)
			assert.Equal(t, tt.expected, validationErr.Code)
		})
	}
}

func TestValidatePolicyValidatesDisabledTargets(t *testing.T) {
	policy := validPolicySnapshot()
	policy.Enabled = false
	target := &policy.TargetsByChannel[11][0]
	target.Enabled = false
	target.Constraints.OutputResolutions = []string{"1440p"}

	err := modelrouting.ValidatePolicy(policy, relaycommon.MaxTaskDurationSeconds)
	require.Error(t, err)
	var validationErr *modelrouting.ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, modelrouting.ValidationInvalidOutputResolution, validationErr.Code)
}

func TestValidatePolicyReportsSyntheticIndexesForNewOverlappingTargets(t *testing.T) {
	policy := validPolicySnapshot()
	policy.TargetsByChannel[11] = []modelrouting.Target{
		validTarget(0, 11, 50, []string{"720p"}, modelrouting.DurationConstraint{Values: []int{10}}, nil),
		validTarget(0, 11, 50, []string{"720p"}, modelrouting.DurationConstraint{Min: intPtr(4), Max: intPtr(15)}, nil),
	}

	err := modelrouting.ValidatePolicy(policy, relaycommon.MaxTaskDurationSeconds)
	var validationErr *modelrouting.ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, []int{-1, -2}, validationErr.TargetIDs)
}

func TestValidatePolicyTreatsReferenceAndRealPersonCapabilitiesAsOverlapping(t *testing.T) {
	supported := true
	unsupported := false
	policy := validPolicySnapshot()
	first := validTarget(21, 11, 50, []string{"720p"}, modelrouting.DurationConstraint{Values: []int{10}}, nil)
	first.Constraints.ReferenceLimits = modelrouting.ReferenceLimits{}
	first.Constraints.SupportsRealPerson = &unsupported
	second := validTarget(22, 11, 50, []string{"720p"}, modelrouting.DurationConstraint{Values: []int{10}}, nil)
	second.Constraints.ReferenceLimits = modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}
	second.Constraints.SupportsRealPerson = &supported
	policy.TargetsByChannel[11] = []modelrouting.Target{first, second}

	err := modelrouting.ValidatePolicy(policy, relaycommon.MaxTaskDurationSeconds)
	var validationErr *modelrouting.ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, modelrouting.ValidationTargetOverlap, validationErr.Code)
}

func TestValidatePolicyInputModesAndReferenceRangesControlOverlap(t *testing.T) {
	t.Run("disjoint input modes", func(t *testing.T) {
		policy := validPolicySnapshot()
		text := validTarget(21, 11, 50, []string{"720p"}, modelrouting.DurationConstraint{Values: []int{10}}, nil)
		text.Constraints.InputModes = []modelrouting.InputMode{modelrouting.InputModeText}
		text.Constraints.ReferenceLimits = modelrouting.ReferenceLimits{}
		frames := validTarget(22, 11, 50, []string{"720p"}, modelrouting.DurationConstraint{Values: []int{10}}, nil)
		frames.Constraints.InputModes = []modelrouting.InputMode{modelrouting.InputModeFirstLastFrames}
		frames.Constraints.ReferenceMinimums.Images = 2
		frames.Constraints.ReferenceLimits = modelrouting.ReferenceLimits{Images: 2}
		policy.TargetsByChannel[11] = []modelrouting.Target{text, frames}

		require.NoError(t, modelrouting.ValidatePolicy(policy, relaycommon.MaxTaskDurationSeconds))
	})

	t.Run("disjoint reference ranges", func(t *testing.T) {
		policy := validPolicySnapshot()
		withImage := validTarget(21, 11, 50, []string{"720p"}, modelrouting.DurationConstraint{Values: []int{10}}, nil)
		withImage.Constraints.ReferenceMinimums.Images = 1
		withoutImage := validTarget(22, 11, 50, []string{"720p"}, modelrouting.DurationConstraint{Values: []int{10}}, nil)
		withoutImage.Constraints.ReferenceLimits = modelrouting.ReferenceLimits{}
		policy.TargetsByChannel[11] = []modelrouting.Target{withImage, withoutImage}

		require.NoError(t, modelrouting.ValidatePolicy(policy, relaycommon.MaxTaskDurationSeconds))
	})
}

func TestValidatePolicyUsesInputModeReferenceMinimumRepresentatives(t *testing.T) {
	discount := validPolicySnapshot()
	target := &discount.TargetsByChannel[11][0]
	target.Constraints.InputModes = []modelrouting.InputMode{
		modelrouting.InputModeFirstFrame,
		modelrouting.InputModeOmniReference,
	}
	target.Constraints.ReferenceMinimums.Images = 1
	discount.Defaults.DurationSeconds = 8

	require.NoError(t, modelrouting.ValidatePolicy(discount, relaycommon.MaxTaskDurationSeconds))

	unreachable := validPolicySnapshot()
	target = &unreachable.TargetsByChannel[11][0]
	target.Constraints.InputModes = []modelrouting.InputMode{modelrouting.InputModeFirstLastFrames}
	target.Constraints.ReferenceLimits.Images = 1
	err := modelrouting.ValidatePolicy(unreachable, relaycommon.MaxTaskDurationSeconds)
	var validationErr *modelrouting.ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, modelrouting.ValidationInvalidReferenceLimit, validationErr.Code)
	assert.Equal(t, "targets.constraints.reference_minimums", validationErr.Field)
}

func validPolicySnapshot() modelrouting.PolicySnapshot {
	return modelrouting.PolicySnapshot{
		ID:             7,
		GroupName:      "分组A",
		CanonicalModel: modelrouting.Seedance20,
		Enabled:        true,
		Defaults: modelrouting.Defaults{
			OutputResolution: "720p",
			DurationSeconds:  10,
			AspectRatio:      "16:9",
		},
		TargetsByChannel: map[int][]modelrouting.Target{
			11: {validTarget(21, 11, 50, []string{"720p"}, modelrouting.DurationConstraint{Min: intPtr(4), Max: intPtr(15)}, nil)},
		},
	}
}

func validTarget(id, channelID, priority int, resolutions []string, durations modelrouting.DurationConstraint, ratios []string) modelrouting.Target {
	return modelrouting.Target{
		ID:            id,
		PolicyID:      7,
		ChannelID:     channelID,
		Name:          "target",
		UpstreamModel: "provider-model",
		Priority:      priority,
		Enabled:       true,
		Constraints: modelrouting.Constraints{
			OutputResolutions: resolutions,
			Durations:         durations,
			AspectRatios:      ratios,
			ReferenceLimits:   modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
		},
	}
}
