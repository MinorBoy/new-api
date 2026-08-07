package ratio_setting

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
)

// GroupRoutingRequirements describes routing constraints that are enforced for
// every request using a particular billing group.
type GroupRoutingRequirements struct {
	RequireRealPerson *bool `json:"require_real_person,omitempty"`
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
	if err := CheckGroupRoutingRequirements(value); err != nil {
		return err
	}

	return groupRoutingRequirementsMap.UnmarshalJSON([]byte(value))
}

func CheckGroupRoutingRequirements(value string) error {
	if common.GetJsonType([]byte(value)) != "object" {
		return errors.New("group routing requirements must be a JSON object")
	}

	var rawRequirements map[string]json.RawMessage
	if err := common.UnmarshalJsonStr(value, &rawRequirements); err != nil {
		return err
	}
	for groupName, raw := range rawRequirements {
		if strings.TrimSpace(groupName) == "" {
			return errors.New("group routing requirement group name must not be empty")
		}
		if common.GetJsonType(raw) != "object" {
			return fmt.Errorf("group routing requirements for %q must be an object", groupName)
		}
		var requirements GroupRoutingRequirements
		if err := common.DecodeJsonStrict(strings.NewReader(string(raw)), &requirements); err != nil {
			return fmt.Errorf("invalid group routing requirements for %q: %w", groupName, err)
		}
	}
	return nil
}
