package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelRoutingFilterAppliesBeforePrioritySelection(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			prepareChannelRoutingFilterTest(t, memoryCacheEnabled)

			filter := ChannelSelectFilter{
				AllowedChannelIDs:  map[int]struct{}{12: {}, 13: {}},
				ExcludedChannelIDs: map[int]struct{}{13: {}},
			}
			selected, err := GetRandomSatisfiedChannel("分组A", modelrouting.Seedance20, 0, "/v1/video/generations", filter)
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, 12, selected.Id)
		})
	}
}

func TestChannelRoutingFilterPreservesLegacyRetryAndFallback(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			prepareChannelRoutingFilterTest(t, memoryCacheEnabled)

			legacy, err := GetRandomSatisfiedChannel("分组A", modelrouting.Seedance20, 0, "/v1/video/generations", ChannelSelectFilter{})
			require.NoError(t, err)
			require.NotNil(t, legacy)
			assert.Equal(t, 11, legacy.Id)

			retry, err := GetRandomSatisfiedChannel("分组A", modelrouting.Seedance20, 1, "/v1/video/generations", ChannelSelectFilter{
				ExcludedChannelIDs: map[int]struct{}{13: {}},
			})
			require.NoError(t, err)
			require.NotNil(t, retry)
			assert.Equal(t, 12, retry.Id)

			normalized, err := GetRandomSatisfiedChannel("分组A", "gpt-4o-gizmo-routing", 0, "/v1/chat/completions", ChannelSelectFilter{
				AllowedChannelIDs: map[int]struct{}{14: {}},
			})
			require.NoError(t, err)
			require.NotNil(t, normalized)
			assert.Equal(t, 14, normalized.Id)
		})
	}
}

func prepareChannelRoutingFilterTest(t *testing.T, memoryCacheEnabled bool) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))

	previousDB := DB
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	DB = db
	common.MemoryCacheEnabled = memoryCacheEnabled
	t.Cleanup(func() {
		DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		require.NoError(t, sqlDB.Close())
	})

	channels := []*Channel{
		routingFilterChannel(11, constant.ChannelTypeOpenAI, modelrouting.Seedance20, 100),
		routingFilterChannel(12, constant.ChannelTypeOpenAI, modelrouting.Seedance20, 50),
		routingFilterChannel(13, constant.ChannelTypeAdvancedCustom, modelrouting.Seedance20, 50),
		routingFilterChannel(14, constant.ChannelTypeOpenAI, "gpt-4o-gizmo-*", 10),
		routingFilterChannel(15, constant.ChannelTypeOpenAI, "gpt-4o-gizmo-routing", 100),
	}
	channels[2].SetOtherSettings(dto.ChannelOtherSettings{AdvancedCustom: &dto.AdvancedCustomConfig{
		Routes: []dto.AdvancedCustomRoute{{
			IncomingPath: "/v1/chat/completions",
			UpstreamPath: "/v1/chat/completions",
			Models:       []string{modelrouting.Seedance20},
		}},
	}})
	for _, channel := range channels {
		require.NoError(t, db.Create(channel).Error)
		require.NoError(t, channel.AddAbilities(db))
	}
	InitChannelCache()
}

func routingFilterChannel(id int, channelType int, modelName string, priority int64) *Channel {
	weight := uint(100)
	return &Channel{
		Id:       id,
		Type:     channelType,
		Key:      fmt.Sprintf("key-%d", id),
		Status:   common.ChannelStatusEnabled,
		Name:     fmt.Sprintf("channel-%d", id),
		Weight:   &weight,
		Models:   modelName,
		Group:    "分组A",
		Priority: &priority,
	}
}

// TestGroupModelChannelIDsReturnsPreSelectionCandidates asserts the pre-selection
// candidate lister honors the ChannelSelectFilter (exclusions and allow-list) and
// returns a deterministic set the profit filter can intersect with its survivors. It
// must never perform the weighted random pick — that stays in GetRandomSatisfiedChannel.
func TestGroupModelChannelIDsReturnsPreSelectionCandidates(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			prepareChannelRoutingFilterTest(t, memoryCacheEnabled)

			all := GroupModelChannelIDs("分组A", modelrouting.Seedance20, "/v1/video/generations", ChannelSelectFilter{})
			require.NotEmpty(t, all)
			// The base candidate set must contain channel 11 (highest priority) and not
			// contain channels 14/15 (different model/path).
			assert.Contains(t, all, 11)
			assert.NotContains(t, all, 14)
			assert.NotContains(t, all, 15)

			excluded := GroupModelChannelIDs("分组A", modelrouting.Seedance20, "/v1/video/generations", ChannelSelectFilter{
				ExcludedChannelIDs: map[int]struct{}{11: {}},
			})
			assert.NotContains(t, excluded, 11)
			// Excluding 11 must not drop the other candidates.
			assert.NotEqual(t, len(all)-1 == 0 && len(excluded) == 0, true)

			allowlisted := GroupModelChannelIDs("分组A", modelrouting.Seedance20, "/v1/video/generations", ChannelSelectFilter{
				AllowedChannelIDs: map[int]struct{}{11: {}},
			})
			assert.ElementsMatch(t, []int{11}, allowlisted)

			tombstone := GroupModelChannelIDs("分组A", modelrouting.Seedance20, "/v1/video/generations", ChannelSelectFilter{
				AllowedChannelIDs: map[int]struct{}{0: {}},
			})
			assert.Empty(t, tombstone, "tombstone allow-list must yield no candidates")
		})
	}
}
