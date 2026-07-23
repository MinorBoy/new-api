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
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
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

	result, taskErr := RelayTaskSubmit(c, info)

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

	result, taskErr := RelayTaskSubmit(c, info)

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
		TaskID: "task_public", Platform: constant.TaskPlatform("59"), UserId: 7,
		Status: model.TaskStatusSuccess, SubmitTime: 111, UpdatedAt: 222,
		Properties:  model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "dim-upstream"},
		Data:        json.RawMessage(`{"task_id":"dim-upstream","status":"completed","result":{"url":"https://x/video.mp4"}}`),
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
}

func TestDimensioTaskFetchPreservesProviderTimestampsAndErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSeedanceTaskDB(t)
	task := model.Task{
		TaskID: "task_failed", Platform: constant.TaskPlatform("59"), UserId: 7,
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
}

func TestDimensioTaskAdaptorIsTaskOnly(t *testing.T) {
	require.NotNil(t, GetTaskAdaptor(constant.TaskPlatform("59")))
	_, success := common.ChannelType2APIType(constant.ChannelTypeDimensio)
	require.False(t, success)
}

func TestNewAPIVideoTaskAdaptorIsTaskOnly(t *testing.T) {
	require.NotNil(t, GetTaskAdaptor(constant.TaskPlatform("60")))
	_, success := common.ChannelType2APIType(constant.ChannelTypeNewAPIVideo)
	require.False(t, success)
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

func createNewAPIVideoQueryTask(t *testing.T) *model.Task {
	t.Helper()
	now := time.Now().Unix()
	task := &model.Task{
		TaskID:     "task_public_newapi",
		Platform:   constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo)),
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

	result, taskErr := RelayTaskSubmit(c, info)

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

			result, taskErr := RelayTaskSubmit(c, info)

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
