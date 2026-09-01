package image_setting

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

const RoutingOptionKey = "ImageRoutingPolicy"

type Strategy string

const (
	StrategyManual       Strategy = "manual"
	StrategyLowestCost   Strategy = "lowest_cost"
	StrategyCostWeighted Strategy = "cost_weighted"
)

type Policy struct {
	Strategy                 Strategy `json:"strategy"`
	MinimumExpectedMarginBPS *int     `json:"minimum_expected_margin_bps,omitempty"`
	CostToleranceBPS         *int     `json:"cost_tolerance_bps,omitempty"`
	RequireCompatibilityTest bool     `json:"require_compatibility_test"`
}

type RoutingConfig struct {
	Version int                          `json:"version"`
	Default Policy                       `json:"default"`
	Groups  map[string]map[string]Policy `json:"groups,omitempty"`
}

var (
	routingMu      sync.RWMutex
	currentRouting = RoutingConfig{Version: CatalogVersion, Default: Policy{Strategy: StrategyManual}}
)

func ParseRoutingJSONString(raw string) (RoutingConfig, error) {
	var config RoutingConfig
	if common.GetJsonType([]byte(raw)) != "object" {
		return RoutingConfig{}, errors.New("image routing policy must be a JSON object")
	}
	if err := common.UnmarshalJsonStr(raw, &config); err != nil {
		return RoutingConfig{}, err
	}
	if err := ValidateRoutingConfig(config); err != nil {
		return RoutingConfig{}, err
	}
	return cloneRouting(config), nil
}

func ValidateRoutingConfig(config RoutingConfig) error {
	if config.Version != CatalogVersion {
		return fmt.Errorf("image routing policy version must be %d", CatalogVersion)
	}
	if err := validatePolicy(config.Default); err != nil {
		return fmt.Errorf("image routing policy default: %w", err)
	}
	for group, models := range config.Groups {
		if strings.TrimSpace(group) == "" {
			return errors.New("image routing policy group must not be empty")
		}
		for model, policy := range models {
			if strings.TrimSpace(model) == "" {
				return fmt.Errorf("image routing policy group %q model must not be empty", group)
			}
			if err := validatePolicy(policy); err != nil {
				return fmt.Errorf("image routing policy group %q model %q: %w", group, model, err)
			}
		}
	}
	return nil
}

func validatePolicy(policy Policy) error {
	if policy.Strategy != StrategyManual && policy.Strategy != StrategyLowestCost && policy.Strategy != StrategyCostWeighted {
		return fmt.Errorf("strategy must be manual, lowest_cost, or cost_weighted")
	}
	if policy.MinimumExpectedMarginBPS != nil && (*policy.MinimumExpectedMarginBPS < 0 || *policy.MinimumExpectedMarginBPS > 10000) {
		return errors.New("minimum_expected_margin_bps must be between 0 and 10000")
	}
	if policy.CostToleranceBPS != nil {
		if policy.Strategy != StrategyCostWeighted {
			return errors.New("cost_tolerance_bps is only valid for cost_weighted strategy")
		}
		if *policy.CostToleranceBPS < 0 || *policy.CostToleranceBPS > 10000 {
			return errors.New("cost_tolerance_bps must be between 0 and 10000")
		}
	}
	return nil
}

func UpdateRoutingByJSONString(raw string) error {
	config, err := ParseRoutingJSONString(raw)
	if err != nil {
		return err
	}
	routingMu.Lock()
	currentRouting = config
	routingMu.Unlock()
	return nil
}

func Routing2JSONString() string {
	routingMu.RLock()
	config := cloneRouting(currentRouting)
	routingMu.RUnlock()
	encoded, err := common.Marshal(config)
	if err != nil {
		common.SysError("failed to marshal image routing policy: " + err.Error())
		return "{}"
	}
	return string(encoded)
}

func RoutingSnapshot() RoutingConfig {
	routingMu.RLock()
	defer routingMu.RUnlock()
	return cloneRouting(currentRouting)
}

func PolicyFor(group, model string) Policy {
	config := RoutingSnapshot()
	if models, ok := config.Groups[strings.TrimSpace(group)]; ok {
		if policy, ok := models[strings.TrimSpace(model)]; ok {
			return policy
		}
	}
	return config.Default
}

func cloneRouting(config RoutingConfig) RoutingConfig {
	clone := RoutingConfig{Version: config.Version, Default: clonePolicy(config.Default)}
	if config.Groups == nil {
		return clone
	}
	clone.Groups = make(map[string]map[string]Policy, len(config.Groups))
	for group, models := range config.Groups {
		clone.Groups[group] = make(map[string]Policy, len(models))
		for model, policy := range models {
			clone.Groups[group][model] = clonePolicy(policy)
		}
	}
	return clone
}

func clonePolicy(policy Policy) Policy {
	if policy.MinimumExpectedMarginBPS != nil {
		value := *policy.MinimumExpectedMarginBPS
		policy.MinimumExpectedMarginBPS = &value
	}
	if policy.CostToleranceBPS != nil {
		value := *policy.CostToleranceBPS
		policy.CostToleranceBPS = &value
	}
	return policy
}
