package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpdateOptionRejectsInvalidDurationPriceBeforePersistence(t *testing.T) {
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

	const optionKey = "billing_setting.duration_price"
	const originalValue = `{"existing":{"price":1,"unit":"second","rounding_step_seconds":1,"minimum_duration_seconds":0}}`
	require.NoError(t, db.Create(&model.Option{Key: optionKey, Value: originalValue}).Error)

	common.OptionMapRWMutex.Lock()
	optionMapWasNil := common.OptionMap == nil
	if optionMapWasNil {
		common.OptionMap = make(map[string]string)
	}
	previousValue, hadPreviousValue := common.OptionMap[optionKey]
	common.OptionMap[optionKey] = originalValue
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		if optionMapWasNil {
			common.OptionMap = nil
			return
		}
		if hadPreviousValue {
			common.OptionMap[optionKey] = previousValue
		} else {
			delete(common.OptionMap, optionKey)
		}
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(
		`{"key":"billing_setting.duration_price","value":"{\"video\":{\"price\":-1,\"unit\":\"second\",\"rounding_step_seconds\":1,\"minimum_duration_seconds\":0}}"}`,
	))
	ctx.Request.Header.Set("Content-Type", "application/json")

	UpdateOption(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "duration price")

	var persisted model.Option
	require.NoError(t, db.First(&persisted, "key = ?", optionKey).Error)
	assert.Equal(t, originalValue, persisted.Value)
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, originalValue, common.OptionMap[optionKey])
	common.OptionMapRWMutex.RUnlock()
}

func TestGetOptionsReturnsEffectiveDurationBillingDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)

	savedBillingOptions := make(map[string]string)
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		if strings.HasPrefix(key, "billing_setting.") {
			savedBillingOptions[key] = value
		}
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(savedBillingOptions))
	})

	const modeKey = "billing_setting.billing_mode"
	const priceKey = "billing_setting.duration_price"
	const oldModeJSON = `{"legacy-model":"tiered_expr"}`
	const oldPriceJSON = `{"legacy-video":{"price":2,"unit":"minute","rounding_step_seconds":5,"minimum_duration_seconds":10}}`
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		modeKey:  oldModeJSON,
		priceKey: oldPriceJSON,
	}))

	common.OptionMapRWMutex.Lock()
	optionMapWasNil := common.OptionMap == nil
	if optionMapWasNil {
		common.OptionMap = make(map[string]string)
	}
	previousMode, hadPreviousMode := common.OptionMap[modeKey]
	previousPrice, hadPreviousPrice := common.OptionMap[priceKey]
	common.OptionMap[modeKey] = oldModeJSON
	common.OptionMap[priceKey] = oldPriceJSON
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		if optionMapWasNil {
			common.OptionMap = nil
			return
		}
		if hadPreviousMode {
			common.OptionMap[modeKey] = previousMode
		} else {
			delete(common.OptionMap, modeKey)
		}
		if hadPreviousPrice {
			common.OptionMap[priceKey] = previousPrice
		} else {
			delete(common.OptionMap, priceKey)
		}
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/option/", nil)

	GetOptions(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool            `json:"success"`
		Data    []*model.Option `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)

	values := make(map[string]string, len(response.Data))
	for _, option := range response.Data {
		values[option.Key] = option.Value
	}

	var modes map[string]string
	require.NoError(t, common.UnmarshalJsonStr(values[modeKey], &modes))
	assert.Equal(t, billing_setting.BillingModeTieredExpr, modes["legacy-model"])
	assert.Equal(t, billing_setting.BillingModePerDuration, modes["jimeng-video-seedance-2.0-vip"])

	var prices map[string]types.DurationPrice
	require.NoError(t, common.UnmarshalJsonStr(values[priceKey], &prices))
	assert.Equal(t, 2.0, prices["legacy-video"].Price)
	defaultRule, ok := prices["jimeng-video-seedance-2.0-vip"]
	require.True(t, ok)
	assert.InDelta(t, 0.62/7.3, defaultRule.Price, 1e-10)
}
