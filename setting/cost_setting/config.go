package cost_setting

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
)

const (
	ConfigName                  = "cost_setting"
	KeyMode                     = "mode"
	KeyMinimumExpectedMarginBPS = "minimum_expected_margin_bps"
)

type CostSetting struct {
	Mode                     types.CostAccountingMode `json:"mode"`
	MinimumExpectedMarginBPS int                      `json:"minimum_expected_margin_bps"`
}

type RuntimeSnapshot struct {
	Mode                     types.CostAccountingMode
	MinimumExpectedMarginBPS int
}

var costSetting = CostSetting{Mode: types.CostAccountingDisabled}

var runtimeSnapshot atomic.Value
var runtimeUpdateGuard sync.RWMutex

func init() {
	config.GlobalConfig.Register(ConfigName, &costSetting)
	UpdateAndSync()
}

func ValidateMode(mode types.CostAccountingMode) error {
	switch mode {
	case types.CostAccountingDisabled, types.CostAccountingTracking, types.CostAccountingStrict:
		return nil
	default:
		return fmt.Errorf("unsupported cost accounting mode %q", mode)
	}
}

func ValidateMinimumExpectedMarginBPS(value int) error {
	if value < 0 || value > 10_000 {
		return fmt.Errorf("minimum expected margin must be between 0 and 10000 basis points")
	}
	return nil
}

func Runtime() RuntimeSnapshot {
	runtimeUpdateGuard.RLock()
	defer runtimeUpdateGuard.RUnlock()
	return runtimeSnapshotValue()
}

func WithRuntimeReadLock(fn func(RuntimeSnapshot) error) error {
	runtimeUpdateGuard.RLock()
	defer runtimeUpdateGuard.RUnlock()
	return fn(runtimeSnapshotValue())
}

func runtimeSnapshotValue() RuntimeSnapshot {
	if loaded := runtimeSnapshot.Load(); loaded != nil {
		if snapshot, ok := loaded.(RuntimeSnapshot); ok {
			return snapshot
		}
	}
	return RuntimeSnapshot{Mode: types.CostAccountingDisabled, MinimumExpectedMarginBPS: 0}
}

func UpdateAndSync() {
	runtimeUpdateGuard.Lock()
	defer runtimeUpdateGuard.Unlock()

	mode := costSetting.Mode
	if err := ValidateMode(mode); err != nil {
		common.SysError(err.Error() + "; using disabled mode")
		mode = types.CostAccountingDisabled
	}
	minimumExpectedMarginBPS := costSetting.MinimumExpectedMarginBPS
	if err := ValidateMinimumExpectedMarginBPS(minimumExpectedMarginBPS); err != nil {
		common.SysError(err.Error() + "; using zero minimum expected margin")
		minimumExpectedMarginBPS = 0
	}
	runtimeSnapshot.Store(RuntimeSnapshot{
		Mode:                     mode,
		MinimumExpectedMarginBPS: minimumExpectedMarginBPS,
	})
}
