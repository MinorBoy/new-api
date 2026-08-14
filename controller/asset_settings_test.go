package controller

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func prepareAssetSettingsDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB := model.DB
	common.OptionMapRWMutex.Lock()
	previousOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Channel{}))
	return db
}

func createAssetSettingsChannel(t *testing.T, db *gorm.DB, name string, status int, key string, group relaydto.SecureVideoGroup, multi bool, channelType int) model.Channel {
	t.Helper()
	baseURL := "https://token.secure-skill.com"
	channel := model.Channel{
		Type: channelType, Key: key, Status: status, Name: name, BaseURL: &baseURL,
		ChannelInfo: model.ChannelInfo{IsMultiKey: multi},
	}
	channel.SetOtherSettings(relaydto.ChannelOtherSettings{SecureVideoGroup: group})
	require.NoError(t, db.Create(&channel).Error)
	return channel
}

func assetSettingsContext(t *testing.T, method string, path string, body string) (*httptest.ResponseRecorder, *gin.Context) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", 1)
	return recorder, ctx
}

func TestSecureAssetSettingsFiltersEligibleChannels(t *testing.T) {
	db := prepareAssetSettingsDB(t)
	eligible := createAssetSettingsChannel(t, db, "eligible", common.ChannelStatusEnabled, "key", relaydto.SecureVideoGroupEnterprise, false, constant.ChannelTypeSecure)
	createAssetSettingsChannel(t, db, "discount", common.ChannelStatusEnabled, "key", relaydto.SecureVideoGroupDiscount, false, constant.ChannelTypeSecure)
	createAssetSettingsChannel(t, db, "disabled", common.ChannelStatusManuallyDisabled, "key", relaydto.SecureVideoGroupEnterprise, false, constant.ChannelTypeSecure)
	createAssetSettingsChannel(t, db, "multi", common.ChannelStatusEnabled, "key-one\nkey-two", relaydto.SecureVideoGroupEnterprise, true, constant.ChannelTypeSecure)
	createAssetSettingsChannel(t, db, "other-type", common.ChannelStatusEnabled, "key", relaydto.SecureVideoGroupEnterprise, false, constant.ChannelTypeOpenAI)
	require.NoError(t, db.Create(&model.Option{Key: service.SecureAssetDefaultChannelOptionKey, Value: strconv.Itoa(eligible.Id)}).Error)

	recorder, ctx := assetSettingsContext(t, http.MethodGet, "/api/asset-settings/secure", "")
	GetSecureAssetSettings(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "eligible")
	assert.NotContains(t, recorder.Body.String(), "discount")
	assert.NotContains(t, recorder.Body.String(), "disabled")
	assert.NotContains(t, recorder.Body.String(), "multi")
	assert.NotContains(t, recorder.Body.String(), "key")
}

func TestSecureAssetDefaultChannelLookupQuotesOptionKeyForMySQL(t *testing.T) {
	var logs bytes.Buffer
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(127.0.0.1:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DisableAutomaticPing: true,
		DryRun:               true,
		Logger:               logger.New(log.New(&logs, "", 0), logger.Config{LogLevel: logger.Info}),
	})
	require.NoError(t, err)

	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	assert.Zero(t, secureAssetDefaultChannelID())
	assert.Contains(t, logs.String(), "`options`.`key`")
	assert.NotContains(t, strings.ReplaceAll(logs.String(), "`options`.`key`", ""), "WHERE key")
}

func TestUpdateSecureAssetSettingsValidatesAndPersistsDefaultChannel(t *testing.T) {
	db := prepareAssetSettingsDB(t)
	eligible := createAssetSettingsChannel(t, db, "eligible", common.ChannelStatusEnabled, "key", relaydto.SecureVideoGroupEnterprise, false, constant.ChannelTypeSecure)
	invalid := createAssetSettingsChannel(t, db, "invalid", common.ChannelStatusEnabled, "key", relaydto.SecureVideoGroupDiscount, false, constant.ChannelTypeSecure)

	recorder, ctx := assetSettingsContext(t, http.MethodPut, "/api/asset-settings/secure", `{"channel_id":0}`)
	UpdateSecureAssetSettings(ctx)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)

	recorder, ctx = assetSettingsContext(t, http.MethodPut, "/api/asset-settings/secure", `{"channel_id":9999}`)
	UpdateSecureAssetSettings(ctx)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)

	recorder, ctx = assetSettingsContext(t, http.MethodPut, "/api/asset-settings/secure", `{"channel_id":`+strconv.Itoa(invalid.Id)+`}`)
	UpdateSecureAssetSettings(ctx)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)

	recorder, ctx = assetSettingsContext(t, http.MethodPut, "/api/asset-settings/secure", `{"channel_id":`+strconv.Itoa(eligible.Id)+`}`)
	UpdateSecureAssetSettings(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	var option model.Option
	require.NoError(t, db.First(&option, "key = ?", service.SecureAssetDefaultChannelOptionKey).Error)
	assert.Equal(t, strconv.Itoa(eligible.Id), option.Value)
}
