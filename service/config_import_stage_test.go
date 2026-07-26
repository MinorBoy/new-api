package service

import (
	"context"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
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

func TestConfigImportBindingSkipExcludesLineDependents(t *testing.T) {
	prepareConfigImportBindingDB(t)
	batch := createConfigImportBindingBatch(t, configImportBindingLineFixture{
		lineRef: "line-openai", channelRef: "supplier-openai", channelType: constant.ChannelTypeOpenAI,
		models: []string{"gpt-test"},
	})

	_, err := UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "line-openai", Action: types.ConfigImportBindingActionSkip, Reason: "supplier retired",
	}})

	require.NoError(t, err)
	var binding model.ConfigImportBinding
	require.NoError(t, model.DB.Where("batch_id = ? AND line_ref = ?", batch.ID, "line-openai").First(&binding).Error)
	assert.Equal(t, string(types.ConfigImportBindingActionSkip), binding.Action)
	assert.Nil(t, binding.ChannelID)

	var items []model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ?", batch.ID).Order("entity_type ASC").Find(&items).Error)
	for _, item := range items {
		if item.EntityType == "channels" {
			assert.Equal(t, string(types.ConfigImportItemStateNew), item.State)
			continue
		}
		assert.Equal(t, string(types.ConfigImportItemStateExcluded), item.State)
		assert.Equal(t, "supplier retired", item.ExclusionReason)
	}
}

func TestConfigImportBindingBindRestoresItemsSkippedByEarlierDecision(t *testing.T) {
	prepareConfigImportBindingDB(t)
	batch := createConfigImportBindingBatch(t, configImportBindingLineFixture{
		lineRef: "line-openai", channelRef: "supplier-openai", channelType: constant.ChannelTypeOpenAI,
		models: []string{"gpt-test"},
	})
	_, err := UpdateConfigImportBindings(context.Background(), 42, batch.ID, []dto.ConfigImportBindingInput{{
		LineRef: "line-openai", Action: types.ConfigImportBindingActionSkip, Reason: "supplier retired",
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

func TestConfigImportBindingMapsDatabaseChannelUniquenessConflict(t *testing.T) {
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
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		return saveConfigImportBinding(tx, batch.ID, dto.ConfigImportBindingInput{
			LineRef: "line-two", Action: types.ConfigImportBindingActionBind, ChannelID: &channelID,
		}, 42)
	})
	require.ErrorContains(t, err, "BINDING_CHANNEL_LINE_CONFLICT")
}

func TestConfigImportBindingRejectsProviderTypeAndModelMismatches(t *testing.T) {
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
	require.ErrorContains(t, err, "BINDING_CHANNEL_MODEL")
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

func TestConfigImportBindingSeparatesSecureLineGroups(t *testing.T) {
	prepareConfigImportBindingDB(t)
	batch := createConfigImportBindingBatch(t,
		configImportBindingLineFixture{lineRef: "secure-discount", channelRef: "secure", channelType: constant.ChannelTypeSecure, models: []string{"video-2.0-pro"}},
		configImportBindingLineFixture{lineRef: "secure-overseas", channelRef: "secure", channelType: constant.ChannelTypeSecure, models: []string{"video-2.0-pro"}},
	)
	channel := &model.Channel{Type: constant.ChannelTypeSecure, Name: "Secure discount", Models: "video-2.0-pro", Key: "key"}
	channel.SetOtherSettings(dto.ChannelOtherSettings{SecureVideoGroup: dto.SecureVideoGroupDiscount})
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
	wrongGroup.SetOtherSettings(dto.ChannelOtherSettings{SecureVideoGroup: dto.SecureVideoGroupEnterprise})
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
		CreatedBy: 42, SummaryJSON: string(summaryJSON), BaselineJSON: "{}",
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
			persistConfigImportBindingItem(t, batch.ID, "model_skus", fixture.lineRef+"-sku-"+string(rune('a'+index)), types.ConfigImportModelSKU{
				ConfigImportAuthoritativeEntity: types.ConfigImportAuthoritativeEntity{BusinessID: fixture.lineRef + "-sku-" + string(rune('a'+index))},
				LineRef:                         fixture.lineRef, UpstreamModel: upstreamModel,
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
