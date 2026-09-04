package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/imageprofile"
	"github.com/QuantumNous/new-api/setting/image_setting"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
)

type imageCostRuleKey struct {
	upstreamModel string
	variant       string
}

// SyncImageCostRulesForChannel retires image cost rules that are no longer
// reachable through the channel's current image capability matrix. It never
// creates a price for a newly enabled capability.
func SyncImageCostRulesForChannel(channelID int, adminID int) error {
	if channelID <= 0 {
		return errors.New("image cost sync channel is required")
	}
	var channel model.Channel
	if err := model.DB.Where("id = ?", channelID).First(&channel).Error; err != nil {
		return err
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		return syncImageCostRulesForChannelWithTx(tx, &channel, adminID)
	})
}

// SyncImageCostRulesForChannelSnapshot applies synchronization to the supplied
// effective channel snapshot. This is useful when the caller has already
// merged a sparse update with the stored channel record.
func SyncImageCostRulesForChannelSnapshot(channel *model.Channel, adminID int) error {
	if channel == nil || channel.Id <= 0 {
		return errors.New("image cost sync channel is required")
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		return syncImageCostRulesForChannelWithTx(tx, channel, adminID)
	})
}

// SyncImageCostRulesForChannelWithTx applies image cost-rule synchronization
// inside the caller's transaction. It is used by channel updates that need
// the channel and its related cost rules to commit atomically.
func SyncImageCostRulesForChannelWithTx(tx *gorm.DB, channel *model.Channel, adminID int) error {
	return syncImageCostRulesForChannelWithTx(tx, channel, adminID)
}

func syncImageCostRulesForChannelWithTx(tx *gorm.DB, channel *model.Channel, adminID int) error {
	if tx == nil {
		return errors.New("image cost sync transaction is required")
	}
	if channel == nil || channel.Id <= 0 {
		return errors.New("image cost sync channel is required")
	}
	_ = adminID

	allowed, err := imageCostRuleAllowances(channel)
	if err != nil {
		return err
	}
	var rules []model.ChannelModelCostRule
	if err := tx.Where("channel_id = ? AND cost_mode = ?", channel.Id, types.CostModePerImage).Find(&rules).Error; err != nil {
		return err
	}
	now := common.GetTimestamp()
	changed := false
	for _, rule := range rules {
		if rule.Status != string(types.CostRuleActive) && rule.Status != string(types.CostRuleDraft) {
			continue
		}
		variant, normalizeErr := types.NormalizeCostVariantKey(rule.CostVariantKey)
		if normalizeErr != nil {
			variant = strings.TrimSpace(rule.CostVariantKey)
		}
		if _, keep := allowed[imageCostRuleKey{upstreamModel: strings.TrimSpace(rule.BillableUpstreamModel), variant: variant}]; keep {
			continue
		}
		result := tx.Model(&model.ChannelModelCostRule{}).
			Where("id = ? AND status IN ?", rule.ID, []string{string(types.CostRuleActive), string(types.CostRuleDraft)}).
			Updates(map[string]any{
				"status":       string(types.CostRuleRetired),
				"effective_to": now,
				"updated_at":   now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			changed = true
		}
	}
	if changed {
		InvalidateCostCoverage(channel.Id, "", "")
	}
	return nil
}

func imageCostRuleAllowances(channel *model.Channel) (map[imageCostRuleKey]struct{}, error) {
	allowed := make(map[imageCostRuleKey]struct{})
	if !model.SupportsOpenAIImagesChannelType(channel.Type) {
		return allowed, nil
	}
	settings := channel.GetOtherSettings()
	if settings.ImageProfile == nil {
		return allowed, nil
	}
	if err := settings.ImageProfile.Validate(); err != nil {
		return nil, fmt.Errorf("validate channel image profile: %w", err)
	}
	profile, ok := imageprofile.Lookup(settings.ImageProfile.Profile, settings.ImageProfile.ProfileVersion)
	if !ok {
		return nil, fmt.Errorf("channel image profile %q version %d is not registered", settings.ImageProfile.Profile, settings.ImageProfile.ProfileVersion)
	}
	catalog := image_setting.Snapshot()
	for _, publicModel := range channel.GetModels() {
		publicModel = strings.TrimSpace(publicModel)
		entry, exists := catalog.Models[publicModel]
		if !exists {
			continue
		}
		upstreamModel, ok := imagePricingMappedModel(*channel, publicModel)
		if !ok {
			continue
		}
		for _, endpoint := range []imageprofile.Endpoint{imageprofile.EndpointGenerations, imageprofile.EndpointEdits} {
			endpointCatalog, exists := entry.Endpoints[endpoint]
			if !exists || !endpointCatalog.Capability.Enabled {
				continue
			}
			profileCapability, exists := profile.Capabilities[endpoint]
			if !exists || !profileCapability.Enabled {
				continue
			}
			override, hasOverride := settings.ImageProfile.CapabilityOverrides[publicModel]
			if hasOverride && imageEndpointDisabled(endpoint, override) {
				continue
			}
			for skuKey, sku := range entry.SKUs {
				canonicalKey, normalizeErr := normalizeImageCostMatrixKey(skuKey)
				if normalizeErr != nil || sku.Endpoint != endpoint {
					continue
				}
				parts := strings.Split(canonicalKey, "-")
				if len(parts) != 3 || !imageCapabilityAllowsSKU(endpointCatalog.Capability, profileCapability, override, hasOverride, parts[1], parts[2]) {
					continue
				}
				allowed[imageCostRuleKey{upstreamModel: strings.TrimSpace(upstreamModel), variant: canonicalKey}] = struct{}{}
			}
		}
	}
	return allowed, nil
}

func imageEndpointDisabled(endpoint imageprofile.Endpoint, override imageprofile.ModelCapabilities) bool {
	switch endpoint {
	case imageprofile.EndpointGenerations:
		return override.GenerationsSet() && !override.Generations
	case imageprofile.EndpointEdits:
		return override.EditsSet() && !override.Edits
	default:
		return true
	}
}

func imageCapabilityAllowsSKU(catalogCapability, profileCapability imageprofile.Capability, override imageprofile.ModelCapabilities, hasOverride bool, tier, quality string) bool {
	if !imageCapabilityAllowsTierAndQuality(catalogCapability, tier, quality) || !imageCapabilityAllowsTierAndQuality(profileCapability, tier, quality) {
		return false
	}
	if !hasOverride {
		return true
	}
	if len(override.ResolutionTiers) > 0 && !contains(override.ResolutionTiers, tier) {
		return false
	}
	if override.ResolutionQualities != nil && !contains(override.ResolutionQualities, tier+":"+quality) {
		return false
	}
	if len(override.Qualities) > 0 && !contains(override.Qualities, quality) {
		return false
	}
	return true
}

func imageCapabilityAllowsTierAndQuality(capability imageprofile.Capability, tier, quality string) bool {
	if len(capability.ResolutionTiers) > 0 && !contains(capability.ResolutionTiers, tier) {
		return false
	}
	if capability.ResolutionQualities != nil && !contains(capability.ResolutionQualities, tier+":"+quality) {
		return false
	}
	if len(capability.Qualities) > 0 && !contains(capability.Qualities, quality) {
		return false
	}
	return true
}
