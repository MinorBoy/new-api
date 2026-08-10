package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouteMarginCatalogPageJSONContract(t *testing.T) {
	revenue := int64(1000)
	cost := int64(700)
	profit := int64(300)
	margin := int64(300000)
	encoded, err := common.Marshal(RouteMarginCatalogPage{
		Items: []RouteMarginCatalogItem{{
			TargetID: 1, TargetName: "route-target/a", PolicyID: 2,
			GroupName: "default", CanonicalModel: "doubao-seedance-2-0-260128",
			ChannelID: 3, ChannelName: "channel-a", ChannelType: 47,
			UpstreamModel: "vendor-model", CostVariantKey: "720p",
			Resolution: "720p", DurationSeconds: 4, Scenario: "no_video", GroupRatio: "1",
			CostMode: types.CostModePerRequest, RuleID: 4, RuleVersion: 1,
			EstimatedRevenueNanoUSD: &revenue, EstimatedCostNanoUSD: &cost,
			EstimatedProfitNanoUSD: &profit, GrossMarginPPM: &margin,
			RequestedMinimumMarginPPM: 300000, Eligible: true,
			CostSource: "config_import", RevenueSource: "runtime_billing_settings",
		}},
		Total: 1, Page: 1, PageSize: 50,
		Summary: RouteMarginCatalogSummary{TargetCount: 1, ScenarioCount: 1, EligibleTargetCount: 1, FullyEligibleTargetCount: 1, EligibleScenarioCount: 1},
		Facets:  RouteMarginCatalogFacets{Resolutions: []string{"720p"}, CanonicalModels: []string{"doubao-seedance-2-0-260128"}},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{
      "items":[{
        "target_id":1,"target_name":"route-target/a","policy_id":2,
        "group_name":"default","canonical_model":"doubao-seedance-2-0-260128",
        "channel_id":3,"channel_name":"channel-a","channel_type":47,
        "upstream_model":"vendor-model","cost_variant_key":"720p",
        "resolution":"720p","duration_seconds":4,"scenario":"no_video","group_ratio":"1",
        "cost_mode":"per_request","rule_id":4,"rule_version":1,
        "estimated_revenue_nano_usd":1000,"estimated_cost_nano_usd":700,
        "estimated_profit_nano_usd":300,"gross_margin_ppm":300000,
        "requested_minimum_margin_ppm":300000,"eligible":true,
        "cost_source":"config_import","revenue_source":"runtime_billing_settings"
      }],
      "total":1,"page":1,"page_size":50,
      "summary":{"target_count":1,"scenario_count":1,"eligible_target_count":1,"fully_eligible_target_count":1,"partially_eligible_target_count":0,"ineligible_target_count":0,"eligible_scenario_count":1},
      "facets":{"channels":null,"resolutions":["720p"],"canonical_models":["doubao-seedance-2-0-260128"]}
    }`, string(encoded))
}
