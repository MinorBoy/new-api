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

func TestListSatisfiedChannelsMatchesWithAndWithoutMemoryCache(t *testing.T) {
	var want []SatisfiedChannel
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			prepareChannelSelectionTest(t, memoryCacheEnabled)

			candidates, err := ListSatisfiedChannels(
				"分组A", modelrouting.Seedance20, "", ChannelSelectFilter{},
			)
			require.NoError(t, err)
			require.Len(t, candidates, 3)
			ids := satisfiedChannelIDs(candidates)
			assert.Equal(t, []int{11, 12, 13}, ids)
			assert.Equal(t, []int64{100, 50, 50}, satisfiedChannelPriorities(candidates))

			if want == nil {
				want = candidates
				return
			}
			assert.Equal(t, satisfiedChannelIDs(want), ids)
			assert.Equal(t, satisfiedChannelPriorities(want), satisfiedChannelPriorities(candidates))
		})
	}
}

func TestListSatisfiedChannelsUsesExactModelBeforeNormalizedFallback(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			prepareChannelSelectionTest(t, memoryCacheEnabled)

			// The exact model has a matching channel, so the wildcard/normalized
			// fallback must not be merged into the result.
			candidates, err := ListSatisfiedChannels(
				"分组A", "gpt-4o-gizmo-routing", "/v1/chat/completions", ChannelSelectFilter{},
			)
			require.NoError(t, err)
			assert.Equal(t, []int{15}, satisfiedChannelIDs(candidates))

			// With no exact model match, FormatMatchingModelName fallback is used.
			candidates, err = ListSatisfiedChannels(
				"分组A", "gpt-4o-gizmo-other", "/v1/chat/completions", ChannelSelectFilter{},
			)
			require.NoError(t, err)
			assert.Equal(t, []int{14}, satisfiedChannelIDs(candidates))
		})
	}
}

func TestListSatisfiedChannelsAppliesPathAndChannelFilters(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			prepareChannelSelectionTest(t, memoryCacheEnabled)

			// Advanced Custom channel 13 only advertises chat in this fixture and
			// must not leak into the image/video path candidate set.
			candidates, err := ListSatisfiedChannels(
				"分组A", modelrouting.Seedance20, "/v1/video/generations", ChannelSelectFilter{
					ExcludedChannelIDs: map[int]struct{}{11: {}, 13: {}},
				},
			)
			require.NoError(t, err)
			assert.Equal(t, []int{12}, satisfiedChannelIDs(candidates))

			candidates, err = ListSatisfiedChannels(
				"分组A", modelrouting.Seedance20, "/v1/video/generations", ChannelSelectFilter{
					AllowedChannelIDs: map[int]struct{}{13: {}},
				},
			)
			require.NoError(t, err)
			assert.Empty(t, candidates)
		})
	}
}

func TestListSatisfiedChannelsSkipsDisabledChannelsOnDBAndCachePaths(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			prepareChannelSelectionTest(t, memoryCacheEnabled)

			disabled := selectionTestChannel(16, constant.ChannelTypeOpenAI, modelrouting.Seedance20, 1000)
			require.NoError(t, DB.Create(disabled).Error)
			require.NoError(t, disabled.AddAbilities(DB))
			require.NoError(t, DB.Model(&Channel{}).Where("id = ?", disabled.Id).Update("status", common.ChannelStatusManuallyDisabled).Error)
			if memoryCacheEnabled {
				InitChannelCache()
			}

			candidates, err := ListSatisfiedChannels("分组A", modelrouting.Seedance20, "", ChannelSelectFilter{})
			require.NoError(t, err)
			assert.Equal(t, []int{11, 12, 13}, satisfiedChannelIDs(candidates))
		})
	}
}

func TestSelectManualChannelPreservesPriorityRetryLayers(t *testing.T) {
	highPriority := int64(100)
	lowPriority := int64(10)
	highWeight := uint(0)
	lowWeight := uint(0)
	high := &Channel{Id: 21, Priority: &highPriority, Weight: &highWeight}
	low := &Channel{Id: 22, Priority: &lowPriority, Weight: &lowWeight}
	candidates := []SatisfiedChannel{
		{Channel: low, Priority: lowPriority, Weight: 0},
		{Channel: high, Priority: highPriority, Weight: 0},
	}

	assert.Same(t, high, SelectManualChannel(candidates, 0))
	assert.Same(t, low, SelectManualChannel(candidates, 1))
	assert.Same(t, low, SelectManualChannel(candidates, 99))
	assert.Nil(t, SelectManualChannel(nil, 0))
}

func satisfiedChannelIDs(candidates []SatisfiedChannel) []int {
	ids := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Channel != nil {
			ids = append(ids, candidate.Channel.Id)
		}
	}
	return ids
}

func satisfiedChannelPriorities(candidates []SatisfiedChannel) []int64 {
	priorities := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		priorities = append(priorities, candidate.Priority)
	}
	return priorities
}

func prepareChannelSelectionTest(t *testing.T, memoryCacheEnabled bool) {
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
		selectionTestChannel(11, constant.ChannelTypeOpenAI, modelrouting.Seedance20, 100),
		selectionTestChannel(12, constant.ChannelTypeOpenAI, modelrouting.Seedance20, 50),
		selectionTestChannel(13, constant.ChannelTypeAdvancedCustom, modelrouting.Seedance20, 50),
		selectionTestChannel(14, constant.ChannelTypeOpenAI, "gpt-4o-gizmo-*", 10),
		selectionTestChannel(15, constant.ChannelTypeOpenAI, "gpt-4o-gizmo-routing", 100),
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

func selectionTestChannel(id int, channelType int, modelName string, priority int64) *Channel {
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
