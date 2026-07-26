package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/cost_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCostRoutingGetChannelSkipsUncoveredCandidate(t *testing.T) {
	prepareCostRoutingControllerTest(t, types.CostAccountingStrict)
	seedCostRoutingChannel(t, 11, 200, `{"client-model":"missing-model"}`)
	seedCostRoutingChannel(t, 12, 100, `{"client-model":"covered-model"}`)
	seedActiveFreeCostRule(t, 12, "covered-model")

	c := costRoutingControllerContext()
	info := costRoutingRelayInfo()
	retryParam := costRoutingRetryParam(c)

	channel, apiErr := getChannel(c, info, retryParam)
	require.Nil(t, apiErr)
	require.NotNil(t, channel)
	assert.Equal(t, 12, channel.Id)
}

func TestCostRoutingGetChannelKeepsLegacySelectionWhenDisabled(t *testing.T) {
	prepareCostRoutingControllerTest(t, types.CostAccountingDisabled)
	seedCostRoutingChannel(t, 11, 200, `{"client-model":"missing-model"}`)
	seedCostRoutingChannel(t, 12, 100, `{"client-model":"covered-model"}`)

	c := costRoutingControllerContext()
	channel, apiErr := getChannel(c, costRoutingRelayInfo(), costRoutingRetryParam(c))
	require.Nil(t, apiErr)
	require.NotNil(t, channel)
	assert.Equal(t, 11, channel.Id)
}

func TestCostRoutingAutoGroupContinuesAfterUncoveredCandidate(t *testing.T) {
	prepareCostRoutingControllerTest(t, types.CostAccountingStrict)
	configureCostRoutingAutoGroups(t)
	seedCostRoutingChannelForGroup(t, 11, 200, `{"client-model":"missing-model"}`, "分组A")
	seedCostRoutingChannelForGroup(t, 12, 100, `{"client-model":"covered-model"}`, "分组B")
	seedActiveFreeCostRule(t, 12, "covered-model")

	c := costRoutingControllerContext()
	common.SetContextKey(c, constant.ContextKeyUserGroup, "分组A")
	info := costRoutingRelayInfo()
	info.TokenGroup = "auto"
	info.UsingGroup = "auto"
	retryParam := costRoutingRetryParam(c)
	retryParam.TokenGroup = "auto"

	channel, apiErr := getChannel(c, info, retryParam)
	require.Nil(t, apiErr)
	require.NotNil(t, channel)
	assert.Equal(t, 12, channel.Id)
	assert.Equal(t, "分组B", common.GetContextKeyString(c, constant.ContextKeyAutoGroup))
}

func TestCostRoutingAllUncoveredReturnsGenericError(t *testing.T) {
	prepareCostRoutingControllerTest(t, types.CostAccountingStrict)
	seedCostRoutingChannel(t, 11, 200, `{"client-model":"supplier-secret-model"}`)

	c := costRoutingControllerContext()
	channel, apiErr := getChannel(c, costRoutingRelayInfo(), costRoutingRetryParam(c))
	assert.Nil(t, channel)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeGetChannelFailed, apiErr.GetErrorCode())

	message := strings.ToLower(apiErr.Error())
	for _, forbidden := range []string{"supplier-secret-model", "cost", "price", "rule"} {
		assert.NotContains(t, message, forbidden)
	}
}

func TestCostRoutingDistributorSkipsUncoveredInitialCandidate(t *testing.T) {
	prepareCostRoutingControllerTest(t, types.CostAccountingStrict)
	seedCostRoutingChannel(t, 11, 200, `{"client-model":"missing-model"}`)
	seedCostRoutingChannel(t, 12, 100, `{"client-model":"covered-model"}`)
	seedActiveFreeCostRule(t, 12, "covered-model")

	recorder := performCostRoutingDistribution(t, "")
	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		ChannelID int `json:"channel_id"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, 12, response.ChannelID)
}

func TestCostRoutingDistributorRejectsUncoveredSpecificChannelGenerically(t *testing.T) {
	prepareCostRoutingControllerTest(t, types.CostAccountingStrict)
	seedCostRoutingChannel(t, 11, 200, `{"client-model":"supplier-secret-model"}`)

	recorder := performCostRoutingDistribution(t, "11")
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	message := strings.ToLower(recorder.Body.String())
	for _, forbidden := range []string{"supplier-secret-model", "cost", "price", "rule"} {
		assert.NotContains(t, message, forbidden)
	}
}

func prepareCostRoutingControllerTest(t *testing.T, mode types.CostAccountingMode) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelCostRule{}))

	previousDB := model.DB
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousLookup := service.CostCapabilityLookup
	model.DB = db
	common.MemoryCacheEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	service.CostCapabilityLookup = func(int, string, constant.TaskPlatform) types.CostCapabilities {
		return types.CostCapabilities{CanResolveBillableModel: true}
	}
	service.InvalidateCostCoverage(0, "", "")

	costConfig := config.GlobalConfig.Get(cost_setting.ConfigName)
	require.NotNil(t, costConfig)
	previousMode := cost_setting.Runtime().Mode
	require.NoError(t, config.UpdateConfigFromMap(costConfig, map[string]string{cost_setting.KeyMode: string(mode)}))
	cost_setting.UpdateAndSync()

	// Inject a revenue preview callback so strict-mode profit routing has a positive
	// revenue to compare against the (typically free) cost rules these tests seed.
	// Mirrors the production main.go wiring; the callback returns a fixed positive
	// quota so free-cost candidates pass the margin gate.
	previousRevenueHook := service.RevenuePreviewHookForTest()
	service.SetRoutingRevenuePreview(func(_ context.Context, _ service.RoutingRevenuePreviewInput) (int64, string, error) {
		return 1_000_000, "500000", nil
	})

	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(costConfig, map[string]string{cost_setting.KeyMode: string(previousMode)}))
		cost_setting.UpdateAndSync()
		service.InvalidateCostCoverage(0, "", "")
		service.CostCapabilityLookup = previousLookup
		service.SetRoutingRevenuePreview(previousRevenueHook)
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		model.DB = previousDB
		require.NoError(t, sqlDB.Close())
	})
}

func seedCostRoutingChannel(t *testing.T, channelID int, priority int64, mapping string) {
	t.Helper()
	seedCostRoutingChannelForGroup(t, channelID, priority, mapping, "default")
}

func seedCostRoutingChannelForGroup(t *testing.T, channelID int, priority int64, mapping, group string) {
	t.Helper()
	weight := uint(10)
	require.NoError(t, model.DB.Create(&model.Channel{
		Id: channelID, Type: constant.ChannelTypeOpenAI, Name: "candidate", Key: "secret",
		Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight, ModelMapping: &mapping, Group: group,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group: group, Model: "client-model", ChannelId: channelID, Enabled: true,
		Priority: &priority, Weight: weight,
	}).Error)
}

func configureCostRoutingAutoGroups(t *testing.T) {
	t.Helper()
	previousAutoGroups := setting.AutoGroups2JsonString()
	previousUsableGroups := setting.UserUsableGroups2JSONString()
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["分组A","分组B"]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"分组A":"A","分组B":"B"}`))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(previousAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousUsableGroups))
	})
}

func seedActiveFreeCostRule(t *testing.T, channelID int, modelName string) {
	t.Helper()
	configJSON, err := common.Marshal(types.CostRuleConfigV1{ZeroCostReason: "supplier contract"})
	require.NoError(t, err)
	now := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.ChannelModelCostRule{
		ChannelID: channelID, BillableUpstreamModel: modelName, CostVariantKey: string(types.DefaultCostVariantKey), Version: 1,
		Status: string(types.CostRuleActive), CostMode: string(types.CostModeFree), SchemaVersion: 1,
		ConfigJSON: string(configJSON), Source: "manual", EffectiveFrom: &now,
		CreatedAt: now, UpdatedAt: now,
	}).Error)
}

func costRoutingControllerContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	return c
}

func costRoutingRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{}, OriginModelName: "client-model",
		TokenGroup: "default", UserGroup: "default", UsingGroup: "default",
	}
}

func costRoutingRetryParam(c *gin.Context) *service.RetryParam {
	return &service.RetryParam{
		Ctx: c, TokenGroup: "default", ModelName: "client-model",
		RequestPath: c.Request.URL.Path, Retry: common.GetPointer(0),
	}
}

func performCostRoutingDistribution(t *testing.T, specificChannelID string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		if specificChannelID != "" {
			common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, specificChannelID)
		}
		c.Next()
	})
	router.Use(middleware.Distribute())
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"channel_id": common.GetContextKeyInt(c, constant.ContextKeyChannelId)})
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"client-model"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}
