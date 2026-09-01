package model

import (
	"fmt"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// SatisfiedChannel is a fully resolved channel candidate before routing policy
// or weighted selection is applied. Priority and Weight are copied from the
// ability/channel snapshot used by the corresponding selection path.
type SatisfiedChannel struct {
	Channel  *Channel
	Priority int64
	Weight   int
}

type ChannelSelectFilter struct {
	AllowedChannelIDs  map[int]struct{}
	ExcludedChannelIDs map[int]struct{}
}

func (f ChannelSelectFilter) Allows(channelID int) bool {
	if len(f.AllowedChannelIDs) > 0 {
		if _, ok := f.AllowedChannelIDs[channelID]; !ok {
			return false
		}
	}
	_, excluded := f.ExcludedChannelIDs[channelID]
	return !excluded
}

func filterChannelIDs(ids []int, filter ChannelSelectFilter) []int {
	if len(ids) == 0 || len(filter.AllowedChannelIDs) == 0 && len(filter.ExcludedChannelIDs) == 0 {
		return ids
	}
	filtered := make([]int, 0, len(ids))
	for _, id := range ids {
		if filter.Allows(id) {
			filtered = append(filtered, id)
		}
	}
	return filtered
}

func IsChannelEnabledForGroupModel(group string, modelName string, channelID int) bool {
	if group == "" || modelName == "" || channelID <= 0 {
		return false
	}
	if !common.MemoryCacheEnabled {
		return isChannelEnabledForGroupModelDB(group, modelName, channelID)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	if group2model2channels == nil {
		return false
	}

	if isChannelIDInList(group2model2channels[group][modelName], channelID) {
		return true
	}
	normalized := ratio_setting.FormatMatchingModelName(modelName)
	if normalized != "" && normalized != modelName {
		return isChannelIDInList(group2model2channels[group][normalized], channelID)
	}
	return false
}

func IsChannelEnabledForAnyGroupModel(groups []string, modelName string, channelID int) bool {
	if len(groups) == 0 {
		return false
	}
	for _, g := range groups {
		if IsChannelEnabledForGroupModel(g, modelName, channelID) {
			return true
		}
	}
	return false
}

func isChannelEnabledForGroupModelDB(group string, modelName string, channelID int) bool {
	var count int64
	err := DB.Model(&Ability{}).
		Where(commonGroupCol+" = ? and model = ? and channel_id = ? and enabled = ?", group, modelName, channelID, true).
		Count(&count).Error
	if err == nil && count > 0 {
		return true
	}
	normalized := ratio_setting.FormatMatchingModelName(modelName)
	if normalized == "" || normalized == modelName {
		return false
	}
	count = 0
	err = DB.Model(&Ability{}).
		Where(commonGroupCol+" = ? and model = ? and channel_id = ? and enabled = ?", group, normalized, channelID, true).
		Count(&count).Error
	return err == nil && count > 0
}

func isChannelIDInList(list []int, channelID int) bool {
	for _, id := range list {
		if id == channelID {
			return true
		}
	}
	return false
}

// GroupModelChannelIDs returns the enabled channel IDs for a group+model that also
// satisfy the request-path filter, WITHOUT performing the weighted random pick. It is
// the pre-selection candidate list the profit-aware routing layer intersects with its
// margin survivors; selection itself still runs through GetRandomSatisfiedChannel.
//
// The function mirrors GetRandomSatisfiedChannel's filtering (request path + model +
// ChannelSelectFilter) so the candidate set the profit filter sees is exactly the set
// the random picker would otherwise choose from. It performs no random selection and
// preserves the original priority order.
func GroupModelChannelIDs(group, modelName, requestPath string, filter ChannelSelectFilter) []int {
	if group == "" || modelName == "" {
		return nil
	}
	if !common.MemoryCacheEnabled {
		return groupModelChannelIDsDB(group, modelName, requestPath, filter)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	if group2model2channels == nil {
		return nil
	}
	channels := filterChannelsByRequestPathAndModel(group2model2channels[group][modelName], requestPath, modelName)
	filtered := filterChannelIDs(channels, filter)
	if len(filtered) > 0 {
		return filtered
	}
	normalized := ratio_setting.FormatMatchingModelName(modelName)
	if normalized == "" || normalized == modelName {
		return nil
	}
	channels = filterChannelsByRequestPathAndModel(group2model2channels[group][normalized], requestPath, modelName)
	return filterChannelIDs(channels, filter)
}

// ListSatisfiedChannels returns all enabled channels that can serve a group,
// model, and request path. It deliberately performs no random selection so
// callers such as cost-aware image routing can inspect and sort the complete
// candidate set. Exact model matches are preferred; the normalized matching
// model is consulted only when the exact candidate set is empty.
func ListSatisfiedChannels(group, modelName, requestPath string, filter ChannelSelectFilter) ([]SatisfiedChannel, error) {
	if group == "" || modelName == "" {
		return nil, nil
	}
	if common.MemoryCacheEnabled {
		return listSatisfiedChannelsCache(group, modelName, requestPath, filter)
	}
	return listSatisfiedChannelsDB(group, modelName, requestPath, filter)
}

func listSatisfiedChannelsCache(group, modelName, requestPath string, filter ChannelSelectFilter) ([]SatisfiedChannel, error) {
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	if group2model2channels == nil {
		return nil, nil
	}
	channels := filterChannelsByRequestPathAndModel(group2model2channels[group][modelName], requestPath, modelName)
	channels = filterChannelIDs(channels, filter)
	if len(channels) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(modelName)
		if normalizedModel == "" || normalizedModel == modelName {
			return nil, nil
		}
		channels = filterChannelsByRequestPathAndModel(group2model2channels[group][normalizedModel], requestPath, modelName)
		channels = filterChannelIDs(channels, filter)
	}
	return satisfiedChannelsFromCacheIDs(channels)
}

func satisfiedChannelsFromCacheIDs(channelIDs []int) ([]SatisfiedChannel, error) {
	if len(channelIDs) == 0 {
		return nil, nil
	}
	candidates := make([]SatisfiedChannel, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		channel, ok := channelsIDM[channelID]
		if !ok {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelID)
		}
		candidates = append(candidates, SatisfiedChannel{
			Channel:  channel,
			Priority: channel.GetPriority(),
			Weight:   channel.GetWeight(),
		})
	}
	sortSatisfiedChannels(candidates)
	return candidates, nil
}

func listSatisfiedChannelsDB(group, modelName, requestPath string, filter ChannelSelectFilter) ([]SatisfiedChannel, error) {
	abilities, err := getSelectableAbilities(group, modelName)
	if err != nil {
		return nil, err
	}
	abilities = filterAbilitiesByRequestPathAndModel(abilities, requestPath, modelName)
	abilities = filterAbilitiesByChannelSelectFilter(abilities, filter)
	if len(abilities) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(modelName)
		if normalizedModel == "" || normalizedModel == modelName {
			return nil, nil
		}
		abilities, err = getSelectableAbilities(group, normalizedModel)
		if err != nil {
			return nil, err
		}
		abilities = filterAbilitiesByRequestPathAndModel(abilities, requestPath, modelName)
		abilities = filterAbilitiesByChannelSelectFilter(abilities, filter)
	}
	if len(abilities) == 0 {
		return nil, nil
	}

	channelIDs := make([]int, 0, len(abilities))
	for _, ability := range abilities {
		channelIDs = append(channelIDs, ability.ChannelId)
	}
	var channels []*Channel
	if err := DB.Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
		return nil, err
	}
	channelByID := make(map[int]*Channel, len(channels))
	for _, channel := range channels {
		channelByID[channel.Id] = channel
	}

	candidates := make([]SatisfiedChannel, 0, len(abilities))
	for _, ability := range abilities {
		channel, ok := channelByID[ability.ChannelId]
		if !ok {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", ability.ChannelId)
		}
		priority := int64(0)
		if ability.Priority != nil {
			priority = *ability.Priority
		}
		candidates = append(candidates, SatisfiedChannel{
			Channel:  channel,
			Priority: priority,
			Weight:   int(ability.Weight),
		})
	}
	sortSatisfiedChannels(candidates)
	return candidates, nil
}

func sortSatisfiedChannels(candidates []SatisfiedChannel) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority > candidates[j].Priority
		}
		leftID, rightID := 0, 0
		if candidates[i].Channel != nil {
			leftID = candidates[i].Channel.Id
		}
		if candidates[j].Channel != nil {
			rightID = candidates[j].Channel.Id
		}
		return leftID < rightID
	})
}

// SelectManualChannel applies the legacy priority-layer and weight selection
// semantics to a pre-resolved candidate list. Larger Priority values are tried
// first; retry selects the next distinct priority layer and clamps after the
// last layer. The database selector historically gives every channel a +10
// baseline weight, so that behavior is retained here.
func SelectManualChannel(candidates []SatisfiedChannel, priorityRetry int) *Channel {
	return selectManualChannel(candidates, priorityRetry, false)
}

func selectManualChannel(candidates []SatisfiedChannel, priorityRetry int, memoryCache bool) *Channel {
	if len(candidates) == 0 {
		return nil
	}
	priorities := make([]int64, 0, len(candidates))
	seen := make(map[int64]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate.Channel == nil {
			continue
		}
		if _, ok := seen[candidate.Priority]; ok {
			continue
		}
		seen[candidate.Priority] = struct{}{}
		priorities = append(priorities, candidate.Priority)
	}
	if len(priorities) == 0 {
		return nil
	}
	sort.Slice(priorities, func(i, j int) bool { return priorities[i] > priorities[j] })
	if priorityRetry < 0 {
		priorityRetry = 0
	}
	if priorityRetry >= len(priorities) {
		priorityRetry = len(priorities) - 1
	}
	targetPriority := priorities[priorityRetry]
	targets := make([]SatisfiedChannel, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Channel != nil && candidate.Priority == targetPriority {
			targets = append(targets, candidate)
		}
	}
	if len(targets) == 0 {
		return nil
	}
	if len(targets) == 1 {
		return targets[0].Channel
	}

	if memoryCache {
		return selectMemoryWeightedChannel(targets)
	}
	return selectBaselineWeightedChannel(targets)
}

func selectBaselineWeightedChannel(candidates []SatisfiedChannel) *Channel {
	// Keep the arithmetic bounded enough for rand.Intn and fall back to a
	// deterministic first candidate if a malformed weight would overflow.
	total := int64(0)
	for _, candidate := range candidates {
		weight := int64(candidate.Weight)
		if weight < 0 {
			weight = 0
		}
		if total > int64(^uint(0)>>1)-weight-10 {
			return candidates[0].Channel
		}
		total += weight + 10
	}
	if total <= 0 {
		return candidates[0].Channel
	}
	randomWeight := common.GetRandomInt(int(total))
	for _, candidate := range candidates {
		weight := candidate.Weight
		if weight < 0 {
			weight = 0
		}
		randomWeight -= weight + 10
		if randomWeight < 0 {
			return candidate.Channel
		}
	}
	return candidates[len(candidates)-1].Channel
}

func selectMemoryWeightedChannel(candidates []SatisfiedChannel) *Channel {
	sumWeight := 0
	for _, candidate := range candidates {
		if candidate.Weight > 0 {
			sumWeight += candidate.Weight
		}
	}
	smoothingFactor := 1
	smoothingAdjustment := 0
	if sumWeight == 0 {
		sumWeight = len(candidates) * 100
		smoothingAdjustment = 100
	} else if sumWeight/len(candidates) < 10 {
		smoothingFactor = 100
	}
	totalWeight := sumWeight * smoothingFactor
	if totalWeight <= 0 {
		return candidates[0].Channel
	}
	randomWeight := common.GetRandomInt(totalWeight)
	for _, candidate := range candidates {
		weight := candidate.Weight
		if weight < 0 {
			weight = 0
		}
		randomWeight -= weight*smoothingFactor + smoothingAdjustment
		if randomWeight < 0 {
			return candidate.Channel
		}
	}
	return candidates[len(candidates)-1].Channel
}

func groupModelChannelIDsDB(group, modelName, requestPath string, filter ChannelSelectFilter) []int {
	abilities, err := getSelectableAbilities(group, modelName)
	if err != nil {
		return nil
	}
	abilities = filterAbilitiesByRequestPathAndModel(abilities, requestPath, modelName)
	channelIDs := make([]int, 0, len(abilities))
	for _, ability := range abilities {
		channelIDs = append(channelIDs, ability.ChannelId)
	}
	return filterChannelIDs(channelIDs, filter)
}
