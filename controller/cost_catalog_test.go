package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCostCatalogFilterFromQueryUsesSafeDefaults(t *testing.T) {
	ctx, _ := costCatalogTestContext(http.MethodGet, "/api/cost-accounting/catalog")

	filter, err := costCatalogFilterFromQuery(ctx)
	require.NoError(t, err)
	assert.Equal(t, "active", filter.Status)
	assert.Equal(t, 1, filter.Page)
	assert.Equal(t, 50, filter.PageSize)
	assert.Equal(t, "channel_name", filter.SortBy)
	assert.Equal(t, "asc", filter.SortOrder)
}

func TestCostCatalogFilterFromQueryAcceptsAllStatuses(t *testing.T) {
	for _, status := range []string{"active", "draft", "retired", "all"} {
		t.Run(status, func(t *testing.T) {
			ctx, _ := costCatalogTestContext(http.MethodGet,
				"/api/cost-accounting/catalog?status="+status+"&channel_id=7&page=2&page_size=25&currency=cny&source=config_import&sort_by=version&sort_order=desc",
			)
			filter, err := costCatalogFilterFromQuery(ctx)
			require.NoError(t, err)
			assert.Equal(t, status, filter.Status)
			assert.Equal(t, 7, filter.ChannelID)
			assert.Equal(t, 2, filter.Page)
			assert.Equal(t, 25, filter.PageSize)
			assert.Equal(t, "CNY", filter.Currency)
			assert.Equal(t, "config_import", filter.Source)
			assert.Equal(t, "version", filter.SortBy)
			assert.Equal(t, "desc", filter.SortOrder)
		})
	}
}

func TestCostCatalogFilterFromQueryRejectsInvalidEnumsAndPageSize(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "negative channel", query: "channel_id=-1"},
		{name: "zero page", query: "page=0"},
		{name: "invalid page size", query: "page_size=30"},
		{name: "unknown status", query: "status=pending"},
		{name: "unknown cost mode", query: "cost_mode=per_minute"},
		{name: "unknown sort", query: "sort_by=config_json"},
		{name: "unknown sort order", query: "sort_order=sideways"},
		{name: "long model", query: "billable_upstream_model=" + strings.Repeat("m", 192)},
		{name: "long source", query: "source=" + strings.Repeat("s", 33)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, _ := costCatalogTestContext(http.MethodGet, "/api/cost-accounting/catalog?"+test.query)
			_, err := costCatalogFilterFromQuery(ctx)
			require.Error(t, err)
		})
	}
}

func TestCostCatalogDetailEndpointReturnsNotFound(t *testing.T) {
	prepareCostCatalogControllerDB(t)
	ctx, recorder := costCatalogTestContext(http.MethodGet, "/api/cost-accounting/catalog/999")
	ctx.AddParam("rule_id", "999")

	GetSupplierCostCatalogDetail(ctx)

	assert.Equal(t, http.StatusNotFound, recorder.Code)
	var response struct {
		Code string `json:"code"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "not_found", response.Code)
}

func TestCostCatalogInternalErrorIsRedacted(t *testing.T) {
	ctx, recorder := costCatalogTestContext(http.MethodGet, "/api/cost-accounting/catalog")
	writeCostAccountingError(ctx, fmt.Errorf("%w: SELECT config_json, secret-key", service.ErrCostCatalogUnavailable))

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	var response struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "cost_catalog_unavailable", response.Code)
	assert.Equal(t, "cost accounting operation failed", response.Message)
	assert.NotContains(t, recorder.Body.String(), "config_json")
	assert.NotContains(t, recorder.Body.String(), "secret-key")
}

func TestExportSupplierCostCatalogRequiresExplicitScope(t *testing.T) {
	ctx, recorder := costCatalogTestContext(http.MethodGet, "/api/cost-accounting/catalog/export")

	ExportSupplierCostCatalog(ctx)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestExportSupplierCostCatalogReturnsExcelCompatibleHeaders(t *testing.T) {
	prepareCostCatalogControllerDB(t)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 1, Name: "Alpha", Type: 1, Key: "secret"}).Error)
	unitPrice := "2"
	config, err := service.NormalizeCostRuleConfig(types.CostModePerRequest, types.CostRuleConfigV1{
		Currency: "USD", BillingMultiplier: "1", PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1", FeeRate: "0", CurrencyToUSDRate: "1", UnitPrice: &unitPrice,
	})
	require.NoError(t, err)
	configJSON, err := common.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: 1, BillableUpstreamModel: "vendor-model", CostVariantKey: string(types.DefaultCostVariantKey),
		Version: 1, Status: string(types.CostRuleActive), CostMode: string(types.CostModePerRequest),
		SchemaVersion: 1, ConfigJSON: string(configJSON), Source: "manual",
	}).Error)
	ctx, recorder := costCatalogTestContext(http.MethodGet, "/api/cost-accounting/catalog/export?scope=all")

	ExportSupplierCostCatalog(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "text/csv; charset=utf-8", recorder.Header().Get("Content-Type"))
	assert.Contains(t, recorder.Header().Get("Content-Disposition"), "filename*=UTF-8''")
	assert.Equal(t, "1", recorder.Header().Get("X-Exported-Row-Count"))
	assert.True(t, strings.HasPrefix(recorder.Body.String(), "\uFEFF"))
}

func TestExportSupplierCostCatalogMapsTooLargeTo413(t *testing.T) {
	ctx, recorder := costCatalogTestContext(http.MethodGet, "/api/cost-accounting/catalog/export?scope=all")

	writeCostAccountingError(ctx, service.ErrCostCatalogExportTooLarge)

	assert.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	var response struct {
		Code string `json:"code"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "cost_catalog_export_too_large", response.Code)
}

func costCatalogTestContext(method, target string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, nil)
	return ctx, recorder
}

func prepareCostCatalogControllerDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB := model.DB
	previousMainType := common.MainDatabaseType()
	previousLogType := common.LogDatabaseType()
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, previousLogType)
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}))
}
