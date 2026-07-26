package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// prepareCostVariantDB mirrors prepareCostAccountingDB but also migrates the
// routing-policy tables so route-target variant backfill can be exercised.
func prepareCostVariantDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	previousDB := DB
	DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB = previousDB
		require.NoError(t, sqlDB.Close())
	})

	require.NoError(t, db.AutoMigrate(
		&ChannelModelCostRule{},
		&RoutingPolicy{},
		&RouteTarget{},
		&CostAccountingRequest{},
		&CostAccountingAttempt{},
		&CostAccountingAudit{},
	))
}

func TestCostVariantDraftsAreScopedByVariantKey(t *testing.T) {
	prepareCostVariantDB(t)

	require.NoError(t, CreateCostRuleDraft(&ChannelModelCostRule{
		ChannelID: 7, BillableUpstreamModel: "videos-fast", CostVariantKey: "480p", Version: 1,
		Status: string(types.CostRuleDraft),
	}))
	require.NoError(t, CreateCostRuleDraft(&ChannelModelCostRule{
		ChannelID: 7, BillableUpstreamModel: "videos-fast", CostVariantKey: "720p", Version: 1,
		Status: string(types.CostRuleDraft),
	}))

	// A second draft on the same extended business key is rejected.
	dup := ChannelModelCostRule{
		ChannelID: 7, BillableUpstreamModel: "videos-fast", CostVariantKey: "480p", Version: 1,
		Status: string(types.CostRuleDraft),
	}
	require.Error(t, CreateCostRuleDraft(&dup))

	// A new version of the same variant is allowed.
	require.NoError(t, CreateCostRuleDraft(&ChannelModelCostRule{
		ChannelID: 7, BillableUpstreamModel: "videos-fast", CostVariantKey: "480p", Version: 2,
		Status: string(types.CostRuleDraft),
	}))
}

func TestCostVariantBlankNormalizesToDefault(t *testing.T) {
	prepareCostVariantDB(t)

	normalized, err := types.NormalizeCostVariantKey("")
	require.NoError(t, err)
	assert.Equal(t, types.DefaultCostVariantKey, normalized)

	rule := ChannelModelCostRule{
		ChannelID: 7, BillableUpstreamModel: "videos-fast", Version: 1,
		Status: string(types.CostRuleDraft),
	}
	require.NoError(t, CreateCostRuleDraft(&rule))
	require.NoError(t, DB.First(&rule, rule.ID).Error)
	assert.Equal(t, string(types.DefaultCostVariantKey), rule.CostVariantKey)
}

func TestCostVariantActivationIsVariantScoped(t *testing.T) {
	prepareCostVariantDB(t)

	// An active 480p rule exists; activating a 720p draft must not retire it.
	active := ChannelModelCostRule{
		ChannelID: 7, BillableUpstreamModel: "videos-fast", CostVariantKey: "480p", Version: 1,
		Status: string(types.CostRuleActive), EffectiveFrom: costInt64Pointer(100),
	}
	require.NoError(t, DB.Create(&active).Error)

	draft := ChannelModelCostRule{
		ChannelID: 7, BillableUpstreamModel: "videos-fast", CostVariantKey: "720p", Version: 1,
		Status: string(types.CostRuleDraft),
	}
	require.NoError(t, CreateCostRuleDraft(&draft))

	activated, err := ActivateChannelModelCostRule(draft.ID, 9, 200, nil)
	require.NoError(t, err)
	assert.Equal(t, string(types.CostRuleActive), activated.Status)

	require.NoError(t, DB.First(&active, active.ID).Error)
	assert.Equal(t, string(types.CostRuleActive), active.Status,
		"activating a different variant must not retire an unrelated active rule")
}

func TestCostVariantRouteTargetBackfillDefaults(t *testing.T) {
	prepareCostVariantDB(t)

	// A freshly migrated route target that omits the variant still reads back
	// as the default variant, because the model layer normalizes blank input.
	require.NoError(t, func() error {
		_, err := ReplaceRoutingPolicy(0, RoutingPolicy{
			GroupName:         "default",
			Model:             modelrouting.Seedance20Fast,
			Enabled:           false,
			DefaultResolution: "720p",
			DefaultDuration:   5,
			DefaultRatio:      "16:9",
		}, []RouteTarget{{
			ChannelID:     7,
			Name:          "legacy",
			UpstreamModel: "videos-fast",
			Constraints:   `{"output_resolutions":["720p"],"durations":{"min":4,"max":15},"aspect_ratios":["16:9"],"input_modes":["text"],"reference_limits":{"images":0,"videos":0,"audios":0}}`,
			Enabled:       false,
		}})
		return err
	}())

	policy, err := GetRoutingPolicy(1)
	require.NoError(t, err)
	require.Len(t, policy.Targets, 1)
	assert.Equal(t, string(types.DefaultCostVariantKey), policy.Targets[0].CostVariantKey)
}
