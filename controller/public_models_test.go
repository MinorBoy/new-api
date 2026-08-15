package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const internalSeedanceModel = "mg-seedance2.0-480p-fast-gz-15s"

func TestProjectPublicPricingOnlyProjectsSeedanceFamily(t *testing.T) {
	pricing := []model.Pricing{
		{ModelName: "gpt-4o", VendorID: 10, OwnerBy: "openai", Icon: "OpenAI", SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAIResponse}},
		{ModelName: internalSeedanceModel, VendorID: 99, OwnerBy: "internal", SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeAnthropic}},
		{ModelName: modelrouting.Seedance20Mini, VendorID: 99, OwnerBy: "internal", SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAI}},
		{ModelName: modelrouting.Seedance20, VendorID: 99, OwnerBy: "internal", SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAI}},
		{ModelName: modelrouting.Seedance20Fast, VendorID: 99, OwnerBy: "internal", SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAI}},
	}
	vendors := []model.PricingVendor{
		{ID: 10, Name: "OpenAI", Icon: "OpenAI"},
		{ID: 99, Name: "Internal Seedance", Icon: "Internal"},
	}
	endpoints := map[string]common.EndpointInfo{
		string(constant.EndpointTypeOpenAI):         {Path: "/v1/chat/completions", Method: "POST"},
		string(constant.EndpointTypeOpenAIResponse): {Path: "/v1/responses", Method: "POST"},
		string(constant.EndpointTypeAnthropic):      {Path: "/v1/messages", Method: "POST"},
	}

	projection := projectPublicPricing(pricing, vendors, endpoints)
	require.Equal(t, []string{
		"gpt-4o",
		modelrouting.Seedance20Mini,
		modelrouting.Seedance20,
		modelrouting.Seedance20Fast,
	}, []string{
		projection.Pricing[0].ModelName,
		projection.Pricing[1].ModelName,
		projection.Pricing[2].ModelName,
		projection.Pricing[3].ModelName,
	})
	assert.Equal(t, 10, projection.Pricing[0].VendorID)
	assert.Equal(t, "openai", projection.Pricing[0].OwnerBy)
	assert.Equal(t, "OpenAI", projection.Pricing[0].Icon)
	for _, item := range projection.Pricing[1:] {
		assert.Equal(t, publicDoubaoVendor.ID, item.VendorID)
		assert.Equal(t, modelrouting.PublicModelOwner, item.OwnerBy)
		assert.Equal(t, publicDoubaoVendor.Icon, item.Icon)
	}
	require.Equal(t, []model.PricingVendor{vendors[0], publicDoubaoVendor}, projection.Vendors)
	require.Contains(t, projection.SupportedEndpoints, string(constant.EndpointTypeOpenAI))
	require.Contains(t, projection.SupportedEndpoints, string(constant.EndpointTypeOpenAIResponse))
	require.NotContains(t, projection.SupportedEndpoints, string(constant.EndpointTypeAnthropic))
}

func seedPublicModelAbilities(t *testing.T, db *gorm.DB, modelNames []string) {
	t.Helper()

	require.NoError(t, db.Create(&model.User{
		Id: 19001, Username: "public-model-user", Password: "password",
		Group: "default", Status: common.UserStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Id: 19001, Type: constant.ChannelTypeOpenAI, Name: "public-model-channel",
		Models: strings.Join(modelNames, ","), Group: "default", Status: common.ChannelStatusEnabled,
	}).Error)

	abilities := make([]model.Ability, 0, len(modelNames))
	for _, modelName := range modelNames {
		abilities = append(abilities, model.Ability{
			Group: "default", Model: modelName, ChannelId: 19001, Enabled: true,
		})
	}
	require.NoError(t, db.Create(&abilities).Error)
}

func mixedPublicModelNames() []string {
	return []string{
		"gpt-4o",
		internalSeedanceModel,
		modelrouting.Seedance20,
		modelrouting.Seedance20Fast,
		modelrouting.Seedance20Mini,
		modelrouting.Seedance25,
	}
}

func TestListModelsKeepsNonSeedanceAndUsesDoubaoOwnerOnlyForPublicSeedance(t *testing.T) {
	withSelfUseModeEnabled(t)
	db := setupModelListControllerTestDB(t)
	seedPublicModelAbilities(t, db, mixedPublicModelNames())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	ListModels(ctx, constant.ChannelTypeOpenAI)

	payload := decodeListModelsPayload(t, recorder)
	require.Len(t, payload.Data, 5)
	modelsByID := make(map[string]dto.OpenAIModels, len(payload.Data))
	for _, item := range payload.Data {
		modelsByID[item.Id] = item
	}
	assert.Equal(t, "openai", modelsByID["gpt-4o"].OwnedBy)
	for _, modelName := range modelrouting.CanonicalModels {
		assert.Equal(t, modelrouting.PublicModelOwner, modelsByID[modelName].OwnedBy)
	}
	assert.NotContains(t, modelsByID, internalSeedanceModel)
}

func TestListModelsCompatibilityFormatsKeepNonSeedanceAndHideInternalSeedance(t *testing.T) {
	for _, modelType := range []int{constant.ChannelTypeAnthropic, constant.ChannelTypeGemini} {
		t.Run(constant.GetChannelTypeName(modelType), func(t *testing.T) {
			withSelfUseModeEnabled(t)
			db := setupModelListControllerTestDB(t)
			seedPublicModelAbilities(t, db, mixedPublicModelNames())

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
			ListModels(ctx, modelType)

			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "gpt-4o")
			assert.NotContains(t, recorder.Body.String(), internalSeedanceModel)
			for _, modelName := range modelrouting.CanonicalModels {
				assert.Contains(t, recorder.Body.String(), modelName)
			}
		})
	}
}

func TestListModelsTokenLimitCannotExposeInternalSeedance(t *testing.T) {
	withSelfUseModeEnabled(t)
	db := setupModelListControllerTestDB(t)
	seedModelListAbility(t, db, "default", "gpt-4o")

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimit, map[string]bool{
		"gpt-4o":              true,
		internalSeedanceModel: true,
	})

	ListModels(ctx, constant.ChannelTypeOpenAI)

	payload := decodeListModelsPayload(t, recorder)
	require.Len(t, payload.Data, 1)
	assert.Equal(t, "gpt-4o", payload.Data[0].Id)
}

func TestListModelsTokenLimitCannotBypassDynamicGroupProfile(t *testing.T) {
	withSelfUseModeEnabled(t)
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.RoutingPolicy{}, &model.RouteTarget{}, &model.ChannelModelCostRule{}))
	priority := int64(100)
	weight := uint(10)
	require.NoError(t, db.Create(&model.Channel{
		Id: 11, Type: constant.ChannelTypeOpenAI, Name: "request-only", Key: "secret",
		Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group: "default", Model: modelrouting.Seedance20, ChannelId: 11, Enabled: true,
		Priority: &priority, Weight: weight,
	}).Error)
	require.NoError(t, db.Create(&model.RoutingPolicy{
		ID: 1, GroupName: "default", Model: modelrouting.Seedance20, Enabled: true,
		DefaultResolution: "720p", DefaultDuration: 10, DefaultRatio: "16:9",
	}).Error)
	supportsRealPerson := true
	constraints, err := common.Marshal(modelrouting.Constraints{
		OutputResolutions:  []string{"720p"},
		Durations:          modelrouting.DurationConstraint{Min: common.GetPointer(4), Max: common.GetPointer(15)},
		AspectRatios:       []string{"16:9"},
		ReferenceLimits:    modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
		SupportsRealPerson: &supportsRealPerson,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.RouteTarget{
		PolicyID: 1, ChannelID: 11, Name: "request-upstream", UpstreamModel: "request-upstream",
		CostVariantKey: string(hosttypes.DefaultCostVariantKey), TargetPriority: 100,
		Enabled: true, Constraints: string(constraints),
	}).Error)
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&model.ChannelModelCostRule{
		ChannelID: 11, BillableUpstreamModel: "request-upstream", CostVariantKey: string(hosttypes.DefaultCostVariantKey),
		Version: 1, Status: string(hosttypes.CostRuleActive), CostMode: string(hosttypes.CostModePerRequest),
		SchemaVersion: 1, ConfigJSON: `{}`, Source: "manual", EffectiveFrom: &now, CreatedAt: now, UpdatedAt: now,
	}).Error)
	originalProfiles := ratio_setting.GroupRoutingRequirements2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRoutingRequirementsByJSONString(`{
		"按秒客户":{"status":"active","routing_source":"default","allowed_cost_modes":["per_duration"]}
	}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRoutingRequirementsByJSONString(originalProfiles))
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "按秒客户")
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimit, map[string]bool{
		modelrouting.Seedance20: true,
	})

	ListModels(ctx, constant.ChannelTypeOpenAI)

	payload := decodeListModelsPayload(t, recorder)
	assert.Empty(t, payload.Data)
}

func TestListModelsEmptyAnthropicCatalogDoesNotPanic(t *testing.T) {
	withSelfUseModeEnabled(t)
	setupModelListControllerTestDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimit, map[string]bool{
		internalSeedanceModel: true,
	})

	ListModels(ctx, constant.ChannelTypeAnthropic)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Data    []dto.AnthropicModel `json:"data"`
		HasMore bool                 `json:"has_more"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	assert.Empty(t, payload.Data)
	assert.False(t, payload.HasMore)
}

func TestRetrieveModelRejectsInternalSeedanceAndKeepsOtherPublicModels(t *testing.T) {
	for _, hiddenModel := range []string{internalSeedanceModel, "doubao-seedance-2-0-mini-260128"} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Params = gin.Params{{Key: "model", Value: hiddenModel}}

		RetrieveModel(ctx, constant.ChannelTypeOpenAI)

		require.Equal(t, http.StatusOK, recorder.Code)
		var payload struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
		assert.Equal(t, "model_not_found", payload.Error.Code)
	}

	for modelName, expectedOwner := range map[string]string{
		"gpt-4o":                openAIModelsMap["gpt-4o"].OwnedBy,
		modelrouting.Seedance20: modelrouting.PublicModelOwner,
	} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Params = gin.Params{{Key: "model", Value: modelName}}

		RetrieveModel(ctx, constant.ChannelTypeOpenAI)

		require.Equal(t, http.StatusOK, recorder.Code)
		var publicModel dto.OpenAIModels
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &publicModel))
		assert.Equal(t, modelName, publicModel.Id)
		assert.Equal(t, expectedOwner, publicModel.OwnedBy)
		if modelName == "gpt-4o" {
			assert.NotEqual(t, modelrouting.PublicModelOwner, publicModel.OwnedBy)
		}
	}
}

func TestGetUserModelsKeepsNonSeedanceAndHidesInternalSeedance(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	seedPublicModelAbilities(t, db, mixedPublicModelNames())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/models?group=default", nil)
	ctx.Set("id", 19001)

	GetUserModels(ctx)

	assert.ElementsMatch(t, append([]string{"gpt-4o"}, modelrouting.CanonicalModels...), decodeUserModelsResponse(t, recorder))
}

func TestDashboardListModelsOnlyFiltersInternalSeedancePerChannel(t *testing.T) {
	original := channelId2Models
	channelId2Models = map[int][]string{
		constant.ChannelTypeOpenAI: {
			"gpt-4o", internalSeedanceModel, modelrouting.Seedance20,
		},
		constant.ChannelTypeAnthropic: {"claude-sonnet-4-5"},
	}
	t.Cleanup(func() { channelId2Models = original })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	DashboardListModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Success bool             `json:"success"`
		Data    map[int][]string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	assert.Equal(t, []string{"gpt-4o", modelrouting.Seedance20}, payload.Data[constant.ChannelTypeOpenAI])
	assert.Equal(t, []string{"claude-sonnet-4-5"}, payload.Data[constant.ChannelTypeAnthropic])
}

func TestChannelListModelsKeepsInternalCatalogForAdmins(t *testing.T) {
	original := openAIModels
	openAIModels = []dto.OpenAIModels{{Id: internalSeedanceModel, Object: "model"}}
	t.Cleanup(func() { openAIModels = original })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ChannelListModels(ctx)

	require.Contains(t, recorder.Body.String(), internalSeedanceModel)
}
