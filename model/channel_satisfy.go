package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

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
