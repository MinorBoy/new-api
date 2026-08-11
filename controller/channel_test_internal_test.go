package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateChannelProxy(t *testing.T) {
	tests := []struct {
		name    string
		proxy   string
		wantErr bool
	}{
		{name: "empty"},
		{name: "http", proxy: "http://proxy.example:8080"},
		{name: "https", proxy: "https://proxy.example:8443"},
		{name: "socks5", proxy: "socks5://proxy.example"},
		{name: "socks5h", proxy: "socks5h://proxy.example:1080/"},
		{name: "unsupported", proxy: "ftp://proxy.example", wantErr: true},
		{name: "path", proxy: "socks5://proxy.example:1080/path", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setting, err := common.Marshal(dto.ChannelSettings{Proxy: test.proxy})
			require.NoError(t, err)
			channel := &model.Channel{
				Type:    constant.ChannelTypeOpenAI,
				Setting: common.GetPointer(string(setting)),
			}

			err = validateChannel(channel, false)

			if test.wantErr {
				require.ErrorContains(t, err, "invalid channel proxy")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateChannelRequiresNewAPIBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL *string
		wantErr bool
	}{
		{name: "missing", wantErr: true},
		{name: "blank", baseURL: common.GetPointer("  "), wantErr: true},
		{name: "configured", baseURL: common.GetPointer("https://new-api.example")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := &model.Channel{
				Type:    constant.ChannelTypeNewAPI,
				BaseURL: test.baseURL,
			}

			err := validateChannel(channel, false)

			if test.wantErr {
				require.ErrorContains(t, err, "New API channel base URL cannot be empty")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestNewAPIChannelRegistration(t *testing.T) {
	apiType, ok := common.ChannelType2APIType(constant.ChannelTypeNewAPI)

	require.True(t, ok)
	assert.Equal(t, constant.APITypeNewAPI, apiType)
	assert.Equal(t, "New API", constant.GetChannelTypeName(constant.ChannelTypeNewAPI))
	require.Greater(t, len(constant.ChannelBaseURLs), constant.ChannelTypeNewAPI)
	assert.Empty(t, constant.ChannelBaseURLs[constant.ChannelTypeNewAPI])
}

func TestResponsesCompactAPITypeSupport(t *testing.T) {
	tests := []struct {
		name    string
		apiType int
		want    bool
	}{
		{name: "OpenAI", apiType: constant.APITypeOpenAI, want: true},
		{name: "Codex", apiType: constant.APITypeCodex, want: true},
		{name: "Advanced Custom", apiType: constant.APITypeAdvancedCustom, want: true},
		{name: "Sub2API", apiType: constant.APITypeSub2API, want: true},
		{name: "New API", apiType: constant.APITypeNewAPI, want: true},
		{name: "Anthropic", apiType: constant.APITypeAnthropic, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, common.IsResponsesCompactAPIType(test.apiType))
		})
	}
}

func TestMultiprotocolGatewayEndpointTypes(t *testing.T) {
	want := []constant.EndpointType{
		constant.EndpointTypeOpenAI,
		constant.EndpointTypeOpenAIResponse,
		constant.EndpointTypeOpenAIResponseCompact,
		constant.EndpointTypeAnthropic,
		constant.EndpointTypeGemini,
		constant.EndpointTypeOpenAIAlphaSearch,
	}

	assert.Equal(t, want, common.GetEndpointTypesByChannelType(constant.ChannelTypeNewAPI, "gpt-5"))
	assert.Equal(t, want, common.GetEndpointTypesByChannelType(constant.ChannelTypeSub2API, "gpt-5"))
}

func TestCopyChannelRejectsInvalidLegacyProxySettings(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	settingBytes, err := common.Marshal(dto.ChannelSettings{
		Proxy: "socks5://proxy.example/legacy-path",
	})
	require.NoError(t, err)
	setting := string(settingBytes)
	origin := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		Name:    "legacy proxy channel",
		Key:     "test-key",
		Models:  "gpt-test",
		Group:   "default",
		Setting: &setting,
	}
	require.NoError(t, db.Create(origin).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", origin.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/copy", nil)

	CopyChannel(ctx)

	assert.Contains(t, recorder.Body.String(), "invalid channel settings")
	var channelCount int64
	require.NoError(t, db.Model(&model.Channel{}).Count(&channelCount).Error)
	assert.Equal(t, int64(1), channelCount)
}

func TestDeleteChannelResetsProxyCacheWhenPreReadFails(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.RoutingPolicy{}, &model.RouteTarget{}))
	service.ResetProxyClientCache()
	t.Cleanup(service.ResetProxyClientCache)

	proxyURL := "http://proxy.example:8080"
	beforeDelete, err := service.GetHttpClientWithProxy(proxyURL)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "999999"}}
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/channel/999999", nil)

	DeleteChannel(ctx)

	assert.Contains(t, recorder.Body.String(), `"success":true`)
	afterDelete, err := service.GetHttpClientWithProxy(proxyURL)
	require.NoError(t, err)
	assert.NotSame(t, beforeDelete, afterDelete)
}

func TestDeleteChannelBatchReportsAndAuditsActualDeletedCount(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.RoutingPolicy{}, &model.RouteTarget{}))
	channel := &model.Channel{Name: "existing", Key: "test-key"}
	require.NoError(t, db.Create(channel).Error)

	requestBody, err := common.Marshal(ChannelBatch{Ids: []int{channel.Id, 999999}})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/channel/batch", bytes.NewReader(requestBody))
	ctx.Request.Header.Set("Content-Type", "application/json")

	DeleteChannelBatch(ctx)

	var response struct {
		Success bool  `json:"success"`
		Data    int64 `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, int64(1), response.Data)

	var auditLog model.Log
	require.NoError(t, db.Order("id desc").First(&auditLog).Error)
	var auditData struct {
		Operation struct {
			Params map[string]any `json:"params"`
		} `json:"op"`
	}
	require.NoError(t, common.UnmarshalJsonStr(auditLog.Other, &auditData))
	assert.Equal(t, float64(1), auditData.Operation.Params["count"])
}

func TestSettleTestQuotaUsesTieredBilling(t *testing.T) {
	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:   "tiered_expr",
			ExprString:    `param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`,
			ExprHash:      billingexpr.ExprHashString(`param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`),
			GroupRatio:    1,
			EstimatedTier: "stream",
			QuotaPerUnit:  common.QuotaPerUnit,
			ExprVersion:   1,
		},
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{"stream":true}`),
		},
	}

	quota, result := settleTestQuota(info, types.PriceData{
		ModelRatio:      1,
		CompletionRatio: 2,
	}, &dto.Usage{
		PromptTokens: 1000,
	})

	require.Equal(t, 1500, quota)
	require.NotNil(t, result)
	require.Equal(t, "stream", result.MatchedTier)
}

func TestBuildTestLogOtherInjectsTieredInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr",
			ExprString:  `tier("base", p * 2)`,
		},
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	priceData := types.PriceData{
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
	}
	usage := &dto.Usage{
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 12,
		},
	}

	other := buildTestLogOther(ctx, info, priceData, usage, &billingexpr.TieredResult{
		MatchedTier: "base",
	})

	require.Equal(t, "tiered_expr", other["billing_mode"])
	require.Equal(t, "base", other["matched_tier"])
	require.NotEmpty(t, other["expr_b64"])
}

func TestResolveChannelTestUserIDUsesRequestUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("id", 2)

	userID, err := resolveChannelTestUserID(ctx)

	require.NoError(t, err)
	require.Equal(t, 2, userID)
}

func TestSupportsGenericChannelTestRejectsDimensio(t *testing.T) {
	require.False(t, supportsGenericChannelTest(constant.ChannelTypeDimensio))
	require.False(t, supportsGenericChannelTest(constant.ChannelTypeNewAPIVideo))
	require.False(t, supportsGenericChannelTest(constant.ChannelTypeClmmMall))
	require.False(t, supportsGenericChannelTest(constant.ChannelTypeLucen))
	require.False(t, supportsGenericChannelTest(constant.ChannelTypeMegaByAI))
	require.False(t, supportsGenericChannelTest(constant.ChannelTypeCangyuan))
	require.False(t, supportsGenericChannelTest(constant.ChannelTypePaipu))
	require.False(t, supportsGenericChannelTest(constant.ChannelTypeSecure))
	require.False(t, supportsGenericChannelTest(constant.ChannelTypeOmegaAI))
	require.False(t, supportsGenericChannelTest(constant.ChannelTypeFourSToken))
	require.False(t, supportsGenericChannelTest(constant.ChannelTypeEightYes))
	require.False(t, supportsGenericChannelTest(constant.ChannelTypeZ5API))
	require.False(t, supportsGenericChannelTest(constant.ChannelTypeMikoto))
	require.True(t, supportsGenericChannelTest(constant.ChannelTypeOpenAI))
}

func TestValidateChannelNormalizesPaipuBaseURL(t *testing.T) {
	baseURL := " https://override.paipu.example/// "
	channel := &model.Channel{Type: constant.ChannelTypePaipu, BaseURL: &baseURL}
	require.NoError(t, validateChannel(channel, false))
	require.NotNil(t, channel.BaseURL)
	require.Equal(t, "https://override.paipu.example", *channel.BaseURL)
}

func TestValidateChannelSecureVideoGroup(t *testing.T) {
	for _, group := range []dto.SecureVideoGroup{
		dto.SecureVideoGroupDiscount,
		dto.SecureVideoGroupOverseas,
		dto.SecureVideoGroupEnterprise,
	} {
		t.Run(string(group), func(t *testing.T) {
			channel := &model.Channel{Type: constant.ChannelTypeSecure}
			channel.SetOtherSettings(dto.ChannelOtherSettings{SecureVideoGroup: group})
			require.NoError(t, validateChannel(channel, false))
		})
	}

	for _, test := range []struct {
		name  string
		group dto.SecureVideoGroup
		want  string
	}{
		{name: "missing", want: "secure_video_group is required"},
		{name: "unknown", group: "other", want: "secure_video_group must be one of"},
	} {
		t.Run(test.name, func(t *testing.T) {
			channel := &model.Channel{Type: constant.ChannelTypeSecure}
			channel.SetOtherSettings(dto.ChannelOtherSettings{SecureVideoGroup: test.group})
			require.ErrorContains(t, validateChannel(channel, false), test.want)
		})
	}

	t.Run("non secure rejects leaked group", func(t *testing.T) {
		channel := &model.Channel{Type: constant.ChannelTypeOpenAI}
		channel.SetOtherSettings(dto.ChannelOtherSettings{SecureVideoGroup: dto.SecureVideoGroupDiscount})
		require.ErrorContains(t, validateChannel(channel, false), "secure_video_group is only valid for Secure")
	})

	t.Run("secure rejects multi key channel", func(t *testing.T) {
		channel := &model.Channel{
			Type: constant.ChannelTypeSecure,
			ChannelInfo: model.ChannelInfo{
				IsMultiKey: true,
			},
		}
		channel.SetOtherSettings(dto.ChannelOtherSettings{SecureVideoGroup: dto.SecureVideoGroupDiscount})
		require.ErrorContains(t, validateChannel(channel, false), "Secure channels do not support multi-key mode")
	})
}

func TestAddChannelRejectsSecureMultiToSingle(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	channel := &model.Channel{
		Type:   constant.ChannelTypeSecure,
		Name:   "Secure discount",
		Key:    "key-one\nkey-two",
		Models: "video-2.0-pro",
		Group:  "default",
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{SecureVideoGroup: dto.SecureVideoGroupDiscount})
	requestBody, err := common.Marshal(AddChannelRequest{
		Mode:         "multi_to_single",
		MultiKeyMode: constant.MultiKeyModeRandom,
		Channel:      channel,
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/", bytes.NewReader(requestBody))
	ctx.Request.Header.Set("Content-Type", "application/json")

	AddChannel(ctx)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "Secure channels do not support multi_to_single mode")
	var count int64
	require.NoError(t, db.Model(&model.Channel{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestAddChannelReturnsCreatedIDs(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	channel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Name:   "Batch supplier",
		Key:    "key-one\nkey-two",
		Models: "gpt-test",
		Group:  "default",
	}
	requestBody, err := common.Marshal(AddChannelRequest{Mode: "batch", Channel: channel})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/", bytes.NewReader(requestBody))
	ctx.Request.Header.Set("Content-Type", "application/json")

	AddChannel(ctx)

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			ChannelIDs []int `json:"channel_ids"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Len(t, response.Data.ChannelIDs, 2)
	for _, channelID := range response.Data.ChannelIDs {
		assert.Positive(t, channelID)
	}
	var count int64
	require.NoError(t, db.Model(&model.Channel{}).Count(&count).Error)
	assert.Equal(t, int64(2), count)
}

func TestAddChannelDisablesUnacceptedVideoChannels(t *testing.T) {
	for _, channelType := range []int{constant.ChannelTypeOmegaAI, constant.ChannelTypeFourSToken, constant.ChannelTypeEightYes, constant.ChannelTypeMikoto} {
		t.Run(constant.GetChannelTypeName(channelType), func(t *testing.T) {
			db := setupModelListControllerTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.Log{}))
			channel := &model.Channel{
				Type: channelType, Name: "unaccepted video", Key: "secret",
				Models: "client-video", Group: "default", Status: common.ChannelStatusEnabled,
			}
			requestBody, err := common.Marshal(AddChannelRequest{Mode: "single", Channel: channel})
			require.NoError(t, err)

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/", bytes.NewReader(requestBody))
			ctx.Request.Header.Set("Content-Type", "application/json")
			AddChannel(ctx)

			var response struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.Success, recorder.Body.String())
			var persisted model.Channel
			require.NoError(t, db.First(&persisted).Error)
			assert.Equal(t, common.ChannelStatusManuallyDisabled, persisted.Status)
		})
	}
}

func TestUpdateChannelDisablesTransitionToUnacceptedVideoChannel(t *testing.T) {
	for _, channelType := range []int{constant.ChannelTypeOmegaAI, constant.ChannelTypeFourSToken, constant.ChannelTypeEightYes, constant.ChannelTypeMikoto} {
		t.Run(constant.GetChannelTypeName(channelType), func(t *testing.T) {
			db := setupModelListControllerTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.Log{}, &model.RoutingPolicy{}, &model.RouteTarget{}))
			channel := &model.Channel{
				Type: constant.ChannelTypeOpenAI, Name: "enabled channel", Key: "secret",
				Models: "client-video", Group: "default", Status: common.ChannelStatusEnabled,
			}
			require.NoError(t, db.Create(channel).Error)
			requestBody, err := common.Marshal(map[string]any{"id": channel.Id, "type": channelType})
			require.NoError(t, err)

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Set("role", common.RoleRootUser)
			ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/", bytes.NewReader(requestBody))
			ctx.Request.Header.Set("Content-Type", "application/json")
			UpdateChannel(ctx)

			var response struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.Success, recorder.Body.String())
			var persisted model.Channel
			require.NoError(t, db.First(&persisted, channel.Id).Error)
			assert.Equal(t, channelType, persisted.Type)
			assert.Equal(t, common.ChannelStatusManuallyDisabled, persisted.Status)
		})
	}
}

func TestUpdateChannelValidatesMergedSecureConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		request     func(channel *model.Channel) map[string]any
		wantSuccess bool
		wantMessage string
		wantType    int
		wantGroup   dto.SecureVideoGroup
	}{
		{
			name: "partial update retains existing secure configuration",
			request: func(channel *model.Channel) map[string]any {
				return map[string]any{"id": channel.Id, "name": "renamed"}
			},
			wantSuccess: true,
			wantType:    constant.ChannelTypeSecure,
			wantGroup:   dto.SecureVideoGroupDiscount,
		},
		{
			name: "secure group is immutable",
			request: func(channel *model.Channel) map[string]any {
				return map[string]any{
					"id":       channel.Id,
					"type":     constant.ChannelTypeSecure,
					"settings": `{"secure_video_group":"enterprise"}`,
				}
			},
			wantMessage: "secure_video_group cannot be changed",
			wantType:    constant.ChannelTypeSecure,
			wantGroup:   dto.SecureVideoGroupDiscount,
		},
		{
			name: "changing secure channel type cannot bypass group immutability",
			request: func(channel *model.Channel) map[string]any {
				return map[string]any{"id": channel.Id, "type": constant.ChannelTypeOpenAI, "settings": ""}
			},
			wantMessage: "secure_video_group cannot be changed",
			wantType:    constant.ChannelTypeSecure,
			wantGroup:   dto.SecureVideoGroupDiscount,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := setupModelListControllerTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.Log{}, &model.RoutingPolicy{}, &model.RouteTarget{}))
			channel := &model.Channel{
				Type:   constant.ChannelTypeSecure,
				Name:   "Secure discount",
				Key:    "secret",
				Models: "video-2.0-pro",
				Group:  "default",
				Status: common.ChannelStatusEnabled,
			}
			channel.SetOtherSettings(dto.ChannelOtherSettings{SecureVideoGroup: dto.SecureVideoGroupDiscount})
			require.NoError(t, db.Create(channel).Error)
			requestBody, err := common.Marshal(test.request(channel))
			require.NoError(t, err)

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Set("role", common.RoleRootUser)
			ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/", bytes.NewReader(requestBody))
			ctx.Request.Header.Set("Content-Type", "application/json")
			UpdateChannel(ctx)

			var response struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, test.wantSuccess, response.Success)
			if test.wantMessage != "" {
				assert.Contains(t, response.Message, test.wantMessage)
			}

			var persisted model.Channel
			require.NoError(t, db.First(&persisted, channel.Id).Error)
			assert.Equal(t, test.wantType, persisted.Type)
			assert.Equal(t, test.wantGroup, persisted.GetOtherSettings().SecureVideoGroup)
		})
	}
}

func TestUpdateChannelClearsExplicitEmptySettings(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.RoutingPolicy{}, &model.RouteTarget{}))
	channel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Name:   "legacy contaminated channel",
		Key:    "secret",
		Models: "gpt-4o",
		Group:  "default",
		Status: common.ChannelStatusEnabled,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{SecureVideoGroup: dto.SecureVideoGroupDiscount})
	require.NoError(t, db.Create(channel).Error)
	requestBody, err := common.Marshal(map[string]any{
		"id":       channel.Id,
		"type":     constant.ChannelTypeOpenAI,
		"settings": "",
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("role", common.RoleRootUser)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/", bytes.NewReader(requestBody))
	ctx.Request.Header.Set("Content-Type", "application/json")
	UpdateChannel(ctx)

	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	var persisted model.Channel
	require.NoError(t, db.First(&persisted, channel.Id).Error)
	assert.Equal(t, "{}", persisted.OtherSettings)
	assert.Empty(t, persisted.GetOtherSettings().SecureVideoGroup)
}

func TestUpdateChannelRejectsPartialUpdateWhenExistingConfigurationIsInvalid(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.RoutingPolicy{}, &model.RouteTarget{}))
	channel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Name:   "legacy contaminated channel",
		Key:    "secret",
		Models: "gpt-4o",
		Group:  "default",
		Status: common.ChannelStatusEnabled,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{SecureVideoGroup: dto.SecureVideoGroupDiscount})
	require.NoError(t, db.Create(channel).Error)
	requestBody, err := common.Marshal(map[string]any{
		"id":   channel.Id,
		"name": "rename must not bypass validation",
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("role", common.RoleRootUser)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/", bytes.NewReader(requestBody))
	ctx.Request.Header.Set("Content-Type", "application/json")
	UpdateChannel(ctx)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "secure_video_group is only valid for Secure")
	var persisted model.Channel
	require.NoError(t, db.First(&persisted, channel.Id).Error)
	assert.Equal(t, "legacy contaminated channel", persisted.Name)
}

func TestSelectChannelsForAutomaticTestPassiveRecoveryOnlyUsesAutoDisabled(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModePassiveRecovery)

	require.Len(t, selected, 1)
	require.Equal(t, 2, selected[0].Id)
}

func TestSelectChannelsForAutomaticTestScheduledSkipsManualDisabled(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModeScheduledAll)

	require.Len(t, selected, 2)
	require.Equal(t, 1, selected[0].Id)
	require.Equal(t, 2, selected[1].Id)
}

func TestTestAllChannelsRejectsExistingActiveTask(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))

	existing, err := model.CreateSystemTask(model.SystemTaskTypeChannelTest, nil, nil)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/test", nil)

	TestAllChannels(ctx)

	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Contains(t, recorder.Body.String(), existing.TaskID)
	require.Contains(t, recorder.Body.String(), "已有通道测试任务正在运行或等待中")
}
