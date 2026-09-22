package service

import (
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/imageprofile"
	"github.com/QuantumNous/new-api/setting/image_setting"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
)

type ImageCatalogLegacyCost struct {
	ChannelID     int
	UpstreamModel string
	Endpoint      imageprofile.Endpoint
	SKUKey        string
	CostUSD       string
}

type ImageCatalogMigrationConflict struct {
	ChannelID     int
	UpstreamModel string
	Endpoint      imageprofile.Endpoint
	TargetSKU     string
	// LegacySKUs and CostsUSD are retained for the admin migration report.
	LegacySKUs []string
	CostsUSD   []string
}

type ImageCatalogMigrationResult struct {
	Catalog     image_setting.Catalog
	MappedCosts []ImageCatalogLegacyCost
	Conflicts   []ImageCatalogMigrationConflict
	Errors      []string
}

// ImageCatalogStartupMigrationResult describes the one-time, idempotent
// migration performed after options and cost-rule tables are available.
// Conflicts and errors are reported without changing the active catalog.
type ImageCatalogStartupMigrationResult struct {
	CatalogChanged   bool
	CostRulesCreated int
	Conflicts        []ImageCatalogMigrationConflict
	Errors           []string
}

func MigrateImageCatalog(catalog image_setting.Catalog) (ImageCatalogMigrationResult, error) {
	result := ImageCatalogMigrationResult{Catalog: cloneImageCatalog(catalog)}
	for modelName, model := range catalog.Models {
		migrated := make(map[string]image_setting.SKU)
		for key, sku := range model.SKUs {
			tier := strings.TrimSpace(sku.Tier)
			if tier == "" {
				resolvedTier, _, err := image_setting.ResolveLegacyResolutionTier(sku.Size)
				if err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("%s/%s: %v", modelName, key, err))
					continue
				}
				tier = string(resolvedTier)
			}
			targetKey := image_setting.BuildTierSKUKey(sku.Endpoint, image_setting.ResolutionTier(tier), sku.Quality)
			if existing, ok := migrated[targetKey]; ok {
				if existing.SalePriceUSD != sku.SalePriceUSD {
					result.Conflicts = append(result.Conflicts, ImageCatalogMigrationConflict{
						UpstreamModel: modelName,
						Endpoint:      sku.Endpoint,
						TargetSKU:     targetKey,
						LegacySKUs:    []string{key},
						CostsUSD:      []string{existing.SalePriceUSD, sku.SalePriceUSD},
					})
					delete(migrated, targetKey)
					continue
				}
				continue
			}
			sku.Tier = tier
			sku.Size = ""
			migrated[targetKey] = sku
		}
		result.Catalog.Models[modelName] = image_setting.ModelEntry{
			Profile: model.Profile, ProfileVersion: model.ProfileVersion,
			Endpoints: model.Endpoints, SKUs: migrated,
		}
	}
	return result, nil
}

// MigrateImageCatalogAtStartup upgrades an old concrete-size image catalog to
// tier SKUs and creates equivalent canonical cost rules. It is deliberately
// conservative: if two active/draft legacy prices collapse into one tier with
// different values, neither the catalog nor the cost rules are changed.
// Retired legacy rows remain untouched so historical logs keep their original
// variant keys.
func MigrateImageCatalogAtStartup() (ImageCatalogStartupMigrationResult, error) {
	result := ImageCatalogStartupMigrationResult{}
	if model.DB == nil {
		return result, fmt.Errorf("image catalog migration requires an initialized database")
	}
	original := image_setting.Snapshot()
	migrated, err := MigrateImageCatalog(original)
	if err != nil {
		return result, err
	}
	// Seed catalog entries for image models advertised by enabled image
	// channels but missing from the catalog. A failed channel lookup only
	// skips seeding (logged); it must not block the tier migration itself.
	missingByProfile, seedErr := missingImageModelsByProfile(migrated.Catalog)
	if seedErr != nil {
		common.SysError("image catalog seeding skipped, channel lookup failed: " + seedErr.Error())
		missingByProfile = nil
	}
	ensuredCatalog, modelsChanged := image_setting.EnsureMissingProfileModels(migrated.Catalog, missingByProfile)
	migrated.Catalog = ensuredCatalog
	result.Conflicts = append(result.Conflicts, migrated.Conflicts...)
	result.Errors = append(result.Errors, migrated.Errors...)
	if len(result.Conflicts) > 0 || len(result.Errors) > 0 {
		return result, nil
	}
	legacyToTier := imageCatalogLegacySKUMap(original, migrated.Catalog)
	if len(legacyToTier) == 0 && !modelsChanged {
		return result, nil
	}

	plans, conflicts, err := planImageCostRuleMigration(legacyToTier)
	if err != nil {
		return result, err
	}
	result.Conflicts = append(result.Conflicts, conflicts...)
	if len(result.Conflicts) > 0 {
		return result, nil
	}

	encodedCatalog, err := common.Marshal(migrated.Catalog)
	if err != nil {
		return result, err
	}
	values := map[string]string{image_setting.CatalogOptionKey: string(encodedCatalog)}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := model.UpdateOptionsWithTx(tx, values); err != nil {
			return err
		}
		for _, plan := range plans {
			if err := tx.Create(&plan).Error; err != nil {
				return err
			}
			result.CostRulesCreated++
		}
		return nil
	})
	if err != nil {
		return ImageCatalogStartupMigrationResult{}, err
	}
	if err := model.RefreshOptions(values); err != nil {
		return ImageCatalogStartupMigrationResult{}, err
	}
	result.CatalogChanged = true
	return result, nil
}

func imageCatalogLegacySKUMap(original, migrated image_setting.Catalog) map[string]string {
	result := make(map[string]string)
	for modelName, entry := range original.Models {
		migratedEntry, ok := migrated.Models[modelName]
		if !ok {
			continue
		}
		for key, sku := range entry.SKUs {
			if strings.TrimSpace(sku.Tier) != "" {
				continue
			}
			tier, _, err := image_setting.ResolveLegacyResolutionTier(sku.Size)
			if err != nil {
				continue
			}
			target := image_setting.BuildTierSKUKey(sku.Endpoint, tier, sku.Quality)
			if key != target {
				if _, exists := migratedEntry.SKUs[target]; exists {
					result[key] = target
				}
			}
		}
	}
	return result
}

type imageCostRuleMigrationPlan = model.ChannelModelCostRule

func planImageCostRuleMigration(legacyToTier map[string]string) ([]imageCostRuleMigrationPlan, []ImageCatalogMigrationConflict, error) {
	legacyKeys := make([]string, 0, len(legacyToTier))
	targetKeys := make(map[string]struct{}, len(legacyToTier))
	for legacy, target := range legacyToTier {
		legacyKeys = append(legacyKeys, legacy)
		targetKeys[target] = struct{}{}
	}
	queryKeys := append([]string(nil), legacyKeys...)
	for target := range targetKeys {
		queryKeys = append(queryKeys, target)
	}
	var rules []model.ChannelModelCostRule
	if err := model.DB.Where("cost_variant_key IN ?", queryKeys).Find(&rules).Error; err != nil {
		return nil, nil, err
	}

	type group struct {
		target  string
		legacy  []model.ChannelModelCostRule
		current []model.ChannelModelCostRule
	}
	groups := make(map[string]*group)
	for _, rule := range rules {
		target, isLegacy := legacyToTier[rule.CostVariantKey]
		if !isLegacy {
			if _, ok := targetKeys[rule.CostVariantKey]; !ok {
				continue
			}
			target = rule.CostVariantKey
		}
		key := fmt.Sprintf("%d|%s|%s", rule.ChannelID, rule.BillableUpstreamModel, target)
		entry := groups[key]
		if entry == nil {
			entry = &group{target: target}
			groups[key] = entry
		}
		if isLegacy {
			entry.legacy = append(entry.legacy, rule)
		} else {
			entry.current = append(entry.current, rule)
		}
	}

	plans := make([]imageCostRuleMigrationPlan, 0)
	conflicts := make([]ImageCatalogMigrationConflict, 0)
	for key, entry := range groups {
		activeSources := make([]model.ChannelModelCostRule, 0)
		for _, rule := range entry.legacy {
			if rule.Status == string(types.CostRuleActive) || rule.Status == string(types.CostRuleDraft) {
				activeSources = append(activeSources, rule)
			}
		}
		if len(activeSources) == 0 {
			continue
		}
		costFingerprints := make(map[string]struct{})
		for _, rule := range activeSources {
			costFingerprints[imageCostRuleFingerprint(rule)] = struct{}{}
		}
		for _, rule := range entry.current {
			if rule.Status == string(types.CostRuleActive) || rule.Status == string(types.CostRuleDraft) {
				costFingerprints[imageCostRuleFingerprint(rule)] = struct{}{}
			}
		}
		if len(costFingerprints) > 1 {
			parts := strings.SplitN(key, "|", 3)
			conflict := ImageCatalogMigrationConflict{TargetSKU: entry.target}
			if len(parts) == 3 {
				fmt.Sscanf(parts[0], "%d", &conflict.ChannelID)
				conflict.UpstreamModel = parts[1]
			}
			for _, rule := range activeSources {
				conflict.LegacySKUs = append(conflict.LegacySKUs, rule.CostVariantKey)
				if cost, ok := imagePricingRuleCost(&rule); ok {
					conflict.CostsUSD = append(conflict.CostsUSD, cost.String())
				} else {
					conflict.CostsUSD = append(conflict.CostsUSD, "unknown")
				}
			}
			conflicts = append(conflicts, conflict)
			continue
		}

		latestByStatus := make(map[string]model.ChannelModelCostRule)
		for _, rule := range activeSources {
			latest, ok := latestByStatus[rule.Status]
			if !ok || rule.Version > latest.Version || rule.Version == latest.Version && rule.ID > latest.ID {
				latestByStatus[rule.Status] = rule
			}
		}
		maxVersion := 0
		statusExists := make(map[string]bool)
		for _, rule := range entry.current {
			if rule.Version > maxVersion {
				maxVersion = rule.Version
			}
			statusExists[rule.Status] = true
		}
		statuses := []string{string(types.CostRuleActive), string(types.CostRuleDraft)}
		for _, status := range statuses {
			rule, ok := latestByStatus[status]
			if !ok || statusExists[status] {
				continue
			}
			maxVersion++
			rule.ID = 0
			rule.CostVariantKey = entry.target
			rule.Version = maxVersion
			rule.Source = "migration"
			rule.Note = strings.TrimSpace(strings.TrimSpace(rule.Note) + " migrated from legacy image SKU")
			plans = append(plans, imageCostRuleMigrationPlan(rule))
		}
	}
	sort.Slice(conflicts, func(i, j int) bool {
		if conflicts[i].ChannelID != conflicts[j].ChannelID {
			return conflicts[i].ChannelID < conflicts[j].ChannelID
		}
		return conflicts[i].TargetSKU < conflicts[j].TargetSKU
	})
	sort.Slice(plans, func(i, j int) bool {
		if plans[i].ChannelID != plans[j].ChannelID {
			return plans[i].ChannelID < plans[j].ChannelID
		}
		return plans[i].CostVariantKey < plans[j].CostVariantKey
	})
	return plans, conflicts, nil
}

func imageCostRuleFingerprint(rule model.ChannelModelCostRule) string {
	return strings.TrimSpace(rule.CostMode) + "|" + strings.TrimSpace(rule.ConfigJSON)
}

func MigrateImageCatalogWithCosts(catalog image_setting.Catalog, costs []ImageCatalogLegacyCost) (ImageCatalogMigrationResult, error) {
	result, err := MigrateImageCatalog(catalog)
	if err != nil {
		return result, err
	}
	type costGroup struct {
		items []ImageCatalogLegacyCost
		costs map[string]struct{}
	}
	groups := make(map[string]*costGroup)
	for _, cost := range costs {
		tier, _, tierErr := legacyCostTier(cost.SKUKey)
		if tierErr != nil {
			result.Errors = append(result.Errors, tierErr.Error())
			continue
		}
		target := image_setting.BuildTierSKUKey(cost.Endpoint, tier, qualityFromLegacySKU(cost.SKUKey))
		key := fmt.Sprintf("%d|%s|%s|%s", cost.ChannelID, cost.UpstreamModel, cost.Endpoint, target)
		group := groups[key]
		if group == nil {
			group = &costGroup{costs: make(map[string]struct{})}
			groups[key] = group
		}
		group.items = append(group.items, cost)
		group.costs[cost.CostUSD] = struct{}{}
	}
	for key, group := range groups {
		parts := strings.SplitN(key, "|", 4)
		if len(group.costs) > 1 {
			conflict := ImageCatalogMigrationConflict{TargetSKU: parts[3], LegacySKUs: make([]string, 0, len(group.items)), CostsUSD: make([]string, 0, len(group.items))}
			fmt.Sscanf(parts[0], "%d", &conflict.ChannelID)
			conflict.UpstreamModel = parts[1]
			conflict.Endpoint = imageprofile.Endpoint(parts[2])
			for _, item := range group.items {
				conflict.LegacySKUs = append(conflict.LegacySKUs, item.SKUKey)
				conflict.CostsUSD = append(conflict.CostsUSD, item.CostUSD)
			}
			result.Conflicts = append(result.Conflicts, conflict)
			continue
		}
		mapped := group.items[0]
		mapped.SKUKey = parts[3]
		result.MappedCosts = append(result.MappedCosts, mapped)
	}
	sort.Slice(result.Conflicts, func(i, j int) bool { return result.Conflicts[i].TargetSKU < result.Conflicts[j].TargetSKU })
	sort.Slice(result.MappedCosts, func(i, j int) bool { return result.MappedCosts[i].SKUKey < result.MappedCosts[j].SKUKey })
	return result, nil
}

func legacyCostTier(key string) (image_setting.ResolutionTier, string, error) {
	parts := strings.Split(strings.TrimSpace(key), "-")
	if len(parts) != 3 {
		return "", "", fmt.Errorf("legacy image SKU %q is invalid", key)
	}
	if tier := image_setting.ResolutionTier(parts[1]); tier == image_setting.ResolutionTier1K || tier == image_setting.ResolutionTier2K || tier == image_setting.ResolutionTier4K {
		return tier, parts[2], nil
	}
	tier, _, err := image_setting.ResolveLegacyResolutionTier(parts[1])
	if err != nil {
		return "", "", fmt.Errorf("legacy image SKU %q cannot be migrated: %w", key, err)
	}
	return tier, parts[2], nil
}

func qualityFromLegacySKU(key string) string {
	parts := strings.Split(strings.TrimSpace(key), "-")
	if len(parts) < 3 {
		return ""
	}
	return parts[len(parts)-1]
}

func cloneImageCatalog(catalog image_setting.Catalog) image_setting.Catalog {
	clone := image_setting.Catalog{Version: catalog.Version, Models: make(map[string]image_setting.ModelEntry, len(catalog.Models))}
	for name, entry := range catalog.Models {
		clonedEntry := entry
		clonedEntry.Endpoints = make(map[imageprofile.Endpoint]image_setting.EndpointCatalog, len(entry.Endpoints))
		for endpoint, endpointCatalog := range entry.Endpoints {
			clonedEntry.Endpoints[endpoint] = endpointCatalog
		}
		clonedEntry.SKUs = make(map[string]image_setting.SKU, len(entry.SKUs))
		for key, sku := range entry.SKUs {
			clonedEntry.SKUs[key] = sku
		}
		clone.Models[name] = clonedEntry
	}
	return clone
}

// missingImageModelsByProfile returns image models advertised by enabled
// channels with an image profile binding but missing from the catalog,
// grouped by profile. Only profiles that already have a catalog entry are
// considered, because seeding clones a sibling entry of the same profile.
func missingImageModelsByProfile(catalog image_setting.Catalog) (map[string][]string, error) {
	var channels []model.Channel
	if err := model.DB.Where("status = ?", common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
		return nil, err
	}
	profiles := make(map[string]struct{})
	for _, entry := range catalog.Models {
		profiles[entry.Profile] = struct{}{}
	}
	missing := make(map[string][]string)
	seen := make(map[string]struct{})
	for _, channel := range channels {
		binding := channel.GetOtherSettings().ImageProfile
		if binding == nil {
			continue
		}
		if _, ok := profiles[binding.Profile]; !ok {
			continue
		}
		for _, name := range channel.GetModels() {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if _, exists := catalog.Models[name]; exists {
				continue
			}
			key := binding.Profile + "\x00" + name
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			missing[binding.Profile] = append(missing[binding.Profile], name)
		}
	}
	for profile := range missing {
		sort.Strings(missing[profile])
	}
	return missing, nil
}
