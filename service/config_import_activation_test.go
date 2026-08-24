package service

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type activationFixture struct {
	BatchID        int64
	BindingID      int64
	ChannelID      int
	PolicyID       int
	TargetID       int
	RetireTargetID int
	MappingItemID  int64
	CostDraftID    int64
}

type activationApplyFixture struct {
	activationFixture
	CurrentCostRuleID  int64
	PreviousCostRuleID int64
	ManualTargetID     int
}

func TestActivateConfigImportBatchAppliesConfigurationAtomically(t *testing.T) {
	fixture := createActivationApplyFixture(t)
	previousRefresh := refreshConfigImportActivation
	var refreshed ConfigImportRefreshKeys
	refreshConfigImportActivation = func(keys ConfigImportRefreshKeys) error {
		refreshed = keys
		return nil
	}
	t.Cleanup(func() { refreshConfigImportActivation = previousRefresh })

	detail, err := ActivateConfigImportBatch(context.Background(), fixture.BatchID, 42)

	require.NoError(t, err)
	require.NotNil(t, detail.ActivatedAt)
	var previousCostRule model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&previousCostRule, fixture.PreviousCostRuleID).Error)
	assert.Equal(t, string(types.CostRuleRetired), previousCostRule.Status)
	var currentCostRule model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&currentCostRule, fixture.CurrentCostRuleID).Error)
	assert.Equal(t, string(types.CostRuleActive), currentCostRule.Status)
	var option model.Option
	require.NoError(t, model.DB.Where("key = ?", "ModelPrice").First(&option).Error)
	var prices map[string]any
	require.NoError(t, common.UnmarshalJsonStr(option.Value, &prices))
	assert.Equal(t, float64(2), prices[modelrouting.Seedance20])
	assert.Equal(t, float64(9), prices["legacy-model"])
	var channel model.Channel
	require.NoError(t, model.DB.First(&channel, fixture.ChannelID).Error)
	assert.Equal(t, common.ChannelStatusEnabled, channel.Status)
	assert.Equal(t, modelrouting.Seedance20+",vendor-video", channel.Models)
	var mapping map[string]string
	require.NoError(t, common.UnmarshalJsonStr(channel.GetModelMapping(), &mapping))
	assert.Equal(t, "vendor-video", mapping[modelrouting.Seedance20])
	var current model.RouteTarget
	require.NoError(t, model.DB.First(&current, fixture.TargetID).Error)
	assert.True(t, current.Enabled)
	assert.Nil(t, current.RetiredAt)
	var previous model.RouteTarget
	require.NoError(t, model.DB.First(&previous, fixture.RetireTargetID).Error)
	assert.False(t, previous.Enabled)
	require.NotNil(t, previous.RetiredAt)
	var manual model.RouteTarget
	require.NoError(t, model.DB.First(&manual, fixture.ManualTargetID).Error)
	assert.True(t, manual.Enabled)
	assert.Nil(t, manual.RetiredAt)
	var policy model.RoutingPolicy
	require.NoError(t, model.DB.First(&policy, fixture.PolicyID).Error)
	assert.True(t, policy.Enabled)
	assert.Equal(t, "720p", policy.DefaultResolution)
	assert.Equal(t, 10, policy.DefaultDuration)
	assert.Equal(t, "16:9", policy.DefaultRatio)
	var ability model.Ability
	require.NoError(t, model.DB.Where("channel_id = ? AND model = ?", fixture.ChannelID, modelrouting.Seedance20).First(&ability).Error)
	assert.True(t, ability.Enabled)
	var batch model.ConfigImportBatch
	require.NoError(t, model.DB.First(&batch, fixture.BatchID).Error)
	require.NotNil(t, batch.ActivatedAt)
	assert.Equal(t, *batch.ActivatedAt, *previous.RetiredAt)
	assert.Contains(t, refreshed.OptionKeys, "ModelPrice")
	assert.Contains(t, refreshed.ChannelIDs, fixture.ChannelID)
	assert.Contains(t, refreshed.RoutingPolicyKeys, model.RoutingPolicyKey{GroupName: "default", Model: modelrouting.Seedance20})
	var audit model.ConfigImportActivationAudit
	require.NoError(t, model.DB.Where("batch_id = ? AND outcome = ?", fixture.BatchID, "activated").First(&audit).Error)
	assert.NotEmpty(t, audit.BeforeSHA256)
	assert.NotEmpty(t, audit.AfterSHA256)

	_, err = ActivateConfigImportBatch(context.Background(), fixture.BatchID, 42)
	require.NoError(t, err)
	var auditCount int64
	require.NoError(t, model.DB.Model(&model.ConfigImportActivationAudit{}).Where("batch_id = ?", fixture.BatchID).Count(&auditCount).Error)
	assert.Equal(t, int64(1), auditCount)
}

func TestActivateConfigImportPublishesGroupRoutingRequirementsAtomically(t *testing.T) {
	fixture := createActivationApplyFixture(t)
	require.NoError(t, model.DB.Create(&model.Option{Key: "GroupRoutingRequirements", Value: `{}`}).Error)
	require.NoError(t, persistConfigImportGroupRequirementItem(t, fixture.BatchID, "req-default", "default", true))
	persistActivationBaseline(t, fixture.BatchID)
	previousRefresh := refreshConfigImportActivation
	var refreshed ConfigImportRefreshKeys
	refreshConfigImportActivation = func(keys ConfigImportRefreshKeys) error { refreshed = keys; return nil }
	t.Cleanup(func() { refreshConfigImportActivation = previousRefresh })

	_, err := ActivateConfigImportBatch(context.Background(), fixture.BatchID, 42)
	require.NoError(t, err)
	var option model.Option
	require.NoError(t, model.DB.Where("key = ?", "GroupRoutingRequirements").First(&option).Error)
	assert.JSONEq(t, `{"default":{"require_real_person":true}}`, option.Value)
	assert.Contains(t, refreshed.OptionKeys, "GroupRoutingRequirements")
}

func TestActivateConfigImportPreservesManualTargetExclusions(t *testing.T) {
	fixture := createActivationApplyFixture(t)
	previousUsableGroups := setting.UserUsableGroups2JSONString()
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"客户A":"客户 A"}`))
	t.Cleanup(func() { require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousUsableGroups)) })
	require.NoError(t, model.DB.Create(&model.Option{Key: "GroupRoutingRequirements", Value: `{
		"客户A":{"status":"active","routing_source":"default","allowed_cost_modes":["per_request"],"excluded_target_keys":["grt_keep"]}
	}`}).Error)
	require.NoError(t, persistConfigImportGroupProfileItem(t, fixture.BatchID, "req-customer-a", "客户A", types.ConfigImportGroupRoutingValues{
		Status: "active", RoutingSource: "default", AllowedCostModes: []string{"per_request"},
	}))
	persistActivationBaseline(t, fixture.BatchID)
	previousRefresh := refreshConfigImportActivation
	refreshConfigImportActivation = func(ConfigImportRefreshKeys) error { return nil }
	t.Cleanup(func() { refreshConfigImportActivation = previousRefresh })

	_, err := ActivateConfigImportBatch(context.Background(), fixture.BatchID, 42)

	require.NoError(t, err)
	var option model.Option
	require.NoError(t, model.DB.Where("key = ?", "GroupRoutingRequirements").First(&option).Error)
	assert.JSONEq(t, `{
		"客户A":{"status":"active","routing_source":"default","allowed_cost_modes":["per_request"],"excluded_target_keys":["grt_keep"]}
	}`, option.Value)
	require.NoError(t, ValidateActiveGroupRoutingProfiles(`{
		"客户A":{"status":"active","routing_source":"default","allowed_cost_modes":["per_request"]}
	}`))
}

func TestActivateConfigImportRollsBackWhenActiveGroupProfileHasNoTargets(t *testing.T) {
	fixture := createActivationApplyFixture(t)
	previousUsableGroups := setting.UserUsableGroups2JSONString()
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"客户A":"客户 A"}`))
	t.Cleanup(func() { require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousUsableGroups)) })
	require.NoError(t, model.DB.Create(&model.Option{Key: "GroupRoutingRequirements", Value: `{}`}).Error)
	require.NoError(t, persistConfigImportGroupProfileItem(t, fixture.BatchID, "req-customer-a", "客户A", types.ConfigImportGroupRoutingValues{
		Status: "active", RoutingSource: "default", AllowedCostModes: []string{"per_token"},
	}))
	persistActivationBaseline(t, fixture.BatchID)

	_, err := ActivateConfigImportBatch(context.Background(), fixture.BatchID, 42)

	require.ErrorContains(t, err, "客户A")
	var currentRule model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&currentRule, fixture.CurrentCostRuleID).Error)
	assert.Equal(t, string(types.CostRuleDraft), currentRule.Status)
	var previousRule model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&previousRule, fixture.PreviousCostRuleID).Error)
	assert.Equal(t, string(types.CostRuleActive), previousRule.Status)
	var candidate model.RouteTarget
	require.NoError(t, model.DB.First(&candidate, fixture.TargetID).Error)
	assert.False(t, candidate.Enabled)
	var previousTarget model.RouteTarget
	require.NoError(t, model.DB.First(&previousTarget, fixture.RetireTargetID).Error)
	assert.True(t, previousTarget.Enabled)
	var option model.Option
	require.NoError(t, model.DB.Where("key = ?", "GroupRoutingRequirements").First(&option).Error)
	assert.JSONEq(t, `{}`, option.Value)
	var auditCount int64
	require.NoError(t, model.DB.Model(&model.ConfigImportActivationAudit{}).Where("batch_id = ?", fixture.BatchID).Count(&auditCount).Error)
	assert.Zero(t, auditCount)
}

func TestActivateConfigImportDoesNotPartiallyWriteGroupRequirements(t *testing.T) {
	fixture := createActivationApplyFixture(t)
	require.NoError(t, model.DB.Create(&model.Option{Key: "GroupRoutingRequirements", Value: `{}`}).Error)
	require.NoError(t, persistConfigImportGroupRequirementItem(t, fixture.BatchID, "req-default", "default", true))
	persistActivationBaseline(t, fixture.BatchID)
	callbackName := "test:config_import_activation_group_requirement_failure"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(db *gorm.DB) {
		if db.Statement != nil && db.Statement.Table == "options" {
			db.AddError(errors.New("injected option update failure"))
		}
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove(callbackName)) })

	_, err := ActivateConfigImportBatch(context.Background(), fixture.BatchID, 42)
	require.ErrorContains(t, err, "injected option update failure")
	var option model.Option
	require.NoError(t, model.DB.Where("key = ?", "GroupRoutingRequirements").First(&option).Error)
	assert.JSONEq(t, `{}`, option.Value)
	var batch model.ConfigImportBatch
	require.NoError(t, model.DB.First(&batch, fixture.BatchID).Error)
	assert.Nil(t, batch.ActivatedAt)
}

func TestActivateConfigImportBatchRetiresImportedTargetsByMergeMode(t *testing.T) {
	for _, test := range []struct {
		name                   string
		mergeMode              types.ConfigImportRouteMergeMode
		expectUnrelatedRetired bool
	}{
		{name: "merge", mergeMode: types.ConfigImportRouteMergeModeMerge},
		{name: "replace", mergeMode: types.ConfigImportRouteMergeModeReplace, expectUnrelatedRetired: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := createActivationApplyFixture(t)
			previousRefresh := refreshConfigImportActivation
			refreshConfigImportActivation = func(ConfigImportRefreshKeys) error { return nil }
			t.Cleanup(func() { refreshConfigImportActivation = previousRefresh })
			var current model.RouteTarget
			require.NoError(t, model.DB.First(&current, fixture.TargetID).Error)
			previousBatchID := int64(18)
			unrelated := current
			unrelated.ID = 0
			unrelated.Name = "unrelated-import"
			unrelated.TargetPriority--
			unrelated.Enabled = true
			unrelated.SourceBatchID = &previousBatchID
			require.NoError(t, model.DB.Create(&unrelated).Error)
			var routeItem model.ConfigImportItem
			require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", fixture.BatchID, "route_blueprints").First(&routeItem).Error)
			var blueprint types.ConfigImportRouteBlueprint
			require.NoError(t, common.UnmarshalJsonStr(routeItem.CanonicalJSON, &blueprint))
			blueprint.MergeMode = test.mergeMode
			encoded, err := common.Marshal(blueprint)
			require.NoError(t, err)
			require.NoError(t, model.DB.Model(&routeItem).Update("canonical_json", string(encoded)).Error)
			persistActivationBaseline(t, fixture.BatchID)

			_, err = ActivateConfigImportBatch(context.Background(), fixture.BatchID, 42)

			require.NoError(t, err)
			var sameName model.RouteTarget
			require.NoError(t, model.DB.First(&sameName, fixture.RetireTargetID).Error)
			assert.False(t, sameName.Enabled)
			var unrelatedLoaded model.RouteTarget
			require.NoError(t, model.DB.First(&unrelatedLoaded, unrelated.ID).Error)
			assert.Equal(t, !test.expectUnrelatedRetired, unrelatedLoaded.Enabled)
			if test.expectUnrelatedRetired {
				assert.NotNil(t, unrelatedLoaded.RetiredAt)
			} else {
				assert.Nil(t, unrelatedLoaded.RetiredAt)
			}
		})
	}
	t.Run("skip", func(t *testing.T) {
		fixture := createActivationApplyFixture(t)
		previousRefresh := refreshConfigImportActivation
		refreshConfigImportActivation = func(ConfigImportRefreshKeys) error { return nil }
		t.Cleanup(func() { refreshConfigImportActivation = previousRefresh })
		require.NoError(t, model.DB.Model(&model.ConfigImportItem{}).
			Where("batch_id = ? AND entity_type = ?", fixture.BatchID, "route_blueprints").
			Updates(map[string]any{"state": string(types.ConfigImportItemStateExcluded), "exclusion_reason": "route merge mode skip"}).Error)
		require.NoError(t, model.DB.Delete(&model.RouteTarget{}, fixture.TargetID).Error)
		persistActivationBaseline(t, fixture.BatchID)

		_, err := ActivateConfigImportBatch(context.Background(), fixture.BatchID, 42)

		require.NoError(t, err)
		var previous model.RouteTarget
		require.NoError(t, model.DB.First(&previous, fixture.RetireTargetID).Error)
		assert.True(t, previous.Enabled)
		assert.Nil(t, previous.RetiredAt)
		var policy model.RoutingPolicy
		require.NoError(t, model.DB.First(&policy, fixture.PolicyID).Error)
		assert.False(t, policy.Enabled)
		var channel model.Channel
		require.NoError(t, model.DB.First(&channel, fixture.ChannelID).Error)
		assert.Equal(t, common.ChannelStatusManuallyDisabled, channel.Status)
	})
}

func TestActivateConfigImportBatchRollsBackAllConfigurationWhenAbilityUpdateFails(t *testing.T) {
	fixture := createActivationApplyFixture(t)
	callbackName := "test:config_import_activation_ability_failure"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(db *gorm.DB) {
		if db.Statement != nil && db.Statement.Table == "abilities" {
			db.AddError(errors.New("injected ability update failure"))
		}
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove(callbackName)) })

	_, err := ActivateConfigImportBatch(context.Background(), fixture.BatchID, 42)

	require.ErrorContains(t, err, "injected ability update failure")
	var previousCostRule model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&previousCostRule, fixture.PreviousCostRuleID).Error)
	assert.Equal(t, string(types.CostRuleActive), previousCostRule.Status)
	var currentCostRule model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&currentCostRule, fixture.CurrentCostRuleID).Error)
	assert.Equal(t, string(types.CostRuleDraft), currentCostRule.Status)
	var option model.Option
	require.NoError(t, model.DB.Where("key = ?", "ModelPrice").First(&option).Error)
	assert.JSONEq(t, `{"legacy-model":9}`, option.Value)
	var channel model.Channel
	require.NoError(t, model.DB.First(&channel, fixture.ChannelID).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, channel.Status)
	assert.Equal(t, "vendor-video", channel.Models)
	assert.Empty(t, channel.GetModelMapping())
	var current model.RouteTarget
	require.NoError(t, model.DB.First(&current, fixture.TargetID).Error)
	assert.False(t, current.Enabled)
	var previous model.RouteTarget
	require.NoError(t, model.DB.First(&previous, fixture.RetireTargetID).Error)
	assert.True(t, previous.Enabled)
	assert.Nil(t, previous.RetiredAt)
	var policy model.RoutingPolicy
	require.NoError(t, model.DB.First(&policy, fixture.PolicyID).Error)
	assert.False(t, policy.Enabled)
	var batch model.ConfigImportBatch
	require.NoError(t, model.DB.First(&batch, fixture.BatchID).Error)
	assert.Nil(t, batch.ActivatedAt)
	var auditCount int64
	require.NoError(t, model.DB.Model(&model.ConfigImportActivationAudit{}).Where("batch_id = ?", fixture.BatchID).Count(&auditCount).Error)
	assert.Zero(t, auditCount)
}

func TestActivateConfigImportBatchRecordsAndRecoversCacheRefreshFailure(t *testing.T) {
	fixture := createActivationApplyFixture(t)
	previousRefresh := refreshConfigImportActivation
	refreshConfigImportActivation = func(ConfigImportRefreshKeys) error { return errors.New("injected cache refresh failure") }
	t.Cleanup(func() { refreshConfigImportActivation = previousRefresh })

	detail, err := ActivateConfigImportBatch(context.Background(), fixture.BatchID, 42)

	assert.Nil(t, detail)
	var schemaErr *ConfigImportSchemaError
	require.ErrorAs(t, err, &schemaErr)
	assert.Equal(t, "ACTIVATION_CACHE_REFRESH_PENDING", schemaErr.Code)
	var pending model.ConfigImportIssue
	require.NoError(t, model.DB.Where("batch_id = ? AND code = ?", fixture.BatchID, "ACTIVATION_CACHE_REFRESH_PENDING").First(&pending).Error)
	assert.Equal(t, "open", pending.ResolutionStatus)
	var activatedAudit model.ConfigImportActivationAudit
	require.NoError(t, model.DB.Where("batch_id = ? AND outcome = ?", fixture.BatchID, "activated").First(&activatedAudit).Error)
	var pendingAudit model.ConfigImportActivationAudit
	require.NoError(t, model.DB.Where("batch_id = ? AND outcome = ?", fixture.BatchID, "cache_refresh_pending").First(&pendingAudit).Error)
	assert.Equal(t, activatedAudit.BeforeSHA256, pendingAudit.BeforeSHA256)
	assert.Equal(t, activatedAudit.AfterSHA256, pendingAudit.AfterSHA256)

	refreshConfigImportActivation = previousRefresh
	require.NoError(t, RetryConfigImportBatchCache(context.Background(), fixture.BatchID, 42))
	require.NoError(t, model.DB.First(&pending, pending.ID).Error)
	assert.Equal(t, "resolved", pending.ResolutionStatus)
	var recoveredAudit model.ConfigImportActivationAudit
	require.NoError(t, model.DB.Where("batch_id = ? AND outcome = ?", fixture.BatchID, "cache_refreshed").First(&recoveredAudit).Error)
	assert.Equal(t, activatedAudit.BeforeSHA256, recoveredAudit.BeforeSHA256)
	assert.Equal(t, activatedAudit.AfterSHA256, recoveredAudit.AfterSHA256)
	var current model.RouteTarget
	require.NoError(t, model.DB.First(&current, fixture.TargetID).Error)
	assert.True(t, current.Enabled)
}

func TestActivateConfigImportBatchAuditsRejectedBlockers(t *testing.T) {
	for _, test := range []struct {
		name           string
		mutate         func(t *testing.T, fixture activationApplyFixture)
		expectSameHash bool
	}{
		{name: "missing key", expectSameHash: true, mutate: func(t *testing.T, fixture activationApplyFixture) {
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", fixture.ChannelID).Update("key", "").Error)
		}},
		{name: "stale baseline", mutate: func(t *testing.T, fixture activationApplyFixture) {
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", fixture.ChannelID).Update("models", "concurrent-model").Error)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := createActivationApplyFixture(t)
			test.mutate(t, fixture)

			_, err := ActivateConfigImportBatch(context.Background(), fixture.BatchID, 42)

			var schemaErr *ConfigImportSchemaError
			require.ErrorAs(t, err, &schemaErr)
			assert.Equal(t, "ACTIVATION_BLOCKED", schemaErr.Code)
			var audit model.ConfigImportActivationAudit
			require.NoError(t, model.DB.Where("batch_id = ? AND outcome = ?", fixture.BatchID, "rejected").First(&audit).Error)
			assert.NotEmpty(t, audit.BeforeSHA256)
			assert.NotEmpty(t, audit.AfterSHA256)
			assert.Equal(t, test.expectSameHash, audit.BeforeSHA256 == audit.AfterSHA256)
		})
	}
}

func TestPreviewConfigImportBatchActivationBlockers(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, fixture *activationFixture)
		code   string
	}{
		{name: "unpublished batch", code: "ACTIVATION_BATCH_STATUS", mutate: func(t *testing.T, fixture *activationFixture) {
			require.NoError(t, model.DB.Model(&model.ConfigImportBatch{}).Where("id = ?", fixture.BatchID).Update("status", string(types.ConfigImportBatchStatusReady)).Error)
		}},
		{name: "open issue", code: "ACTIVATION_OPEN_ISSUES", mutate: func(t *testing.T, fixture *activationFixture) {
			require.NoError(t, model.DB.Create(&model.ConfigImportIssue{BatchID: fixture.BatchID, Severity: string(types.ConfigImportIssueSeverityWarning), Code: "OPEN_WARNING", Message: "review required", ResolutionStatus: "open"}).Error)
		}},
		{name: "stale activation baseline", code: "ACTIVATION_STALE_BASE_VERSION", mutate: func(t *testing.T, fixture *activationFixture) {
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", fixture.ChannelID).Update("models", "concurrent-model").Error)
		}},
		{name: "unconfirmed binding", code: "ACTIVATION_CREDENTIALS_UNCONFIRMED", mutate: func(t *testing.T, fixture *activationFixture) {
			require.NoError(t, model.DB.Model(&model.ConfigImportBinding{}).Where("id = ?", fixture.BindingID).Updates(map[string]any{"credentials_confirmed_by": 0, "credentials_confirmed_at": nil}).Error)
		}},
		{name: "missing candidate", code: "ACTIVATION_TARGET_MISSING", mutate: func(t *testing.T, fixture *activationFixture) {
			require.NoError(t, model.DB.Delete(&model.RouteTarget{}, fixture.TargetID).Error)
			persistActivationBaseline(t, fixture.BatchID)
		}},
		{name: "empty key", code: "ACTIVATION_CHANNEL_KEY_MISSING", mutate: func(t *testing.T, fixture *activationFixture) {
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", fixture.ChannelID).Update("key", "").Error)
		}},
		{name: "auto disabled", code: "ACTIVATION_CHANNEL_AUTO_DISABLED", mutate: func(t *testing.T, fixture *activationFixture) {
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", fixture.ChannelID).Update("status", common.ChannelStatusAutoDisabled).Error)
		}},
		{name: "missing model mapping", code: "ACTIVATION_MODEL_MAPPING_MISSING", mutate: func(t *testing.T, fixture *activationFixture) {
			require.NoError(t, model.DB.Delete(&model.ConfigImportItem{}, fixture.MappingItemID).Error)
			persistActivationBaseline(t, fixture.BatchID)
		}},
		{name: "missing cost draft", code: "ACTIVATION_COST_DRAFT_MISSING", mutate: func(t *testing.T, fixture *activationFixture) {
			require.NoError(t, model.DB.Delete(&model.ChannelModelCostRule{}, fixture.CostDraftID).Error)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := createActivationFixture(t)
			test.mutate(t, &fixture)

			preview, err := PreviewConfigImportBatchActivation(context.Background(), fixture.BatchID)

			require.NoError(t, err)
			assert.False(t, preview.Ready)
			require.Len(t, preview.Blockers, 1)
			assert.Equal(t, test.code, preview.Blockers[0].Code)
		})
	}
}

func TestPreviewConfigImportBatchActivationRejectsContractMismatch(t *testing.T) {
	fixture := createActivationFixture(t)
	previousValidator := RouteTargetContractValidator
	RouteTargetContractValidator = func(*model.Channel, string, modelrouting.Target) error {
		return errors.New("provider contract mismatch")
	}
	t.Cleanup(func() { RouteTargetContractValidator = previousValidator })

	preview, err := PreviewConfigImportBatchActivation(context.Background(), fixture.BatchID)

	require.NoError(t, err)
	assert.False(t, preview.Ready)
	require.Len(t, preview.Blockers, 1)
	assert.Equal(t, "ACTIVATION_CHANNEL_CONTRACT", preview.Blockers[0].Code)
}

func TestPreviewConfigImportBatchActivationRejectsManualOverlap(t *testing.T) {
	fixture := createActivationFixture(t)
	var candidate model.RouteTarget
	require.NoError(t, model.DB.First(&candidate, fixture.TargetID).Error)
	candidate.ID = 0
	candidate.Name = "manual-overlap"
	candidate.Enabled = true
	candidate.ManagedBy = string(types.RouteTargetManagedByManual)
	candidate.SourceBatchID = nil
	require.NoError(t, model.DB.Create(&candidate).Error)
	persistActivationBaseline(t, fixture.BatchID)

	preview, err := PreviewConfigImportBatchActivation(context.Background(), fixture.BatchID)

	require.NoError(t, err)
	assert.False(t, preview.Ready)
	require.Len(t, preview.Blockers, 1)
	assert.Equal(t, "ACTIVATION_ROUTING_CONFLICT", preview.Blockers[0].Code)
}

func TestPreviewConfigImportBatchActivationRejectsUnexpectedBatchCandidate(t *testing.T) {
	fixture := createActivationFixture(t)
	var candidate model.RouteTarget
	require.NoError(t, model.DB.First(&candidate, fixture.TargetID).Error)
	candidate.ID = 0
	candidate.Name = "unexpected-candidate"
	require.NoError(t, model.DB.Create(&candidate).Error)
	persistActivationBaseline(t, fixture.BatchID)

	preview, err := PreviewConfigImportBatchActivation(context.Background(), fixture.BatchID)

	require.NoError(t, err)
	assert.False(t, preview.Ready)
	require.Len(t, preview.Blockers, 1)
	assert.Equal(t, "ACTIVATION_TARGET_MISSING", preview.Blockers[0].Code)
}

func TestPreviewConfigImportBatchActivationIsDeterministicAndReadOnly(t *testing.T) {
	fixture := createActivationFixture(t)
	before, err := CaptureConfigImportBaseline(model.DB, fixture.BatchID)
	require.NoError(t, err)

	preview, err := PreviewConfigImportBatchActivation(context.Background(), fixture.BatchID)

	require.NoError(t, err)
	assert.True(t, preview.Ready)
	assert.Equal(t, 1, preview.ChannelCount)
	assert.Equal(t, 1, preview.PolicyCount)
	assert.Equal(t, 1, preview.TargetCount)
	assert.Equal(t, 1, preview.RetireTargetCount)
	assert.Empty(t, preview.Blockers)
	after, err := CaptureConfigImportBaseline(model.DB, fixture.BatchID)
	require.NoError(t, err)
	assert.Equal(t, before.Hash, after.Hash)

	var channel model.Channel
	require.NoError(t, model.DB.First(&channel, fixture.ChannelID).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, channel.Status)
	var policy model.RoutingPolicy
	require.NoError(t, model.DB.First(&policy, fixture.PolicyID).Error)
	assert.False(t, policy.Enabled)
	var candidate model.RouteTarget
	require.NoError(t, model.DB.First(&candidate, fixture.TargetID).Error)
	assert.False(t, candidate.Enabled)
	var retired model.RouteTarget
	require.NoError(t, model.DB.First(&retired, fixture.RetireTargetID).Error)
	assert.True(t, retired.Enabled)
	assert.Nil(t, retired.RetiredAt)
	var draft model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&draft, fixture.CostDraftID).Error)
	assert.Equal(t, string(types.CostRuleDraft), draft.Status)
}

func createActivationApplyFixture(t *testing.T) activationApplyFixture {
	t.Helper()
	fixture := createActivationFixture(t)
	common.OptionMapRWMutex.Lock()
	previousOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
	})
	var currentCostRule model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&currentCostRule, fixture.CostDraftID).Error)
	previousCostRule := &model.ChannelModelCostRule{
		ChannelID: currentCostRule.ChannelID, BillableUpstreamModel: currentCostRule.BillableUpstreamModel,
		CostVariantKey: currentCostRule.CostVariantKey, Version: currentCostRule.Version + 1,
		Status: string(types.CostRuleActive), CostMode: currentCostRule.CostMode,
		SchemaVersion: currentCostRule.SchemaVersion, ConfigJSON: currentCostRule.ConfigJSON,
	}
	require.NoError(t, model.DB.Create(previousCostRule).Error)
	var current model.RouteTarget
	require.NoError(t, model.DB.First(&current, fixture.TargetID).Error)
	manual := current
	manual.ID = 0
	manual.Name = "manual-target"
	manual.TargetPriority++
	manual.Enabled = true
	manual.ManagedBy = string(types.RouteTargetManagedByManual)
	manual.SourceBatchID = nil
	require.NoError(t, model.DB.Create(&manual).Error)
	require.NoError(t, model.DB.Create(&model.Option{Key: "ModelPrice", Value: `{"legacy-model":9}`}).Error)
	var saleItem model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", fixture.BatchID, "sale_proposals").First(&saleItem).Error)
	saleDocument := map[string]any{
		"business_id":   "sale-a",
		"model_sku_ref": "sku-a",
		"staged_proposal": map[string]any{
			"option_patches": map[string]any{
				"ModelPrice": map[string]any{modelrouting.Seedance20: 2},
			},
		},
	}
	encoded, err := common.Marshal(saleDocument)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&saleItem).Updates(map[string]any{
		"canonical_json": string(encoded), "state": string(types.ConfigImportItemStateChanged), "exclusion_reason": "",
	}).Error)
	persistActivationBaseline(t, fixture.BatchID)
	return activationApplyFixture{
		activationFixture: fixture, CurrentCostRuleID: currentCostRule.ID,
		PreviousCostRuleID: previousCostRule.ID, ManualTargetID: manual.ID,
	}
}

func createActivationFixture(t *testing.T) activationFixture {
	t.Helper()
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{}, &model.Ability{}, &model.ChannelModelCostRule{}, &model.Option{},
		&model.RoutingPolicy{}, &model.RouteTarget{}, &model.ConfigImportActivationAudit{},
	))
	previousLookup := CostCapabilityLookup
	CostCapabilityLookup = func(int, string, constant.TaskPlatform) types.CostCapabilities {
		return completeCostCapabilities()
	}
	t.Cleanup(func() { CostCapabilityLookup = previousLookup })

	channel := &model.Channel{
		Type: 1, Name: "activation-channel", Group: "default", Status: common.ChannelStatusManuallyDisabled,
		Models: "vendor-video", Key: "real-key",
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, channel.UpdateAbilities(model.DB))
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")

	var routeItem model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "route_blueprints").First(&routeItem).Error)
	var blueprint types.ConfigImportRouteBlueprint
	require.NoError(t, common.UnmarshalJsonStr(routeItem.CanonicalJSON, &blueprint))
	blueprint.CanonicalModel = modelrouting.Seedance20
	blueprint.ClientModel = modelrouting.Seedance20
	blueprint.Targets[0].OutputResolutions = []string{"720p"}
	blueprint.Targets[0].DurationValues = []int{10}
	blueprint.Targets[0].AspectRatios = []string{"16:9"}
	blueprint.Targets[0].InputModes = []string{"text"}
	encoded, err := common.Marshal(blueprint)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&routeItem).Update("canonical_json", string(encoded)).Error)
	require.NoError(t, model.DB.Model(&model.ConfigImportItem{}).
		Where("batch_id = ? AND entity_type = ?", batch.ID, "sale_proposals").
		Updates(map[string]any{"state": string(types.ConfigImportItemStateExcluded), "exclusion_reason": "not part of activation fixture"}).Error)

	mapping := types.ConfigImportModelMapping{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "mapping-a"},
		CanonicalModel:                  modelrouting.Seedance20, ClientModel: modelrouting.Seedance20,
		LineRef: "line-a", UpstreamModel: "vendor-video", SKURef: "sku-a",
	}
	encoded, err = common.Marshal(mapping)
	require.NoError(t, err)
	mappingItem := &model.ConfigImportItem{
		BatchID: batch.ID, EntityType: "model_mappings", BusinessID: mapping.BusinessID,
		CanonicalJSON: string(encoded), State: string(types.ConfigImportItemStateNew),
	}
	require.NoError(t, model.DB.Create(mappingItem).Error)

	staged, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	require.Equalf(t, types.ConfigImportBatchStatusReady, staged.Status, "stage issues: %+v", staged.Issues)
	require.NoError(t, PublishConfigImportBatch(context.Background(), batch.ID, 42))

	var binding model.ConfigImportBinding
	require.NoError(t, model.DB.Where("batch_id = ? AND line_ref = ?", batch.ID, "line-a").First(&binding).Error)
	var candidate model.RouteTarget
	require.NoError(t, model.DB.Where("source_batch_id = ?", batch.ID).First(&candidate).Error)
	previousBatchID := int64(19)
	retired := candidate
	retired.ID = 0
	retired.Enabled = true
	retired.SourceBatchID = &previousBatchID
	require.NoError(t, model.DB.Create(&retired).Error)
	var costItem model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "cost_rule_drafts").First(&costItem).Error)
	require.NotNil(t, costItem.MaterializedID)
	persistActivationBaseline(t, batch.ID)

	return activationFixture{
		BatchID: batch.ID, BindingID: binding.ID, ChannelID: channel.Id, PolicyID: candidate.PolicyID,
		TargetID: candidate.ID, RetireTargetID: retired.ID, MappingItemID: mappingItem.ID,
		CostDraftID: int64(*costItem.MaterializedID),
	}
}

func persistActivationBaseline(t *testing.T, batchID int64) {
	t.Helper()
	baseline, err := CaptureConfigImportBaseline(model.DB, batchID)
	require.NoError(t, err)
	encoded, err := common.Marshal(baseline)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.ConfigImportBatch{}).
		Where("id = ?", batchID).Update("baseline_json", string(encoded)).Error)
}
