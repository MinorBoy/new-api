package service

import (
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/image_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
)

// ListImagePricingCostSummary returns the lowest known active supplier cost
// for each SKU in the configured public image catalog.
func ListImagePricingCostSummary(models []string) (dto.ImagePricingCostSummary, error) {
	requestedModels := normalizeImagePricingModels(models)
	catalog := image_setting.Snapshot()
	if len(requestedModels) == 0 {
		requestedModels = make([]string, 0, len(catalog.Models))
		for modelName := range catalog.Models {
			requestedModels = append(requestedModels, modelName)
		}
		sort.Strings(requestedModels)
	}

	items := make([]dto.ImagePricingCostItem, 0)
	candidates := make([]CostRuleCandidate, 0)
	for _, modelName := range requestedModels {
		entry, ok := catalog.Models[modelName]
		if !ok {
			continue
		}
		for skuKey := range entry.SKUs {
			items = append(items, dto.ImagePricingCostItem{Model: modelName, SKU: skuKey})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Model != items[j].Model {
			return items[i].Model < items[j].Model
		}
		return items[i].SKU < items[j].SKU
	})

	channels, err := listEnabledImagePricingChannels()
	if err != nil {
		return dto.ImagePricingCostSummary{}, err
	}
	for _, channel := range channels {
		for _, modelName := range requestedModels {
			entry, ok := catalog.Models[modelName]
			if !ok || !channelAdvertisesModel(channel, modelName) {
				continue
			}
			upstreamModel, ok := imagePricingMappedModel(channel, modelName)
			if !ok {
				continue
			}
			for skuKey := range entry.SKUs {
				candidates = append(candidates, CostRuleCandidate{
					ChannelID: channel.Id, BillableUpstreamModel: upstreamModel, CostVariantKey: skuKey,
				})
			}
		}
	}
	rules, err := ActiveCostRules(candidates, true)
	if err != nil {
		return dto.ImagePricingCostSummary{}, err
	}

	bySKU := make(map[string]*imagePricingCostAggregate, len(items))
	for index := range items {
		bySKU[items[index].Model+"|"+items[index].SKU] = &imagePricingCostAggregate{}
	}
	for _, channel := range channels {
		for _, modelName := range requestedModels {
			entry, ok := catalog.Models[modelName]
			if !ok || !channelAdvertisesModel(channel, modelName) {
				continue
			}
			upstreamModel, ok := imagePricingMappedModel(channel, modelName)
			if !ok {
				continue
			}
			for skuKey := range entry.SKUs {
				rule := rules[CostRuleCandidate{ChannelID: channel.Id, BillableUpstreamModel: upstreamModel, CostVariantKey: skuKey}]
				if rule == nil {
					continue
				}
				cost, ok := imagePricingRuleCost(rule)
				if !ok {
					continue
				}
				aggregate := bySKU[modelName+"|"+skuKey]
				if aggregate == nil {
					continue
				}
				aggregate.SourceCount++
				if !aggregate.Known || cost.LessThan(aggregate.MinimumCost) {
					aggregate.MinimumCost = cost
					aggregate.Known = true
				}
			}
		}
	}
	for index := range items {
		aggregate := bySKU[items[index].Model+"|"+items[index].SKU]
		if aggregate != nil && aggregate.Known {
			items[index].Known = true
			items[index].MinimumCostUSD = aggregate.MinimumCost.String()
			items[index].SourceCount = aggregate.SourceCount
		}
	}
	return dto.ImagePricingCostSummary{Items: items}, nil
}

type imagePricingCostAggregate struct {
	Known       bool
	MinimumCost decimal.Decimal
	SourceCount int
}

func normalizeImagePricingModels(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	result := make([]string, 0, len(models))
	for _, modelName := range models {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		if _, ok := seen[modelName]; ok {
			continue
		}
		seen[modelName] = struct{}{}
		result = append(result, modelName)
	}
	sort.Strings(result)
	return result
}

func listEnabledImagePricingChannels() ([]model.Channel, error) {
	channels := make([]model.Channel, 0)
	err := model.DB.Model(&model.Channel{}).
		Select("id", "type", "status", "models", "model_mapping", "settings").
		Where("status = ?", common.ChannelStatusEnabled).
		Find(&channels).Error
	if err != nil {
		return nil, fmt.Errorf("list image pricing channels: %w", err)
	}
	filtered := channels[:0]
	for _, channel := range channels {
		if model.SupportsOpenAIImagesChannelType(channel.Type) {
			filtered = append(filtered, channel)
		}
	}
	return filtered, nil
}

func channelAdvertisesModel(channel model.Channel, publicModel string) bool {
	for _, advertised := range channel.GetModels() {
		if strings.TrimSpace(advertised) == publicModel {
			return true
		}
	}
	return false
}

func imagePricingMappedModel(channel model.Channel, publicModel string) (string, bool) {
	mapping := strings.TrimSpace(channel.GetModelMapping())
	if mapping == "" || mapping == "{}" {
		return publicModel, true
	}
	mapped, _, err := ResolveMappedModel(publicModel, mapping)
	if err != nil || strings.TrimSpace(mapped) == "" {
		return "", false
	}
	return mapped, true
}

func imagePricingRuleCost(rule *model.ChannelModelCostRule) (decimal.Decimal, bool) {
	if rule == nil {
		return decimal.Zero, false
	}
	mode := types.CostMode(rule.CostMode)
	if mode != types.CostModeFree && mode != types.CostModePerRequest && mode != types.CostModePerImage {
		return decimal.Zero, false
	}
	var config types.CostRuleConfigV1
	if err := common.UnmarshalJsonStr(rule.ConfigJSON, &config); err != nil {
		return decimal.Zero, false
	}
	meter := types.CostMeter{}
	if mode == types.CostModePerImage {
		count := int64(1)
		meter.ImageCount = &count
	}
	_, nanoUSD, err := CalculateAttemptCost(mode, config, meter)
	if err != nil || nanoUSD < 0 {
		return decimal.Zero, false
	}
	return decimal.NewFromInt(nanoUSD).Div(decimal.NewFromInt(1_000_000_000)), true
}
