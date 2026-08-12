package controller

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouteMarginCatalogFilterFromQueryDefaults(t *testing.T) {
	ctx, _ := costCatalogTestContext(http.MethodGet, "/api/cost-accounting/route-margin-catalog")

	filter, err := routeMarginCatalogFilterFromQuery(ctx)

	require.NoError(t, err)
	assert.Equal(t, int64(300000), filter.MinimumMarginPPM)
	assert.Equal(t, 4, filter.DurationSeconds)
	assert.Equal(t, 1.0, filter.GroupRatio)
	assert.Equal(t, "all", filter.Scenario)
	assert.Equal(t, "all", filter.Status)
	assert.Equal(t, 1, filter.Page)
	assert.Equal(t, 50, filter.PageSize)
	assert.Equal(t, "gross_margin_ppm", filter.SortBy)
	assert.Equal(t, "desc", filter.SortOrder)
}

func TestRouteMarginCatalogFilterFromQueryPreservesExplicitZeroMargin(t *testing.T) {
	ctx, _ := costCatalogTestContext(http.MethodGet, "/api/cost-accounting/route-margin-catalog?min_margin_ppm=0&duration_seconds=6&group_ratio=1.25&scenario=with_video&status=eligible&resolution=720p&sort_by=target_name&sort_order=asc")

	filter, err := routeMarginCatalogFilterFromQuery(ctx)

	require.NoError(t, err)
	assert.Equal(t, int64(0), filter.MinimumMarginPPM)
	assert.Equal(t, 6, filter.DurationSeconds)
	assert.Equal(t, 1.25, filter.GroupRatio)
	assert.Equal(t, "with_video", filter.Scenario)
	assert.Equal(t, "eligible", filter.Status)
	assert.Equal(t, "720p", filter.Resolution)
	assert.Equal(t, "target_name", filter.SortBy)
	assert.Equal(t, "asc", filter.SortOrder)
}

func TestRouteMarginCatalogFilterFromQueryRejectsUnsafeValues(t *testing.T) {
	prepareCostCatalogControllerDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.RoutingPolicy{}, &model.RouteTarget{}))
	queries := []string{
		"min_margin_ppm=1000001",
		"duration_seconds=0",
		"duration_seconds=4&group_ratio=0",
		"duration_seconds=4&group_ratio=101",
		"duration_seconds=4&group_ratio=1&scenario=unknown",
		"duration_seconds=4&group_ratio=1&status=pending",
		"duration_seconds=4&group_ratio=1&page_size=30",
		"duration_seconds=4&group_ratio=1&sort_by=config_json",
		"duration_seconds=4&group_ratio=1&sort_order=sideways",
		"duration_seconds=4&group_ratio=1&resolution=" + strings.Repeat("x", 192),
	}
	for _, query := range queries {
		t.Run(query, func(t *testing.T) {
			ctx, recorder := costCatalogTestContext(http.MethodGet, "/api/cost-accounting/route-margin-catalog?"+query)

			ListRouteMarginCatalog(ctx)

			assert.Equal(t, http.StatusBadRequest, recorder.Code)
		})
	}
}

func TestExportRouteMarginCatalogReturnsExcelCompatibleHeaders(t *testing.T) {
	prepareCostCatalogControllerDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.RoutingPolicy{}, &model.RouteTarget{}))
	require.NoError(t, model.DB.Create(&model.Channel{Id: 7, Name: "supplier", Type: 1, Key: "test-key"}).Error)
	policy := model.RoutingPolicy{
		GroupName: "default", Model: "doubao-seedance-2-0-260128", Enabled: true,
		DefaultResolution: "720p", DefaultDuration: 4, DefaultRatio: "16:9",
	}
	require.NoError(t, model.DB.Create(&policy).Error)
	require.NoError(t, model.DB.Create(&model.RouteTarget{
		PolicyID: policy.ID, ChannelID: 7, Name: "route-target/export", UpstreamModel: "vendor-model",
		CostVariantKey: "default", Constraints: `{"output_resolutions":["720p"],"durations":{"values":[4]},"reference_limits":{"images":9,"videos":3,"audios":3}}`,
		Enabled: true, ManagedBy: string(types.RouteTargetManagedByConfigImport),
	}).Error)
	unitPrice := "0.7"
	config, err := service.NormalizeCostRuleConfig(types.CostModePerRequest, types.CostRuleConfigV1{
		Currency: "USD", BillingMultiplier: "1", PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1", FeeRate: "0", CurrencyToUSDRate: "1", UnitPrice: &unitPrice,
	})
	require.NoError(t, err)
	config.ChargeEvent = types.CostChargeResponseSucceeded
	configJSON, err := common.Marshal(config)
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: 7, BillableUpstreamModel: "vendor-model", CostVariantKey: "default", Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(types.CostModePerRequest), SchemaVersion: 1,
		ConfigJSON: string(configJSON), Source: "config_import", EffectiveFrom: &now,
	}).Error)
	previousHook := service.RevenuePreviewHookForTest()
	service.SetRoutingRevenuePreview(func(context.Context, service.RoutingRevenuePreviewInput) (int64, string, error) {
		return 500_000, "500000", nil
	})
	t.Cleanup(func() { service.SetRoutingRevenuePreview(previousHook) })
	ctx, recorder := costCatalogTestContext(http.MethodGet, "/api/cost-accounting/route-margin-catalog/export")

	ExportRouteMarginCatalog(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "text/csv; charset=utf-8", recorder.Header().Get("Content-Type"))
	assert.Contains(t, recorder.Header().Get("Content-Disposition"), `filename="route-margin-catalog-`)
	assert.Contains(t, recorder.Header().Get("Content-Disposition"), "filename*=UTF-8''")
	assert.Equal(t, "2", recorder.Header().Get("X-Exported-Row-Count"))
}

func TestRouteMarginCatalogInternalErrorIsRedacted(t *testing.T) {
	prepareCostCatalogControllerDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.RoutingPolicy{}, &model.RouteTarget{}))
	require.NoError(t, model.DB.Migrator().DropTable(&model.RouteTarget{}))
	ctx, recorder := costCatalogTestContext(http.MethodGet, "/api/cost-accounting/route-margin-catalog")

	ListRouteMarginCatalog(ctx)

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	var response struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "route_margin_catalog_unavailable", response.Code)
	assert.Equal(t, "cost accounting operation failed", response.Message)
	assert.NotContains(t, recorder.Body.String(), "route_targets")
}
