package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGroupRoutingTargetKeyIgnoresDatabaseIDs(t *testing.T) {
	left := modelrouting.Target{ID: 10, ChannelID: 23, Name: "stable", UpstreamModel: "vendor-model", CostVariantKey: "default"}
	right := left
	right.ID = 99
	assert.Equal(t,
		GroupRoutingTargetKey("default", modelrouting.Seedance20, left),
		GroupRoutingTargetKey("default", modelrouting.Seedance20, right),
	)
}

func TestGroupRoutingTargetKeyChangesWithBusinessIdentity(t *testing.T) {
	target := modelrouting.Target{ID: 10, ChannelID: 23, Name: "stable", UpstreamModel: "vendor-model", CostVariantKey: "default"}
	key := GroupRoutingTargetKey(" default ", " "+modelrouting.Seedance20+" ", target)

	changed := target
	changed.UpstreamModel = "vendor-model-v2"
	assert.NotEqual(t, key, GroupRoutingTargetKey("default", modelrouting.Seedance20, changed))
	assert.Equal(t, key, GroupRoutingTargetKey("default", modelrouting.Seedance20, target))
}

func TestEvaluateGroupRoutingProfileSelectsLowerPriorityCompatibleTarget(t *testing.T) {
	profile := ratio_setting.GroupRoutingRequirements{
		Status:           ratio_setting.GroupRoutingProfileActive,
		RoutingSource:    "default",
		AllowedCostModes: []types.CostMode{types.CostModePerDuration},
	}
	snapshot := profileTestSnapshot(
		profileTarget(11, 23, 100, "per-request-target", "request-model"),
		profileTarget(12, 23, 50, "per-duration-target", "duration-model"),
	)
	rules := profileTestRules(
		profileRule(23, "request-model", types.CostModePerRequest),
		profileRule(23, "duration-model", types.CostModePerDuration),
	)
	available := map[GroupRoutingAvailabilityKey]struct{}{
		{CanonicalModel: snapshot.CanonicalModel, ChannelID: 23}: {},
	}

	result := EvaluateGroupRoutingProfile(profile, snapshot, rules, available)
	require.Contains(t, result.Snapshot.TargetsByChannel, 23)
	require.Len(t, result.Snapshot.TargetsByChannel[23], 1)
	assert.Equal(t, "per-duration-target", result.Snapshot.TargetsByChannel[23][0].Name)
	assert.Equal(t, 1, result.MismatchCounts[GroupRoutingTargetCostModeMismatch])
	require.Len(t, result.Targets, 2)
}

func TestEvaluateGroupRoutingProfileReportsStaticTargetStatuses(t *testing.T) {
	trueValue := true
	falseValue := false
	matchingRule := profileRule(23, "vendor-model", types.CostModePerRequest)
	tests := []struct {
		name          string
		profile       ratio_setting.GroupRoutingRequirements
		target        modelrouting.Target
		rules         map[CostRuleCandidate]*model.ChannelModelCostRule
		available     bool
		exclude       bool
		expected      GroupRoutingTargetStatus
		eligible      bool
		expectedIssue GroupRoutingTargetStatus
	}{
		{
			name: "matched", profile: profileRequirements(), target: profileTarget(11, 23, 100, "target", "vendor-model"),
			rules: profileTestRules(matchingRule), available: true, expected: GroupRoutingTargetMatched, eligible: true,
		},
		{
			name: "disabled is primary", profile: profileRequirements(), target: func() modelrouting.Target {
				target := profileTarget(11, 23, 100, "target", "vendor-model")
				target.Enabled = false
				return target
			}(), rules: profileTestRules(matchingRule), exclude: true, expected: GroupRoutingTargetDisabled,
		},
		{
			name: "channel unavailable", profile: profileRequirements(), target: profileTarget(11, 23, 100, "target", "vendor-model"),
			rules: profileTestRules(matchingRule), expected: GroupRoutingTargetChannelUnavailable,
		},
		{
			name: "manual exclusion", profile: profileRequirements(), target: profileTarget(11, 23, 100, "target", "vendor-model"),
			rules: profileTestRules(matchingRule), available: true, exclude: true, expected: GroupRoutingTargetExcluded,
		},
		{
			name: "required real person mismatch", profile: profileRequirementsWithRealPerson(ratio_setting.GroupRealPersonRequired),
			target: profileTargetWithRealPerson(11, 23, "vendor-model", &falseValue), rules: profileTestRules(matchingRule), available: true,
			expected: GroupRoutingTargetRealPersonMismatch,
		},
		{
			name: "required real person unknown", profile: profileRequirementsWithRealPerson(ratio_setting.GroupRealPersonRequired),
			target: profileTargetWithRealPerson(11, 23, "vendor-model", nil), rules: profileTestRules(matchingRule), available: true,
			expected: GroupRoutingTargetRealPersonUnknown,
		},
		{
			name: "forbidden real person mismatch", profile: profileRequirementsWithRealPerson(ratio_setting.GroupRealPersonForbidden),
			target: profileTargetWithRealPerson(11, 23, "vendor-model", &trueValue), rules: profileTestRules(matchingRule), available: true,
			expected: GroupRoutingTargetRealPersonMismatch,
		},
		{
			name: "cost mode mismatch", profile: profileRequirementsWithCostModes(types.CostModePerDuration),
			target: profileTarget(11, 23, 100, "target", "vendor-model"), rules: profileTestRules(matchingRule), available: true,
			expected: GroupRoutingTargetCostModeMismatch,
		},
		{
			name: "cost rule missing with restriction", profile: profileRequirementsWithCostModes(types.CostModePerDuration),
			target: profileTarget(11, 23, 100, "target", "vendor-model"), available: true,
			expected: GroupRoutingTargetCostRuleMissing,
		},
		{
			name: "cost rule missing without restriction is warning", profile: profileRequirements(),
			target: profileTarget(11, 23, 100, "target", "vendor-model"), available: true,
			expected: GroupRoutingTargetMatched, eligible: true, expectedIssue: GroupRoutingTargetCostRuleMissing,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := profileTestSnapshot(test.target)
			available := map[GroupRoutingAvailabilityKey]struct{}{}
			if test.available {
				available[GroupRoutingAvailabilityKey{CanonicalModel: snapshot.CanonicalModel, ChannelID: test.target.ChannelID}] = struct{}{}
			}
			if test.exclude {
				test.profile.ExcludedTargetKeys = []string{GroupRoutingTargetKey(test.profile.RoutingSource, snapshot.CanonicalModel, test.target)}
			}

			result := EvaluateGroupRoutingProfile(test.profile, snapshot, test.rules, available)
			require.Len(t, result.Targets, 1)
			evaluation := result.Targets[0]
			assert.Equal(t, test.expected, evaluation.Status)
			assert.Equal(t, test.eligible, evaluation.Eligible)
			if test.expectedIssue != "" {
				assert.Contains(t, evaluation.Issues, test.expectedIssue)
			}
			if test.eligible {
				assert.Contains(t, result.Snapshot.TargetsByChannel, test.target.ChannelID)
				assert.Empty(t, result.MismatchCounts)
			} else {
				assert.Contains(t, evaluation.Issues, test.expected)
				assert.NotContains(t, result.Snapshot.TargetsByChannel, test.target.ChannelID)
				assert.Equal(t, 1, result.MismatchCounts[test.expected])
			}
		})
	}
}

func TestEvaluateGroupRoutingProfilePreservesExactIssueOrder(t *testing.T) {
	profile := profileRequirementsWithCostModes(types.CostModePerDuration)
	profile.RealPersonMode = ratio_setting.GroupRealPersonRequired
	target := profileTargetWithRealPerson(11, 23, "vendor-model", nil)
	target.Enabled = false
	profile.ExcludedTargetKeys = []string{GroupRoutingTargetKey(profile.RoutingSource, modelrouting.Seedance20, target)}

	result := EvaluateGroupRoutingProfile(profile, profileTestSnapshot(target), nil, nil)
	require.Len(t, result.Targets, 1)
	assert.Equal(t, GroupRoutingTargetDisabled, result.Targets[0].Status)
	assert.Equal(t, []GroupRoutingTargetStatus{
		GroupRoutingTargetDisabled,
		GroupRoutingTargetChannelUnavailable,
		GroupRoutingTargetRealPersonUnknown,
		GroupRoutingTargetExcluded,
		GroupRoutingTargetCostRuleMissing,
	}, result.Targets[0].Issues)
}

func TestEvaluateGroupRoutingProfileTreatsInvalidCostVariantAsMissingRuleWarning(t *testing.T) {
	target := profileTarget(11, 23, 100, "invalid-variant", "vendor-model")
	target.CostVariantKey = "invalid variant!"
	snapshot := profileTestSnapshot(target)
	available := map[GroupRoutingAvailabilityKey]struct{}{
		{CanonicalModel: snapshot.CanonicalModel, ChannelID: target.ChannelID}: {},
	}

	result := EvaluateGroupRoutingProfile(profileRequirements(), snapshot, nil, available)
	require.Len(t, result.Targets, 1)
	assert.True(t, result.Targets[0].Eligible)
	assert.Equal(t, GroupRoutingTargetMatched, result.Targets[0].Status)
	assert.Equal(t, []GroupRoutingTargetStatus{GroupRoutingTargetCostRuleMissing}, result.Targets[0].Issues)
}

func TestEvaluateGroupRoutingProfileHandlesNilAndEmptyInputs(t *testing.T) {
	tests := []struct {
		name         string
		snapshot     modelrouting.PolicySnapshot
		rules        map[CostRuleCandidate]*model.ChannelModelCostRule
		availability map[GroupRoutingAvailabilityKey]struct{}
	}{
		{name: "nil"},
		{
			name:         "empty",
			snapshot:     modelrouting.PolicySnapshot{TargetsByChannel: map[int][]modelrouting.Target{}},
			rules:        map[CostRuleCandidate]*model.ChannelModelCostRule{},
			availability: map[GroupRoutingAvailabilityKey]struct{}{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expectedSnapshot := test.snapshot
			expectedSnapshot.TargetsByChannel = map[int][]modelrouting.Target{}
			expected := GroupRoutingProfileEvaluation{
				Snapshot:       expectedSnapshot,
				Targets:        []GroupRoutingTargetEvaluation{},
				MismatchCounts: map[GroupRoutingTargetStatus]int{},
				CostRules:      test.rules,
			}

			result := EvaluateGroupRoutingProfile(ratio_setting.GroupRoutingRequirements{}, test.snapshot, test.rules, test.availability)
			assert.Equal(t, expected, result)
		})
	}
}

func TestResolveGroupRoutingProfilePoliciesBatchesAcrossPoliciesAndTargets(t *testing.T) {
	prepareCostRuleServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Ability{}))
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		model.DB.Where("channel_id = ?", 7).Delete(&model.Ability{})
	})
	priority := int64(100)
	require.NoError(t, model.DB.Create(&[]model.Ability{
		{Group: "default", Model: modelrouting.Seedance20, ChannelId: 7, Enabled: true, Priority: &priority, Weight: 10},
		{Group: "default", Model: modelrouting.Seedance20Fast, ChannelId: 7, Enabled: true, Priority: &priority, Weight: 10},
	}).Error)
	now := common.GetTimestamp()
	for _, upstreamModel := range []string{"duration-a", "duration-b", "duration-fast-a", "duration-fast-b"} {
		seedActiveCostRuleRow(t, 7, upstreamModel, types.CostModePerDuration, 1, &now)
	}
	policies := []model.RoutingPolicy{
		{ID: 11, GroupName: "default", Model: modelrouting.Seedance20, Enabled: true, DefaultResolution: "720p", DefaultDuration: 10, DefaultRatio: "16:9"},
		{ID: 12, GroupName: "default", Model: modelrouting.Seedance20Fast, Enabled: true, DefaultResolution: "720p", DefaultDuration: 10, DefaultRatio: "16:9"},
	}
	targetsByPolicy := [][]modelrouting.Target{
		{
			profileTarget(21, 7, 100, "duration-a", "duration-a"),
			profileTarget(22, 7, 50, "duration-b", "duration-b"),
		},
		{
			profileTarget(23, 7, 100, "duration-fast-a", "duration-fast-a"),
			profileTarget(24, 7, 50, "duration-fast-b", "duration-fast-b"),
		},
	}
	for policyIndex := range policies {
		for _, target := range targetsByPolicy[policyIndex] {
			constraints, err := common.Marshal(target.Constraints)
			require.NoError(t, err)
			policies[policyIndex].Targets = append(policies[policyIndex].Targets, model.RouteTarget{
				ID: target.ID, PolicyID: policies[policyIndex].ID, ChannelID: target.ChannelID, Name: target.Name,
				UpstreamModel: target.UpstreamModel, CostVariantKey: target.CostVariantKey,
				TargetPriority: target.Priority, Enabled: true, Constraints: string(constraints),
			})
		}
	}
	InvalidateCostCoverage(0, "", "")
	queryCount := 0
	const callbackName = "test:count-group-routing-profile-resolver-queries"
	callbackRegistered := true
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(callbackName, func(*gorm.DB) {
		queryCount++
	}))
	t.Cleanup(func() {
		if callbackRegistered {
			_ = model.DB.Callback().Query().Remove(callbackName)
		}
	})

	results, err := ResolveGroupRoutingProfilePolicies(profileRequirementsWithCostModes(types.CostModePerDuration), policies)
	require.NoError(t, model.DB.Callback().Query().Remove(callbackName))
	callbackRegistered = false
	require.NoError(t, err)
	assert.Equal(t, 3, queryCount)
	require.Len(t, results, 2)
	for _, result := range results {
		require.Len(t, result.Targets, 2)
		assert.True(t, result.Targets[0].Eligible)
		assert.True(t, result.Targets[1].Eligible)
		assert.Len(t, result.CostRules, 4)
	}
}

type profileRuleFixture struct {
	candidate CostRuleCandidate
	rule      *model.ChannelModelCostRule
}

func profileTestSnapshot(targets ...modelrouting.Target) modelrouting.PolicySnapshot {
	snapshot := modelrouting.PolicySnapshot{
		ID: 1, GroupName: "default", CanonicalModel: modelrouting.Seedance20, Enabled: true,
		Defaults:         modelrouting.Defaults{OutputResolution: "720p", DurationSeconds: 10, AspectRatio: "16:9"},
		TargetsByChannel: make(map[int][]modelrouting.Target),
	}
	for _, target := range targets {
		snapshot.TargetsByChannel[target.ChannelID] = append(snapshot.TargetsByChannel[target.ChannelID], target)
	}
	return snapshot
}

func profileTarget(id, channelID, priority int, name, upstreamModel string) modelrouting.Target {
	supportsRealPerson := true
	return modelrouting.Target{
		ID: id, PolicyID: 1, ChannelID: channelID, Name: name, UpstreamModel: upstreamModel,
		CostVariantKey: string(types.DefaultCostVariantKey), Priority: priority, Enabled: true,
		Constraints: modelrouting.Constraints{
			OutputResolutions: []string{"720p"}, Durations: modelrouting.DurationConstraint{Min: profileIntPointer(4), Max: profileIntPointer(15)},
			AspectRatios: []string{"16:9"}, ReferenceLimits: modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
			SupportsRealPerson: &supportsRealPerson,
		},
	}
}

func profileTargetWithRealPerson(id, channelID int, upstreamModel string, supports *bool) modelrouting.Target {
	target := profileTarget(id, channelID, 100, "target", upstreamModel)
	target.Constraints.SupportsRealPerson = supports
	return target
}

func profileRule(channelID int, upstreamModel string, mode types.CostMode) profileRuleFixture {
	return profileRuleFixture{
		candidate: CostRuleCandidate{ChannelID: channelID, BillableUpstreamModel: upstreamModel, CostVariantKey: string(types.DefaultCostVariantKey)},
		rule: &model.ChannelModelCostRule{
			ID: int64(channelID), ChannelID: channelID, BillableUpstreamModel: upstreamModel,
			CostVariantKey: string(types.DefaultCostVariantKey), Version: 1, CostMode: string(mode),
		},
	}
}

func profileTestRules(fixtures ...profileRuleFixture) map[CostRuleCandidate]*model.ChannelModelCostRule {
	rules := make(map[CostRuleCandidate]*model.ChannelModelCostRule, len(fixtures))
	for _, fixture := range fixtures {
		rules[fixture.candidate] = fixture.rule
	}
	return rules
}

func profileRequirements() ratio_setting.GroupRoutingRequirements {
	return ratio_setting.GroupRoutingRequirements{Status: ratio_setting.GroupRoutingProfileActive, RoutingSource: "default"}
}

func profileRequirementsWithRealPerson(mode ratio_setting.GroupRealPersonMode) ratio_setting.GroupRoutingRequirements {
	profile := profileRequirements()
	profile.RealPersonMode = mode
	return profile
}

func profileRequirementsWithCostModes(modes ...types.CostMode) ratio_setting.GroupRoutingRequirements {
	profile := profileRequirements()
	profile.AllowedCostModes = modes
	return profile
}

func profileIntPointer(value int) *int {
	return &value
}
