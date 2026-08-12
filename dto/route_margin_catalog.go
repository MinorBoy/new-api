package dto

import "github.com/QuantumNous/new-api/types"

type RouteMarginCatalogItem struct {
	TargetID                   int            `json:"target_id"`
	TargetName                 string         `json:"target_name"`
	PolicyID                   int            `json:"policy_id"`
	GroupName                  string         `json:"group_name"`
	CanonicalModel             string         `json:"canonical_model"`
	ChannelID                  int            `json:"channel_id"`
	ChannelName                string         `json:"channel_name"`
	ChannelType                int            `json:"channel_type"`
	UpstreamModel              string         `json:"upstream_model"`
	CostVariantKey             string         `json:"cost_variant_key"`
	Resolution                 string         `json:"resolution"`
	DurationSeconds            int            `json:"duration_seconds"`
	Scenario                   string         `json:"scenario"`
	GroupRatio                 string         `json:"group_ratio"`
	CostMode                   types.CostMode `json:"cost_mode,omitempty"`
	RuleID                     int64          `json:"rule_id,omitempty"`
	RuleVersion                int            `json:"rule_version,omitempty"`
	EstimatedRevenueNanoUSD    *int64         `json:"estimated_revenue_nano_usd,omitempty"`
	EstimatedCostNanoUSD       *int64         `json:"estimated_cost_nano_usd,omitempty"`
	EstimatedProfitNanoUSD     *int64         `json:"estimated_profit_nano_usd,omitempty"`
	GrossMarginPPM             *int64         `json:"gross_margin_ppm,omitempty"`
	RequestedMinimumMarginPPM  int64          `json:"requested_minimum_margin_ppm"`
	ConfiguredMinimumMarginBPS *int           `json:"configured_minimum_margin_bps,omitempty"`
	Eligible                   bool           `json:"eligible"`
	FailureReason              string         `json:"failure_reason,omitempty"`
	CostSource                 string         `json:"cost_source,omitempty"`
	RevenueSource              string         `json:"revenue_source"`
}

type RouteMarginCatalogSummary struct {
	TargetCount                  int `json:"target_count"`
	ScenarioCount                int `json:"scenario_count"`
	EligibleTargetCount          int `json:"eligible_target_count"`
	FullyEligibleTargetCount     int `json:"fully_eligible_target_count"`
	PartiallyEligibleTargetCount int `json:"partially_eligible_target_count"`
	IneligibleTargetCount        int `json:"ineligible_target_count"`
	EligibleScenarioCount        int `json:"eligible_scenario_count"`
}

type RouteMarginCatalogFacets struct {
	Channels        []CostCatalogChannelFacet `json:"channels"`
	Resolutions     []string                  `json:"resolutions"`
	CanonicalModels []string                  `json:"canonical_models"`
}

type RouteMarginCatalogPage struct {
	Items    []RouteMarginCatalogItem  `json:"items"`
	Total    int                       `json:"total"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"page_size"`
	Summary  RouteMarginCatalogSummary `json:"summary"`
	Facets   RouteMarginCatalogFacets  `json:"facets"`
}
