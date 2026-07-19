# Per-Duration Model Pricing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a first-class, provider-independent `per_duration` billing mode, migrate Dimensio Seedance 2.0 to it, expose complete admin/public frontend support, and verify the full ARK-to-Dimensio lifecycle with cost-free mock E2E tests.

**Architecture:** Store duration prices as structured per-model rules under `billing_setting.duration_price`; keep `ModelPrice` exclusively fixed-per-request. Task adaptors explicitly supply validated request duration while the central relay applies minimum duration, step rounding, units, group ratios, non-duration ratios, quota saturation auditing, snapshots, refunds, and logs. Price lookup always uses the client-facing origin model, while mapped upstream models are used only for provider capability and transport.

**Tech Stack:** Go 1.22+, Gin, GORM v2, shopspring/decimal, testify, React 19, TypeScript, Base UI/shadcn, Tailwind CSS, i18next, Bun, Go `httptest` mock E2E.

---

## Inputs and Constraints

- Design: `docs/superpowers/specs/2026-07-19-per-duration-billing-design.md`
- Existing Dimensio plan: `docs/superpowers/plans/2026-07-18-dimensio-translator.md`
- Provider contract: `docs/channel/jimeng.dimensio.cn.md`
- Existing acceptance report: `docs/superpowers/reports/2026-07-19-dimensio-e2e-acceptance-report.md`
- Billing expression contract: `pkg/billingexpr/expr.md`
- JSON operations must use `common.*` wrappers.
- Billing multipliers must be bounded before quota calculation.
- Quota conversion must use a checked helper from `common/quota_math.go` and surface its clamp on `RelayInfo`.
- Frontend work must load and follow `i18n-translate`, `shadcn-ui`, and `vercel-react-best-practices` before editing.
- Frontend commands use Bun.
- All three databases remain supported; this feature adds no schema migration.

## File Structure

- Create `types/duration_price.go` and tests for the shared duration rule.
- Create `setting/billing_setting/duration_billing.go` and tests for effective defaults and validation.
- Modify `types/price_data.go`, `relay/channel/adapter.go`, `relay/helper/price.go`, and `relay/relay_task.go` for central task billing.
- Modify the Dimensio adaptor, task snapshot, logs, and settlement tests.
- Modify pricing/sync APIs and the default frontend admin/public pricing surfaces.
- Extend mock E2E and update the provider document, original plan, and acceptance report.

---

### Task 1: Define and Validate the Duration Pricing Domain

**Files:**
- Create: `types/duration_price.go`
- Create: `types/duration_price_test.go`
- Create: `setting/billing_setting/duration_billing.go`
- Create: `setting/billing_setting/duration_billing_test.go`
- Modify: `setting/billing_setting/tiered_billing.go`

- [ ] **Step 1: Write failing shared-domain tests**

Create `types/duration_price_test.go`:

```go
package types

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDurationPriceValidate(t *testing.T) {
	valid := DurationPrice{Price: 0.5, Unit: DurationUnitMinute, RoundingStepSeconds: 5, MinimumDurationSeconds: 4}
	require.NoError(t, valid.Validate(3600))
	for name, rule := range map[string]DurationPrice{
		"negative price": {Price: -1, Unit: DurationUnitSecond, RoundingStepSeconds: 1},
		"nan price":      {Price: math.NaN(), Unit: DurationUnitSecond, RoundingStepSeconds: 1},
		"bad unit":       {Price: 1, Unit: "hour", RoundingStepSeconds: 1},
		"zero step":      {Price: 1, Unit: DurationUnitSecond},
		"large step":     {Price: 1, Unit: DurationUnitSecond, RoundingStepSeconds: 3601},
		"negative min":   {Price: 1, Unit: DurationUnitSecond, RoundingStepSeconds: 1, MinimumDurationSeconds: -1},
		"large min":      {Price: 1, Unit: DurationUnitSecond, RoundingStepSeconds: 1, MinimumDurationSeconds: 3601},
	} {
		t.Run(name, func(t *testing.T) { assert.Error(t, rule.Validate(3600)) })
	}
}

func TestDurationPriceBillableSeconds(t *testing.T) {
	rule := DurationPrice{Price: 1, Unit: DurationUnitSecond, RoundingStepSeconds: 5, MinimumDurationSeconds: 4}
	for _, test := range []struct{ requested, expected int }{{1, 5}, {4, 5}, {5, 5}, {6, 10}, {11, 15}} {
		actual, err := rule.BillableSeconds(test.requested, 3600)
		require.NoError(t, err)
		assert.Equal(t, test.expected, actual)
	}
	_, err := rule.BillableSeconds(3601, 3600)
	assert.Error(t, err)
}
```

- [ ] **Step 2: Verify the tests fail before implementation**

```bash
go test ./types -run 'TestDurationPrice' -count=1
```

Expected: FAIL because `DurationPrice` and its unit constants are undefined.

- [ ] **Step 3: Implement the shared duration rule**

Create `types/duration_price.go`:

```go
package types

import (
	"fmt"
	"math"
)

const (
	DurationUnitSecond    = "second"
	DurationUnitMinute    = "minute"
	DurationSourceRequest = "request"
)

type DurationPrice struct {
	Price                  float64 `json:"price"`
	Unit                   string  `json:"unit"`
	RoundingStepSeconds    int     `json:"rounding_step_seconds"`
	MinimumDurationSeconds int     `json:"minimum_duration_seconds"`
}

func (p DurationPrice) Validate(maxSeconds int) error {
	if p.Price < 0 || math.IsNaN(p.Price) || math.IsInf(p.Price, 0) {
		return fmt.Errorf("duration price must be a finite non-negative number")
	}
	if p.Unit != DurationUnitSecond && p.Unit != DurationUnitMinute {
		return fmt.Errorf("duration unit must be second or minute")
	}
	if p.RoundingStepSeconds <= 0 || p.RoundingStepSeconds > maxSeconds {
		return fmt.Errorf("rounding_step_seconds must be between 1 and %d", maxSeconds)
	}
	if p.MinimumDurationSeconds < 0 || p.MinimumDurationSeconds > maxSeconds {
		return fmt.Errorf("minimum_duration_seconds must be between 0 and %d", maxSeconds)
	}
	return nil
}

func (p DurationPrice) UnitSeconds() int {
	if p.Unit == DurationUnitMinute {
		return 60
	}
	return 1
}

func (p DurationPrice) BillableSeconds(requested, maxSeconds int) (int, error) {
	if err := p.Validate(maxSeconds); err != nil {
		return 0, err
	}
	if requested <= 0 || requested > maxSeconds {
		return 0, fmt.Errorf("requested duration must be between 1 and %d seconds", maxSeconds)
	}
	normalized := requested
	if normalized < p.MinimumDurationSeconds {
		normalized = p.MinimumDurationSeconds
	}
	return ((normalized + p.RoundingStepSeconds - 1) / p.RoundingStepSeconds) * p.RoundingStepSeconds, nil
}
```

- [ ] **Step 4: Verify the shared-domain tests pass**

```bash
go test ./types -run 'TestDurationPrice' -count=1
```

Expected: PASS.

- [ ] **Step 5: Write failing default and override tests**

Create `setting/billing_setting/duration_billing_test.go`:

```go
package billing_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDimensioDurationPriceDefaults(t *testing.T) {
	tests := map[string]float64{
		"jimeng-video-seedance-2.0-fast-vip": 0.48 / 7.3,
		"jimeng-video-seedance-2.0-mini": 0.39 / 7.3,
		"jimeng-video-seedance-2.0-vip": 0.62 / 7.3,
	}
	for model, expected := range tests {
		assert.Equal(t, BillingModePerDuration, GetBillingMode(model))
		rule, ok := GetDurationPrice(model)
		require.True(t, ok)
		assert.InDelta(t, expected, rule.Price, 1e-10)
		assert.Equal(t, types.DurationUnitSecond, rule.Unit)
		assert.Equal(t, 1, rule.RoundingStepSeconds)
		assert.Equal(t, 4, rule.MinimumDurationSeconds)
	}
}

func TestDurationPriceConfiguredRuleOverridesDefault(t *testing.T) {
	model := "jimeng-video-seedance-2.0-vip"
	originalModes, originalPrices := billingSetting.BillingMode, billingSetting.DurationPrice
	t.Cleanup(func() { billingSetting.BillingMode, billingSetting.DurationPrice = originalModes, originalPrices })
	billingSetting.BillingMode = map[string]string{model: BillingModeRatio}
	billingSetting.DurationPrice = map[string]types.DurationPrice{
		model: {Price: 9, Unit: types.DurationUnitMinute, RoundingStepSeconds: 60},
	}
	assert.Equal(t, BillingModeRatio, GetBillingMode(model))
	rule, ok := GetDurationPrice(model)
	require.True(t, ok)
	assert.Equal(t, 9.0, rule.Price)
}
```

- [ ] **Step 6: Register the new mode, map, defaults, and getters**

Modify `setting/billing_setting/tiered_billing.go`:

```go
const (
	BillingModeRatio       = "ratio"
	BillingModeTieredExpr  = "tiered_expr"
	BillingModePerDuration = "per_duration"
	BillingModeField       = "billing_mode"
	BillingExprField       = "billing_expr"
	DurationPriceField     = "duration_price"
)

type BillingSetting struct {
	BillingMode   map[string]string              `json:"billing_mode"`
	BillingExpr   map[string]string              `json:"billing_expr"`
	DurationPrice map[string]types.DurationPrice `json:"duration_price"`
}
```

Initialize all three maps. Make `GetBillingMode` return an explicit configured
mode first, then `per_duration` for a built-in default rule, then `ratio`.

Create `setting/billing_setting/duration_billing.go`:

```go
package billing_setting

import (
	"github.com/QuantumNous/new-api/types"
	"github.com/samber/lo"
)

var defaultDurationPrice = map[string]types.DurationPrice{
	"jimeng-video-seedance-2.0-fast-vip": {Price: 0.48 / 7.3, Unit: types.DurationUnitSecond, RoundingStepSeconds: 1, MinimumDurationSeconds: 4},
	"jimeng-video-seedance-2.0-mini": {Price: 0.39 / 7.3, Unit: types.DurationUnitSecond, RoundingStepSeconds: 1, MinimumDurationSeconds: 4},
	"jimeng-video-seedance-2.0-vip": {Price: 0.62 / 7.3, Unit: types.DurationUnitSecond, RoundingStepSeconds: 1, MinimumDurationSeconds: 4},
}

func GetDurationPrice(model string) (types.DurationPrice, bool) {
	if price, ok := billingSetting.DurationPrice[model]; ok {
		return price, true
	}
	price, ok := defaultDurationPrice[model]
	return price, ok
}

func GetDurationPriceCopy() map[string]types.DurationPrice {
	return lo.Assign(defaultDurationPrice, billingSetting.DurationPrice)
}

func GetDefaultDurationPriceMap() map[string]types.DurationPrice {
	return lo.Assign(defaultDurationPrice)
}
```

Update `GetBillingModeCopy` to merge built-in duration modes first and explicit
values second. Include the effective duration map in `GetPricingSyncData`.

- [ ] **Step 7: Run configuration tests and commit**

```bash
go test ./setting/billing_setting ./types -count=1
git add types/duration_price.go types/duration_price_test.go setting/billing_setting/duration_billing.go setting/billing_setting/duration_billing_test.go setting/billing_setting/tiered_billing.go
git commit -m "feat(billing): add per-duration pricing rules"
```

Expected: tests PASS and the commit contains only the listed files.

---

### Task 2: Protect Configuration Updates and Effective Defaults

**Files:**
- Modify: `setting/billing_setting/duration_billing.go`
- Modify: `setting/billing_setting/duration_billing_test.go`
- Modify: `controller/option.go`

- [ ] **Step 1: Add failing JSON validation tests**

Append:

```go
func TestValidateDurationPriceJSONString(t *testing.T) {
	require.NoError(t, ValidateDurationPriceJSONString(`{"video":{"price":1.5,"unit":"minute","rounding_step_seconds":5,"minimum_duration_seconds":10}}`))
	for _, raw := range []string{
		`[]`,
		`{"video":{"price":-1,"unit":"second","rounding_step_seconds":1,"minimum_duration_seconds":0}}`,
		`{"video":{"price":1,"unit":"hour","rounding_step_seconds":1,"minimum_duration_seconds":0}}`,
		`{"video":{"price":1,"unit":"second","rounding_step_seconds":0,"minimum_duration_seconds":0}}`,
	} {
		assert.Error(t, ValidateDurationPriceJSONString(raw))
	}
}
```

- [ ] **Step 2: Implement non-mutating validation and effective JSON**

Add to `duration_billing.go`:

```go
func ValidateDurationPriceJSONString(raw string) error {
	var prices map[string]types.DurationPrice
	if err := common.UnmarshalJsonStr(raw, &prices); err != nil {
		return fmt.Errorf("invalid duration price JSON: %w", err)
	}
	for model, price := range prices {
		if strings.TrimSpace(model) == "" {
			return fmt.Errorf("duration price model name cannot be empty")
		}
		if err := price.Validate(relaycommon.MaxTaskDurationSeconds); err != nil {
			return fmt.Errorf("invalid duration price for %s: %w", model, err)
		}
	}
	return nil
}

func DurationPrice2JSONString() string {
	data, err := common.Marshal(GetDurationPriceCopy())
	if err != nil {
		return "{}"
	}
	return string(data)
}
```

Import `fmt`, `strings`, project `common`, and `relay/common` as
`relaycommon`.

- [ ] **Step 3: Reject invalid option updates before persistence**

In `controller/option.go`, import `setting/billing_setting` and add:

```go
case "billing_setting.duration_price":
	if err := billing_setting.ValidateDurationPriceJSONString(option.Value.(string)); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
```

When `GetOptions` visits `billing_setting.billing_mode` or
`billing_setting.duration_price`, marshal effective copies instead of returning
a stale persisted map. This makes built-in Dimensio defaults visible after an
upgrade even if an older billing map already exists.

- [ ] **Step 4: Run focused validation and commit**

```bash
go test ./setting/billing_setting ./controller -run 'Duration|Option' -count=1
go vet ./setting/billing_setting ./controller
git add setting/billing_setting/duration_billing.go setting/billing_setting/duration_billing_test.go controller/option.go
git commit -m "feat(billing): validate duration price settings"
```

Expected: tests and vet PASS.

---

### Task 3: Add the Central Duration Task Calculator

**Files:**
- Modify: `types/price_data.go`
- Modify: `relay/channel/adapter.go`
- Modify: `relay/helper/price.go`
- Modify: `relay/helper/price_test.go`
- Modify: `relay/relay_task.go`
- Modify: `relay/relay_task_billing_test.go`

- [ ] **Step 1: Write failing exact-quota tests**

Append to `relay/relay_task_billing_test.go`:

```go
func TestTaskDurationQuota(t *testing.T) {
	tests := []struct {
		name string
		rule types.DurationPrice
		requested int
		groupRatio float64
		ratios map[string]float64
		wantQuota, wantBilled int
	}{
		{"seconds with resolution", types.DurationPrice{Price: 0.1, Unit: types.DurationUnitSecond, RoundingStepSeconds: 1, MinimumDurationSeconds: 4}, 6, 1, map[string]float64{"resolution": 2.5}, 750000, 6},
		{"minute with five-second step", types.DurationPrice{Price: 6, Unit: types.DurationUnitMinute, RoundingStepSeconds: 5, MinimumDurationSeconds: 4}, 6, 0.8, nil, 400000, 10},
		{"minimum duration", types.DurationPrice{Price: 0.2, Unit: types.DurationUnitSecond, RoundingStepSeconds: 1, MinimumDurationSeconds: 4}, 1, 1, nil, 400000, 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			priceData := types.PriceData{BillingMode: billing_setting.BillingModePerDuration, DurationPrice: &test.rule, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: test.groupRatio}}
			priceData.ReplaceOtherRatios(test.ratios)
			quota, billable, clamp, err := taskDurationQuota(priceData, test.requested)
			require.NoError(t, err)
			assert.Nil(t, clamp)
			assert.Equal(t, test.wantQuota, quota)
			assert.Equal(t, test.wantBilled, billable)
		})
	}
}

func TestTaskDurationQuotaRejectsReservedRatio(t *testing.T) {
	rule := types.DurationPrice{Price: 1, Unit: types.DurationUnitSecond, RoundingStepSeconds: 1}
	priceData := types.PriceData{DurationPrice: &rule, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}}
	priceData.AddOtherRatio("seconds", 6)
	_, _, _, err := taskDurationQuota(priceData, 6)
	assert.ErrorContains(t, err, "reserved duration ratio")
}

func TestTaskDurationQuotaReportsSaturation(t *testing.T) {
	rule := types.DurationPrice{Price: math.MaxFloat64, Unit: types.DurationUnitSecond, RoundingStepSeconds: 1}
	priceData := types.PriceData{DurationPrice: &rule, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}}
	quota, _, clamp, err := taskDurationQuota(priceData, 6)
	require.NoError(t, err)
	assert.Equal(t, common.MaxQuota, quota)
	require.NotNil(t, clamp)
	assert.Equal(t, common.QuotaClampOverflow, clamp.Kind)
}
```

Import `math` and project `common` in the test file. Add a relay-path case with
an insufficient-quota user and the same oversized finite price; assert the
request fails pre-consume, does not call the upstream mock, never produces a
negative quota, and keeps `RelayInfo.QuotaClamp` for admin-log auditing.

- [ ] **Step 2: Carry explicit duration state on `PriceData`**

Add:

```go
BillingMode              string
DurationPrice            *DurationPrice
DurationSource           string
RequestedDurationSeconds int
BillableDurationSeconds  int
```

Update `ToSetting` to include the mode and both durations. Do not copy the
duration unit price into `ModelPrice`.

- [ ] **Step 3: Add the optional adaptor interface**

In `relay/channel/adapter.go`:

```go
type TaskDurationEstimator interface {
	EstimateDurationSeconds(c *gin.Context, info *relaycommon.RelayInfo) (int, *dto.TaskError)
}
```

Do not add this method to `BaseBilling`.

- [ ] **Step 4: Implement the central decimal calculation**

In `relay/relay_task.go`:

```go
func taskDurationQuota(priceData types.PriceData, requestedSeconds int) (int, int, *common.QuotaClamp, error) {
	if priceData.DurationPrice == nil {
		return 0, 0, nil, fmt.Errorf("duration price is not configured")
	}
	if priceData.HasOtherRatio("seconds") || priceData.HasOtherRatio("duration") {
		return 0, 0, nil, fmt.Errorf("reserved duration ratio cannot be used with per_duration billing")
	}
	billableSeconds, err := priceData.DurationPrice.BillableSeconds(requestedSeconds, relaycommon.MaxTaskDurationSeconds)
	if err != nil {
		return 0, 0, nil, err
	}
	quota := decimal.NewFromFloat(priceData.DurationPrice.Price).
		Mul(decimal.NewFromInt(int64(billableSeconds))).
		Div(decimal.NewFromInt(int64(priceData.DurationPrice.UnitSeconds()))).
		Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
		Mul(decimal.NewFromFloat(priceData.GroupRatioInfo.GroupRatio))
	quota = priceData.ApplyOtherRatiosToDecimal(quota)
	value, clamp := common.QuotaFromDecimalChecked(quota)
	return value, billableSeconds, clamp, nil
}
```

Import `github.com/shopspring/decimal`.

- [ ] **Step 5: Select duration mode before legacy price branches**

At the beginning of `ModelPriceHelperPerCall`, after group resolution:

```go
if billing_setting.GetBillingMode(info.OriginModelName) == billing_setting.BillingModePerDuration {
	durationPrice, ok := billing_setting.GetDurationPrice(info.OriginModelName)
	if !ok {
		return types.PriceData{}, fmt.Errorf("model %s is configured as per_duration but has no duration price", info.OriginModelName)
	}
	if err := durationPrice.Validate(relaycommon.MaxTaskDurationSeconds); err != nil {
		return types.PriceData{}, fmt.Errorf("model %s has invalid duration price: %w", info.OriginModelName, err)
	}
	freeModel := !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume && (durationPrice.Price == 0 || groupRatioInfo.GroupRatio == 0)
	return types.PriceData{BillingMode: billing_setting.BillingModePerDuration, DurationPrice: &durationPrice, DurationSource: types.DurationSourceRequest, GroupRatioInfo: groupRatioInfo, FreeModel: freeModel}, nil
}
```

Update `HasModelBillingConfig` to accept a valid duration rule.

- [ ] **Step 6: Route duration models through the estimator**

In `relay/relay_task.go`:

```go
if info.PriceData.BillingMode == billing_setting.BillingModePerDuration {
	estimator, ok := adaptor.(channel.TaskDurationEstimator)
	if !ok {
		return nil, service.TaskErrorWrapperLocal(fmt.Errorf("model %s uses per_duration but adaptor %s does not provide duration", modelName, adaptor.GetChannelName()), "duration_billing_not_supported", http.StatusBadRequest)
	}
	requestedSeconds, taskErr := estimator.EstimateDurationSeconds(c, info)
	if taskErr != nil {
		return nil, taskErr
	}
	quota, billableSeconds, clamp, err := taskDurationQuota(info.PriceData, requestedSeconds)
	if err != nil {
		return nil, service.TaskErrorWrapperLocal(err, "duration_billing_error", http.StatusBadRequest)
	}
	info.PriceData.RequestedDurationSeconds = requestedSeconds
	info.PriceData.BillableDurationSeconds = billableSeconds
	info.PriceData.Quota = quota
	noteTaskQuotaClamp(info, clamp)
} else if !common.StringsContains(constant.TaskPricePatches, modelName) {
	quota, clamp := taskQuotaWithOtherRatios(info.PriceData)
	info.PriceData.Quota = quota
	noteTaskQuotaClamp(info, clamp)
}
```

- [ ] **Step 7: Add helper tests, run regressions, and commit**

In `relay/helper/price_test.go`, configure an alias as `per_duration` and
assert mode, non-nil rule, `ModelPrice == 0`, and `UsePrice == false`.

```bash
go test ./relay ./relay/helper ./types -run 'Duration|TaskDurationQuota' -count=1
go vet ./relay ./relay/helper ./types
git add types/price_data.go relay/channel/adapter.go relay/helper/price.go relay/helper/price_test.go relay/relay_task.go relay/relay_task_billing_test.go
git commit -m "feat(billing): calculate task prices by duration"
```

Expected: tests and vet PASS.

### Task 4: Migrate Dimensio and Enforce Origin-Model Pricing

**Files:**
- Modify: `relay/channel/task/dimensio/adaptor.go`
- Modify: `relay/channel/task/dimensio/adaptor_test.go`
- Modify: `relay/channel/task/dimensio/e2e_test.go`
- Modify: `relay/relay_task.go`
- Modify: `relay/relay_task_seedance_test.go`

- [ ] **Step 1: Change the adaptor test contract first**

Replace the old seconds/resolution assertion in `adaptor_test.go` with:

```go
func TestDurationBillingUsesValidatedRequestDuration(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	duration := 7
	c.Set("dimensio_ark_request", ArkRequest{Duration: &duration, Resolution: "1080p"})
	c.Set("task_resolution", "1080p")

	requested, taskErr := (&TaskAdaptor{}).EstimateDurationSeconds(c, nil)
	require.Nil(t, taskErr)
	assert.Equal(t, 7, requested)
	ratios := (&TaskAdaptor{}).EstimateBilling(c, nil)
	assert.Equal(t, map[string]float64{"resolution": 2.5}, ratios)
	assert.NotContains(t, ratios, "seconds")
}
```

Add missing/wrong-context and out-of-range cases that return
`invalid_duration` before any upstream request.

- [ ] **Step 2: Implement explicit duration and non-duration ratios**

In `adaptor.go`:

```go
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	if strings.EqualFold(c.GetString("task_resolution"), "1080p") {
		return map[string]float64{"resolution": 2.5}
	}
	return map[string]float64{"resolution": 1}
}

func (a *TaskAdaptor) EstimateDurationSeconds(c *gin.Context, _ *relaycommon.RelayInfo) (int, *dto.TaskError) {
	v, ok := c.Get("dimensio_ark_request")
	if !ok {
		return 0, service.TaskErrorWrapperLocal(fmt.Errorf("dimensio request is missing"), "invalid_duration", http.StatusBadRequest)
	}
	req, ok := v.(ArkRequest)
	if !ok || req.Duration == nil || *req.Duration < 4 || *req.Duration > 15 || *req.Duration > relaycommon.MaxTaskDurationSeconds {
		return 0, service.TaskErrorWrapperLocal(fmt.Errorf("duration must be between 4 and 15 seconds"), "invalid_duration", http.StatusBadRequest)
	}
	return *req.Duration, nil
}
```

- [ ] **Step 3: Remove mapped-target pricing**

Delete the Dimensio-only `priceInfo` copy from `relay/relay_task.go` and always
call:

```go
priceData, err := helper.ModelPriceHelperPerCall(c, info)
```

Keep `OriginModelName` as the client model. Keep `UpstreamModelName` for
`ValidateBillingRequest` and `BuildRequestBody` only.

- [ ] **Step 4: Add the origin-price mapping regression**

In `relay/relay_task_seedance_test.go`, configure client model
`client-seedance-vip` at USD 0.1/second and mapped model
`jimeng-video-seedance-2.0-vip` at USD 9/second. Submit 6 seconds at 720p and
assert:

```go
assert.Equal(t, 300000, result.Quota)
assert.Equal(t, "client-seedance-vip", info.OriginModelName)
assert.Equal(t, "jimeng-video-seedance-2.0-vip", info.UpstreamModelName)
```

- [ ] **Step 5: Update protocol E2E and run tests**

In `relay/channel/task/dimensio/e2e_test.go`, assert explicit duration `6`,
`resolution` ratio only, and absence of `seconds`. Preserve the complete prompt
+ reference image + reference video + reference audio conversion assertions.

```bash
go test ./relay/channel/task/dimensio ./relay -run 'Dimensio|Duration|Origin' -count=1
```

Expected: PASS and no test contacts `jimeng.dimensio.cn`.

- [ ] **Step 6: Commit the adaptor migration**

```bash
git add relay/channel/task/dimensio/adaptor.go relay/channel/task/dimensio/adaptor_test.go relay/channel/task/dimensio/e2e_test.go relay/relay_task.go relay/relay_task_seedance_test.go
git commit -m "feat(dimensio): use explicit duration billing"
```

---

### Task 5: Freeze Duration Snapshots, Logs, and Settlement

**Files:**
- Modify: `model/task.go`
- Modify: `controller/relay.go`
- Modify: `service/task_billing.go`
- Modify: `service/task_billing_test.go`

- [ ] **Step 1: Write failing snapshot metadata tests**

Add a `service/task_billing_test.go` fixture:

```go
BillingContext: &model.TaskBillingContext{
	BillingMode: billing_setting.BillingModePerDuration,
	DurationPrice: &types.DurationPrice{
		Price: 0.1, Unit: types.DurationUnitSecond,
		RoundingStepSeconds: 1, MinimumDurationSeconds: 4,
	},
	DurationSource: types.DurationSourceRequest,
	RequestedDurationSeconds: 6,
	BillableDurationSeconds: 6,
	GroupRatio: 1,
	OtherRatios: map[string]float64{"resolution": 2.5},
}
```

Assert `taskBillingOther` contains every explicit duration field and
`resolution_ratio=2.5`, and does not contain `model_price`.

- [ ] **Step 2: Extend the persisted task context**

Add to `model.TaskBillingContext` and import project `types`:

```go
BillingMode              string               `json:"billing_mode,omitempty"`
DurationPrice            *types.DurationPrice `json:"duration_price,omitempty"`
DurationSource           string               `json:"duration_source,omitempty"`
RequestedDurationSeconds int                  `json:"requested_duration_seconds,omitempty"`
BillableDurationSeconds  int                  `json:"billable_duration_seconds,omitempty"`
```

- [ ] **Step 3: Freeze the submission calculation**

In `controller/relay.go`, copy all five fields from `relayInfo.PriceData` into
`TaskBillingContext`. Change the final-at-submit marker to:

```go
PerCallBilling: common.StringsContains(constant.TaskPricePatches, relayInfo.OriginModelName) ||
	relayInfo.PriceData.UsePrice ||
	relayInfo.PriceData.BillingMode == billing_setting.BillingModePerDuration,
```

- [ ] **Step 4: Emit explicit consume/refund log fields**

Add to `service/task_billing.go`:

```go
func appendDurationBillingOther(other map[string]interface{}, mode string, price *types.DurationPrice, source string, requested, billable int) {
	if mode != billing_setting.BillingModePerDuration || price == nil {
		return
	}
	other["billing_mode"] = mode
	other["duration_price"] = price.Price
	other["duration_unit"] = price.Unit
	other["rounding_step_seconds"] = price.RoundingStepSeconds
	other["minimum_duration_seconds"] = price.MinimumDurationSeconds
	other["duration_source"] = source
	other["requested_duration_seconds"] = requested
	other["billable_duration_seconds"] = billable
}
```

Call it from `LogTaskConsumption` and `taskBillingOther`. Do not emit
`model_price` for `per_duration`. Store the numeric non-duration ratio under
`resolution_ratio` so it does not overwrite the request's `resolution` string.

- [ ] **Step 5: Verify terminal accounting**

Add assertions that successful completion keeps `task.Quota`, while
`RefundTaskQuota` restores user/subscription funding, token quota, user used
quota, channel used quota, and quota-data totals. Assert the refund log retains
the complete duration snapshot.

```bash
go test ./service ./model ./controller -run 'Duration|TaskBilling|Refund|Settle' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit snapshots and logs**

```bash
git add model/task.go controller/relay.go service/task_billing.go service/task_billing_test.go
git commit -m "feat(billing): snapshot duration task charges"
```

---

### Task 6: Expose Duration Pricing Through Pricing and Sync APIs

**Files:**
- Modify: `model/pricing.go`
- Modify: `controller/model_list_test.go`
- Modify: `controller/ratio_sync.go`
- Create: `controller/ratio_sync_duration_test.go`

- [ ] **Step 1: Add a failing model-list contract test**

Configure an enabled `duration-visible-model` with mode `per_duration` and:

```go
types.DurationPrice{Price: 0.25, Unit: types.DurationUnitMinute, RoundingStepSeconds: 5, MinimumDurationSeconds: 10}
```

Assert the returned `model.Pricing` has mode `per_duration`, the exact
structured rule, `ModelPrice == 0`, and no token fallback.

- [ ] **Step 2: Add duration data to the pricing API**

Add to `model.Pricing`:

```go
DurationPrice *types.DurationPrice `json:"duration_price,omitempty"`
```

In `updatePricing`, branch before fixed/ratio pricing:

```go
billingMode := billing_setting.GetBillingMode(model)
if billingMode == billing_setting.BillingModePerDuration {
	if durationPrice, ok := billing_setting.GetDurationPrice(model); ok {
		pricing.BillingMode = billingMode
		pricing.DurationPrice = &durationPrice
		pricing.QuotaType = 1
	}
} else if modelPrice, ok := ratio_setting.GetModelPrice(model, false); ok {
	pricing.ModelPrice = modelPrice
	pricing.QuotaType = 1
} else {
	modelRatio, _, _ := ratio_setting.GetModelRatio(model)
	pricing.ModelRatio = modelRatio
	pricing.CompletionRatio = ratio_setting.GetCompletionRatio(model)
	pricing.QuotaType = 0
}
```

- [ ] **Step 3: Synchronize structured duration rules**

Add `billing_setting.DurationPriceField` to `pricingSyncFields` and allowed
fields in `controller/ratio_sync.go`. Extend the remote pricing item with:

```go
DurationPrice *types.DurationPrice `json:"duration_price"`
```

When a remote item has mode `per_duration` and a rule, populate both maps and
do not populate `model_price`. In `ratio_sync_duration_test.go`, prove the
structured map survives comparison and applying it also applies the mode.

- [ ] **Step 4: Run and commit API support**

```bash
go test ./controller ./model ./setting/billing_setting -run 'Duration|Pricing|RatioSync|ListModels' -count=1
git add model/pricing.go controller/model_list_test.go controller/ratio_sync.go controller/ratio_sync_duration_test.go
git commit -m "feat(billing): expose duration model prices"
```

Expected: PASS.

---

### Task 7: Add Frontend Duration State and Serialization

**Files:**
- Modify: `web/default/src/features/system-settings/types.ts`
- Modify: `web/default/src/features/system-settings/billing/index.tsx`
- Modify: `web/default/src/features/system-settings/billing/section-registry.tsx`
- Modify: `web/default/src/features/system-settings/models/model-pricing-core.ts`
- Modify: `web/default/src/features/system-settings/models/model-pricing-snapshots.ts`
- Modify: `web/default/src/features/system-settings/models/model-ratio-form.tsx`
- Modify: `web/default/src/features/system-settings/models/ratio-settings-card.tsx`
- Modify: `web/default/src/features/system-settings/models/model-ratio-visual-editor.tsx`
- Modify: `web/default/src/features/system-settings/models/upstream-ratio-sync.tsx`
- Modify: `web/default/src/features/system-settings/models/upstream-ratio-sync-helpers.ts`
- Modify: `web/default/src/features/system-settings/models/upstream-ratio-sync-table.tsx`
- Create: `web/default/src/features/system-settings/models/model-pricing-duration.test.ts`

- [ ] **Step 1: Load the required frontend instructions**

Read `i18n-translate`, `shadcn-ui`, `vercel-react-best-practices`, and
`web/default/AGENTS.md` before editing frontend files.

- [ ] **Step 2: Write failing pure serialization tests**

Create a `node:test` file that passes this rule through `buildModelSnapshots`:

```ts
const rule = {
  price: 0.25,
  unit: 'minute' as const,
  rounding_step_seconds: 5,
  minimum_duration_seconds: 10,
}

const rows = buildModelSnapshots({
  modelPrice: '{}', modelRatio: '{}', cacheRatio: '{}',
  createCacheRatio: '{}', completionRatio: '{}', imageRatio: '{}',
  audioRatio: '{}', audioCompletionRatio: '{}',
  billingMode: '{"video":"per_duration"}', billingExpr: '{}',
  durationPrice: JSON.stringify({ video: rule }),
})
assert.equal(rows[0].billingMode, 'per_duration')
assert.deepEqual(rows[0].durationPrice, rule)
assert.equal(getPriceSummary(rows[0], (key) => key), '$0.25 / minute')
assert.equal(isBasePricingUnset(rows[0]), false)
```

Also test missing rule, dirty signature, mode switching, deletion, and batch-copy
data.

- [ ] **Step 3: Add frontend types and settings fields**

In `model-pricing-core.ts`:

```ts
export type DurationUnit = 'second' | 'minute'
export type DurationPrice = {
  price: number
  unit: DurationUnit
  rounding_step_seconds: number
  minimum_duration_seconds: number
}
export type PricingMode =
  | 'per-token'
  | 'per-request'
  | 'per_duration'
  | 'tiered_expr'
```

Add `durationPrice?: DurationPrice` to model/snapshot data. Add
`DurationPrice: string` and `billing_setting.duration_price` to system types,
defaults, section registry, form schema, saved values, JSON editor fields, and
API key maps.

- [ ] **Step 4: Parse, summarize, and compare rules**

Parse `Record<string, DurationPrice>`, include its keys in the model-name set,
return `billingMode: 'per_duration'` only with a rule, and include the complete
rule in the snapshot signature. Add summary:

```ts
if (row.billingMode === 'per_duration') {
  const rule = row.durationPrice
  return rule ? `$${rule.price} / ${t(rule.unit)}` : t('Unset price')
}
```

- [ ] **Step 5: Persist duration rules and explicit overrides**

In `model-ratio-visual-editor.tsx`, parse `durationPriceMap`, delete conflicts
for each target, then write:

```ts
if (data.billingMode === 'per_duration' && data.durationPrice) {
  billingModeMap[name] = 'per_duration'
  durationPriceMap[name] = data.durationPrice
} else if (data.billingMode === 'tiered_expr') {
  billingModeMap[name] = 'tiered_expr'
} else {
  billingModeMap[name] = 'ratio'
}
```

Always emit `billing_setting.duration_price`. Carry it through delete, batch
copy, saved/draft comparison, memo equality, mode counts, and filters. The
explicit `ratio` entry prevents a built-in Dimensio duration default from
reappearing when an administrator switches that model to per-token or
per-request.

Extend upstream synchronization with `duration_price`: parse the structured
map, display `price + unit + step + minimum`, select it together with
`billing_mode=per_duration`, and delete conflicting `ModelPrice`/`ModelRatio`
entries when applying it.

- [ ] **Step 6: Run and commit frontend state support**

From `web/default`:

```bash
bun test src/features/system-settings/models/model-pricing-duration.test.ts
bun run typecheck
```

Then:

```bash
git add web/default/src/features/system-settings/types.ts web/default/src/features/system-settings/billing/index.tsx web/default/src/features/system-settings/billing/section-registry.tsx web/default/src/features/system-settings/models
git commit -m "feat(web): persist per-duration model prices"
```

Expected: tests and typecheck PASS.

### Task 8: Build the Per-Duration Editor and Complete i18n

**Files:**
- Modify: `web/default/src/features/system-settings/models/model-pricing-sheet.tsx`
- Modify: `web/default/src/features/system-settings/models/model-pricing-core.ts`
- Modify: `web/default/src/features/system-settings/models/model-ratio-table-columns.tsx`
- Modify: `web/default/src/i18n/locales/en.json`
- Modify: `web/default/src/i18n/locales/zh.json`
- Modify: `web/default/src/i18n/locales/fr.json`
- Modify: `web/default/src/i18n/locales/ru.json`
- Modify: `web/default/src/i18n/locales/ja.json`
- Modify: `web/default/src/i18n/locales/vi.json`

- [ ] **Step 1: Add strict form validation**

Extend the pricing schema with:

```ts
durationPrice: z.string().optional(),
durationUnit: z.enum(['second', 'minute']).optional(),
roundingStepSeconds: z.string().optional(),
minimumDurationSeconds: z.string().optional(),
```

When mode is `per_duration`, use `form.setError` on the exact field to require
a finite non-negative price, a step integer from 1 through 3600, and a minimum
integer from 0 through 3600.

- [ ] **Step 2: Add the fourth editor tab and controls**

Use a responsive tabs list:

```tsx
<TabsList className='grid w-full grid-cols-2 sm:grid-cols-4'>
  <TabsTrigger value='per-token'>{t('Per-token')}</TabsTrigger>
  <TabsTrigger value='per-request'>{t('Per-request')}</TabsTrigger>
  <TabsTrigger value='per_duration'>{t('Per-duration')}</TabsTrigger>
  <TabsTrigger value='tiered_expr'>{t('Expression')}</TabsTrigger>
</TabsList>
```

The duration tab contains a USD numeric input, a Base UI/shadcn `Select` for
second/minute, and integer inputs for step seconds and minimum billable
seconds. Associate every label with its control and preserve keyboard access.

- [ ] **Step 3: Build preview and submit data**

When duration mode is active:

```ts
data.durationPrice = {
  price: Number(values.durationPrice),
  unit: values.durationUnit as DurationUnit,
  rounding_step_seconds: Number(values.roundingStepSeconds),
  minimum_duration_seconds: Number(values.minimumDurationSeconds),
}
```

Preview mode, unit price, unit, step, and minimum. Editing an existing rule
must repopulate all four values exactly.

- [ ] **Step 4: Add all locale entries**

Use the English source string as the flat JSON key and these exact values:

| English key | zh | fr | ru | ja | vi |
|---|---|---|---|---|---|
| `Per-duration` | 按时长 | À la durée | По длительности | 時間単位 | Theo thời lượng |
| `Duration-based` | 按时长计费 | Facturation à la durée | По длительности | 時間ベース | Tính phí theo thời lượng |
| `Duration price` | 时长价格 | Prix à la durée | Цена за длительность | 時間単価 | Giá theo thời lượng |
| `Duration unit` | 时长单位 | Unité de durée | Единица длительности | 時間単位 | Đơn vị thời lượng |
| `USD price per duration unit.` | 每个时长单位的美元价格。 | Prix en USD par unité de durée. | Цена в долларах США за единицу длительности. | 時間単位あたりの米ドル価格。 | Giá USD cho mỗi đơn vị thời lượng. |
| `Second` | 秒 | Seconde | Секунда | 秒 | Giây |
| `Minute` | 分钟 | Minute | Минута | 分 | Phút |
| `second` | 秒 | seconde | секунда | 秒 | giây |
| `minute` | 分钟 | minute | минута | 分 | phút |
| `Rounding step` | 计费步长 | Pas de facturation | Шаг тарификации | 課金ステップ | Bước làm tròn |
| `Rounding step in seconds.` | 以秒为单位的计费步长。 | Pas de facturation en secondes. | Шаг тарификации в секундах. | 秒単位の課金ステップ。 | Bước làm tròn tính bằng giây. |
| `Minimum billable duration` | 最低计费时长 | Durée minimale facturable | Минимальная оплачиваемая длительность | 最低課金時間 | Thời lượng tính phí tối thiểu |
| `Minimum billable duration in seconds.` | 以秒为单位的最低计费时长。 | Durée minimale facturable en secondes. | Минимальная оплачиваемая длительность в секундах. | 秒単位の最低課金時間。 | Thời lượng tính phí tối thiểu tính bằng giây. |
| `Duration price is required.` | 请输入时长价格。 | Le prix à la durée est requis. | Укажите цену за длительность. | 時間単価は必須です。 | Giá theo thời lượng là bắt buộc. |
| `Duration price must be zero or greater.` | 时长价格必须大于或等于 0。 | Le prix à la durée doit être supérieur ou égal à zéro. | Цена за длительность должна быть не меньше нуля. | 時間単価は 0 以上である必要があります。 | Giá theo thời lượng phải lớn hơn hoặc bằng 0. |
| `Rounding step must be an integer between 1 and 3600.` | 计费步长必须是 1 到 3600 之间的整数。 | Le pas de facturation doit être un entier compris entre 1 et 3600. | Шаг тарификации должен быть целым числом от 1 до 3600. | 課金ステップは 1 から 3600 までの整数である必要があります。 | Bước làm tròn phải là số nguyên từ 1 đến 3600. |
| `Minimum billable duration must be an integer between 0 and 3600.` | 最低计费时长必须是 0 到 3600 之间的整数。 | La durée minimale facturable doit être un entier compris entre 0 et 3600. | Минимальная оплачиваемая длительность должна быть целым числом от 0 до 3600. | 最低課金時間は 0 から 3600 までの整数である必要があります。 | Thời lượng tính phí tối thiểu phải là số nguyên từ 0 đến 3600. |

The English locale maps every key to itself.

- [ ] **Step 5: Run frontend checks and commit**

From `web/default`:

```bash
bun run i18n:sync
bun run format
bun run lint
bun run typecheck
bun run build
```

Then:

```bash
git add web/default/src/features/system-settings/models/model-pricing-sheet.tsx web/default/src/features/system-settings/models/model-pricing-core.ts web/default/src/features/system-settings/models/model-ratio-table-columns.tsx web/default/src/i18n/locales/en.json web/default/src/i18n/locales/zh.json web/default/src/i18n/locales/fr.json web/default/src/i18n/locales/ru.json web/default/src/i18n/locales/ja.json web/default/src/i18n/locales/vi.json
git commit -m "feat(web): edit per-duration model pricing"
```

Expected: every command exits 0 and all six required locales contain real
translations.

---

### Task 9: Render Duration Prices in the Public Pricing Catalog

**Files:**
- Modify: `web/default/src/features/pricing/types.ts`
- Modify: `web/default/src/features/pricing/constants.ts`
- Modify: `web/default/src/features/pricing/lib/model-helpers.ts`
- Modify: `web/default/src/features/pricing/lib/price.ts`
- Modify: `web/default/src/features/pricing/lib/filters.ts`
- Modify: `web/default/src/features/pricing/components/model-billing-mode-badge.tsx`
- Modify: `web/default/src/features/pricing/components/pricing-columns.tsx`
- Modify: `web/default/src/features/pricing/components/model-card.tsx`
- Modify: `web/default/src/features/pricing/components/model-details.tsx`
- Modify: `web/default/src/features/pricing/components/pricing-sidebar.tsx`

- [ ] **Step 1: Add duration typing and detection**

Add to `PricingModel`:

```ts
duration_price?: {
  price: number
  unit: 'second' | 'minute'
  rounding_step_seconds: number
  minimum_duration_seconds: number
}
```

Export:

```ts
export function isDurationBasedModel(model: PricingModel): boolean {
  return model.billing_mode === 'per_duration' && Boolean(model.duration_price)
}
```

- [ ] **Step 2: Add currency-aware duration formatting**

In `pricing/lib/price.ts`, use the existing group and recharge transformations:

```ts
export function formatDurationPrice(
  model: PricingModel,
  showWithRecharge = false,
  priceRate = 1,
  usdExchangeRate = 1,
  selectedGroup?: string
): string {
  if (!model.duration_price) return '-'
  const groupRatio = getDisplayGroupRatio(model, selectedGroup)
  const adjusted = applyRechargeRate(
    model.duration_price.price * groupRatio,
    showWithRecharge,
    priceRate,
    usdExchangeRate
  )
  return formatCurrencyFromUSD(adjusted, {
    digitsLarge: 4,
    digitsSmall: 6,
    abbreviate: false,
  })
}
```

- [ ] **Step 3: Render the correct badge and unit everywhere**

Make `ModelBillingModeBadge` check duration mode before token/request mode:

```tsx
if (isDurationBasedModel(props.model)) {
  label = t('Duration-based')
  variant = 'success'
} else if (isDynamicPricingModel(props.model)) {
  // retain the existing expression branch
}
```

In table, card, and detail branches, render `formatDurationPrice` followed by
`/ second` or `/ minute`. No duration model may reach a branch that appends
`/ request`.

- [ ] **Step 4: Add duration filtering and sorting**

Add `DURATION: 'duration'` to `QUOTA_TYPES`. Use:

```ts
if (quotaType === QUOTA_TYPES.DURATION) {
  return models.filter(isDurationBasedModel)
}
if (quotaType === QUOTA_TYPES.REQUEST) {
  return models.filter(
    (model) =>
      model.quota_type === QUOTA_TYPE_VALUES.REQUEST &&
      !isDurationBasedModel(model)
  )
}
```

Sort duration models by `duration_price.price` and add a distinct duration
count to the sidebar.

- [ ] **Step 5: Run checks and commit public display support**

```bash
cd web/default
bun run format
bun run lint
bun run typecheck
bun run build
```

Then, from the repository root:

```bash
git add web/default/src/features/pricing
git commit -m "feat(web): display duration-based model prices"
```

Expected: PASS.

---

### Task 10: Replace the Mock E2E Billing Contract and Update Documentation

**Files:**
- Modify: `e2e/seedance_native_e2e_test.go`
- Modify: `relay/channel/task/dimensio/e2e_test.go`
- Modify: `docs/channel/jimeng.dimensio.cn.md`
- Modify: `docs/superpowers/plans/2026-07-18-dimensio-translator.md`
- Modify: `docs/superpowers/reports/2026-07-19-dimensio-e2e-acceptance-report.md`

- [ ] **Step 1: Seed origin-model duration rules in application E2E**

Replace target-model `ModelRatio` fixtures with:

```go
durationRule := types.DurationPrice{
	Price: modelCase.pricePerSecond,
	Unit: types.DurationUnitSecond,
	RoundingStepSeconds: 1,
	MinimumDurationSeconds: 4,
}
modeJSON, err := common.Marshal(map[string]string{originModel: billing_setting.BillingModePerDuration})
require.NoError(t, err)
priceJSON, err := common.Marshal(map[string]types.DurationPrice{originModel: durationRule})
require.NoError(t, err)
require.NoError(t, config.UpdateConfigFromMap(
	config.GlobalConfig.Get("billing_setting"),
	map[string]string{"billing_mode": string(modeJSON), "duration_price": string(priceJSON)},
))
```

Use production-equivalent base prices `0.48/7.3`, `0.39/7.3`, and `0.62/7.3`.
VIP 1080p retains the adaptor's `2.5` resolution ratio.

- [ ] **Step 2: Assert the exact frozen calculation**

For every submitted task:

```go
assert.Equal(t, billing_setting.BillingModePerDuration, task.PrivateData.BillingContext.BillingMode)
assert.Equal(t, types.DurationSourceRequest, task.PrivateData.BillingContext.DurationSource)
assert.Equal(t, 6, task.PrivateData.BillingContext.RequestedDurationSeconds)
assert.Equal(t, 6, task.PrivateData.BillingContext.BillableDurationSeconds)
assert.NotContains(t, task.PrivateData.BillingContext.OtherRatios, "seconds")
assert.Equal(t, modelCase.resolutionRatio, task.PrivateData.BillingContext.OtherRatios["resolution"])
assert.Equal(t, modelCase.expectedQuota, task.Quota)
```

Derive `expectedQuota` with `common.QuotaFromDecimal`, never a bare cast.

- [ ] **Step 3: Preserve the complete application matrix**

Keep all three models and all four outcomes: `completed`, `failed`, `-2011`,
and `1057`. Every case submits prompt + reference image + reference video +
reference audio and asserts the captured Dimensio request. Success keeps the
charge; failure and `-2011` refund all three ledgers; `1057` keeps the task and
pre-consumed amount.

- [ ] **Step 4: Correct all billing documentation**

Update the channel document to 48/39/62/155 points per second and add
`1 point = CNY 0.01`. State that query responses do not define duration.

In the old translator plan, link this plan as authoritative, remove implicit
`OtherRatio: seconds`, reserve `seconds` against duration mode, retain the 2.5
resolution ratio, and preserve every protocol-translation decision.

Rewrite the report billing section around:

```text
duration_source = request
requested_duration_seconds = 6
billable_duration_seconds = 6
charge = duration_price * 6 * resolution_ratio * group_ratio
```

Include the exact ARK request, captured Dimensio request, mock success/failure
responses, converted ARK structures, quota/refund observations, commands, and
the statement that no real upstream request or point spend occurred.

- [ ] **Step 5: Run mock E2E and commit**

```bash
go test ./relay/channel/task/dimensio -run 'TestDimensioSeedance20ProtocolE2E|TestDurationBillingUsesValidatedRequestDuration' -count=1 -v
go test ./e2e -run TestDimensioSeedance20MultimodalLifecycleE2E -count=1 -v
```

Then:

```bash
git add e2e/seedance_native_e2e_test.go relay/channel/task/dimensio/e2e_test.go docs/channel/jimeng.dimensio.cn.md docs/superpowers/plans/2026-07-18-dimensio-translator.md docs/superpowers/reports/2026-07-19-dimensio-e2e-acceptance-report.md
git commit -m "test(dimensio): accept per-duration Seedance billing"
```

Expected: the full 12-scenario matrix passes without external traffic.

---

### Task 11: Run Full Verification and Browser Acceptance

**Files:**
- Modify only feature files required to correct failures found below.

- [ ] **Step 1: Format and statically check backend files**

Run `gofmt -w` on every changed `.go` file, then:

```bash
go vet ./types ./setting/billing_setting ./relay/helper ./relay/channel/task/dimensio ./relay ./model ./controller ./service ./e2e
```

Expected: exits 0.

- [ ] **Step 2: Run the full backend suite and build**

```bash
go test ./... -count=1
go build ./...
```

Expected: PASS.

- [ ] **Step 3: Run all frontend checks**

From `web/default`:

```bash
bun test src/features/system-settings/models/model-pricing-duration.test.ts
bun run i18n:sync
bun run format:check
bun run lint
bun run typecheck
bun run build
```

Expected: every command exits 0.

- [ ] **Step 4: Automate the admin and public UI with Playwright**

Start the local backend and `web/default` dev server. At desktop `1440x900`
and mobile `390x844`:

1. Open System Settings -> Billing -> Model Pricing.
2. Edit a model, select `Per-duration`, and enter `0.25`, `minute`, step `5`,
   minimum `10`.
3. Verify the preview, save, reload, and verify persistence.
4. Switch to per-request and verify the duration rule is removed.
5. Restore duration mode and verify the public catalog shows
   `Duration-based` and `/ minute`, never `/ request`.

Capture desktop/mobile screenshots for the report. Verify no text overlap,
horizontal clipping, layout shift, or inaccessible controls.

- [ ] **Step 5: Inspect the diff and report only observed evidence**

```bash
git diff --check
git status --short
```

Confirm protected identifiers are unchanged, `examples/` remains untouched,
and the report lists only commands actually run with their observed results.
If a correction is required, return to the owning task, rerun that task's
listed checks, stage only the exact files listed by that task, and commit with
`fix(billing): address per-duration verification findings`. If no correction
is needed, do not create an empty commit.
