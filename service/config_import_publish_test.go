package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublishConfigImportBatchRejectsStaleBaselineWithoutActivatingDraft(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}, &model.ConfigImportPublishAudit{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")
	_, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)

	var item model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "cost_rule_drafts").First(&item).Error)
	var rule model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&rule, *item.MaterializedID).Error)
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{ChannelID: channel.Id, BillableUpstreamModel: "unrelated-model", CostVariantKey: "default", Version: 1, Status: string(types.CostRuleActive), CostMode: "per_request", SchemaVersion: 1, ConfigJSON: `{}`}).Error)

	err = PublishConfigImportBatch(context.Background(), batch.ID, 42)
	var schemaErr *ConfigImportSchemaError
	require.ErrorAs(t, err, &schemaErr)
	assert.Equal(t, "STALE_BASE_VERSION", schemaErr.Code)
	var loaded model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&loaded, rule.ID).Error)
	assert.Equal(t, string(types.CostRuleDraft), loaded.Status)
	var loadedBatch model.ConfigImportBatch
	require.NoError(t, model.DB.First(&loadedBatch, batch.ID).Error)
	assert.Equal(t, string(types.ConfigImportBatchStatusReady), loadedBatch.Status)
}

func TestPublishConfigImportBatchActivatesDraftAndAudits(t *testing.T) {
	prepareConfigImportServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}, &model.ConfigImportPublishAudit{}))
	channel := &model.Channel{Type: 1, Name: "supplier", Models: "vendor-video", Key: "key"}
	require.NoError(t, model.DB.Create(channel).Error)
	batch := createConfigImportStageBatch(t, channel.Id, "line-a", "vendor-video")
	_, err := StageConfigImportBatch(context.Background(), 42, batch.ID)
	require.NoError(t, err)
	err = PublishConfigImportBatch(context.Background(), batch.ID, 42)
	require.NoError(t, err)

	var item model.ConfigImportItem
	require.NoError(t, model.DB.Where("batch_id = ? AND entity_type = ?", batch.ID, "cost_rule_drafts").First(&item).Error)
	var rule model.ChannelModelCostRule
	require.NoError(t, model.DB.First(&rule, *item.MaterializedID).Error)
	assert.Equal(t, string(types.CostRuleActive), rule.Status)
	var loadedBatch model.ConfigImportBatch
	require.NoError(t, model.DB.First(&loadedBatch, batch.ID).Error)
	assert.Equal(t, string(types.ConfigImportBatchStatusPublished), loadedBatch.Status)
	var audit model.ConfigImportPublishAudit
	require.NoError(t, model.DB.Where("batch_id = ?", batch.ID).First(&audit).Error)
	assert.Equal(t, "published", audit.Outcome)
	assert.NotEmpty(t, audit.BeforeSHA256)
	_ = common.GetTimestamp()
}
