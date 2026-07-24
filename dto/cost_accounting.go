package dto

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/types"
)

type UpdateCostAccountingModeRequest struct {
	Mode types.CostAccountingMode `json:"mode" binding:"required"`
}

type CostRuleWriteRequest struct {
	ChannelID             int                    `json:"channel_id" binding:"required"`
	BillableUpstreamModel string                 `json:"billable_upstream_model" binding:"required"`
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
	Usage                  *Usage                      `json:"usage,omitempty"`
	TokenMeta              *types.TokenCountMeta       `json:"token_meta,omitempty"`
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
	Covered                bool   `json:"covered"`
	Reason                 string `json:"reason,omitempty"`
}
