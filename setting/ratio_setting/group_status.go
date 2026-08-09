package ratio_setting

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
)

var groupStatusMap = types.NewRWMap[string, bool]()

func ParseGroupStatusJSONString(value string) (map[string]bool, error) {
	statuses := make(map[string]bool)
	if err := common.UnmarshalJsonStr(value, &statuses); err != nil {
		return nil, err
	}
	for name := range statuses {
		if strings.TrimSpace(name) == "" {
			return nil, errors.New("group status name must not be empty")
		}
	}
	return statuses, nil
}

func UpdateGroupStatusByJSONString(value string) error {
	statuses, err := ParseGroupStatusJSONString(value)
	if err != nil {
		return err
	}
	groupStatusMap.Clear()
	groupStatusMap.AddAll(statuses)
	return nil
}

func GroupStatus2JSONString() string {
	return groupStatusMap.MarshalJSONString()
}

func IsGroupEnabled(name string) bool {
	if name == "auto" {
		return true
	}
	enabled, exists := groupStatusMap.Get(name)
	return !exists || enabled
}
