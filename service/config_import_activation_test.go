package service

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	RouteTargetContractValidator = func(*model.Channel, modelrouting.Target) error {
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

func createActivationFixture(t *testing.T) activationFixture {
	t.Helper()
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{}, &model.Ability{}, &model.ChannelModelCostRule{}, &model.Option{},
		&model.RoutingPolicy{}, &model.RouteTarget{},
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
