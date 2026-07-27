package dto

import (
	"github.com/QuantumNous/new-api/constant"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/types"
)

type UpdateCostAccountingSettingsRequest struct {
	Mode                     *types.CostAccountingMode `json:"mode"`
	MinimumExpectedMarginBPS *int                      `json:"minimum_expected_margin_bps"`
}

type CostRuleWriteRequest struct {
	ChannelID             int                    `json:"channel_id" binding:"required"`
	BillableUpstreamModel string                 `json:"billable_upstream_model" binding:"required"`
	CostVariantKey        string                 `json:"cost_variant_key,omitempty"`
	CostMode              types.CostMode         `json:"cost_mode" binding:"required"`
	Config                types.CostRuleConfigV1 `json:"config" binding:"required"`
	Note                  string                 `json:"note,omitempty"`
	RequestPath           string                 `json:"request_path,omitempty"`
	TaskPlatform          constant.TaskPlatform  `json:"task_platform,omitempty"`
}

type CostRuleUpdateRequest struct {
	CostMode     types.CostMode         `json:"cost_mode" binding:"required"`
	Config       types.CostRuleConfigV1 `json:"config" binding:"required"`
	Note         string                 `json:"note,omitempty"`
	RequestPath  string                 `json:"request_path,omitempty"`
	TaskPlatform constant.TaskPlatform  `json:"task_platform,omitempty"`
}

type CostRuleResponse struct {
	ID                    int64                  `json:"id"`
	ChannelID             int                    `json:"channel_id"`
	BillableUpstreamModel string                 `json:"billable_upstream_model"`
	CostVariantKey        string                 `json:"cost_variant_key"`
	Version               int                    `json:"version"`
	Status                string                 `json:"status"`
	CostMode              types.CostMode         `json:"cost_mode"`
	SchemaVersion         int                    `json:"schema_version"`
	Config                types.CostRuleConfigV1 `json:"config"`
	Source                string                 `json:"source"`
	Note                  string                 `json:"note"`
	CreatedBy             int                    `json:"created_by"`
	ActivatedBy           int                    `json:"activated_by"`
	EffectiveFrom         *int64                 `json:"effective_from,omitempty"`
	EffectiveTo           *int64                 `json:"effective_to,omitempty"`
	CreatedAt             int64                  `json:"created_at"`
	UpdatedAt             int64                  `json:"updated_at"`
}

type CostExpressionRequestInput struct {
	Headers map[string]string `json:"headers,omitempty"`
	Body    map[string]any    `json:"body,omitempty"`
}

type CostPreviewRequest struct {
	OriginModel            string                      `json:"origin_model" binding:"required"`
	UserGroup              string                      `json:"user_group" binding:"required"`
	RelayMode              int                         `json:"relay_mode" binding:"required"`
	RequestPath            string                      `json:"request_path,omitempty"`
	Usage                  *relaydto.Usage             `json:"usage,omitempty"`
	TokenMeta              *relaytypes.TokenCountMeta  `json:"token_meta,omitempty"`
	DurationSeconds        *int                        `json:"duration_seconds,omitempty"`
	ExpressionRequestInput *CostExpressionRequestInput `json:"expression_request_input,omitempty"`
	CostMode               types.CostMode              `json:"cost_mode" binding:"required"`
	Config                 types.CostRuleConfigV1      `json:"config" binding:"required"`
	Meter                  *types.CostMeter            `json:"meter,omitempty"`
}

type CostPreviewResponse struct {
	Estimated      bool    `json:"estimated"`
	OriginalCost   string  `json:"original_cost"`
	RevenueNanoUSD string  `json:"revenue_nano_usd"`
	CostNanoUSD    string  `json:"cost_nano_usd"`
	ProfitNanoUSD  string  `json:"profit_nano_usd"`
	MarginPPM      *string `json:"margin_ppm,omitempty"`
}

type ReconcileCostAttemptRequest struct {
	Action string           `json:"action" binding:"required"`
	Meter  *types.CostMeter `json:"meter,omitempty"`
	Reason string           `json:"reason" binding:"required"`
}

type ReconcileCostRevenueRequest struct {
	Action               string  `json:"action" binding:"required"`
	FinalUserQuota       *int64  `json:"final_user_quota,omitempty"`
	QuotaPerUnitSnapshot *string `json:"quota_per_unit_snapshot,omitempty"`
	Reason               string  `json:"reason" binding:"required"`
}

type CostCoverageItem struct {
	ChannelID              int    `json:"channel_id"`
	OriginModel            string `json:"origin_model"`
	PredictedUpstreamModel string `json:"predicted_upstream_model"`
	CostVariantKey         string `json:"cost_variant_key"`
	Covered                bool   `json:"covered"`
	Reason                 string `json:"reason,omitempty"`
}
