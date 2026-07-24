package cost_setting

import (
	"fmt"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
)

const (
	ConfigName = "cost_setting"
	KeyMode    = "mode"
)

type CostSetting struct {
	Mode types.CostAccountingMode `json:"mode"`
}

type RuntimeSnapshot struct {
	Mode types.CostAccountingMode
}

var costSetting = CostSetting{Mode: types.CostAccountingDisabled}

var runtimeSnapshot atomic.Value

func init() {
	config.GlobalConfig.Register(ConfigName, &costSetting)
	UpdateAndSync()
}

func ValidateMode(mode types.CostAccountingMode) error {
	switch mode {
	case types.CostAccountingDisabled, types.CostAccountingStrict:
		return nil
	default:
		return fmt.Errorf("unsupported cost accounting mode %q", mode)
	}
}

func Runtime() RuntimeSnapshot {
	if loaded := runtimeSnapshot.Load(); loaded != nil {
		if snapshot, ok := loaded.(RuntimeSnapshot); ok {
			return snapshot
		}
	}
	return RuntimeSnapshot{Mode: types.CostAccountingDisabled}
}

func UpdateAndSync() {
	mode := costSetting.Mode
	if err := ValidateMode(mode); err != nil {
		common.SysError(err.Error() + "; using disabled mode")
		mode = types.CostAccountingDisabled
	}
	runtimeSnapshot.Store(RuntimeSnapshot{Mode: mode})
}
