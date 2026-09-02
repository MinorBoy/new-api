package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/image_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetImagePricingCostSummaryReturnsStableContract(t *testing.T) {
	prepareImagePricingControllerDB(t)
	originalCatalog := image_setting.Catalog2JSONString()
	t.Cleanup(func() { require.NoError(t, image_setting.UpdateCatalogByJSONString(originalCatalog)) })
	require.NoError(t, image_setting.UpdateCatalogByJSONString(controllerImagePricingCatalogFixture))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/cost-accounting/image-pricing?model=gpt-image-2", nil)
	GetImagePricingCostSummary(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"items"`)
	assert.NotContains(t, body, "secret")
}

func TestGetImagePricingCostSummaryRejectsOverlongModel(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/cost-accounting/image-pricing?model="+url.QueryEscape(strings.Repeat("m", 192)), nil)
	GetImagePricingCostSummary(ctx)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
}

func prepareImagePricingControllerDB(t *testing.T) {
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
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelModelCostRule{}))
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		require.NoError(t, sqlDB.Close())
	})
}

const controllerImagePricingCatalogFixture = `{"version":1,"models":{"gpt-image-2":{"profile":"openai_images","profile_version":1,"endpoints":{"generations":{"capability":{"enabled":true,"sizes":["1024x1024"],"qualities":["medium"],"response_formats":["b64_json"],"max_n":4},"default_size":"1024x1024","default_quality":"medium","default_response_format":"b64_json"}},"skus":{"gen-1024x1024-medium":{"endpoint":"generations","size":"1024x1024","quality":"medium","unit":"image","sale_price_usd":"0.03"}}}}}`
