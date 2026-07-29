package e2e

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigImportV1FixtureBlocksUnverifiedReferenceLimitsE2E(t *testing.T) {
	setupSeedanceE2EDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.ConfigImportBatch{},
		&model.ConfigImportItem{},
		&model.ConfigImportBinding{},
		&model.ConfigImportIssue{},
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
		CostRuleDrafts: 98, ModelMappings: 104, RouteBlueprints: 98,
		Sources: 13, UnresolvedVariants: 1,
	}, first.ItemCounts)
	assert.Equal(t, types.ConfigImportBatchStatusBlocked, first.Status)
	assert.Empty(t, first.AllowedActions)
	unresolvedLimitBusinessIDs := make(map[string]struct{})
	for _, issue := range first.Issues {
		if issue.Code == "ROUTE_REFERENCE_LIMITS_UNRESOLVED" {
			assert.Equal(t, types.ConfigImportIssueSeverityError, issue.Severity)
			assert.Equal(t, "model_mappings", issue.EntityType)
			unresolvedLimitBusinessIDs[issue.BusinessID] = struct{}{}
		}
	}
	assert.Equal(t,
		map[string]struct{}{
			"MAP-8YES-R60-480":  {},
			"MAP-8YES-R60-720":  {},
			"MAP-8YES-R61-1080": {},
			"MAP-8YES-R62-480":  {},
			"MAP-8YES-R63-4K":   {},
			"MAP-8YES-R64-720":  {},
		},
		unresolvedLimitBusinessIDs,
	)
	mappingFound := false
	for _, mapping := range document.Entities.ModelMappings {
		if mapping.BusinessID == "MAP-8YES-R60-480" {
			mappingFound = true
			break
		}
	}
	assert.True(t, mappingFound)
	for _, draft := range document.Entities.CostRuleDrafts {
		assert.NotEqual(t, "route-target/MAP-8YES-R60-480", draft.RouteTargetRef)
	}
	for _, blueprint := range document.Entities.RouteBlueprints {
		assert.NotEqual(t, "route-blueprint/MAP-8YES-R60-480", blueprint.BusinessID)
	}

	duplicate, created, err := service.CreateConfigImportBatch(context.Background(), 1, bytes.NewReader(payload))
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.ID, duplicate.ID)
}
