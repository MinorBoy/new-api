package ratio_setting

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
)

type GroupRoutingProfileStatus string

type GroupRealPersonMode string

const (
	GroupRoutingProfileDraft  GroupRoutingProfileStatus = "draft"
	GroupRoutingProfileActive GroupRoutingProfileStatus = "active"

	GroupRealPersonAny       GroupRealPersonMode = "any"
	GroupRealPersonRequired  GroupRealPersonMode = "required"
	GroupRealPersonForbidden GroupRealPersonMode = "forbidden"

	GroupRoutingSourceDefault  = "default"
	maxGroupRoutingProfiles    = 200
	maxExcludedTargetsPerGroup = 500
	maxExcludedTargetKeyLength = 128
)

// GroupRoutingRequirements describes routing constraints that are enforced for
// every request using a particular billing group.
type GroupRoutingRequirements struct {
	RequireRealPerson  *bool                     `json:"require_real_person,omitempty"`
	Status             GroupRoutingProfileStatus `json:"status,omitempty"`
	RoutingSource      string                    `json:"routing_source,omitempty"`
	RealPersonMode     GroupRealPersonMode       `json:"real_person_mode,omitempty"`
	AllowedCostModes   []types.CostMode          `json:"allowed_cost_modes,omitempty"`
	ExcludedTargetKeys []string                  `json:"excluded_target_keys,omitempty"`
}

func (r GroupRoutingRequirements) IsDynamic() bool {
	return r.RoutingSource != ""
}

func (r GroupRoutingRequirements) EffectiveRealPersonMode() GroupRealPersonMode {
	if r.RealPersonMode != "" {
		return r.RealPersonMode
	}
	if r.RequireRealPerson != nil && *r.RequireRealPerson {
		return GroupRealPersonRequired
	}
	return GroupRealPersonAny
}

var groupRoutingRequirementsMap = types.NewRWMap[string, GroupRoutingRequirements]()

func GetGroupRoutingRequirements(group string) GroupRoutingRequirements {
	requirements, ok := groupRoutingRequirementsMap.Get(group)
	if !ok {
		return GroupRoutingRequirements{}
	}
	return requirements
}

func GroupRoutingRequirements2JSONString() string {
	return groupRoutingRequirementsMap.MarshalJSONString()
}

func UpdateGroupRoutingRequirementsByJSONString(value string) error {
	profiles, err := ParseGroupRoutingRequirementsJSONString(value)
	if err != nil {
		return err
	}
	encoded, err := common.Marshal(profiles)
	if err != nil {
		return err
	}
	return groupRoutingRequirementsMap.UnmarshalJSON(encoded)
}

func CheckGroupRoutingRequirements(value string) error {
	_, err := ParseGroupRoutingRequirementsJSONString(value)
	return err
}

func ParseGroupRoutingRequirementsJSONString(value string) (map[string]GroupRoutingRequirements, error) {
	if common.GetJsonType([]byte(value)) != "object" {
		return nil, errors.New("group routing requirements must be a JSON object")
	}

	var rawRequirements map[string]json.RawMessage
	if err := common.UnmarshalJsonStr(value, &rawRequirements); err != nil {
		return nil, err
	}
	if len(rawRequirements) > maxGroupRoutingProfiles {
		return nil, fmt.Errorf("group routing requirements must not contain more than %d profiles", maxGroupRoutingProfiles)
	}

	profiles := make(map[string]GroupRoutingRequirements, len(rawRequirements))
	for groupName, raw := range rawRequirements {
		if strings.TrimSpace(groupName) == "" {
			return nil, errors.New("group routing requirement group name must not be empty")
		}
		if common.GetJsonType(raw) != "object" {
			return nil, fmt.Errorf("group routing requirements for %q must be an object", groupName)
		}
		var requirements GroupRoutingRequirements
		if err := common.DecodeJsonStrict(strings.NewReader(string(raw)), &requirements); err != nil {
			return nil, fmt.Errorf("invalid group routing requirements for %q: %w", groupName, err)
		}

		if groupName == "auto" {
			return nil, errors.New("group routing requirements must not define the auto pseudo group")
		}
		if requirements.IsDynamic() {
			if groupName == GroupRoutingSourceDefault {
				return nil, errors.New("default group routing requirements must not inherit from itself")
			}
			if requirements.RoutingSource != GroupRoutingSourceDefault {
				return nil, fmt.Errorf("invalid routing source %q for group %q", requirements.RoutingSource, groupName)
			}
			if requirements.Status != GroupRoutingProfileDraft && requirements.Status != GroupRoutingProfileActive {
				return nil, fmt.Errorf("dynamic group routing requirements for %q must have draft or active status", groupName)
			}
		} else if requirements.Status != "" && requirements.Status != GroupRoutingProfileDraft && requirements.Status != GroupRoutingProfileActive {
			return nil, fmt.Errorf("invalid group routing profile status %q for group %q", requirements.Status, groupName)
		}

		switch requirements.RealPersonMode {
		case "", GroupRealPersonAny, GroupRealPersonRequired, GroupRealPersonForbidden:
		default:
			return nil, fmt.Errorf("invalid real person mode %q for group %q", requirements.RealPersonMode, groupName)
		}
		if requirements.RequireRealPerson != nil && requirements.RealPersonMode != "" {
			legacyMode := GroupRealPersonAny
			if *requirements.RequireRealPerson {
				legacyMode = GroupRealPersonRequired
			}
			if requirements.RealPersonMode != legacyMode {
				return nil, fmt.Errorf("conflicting real person requirements for group %q", groupName)
			}
		}

		for _, costMode := range requirements.AllowedCostModes {
			switch costMode {
			case types.CostModeFree, types.CostModePerRequest, types.CostModePerDuration, types.CostModePerToken:
			default:
				return nil, fmt.Errorf("invalid allowed cost mode %q for group %q", costMode, groupName)
			}
		}
		sort.Slice(requirements.AllowedCostModes, func(i, j int) bool {
			return requirements.AllowedCostModes[i] < requirements.AllowedCostModes[j]
		})
		allowedCostModes := requirements.AllowedCostModes[:0]
		for _, costMode := range requirements.AllowedCostModes {
			if len(allowedCostModes) == 0 || allowedCostModes[len(allowedCostModes)-1] != costMode {
				allowedCostModes = append(allowedCostModes, costMode)
			}
		}
		requirements.AllowedCostModes = allowedCostModes

		if len(requirements.ExcludedTargetKeys) > maxExcludedTargetsPerGroup {
			return nil, fmt.Errorf("group %q must not exclude more than %d routing targets", groupName, maxExcludedTargetsPerGroup)
		}
		for _, targetKey := range requirements.ExcludedTargetKeys {
			if strings.TrimSpace(targetKey) == "" {
				return nil, fmt.Errorf("excluded target key for group %q must not be empty", groupName)
			}
			if len(targetKey) > maxExcludedTargetKeyLength {
				return nil, fmt.Errorf("excluded target key for group %q must not exceed %d bytes", groupName, maxExcludedTargetKeyLength)
			}
		}
		sort.Strings(requirements.ExcludedTargetKeys)
		excludedTargetKeys := requirements.ExcludedTargetKeys[:0]
		for _, targetKey := range requirements.ExcludedTargetKeys {
			if len(excludedTargetKeys) == 0 || excludedTargetKeys[len(excludedTargetKeys)-1] != targetKey {
				excludedTargetKeys = append(excludedTargetKeys, targetKey)
			}
		}
		requirements.ExcludedTargetKeys = excludedTargetKeys
		profiles[groupName] = requirements
	}
	return profiles, nil
}
