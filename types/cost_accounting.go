package types

type CostAccountingMode string

const (
	CostAccountingDisabled CostAccountingMode = "disabled"
	CostAccountingStrict   CostAccountingMode = "strict"
)

type CostMode string

const (
	CostModeFree        CostMode = "free"
	CostModePerRequest  CostMode = "per_request"
	CostModePerDuration CostMode = "per_duration"
	CostModePerToken    CostMode = "per_token"
)

type CostRuleStatus string

const (
	CostRuleDraft   CostRuleStatus = "draft"
	CostRuleActive  CostRuleStatus = "active"
	CostRuleRetired CostRuleStatus = "retired"
)

type CostRevenueStatus string

const (
	CostRevenuePending       CostRevenueStatus = "pending"
	CostRevenueSettled       CostRevenueStatus = "settled"
	CostRevenueConfirmedZero CostRevenueStatus = "confirmed_zero"
	CostRevenueFailed        CostRevenueStatus = "revenue_failed"
)

type CostProfitStatus string

const (
	CostProfitComplete          CostProfitStatus = "complete"
	CostProfitIncompleteCost    CostProfitStatus = "incomplete_cost"
	CostProfitIncompleteRevenue CostProfitStatus = "incomplete_revenue"
)

type CostChargeEvent string

const (
	CostChargeResponseSucceeded CostChargeEvent = "response_succeeded"
	CostChargeSubmitAccepted    CostChargeEvent = "submit_accepted"
	CostChargeTaskSucceeded     CostChargeEvent = "task_succeeded"
)

type CostMeterSource string

const (
	CostMeterValidatedRequest CostMeterSource = "validated_request"
	CostMeterUpstreamActual   CostMeterSource = "upstream_actual"
	CostMeterUpstreamUsage    CostMeterSource = "upstream_usage"
	CostMeterLocalUsage       CostMeterSource = "local_usage"
)

type CostAttemptStatus string

const (
	CostAttemptPrepared         CostAttemptStatus = "prepared"
	CostAttemptDispatching      CostAttemptStatus = "dispatching"
	CostAttemptNotDispatched    CostAttemptStatus = "not_dispatched"
	CostAttemptAwaitingMeter    CostAttemptStatus = "awaiting_meter"
	CostAttemptSettled          CostAttemptStatus = "settled"
	CostAttemptConfirmedZero    CostAttemptStatus = "confirmed_zero"
	CostAttemptUnknown          CostAttemptStatus = "cost_unknown"
	CostAttemptSettlementFailed CostAttemptStatus = "settlement_failed"
)

type CostTokenMode string

const (
	CostTokenModeTotal       CostTokenMode = "total_tokens"
	CostTokenModeCompletion  CostTokenMode = "completion_tokens"
	CostTokenModeInputOutput CostTokenMode = "input_output"
)

type CostMeter struct {
	Source           CostMeterSource `json:"source"`
	DurationSeconds  *string         `json:"duration_seconds,omitempty"`
	InputTokens      *int64          `json:"input_tokens,omitempty"`
	OutputTokens     *int64          `json:"output_tokens,omitempty"`
	CompletionTokens *int64          `json:"completion_tokens,omitempty"`
	TotalTokens      *int64          `json:"total_tokens,omitempty"`
}

type CostCapabilities struct {
	CanResolveBillableModel bool              `json:"can_resolve_billable_model"`
	ChargeEvents            []CostChargeEvent `json:"charge_events"`
	MeterSources            []CostMeterSource `json:"meter_sources"`
}

type CostAttemptHandle struct {
	CostRequestID int64
	AttemptID     int64
	AttemptNo     int
	CostMode      CostMode
	ChargeEvent   CostChargeEvent
}

// CostProfitRecheckSnapshot records the state that passed the authoritative
// pre-dispatch profit recheck. PrepareCostAttempt compares it with the locked
// current state before creating a cost request or attempt.
type CostProfitRecheckSnapshot struct {
	ChannelID                      int
	BillableUpstreamModel          string
	RuleID                         int64
	RuleVersion                    int
	GlobalMinimumExpectedMarginBPS int
	RouteTarget                    *CostRoutingTargetSnapshot
}

// CostRoutingTargetSnapshot holds the routing values that affect profit
// eligibility for the selected target. A nil target means legacy routing.
type CostRoutingTargetSnapshot struct {
	ID                       int
	PolicyID                 int
	ChannelID                int
	UpstreamModel            string
	MinimumExpectedMarginBPS *int
}

type CostOutcome struct {
	Status           CostAttemptStatus
	UpstreamAccepted bool
	FailureCode      string
}

type CostRulePricesV1 struct {
	UnitPrice            *string `json:"unit_price,omitempty"`
	PricePerSecond       *string `json:"price_per_second,omitempty"`
	TotalPerMillion      *string `json:"total_per_million,omitempty"`
	CompletionPerMillion *string `json:"completion_per_million,omitempty"`
	InputPerMillion      *string `json:"input_per_million,omitempty"`
	OutputPerMillion     *string `json:"output_per_million,omitempty"`
}

type CostRuleConfigV1 struct {
	Currency              string           `json:"currency,omitempty"`
	BillingMultiplier     string           `json:"billing_multiplier,omitempty"`
	PurchaseDiscountRatio string           `json:"purchase_discount_ratio,omitempty"`
	RechargeExchangeRatio string           `json:"recharge_exchange_ratio,omitempty"`
	FeeRate               string           `json:"fee_rate,omitempty"`
	CurrencyToUSDRate     string           `json:"currency_to_usd_rate,omitempty"`
	UnitPrice             *string          `json:"unit_price,omitempty"`
	PricePerSecond        *string          `json:"price_per_second,omitempty"`
	TotalPerMillion       *string          `json:"total_per_million,omitempty"`
	CompletionPerMillion  *string          `json:"completion_per_million,omitempty"`
	InputPerMillion       *string          `json:"input_per_million,omitempty"`
	OutputPerMillion      *string          `json:"output_per_million,omitempty"`
	ZeroCostReason        string           `json:"zero_cost_reason,omitempty"`
	ChargeEvent           CostChargeEvent  `json:"charge_event,omitempty"`
	MeterSource           CostMeterSource  `json:"meter_source,omitempty"`
	TokenMode             CostTokenMode    `json:"token_mode,omitempty"`
	NormalizedUSDPrices   CostRulePricesV1 `json:"normalized_usd_prices"`
}
