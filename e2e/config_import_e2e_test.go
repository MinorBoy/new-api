package e2e

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigImportV1FixturePublishesDisabledConfigurationE2E(t *testing.T) {
	setupSeedanceE2EDB(t)
	common.OptionMapRWMutex.Lock()
	previousOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
	})
	require.NoError(t, model.DB.AutoMigrate(
		&model.ConfigImportBatch{},
		&model.ConfigImportItem{},
		&model.ConfigImportBinding{},
		&model.ConfigImportIssue{},
		&model.ConfigImportResolution{},
		&model.ConfigImportPublishAudit{},
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
		Channels: 9, ChannelLines: 12, ModelSKUs: 9, SaleProposals: 16,
		CostRuleDrafts: 104, ModelMappings: 104, RouteBlueprints: 104,
		Sources: 13, UnresolvedVariants: 1,
	}, first.ItemCounts)

	duplicate, created, err := service.CreateConfigImportBatch(context.Background(), 1, bytes.NewReader(payload))
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.ID, duplicate.ID)

	channelIDs := bindFixtureLinesToDisabledChannels(t, document, first.ID)
	staged, err := service.StageConfigImportBatch(context.Background(), 1, first.ID)
	require.NoError(t, err)
	assert.Equal(t, types.ConfigImportBatchStatusStaged, staged.Status)

	_, err = service.UpdateConfigImportResolutions(context.Background(), 1, first.ID, []dto.ConfigImportResolutionInput{{
		ItemBusinessID: "CH-MEGABYAI/videos-standard",
		Action:         types.ConfigImportResolutionActionExclude,
		Reason:         "the workbook has no verified target line",
	}})
	require.NoError(t, err)

	ready, err := service.StageConfigImportBatch(context.Background(), 1, first.ID)
	require.NoError(t, err)
	conflicts := make([]string, 0)
	for _, item := range ready.Items {
		if item.State == types.ConfigImportItemStateConflict {
			conflicts = append(conflicts, item.BusinessID+": "+item.ConflictReason)
		}
	}
	require.Equalf(t, types.ConfigImportBatchStatusReady, ready.Status, "issues: %+v; conflicts: %v", ready.Issues, conflicts)

	var activeBefore int64
	require.NoError(t, model.DB.Model(&model.ChannelModelCostRule{}).
		Where("status = ?", types.CostRuleActive).Count(&activeBefore).Error)
	assert.Zero(t, activeBefore)

	require.NoError(t, service.PublishConfigImportBatch(context.Background(), first.ID, 1))
	published, err := service.GetConfigImportBatch(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, types.ConfigImportBatchStatusPublished, published.Status)
	var audit model.ConfigImportPublishAudit
	require.NoError(t, model.DB.Where("batch_id = ?", first.ID).First(&audit).Error)
	assert.NotEmpty(t, audit.BeforeSHA256)
	assert.NotEmpty(t, audit.AfterSHA256)
	assert.NotEqual(t, audit.BeforeSHA256, audit.AfterSHA256)
	var billingExpression model.Option
	require.NoError(t, model.DB.Where("key = ?", "billing_setting.billing_expr").First(&billingExpression).Error)
	assert.Contains(t, billingExpression.Value, modelrouting.Seedance20)

	var activeAfter int64
	require.NoError(t, model.DB.Model(&model.ChannelModelCostRule{}).
		Where("status = ?", types.CostRuleActive).Count(&activeAfter).Error)
	assert.Positive(t, activeAfter)

	for _, channelID := range channelIDs {
		var channel model.Channel
		require.NoError(t, model.DB.First(&channel, channelID).Error)
		assert.Equal(t, common.ChannelStatusManuallyDisabled, channel.Status)
	}
	var policies []model.RoutingPolicy
	require.NoError(t, model.DB.Find(&policies).Error)
	require.NotEmpty(t, policies)
	for _, policy := range policies {
		assert.False(t, policy.Enabled)
		var targets []model.RouteTarget
		require.NoError(t, model.DB.Where("policy_id = ?", policy.ID).Find(&targets).Error)
		for _, target := range targets {
			assert.False(t, target.Enabled)
		}
	}

	err = service.PublishConfigImportBatch(context.Background(), first.ID, 1)
	var schemaErr *service.ConfigImportSchemaError
	require.ErrorAs(t, err, &schemaErr)
	assert.Equal(t, "PUBLISH_ALREADY_COMPLETE", schemaErr.Code)
}

func bindFixtureLinesToDisabledChannels(t *testing.T, document *types.ConfigImportDocument, batchID int64) []int {
	t.Helper()
	channelsByRef := make(map[string]types.ConfigImportChannel, len(document.Entities.Channels))
	for _, channel := range document.Entities.Channels {
		channelsByRef[channel.BusinessID] = channel
	}
	modelsByLine := make(map[string]map[string]struct{}, len(document.Entities.ChannelLines))
	for _, mapping := range document.Entities.ModelMappings {
		if modelsByLine[mapping.LineRef] == nil {
			modelsByLine[mapping.LineRef] = make(map[string]struct{})
		}
		modelsByLine[mapping.LineRef][mapping.UpstreamModel] = struct{}{}
	}

	bindings := make([]dto.ConfigImportBindingInput, 0, len(document.Entities.ChannelLines))
	channelIDs := make([]int, 0, len(document.Entities.ChannelLines))
	for _, line := range document.Entities.ChannelLines {
		definition, found := channelsByRef[line.ChannelRef]
		require.True(t, found)
		require.NotNil(t, definition.ChannelType)

		models := make([]string, 0, len(modelsByLine[line.LineRef]))
		for upstreamModel := range modelsByLine[line.LineRef] {
			models = append(models, upstreamModel)
		}
		sort.Strings(models)
		require.NotEmpty(t, models)

		settings := dto.ChannelOtherSettings{}
		switch line.LineRef {
		case "secure-discount":
			settings.SecureVideoGroup = dto.SecureVideoGroupDiscount
		case "secure-overseas":
			settings.SecureVideoGroup = dto.SecureVideoGroupOverseas
		case "secure-enterprise":
			settings.SecureVideoGroup = dto.SecureVideoGroupEnterprise
		}
		settingsJSON, err := common.Marshal(settings)
		require.NoError(t, err)
		channel := model.Channel{
			Type:          *definition.ChannelType,
			Name:          "config-import-" + line.LineRef,
			Models:        strings.Join(models, ","),
			Status:        common.ChannelStatusManuallyDisabled,
			OtherSettings: string(settingsJSON),
		}
		require.NoError(t, model.DB.Create(&channel).Error)
		channelIDs = append(channelIDs, channel.Id)
		channelID := channel.Id
		bindings = append(bindings, dto.ConfigImportBindingInput{
			LineRef: line.LineRef, Action: types.ConfigImportBindingActionCreate,
			ChannelID: &channelID, CredentialsConfirmed: true,
		})
	}

	_, err := service.UpdateConfigImportBindings(context.Background(), 1, batchID, bindings)
	require.NoError(t, err)
	return channelIDs
}
