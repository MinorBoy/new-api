package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/video_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupOptionControllerVideoSettingTest(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalRedisEnabled := common.RedisEnabled
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.RedisEnabled = originalRedisEnabled
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.User{}, &model.Log{}))
	setupVideoSettingOptionMap(t)
	resetVideoSettingConfig(t)
	return db
}

func setupVideoSettingOptionMap(t *testing.T) {
	t.Helper()

	keys := []string{
		"video_setting.base64_input_enabled",
		"video_setting.json_request_body_max_mb",
	}
	common.OptionMapRWMutex.Lock()
	optionMapWasNil := common.OptionMap == nil
	if optionMapWasNil {
		common.OptionMap = make(map[string]string)
	}
	previousValues := make(map[string]string, len(keys))
	hadPreviousValues := make(map[string]bool, len(keys))
	for _, key := range keys {
		previousValues[key] = common.OptionMap[key]
		_, hadPreviousValues[key] = common.OptionMap[key]
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		if optionMapWasNil {
			common.OptionMap = nil
			return
		}
		for _, key := range keys {
			if hadPreviousValues[key] {
				common.OptionMap[key] = previousValues[key]
			} else {
				delete(common.OptionMap, key)
			}
		}
	})
}

func resetVideoSettingConfig(t *testing.T) {
	t.Helper()

	cfg := config.GlobalConfig.Get(video_setting.ConfigName)
	require.NotNil(t, cfg)
	snapshot := video_setting.Runtime()
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{
			video_setting.KeyBase64InputEnabled:   strconv.FormatBool(snapshot.Base64InputEnabled),
			video_setting.KeyJSONRequestBodyMaxMB: strconv.Itoa(snapshot.JSONRequestBodyMaxMB),
		}))
		video_setting.UpdateAndSync()
	})
	require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{
		video_setting.KeyBase64InputEnabled:   "false",
		video_setting.KeyJSONRequestBodyMaxMB: strconv.Itoa(video_setting.DefaultJSONRequestBodyMaxMB),
	}))
	video_setting.UpdateAndSync()
}

func performUpdateOptionRequest(t *testing.T, key string, rawValue string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(
		fmt.Sprintf(`{"key":%q,"value":%s}`, key, rawValue),
	))
	ctx.Request.Header.Set("Content-Type", "application/json")

	UpdateOption(ctx)

	return recorder
}

func decodeOptionUpdateResponse(t *testing.T, recorder *httptest.ResponseRecorder) struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
} {
	t.Helper()

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestUpdateOptionRejectsInvalidVideoJSONBodyLimitBeforePersistence(t *testing.T) {
	db := setupOptionControllerVideoSettingTest(t)

	for _, rawValue := range []string{`0`, `-1`, `129`, `"1.5"`, `"abc"`} {
		t.Run(rawValue, func(t *testing.T) {
			recorder := performUpdateOptionRequest(t, "video_setting.json_request_body_max_mb", rawValue)
			response := decodeOptionUpdateResponse(t, recorder)

			assert.False(t, response.Success)
			assert.Contains(t, response.Message, "video JSON request body limit")

			var count int64
			require.NoError(t, db.Model(&model.Option{}).Where("key = ?", "video_setting.json_request_body_max_mb").Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestUpdateOptionAcceptsVideoJSONBodyLimitAndSyncsRuntime(t *testing.T) {
	setupOptionControllerVideoSettingTest(t)

	recorder := performUpdateOptionRequest(t, "video_setting.json_request_body_max_mb", `32`)
	response := decodeOptionUpdateResponse(t, recorder)

	require.True(t, response.Success)
	assert.Equal(t, 32, video_setting.Runtime().JSONRequestBodyMaxMB)
}

func TestUpdateOptionRejectsInvalidVideoBase64Switch(t *testing.T) {
	db := setupOptionControllerVideoSettingTest(t)

	recorder := performUpdateOptionRequest(t, "video_setting.base64_input_enabled", `"yes"`)
	response := decodeOptionUpdateResponse(t, recorder)

	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "video Base64 input setting")

	var count int64
	require.NoError(t, db.Model(&model.Option{}).Where("key = ?", "video_setting.base64_input_enabled").Count(&count).Error)
	assert.Zero(t, count)
}
