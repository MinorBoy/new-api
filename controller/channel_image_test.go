package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/image_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTestChannelImageProfileDoesNotPersistInvalidRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB := model.DB
	previousMemoryCache := common.MemoryCacheEnabled
	model.DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCache
		_ = sqlDB.Close()
	})
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	require.NoError(t, image_setting.UpdateCatalogByJSONString(`{"version":1,"models":{"gpt-image-1":{"profile":"openai_images","profile_version":1,"endpoints":{"generations":{"capability":{"enabled":true,"sizes":["1024x1024"],"qualities":["medium"],"response_formats":["b64_json"],"max_n":1},"default_size":"1024x1024","default_quality":"medium","default_response_format":"b64_json"}},"skus":{"gen-1024x1024-medium":{"endpoint":"generations","size":"1024x1024","quality":"medium","unit":"image","sale_price_usd":"0.1"}}}}}`))
	baseURL := "https://example.invalid"
	channel := &model.Channel{Id: 1, Type: constant.ChannelTypeOpenAI, Name: "image", Models: "gpt-image-1", BaseURL: &baseURL, OtherSettings: `{"image_profile":{"profile":"openai_images","profile_version":1}}`}
	require.NoError(t, db.Create(channel).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "1"}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/1/image-profile/test", strings.NewReader(`{"model":"unknown-image","endpoint":"generations"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	TestChannelImageProfile(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	reloaded, err := model.GetChannelById(1, true)
	require.NoError(t, err)
	assert.NotContains(t, reloaded.OtherSettings, "unknown-image:generations")
}
