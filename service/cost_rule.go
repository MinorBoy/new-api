package service

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var CostCapabilityLookup func(channelType int, requestPath string, taskPlatform constant.TaskPlatform) types.CostCapabilities

type CreateCostRuleInput struct {
	ChannelID             int
	BillableUpstreamModel string
	CostVariantKey        string
	CostMode              types.CostMode
	Config                types.CostRuleConfigV1
	Note                  string
	AdminID               int
	RequestPath           string
	TaskPlatform          constant.TaskPlatform
}

type UpdateCostRuleInput struct {
	CostMode     types.CostMode
	Config       types.CostRuleConfigV1
	Note         string
	RequestPath  string
	TaskPlatform constant.TaskPlatform
}

type PredictedCoverageInput struct {
	ChannelID              int
	PredictedUpstreamModel string
	CostVariantKey         string
	RequestPath            string
	TaskPlatform           constant.TaskPlatform
	ContractTargets        []CostContractTarget
	Authoritative          bool
}

type CostContractTarget struct {
	RequestPath  string
	TaskPlatform constant.TaskPlatform
}

type CostCoverageResult struct {
	ChannelID              int
	OriginModel            string
	PredictedUpstreamModel string
	CostVariantKey         string
	Covered                bool
	Reason                 string
}

type costCoverageKey struct {
	channelID int
	model     string
	variant   string
}

var activeCostRuleCache sync.Map

func ValidateCostRuleDraft(rule *model.ChannelModelCostRule, capabilities types.CostCapabilities) (types.CostRuleConfigV1, error) {
	if rule == nil {
		return types.CostRuleConfigV1{}, errors.New("cost rule is required")
	}
	if rule.ChannelID <= 0 {
		return types.CostRuleConfigV1{}, errors.New("cost rule channel is required")
	}
	if _, err := model.GetChannelById(rule.ChannelID, false); err != nil {
		return types.CostRuleConfigV1{}, fmt.Errorf("load cost rule channel: %w", err)
	}
	return validateCostRuleContract(rule, capabilities)
}

func CreateCostRuleDraft(input CreateCostRuleInput) (*model.ChannelModelCostRule, error) {
	channel, err := model.GetChannelById(input.ChannelID, false)
	if err != nil {
		return nil, fmt.Errorf("load cost rule channel: %w", err)
	}
	capabilities, err := lookupChannelCostCapabilities(channel.Type, input.RequestPath, input.TaskPlatform)
	if err != nil {
		return nil, err
	}
	rule, err := buildCostRuleDraft(input, capabilities)
	if err != nil {
		return nil, err
	}

	var createErr error
	for attempt := 0; attempt < 2; attempt++ {
		candidate := *rule
		createErr = model.DB.Transaction(func(tx *gorm.DB) error {
			var latestVersion int
			if err := tx.Model(&model.ChannelModelCostRule{}).
				Where("channel_id = ? AND billable_upstream_model = ? AND cost_variant_key = ?",
					candidate.ChannelID, candidate.BillableUpstreamModel, candidate.CostVariantKey).
				Select("COALESCE(MAX(version), 0)").
				Scan(&latestVersion).Error; err != nil {
				return err
			}
			candidate.Version = latestVersion + 1
			return tx.Create(&candidate).Error
		})
		if createErr == nil {
			return &candidate, nil
		}
	}
	return nil, createErr
}

func UpdateCostRuleDraft(id int64, input UpdateCostRuleInput) (*model.ChannelModelCostRule, error) {
	var existing model.ChannelModelCostRule
	if err := model.DB.Where("id = ?", id).First(&existing).Error; err != nil {
		return nil, err
	}
	if existing.Status != string(types.CostRuleDraft) {
		return nil, model.ErrCostRuleStateConflict
	}
	channel, err := model.GetChannelById(existing.ChannelID, false)
	if err != nil {
		return nil, err
	}
	capabilities, err := lookupChannelCostCapabilities(channel.Type, input.RequestPath, input.TaskPlatform)
	if err != nil {
		return nil, err
	}

	normalized, err := NormalizeCostRuleConfig(input.CostMode, input.Config)
	if err != nil {
		return nil, err
	}
	configJSON, err := common.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	candidate := existing
	candidate.CostMode = string(input.CostMode)
	candidate.ConfigJSON = string(configJSON)
	candidate.Note = strings.TrimSpace(input.Note)
	if _, err := validateCostRuleContract(&candidate, capabilities); err != nil {
		return nil, err
	}

	now := common.GetTimestamp()
	result := model.DB.Model(&model.ChannelModelCostRule{}).
		Where("id = ? AND status = ?", id, types.CostRuleDraft).
		Updates(map[string]any{
			"cost_mode":   candidate.CostMode,
			"config_json": candidate.ConfigJSON,
			"note":        candidate.Note,
			"updated_at":  now,
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, model.ErrCostRuleStateConflict
	}
	if err := model.DB.First(&candidate, id).Error; err != nil {
		return nil, err
	}
	return &candidate, nil
}

func ActivateCostRule(id int64, adminID int) (*model.ChannelModelCostRule, error) {
	var draft model.ChannelModelCostRule
	if err := model.DB.Where("id = ?", id).First(&draft).Error; err != nil {
		return nil, err
	}
	channel, err := model.GetChannelById(draft.ChannelID, false)
	if err != nil {
		return nil, err
	}
	capabilities, err := lookupChannelCostCapabilities(channel.Type, "", "")
	if err != nil {
		return nil, err
	}

	activated, err := model.ActivateChannelModelCostRule(id, adminID, common.GetTimestamp(), func(locked *model.ChannelModelCostRule) error {
		_, validateErr := validateCostRuleContract(locked, capabilities)
		return validateErr
	})
	if err != nil {
		return nil, err
	}
	InvalidateCostCoverage(activated.ChannelID, activated.BillableUpstreamModel, activated.CostVariantKey)
	return activated, nil
}

func RetireCostRule(id int64, adminID int) error {
	var rule model.ChannelModelCostRule
	if err := model.DB.Select("id", "channel_id", "billable_upstream_model", "cost_variant_key").Where("id = ?", id).First(&rule).Error; err != nil {
		return err
	}
	if err := model.RetireChannelModelCostRule(id, adminID, common.GetTimestamp()); err != nil {
		return err
	}
	InvalidateCostCoverage(rule.ChannelID, rule.BillableUpstreamModel, rule.CostVariantKey)
	return nil
}

func ActiveCostRule(channelID int, billableModel, costVariantKey string, authoritative bool) (*model.ChannelModelCostRule, error) {
	variant, err := types.NormalizeCostVariantKey(costVariantKey)
	if err != nil {
		return nil, err
	}
	key := costCoverageKey{channelID: channelID, model: strings.TrimSpace(billableModel), variant: variant}
	if !authoritative {
		if cached, ok := activeCostRuleCache.Load(key); ok {
			rule := cached.(model.ChannelModelCostRule)
			return &rule, nil
		}
	}

	var rules []model.ChannelModelCostRule
	err = model.DB.Where(
		"channel_id = ? AND billable_upstream_model = ? AND cost_variant_key = ? AND status = ?",
		key.channelID, key.model, key.variant, types.CostRuleActive,
	).Order("version DESC").Limit(2).Find(&rules).Error
	if err != nil {
		activeCostRuleCache.Delete(key)
		return nil, err
	}
	if len(rules) == 0 {
		activeCostRuleCache.Delete(key)
		return nil, gorm.ErrRecordNotFound
	}
	if len(rules) > 1 {
		activeCostRuleCache.Delete(key)
		return nil, model.ErrCostActiveRuleConflict
	}
	activeCostRuleCache.Store(key, rules[0])
	rule := rules[0]
	return &rule, nil
}

// CostRuleCandidate identifies one (channel, billable upstream model, cost variant)
// triple the routing layer wants to price. It is the batch counterpart of
// ActiveCostRule's scalar arguments, and serves as the map key returned by
// ActiveCostRules.
type CostRuleCandidate struct {
	ChannelID             int
	BillableUpstreamModel string
	CostVariantKey        string
}

// ActiveCostRules resolves the active cost rule for every candidate in a single
// database query, avoiding the N+1 pattern the per-candidate ActiveCostRule would
// produce during channel selection.
//
// The non-authoritative path reuses the same activeCostRuleCache as ActiveCostRule,
// priming it from the batch result so subsequent single-candidate lookups (including
// the authoritative pre-dispatch recheck in Task 7) hit the cache. Missing candidates
// (draft/retired/no-rule) are simply absent from the returned map; the caller treats
// absence as "no active rule" exactly as a gorm.ErrRecordNotFound from ActiveCostRule.
// An active-rule conflict (two active versions for one key) surfaces as
// model.ErrCostActiveRuleConflict and drops both entries, matching single-key semantics.
//
// The query uses only GORM methods with a status equality filter — no dialect SQL —
// so it is valid across SQLite, MySQL and PostgreSQL.
func ActiveCostRules(candidates []CostRuleCandidate, authoritative bool) (map[CostRuleCandidate]*model.ChannelModelCostRule, error) {
	if len(candidates) == 0 {
		return map[CostRuleCandidate]*model.ChannelModelCostRule{}, nil
	}

	// De-duplicate by the normalized key so repeated candidates do not inflate the
	// result or trigger spurious conflict detection.
	normalized := make(map[costCoverageKey]CostRuleCandidate, len(candidates))
	cacheHits := make(map[costCoverageKey]*model.ChannelModelCostRule)
	pending := make([]costCoverageKey, 0, len(candidates))
	for _, candidate := range candidates {
		modelName := strings.TrimSpace(candidate.BillableUpstreamModel)
		if candidate.ChannelID == 0 || modelName == "" {
			continue
		}
		variant, err := types.NormalizeCostVariantKey(candidate.CostVariantKey)
		if err != nil {
			return nil, err
		}
		key := costCoverageKey{channelID: candidate.ChannelID, model: modelName, variant: variant}
		if _, exists := normalized[key]; exists {
			continue
		}
		normalized[key] = CostRuleCandidate{ChannelID: candidate.ChannelID, BillableUpstreamModel: modelName, CostVariantKey: variant}
		if !authoritative {
			if cached, ok := activeCostRuleCache.Load(key); ok {
				rule := cached.(model.ChannelModelCostRule)
				cacheHits[key] = &rule
				continue
			}
		}
		pending = append(pending, key)
	}

	results := make(map[CostRuleCandidate]*model.ChannelModelCostRule, len(candidates))
	for key, rule := range cacheHits {
		results[normalized[key]] = rule
	}
	if len(pending) == 0 {
		return results, nil
	}

	// Build a single WHERE clause matching any pending (channel_id, model, variant)
	// triple. We avoid a cross-database tuple-IN by OR-ing channel-scoped clauses,
	// and repeat the active-status predicate in each branch so the OR cannot pull in
	// draft/retired rows. This keeps the query portable across SQLite/MySQL/PostgreSQL
	// without dialect branches.
	channelModels := make(map[int]map[string][]string)
	for _, key := range pending {
		variants, ok := channelModels[key.channelID]
		if !ok {
			variants = make(map[string][]string)
			channelModels[key.channelID] = variants
		}
		variants[key.variant] = append(variants[key.variant], key.model)
	}
	query := model.DB.Model(&model.ChannelModelCostRule{})
	first := true
	for channelID, variantModels := range channelModels {
		for variant, models := range variantModels {
			clause := "channel_id = ? AND cost_variant_key = ? AND billable_upstream_model IN ? AND status = ?"
			args := []any{channelID, variant, models, types.CostRuleActive}
			if first {
				query = query.Where(clause, args...)
				first = false
				continue
			}
			query = query.Or(clause, args...)
		}
	}
	var rows []model.ChannelModelCostRule
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}

	byKey := make(map[costCoverageKey][]model.ChannelModelCostRule)
	for index := range rows {
		row := rows[index]
		key := costCoverageKey{channelID: row.ChannelID, model: row.BillableUpstreamModel, variant: row.CostVariantKey}
		byKey[key] = append(byKey[key], row)
	}
	for _, key := range pending {
		matches := byKey[key]
		if len(matches) == 0 {
			activeCostRuleCache.Delete(key)
			continue
		}
		// Mirror ActiveCostRule: keep only the highest version; more than one active
		// version is an unrecoverable conflict.
		latest := matches[0]
		for _, row := range matches[1:] {
			if row.Version > latest.Version {
				latest = row
			}
		}
		if len(matches) > 1 {
			activeCostRuleCache.Delete(key)
			return nil, fmt.Errorf("%w: channel=%d model=%s", model.ErrCostActiveRuleConflict, key.channelID, key.model)
		}
		activeCostRuleCache.Store(key, latest)
		latestCopy := latest
		results[normalized[key]] = &latestCopy
	}
	return results, nil
}

func CheckPredictedCostCoverage(input PredictedCoverageInput) (bool, error) {
	channel, err := model.GetChannelById(input.ChannelID, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	rule, err := ActiveCostRule(input.ChannelID, input.PredictedUpstreamModel, input.CostVariantKey, input.Authoritative)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	targets := input.ContractTargets
	if len(targets) == 0 {
		targets = []CostContractTarget{{RequestPath: input.RequestPath, TaskPlatform: input.TaskPlatform}}
	}
	for _, target := range targets {
		capabilities, err := lookupChannelCostCapabilities(channel.Type, target.RequestPath, target.TaskPlatform)
		if err != nil {
			return false, nil
		}
		if _, err := validateCostRuleContract(rule, capabilities); err != nil {
			return false, nil
		}
	}
	return true, nil
}

// InvalidateCostCoverage drops cached active cost rules. Each selector argument
// is optional: a zero channelID, empty billableModel, or empty costVariantKey
// matches every value for that dimension (so InvalidateCostCoverage(0, "", "")
// clears the whole cache, matching the legacy bulk-reset behavior). A non-empty
// variant narrows the eviction to that single variant only.
func InvalidateCostCoverage(channelID int, billableModel, costVariantKey string) {
	variant := strings.TrimSpace(costVariantKey)
	if variant != "" {
		normalized, err := types.NormalizeCostVariantKey(variant)
		if err != nil {
			// An invalid variant cannot match any cached key; nothing to evict.
			return
		}
		variant = normalized
	}
	activeCostRuleCache.Range(func(key, _ any) bool {
		coverageKey, ok := key.(costCoverageKey)
		if !ok {
			activeCostRuleCache.Delete(key)
			return true
		}
		channelMatches := channelID == 0 || coverageKey.channelID == channelID
		modelMatches := billableModel == "" || coverageKey.model == billableModel
		variantMatches := variant == "" || coverageKey.variant == variant
		if channelMatches && modelMatches && variantMatches {
			activeCostRuleCache.Delete(key)
		}
		return true
	})
}

func GetCostRule(id int64) (*model.ChannelModelCostRule, error) {
	var rule model.ChannelModelCostRule
	if err := model.DB.Where("id = ?", id).First(&rule).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

func ListCostRules(channelID int, billableModel, costVariantKey string) ([]model.ChannelModelCostRule, error) {
	query := model.DB.Model(&model.ChannelModelCostRule{})
	if channelID > 0 {
		query = query.Where("channel_id = ?", channelID)
	}
	if strings.TrimSpace(billableModel) != "" {
		query = query.Where("billable_upstream_model = ?", strings.TrimSpace(billableModel))
	}
	if strings.TrimSpace(costVariantKey) != "" {
		variant, err := types.NormalizeCostVariantKey(costVariantKey)
		if err != nil {
			return nil, err
		}
		query = query.Where("cost_variant_key = ?", variant)
	}
	var rules []model.ChannelModelCostRule
	err := query.Order("channel_id ASC, billable_upstream_model ASC, cost_variant_key ASC, version DESC").Find(&rules).Error
	return rules, err
}

func CostRuleHistory(id int64) ([]model.ChannelModelCostRule, error) {
	rule, err := GetCostRule(id)
	if err != nil {
		return nil, err
	}
	return ListCostRules(rule.ChannelID, rule.BillableUpstreamModel, rule.CostVariantKey)
}

func ValidateCostRuleByID(id int64) (types.CostRuleConfigV1, error) {
	rule, err := GetCostRule(id)
	if err != nil {
		return types.CostRuleConfigV1{}, err
	}
	channel, err := model.GetChannelById(rule.ChannelID, false)
	if err != nil {
		return types.CostRuleConfigV1{}, err
	}
	capabilities, err := lookupChannelCostCapabilities(channel.Type, "", "")
	if err != nil {
		return types.CostRuleConfigV1{}, err
	}
	return ValidateCostRuleDraft(rule, capabilities)
}

func CheckAuthoritativeCostCoverage() ([]CostCoverageResult, error) {
	abilities, err := model.GetAllEnableAbilityWithChannels()
	if err != nil {
		return nil, err
	}
	var activeRules []model.ChannelModelCostRule
	if err := model.DB.Select("channel_id", "billable_upstream_model", "cost_variant_key").
		Where("status = ?", types.CostRuleActive).Find(&activeRules).Error; err != nil {
		return nil, err
	}
	activeVariants := make(map[int]map[string]map[string]struct{})
	for _, rule := range activeRules {
		variant, err := types.NormalizeCostVariantKey(rule.CostVariantKey)
		if err != nil {
			return nil, err
		}
		models, ok := activeVariants[rule.ChannelID]
		if !ok {
			models = make(map[string]map[string]struct{})
			activeVariants[rule.ChannelID] = models
		}
		variants, ok := models[rule.BillableUpstreamModel]
		if !ok {
			variants = make(map[string]struct{})
			models[rule.BillableUpstreamModel] = variants
		}
		variants[variant] = struct{}{}
	}

	type capabilityPolicyKey struct {
		groupName string
		model     string
	}
	capabilityPolicies := make(map[capabilityPolicyKey]struct{})
	var routingTargets []struct {
		ChannelID      int
		OriginModel    string
		UpstreamModel  string
		CostVariantKey string
	}
	if model.DB.Migrator().HasTable(&model.RouteTarget{}) && model.DB.Migrator().HasTable(&model.RoutingPolicy{}) {
		var enabledPolicies []struct {
			GroupName string
			Model     string
		}
		if err := model.DB.Model(&model.RoutingPolicy{}).
			Select("group_name", "model").Where("enabled = ?", true).Find(&enabledPolicies).Error; err != nil {
			return nil, err
		}
		for _, policy := range enabledPolicies {
			capabilityPolicies[capabilityPolicyKey{groupName: policy.GroupName, model: policy.Model}] = struct{}{}
		}
		if err := model.DB.Table("route_targets").
			Select("route_targets.channel_id, routing_policies.model AS origin_model, route_targets.upstream_model, route_targets.cost_variant_key").
			Joins("JOIN routing_policies ON routing_policies.id = route_targets.policy_id").
			Where("route_targets.enabled = ? AND routing_policies.enabled = ?", true, true).
			Order("route_targets.channel_id ASC, route_targets.upstream_model ASC, route_targets.cost_variant_key ASC, route_targets.id ASC").
			Scan(&routingTargets).Error; err != nil {
			return nil, err
		}
	}

	results := make([]CostCoverageResult, 0, len(abilities)+len(activeRules))
	seen := make(map[costCoverageKey]struct{}, len(abilities))
	for _, ability := range abilities {
		if _, ok := capabilityPolicies[capabilityPolicyKey{groupName: ability.Group, model: ability.Model}]; ok {
			continue
		}
		channel, err := model.GetChannelById(ability.ChannelId, false)
		if err != nil {
			return nil, err
		}
		predictedModel, err := predictedCostModel(channel.GetModelMapping(), ability.Model)
		result := CostCoverageResult{
			ChannelID:              ability.ChannelId,
			OriginModel:            ability.Model,
			PredictedUpstreamModel: predictedModel,
			CostVariantKey:         string(types.DefaultCostVariantKey),
		}
		if err != nil {
			result.Reason = "invalid_model_mapping"
			results = append(results, result)
			continue
		}
		variants := make(map[string]struct{})
		if models, ok := activeVariants[ability.ChannelId]; ok {
			for variant := range models[predictedModel] {
				variants[variant] = struct{}{}
			}
		}
		if len(variants) == 0 {
			variants[string(types.DefaultCostVariantKey)] = struct{}{}
		}
		variantKeys := make([]string, 0, len(variants))
		for variant := range variants {
			variantKeys = append(variantKeys, variant)
		}
		sort.Strings(variantKeys)
		for _, variant := range variantKeys {
			key := costCoverageKey{channelID: ability.ChannelId, model: predictedModel, variant: variant}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			variantResult := result
			variantResult.CostVariantKey = variant
			variantResult.Covered, err = CheckPredictedCostCoverage(PredictedCoverageInput{
				ChannelID:              ability.ChannelId,
				PredictedUpstreamModel: predictedModel,
				CostVariantKey:         variant,
				Authoritative:          true,
			})
			if err != nil {
				return nil, err
			}
			if !variantResult.Covered {
				variantResult.Reason = "missing_or_incompatible_cost_rule"
			}
			results = append(results, variantResult)
		}
	}

	for _, target := range routingTargets {
		variant, err := types.NormalizeCostVariantKey(target.CostVariantKey)
		if err != nil {
			return nil, err
		}
		key := costCoverageKey{channelID: target.ChannelID, model: target.UpstreamModel, variant: variant}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result := CostCoverageResult{
			ChannelID:              target.ChannelID,
			OriginModel:            target.OriginModel,
			PredictedUpstreamModel: target.UpstreamModel,
			CostVariantKey:         variant,
		}
		result.Covered, err = CheckPredictedCostCoverage(PredictedCoverageInput{
			ChannelID:              target.ChannelID,
			PredictedUpstreamModel: target.UpstreamModel,
			CostVariantKey:         variant,
			Authoritative:          true,
		})
		if err != nil {
			return nil, err
		}
		if !result.Covered {
			result.Reason = "missing_or_incompatible_cost_rule"
		}
		results = append(results, result)
	}
	return results, nil
}

func buildCostRuleDraft(input CreateCostRuleInput, capabilities types.CostCapabilities) (*model.ChannelModelCostRule, error) {
	normalized, err := NormalizeCostRuleConfig(input.CostMode, input.Config)
	if err != nil {
		return nil, err
	}
	configJSON, err := common.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	variant, err := types.NormalizeCostVariantKey(input.CostVariantKey)
	if err != nil {
		return nil, err
	}
	now := common.GetTimestamp()
	rule := &model.ChannelModelCostRule{
		ChannelID:             input.ChannelID,
		BillableUpstreamModel: strings.TrimSpace(input.BillableUpstreamModel),
		CostVariantKey:        variant,
		Status:                string(types.CostRuleDraft),
		CostMode:              string(input.CostMode),
		SchemaVersion:         1,
		ConfigJSON:            string(configJSON),
		Source:                "manual",
		Note:                  strings.TrimSpace(input.Note),
		CreatedBy:             input.AdminID,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if _, err := validateCostRuleContract(rule, capabilities); err != nil {
		return nil, err
	}
	return rule, nil
}

func validateCostRuleContract(rule *model.ChannelModelCostRule, capabilities types.CostCapabilities) (types.CostRuleConfigV1, error) {
	if rule.ChannelID <= 0 || strings.TrimSpace(rule.BillableUpstreamModel) == "" {
		return types.CostRuleConfigV1{}, errors.New("cost rule channel and billable model are required")
	}
	if len(rule.BillableUpstreamModel) > 191 {
		return types.CostRuleConfigV1{}, errors.New("billable model exceeds 191 characters")
	}
	if rule.SchemaVersion != 1 {
		return types.CostRuleConfigV1{}, fmt.Errorf("unsupported cost rule schema version %d", rule.SchemaVersion)
	}
	mode := types.CostMode(rule.CostMode)
	if mode != types.CostModeFree && mode != types.CostModePerRequest && mode != types.CostModePerDuration && mode != types.CostModePerToken {
		return types.CostRuleConfigV1{}, fmt.Errorf("unsupported cost mode %q", rule.CostMode)
	}
	var stored types.CostRuleConfigV1
	if err := common.UnmarshalJsonStr(rule.ConfigJSON, &stored); err != nil {
		return types.CostRuleConfigV1{}, fmt.Errorf("decode cost rule config: %w", err)
	}
	normalized, err := NormalizeCostRuleConfig(mode, stored)
	if err != nil {
		return types.CostRuleConfigV1{}, err
	}
	if !reflect.DeepEqual(stored.NormalizedUSDPrices, normalized.NormalizedUSDPrices) {
		return types.CostRuleConfigV1{}, errors.New("normalized USD prices do not match rule inputs")
	}
	if !capabilities.CanResolveBillableModel {
		return types.CostRuleConfigV1{}, errors.New("adaptor cannot resolve the billable model")
	}

	if mode == types.CostModeFree {
		if stored.ChargeEvent != "" || stored.MeterSource != "" || stored.TokenMode != "" {
			return types.CostRuleConfigV1{}, errors.New("free rules cannot declare charge or meter fields")
		}
		return normalized, nil
	}
	if !validCostChargeEvent(stored.ChargeEvent) || !containsCostChargeEvent(capabilities.ChargeEvents, stored.ChargeEvent) {
		return types.CostRuleConfigV1{}, fmt.Errorf("unsupported cost charge event %q", stored.ChargeEvent)
	}

	switch mode {
	case types.CostModePerRequest:
		if stored.MeterSource != "" || stored.TokenMode != "" {
			return types.CostRuleConfigV1{}, errors.New("per-request rules cannot declare meter or token modes")
		}
		if err := validatePositiveCostPrice(stored.UnitPrice, stored.NormalizedUSDPrices.UnitPrice); err != nil {
			return types.CostRuleConfigV1{}, err
		}
	case types.CostModePerDuration:
		if stored.MeterSource != types.CostMeterValidatedRequest && stored.MeterSource != types.CostMeterUpstreamActual {
			return types.CostRuleConfigV1{}, fmt.Errorf("unsupported duration meter source %q", stored.MeterSource)
		}
		if stored.TokenMode != "" || !containsCostMeterSource(capabilities.MeterSources, stored.MeterSource) {
			return types.CostRuleConfigV1{}, errors.New("duration meter source is not supported by the adaptor")
		}
		if stored.ChargeEvent == types.CostChargeSubmitAccepted && stored.MeterSource != types.CostMeterValidatedRequest {
			return types.CostRuleConfigV1{}, errors.New("submit-accepted duration rules require a validated-request meter")
		}
		if err := validatePositiveCostPrice(stored.PricePerSecond, stored.NormalizedUSDPrices.PricePerSecond); err != nil {
			return types.CostRuleConfigV1{}, err
		}
	case types.CostModePerToken:
		if stored.MeterSource != types.CostMeterUpstreamUsage && stored.MeterSource != types.CostMeterLocalUsage {
			return types.CostRuleConfigV1{}, fmt.Errorf("unsupported token meter source %q", stored.MeterSource)
		}
		if !containsCostMeterSource(capabilities.MeterSources, stored.MeterSource) {
			return types.CostRuleConfigV1{}, errors.New("token meter source is not supported by the adaptor")
		}
		if stored.ChargeEvent == types.CostChargeSubmitAccepted {
			return types.CostRuleConfigV1{}, errors.New("token meters are unavailable at submit acceptance")
		}
		pricePairs := [][2]*string{}
		switch stored.TokenMode {
		case types.CostTokenModeTotal:
			pricePairs = append(pricePairs, [2]*string{stored.TotalPerMillion, stored.NormalizedUSDPrices.TotalPerMillion})
		case types.CostTokenModeCompletion:
			pricePairs = append(pricePairs, [2]*string{stored.CompletionPerMillion, stored.NormalizedUSDPrices.CompletionPerMillion})
		case types.CostTokenModeInputOutput:
			pricePairs = append(pricePairs,
				[2]*string{stored.InputPerMillion, stored.NormalizedUSDPrices.InputPerMillion},
				[2]*string{stored.OutputPerMillion, stored.NormalizedUSDPrices.OutputPerMillion},
			)
		default:
			return types.CostRuleConfigV1{}, fmt.Errorf("unsupported token mode %q", stored.TokenMode)
		}
		for _, pair := range pricePairs {
			if err := validatePositiveCostPrice(pair[0], pair[1]); err != nil {
				return types.CostRuleConfigV1{}, err
			}
		}
	}
	return normalized, nil
}

func lookupCostCapabilities(channelType int, requestPath string, taskPlatform constant.TaskPlatform) (types.CostCapabilities, error) {
	if CostCapabilityLookup == nil {
		return types.CostCapabilities{}, errors.New("cost capability lookup is not registered")
	}
	return CostCapabilityLookup(channelType, requestPath, taskPlatform), nil
}

func lookupChannelCostCapabilities(channelType int, requestPath string, taskPlatform constant.TaskPlatform) (types.CostCapabilities, error) {
	capabilities, err := lookupCostCapabilities(channelType, requestPath, taskPlatform)
	if err != nil || requestPath != "" || taskPlatform != "" || capabilities.CanResolveBillableModel {
		return capabilities, err
	}
	return lookupCostCapabilities(channelType, "", constant.TaskPlatform(strconv.Itoa(channelType)))
}

func validCostChargeEvent(event types.CostChargeEvent) bool {
	return event == types.CostChargeResponseSucceeded || event == types.CostChargeSubmitAccepted || event == types.CostChargeTaskSucceeded
}

func containsCostChargeEvent(events []types.CostChargeEvent, expected types.CostChargeEvent) bool {
	for _, event := range events {
		if event == expected {
			return true
		}
	}
	return false
}

func containsCostMeterSource(sources []types.CostMeterSource, expected types.CostMeterSource) bool {
	for _, source := range sources {
		if source == expected {
			return true
		}
	}
	return false
}

func validatePositiveCostPrice(original, normalized *string) error {
	if original == nil || normalized == nil {
		return errors.New("cost rule price is required")
	}
	originalAmount, err := decimal.NewFromString(*original)
	if err != nil || !originalAmount.IsPositive() {
		return errors.New("cost rule price must be positive")
	}
	normalizedAmount, err := decimal.NewFromString(*normalized)
	if err != nil || !normalizedAmount.IsPositive() {
		return errors.New("normalized USD price must be positive")
	}
	if _, err := DecimalToNanoUSD(normalizedAmount); err != nil {
		return fmt.Errorf("normalized USD price is unsupported: %w", err)
	}
	return nil
}

func predictedCostModel(mappingJSON, originModel string) (string, error) {
	mappingJSON = strings.TrimSpace(mappingJSON)
	if mappingJSON == "" {
		mappingJSON = "{}"
	}
	mappedModel, _, err := ResolveMappedModel(originModel, mappingJSON)
	return mappedModel, err
}
