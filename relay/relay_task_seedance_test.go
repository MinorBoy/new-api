package relay

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type seedanceTaskTestBilling struct{}

func (seedanceTaskTestBilling) Settle(int) error         { return nil }
func (seedanceTaskTestBilling) Refund(*gin.Context)      {}
func (seedanceTaskTestBilling) NeedsRefund() bool        { return false }
func (seedanceTaskTestBilling) GetPreConsumedQuota() int { return 0 }
func (seedanceTaskTestBilling) Reserve(int) error        { return nil }

func configureSeedanceDurationPricing(t *testing.T, prices map[string]types.DurationPrice) {
	t.Helper()
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	modes := make(map[string]string, len(prices))
	for modelName := range prices {
		modes[modelName] = billing_setting.BillingModePerDuration
	}
	modeJSON, err := common.Marshal(modes)
	require.NoError(t, err)
	priceJSON, err := common.Marshal(prices)
	require.NoError(t, err)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":    string(modeJSON),
		"billing_setting.duration_price":  string(priceJSON),
		"group_ratio_setting.group_ratio": `{"default":1}`,
	}))
}

func TestSeedanceDurationPricingUsesBasePriceFor720pText(t *testing.T) {
	price := types.DurationPrice{Price: 1, Unit: types.DurationUnitSecond, RoundingStepSeconds: 1}
	priceData := types.PriceData{
		BillingMode:    billing_setting.BillingModePerDuration,
		DurationPrice:  &price,
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
	}

	require.NoError(t, applySeedanceDurationPricing(&priceData, modelrouting.Seedance20Mini, "720p", false, 0, 5))
	assert.InDelta(t, 1, priceData.OtherRatios()["seedance_price_matrix"], 1e-9)
	quota, _, _, err := taskDurationQuota(priceData, 5)
	require.NoError(t, err)
	assert.Equal(t, 2_500_000, quota)
}

func TestSeedanceDurationPricingReplacesLegacyAdapterRatiosForReferenceVideo(t *testing.T) {
	price := types.DurationPrice{Price: 1, Unit: types.DurationUnitSecond, RoundingStepSeconds: 1}
	priceData := types.PriceData{
		BillingMode:    billing_setting.BillingModePerDuration,
		DurationPrice:  &price,
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
	}
	priceData.AddOtherRatio("resolution", 2.5)
	priceData.AddOtherRatio("video_input", 14.0/23.0)

	require.NoError(t, applySeedanceDurationPricing(&priceData, modelrouting.Seedance20Mini, "720p", true, 5000, 5))
	ratios := priceData.OtherRatios()
	assert.NotContains(t, ratios, "resolution")
	assert.NotContains(t, ratios, "video_input")
	assert.InDelta(t, 28.0/23.0, ratios["seedance_price_matrix"], 1e-9)
	quota, _, _, err := taskDurationQuota(priceData, 5)
	require.NoError(t, err)
	assert.Equal(t, 3_043_478, quota)
}

func TestDimensioDurationBillingUsesOriginModelPrice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()

	const originModel = "client-seedance-vip"
	const upstreamModel = "jimeng-video-seedance-2.0-vip"
	configureSeedanceDurationPricing(t, map[string]types.DurationPrice{
		originModel: {
			Price: 0.1, Unit: types.DurationUnitSecond,
			RoundingStepSeconds: 1, MinimumDurationSeconds: 4,
		},
		upstreamModel: {
			Price: 9, Unit: types.DurationUnitSecond,
			RoundingStepSeconds: 1, MinimumDurationSeconds: 4,
		},
	})

	var upstreamCalls atomic.Int32
	capturedBodyCh := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		body, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		capturedBodyCh <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1,"task_id":"dim-upstream","status":"pending"}`))
	}))
	t.Cleanup(server.Close)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", bytes.NewBufferString(`{
		"model":"client-seedance-vip",
		"content":[{"type":"text","text":"generate a video"}],
		"duration":6,
		"resolution":"720p"
	}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.KeySeedanceOfficialAPI, true)
	c.Set(string(constant.ContextKeyChannelType), constant.ChannelTypeDimensio)
	c.Set(string(constant.ContextKeyChannelBaseUrl), server.URL)
	c.Set(string(constant.ContextKeyChannelKey), "mock-key")
	c.Set("model_mapping", `{"client-seedance-vip":"jimeng-video-seedance-2.0-vip"}`)

	info := &relaycommon.RelayInfo{
		OriginModelName: originModel,
		UserGroup:       "default",
		UsingGroup:      "default",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		Billing:         seedanceTaskTestBilling{},
	}

	result, taskErr := RelayTaskSubmit(c, info, nil)

	require.Nil(t, taskErr)
	require.NotNil(t, result)
	assert.Equal(t, int32(1), upstreamCalls.Load())
	assert.Equal(t, 300_000, result.Quota)
	assert.Equal(t, originModel, info.OriginModelName)
	assert.Equal(t, upstreamModel, info.UpstreamModelName)
	assert.Equal(t, billing_setting.BillingModePerDuration, info.PriceData.BillingMode)
	assert.Equal(t, 6, info.PriceData.RequestedDurationSeconds)
	assert.Equal(t, 6, info.PriceData.BillableDurationSeconds)
	assert.NotContains(t, info.PriceData.OtherRatios(), "seconds")
	var upstreamRequest map[string]interface{}
	capturedBody := <-capturedBodyCh
	require.NoError(t, common.Unmarshal(capturedBody, &upstreamRequest))
	assert.Equal(t, upstreamModel, upstreamRequest["model"])
}

func TestTaskModel2DtoHidesCapabilityRouteFromUsers(t *testing.T) {
	task := &model.Task{
		TaskID:     "task-routing-private",
		Properties: model.Properties{OriginModelName: modelrouting.Seedance20},
		PrivateData: model.TaskPrivateData{Routing: &modelrouting.Audit{
			PolicyID: 7, TargetID: 21, UpstreamModel: "provider-1080p",
		}},
		Data: json.RawMessage(`{"model":"provider-1080p"}`),
	}

	userDTO := TaskModel2Dto(task, false)
	assert.Equal(t, modelrouting.Seedance20, userDTO.Properties.(model.Properties).OriginModelName)
	assert.Empty(t, userDTO.Properties.(model.Properties).UpstreamModelName)
	assert.Nil(t, userDTO.Data)

	adminDTO := TaskModel2Dto(task, true)
	assert.Equal(t, task.Data, adminDTO.Data)
}

func TestTaskModel2DtoExposesAuditPayloadsToAdminsOnly(t *testing.T) {
	task := &model.Task{
		Platform: constant.TaskPlatform("task-test"),
		PrivateData: model.TaskPrivateData{
			UserRequestData:      json.RawMessage(`{"model":"client-model","prompt":"user request"}`),
			UpstreamResponseData: json.RawMessage(`{"id":"upstream-task","status":"queued"}`),
			UserResponseData:     json.RawMessage(`{"id":"task-public","status":"queued"}`),
		},
	}

	adminPayload, err := common.Marshal(TaskModel2Dto(task, true))
	require.NoError(t, err)
	var adminResponse map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(adminPayload, &adminResponse))
	assert.JSONEq(t, `{"model":"client-model","prompt":"user request"}`, string(adminResponse["user_request_data"]))
	assert.JSONEq(t, `{"id":"upstream-task","status":"queued"}`, string(adminResponse["upstream_response_data"]))
	assert.JSONEq(t, `{"id":"task-public","status":"queued"}`, string(adminResponse["user_response_data"]))

	userPayload, err := common.Marshal(TaskModel2Dto(task, false))
	require.NoError(t, err)
	var userResponse map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(userPayload, &userResponse))
	assert.JSONEq(t, `{"model":"client-model","prompt":"user request"}`, string(userResponse["user_request_data"]))
	assert.NotContains(t, userResponse, "upstream_response_data")
	assert.NotContains(t, userResponse, "user_response_data")
}

func TestTaskModel2DtoIncludesInboundRequestPath(t *testing.T) {
	task := &model.Task{
		Properties: model.Properties{RequestPath: "/v1/video/generations"},
	}

	taskDTO := TaskModel2Dto(task, true)

	assert.Equal(t, "/v1/video/generations", taskDTO.RequestPath)
}

func TestTaskModel2DtoHidesMegaByAIDataWithoutRoutingFromUsers(t *testing.T) {
	task := &model.Task{
		TaskID:   "task-megabyai-private",
		Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeMegaByAI)),
		Properties: model.Properties{
			OriginModelName:   "client-video",
			UpstreamModelName: "videos-mini",
		},
		Data: json.RawMessage(`{"task_id":"upstream-private-id","model":"videos-mini"}`),
	}

	userDTO := TaskModel2Dto(task, false)
	assert.Equal(t, model.Properties{OriginModelName: "client-video"}, userDTO.Properties)
	assert.Nil(t, userDTO.Data)

	adminDTO := TaskModel2Dto(task, true)
	assert.Equal(t, task.Properties, adminDTO.Properties)
	assert.Equal(t, task.Data, adminDTO.Data)
}

func TestDimensioDurationBillingSaturationStopsBeforeUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()

	const originModel = "client-seedance-overflow"
	configureSeedanceDurationPricing(t, map[string]types.DurationPrice{
		originModel: {
			Price: math.MaxFloat64, Unit: types.DurationUnitSecond,
			RoundingStepSeconds: 1, MinimumDurationSeconds: 4,
		},
	})

	var upstreamCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", bytes.NewBufferString(`{
		"model":"client-seedance-overflow",
		"content":[{"type":"text","text":"generate a video"}],
		"duration":6,
		"resolution":"720p"
	}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.KeySeedanceOfficialAPI, true)
	c.Set(string(constant.ContextKeyChannelType), constant.ChannelTypeDimensio)
	c.Set(string(constant.ContextKeyChannelBaseUrl), server.URL)
	c.Set(string(constant.ContextKeyChannelKey), "mock-key")
	c.Set("model_mapping", `{"client-seedance-overflow":"jimeng-video-seedance-2.0-vip"}`)

	info := &relaycommon.RelayInfo{
		OriginModelName: originModel,
		UserGroup:       "default",
		UsingGroup:      "default",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}

	result, taskErr := RelayTaskSubmit(c, info, nil)

	assert.Nil(t, result)
	require.NotNil(t, taskErr)
	assert.Equal(t, int32(0), upstreamCalls.Load())
	assert.Equal(t, common.MaxQuota, info.PriceData.Quota)
	assert.GreaterOrEqual(t, info.PriceData.Quota, 0)
	require.NotNil(t, info.QuotaClamp)
	assert.Equal(t, common.QuotaClampOverflow, info.QuotaClamp.Kind)
}

func setupSeedanceTaskDB(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		model.DB = originalDB
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, database.AutoMigrate(&model.Task{}))
	model.DB = database
}

func TestClmmMallSeedanceTaskPayloadRequiresArkConverter(t *testing.T) {
	task := &model.Task{
		Platform:    constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeClmmMall)),
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "clmm-private-upstream"},
		Data:        json.RawMessage(`{"task_id":"clmm-private-upstream","diagnostic":"raw-private-data"}`),
	}

	adaptors := []struct {
		name    string
		adaptor channel.TaskAdaptor
	}{
		{name: "nil adaptor", adaptor: nil},
		{name: "adaptor without converter", adaptor: GetTaskAdaptor(constant.TaskPlatformSuno)},
	}
	require.NotNil(t, adaptors[1].adaptor)
	_, supportsArkConversion := adaptors[1].adaptor.(channel.ArkVideoTaskConverter)
	require.False(t, supportsArkConversion)
	for _, test := range adaptors {
		t.Run(test.name, func(t *testing.T) {
			response, err := seedanceTaskPayload(task, test.adaptor)

			require.EqualError(t, err, "CLMM Mall Ark task converter is unavailable")
			assert.Nil(t, response)
			assert.NotContains(t, err.Error(), "clmm-private-upstream")
			assert.NotContains(t, err.Error(), "raw-private-data")
		})
	}
}

func TestSeedanceTaskPayloadPreservesRawFallbackForOtherPlatforms(t *testing.T) {
	task := &model.Task{
		Platform: constant.TaskPlatform("999"),
		Data:     json.RawMessage(`{"task_id":"legacy-upstream","status":"queued"}`),
	}

	response, err := seedanceTaskPayload(task, nil)

	require.NoError(t, err)
	assert.Equal(t, "legacy-upstream", response["task_id"])
}

func TestSeedanceTaskFetchUsesPublicIDAndOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSeedanceTaskDB(t)
	now := time.Now().Unix()
	task := model.Task{
		TaskID:      "task_public",
		Platform:    constant.TaskPlatform("54"),
		UserId:      7,
		Status:      model.TaskStatusSuccess,
		SubmitTime:  now,
		UpdatedAt:   now + 10,
		Properties:  model.Properties{OriginModelName: "seedance-alias", UpstreamModelName: "doubao-seedance-2-0-260128"},
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-secret", ResultURL: "https://example.com/video.mp4"},
		Data:        json.RawMessage(`{"id":"cgt-secret","status":"succeeded","content":{}}`),
	}
	require.NoError(t, model.DB.Create(&task).Error)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_public", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "task_public"}}
	c.Set("id", 7)
	body, taskErr := SeedanceTaskFetch(c)
	require.Nil(t, taskErr)
	assert.NotContains(t, string(body), "cgt-secret")
	var response map[string]interface{}
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, "task_public", response["id"])
	assert.Equal(t, "succeeded", response["status"])
	assert.Equal(t, "seedance-alias", response["model"])
	content := response["content"].(map[string]interface{})
	assert.Equal(t, "https://example.com/video.mp4", content["video_url"])

	other, _ := gin.CreateTestContext(httptest.NewRecorder())
	other.Request = c.Request
	other.Params = c.Params
	other.Set("id", 8)
	_, taskErr = SeedanceTaskFetch(other)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusNotFound, taskErr.StatusCode)
}

func TestClmmMallSeedanceTaskFetchUsesArkConverterAndProtectsPrivateData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSeedanceTaskDB(t)
	tasks := []model.Task{
		{
			TaskID:     "task_clmm_success",
			Platform:   constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeClmmMall)),
			UserId:     7,
			Status:     model.TaskStatusSuccess,
			SubmitTime: 111,
			UpdatedAt:  222,
			Properties: model.Properties{OriginModelName: "client-video-model", UpstreamModelName: "me-videos-720P-10s"},
			PrivateData: model.TaskPrivateData{
				UpstreamTaskID: "clmm-upstream-success",
				ResultURL:      "https://example.com/private-fallback.mp4",
			},
			Data: json.RawMessage(`{"task_id":"clmm-upstream-success","status":"completed","video_url":"https://example.com/clmm-video.mp4","diagnostic":"raw-private-success"}`),
		},
		{
			TaskID:      "task_clmm_failed",
			Platform:    constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeClmmMall)),
			UserId:      7,
			Status:      model.TaskStatusFailure,
			SubmitTime:  333,
			UpdatedAt:   444,
			Properties:  model.Properties{OriginModelName: "client-video-model", UpstreamModelName: "me-videos-720P-10s"},
			PrivateData: model.TaskPrivateData{UpstreamTaskID: "clmm-upstream-failed"},
			Data:        json.RawMessage(`{"task_id":"clmm-upstream-failed","status":"failed","error":{"code":"provider_code","message":"provider detail"},"diagnostic":"raw-private-failure"}`),
		},
	}
	require.NoError(t, model.DB.Create(&tasks).Error)

	t.Run("successful task", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_clmm_success", nil)
		c.Params = gin.Params{{Key: "task_id", Value: "task_clmm_success"}}
		c.Set("id", 7)

		body, taskErr := SeedanceTaskFetch(c)

		require.Nil(t, taskErr)
		assert.NotContains(t, string(body), "clmm-upstream-success")
		assert.NotContains(t, string(body), "raw-private-success")
		var response map[string]interface{}
		require.NoError(t, common.Unmarshal(body, &response))
		assert.Equal(t, "task_clmm_success", response["id"])
		assert.Equal(t, "client-video-model", response["model"])
		assert.Equal(t, "succeeded", response["status"])
		content, ok := response["content"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "https://example.com/clmm-video.mp4", content["video_url"])
	})

	t.Run("failed task", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_clmm_failed", nil)
		c.Params = gin.Params{{Key: "task_id", Value: "task_clmm_failed"}}
		c.Set("id", 7)

		body, taskErr := SeedanceTaskFetch(c)

		require.Nil(t, taskErr)
		assert.NotContains(t, string(body), "clmm-upstream-failed")
		assert.NotContains(t, string(body), "raw-private-failure")
		var response struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Error  struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		require.NoError(t, common.Unmarshal(body, &response))
		assert.Equal(t, "task_clmm_failed", response.ID)
		assert.Equal(t, "failed", response.Status)
		assert.Equal(t, "task_failed", response.Error.Code)
		assert.Equal(t, "task failed", response.Error.Message)
	})

	t.Run("other user", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_clmm_success", nil)
		c.Params = gin.Params{{Key: "task_id", Value: "task_clmm_success"}}
		c.Set("id", 8)

		_, taskErr := SeedanceTaskFetch(c)

		require.NotNil(t, taskErr)
		assert.Equal(t, http.StatusNotFound, taskErr.StatusCode)
	})
}

func TestClmmMallVideoFetchByIDUsesArkConverterAndProtectsPrivateData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSeedanceTaskDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}))
	task := model.Task{
		TaskID:     "task_clmm_public",
		Platform:   constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeClmmMall)),
		UserId:     7,
		Status:     model.TaskStatusSuccess,
		SubmitTime: 111,
		UpdatedAt:  222,
		Properties: model.Properties{OriginModelName: "client-video-model", UpstreamModelName: "me-videos-720P-10s"},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-private-id",
		},
		Data: json.RawMessage(`{"task_id":"upstream-private-id","status":"completed","video_url":"https://example.com/video.mp4","diagnostic":"Authorization: Bearer fake-upstream-secret"}`),
	}
	require.NoError(t, model.DB.Create(&task).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_clmm_public", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "task_clmm_public"}}
	c.Set("id", 7)

	taskErr := RelayTaskFetch(c, relayconstant.RelayModeVideoFetchByID)

	require.Nil(t, taskErr)
	body := recorder.Body.Bytes()
	assert.NotContains(t, string(body), "upstream-private-id")
	assert.NotContains(t, string(body), "fake-upstream-secret")
	var response struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Status  string `json:"status"`
		Content struct {
			VideoURL string `json:"video_url"`
		} `json:"content"`
	}
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, "task_clmm_public", response.ID)
	assert.Equal(t, "client-video-model", response.Model)
	assert.Equal(t, "succeeded", response.Status)
	assert.Equal(t, "https://example.com/video.mp4", response.Content.VideoURL)
}

func TestClmmMallSeedanceTaskListFiltersOwnedTasksWithoutPrivateData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSeedanceTaskDB(t)
	now := time.Now().Unix()
	tasks := []model.Task{
		{
			TaskID:      "task_clmm_match",
			Platform:    constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeClmmMall)),
			UserId:      7,
			Status:      model.TaskStatusSuccess,
			SubmitTime:  now,
			Properties:  model.Properties{OriginModelName: "client-video-model", UpstreamModelName: "me-videos-720P-10s"},
			PrivateData: model.TaskPrivateData{UpstreamTaskID: "clmm-list-upstream"},
			Data:        json.RawMessage(`{"task_id":"clmm-list-upstream","status":"completed","video_url":"https://example.com/list.mp4","service_tier":"priority","diagnostic":"raw-list-private"}`),
		},
		{
			TaskID:     "task_clmm_other_user",
			Platform:   constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeClmmMall)),
			UserId:     8,
			Status:     model.TaskStatusSuccess,
			SubmitTime: now,
			Properties: model.Properties{OriginModelName: "client-video-model"},
			Data:       json.RawMessage(`{"task_id":"other-user-upstream","status":"completed","video_url":"https://example.com/other.mp4","service_tier":"priority"}`),
		},
		{
			TaskID:     "task_clmm_wrong_model",
			Platform:   constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeClmmMall)),
			UserId:     7,
			Status:     model.TaskStatusSuccess,
			SubmitTime: now,
			Properties: model.Properties{OriginModelName: "other-model"},
			Data:       json.RawMessage(`{"task_id":"wrong-model-upstream","status":"completed","video_url":"https://example.com/wrong-model.mp4","service_tier":"priority"}`),
		},
		{
			TaskID:     "task_clmm_wrong_status",
			Platform:   constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeClmmMall)),
			UserId:     7,
			Status:     model.TaskStatusFailure,
			SubmitTime: now,
			Properties: model.Properties{OriginModelName: "client-video-model"},
			Data:       json.RawMessage(`{"task_id":"wrong-status-upstream","status":"failed","service_tier":"priority"}`),
		},
		{
			TaskID:     "task_clmm_wrong_tier",
			Platform:   constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeClmmMall)),
			UserId:     7,
			Status:     model.TaskStatusSuccess,
			SubmitTime: now,
			Properties: model.Properties{OriginModelName: "client-video-model"},
			Data:       json.RawMessage(`{"task_id":"wrong-tier-upstream","status":"completed","video_url":"https://example.com/wrong-tier.mp4","service_tier":"default"}`),
		},
	}
	require.NoError(t, model.DB.Create(&tasks).Error)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks?status=succeeded&filter.model=client-video-model&filter.service_tier=priority", nil)
	c.Set("id", 7)

	body, taskErr := SeedanceTaskFetch(c)

	require.Nil(t, taskErr)
	assert.NotContains(t, string(body), "clmm-list-upstream")
	assert.NotContains(t, string(body), "raw-list-private")
	assert.NotContains(t, string(body), "other-user-upstream")
	var response seedanceTaskListResponse
	require.NoError(t, common.Unmarshal(body, &response))
	require.Equal(t, 1, response.Total)
	require.Len(t, response.Items, 1)
	assert.Equal(t, "task_clmm_match", response.Items[0]["id"])
	assert.Equal(t, "client-video-model", response.Items[0]["model"])
	assert.Equal(t, "succeeded", response.Items[0]["status"])
	content, ok := response.Items[0]["content"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "https://example.com/list.mp4", content["video_url"])
}

func TestDimensioTaskFetchTranslatesStoredResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSeedanceTaskDB(t)
	task := model.Task{
		TaskID: "task_public", Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDimensio)), UserId: 7,
		Status: model.TaskStatusSuccess, SubmitTime: 111, UpdatedAt: 222,
		Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "dim-upstream",
			BillingContext: &model.TaskBillingContext{
				RequestedDurationSeconds: 5,
				Resolution:               "720p",
				UpstreamCostMode:         string(types.CostModePerDuration),
			},
		},
		Data: json.RawMessage(`{"task_id":"dim-upstream","status":"completed","result":{"url":"https://x/video.mp4"}}`),
	}
	require.NoError(t, model.DB.Create(&task).Error)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_public", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "task_public"}}
	c.Set("id", 7)
	body, taskErr := SeedanceTaskFetch(c)
	require.Nil(t, taskErr)
	assert.NotContains(t, string(body), "dim-upstream")
	var response map[string]interface{}
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, "task_public", response["id"])
	assert.Equal(t, "succeeded", response["status"])
	assert.Equal(t, "https://x/video.mp4", response["content"].(map[string]interface{})["video_url"])
	usage, ok := response["usage"].(map[string]interface{})
	require.True(t, ok)
	assert.EqualValues(t, 108000, usage["completion_tokens"])
	assert.EqualValues(t, 108000, usage["total_tokens"])

	var stored model.Task
	require.NoError(t, model.DB.Where("task_id = ?", "task_public").First(&stored).Error)
	assert.JSONEq(t, string(body), string(stored.PrivateData.UserResponseData))
}

func TestDimensioTaskFetchPreservesProviderTimestampsAndErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSeedanceTaskDB(t)
	task := model.Task{
		TaskID: "task_failed", Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDimensio)), UserId: 7,
		Status: model.TaskStatusFailure, SubmitTime: 111, UpdatedAt: 222,
		Properties: model.Properties{OriginModelName: "alias"},
		Data:       json.RawMessage(`{"task_id":"dim-upstream","status":"failed","error":"审核不通过","error_code":"2043","created_at":333,"updated_at":444}`),
	}
	require.NoError(t, model.DB.Create(&task).Error)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_failed", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "task_failed"}}
	c.Set("id", 7)
	body, taskErr := SeedanceTaskFetch(c)
	require.Nil(t, taskErr)
	var response struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		CreatedAt int64  `json:"created_at"`
		UpdatedAt int64  `json:"updated_at"`
		Error     struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, "task_failed", response.ID)
	assert.Equal(t, "failed", response.Status)
	assert.Equal(t, int64(333), response.CreatedAt)
	assert.Equal(t, int64(444), response.UpdatedAt)
	assert.Equal(t, "2043", response.Error.Code)
	assert.Equal(t, "审核不通过", response.Error.Message)

	var stored model.Task
	require.NoError(t, model.DB.Where("task_id = ?", "task_failed").First(&stored).Error)
	assert.JSONEq(t, string(body), string(stored.PrivateData.UserResponseData))
}

func TestDimensioTaskAdaptorIsTaskOnly(t *testing.T) {
	require.NotNil(t, GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDimensio))))
	_, success := common.ChannelType2APIType(constant.ChannelTypeDimensio)
	require.False(t, success)
}

func TestNewAPIVideoTaskAdaptorIsTaskOnly(t *testing.T) {
	require.NotNil(t, GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo))))
	require.NotNil(t, GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeLucen))))
	_, success := common.ChannelType2APIType(constant.ChannelTypeNewAPIVideo)
	require.False(t, success)
	_, success = common.ChannelType2APIType(constant.ChannelTypeLucen)
	require.False(t, success)
}

func TestMegaByAITaskAdaptorIsRegisteredForArkTasks(t *testing.T) {
	adaptor := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeMegaByAI)))
	require.NotNil(t, adaptor)
	assert.Equal(t, "MegaByAI", adaptor.GetChannelName())
	_, supportsArkConversion := adaptor.(channel.ArkVideoTaskConverter)
	assert.True(t, supportsArkConversion)
	_, supportsCostAccounting := adaptor.(channel.TaskCostAccountingAdaptor)
	assert.True(t, supportsCostAccounting)
	assert.True(t, isSeedanceTaskPlatform(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeMegaByAI))))
	assert.Contains(t, seedanceTaskPlatformValues(), strconv.Itoa(constant.ChannelTypeMegaByAI))
}

func TestCangyuanTaskAdaptorIsRegisteredForArkTasks(t *testing.T) {
	adaptor := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeCangyuan)))
	require.NotNil(t, adaptor)
	assert.Equal(t, "Cangyuan", adaptor.GetChannelName())
	_, supportsArkConversion := adaptor.(channel.ArkVideoTaskConverter)
	assert.True(t, supportsArkConversion)
	_, supportsCostAccounting := adaptor.(channel.TaskCostAccountingAdaptor)
	assert.True(t, supportsCostAccounting)
	assert.True(t, isSeedanceTaskPlatform(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeCangyuan))))
	assert.Contains(t, seedanceTaskPlatformValues(), strconv.Itoa(constant.ChannelTypeCangyuan))
}

func TestPaipuTaskAdaptorIsRegisteredForArkTasks(t *testing.T) {
	adaptor := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypePaipu)))
	require.NotNil(t, adaptor)
	assert.Equal(t, "Paipu", adaptor.GetChannelName())
	_, supportsArkConversion := adaptor.(channel.ArkVideoTaskConverter)
	assert.True(t, supportsArkConversion)
	_, supportsCostAccounting := adaptor.(channel.TaskCostAccountingAdaptor)
	assert.True(t, supportsCostAccounting)
	assert.True(t, isSeedanceTaskPlatform(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypePaipu))))
	assert.Contains(t, seedanceTaskPlatformValues(), strconv.Itoa(constant.ChannelTypePaipu))
}

func TestSecureTaskAdaptorIsRegisteredForArkTasks(t *testing.T) {
	adaptor := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSecure)))
	require.NotNil(t, adaptor)
	assert.Equal(t, "Secure", adaptor.GetChannelName())
	_, supportsArkConversion := adaptor.(channel.ArkVideoTaskConverter)
	assert.True(t, supportsArkConversion)
	_, supportsCostAccounting := adaptor.(channel.TaskCostAccountingAdaptor)
	assert.True(t, supportsCostAccounting)
	assert.True(t, isSeedanceTaskPlatform(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSecure))))
	assert.Contains(t, seedanceTaskPlatformValues(), strconv.Itoa(constant.ChannelTypeSecure))
}

const newAPIVideoDetailedZeroUsage = `{
	"code":"success",
	"message":"",
	"data":{
		"task_id":"upstream-secret",
		"status":"SUCCESS",
		"result_url":"https://example.com/video.mp4",
		"submit_time":1784716214,
		"finish_time":1784716351,
		"progress":"100%",
		"user_id":59,
		"channel_id":14,
		"group":"secret-group",
		"platform":"54",
		"quota":2000000,
		"data":{
			"content":{"video_url":"https://example.com/video.mp4"},
			"created_at":1784716214,
			"updated_at":1784716351,
			"draft":false,
			"duration":5,
			"framespersecond":24,
			"generate_audio":true,
			"id":"provider-secret",
			"model":"provider-model",
			"priority":0,
			"service_tier":"default",
			"status":"succeeded",
			"usage":{"completion_tokens":0,"total_tokens":0}
		}
	}
}`

func TestSeedanceTaskResponseCalculatesUsageForPerDurationUpstream(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_public_usage",
		Platform:   constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo)),
		Status:     model.TaskStatusSuccess,
		SubmitTime: 111,
		UpdatedAt:  222,
		Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			RequestedDurationSeconds: 5,
			Resolution:               "720p",
			UpstreamCostMode:         string(types.CostModePerDuration),
		}},
		Data: json.RawMessage(newAPIVideoDetailedZeroUsage),
	}

	response, err := seedanceTaskResponse(task)

	require.NoError(t, err)
	usage, ok := response["usage"].(map[string]interface{})
	require.True(t, ok)
	assert.EqualValues(t, 108000, usage["completion_tokens"])
	assert.EqualValues(t, 108000, usage["total_tokens"])
	assert.Len(t, usage, 2)
}

func TestSeedanceTaskResponseCalculatesReferenceVideoUsage(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_public_reference_usage",
		Platform:   constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo)),
		Status:     model.TaskStatusSuccess,
		SubmitTime: 111,
		UpdatedAt:  222,
		Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			RequestedDurationSeconds: 5,
			Resolution:               "720p",
			HasVideoInput:            true,
			InputVideoDurationMS:     3000,
			UpstreamCostMode:         string(types.CostModePerRequest),
		}},
		Data: json.RawMessage(newAPIVideoDetailedZeroUsage),
	}

	response, err := seedanceTaskResponse(task)

	require.NoError(t, err)
	usage, ok := response["usage"].(map[string]interface{})
	require.True(t, ok)
	assert.EqualValues(t, 108000, usage["completion_tokens"])
	assert.EqualValues(t, 172800, usage["total_tokens"])
}

func TestSeedanceTaskResponseUsesUpstreamFrameRateForUsage(t *testing.T) {
	data := strings.Replace(newAPIVideoDetailedZeroUsage, `"framespersecond":24`, `"framespersecond":30`, 1)
	task := &model.Task{
		TaskID:     "task_public_frame_rate_usage",
		Platform:   constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo)),
		Status:     model.TaskStatusSuccess,
		Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			RequestedDurationSeconds: 5,
			Resolution:               "720p",
			UpstreamCostMode:         string(types.CostModePerDuration),
		}},
		Data: json.RawMessage(data),
	}

	response, err := seedanceTaskResponse(task)

	require.NoError(t, err)
	usage, ok := response["usage"].(map[string]interface{})
	require.True(t, ok)
	assert.EqualValues(t, 135000, usage["completion_tokens"])
	assert.EqualValues(t, 135000, usage["total_tokens"])
}

func TestSeedanceTaskResponsePreservesAuthoritativeUsage(t *testing.T) {
	data := strings.Replace(
		newAPIVideoDetailedZeroUsage,
		`"usage":{"completion_tokens":0,"total_tokens":0}`,
		`"usage":{"completion_tokens":108900,"total_tokens":108900}`,
		1,
	)
	task := &model.Task{
		TaskID:     "task_public_authoritative_usage",
		Platform:   constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo)),
		Status:     model.TaskStatusSuccess,
		Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			RequestedDurationSeconds: 5,
			Resolution:               "720p",
			UpstreamCostMode:         string(types.CostModePerDuration),
		}},
		Data: json.RawMessage(data),
	}

	response, err := seedanceTaskResponse(task)

	require.NoError(t, err)
	usage, ok := response["usage"].(map[string]interface{})
	require.True(t, ok)
	assert.EqualValues(t, 108900, usage["completion_tokens"])
	assert.EqualValues(t, 108900, usage["total_tokens"])
}

func TestSeedanceTaskResponseDoesNotCalculateUsageWithoutTrustedInputs(t *testing.T) {
	tests := []struct {
		name          string
		status        model.TaskStatus
		costMode      types.CostMode
		hasVideoInput bool
		inputDuration int64
		data          string
	}{
		{name: "per token upstream", status: model.TaskStatusSuccess, costMode: types.CostModePerToken},
		{name: "failed task", status: model.TaskStatusFailure, costMode: types.CostModePerDuration},
		{name: "historical reference video without duration", status: model.TaskStatusSuccess, costMode: types.CostModePerDuration, hasVideoInput: true},
		{
			name:     "fractional upstream duration",
			status:   model.TaskStatusSuccess,
			costMode: types.CostModePerDuration,
			data:     strings.Replace(newAPIVideoDetailedZeroUsage, `"duration":5`, `"duration":5.5`, 1),
		},
		{
			name:     "excessive upstream frame rate",
			status:   model.TaskStatusSuccess,
			costMode: types.CostModePerDuration,
			data:     strings.Replace(newAPIVideoDetailedZeroUsage, `"framespersecond":24`, `"framespersecond":241`, 1),
		},
		{
			name:     "overflowing upstream duration",
			status:   model.TaskStatusSuccess,
			costMode: types.CostModePerDuration,
			data:     strings.Replace(newAPIVideoDetailedZeroUsage, `"duration":5`, `"duration":18446744073709551617`, 1),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := tt.data
			if data == "" {
				data = newAPIVideoDetailedZeroUsage
			}
			task := &model.Task{
				TaskID:     "task_public_untrusted_usage",
				Platform:   constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo)),
				Status:     tt.status,
				Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
				PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
					RequestedDurationSeconds: 5,
					Resolution:               "720p",
					HasVideoInput:            tt.hasVideoInput,
					InputVideoDurationMS:     tt.inputDuration,
					UpstreamCostMode:         string(tt.costMode),
				}},
				Data: json.RawMessage(data),
			}

			response, err := seedanceTaskResponse(task)

			require.NoError(t, err)
			usage, ok := response["usage"].(map[string]interface{})
			require.True(t, ok)
			assert.EqualValues(t, 0, usage["completion_tokens"])
			assert.EqualValues(t, 0, usage["total_tokens"])
		})
	}
}

func createNewAPIVideoQueryTask(t *testing.T, channelTypes ...int) *model.Task {
	t.Helper()
	channelType := constant.ChannelTypeNewAPIVideo
	taskID := "task_public_newapi"
	if len(channelTypes) > 0 {
		channelType = channelTypes[0]
		taskID = "task_public_lucen"
	}
	now := time.Now().Unix()
	task := &model.Task{
		TaskID:     taskID,
		Platform:   constant.TaskPlatform(strconv.Itoa(channelType)),
		UserId:     7,
		ChannelId:  14,
		Group:      "secret-group",
		Quota:      2000000,
		Status:     model.TaskStatusSuccess,
		SubmitTime: now,
		FinishTime: now + 10,
		UpdatedAt:  now + 10,
		Progress:   "100%",
		Properties: model.Properties{OriginModelName: "client-model", UpstreamModelName: "provider-model"},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-secret",
			ResultURL:      "https://example.com/video.mp4",
			BillingContext: &model.TaskBillingContext{ServiceTier: "flex"},
		},
		Data: json.RawMessage(newAPIVideoDetailedZeroUsage),
	}
	require.NoError(t, model.DB.Create(task).Error)
	return task
}

func TestLucenARKAndOpenAIQueriesUsePublicProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSeedanceTaskDB(t)
	task := createNewAPIVideoQueryTask(t, constant.ChannelTypeLucen)

	single, _ := gin.CreateTestContext(httptest.NewRecorder())
	single.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/"+task.TaskID, nil)
	single.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	single.Set("id", 7)
	body, taskErr := SeedanceTaskFetch(single)
	require.Nil(t, taskErr)
	assertNewAPIVideoPublicBody(t, body)
	assert.Contains(t, string(body), task.TaskID)
	assert.Contains(t, string(body), `"model":"client-model"`)

	list, _ := gin.CreateTestContext(httptest.NewRecorder())
	list.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks", nil)
	list.Set("id", 7)
	body, taskErr = SeedanceTaskFetch(list)
	require.Nil(t, taskErr)
	assertNewAPIVideoPublicBody(t, body)
	assert.Contains(t, string(body), task.TaskID)

	openAI, _ := gin.CreateTestContext(httptest.NewRecorder())
	openAI.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/"+task.TaskID, nil)
	openAI.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	openAI.Set("id", 7)
	body, taskErr = videoFetchByIDRespBodyBuilder(openAI)
	require.Nil(t, taskErr)
	assertNewAPIVideoPublicBody(t, body)
	assert.Contains(t, string(body), task.TaskID)
	assert.Contains(t, string(body), `"model":"client-model"`)
}

func TestNewAPIVideoOpenAIQueryUsesDirectPublicProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSeedanceTaskDB(t)
	task := createNewAPIVideoQueryTask(t)

	for _, path := range []string{
		"/v1/video/generations/" + task.TaskID,
		"/v1/videos/" + task.TaskID,
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, path, nil)
		c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
		c.Set("id", 7)

		body, taskErr := videoFetchByIDRespBodyBuilder(c)
		require.Nil(t, taskErr)
		assert.NotContains(t, string(body), `"code":"success"`)
		assertNewAPIVideoPublicBody(t, body)
		var response dto.OpenAIVideo
		require.NoError(t, common.Unmarshal(body, &response))
		assert.Equal(t, task.TaskID, response.ID)
		assert.Equal(t, task.TaskID, response.TaskID)
		assert.Equal(t, "client-model", response.Model)
		assert.Equal(t, dto.VideoStatusCompleted, response.Status)
	}
}

func TestNewAPIVideoARKQuerySingleAndList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSeedanceTaskDB(t)
	task := createNewAPIVideoQueryTask(t)

	single, _ := gin.CreateTestContext(httptest.NewRecorder())
	single.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/"+task.TaskID, nil)
	single.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	single.Set("id", 7)
	body, taskErr := SeedanceTaskFetch(single)
	require.Nil(t, taskErr)
	assertNewAPIVideoPublicBody(t, body)
	assert.Contains(t, string(body), `"draft":false`)
	assert.Contains(t, string(body), `"priority":0`)
	assert.Contains(t, string(body), `"completion_tokens":0`)
	assert.Contains(t, string(body), `"total_tokens":0`)

	list, _ := gin.CreateTestContext(httptest.NewRecorder())
	list.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks?filter.service_tier=default", nil)
	list.Set("id", 7)
	body, taskErr = SeedanceTaskFetch(list)
	require.Nil(t, taskErr)
	assertNewAPIVideoPublicBody(t, body)
	var response seedanceTaskListResponse
	require.NoError(t, common.Unmarshal(body, &response))
	require.Equal(t, 1, response.Total)
	require.Len(t, response.Items, 1)
	assert.Equal(t, task.TaskID, response.Items[0]["id"])
}

func TestNewAPIVideoPollingFailureQueriesRemainPublic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSeedanceTaskDB(t)
	now := time.Now().Unix()
	task := model.Task{
		TaskID: "task_public_failed", Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo)), UserId: 7,
		Status: model.TaskStatusFailure, SubmitTime: now, FinishTime: now + 10, UpdatedAt: now + 10, Progress: "100%",
		FailReason:  "task not found or expired",
		Properties:  model.Properties{OriginModelName: "client-model", UpstreamModelName: "provider-model"},
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "upstream-secret"},
		Data:        json.RawMessage(`{"code":"not_found","message":"provider task missing","user_id":59,"quota":2000000}`),
	}
	require.NoError(t, model.DB.Create(&task).Error)

	openAI, _ := gin.CreateTestContext(httptest.NewRecorder())
	openAI.Request = httptest.NewRequest(http.MethodGet, "/v1/video/generations/"+task.TaskID, nil)
	openAI.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	openAI.Set("id", 7)
	body, taskErr := videoFetchByIDRespBodyBuilder(openAI)
	require.Nil(t, taskErr)
	assertNewAPIVideoPublicBody(t, body)
	assert.Contains(t, string(body), `"status":"failed"`)
	assert.Contains(t, string(body), `"message":"task not found or expired"`)

	single, _ := gin.CreateTestContext(httptest.NewRecorder())
	single.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/"+task.TaskID, nil)
	single.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	single.Set("id", 7)
	body, taskErr = SeedanceTaskFetch(single)
	require.Nil(t, taskErr)
	assertNewAPIVideoPublicBody(t, body)
	assert.Contains(t, string(body), `"status":"failed"`)
	assert.Contains(t, string(body), `"message":"task not found or expired"`)

	list, _ := gin.CreateTestContext(httptest.NewRecorder())
	list.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks", nil)
	list.Set("id", 7)
	body, taskErr = SeedanceTaskFetch(list)
	require.Nil(t, taskErr)
	assertNewAPIVideoPublicBody(t, body)
	var response seedanceTaskListResponse
	require.NoError(t, common.Unmarshal(body, &response))
	require.Equal(t, 1, response.Total)
	require.Len(t, response.Items, 1)
	assert.Equal(t, task.TaskID, response.Items[0]["id"])
}

func TestSeedanceTaskFetchRejectsUnsupportedPlatform(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSeedanceTaskDB(t)
	task := model.Task{
		TaskID: "task_unsupported", Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeKling)), UserId: 7,
		Status: model.TaskStatusSuccess, SubmitTime: time.Now().Unix(),
		Data: json.RawMessage(`{"id":"upstream-secret","status":"succeeded","quota":999}`),
	}
	require.NoError(t, model.DB.Create(&task).Error)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/"+task.TaskID, nil)
	c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	c.Set("id", 7)

	body, taskErr := SeedanceTaskFetch(c)
	assert.Nil(t, body)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusNotFound, taskErr.StatusCode)
}

func assertNewAPIVideoPublicBody(t *testing.T, body []byte) {
	t.Helper()
	for _, privateValue := range []string{
		"upstream-secret", "provider-model", "provider-secret", "user_id", "channel_id",
		"secret-group", "platform", "quota",
	} {
		assert.NotContains(t, string(body), privateValue)
	}
}

func configureNewAPIVideoFixedPricing(t *testing.T, modelName string) {
	t.Helper()
	original := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(original)) })
	ratios := ratio_setting.GetModelRatioCopy()
	ratios[modelName] = 0.1
	encoded, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(encoded)))
}

func newNewAPIVideoRelayContext(body, upstreamURL string) (*gin.Context, *relaycommon.RelayInfo) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(constant.ContextKeyChannelType), constant.ChannelTypeNewAPIVideo)
	c.Set(string(constant.ContextKeyChannelBaseUrl), upstreamURL)
	c.Set(string(constant.ContextKeyChannelKey), "mock-newapi-video-key")
	c.Set("model_mapping", `{"client-video":"seedance-720p-token"}`)
	common.SetContextKey(c, constant.ContextKeyRoutingFacts, modelrouting.Facts{
		OutputResolution: "720p",
		DurationSeconds:  5,
	})
	return c, &relaycommon.RelayInfo{
		OriginModelName: "client-video",
		UserGroup:       "default",
		UsingGroup:      "default",
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
		Billing:         seedanceTaskTestBilling{},
	}
}

func TestNewAPIVideoDurationFixedModePreservesFractionalValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
	configureNewAPIVideoFixedPricing(t, "client-video")
	var upstreamCalls atomic.Int32
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"upstream-task","status":"queued"}`))
	}))
	t.Cleanup(server.Close)
	c, info := newNewAPIVideoRelayContext(`{"model":"client-video","prompt":"text","duration":5.5,"watermark":false,"seed":0}`, server.URL)

	result, taskErr := RelayTaskSubmit(c, info, nil)

	require.Nil(t, taskErr)
	require.NotNil(t, result)
	assert.Equal(t, int32(1), upstreamCalls.Load())
	assert.JSONEq(t, `{"model":"seedance-720p-token","prompt":"text","duration":5.5,"watermark":false,"seed":0}`, string(capturedBody))
	assert.NotEqual(t, billing_setting.BillingModePerDuration, info.PriceData.BillingMode)
}

func TestNewAPIVideoDurationPerDurationMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
	configureSeedanceDurationPricing(t, map[string]types.DurationPrice{
		"client-video": {Price: 0.1, Unit: types.DurationUnitSecond, RoundingStepSeconds: 1, MinimumDurationSeconds: 1},
	})

	tests := []struct {
		name              string
		body              string
		wantSuccess       bool
		wantSeconds       int
		wantUpstreamValue string
	}{
		{name: "fractional duration", body: `{"model":"client-video","prompt":"text","duration":5.5}`},
		{name: "integer seconds five", body: `{"model":"client-video","prompt":"text","seconds":"5"}`, wantSuccess: true, wantSeconds: 5, wantUpstreamValue: "5"},
		{name: "numeric seconds", body: `{"model":"client-video","prompt":"text","seconds":5}`},
		{name: "integer seconds ten", body: `{"model":"client-video","prompt":"text","seconds":"10"}`, wantSuccess: true, wantSeconds: 10, wantUpstreamValue: "10"},
		{name: "duration only", body: `{"model":"client-video","prompt":"text","duration":10}`},
		{name: "duration overflow", body: `{"model":"client-video","prompt":"text","duration":3601}`},
		{name: "metadata bypass", body: `{"model":"client-video","prompt":"text","duration":5,"metadata":{"duration":3601}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var upstreamCalls atomic.Int32
			var capturedBody []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamCalls.Add(1)
				capturedBody, _ = io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"upstream-task","status":"queued"}`))
			}))
			t.Cleanup(server.Close)
			c, info := newNewAPIVideoRelayContext(tt.body, server.URL)

			result, taskErr := RelayTaskSubmit(c, info, nil)

			if !tt.wantSuccess {
				assert.Nil(t, result)
				require.NotNil(t, taskErr)
				assert.Equal(t, int32(0), upstreamCalls.Load())
				return
			}
			require.Nil(t, taskErr)
			require.NotNil(t, result)
			assert.Equal(t, int32(1), upstreamCalls.Load())
			assert.Equal(t, tt.wantSeconds, info.PriceData.RequestedDurationSeconds)
			assert.Equal(t, tt.wantSeconds, info.PriceData.BillableDurationSeconds)
			assert.NotContains(t, info.PriceData.OtherRatios(), "seconds")
			var upstream map[string]interface{}
			require.NoError(t, common.Unmarshal(capturedBody, &upstream))
			assert.Equal(t, tt.wantUpstreamValue, upstream["seconds"])
		})
	}
}

func TestSeedanceTaskFetchPreservesOfficialFailedTaskFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSeedanceTaskDB(t)
	task := model.Task{
		TaskID:     "task_public",
		Platform:   constant.TaskPlatform("54"),
		UserId:     7,
		Status:     model.TaskStatusFailure,
		SubmitTime: 111,
		UpdatedAt:  222,
		Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "cgt-20260717171624-cr2n9",
		},
		Data: json.RawMessage(`{
			"id":"cgt-20260717171624-cr2n9",
			"model":"doubao-seedance-2-0-260128",
			"status":"failed",
			"error":{
				"code":"OutputVideoSensitiveContentDetected.PolicyViolation",
				"message":"The request failed because the output video may be related to copyright restrictions. Request id: 02178427978698300000000000000000000ffffac1923a9fc42b8"
			},
			"created_at":1784279786,
			"updated_at":1784280145,
			"service_tier":"default",
			"execution_expires_after":172800,
			"generate_audio":true,
			"draft":false,
			"priority":0
		}`),
	}
	require.NoError(t, model.DB.Create(&task).Error)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_public", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "task_public"}}
	c.Set("id", 7)
	body, taskErr := SeedanceTaskFetch(c)
	require.Nil(t, taskErr)
	assert.NotContains(t, string(body), "cgt-20260717171624-cr2n9")

	var response struct {
		ID                    string `json:"id"`
		Model                 string `json:"model"`
		Status                string `json:"status"`
		CreatedAt             int64  `json:"created_at"`
		UpdatedAt             int64  `json:"updated_at"`
		ServiceTier           string `json:"service_tier"`
		ExecutionExpiresAfter int    `json:"execution_expires_after"`
		GenerateAudio         bool   `json:"generate_audio"`
		Draft                 bool   `json:"draft"`
		Priority              int    `json:"priority"`
		Error                 struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, "task_public", response.ID)
	assert.Equal(t, "doubao-seedance-2-0-260128", response.Model)
	assert.Equal(t, "failed", response.Status)
	assert.Equal(t, int64(1784279786), response.CreatedAt)
	assert.Equal(t, int64(1784280145), response.UpdatedAt)
	assert.Equal(t, "default", response.ServiceTier)
	assert.Equal(t, 172800, response.ExecutionExpiresAfter)
	assert.True(t, response.GenerateAudio)
	assert.False(t, response.Draft)
	assert.Zero(t, response.Priority)
	assert.Equal(t, "OutputVideoSensitiveContentDetected.PolicyViolation", response.Error.Code)
	assert.Contains(t, response.Error.Message, "copyright restrictions")
}

func TestSeedanceTaskFetchPreservesOfficialExpiredAndCancelledStatuses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSeedanceTaskDB(t)
	for index, status := range []string{"expired", "cancelled"} {
		task := model.Task{
			TaskID:     "task_" + status,
			Platform:   constant.TaskPlatform("54"),
			UserId:     7,
			Status:     model.TaskStatusFailure,
			SubmitTime: 1784279786,
			Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
			Data:       json.RawMessage(`{"id":"cgt-secret-` + status + `","model":"doubao-seedance-2-0-260128","status":"` + status + `","created_at":1784279786,"updated_at":1784280145}`),
		}
		task.ID = int64(index + 1)
		require.NoError(t, model.DB.Create(&task).Error)

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/"+task.TaskID, nil)
		c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
		c.Set("id", 7)
		body, taskErr := SeedanceTaskFetch(c)
		require.Nil(t, taskErr)
		var response map[string]interface{}
		require.NoError(t, common.Unmarshal(body, &response))
		assert.Equal(t, status, response["status"])
	}
}

func TestSeedanceTaskListFiltersAndPaginatesJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSeedanceTaskDB(t)
	now := time.Now().Unix()
	tasks := make([]model.Task, 0, seedanceTaskScanBatchSize+5)
	for i := 0; i < seedanceTaskScanBatchSize+5; i++ {
		modelName := "other-model"
		if i%2 == 0 {
			modelName = "wanted-model"
		}
		tasks = append(tasks, model.Task{
			TaskID:     "task_" + time.Unix(int64(i), 0).Format("150405"),
			Platform:   constant.TaskPlatform("45"),
			UserId:     7,
			Status:     model.TaskStatusQueued,
			SubmitTime: now,
			Properties: model.Properties{OriginModelName: modelName},
			Data:       json.RawMessage(`{"service_tier":"default"}`),
		})
	}
	require.NoError(t, model.DB.CreateInBatches(&tasks, 50).Error)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks?filter.model=wanted-model&filter.service_tier=default&page_num=2&page_size=3", nil)
	c.Set("id", 7)
	body, taskErr := SeedanceTaskFetch(c)
	require.Nil(t, taskErr)
	var response seedanceTaskListResponse
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, 103, response.Total)
	assert.Len(t, response.Items, 3)
}

func TestSeedanceTaskListDefaultsMissingServiceTier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSeedanceTaskDB(t)
	now := time.Now().Unix()
	task := model.Task{
		TaskID:     "task_default_tier",
		Platform:   constant.TaskPlatform("54"),
		UserId:     7,
		Status:     model.TaskStatusQueued,
		SubmitTime: now,
		Properties: model.Properties{OriginModelName: "wanted-model"},
		Data:       json.RawMessage(`{"id":"cgt-secret"}`),
	}
	require.NoError(t, model.DB.Create(&task).Error)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks?filter.service_tier=default", nil)
	c.Set("id", 7)
	body, taskErr := SeedanceTaskFetch(c)
	require.Nil(t, taskErr)
	var response seedanceTaskListResponse
	require.NoError(t, common.Unmarshal(body, &response))
	require.Equal(t, 1, response.Total)
	require.Len(t, response.Items, 1)
}

func TestSeedreamNativeImageBodyMapsOnlyModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/images/generations", strings.NewReader(`{"model":"alias","prompt":"x","watermark":false,"seed":0,"unknown":{"value":0}}`))
	c.Request.Header.Set("Content-Type", "application/json")
	_, err := common.GetBodyStorage(c)
	require.NoError(t, err)
	body, err := buildSeedreamImageRequestBody(c, "doubao-seedream-5-0-lite-260128")
	require.NoError(t, err)
	var fields map[string]interface{}
	require.NoError(t, common.Unmarshal(body, &fields))
	assert.Equal(t, "doubao-seedream-5-0-lite-260128", fields["model"])
	assert.Equal(t, false, fields["watermark"])
	assert.Equal(t, float64(0), fields["seed"])
	assert.Equal(t, map[string]interface{}{"value": float64(0)}, fields["unknown"])
}
