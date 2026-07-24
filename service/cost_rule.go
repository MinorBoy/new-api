package service

import (
	"errors"
	"fmt"
	"reflect"
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
	Covered                bool
	Reason                 string
}

type costCoverageKey struct {
	channelID int
	model     string
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
	capabilities, err := lookupCostCapabilities(channel.Type, input.RequestPath, input.TaskPlatform)
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
				Where("channel_id = ? AND billable_upstream_model = ?", candidate.ChannelID, candidate.BillableUpstreamModel).
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
	capabilities, err := lookupCostCapabilities(channel.Type, input.RequestPath, input.TaskPlatform)
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
	capabilities, err := lookupCostCapabilities(channel.Type, "", "")
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
	InvalidateCostCoverage(activated.ChannelID, activated.BillableUpstreamModel)
	return activated, nil
}

func RetireCostRule(id int64, adminID int) error {
	var rule model.ChannelModelCostRule
	if err := model.DB.Select("id", "channel_id", "billable_upstream_model").Where("id = ?", id).First(&rule).Error; err != nil {
		return err
	}
	if err := model.RetireChannelModelCostRule(id, adminID, common.GetTimestamp()); err != nil {
		return err
	}
	InvalidateCostCoverage(rule.ChannelID, rule.BillableUpstreamModel)
	return nil
}

func ActiveCostRule(channelID int, billableModel string, authoritative bool) (*model.ChannelModelCostRule, error) {
	key := costCoverageKey{channelID: channelID, model: strings.TrimSpace(billableModel)}
	if !authoritative {
		if cached, ok := activeCostRuleCache.Load(key); ok {
			rule := cached.(model.ChannelModelCostRule)
			return &rule, nil
		}
	}

	var rules []model.ChannelModelCostRule
	err := model.DB.Where(
		"channel_id = ? AND billable_upstream_model = ? AND status = ?",
		key.channelID, key.model, types.CostRuleActive,
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

func CheckPredictedCostCoverage(input PredictedCoverageInput) (bool, error) {
	channel, err := model.GetChannelById(input.ChannelID, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	rule, err := ActiveCostRule(input.ChannelID, input.PredictedUpstreamModel, input.Authoritative)
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
		capabilities, err := lookupCostCapabilities(channel.Type, target.RequestPath, target.TaskPlatform)
		if err != nil {
			return false, nil
		}
		if _, err := validateCostRuleContract(rule, capabilities); err != nil {
			return false, nil
		}
	}
	return true, nil
}

func InvalidateCostCoverage(channelID int, billableModel string) {
	activeCostRuleCache.Range(func(key, _ any) bool {
		coverageKey, ok := key.(costCoverageKey)
		if !ok {
			activeCostRuleCache.Delete(key)
			return true
		}
		channelMatches := channelID == 0 || coverageKey.channelID == channelID
		modelMatches := billableModel == "" || coverageKey.model == billableModel
		if channelMatches && modelMatches {
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

func ListCostRules(channelID int, billableModel string) ([]model.ChannelModelCostRule, error) {
	query := model.DB.Model(&model.ChannelModelCostRule{})
	if channelID > 0 {
		query = query.Where("channel_id = ?", channelID)
	}
	if strings.TrimSpace(billableModel) != "" {
		query = query.Where("billable_upstream_model = ?", strings.TrimSpace(billableModel))
	}
	var rules []model.ChannelModelCostRule
	err := query.Order("channel_id ASC, billable_upstream_model ASC, version DESC").Find(&rules).Error
	return rules, err
}

func CostRuleHistory(id int64) ([]model.ChannelModelCostRule, error) {
	rule, err := GetCostRule(id)
	if err != nil {
		return nil, err
	}
	return ListCostRules(rule.ChannelID, rule.BillableUpstreamModel)
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
	capabilities, err := lookupCostCapabilities(channel.Type, "", "")
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
	results := make([]CostCoverageResult, 0, len(abilities))
	seen := make(map[costCoverageKey]struct{}, len(abilities))
	for _, ability := range abilities {
		channel, err := model.GetChannelById(ability.ChannelId, false)
		if err != nil {
			return nil, err
		}
		predictedModel, err := predictedCostModel(channel.GetModelMapping(), ability.Model)
		result := CostCoverageResult{
			ChannelID:              ability.ChannelId,
			OriginModel:            ability.Model,
			PredictedUpstreamModel: predictedModel,
		}
		if err != nil {
			result.Reason = "invalid_model_mapping"
			results = append(results, result)
			continue
		}
		key := costCoverageKey{channelID: ability.ChannelId, model: predictedModel}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result.Covered, err = CheckPredictedCostCoverage(PredictedCoverageInput{
			ChannelID:              ability.ChannelId,
			PredictedUpstreamModel: predictedModel,
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
	now := common.GetTimestamp()
	rule := &model.ChannelModelCostRule{
		ChannelID:             input.ChannelID,
		BillableUpstreamModel: strings.TrimSpace(input.BillableUpstreamModel),
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
	if strings.TrimSpace(mappingJSON) == "" || strings.TrimSpace(mappingJSON) == "{}" {
		return originModel, nil
	}
	var mapping map[string]string
	if err := common.UnmarshalJsonStr(mappingJSON, &mapping); err != nil {
		return "", err
	}
	current := originModel
	visited := map[string]struct{}{current: {}}
	for {
		next := strings.TrimSpace(mapping[current])
		if next == "" || next == current {
			return current, nil
		}
		if _, ok := visited[next]; ok {
			return "", errors.New("model mapping contains a cycle")
		}
		visited[next] = struct{}{}
		current = next
	}
}
