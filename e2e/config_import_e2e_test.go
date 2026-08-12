package e2e

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/relay"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigImportV1FixtureStagesStructuredMaterialContractsE2E(t *testing.T) {
	t.Cleanup(func() {
		if model.DB != nil {
			require.NoError(t, model.InitRoutingPolicyCache())
			model.InitChannelCache()
		}
	})
	setupSeedanceE2EDB(t)
	previousCostCapabilityLookup := service.CostCapabilityLookup
	previousRouteTargetContractValidator := service.RouteTargetContractValidator
	service.CostCapabilityLookup = relay.CostCapabilitiesForRoute
	service.RouteTargetContractValidator = relay.ValidateVideoRouteTargetContract
	t.Cleanup(func() {
		service.CostCapabilityLookup = previousCostCapabilityLookup
		service.RouteTargetContractValidator = previousRouteTargetContractValidator
	})
	common.OptionMapRWMutex.Lock()
	previousOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
	})
	billingConfig := config.GlobalConfig.Get("billing_setting")
	originalBillingConfig, err := config.ConfigToMap(billingConfig)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(billingConfig, originalBillingConfig))
	})
	require.NoError(t, model.DB.AutoMigrate(
		&model.ConfigImportBatch{},
		&model.ConfigImportItem{},
		&model.ConfigImportBinding{},
		&model.ConfigImportIssue{},
		&model.ConfigImportResolution{},
		&model.ConfigImportPublishAudit{},
		&model.ConfigImportActivationAudit{},
		&model.ConfigImportRouteOwnershipChange{},
		&model.ChannelModelCostRule{},
		&model.Option{},
		&model.RoutingPolicy{},
		&model.RouteTarget{},
	))

	fixturePath := filepath.Join("testdata", "channel-config-v1.json")
	payload, err := os.ReadFile(fixturePath)
	require.NoError(t, err)
	document, err := service.ParseConfigImportDocument(bytes.NewReader(payload))
	require.NoError(t, err)

	first, created, err := service.CreateConfigImportBatch(context.Background(), 1, bytes.NewReader(payload))
	require.NoError(t, err)
	require.True(t, created)
	assert.Equal(t, types.ConfigImportEntityCounts{
		Channels: 9, ChannelLines: 11, ModelSKUs: 8, SaleProposals: 16,
		CostRuleDrafts: 143, ModelMappings: 143, RouteBlueprints: 143,
		Sources: 12, UnresolvedVariants: 0,
	}, first.ItemCounts)
	assert.Equal(t, types.ConfigImportBatchStatusBinding, first.Status)
	assert.Equal(t, []string{"bind", "resolve", "stage"}, first.AllowedActions)
	assert.Empty(t, first.Issues)
	assert.Empty(t, document.Entities.UnresolvedVariants)

	for _, blueprint := range document.Entities.RouteBlueprints {
		require.Len(t, blueprint.Targets, 1, blueprint.BusinessID)
		target := blueprint.Targets[0]
		assert.NotEmpty(t, target.RouteTargetRef, blueprint.BusinessID)
		assert.NotEmpty(t, target.CostVariantKey, blueprint.BusinessID)
		assert.NotEmpty(t, target.OutputResolutions, blueprint.BusinessID)
		assert.NotEmpty(t, target.InputModes, blueprint.BusinessID)
		hasDurationRange := target.DurationMin != nil || target.DurationMax != nil
		hasDurationValues := len(target.DurationValues) > 0
		require.NotEqual(t, hasDurationRange, hasDurationValues, blueprint.BusinessID)
		if hasDurationValues {
			for _, duration := range target.DurationValues {
				assert.Positive(t, duration, blueprint.BusinessID)
			}
		} else {
			require.NotNil(t, target.DurationMin, blueprint.BusinessID)
			require.NotNil(t, target.DurationMax, blueprint.BusinessID)
			assert.LessOrEqual(t, *target.DurationMin, *target.DurationMax, blueprint.BusinessID)
		}
		require.NotNil(t, target.ReferenceMinimums, blueprint.BusinessID)
		require.NotNil(t, target.ReferenceLimits, blueprint.BusinessID)
	}

	channelTypes := make(map[string]int, len(document.Entities.Channels))
	for _, importedChannel := range document.Entities.Channels {
		require.NotNil(t, importedChannel.ChannelType, importedChannel.BusinessID)
		channelTypes[importedChannel.BusinessID] = *importedChannel.ChannelType
	}
	modelsByLine := make(map[string]map[string]struct{})
	for _, mapping := range document.Entities.ModelMappings {
		if modelsByLine[mapping.LineRef] == nil {
			modelsByLine[mapping.LineRef] = make(map[string]struct{})
		}
		modelsByLine[mapping.LineRef][mapping.UpstreamModel] = struct{}{}
	}
	bindings := make([]dto.ConfigImportBindingInput, 0, len(document.Entities.ChannelLines))
	channelIDsByLine := make(map[string]int, len(document.Entities.ChannelLines))
	for _, line := range document.Entities.ChannelLines {
		models := make([]string, 0, len(modelsByLine[line.LineRef]))
		for upstreamModel := range modelsByLine[line.LineRef] {
			models = append(models, upstreamModel)
		}
		sort.Strings(models)
		channel := &model.Channel{
			Type:   channelTypes[line.ChannelRef],
			Name:   "config-import-e2e-" + line.LineRef,
			Group:  "default",
			Models: strings.Join(models, ","),
			Key:    "mock-key-" + line.LineRef,
			Status: common.ChannelStatusEnabled,
		}
		if strings.HasPrefix(line.LineRef, "secure-") {
			settings, marshalErr := common.Marshal(relaydto.ChannelOtherSettings{
				SecureVideoGroup: relaydto.SecureVideoGroup(strings.TrimPrefix(line.LineRef, "secure-")),
			})
			require.NoError(t, marshalErr)
			channel.OtherSettings = string(settings)
		}
		require.NoError(t, model.DB.Create(channel).Error)
		channelIDsByLine[line.LineRef] = channel.Id
		bindings = append(bindings, dto.ConfigImportBindingInput{
			LineRef: line.LineRef, Action: types.ConfigImportBindingActionBind, ChannelID: &channel.Id, CredentialsConfirmed: true,
		})
	}

	bound, err := service.UpdateConfigImportBindings(context.Background(), 1, first.ID, bindings)
	require.NoError(t, err)
	require.Len(t, bound.Bindings, len(document.Entities.ChannelLines))

	staged, err := service.StageConfigImportBatch(context.Background(), 1, first.ID)
	require.NoError(t, err)
	require.Equalf(t, types.ConfigImportBatchStatusReady, staged.Status, "stage issues: %+v", staged.Issues)
	for _, issue := range staged.Issues {
		assert.NotEqual(t, "COST_VARIANT_AMBIGUOUS", issue.Code, issue.BusinessID)
	}
	var materializedRuleCount int64
	require.NoError(t, model.DB.Model(&model.ChannelModelCostRule{}).Where("source = ?", "config_import").Count(&materializedRuleCount).Error)
	assert.EqualValues(t, len(document.Entities.CostRuleDrafts), materializedRuleCount)
	var stagedRuleCount int64
	require.NoError(t, model.DB.Model(&model.ChannelModelCostRule{}).Where("source = ? AND status = ?", "config_import", types.CostRuleDraft).Count(&stagedRuleCount).Error)
	assert.EqualValues(t, len(document.Entities.CostRuleDrafts), stagedRuleCount)

	var distinctRouteCosts []model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND business_id IN ?", first.ID, []string{
		"COST-PAIPU-R24-480-REQ", "COST-PAIPU-R25-720-REQ",
	}).Order("business_id ASC").Find(&distinctRouteCosts).Error)
	require.Len(t, distinctRouteCosts, 2)
	require.NotNil(t, distinctRouteCosts[0].MaterializedID)
	require.NotNil(t, distinctRouteCosts[1].MaterializedID)
	assert.NotEqual(t, *distinctRouteCosts[0].MaterializedID, *distinctRouteCosts[1].MaterializedID)

	require.NoError(t, service.PublishConfigImportBatch(context.Background(), first.ID, 1))
	var publishedBatch model.ConfigImportBatch
	require.NoError(t, model.DB.First(&publishedBatch, first.ID).Error)
	assert.Equal(t, string(types.ConfigImportBatchStatusPublished), publishedBatch.Status)
	var activeRuleCount int64
	require.NoError(t, model.DB.Model(&model.ChannelModelCostRule{}).Where("source = ? AND status = ?", "config_import", types.CostRuleActive).Count(&activeRuleCount).Error)
	assert.Zero(t, activeRuleCount)
	var remainingDraftCount int64
	require.NoError(t, model.DB.Model(&model.ChannelModelCostRule{}).Where("source = ? AND status = ?", "config_import", types.CostRuleDraft).Count(&remainingDraftCount).Error)
	assert.EqualValues(t, len(document.Entities.CostRuleDrafts), remainingDraftCount)
	preview, err := service.PreviewConfigImportBatchActivation(context.Background(), first.ID)
	require.NoError(t, err)
	require.Truef(t, preview.Ready, "activation blockers: %+v", preview.Blockers)
	_, err = service.ActivateConfigImportBatch(context.Background(), first.ID, 1)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.ChannelModelCostRule{}).Where("source = ? AND status = ?", "config_import", types.CostRuleActive).Count(&activeRuleCount).Error)
	assert.EqualValues(t, len(document.Entities.CostRuleDrafts), activeRuleCount)
	require.NoError(t, model.DB.Model(&model.ChannelModelCostRule{}).Where("source = ? AND status = ?", "config_import", types.CostRuleDraft).Count(&remainingDraftCount).Error)
	assert.Zero(t, remainingDraftCount)
	var dimensioChannel model.Channel
	require.NoError(t, model.DB.First(&dimensioChannel, channelIDsByLine["channel-dimensio"]).Error)
	var dimensioMapping map[string]string
	require.NoError(t, common.UnmarshalJsonStr(dimensioChannel.GetModelMapping(), &dimensioMapping))
	assert.NotContains(t, dimensioMapping, modelrouting.Seedance20)
	assert.Contains(t, dimensioChannel.GetModels(), modelrouting.Seedance20)
	assert.Contains(t, dimensioChannel.GetModels(), "jmg-video-seedance-2.0-vip")
	assert.Contains(t, dimensioChannel.GetModels(), "pxv-seedance-2.0-standard")
	var standardPolicy model.RoutingPolicy
	require.NoError(t, model.DB.Where("group_name = ? AND model = ?", "default", modelrouting.Seedance20).First(&standardPolicy).Error)
	var dimensioTargets []model.RouteTarget
	require.NoError(t, model.DB.Where("policy_id = ? AND channel_id = ?", standardPolicy.ID, dimensioChannel.Id).Find(&dimensioTargets).Error)
	upstreamModels := make(map[string]struct{})
	for _, target := range dimensioTargets {
		upstreamModels[target.UpstreamModel] = struct{}{}
	}
	assert.Contains(t, upstreamModels, "jmg-video-seedance-2.0-vip")
	assert.Contains(t, upstreamModels, "pxv-seedance-2.0-standard")

	var selectedTarget model.RouteTarget
	var selectedConstraints modelrouting.Constraints
	for _, target := range dimensioTargets {
		if target.UpstreamModel != "jmg-video-seedance-2.0-vip" {
			continue
		}
		var constraints modelrouting.Constraints
		require.NoError(t, common.UnmarshalJsonStr(target.Constraints, &constraints))
		if len(constraints.OutputResolutions) == 1 && constraints.OutputResolutions[0] == "720p" {
			selectedTarget = target
			selectedConstraints = constraints
			break
		}
	}
	require.NotZero(t, selectedTarget.ID)
	duration := 4
	if selectedConstraints.Durations.Min != nil {
		duration = *selectedConstraints.Durations.Min
	} else if len(selectedConstraints.Durations.Values) > 0 {
		duration = selectedConstraints.Durations.Values[0]
	}
	ratio := "16:9"
	if len(selectedConstraints.AspectRatios) > 0 {
		ratio = selectedConstraints.AspectRatios[0]
	}
	resolution := "720p"
	policyTargets := make([]service.RouteTargetWriteRequest, 0, len(dimensioTargets))
	for _, target := range dimensioTargets {
		var constraints modelrouting.Constraints
		require.NoError(t, common.UnmarshalJsonStr(target.Constraints, &constraints))
		policyTargets = append(policyTargets, service.RouteTargetWriteRequest{
			ID:        common.GetPointer(target.ID),
			ChannelID: target.ChannelID, Name: target.Name, UpstreamModel: target.UpstreamModel,
			CostVariantKey: target.CostVariantKey, TargetPriority: target.TargetPriority,
			MinimumExpectedMarginBPS: target.MinimumExpectedMarginBPS,
			Enabled:                  target.ID == selectedTarget.ID,
			Constraints:              constraints,
		})
	}
	enabledPolicy, err := service.SaveRoutingPolicy(standardPolicy.ID, service.RoutingPolicyWriteRequest{
		GroupName: standardPolicy.GroupName, Model: standardPolicy.Model, Enabled: true,
		Defaults: modelrouting.Defaults{
			OutputResolution: resolution,
			DurationSeconds:  duration,
			AspectRatio:      ratio,
		},
		Targets: policyTargets,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(enabledPolicy.Targets), len(dimensioTargets))
	var enabledTargetIDs []int
	for _, target := range enabledPolicy.Targets {
		if target.Enabled {
			enabledTargetIDs = append(enabledTargetIDs, target.ID)
		}
	}
	assert.Contains(t, enabledTargetIDs, selectedTarget.ID)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	routingInput := modelrouting.FactsInput{
		CanonicalModel: modelrouting.Seedance20, InputMode: modelrouting.InputModeText,
		OutputResolution: &resolution, DurationSeconds: &duration, AspectRatio: &ratio,
	}
	selectedChannel, selectedGroup, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx: ctx, TokenGroup: "default", ModelName: modelrouting.Seedance20,
		RequestPath: "/v1/video/generations", Retry: common.GetPointer(0), RoutingInput: &routingInput,
	})
	require.NoError(t, err)
	require.NotNil(t, selectedChannel)
	assert.Equal(t, dimensioChannel.Id, selectedChannel.Id)
	assert.Equal(t, "default", selectedGroup)
	assert.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyRoutingCapabilityMode))
	assert.Equal(t, "jmg-video-seedance-2.0-vip", common.GetContextKeyString(ctx, constant.ContextKeyRoutingUpstreamModel))
	assert.Equal(t, selectedTarget.CostVariantKey, common.GetContextKeyString(ctx, constant.ContextKeyRoutingCostVariant))

	duplicate, created, err := service.CreateConfigImportBatch(context.Background(), 1, bytes.NewReader(payload))
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.ID, duplicate.ID)
}
