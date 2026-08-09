package dto

import "github.com/QuantumNous/new-api/types"

type CostCatalogPrice struct {
	Key                 string `json:"key"`
	Unit                string `json:"unit"`
	NativeAmount        string `json:"native_amount"`
	NormalizedUSDAmount string `json:"normalized_usd_amount"`
}

type CostCatalogItem struct {
	RuleID                              int64                 `json:"rule_id"`
	ChannelID                           int                   `json:"channel_id"`
	ChannelName                         string                `json:"channel_name"`
	ChannelType                         int                   `json:"channel_type"`
	ChannelMissing                      bool                  `json:"channel_missing"`
	BillableUpstreamModel               string                `json:"billable_upstream_model"`
	CostVariantKey                      string                `json:"cost_variant_key"`
	Version                             int                   `json:"version"`
	Status                              string                `json:"status"`
	CostMode                            types.CostMode        `json:"cost_mode"`
	SchemaVersion                       int                   `json:"schema_version"`
	Currency                            string                `json:"currency"`
	Prices                              []CostCatalogPrice    `json:"prices"`
	Comparison15SEquivalentUSDPerSecond *string               `json:"comparison_15s_equivalent_usd_per_second,omitempty"`
	ChargeEvent                         types.CostChargeEvent `json:"charge_event,omitempty"`
	MeterSource                         types.CostMeterSource `json:"meter_source,omitempty"`
	TokenMode                           types.CostTokenMode   `json:"token_mode,omitempty"`
	Source                              string                `json:"source"`
	Note                                string                `json:"note"`
	EffectiveFrom                       *int64                `json:"effective_from,omitempty"`
	EffectiveTo                         *int64                `json:"effective_to,omitempty"`
	CreatedAt                           int64                 `json:"created_at"`
	UpdatedAt                           int64                 `json:"updated_at"`
	PriceStatus                         string                `json:"price_status"`
	Issues                              []string              `json:"issues"`
}

type CostCatalogSummary struct {
	ChannelCount     int64 `json:"channel_count"`
	ActiveRuleCount  int64 `json:"active_rule_count"`
	DraftRuleCount   int64 `json:"draft_rule_count"`
	RetiredRuleCount int64 `json:"retired_rule_count"`
}

type CostCatalogChannelFacet struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Type    int    `json:"type"`
	Missing bool   `json:"missing"`
}

type CostCatalogFacets struct {
	Channels   []CostCatalogChannelFacet `json:"channels"`
	Currencies []string                  `json:"currencies"`
	Sources    []string                  `json:"sources"`
}

type CostCatalogPage struct {
	Items    []CostCatalogItem  `json:"items"`
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
	Summary  CostCatalogSummary `json:"summary"`
	Facets   CostCatalogFacets  `json:"facets"`
}

type CostCatalogHistoryEntry struct {
	CostCatalogItem
	CreatedBy   int `json:"created_by"`
	ActivatedBy int `json:"activated_by"`
}

type CostCatalogDetail struct {
	Rule    CostCatalogHistoryEntry   `json:"rule"`
	Config  *types.CostRuleConfigV1   `json:"config,omitempty"`
	History []CostCatalogHistoryEntry `json:"history"`
}
