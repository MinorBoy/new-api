package dto

// ImagePricingCostItem is the aggregated supplier cost for one public image SKU.
// It intentionally omits channel and upstream identifiers so the read API does
// not disclose provider credentials or internal routing details.
type ImagePricingCostItem struct {
	Model          string `json:"model"`
	SKU            string `json:"sku"`
	Known          bool   `json:"known"`
	MinimumCostUSD string `json:"minimum_cost_usd,omitempty"`
	SourceCount    int    `json:"source_count"`
}

type ImagePricingCostSummary struct {
	Items []ImagePricingCostItem `json:"items"`
}
