package service

import (
	"context"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type configImportBindingLineFixture struct {
	lineRef            string
	channelRef         string
	channelType        int
	models             []string
	supportsRealPerson *bool
}

func TestConfigImportBindingBindsExistingMatchingChannel(t *testing.T) {
	prepareConfigImportBindingDB(t)
	batch := createConfigImportBindingBatch(t, configImportBindingLineFixture{
		lineRef: "line-openai", channelRef: "supplier-openai", channelType: constant.ChannelTypeOpenAI,
		models: []string{"gpt-test"},
	})
	channel := &model.Channel{
		Type: constant.ChannelTypeOpenAI, Name: "Existing OpenAI supplier", Models: "gpt-test,unused-model",
		Group: "default", Key: "existing-key", Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, model.DB.Create(channel).Error)

	_, err := UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "line-openai", Action: types.ConfigImportBindingActionBind, ChannelID: &channel.Id,
	}})

	require.NoError(t, err)
	var binding model.ConfigImportBinding
	require.NoError(t, model.DB.Where("batch_id = ? AND line_ref = ?", batch.ID, "line-openai").First(&binding).Error)
	assert.Equal(t, string(types.ConfigImportBindingActionBind), binding.Action)
	require.NotNil(t, binding.ChannelID)
	assert.Equal(t, channel.Id, *binding.ChannelID)
	assert.Zero(t, binding.CredentialsConfirmedBy)
	assert.Nil(t, binding.CredentialsConfirmedAt)
}

func TestConfigImportBindingUsesMappingModelsForGlobalSKU(t *testing.T) {
	prepareConfigImportBindingDB(t)
	batch := createConfigImportBindingBatch(t, configImportBindingLineFixture{
		lineRef: "line-openai", channelRef: "supplier-openai", channelType: constant.ChannelTypeOpenAI,
	})
	persistConfigImportBindingItem(t, batch.ID, "model_skus", "sku-global", types.ConfigImportModelSKU{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "sku-global"},
	})
	persistConfigImportBindingItem(t, batch.ID, "model_mappings", "mapping-openai", types.ConfigImportModelMapping{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "mapping-openai"},
		CanonicalModel:                  "gpt-test",
		ClientModel:                     "gpt-test",
		LineRef:                         "line-openai",
		UpstreamModel:                   "gpt-test",
		SKURef:                          "sku-global",
	})
	channel := &model.Channel{
		Type: constant.ChannelTypeOpenAI, Name: "Existing OpenAI supplier", Models: "gpt-test",
		Group: "default", Key: "existing-key", Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, model.DB.Create(channel).Error)

	_, err := UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "line-openai", Action: types.ConfigImportBindingActionBind, ChannelID: &channel.Id,
	}})

	require.NoError(t, err)
}

func TestConfigImportBindingAllowsSkipWithoutReason(t *testing.T) {
	prepareConfigImportBindingDB(t)
	batch := createConfigImportBindingBatch(t, configImportBindingLineFixture{
		lineRef: "line-openai", channelRef: "supplier-openai", channelType: constant.ChannelTypeOpenAI,
		models: []string{"gpt-test"},
	})

	_, err := UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "line-openai", Action: types.ConfigImportBindingActionSkip,
	}})

	require.NoError(t, err)
	var binding model.ConfigImportBinding
	require.NoError(t, model.DB.Where("batch_id = ? AND line_ref = ?", batch.ID, "line-openai").First(&binding).Error)
	assert.Equal(t, string(types.ConfigImportBindingActionSkip), binding.Action)
	assert.Nil(t, binding.ChannelID)

	var items []model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ?", batch.ID).Order("entity_type ASC").Find(&items).Error)
	for _, item := range items {
		if item.EntityType == "channels" || item.EntityType == "model_skus" {
			assert.Equal(t, string(types.ConfigImportItemStateNew), item.State)
			continue
		}
		assert.Equal(t, string(types.ConfigImportItemStateExcluded), item.State)
		assert.Empty(t, item.ExclusionReason)
	}
}

func TestConfigImportBindingBindRestoresItemsSkippedByEarlierDecision(t *testing.T) {
	prepareConfigImportBindingDB(t)
	batch := createConfigImportBindingBatch(t, configImportBindingLineFixture{
		lineRef: "line-openai", channelRef: "supplier-openai", channelType: constant.ChannelTypeOpenAI,
		models: []string{"gpt-test"},
	})
	_, err := UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "line-openai", Action: types.ConfigImportBindingActionSkip,
	}})
	require.NoError(t, err)

	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "Existing supplier", Models: "gpt-test", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	_, err = UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "line-openai", Action: types.ConfigImportBindingActionBind, ChannelID: &channel.Id,
	}})
	require.NoError(t, err)

	var items []model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ?", batch.ID).Find(&items).Error)
	for _, item := range items {
		assert.Equal(t, string(types.ConfigImportItemStateNew), item.State)
		assert.Empty(t, item.ExclusionReason)
	}
}

func TestConfigImportBindingKeepsOverlappingDependentExcludedUntilEveryOwnerBinds(t *testing.T) {
	prepareConfigImportBindingDB(t)
	batch := createConfigImportBindingBatch(t,
		configImportBindingLineFixture{lineRef: "line-a", channelRef: "supplier", channelType: constant.ChannelTypeOpenAI, models: []string{"model-a"}},
		configImportBindingLineFixture{lineRef: "line-b", channelRef: "supplier", channelType: constant.ChannelTypeOpenAI, models: []string{"model-b"}},
	)
	persistConfigImportBindingItem(t, batch.ID, "route_blueprints", "route-a-b", types.ConfigImportRouteBlueprint{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "route-a-b"},
		Targets: []types.ConfigImportRouteTarget{
			{RouteTargetRef: "target-a", LineRef: "line-a", UpstreamModel: "model-a", SKURef: "line-a-sku-a", CostVariantKey: "default"},
			{RouteTargetRef: "target-b", LineRef: "line-b", UpstreamModel: "model-b", SKURef: "line-b-sku-a", CostVariantKey: "default"},
		},
	})

	_, err := UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "line-a", Action: types.ConfigImportBindingActionSkip,
	}})
	require.NoError(t, err)
	_, err = UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "line-b", Action: types.ConfigImportBindingActionSkip,
	}})
	require.NoError(t, err)

	channelA := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "Supplier A", Models: "model-a", Key: "key-a"}
	require.NoError(t, model.DB.Create(channelA).Error)
	_, err = UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "line-a", Action: types.ConfigImportBindingActionBind, ChannelID: &channelA.Id,
	}})
	require.NoError(t, err)

	var route model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND business_id = ?", batch.ID, "route-a-b").First(&route).Error)
	assert.Equal(t, string(types.ConfigImportItemStateExcluded), route.State)
	assert.Empty(t, route.ExclusionReason)

	channelB := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "Supplier B", Models: "model-b", Key: "key-b"}
	require.NoError(t, model.DB.Create(channelB).Error)
	_, err = UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "line-b", Action: types.ConfigImportBindingActionBind, ChannelID: &channelB.Id,
	}})
	require.NoError(t, err)
	require.NoError(t, model.DB.Where("batch_id = ? AND business_id = ?", batch.ID, "route-a-b").First(&route).Error)
	assert.Equal(t, string(types.ConfigImportItemStateNew), route.State)
	assert.Empty(t, route.ExclusionReason)
}

func TestConfigImportBindingSingleRequestReappliesSkipAfterEarlierBind(t *testing.T) {
	prepareConfigImportBindingDB(t)
	batch := createConfigImportBindingBatch(t,
		configImportBindingLineFixture{lineRef: "line-a", channelRef: "supplier", channelType: constant.ChannelTypeOpenAI, models: []string{"model-a"}},
		configImportBindingLineFixture{lineRef: "line-b", channelRef: "supplier", channelType: constant.ChannelTypeOpenAI, models: []string{"model-b"}},
	)
	persistConfigImportBindingItem(t, batch.ID, "route_blueprints", "route-a-b", types.ConfigImportRouteBlueprint{
		ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: "route-a-b"},
		Targets: []types.ConfigImportRouteTarget{
			{RouteTargetRef: "target-a", LineRef: "line-a", UpstreamModel: "model-a", SKURef: "line-a-sku-a", CostVariantKey: "default"},
			{RouteTargetRef: "target-b", LineRef: "line-b", UpstreamModel: "model-b", SKURef: "line-b-sku-a", CostVariantKey: "default"},
		},
	})
	_, err := UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "line-a", Action: types.ConfigImportBindingActionSkip,
	}})
	require.NoError(t, err)

	channelA := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "Supplier A", Models: "model-a", Key: "key-a"}
	require.NoError(t, model.DB.Create(channelA).Error)
	_, err = UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{
		{LineRef: "line-a", Action: types.ConfigImportBindingActionBind, ChannelID: &channelA.Id},
		{LineRef: "line-b", Action: types.ConfigImportBindingActionSkip},
	})
	require.NoError(t, err)

	var route model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND business_id = ?", batch.ID, "route-a-b").First(&route).Error)
	assert.Equal(t, string(types.ConfigImportItemStateExcluded), route.State)
	assert.Empty(t, route.ExclusionReason)
}

func TestConfigImportBindingAllowsOneChannelAcrossMultipleLines(t *testing.T) {
	prepareConfigImportBindingDB(t)
	batch := createConfigImportBindingBatch(t,
		configImportBindingLineFixture{lineRef: "line-one", channelRef: "supplier", channelType: constant.ChannelTypeOpenAI, models: []string{"gpt-test"}},
		configImportBindingLineFixture{lineRef: "line-two", channelRef: "supplier", channelType: constant.ChannelTypeOpenAI, models: []string{"gpt-test"}},
	)
	channelID := 17
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		return saveConfigImportBinding(tx, batch.ID, dto.ConfigImportBindingInput{
			LineRef: "line-one", Action: types.ConfigImportBindingActionBind, ChannelID: &channelID,
		}, 42)
	}))
	require.NoError(t, model.DB.Transaction(func(tx *gorm.DB) error {
		return saveConfigImportBinding(tx, batch.ID, dto.ConfigImportBindingInput{
			LineRef: "line-two", Action: types.ConfigImportBindingActionBind, ChannelID: &channelID,
		}, 42)
	}))
	var count int64
	require.NoError(t, model.DB.Model(&model.ConfigImportBinding{}).Where("batch_id = ? AND channel_id = ?", batch.ID, channelID).Count(&count).Error)
	assert.Equal(t, int64(2), count)
}

func TestConfigImportBindingRejectsProviderTypeButAllowsSnapshotModels(t *testing.T) {
	prepareConfigImportBindingDB(t)
	batch := createConfigImportBindingBatch(t, configImportBindingLineFixture{
		lineRef: "line-openai", channelRef: "supplier-openai", channelType: constant.ChannelTypeOpenAI,
		models: []string{"gpt-test"},
	})

	wrongType := &model.Channel{Type: constant.ChannelTypeGemini, Name: "Gemini supplier", Models: "gpt-test", Key: "key"}
	require.NoError(t, model.DB.Create(wrongType).Error)
	_, err := UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "line-openai", Action: types.ConfigImportBindingActionBind, ChannelID: &wrongType.Id,
	}})
	require.ErrorContains(t, err, "BINDING_CHANNEL_TYPE")

	wrongModel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "Wrong model supplier", Models: "other-model", Key: "key"}
	require.NoError(t, model.DB.Create(wrongModel).Error)
	_, err = UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "line-openai", Action: types.ConfigImportBindingActionBind, ChannelID: &wrongModel.Id,
	}})
	require.NoError(t, err)
}

func TestConfigImportBindingCreateRequiresDisabledChannelAndRecordsConfirmation(t *testing.T) {
	prepareConfigImportBindingDB(t)
	batch := createConfigImportBindingBatch(t, configImportBindingLineFixture{
		lineRef: "line-openai", channelRef: "supplier-openai", channelType: constant.ChannelTypeOpenAI,
		models: []string{"gpt-test"},
	})
	enabled := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "Enabled supplier", Models: "gpt-test", Key: "key", Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(enabled).Error)
	_, err := UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "line-openai", Action: types.ConfigImportBindingActionCreate, ChannelID: &enabled.Id,
	}})
	require.ErrorContains(t, err, "BINDING_NEW_CHANNEL_STATUS")

	disabled := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "Disabled supplier", Models: "gpt-test", Key: "key", Status: common.ChannelStatusManuallyDisabled}
	require.NoError(t, model.DB.Create(disabled).Error)
	_, err = UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "line-openai", Action: types.ConfigImportBindingActionCreate, ChannelID: &disabled.Id, CredentialsConfirmed: true,
	}})
	require.NoError(t, err)

	var binding model.ConfigImportBinding
	require.NoError(t, model.DB.Where("batch_id = ? AND line_ref = ?", batch.ID, "line-openai").First(&binding).Error)
	assert.Equal(t, string(types.ConfigImportBindingActionCreate), binding.Action)
	require.NotNil(t, binding.ChannelID)
	assert.Equal(t, disabled.Id, *binding.ChannelID)
	assert.Equal(t, 42, binding.CredentialsConfirmedBy)
	require.NotNil(t, binding.CredentialsConfirmedAt)
}

func TestConfigImportBindingCreateDefersModelReplacementUntilPublish(t *testing.T) {
	prepareConfigImportBindingDB(t)
	batch := createConfigImportBindingBatch(t, configImportBindingLineFixture{
		lineRef: "line-openai", channelRef: "supplier-openai", channelType: constant.ChannelTypeOpenAI,
		models: []string{"gpt-test"},
	})
	channel := &model.Channel{
		Type: constant.ChannelTypeOpenAI, Name: "New OpenAI supplier", Models: "existing-model",
		Group: "default", Key: "new-key", Status: common.ChannelStatusManuallyDisabled,
	}
	require.NoError(t, model.DB.Create(channel).Error)

	_, err := UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "line-openai", Action: types.ConfigImportBindingActionCreate, ChannelID: &channel.Id, CredentialsConfirmed: true,
	}})

	require.NoError(t, err)
	require.NoError(t, model.DB.First(channel, channel.Id).Error)
	assert.Equal(t, "existing-model", channel.Models)
	var abilityCount int64
	require.NoError(t, model.DB.Model(&model.Ability{}).Where(&model.Ability{
		ChannelId: channel.Id,
		Group:     "default",
		Model:     "gpt-test",
	}).Count(&abilityCount).Error)
	assert.Zero(t, abilityCount)
}

func TestConfigImportBindingRecoversUnpersistedCreatedChannel(t *testing.T) {
	prepareConfigImportBindingDB(t)
	batch := createConfigImportBindingBatch(t, configImportBindingLineFixture{
		lineRef: "line-openai", channelRef: "supplier-openai", channelType: constant.ChannelTypeOpenAI,
		models: []string{"gpt-test"},
	})
	channel := &model.Channel{
		Type: constant.ChannelTypeOpenAI, Name: "test", Models: "existing-model",
		Group: "default", Key: "new-key", Status: common.ChannelStatusManuallyDisabled,
		CreatedTime: batch.CreatedAt + 1,
	}
	require.NoError(t, model.DB.Create(channel).Error)

	_, err := UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{
		{LineRef: "line-openai", Action: types.ConfigImportBindingActionBind, ChannelID: &channel.Id, CredentialsConfirmed: true},
	})

	require.NoError(t, err)
	require.NoError(t, model.DB.First(channel, channel.Id).Error)
	assert.Equal(t, "existing-model", channel.Models)
	var binding model.ConfigImportBinding
	require.NoError(t, model.DB.Where("batch_id = ? AND line_ref = ?", batch.ID, "line-openai").First(&binding).Error)
	assert.Equal(t, string(types.ConfigImportBindingActionCreate), binding.Action)
}

func TestConfigImportBindingDoesNotRecoverExistingDisabledChannel(t *testing.T) {
	prepareConfigImportBindingDB(t)
	batch := createConfigImportBindingBatch(t, configImportBindingLineFixture{
		lineRef: "line-openai", channelRef: "supplier-openai", channelType: constant.ChannelTypeOpenAI,
		models: []string{"gpt-test"},
	})
	channel := &model.Channel{
		Type: constant.ChannelTypeOpenAI, Name: "test", Models: "existing-model",
		Group: "default", Key: "existing-key", Status: common.ChannelStatusManuallyDisabled,
		CreatedTime: batch.CreatedAt - 1,
	}
	require.NoError(t, model.DB.Create(channel).Error)

	_, err := UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{
		{LineRef: "line-openai", Action: types.ConfigImportBindingActionBind, ChannelID: &channel.Id, CredentialsConfirmed: true},
	})

	require.NoError(t, err)
	require.NoError(t, model.DB.First(channel, channel.Id).Error)
	assert.Equal(t, "existing-model", channel.Models)
}

func TestConfigImportBindingSeparatesSecureLineGroups(t *testing.T) {
	prepareConfigImportBindingDB(t)
	batch := createConfigImportBindingBatch(t,
		configImportBindingLineFixture{lineRef: "secure-discount", channelRef: "secure", channelType: constant.ChannelTypeSecure, models: []string{"video-2.0-pro"}},
		configImportBindingLineFixture{lineRef: "secure-overseas", channelRef: "secure", channelType: constant.ChannelTypeSecure, models: []string{"video-2.0-pro"}},
	)
	channel := &model.Channel{Type: constant.ChannelTypeSecure, Name: "Secure discount", Models: "video-2.0-pro", Key: "key"}
	channel.SetOtherSettings(relaydto.ChannelOtherSettings{SecureVideoGroup: relaydto.SecureVideoGroupDiscount})
	require.NoError(t, model.DB.Create(channel).Error)

	_, err := UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "secure-discount", Action: types.ConfigImportBindingActionBind, ChannelID: &channel.Id,
	}})
	require.NoError(t, err)
	_, err = UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "secure-overseas", Action: types.ConfigImportBindingActionBind, ChannelID: &channel.Id,
	}})
	require.ErrorContains(t, err, "BINDING_CHANNEL_LINE_CONFLICT")

	wrongGroup := &model.Channel{Type: constant.ChannelTypeSecure, Name: "Secure wrong group", Models: "video-2.0-pro", Key: "key"}
	wrongGroup.SetOtherSettings(relaydto.ChannelOtherSettings{SecureVideoGroup: relaydto.SecureVideoGroupEnterprise})
	require.NoError(t, model.DB.Create(wrongGroup).Error)
	_, err = UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "secure-overseas", Action: types.ConfigImportBindingActionBind, ChannelID: &wrongGroup.Id,
	}})
	require.ErrorContains(t, err, "BINDING_LINE_CAPABILITY")
}

func TestConfigImportBindingSeparatesMegaByAIFastRealPersonLines(t *testing.T) {
	prepareConfigImportBindingDB(t)
	realPerson := true
	noRealPerson := false
	batch := createConfigImportBindingBatch(t,
		configImportBindingLineFixture{lineRef: "megabyai-fast-real-person", channelRef: "megabyai", channelType: constant.ChannelTypeMegaByAI, models: []string{"videos-fast"}, supportsRealPerson: &realPerson},
		configImportBindingLineFixture{lineRef: "megabyai-fast-no-real-person", channelRef: "megabyai", channelType: constant.ChannelTypeMegaByAI, models: []string{"videos-fast"}, supportsRealPerson: &noRealPerson},
	)
	channel := &model.Channel{Type: constant.ChannelTypeMegaByAI, Name: "MegaByAI real person", Models: "videos-fast", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)

	_, err := UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "megabyai-fast-real-person", Action: types.ConfigImportBindingActionBind, ChannelID: &channel.Id, CredentialsConfirmed: true,
	}})
	require.NoError(t, err)
	_, err = UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "megabyai-fast-no-real-person", Action: types.ConfigImportBindingActionBind, ChannelID: &channel.Id, CredentialsConfirmed: true,
	}})
	require.ErrorContains(t, err, "BINDING_CHANNEL_LINE_CONFLICT")
}

func TestConfigImportBindingStrictDecodeRejectsCredentialFields(t *testing.T) {
	for _, field := range []string{"key", "api_key", "secret"} {
		t.Run(field, func(t *testing.T) {
			_, err := DecodeConfigImportBindingRequest(strings.NewReader(`{"bindings":[{"line_ref":"line-openai","action":"bind","` + field + `":"forbidden"}]}`))
			require.Error(t, err)
			assert.Contains(t, err.Error(), field)
		})
	}
}

func TestConfigImportBindingStrictDecodeRejectsRemovedSkipReason(t *testing.T) {
	_, err := DecodeConfigImportBindingRequest(strings.NewReader(`{"bindings":[{"line_ref":"line-openai","action":"skip","reason":"legacy"}]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reason")
}

func prepareConfigImportBindingDB(t *testing.T) {
	t.Helper()
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.Ability{}))
}

func createConfigImportBindingBatch(t *testing.T, lines ...configImportBindingLineFixture) model.ConfigImportBatch {
	t.Helper()
	summaryJSON, err := common.Marshal(configImportBatchSummaryStorage{})
	require.NoError(t, err)
	batch := model.ConfigImportBatch{
		SchemaVersion: 1, TemplateVersion: "test", SourceSHA256: strings.Repeat("a", 64),
		PayloadSHA256: strings.Repeat("b", 64), Status: string(types.ConfigImportBatchStatusBinding),
		CreatedBy: 42, SummaryJSON: model.ConfigImportSummaryJSON(summaryJSON), BaselineJSON: "{}",
	}
	require.NoError(t, model.DB.Create(&batch).Error)

	masters := make(map[string]int)
	for _, fixture := range lines {
		if _, exists := masters[fixture.channelRef]; exists {
			continue
		}
		masters[fixture.channelRef] = fixture.channelType
	}
	for channelRef, channelType := range masters {
		persistConfigImportBindingItem(t, batch.ID, "channels", channelRef, types.ConfigImportChannel{
			ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: channelRef},
			ChannelType:                     &channelType,
		})
	}
	for _, fixture := range lines {
		persistConfigImportBindingItem(t, batch.ID, "channel_lines", fixture.lineRef, types.ConfigImportChannelLine{
			ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: fixture.lineRef},
			LineRef:                         fixture.lineRef, ChannelRef: fixture.channelRef, DisplayName: fixture.lineRef,
			ProviderTypeHint: "test", Region: "test", Protocol: "test", StatusProposal: "disabled",
			SupportsRealPerson: fixture.supportsRealPerson,
		})
		for index, upstreamModel := range fixture.models {
			skuRef := fixture.lineRef + "-sku-" + string(rune('a'+index))
			persistConfigImportBindingItem(t, batch.ID, "model_skus", skuRef, types.ConfigImportModelSKU{
				ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: skuRef},
			})
			persistConfigImportBindingItem(t, batch.ID, "model_mappings", fixture.lineRef+"-mapping-"+string(rune('a'+index)), types.ConfigImportModelMapping{
				ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: fixture.lineRef + "-mapping-" + string(rune('a'+index))},
				CanonicalModel:                  upstreamModel,
				ClientModel:                     upstreamModel,
				LineRef:                         fixture.lineRef,
				UpstreamModel:                   upstreamModel,
				SKURef:                          skuRef,
			})
		}
	}
	return batch
}

func persistConfigImportBindingItem(t *testing.T, batchID int64, entityType, businessID string, value any) {
	t.Helper()
	canonicalJSON, err := common.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.ConfigImportItem{
		BatchID: batchID, EntityType: entityType, BusinessID: businessID, EntityHash: strings.Repeat("c", 64),
		CanonicalJSON: string(canonicalJSON), State: string(types.ConfigImportItemStateNew),
	}).Error)
}
