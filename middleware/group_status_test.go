package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTokenAuthRejectsDisabledUserGroupBeforeDownstream(t *testing.T) {
	setupDisabledGroupAuthTest(t, "paused", "")
	called := false
	engine := gin.New()
	engine.Use(TokenAuth())
	engine.POST("/v1/videos", func(c *gin.Context) {
		called = true
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	request.Header.Set("Authorization", "Bearer sk-pausedkey")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.False(t, called)
	assert.Contains(t, recorder.Body.String(), "paused")
}

func TestTokenAuthRejectsDisabledTokenGroupBeforeDownstream(t *testing.T) {
	setupDisabledGroupAuthTest(t, "default", "paused")
	called := false
	engine := gin.New()
	engine.Use(TokenAuth())
	engine.POST("/v1/videos", func(c *gin.Context) {
		called = true
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	request.Header.Set("Authorization", "Bearer sk-pausedkey")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.False(t, called)
	assert.Contains(t, recorder.Body.String(), "paused")
}

func setupDisabledGroupAuthTest(t *testing.T, userGroup, tokenGroup string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	require.NoError(t, db.Create(&model.User{
		Id: 1, Username: "paused-user", Group: userGroup, Status: common.UserStatusEnabled, AffCode: "paused-auth-aff",
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		UserId: 1, Key: "pausedkey", Name: "paused-token", Group: tokenGroup, Status: common.TokenStatusEnabled,
		ExpiredTime: -1, UnlimitedQuota: true,
	}).Error)

	originalDB := model.DB
	originalRedisEnabled := common.RedisEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalStatus := ratio_setting.GroupStatus2JSONString()
	model.DB = db
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	require.NoError(t, ratio_setting.UpdateGroupStatusByJSONString(`{"paused":false}`))
	t.Cleanup(func() {
		model.DB = originalDB
		common.RedisEnabled = originalRedisEnabled
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		require.NoError(t, ratio_setting.UpdateGroupStatusByJSONString(originalStatus))
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
}
