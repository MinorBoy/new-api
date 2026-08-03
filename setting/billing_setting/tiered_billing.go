package billing_setting

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/seedancepricing"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
	"github.com/samber/lo"
)

const (
	BillingModeRatio       = "ratio"
	BillingModeTieredExpr  = "tiered_expr"
	BillingModePerDuration = "per_duration"
	BillingModeField       = "billing_mode"
	BillingExprField       = "billing_expr"
	DurationPriceField     = "duration_price"
)

// BillingSetting is managed by config.GlobalConfig.Register.
// DB keys: billing_setting.billing_mode, billing_setting.billing_expr, billing_setting.duration_price
type BillingSetting struct {
	BillingMode   *types.RWMap[string, string]              `json:"billing_mode"`
	BillingExpr   *types.RWMap[string, string]              `json:"billing_expr"`
	DurationPrice *types.RWMap[string, types.DurationPrice] `json:"duration_price"`
}

var billingSetting = BillingSetting{
	BillingMode:   types.NewRWMap[string, string](),
	BillingExpr:   types.NewRWMap[string, string](),
	DurationPrice: types.NewRWMap[string, types.DurationPrice](),
}

func init() {
	config.GlobalConfig.Register("billing_setting", &billingSetting)
}

// ---------------------------------------------------------------------------
// Read accessors (hot path, must be fast)
// ---------------------------------------------------------------------------

func GetBillingMode(model string) string {
	if mode, ok := billingSetting.BillingMode.Get(model); ok {
		return mode
	}
	return BillingModeRatio
}

func GetBillingExpr(model string) (string, bool) {
	return billingSetting.BillingExpr.Get(model)
}

func GetBillingModeCopy() map[string]string {
	return billingSetting.BillingMode.ReadAll()
}

func ValidateBillingModeJSONString(raw string) error {
	var modes map[string]string
	if err := common.UnmarshalJsonStr(raw, &modes); err != nil {
		return fmt.Errorf("invalid billing mode JSON: %w", err)
	}
	if modes == nil {
		return fmt.Errorf("billing mode must be a JSON object")
	}
	currentModes := GetBillingModeCopy()
	for model, current := range currentModes {
		if seedancepricing.Family(model) == "" {
			continue
		}
		proposed, exists := modes[model]
		if !exists {
			return fmt.Errorf("Seedance billing mode for %s cannot be removed outside config import", model)
		}
		if current != proposed {
			return fmt.Errorf("Seedance billing mode for %s must be updated through config import", model)
		}
	}
	for model := range modes {
		if seedancepricing.Family(model) == "" {
			continue
		}
		if _, exists := currentModes[model]; !exists {
			return fmt.Errorf("Seedance billing mode for %s must be created through config import", model)
		}
	}
	return nil
}

func GetBillingExprCopy() map[string]string {
	return billingSetting.BillingExpr.ReadAll()
}

func GetPricingSyncData(base map[string]any) map[string]any {
	extra := make(map[string]any, 3)
	if modes := GetBillingModeCopy(); len(modes) > 0 {
		extra[BillingModeField] = modes
	}
	if exprs := GetBillingExprCopy(); len(exprs) > 0 {
		extra[BillingExprField] = exprs
	}
	if prices := GetDurationPriceCopy(); len(prices) > 0 {
		extra[DurationPriceField] = prices
	}
	return lo.Assign(base, extra)
}

// ---------------------------------------------------------------------------
// Smoke test (called externally for validation before save)
// ---------------------------------------------------------------------------

func SmokeTestExpr(exprStr string) error {
	return smokeTestExpr(exprStr)
}

func smokeTestExpr(exprStr string) error {
	vectors := []billingexpr.TokenParams{
		{P: 0, C: 0, Len: 0},
		{P: 1000, C: 1000, Len: 1000},
		{P: 100000, C: 100000, Len: 100000},
		{P: 1000000, C: 1000000, Len: 1000000},
	}
	requests := []billingexpr.RequestInput{
		{},
		{
			Headers: map[string]string{
				"anthropic-beta": "fast-mode-2026-02-01",
			},
			Body: []byte(`{"service_tier":"fast","stream_options":{"include_usage":true},"messages":[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21]}`),
		},
	}

	for _, v := range vectors {
		for _, request := range requests {
			result, _, err := billingexpr.RunExprWithRequest(exprStr, v, request)
			if err != nil {
				return fmt.Errorf("vector {p=%g, c=%g}: run failed: %w", v.P, v.C, err)
			}
			if result < 0 {
				return fmt.Errorf("vector {p=%g, c=%g}: result %f < 0", v.P, v.C, result)
			}
		}
	}
	return nil
}
