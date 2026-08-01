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
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestProjectPublicPricingRemovesInternalModelsAndVendors(t *testing.T) {
	pricing := []model.Pricing{
		{ModelName: "provider-hidden", VendorID: 99, SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAIResponse}},
		{ModelName: modelrouting.Seedance20Mini, VendorID: 99, SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAI}},
		{ModelName: modelrouting.Seedance20, VendorID: 99, SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAI}},
		{ModelName: modelrouting.Seedance20Fast, VendorID: 99, SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAI}},
	}
	endpoints := map[string]common.EndpointInfo{
		string(constant.EndpointTypeOpenAI):         {Path: "/v1/chat/completions", Method: "POST"},
		string(constant.EndpointTypeOpenAIResponse): {Path: "/v1/responses", Method: "POST"},
	}

	projection := projectPublicPricing(pricing, endpoints)
	require.Equal(t, []string{
		modelrouting.Seedance20, modelrouting.Seedance20Fast, modelrouting.Seedance20Mini,
	}, []string{
		projection.Pricing[0].ModelName,
		projection.Pricing[1].ModelName,
		projection.Pricing[2].ModelName,
	})
	for _, item := range projection.Pricing {
		assert.Equal(t, publicDoubaoVendor.ID, item.VendorID)
		assert.Equal(t, modelrouting.PublicModelOwner, item.OwnerBy)
		assert.Equal(t, publicDoubaoVendor.Icon, item.Icon)
	}
	require.Equal(t, []model.PricingVendor{publicDoubaoVendor}, projection.Vendors)
	require.Contains(t, projection.SupportedEndpoints, string(constant.EndpointTypeOpenAI))
	require.NotContains(t, projection.SupportedEndpoints, string(constant.EndpointTypeOpenAIResponse))
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

func TestListModelsReturnsOnlyPublicCatalogWithDoubaoOwner(t *testing.T) {
	withSelfUseModeEnabled(t)
	db := setupModelListControllerTestDB(t)
	modelNames := append([]string(nil), modelrouting.CanonicalModels...)
	modelNames = append(modelNames, "provider-hidden")
	seedPublicModelAbilities(t, db, modelNames)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	ListModels(ctx, constant.ChannelTypeOpenAI)

	payload := decodeListModelsPayload(t, recorder)
	require.Len(t, payload.Data, 3)
	for index, modelName := range modelrouting.CanonicalModels {
		assert.Equal(t, modelName, payload.Data[index].Id)
		assert.Equal(t, modelrouting.PublicModelOwner, payload.Data[index].OwnedBy)
	}
}

func TestListModelsCompatibilityFormatsHideInternalModels(t *testing.T) {
	for _, modelType := range []int{
		constant.ChannelTypeAnthropic,
		constant.ChannelTypeGemini,
	} {
		t.Run(constant.GetChannelTypeName(modelType), func(t *testing.T) {
			withSelfUseModeEnabled(t)
			db := setupModelListControllerTestDB(t)
			modelNames := append([]string(nil), modelrouting.CanonicalModels...)
			modelNames = append(modelNames, "provider-hidden")
			seedPublicModelAbilities(t, db, modelNames)

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
			ListModels(ctx, modelType)

			require.Equal(t, http.StatusOK, recorder.Code)
			assert.NotContains(t, recorder.Body.String(), "provider-hidden")
			for _, modelName := range modelrouting.CanonicalModels {
				assert.Contains(t, recorder.Body.String(), modelName)
			}
		})
	}
}

func TestListModelsTokenLimitCannotExposeInternalModel(t *testing.T) {
	withSelfUseModeEnabled(t)
	setupModelListControllerTestDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimit, map[string]bool{
		"provider-hidden": true,
	})

	ListModels(ctx, constant.ChannelTypeOpenAI)

	payload := decodeListModelsPayload(t, recorder)
	require.Empty(t, payload.Data)
}

func TestListModelsEmptyAnthropicCatalogDoesNotPanic(t *testing.T) {
	withSelfUseModeEnabled(t)
	setupModelListControllerTestDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimit, map[string]bool{
		"provider-hidden": true,
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

func TestRetrieveModelRejectsInternalModelAndReturnsPublicModel(t *testing.T) {
	hiddenRecorder := httptest.NewRecorder()
	hiddenContext, _ := gin.CreateTestContext(hiddenRecorder)
	hiddenContext.Params = gin.Params{{Key: "model", Value: "provider-hidden"}}

	RetrieveModel(hiddenContext, constant.ChannelTypeOpenAI)

	require.Equal(t, http.StatusOK, hiddenRecorder.Code)
	var hiddenPayload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(hiddenRecorder.Body.Bytes(), &hiddenPayload))
	assert.Equal(t, "model_not_found", hiddenPayload.Error.Code)

	publicRecorder := httptest.NewRecorder()
	publicContext, _ := gin.CreateTestContext(publicRecorder)
	publicContext.Params = gin.Params{{Key: "model", Value: modelrouting.Seedance20}}

	RetrieveModel(publicContext, constant.ChannelTypeOpenAI)

	require.Equal(t, http.StatusOK, publicRecorder.Code)
	var publicModel dto.OpenAIModels
	require.NoError(t, common.Unmarshal(publicRecorder.Body.Bytes(), &publicModel))
	assert.Equal(t, modelrouting.Seedance20, publicModel.Id)
	assert.Equal(t, modelrouting.PublicModelOwner, publicModel.OwnedBy)
}

func TestGetUserModelsReturnsOnlyPublicCatalog(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	modelNames := append([]string(nil), modelrouting.CanonicalModels...)
	modelNames = append(modelNames, "provider-hidden")
	seedPublicModelAbilities(t, db, modelNames)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/models?group=default", nil)
	ctx.Set("id", 19001)

	GetUserModels(ctx)

	require.Equal(t, modelrouting.CanonicalModels, decodeUserModelsResponse(t, recorder))
}

func TestDashboardListModelsReturnsOnlyPublicModelsPerChannel(t *testing.T) {
	original := channelId2Models
	channelId2Models = map[int][]string{
		constant.ChannelTypeOpenAI: {
			"provider-hidden", modelrouting.Seedance20Mini,
			modelrouting.Seedance20, modelrouting.Seedance20Fast,
		},
		constant.ChannelTypeAnthropic: {"provider-only"},
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
	assert.Equal(t, modelrouting.CanonicalModels, payload.Data[constant.ChannelTypeOpenAI])
	assert.Empty(t, payload.Data[constant.ChannelTypeAnthropic])
}

func TestChannelListModelsKeepsInternalCatalogForAdmins(t *testing.T) {
	original := openAIModels
	openAIModels = []dto.OpenAIModels{{Id: "provider-hidden", Object: "model"}}
	t.Cleanup(func() { openAIModels = original })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ChannelListModels(ctx)

	require.Contains(t, recorder.Body.String(), "provider-hidden")
}
