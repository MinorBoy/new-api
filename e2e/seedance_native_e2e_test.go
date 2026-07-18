package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/router"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	e2eUserID                       = 1001
	e2eOtherUserID                  = 1002
	e2eChannelID                    = 1
	e2eToken                        = "e2e"
	upstreamTaskID                  = "cgt-mock-seedance-2-0"
	failedUpstreamTaskID            = "cgt-20260717171624-cr2n9"
	seedance20MultimodalRequestBody = `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"全程使用视频1的第一视角构图，全程使用音频1作为背景音乐。第一人称视角果茶宣传广告，seedance牌「苹苹安安」苹果果茶限定款。"},{"type":"image_url","image_url":{"url":"https://mock.example/reference-image-1.jpg"},"role":"reference_image"},{"type":"image_url","image_url":{"url":"https://mock.example/reference-image-2.jpg"},"role":"reference_image"},{"type":"video_url","video_url":{"url":"https://mock.example/reference-video.mp4"},"role":"reference_video"},{"type":"audio_url","audio_url":{"url":"https://mock.example/reference-audio.mp3"},"role":"reference_audio"}],"generate_audio":true,"ratio":"16:9","duration":11,"watermark":true}`
	successUpstreamTaskResponse     = `{"id":"cgt-mock-seedance-2-0","model":"doubao-seedance-2-0-260128","status":"succeeded","content":{"video_url":"https://ark-content-generation-cn-beijing.tos-cn-beijing.volces.com/xxx"},"usage":{"completion_tokens":108900,"total_tokens":108900},"created_at":1779348818,"updated_at":1779348874,"seed":78674,"resolution":"720p","ratio":"16:9","duration":5,"framespersecond":24,"service_tier":"default","execution_expires_after":172800,"generate_audio":true,"draft":false,"priority":0}`
	failedUpstreamTaskResponse      = `{"id":"cgt-20260717171624-cr2n9","model":"doubao-seedance-2-0-260128","status":"failed","error":{"code":"OutputVideoSensitiveContentDetected.PolicyViolation","message":"The request failed because the output video may be related to copyright restrictions. Request id: 02178427978698300000000000000000000ffffac1923a9fc42b8"},"created_at":1784279786,"updated_at":1784280145,"service_tier":"default","execution_expires_after":172800,"generate_audio":true,"draft":false,"priority":0}`
)

type mockArkRequest struct {
	Method        string
	Path          string
	Authorization string
	Body          []byte
}

type mockArkServer struct {
	mu               sync.Mutex
	requests         []mockArkRequest
	taskID           string
	terminalResponse string
	submitStatus     int
	submitResponse   string
}

func (m *mockArkServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	m.mu.Lock()
	m.requests = append(m.requests, mockArkRequest{
		Method:        r.Method,
		Path:          r.URL.Path,
		Authorization: r.Header.Get("Authorization"),
		Body:          append([]byte(nil), body...),
	})
	m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	taskID := m.taskID
	if taskID == "" {
		taskID = upstreamTaskID
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v3/contents/generations/tasks":
		if m.submitStatus != 0 {
			w.WriteHeader(m.submitStatus)
		}
		submitResponse := m.submitResponse
		if submitResponse == "" {
			submitResponse = `{"id":"` + taskID + `"}`
		}
		_, _ = w.Write([]byte(submitResponse))
	case r.Method == http.MethodGet && r.URL.Path == "/api/v3/contents/generations/tasks/"+taskID:
		terminalResponse := m.terminalResponse
		if terminalResponse == "" {
			terminalResponse = successUpstreamTaskResponse
		}
		_, _ = w.Write([]byte(terminalResponse))
	default:
		http.NotFound(w, r)
	}
}

func (m *mockArkServer) snapshot() []mockArkRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	requests := make([]mockArkRequest, len(m.requests))
	copy(requests, m.requests)
	return requests
}

func setupSeedanceE2EDB(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalIsMaster := common.IsMasterNode
	originalSQLitePath := common.SQLitePath
	originalMainDB := common.MainDatabaseType()
	originalLogDatabase := common.LogDatabaseType()
	originalRedis := common.RedisEnabled
	originalBatchUpdate := common.BatchUpdateEnabled
	originalLogConsume := common.LogConsumeEnabled
	originalDataExport := common.DataExportEnabled
	originalTaskTimeout := constant.TaskTimeoutMinutes
	originalTaskQueryLimit := constant.TaskQueryLimit
	originalDSN, hadDSN := os.LookupEnv("SQL_DSN")

	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.IsMasterNode = false
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = true
	common.DataExportEnabled = true
	constant.TaskTimeoutMinutes = 0
	constant.TaskQueryLimit = 100
	model.CacheQuotaDataLock.Lock()
	originalQuotaDataCache := model.CacheQuotaData
	model.CacheQuotaData = make(map[string]*model.QuotaData)
	model.CacheQuotaDataLock.Unlock()
	common.SQLitePath = fmt.Sprintf("file:seedance_e2e_%d?mode=memory&cache=shared", time.Now().UnixNano())
	_ = os.Unsetenv("SQL_DSN")
	require.NoError(t, model.InitDB())
	db := model.DB
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.Task{}, &model.Log{}, &model.QuotaData{},
		&model.SubscriptionPlan{}, &model.UserSubscription{},
	))

	originalRatios := ratio_setting.ModelRatio2JSONString()
	ratio_setting.InitRatioSettings()
	prices := ratio_setting.GetModelRatioCopy()
	prices["doubao-seedance-2-0-260128"] = 0.1
	priceJSON, err := common.Marshal(prices)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(priceJSON)))
	service.InitHttpClient()
	t.Cleanup(func() {
		_ = ratio_setting.UpdateModelRatioByJSONString(originalRatios)
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.IsMasterNode = originalIsMaster
		common.SQLitePath = originalSQLitePath
		common.SetDatabaseTypes(originalMainDB, originalLogDatabase)
		common.RedisEnabled = originalRedis
		common.BatchUpdateEnabled = originalBatchUpdate
		common.LogConsumeEnabled = originalLogConsume
		common.DataExportEnabled = originalDataExport
		constant.TaskTimeoutMinutes = originalTaskTimeout
		constant.TaskQueryLimit = originalTaskQueryLimit
		model.CacheQuotaDataLock.Lock()
		model.CacheQuotaData = originalQuotaDataCache
		model.CacheQuotaDataLock.Unlock()
		if hadDSN {
			_ = os.Setenv("SQL_DSN", originalDSN)
		} else {
			_ = os.Unsetenv("SQL_DSN")
		}
	})
}

func seedSeedanceE2EData(t *testing.T, upstreamURL string) {
	t.Helper()
	user := &model.User{
		Id:       e2eUserID,
		Username: "seedance_e2e_user",
		Password: "e2e-password",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
		Quota:    2_000_000_000,
		Group:    "default",
		AffCode:  "seedance-e2e-user",
	}
	otherUser := &model.User{
		Id:       e2eOtherUserID,
		Username: "seedance_e2e_other",
		Password: "e2e-password",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
		Quota:    2_000_000_000,
		Group:    "default",
		AffCode:  "seedance-e2e-other",
	}
	require.NoError(t, model.DB.Create(user).Error)
	require.NoError(t, model.DB.Create(otherUser).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		Id:             1,
		UserId:         e2eUserID,
		Key:            e2eToken,
		Status:         common.TokenStatusEnabled,
		RemainQuota:    2_000_000_000,
		UnlimitedQuota: true,
		Group:          "default",
	}).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		Id:             2,
		UserId:         e2eOtherUserID,
		Key:            "other",
		Status:         common.TokenStatusEnabled,
		RemainQuota:    2_000_000_000,
		UnlimitedQuota: true,
		Group:          "default",
	}).Error)

	channel := &model.Channel{
		Id:            e2eChannelID,
		Type:          constant.ChannelTypeDoubaoVideo,
		Key:           "mock-ark-key",
		Status:        common.ChannelStatusEnabled,
		Name:          "seedance-e2e-mock",
		Weight:        common.GetPointer[uint](1),
		BaseURL:       common.GetPointer(upstreamURL),
		Models:        "doubao-seedance-2-0-260128",
		Group:         "default",
		CreatedTime:   time.Now().Unix(),
		OtherSettings: "{}",
	}
	settings := dto.ChannelOtherSettings{DisableTaskPollingSleep: true}
	channel.SetOtherSettings(settings)
	require.NoError(t, channel.Insert())
}

func seedSecondSeedanceE2EChannel(t *testing.T, upstreamURL string) {
	t.Helper()
	highPriority := int64(10)
	firstChannel, err := model.GetChannelById(e2eChannelID, true)
	require.NoError(t, err)
	firstChannel.Priority = &highPriority
	require.NoError(t, firstChannel.Update())

	lowPriority := int64(0)
	secondChannel := &model.Channel{
		Id:            2,
		Type:          constant.ChannelTypeDoubaoVideo,
		Key:           "mock-ark-key-b",
		Status:        common.ChannelStatusEnabled,
		Name:          "seedance-e2e-mock-b",
		Weight:        common.GetPointer[uint](1),
		Priority:      &lowPriority,
		BaseURL:       common.GetPointer(upstreamURL),
		Models:        "doubao-seedance-2-0-260128",
		Group:         "default",
		CreatedTime:   time.Now().Unix(),
		OtherSettings: "{}",
	}
	secondChannel.SetOtherSettings(dto.ChannelOtherSettings{DisableTaskPollingSleep: true})
	require.NoError(t, secondChannel.Insert())
}

func seedanceE2ERouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	router.SetRelayRouter(engine)
	router.SetVideoRouter(engine)
	return engine
}

func performJSONRequest(t *testing.T, engine http.Handler, method, path, authorization, body string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	return recorder.Code, recorder.Body.Bytes()
}

func TestSeedanceNativeSeedance20MultimodalE2E(t *testing.T) {
	setupSeedanceE2EDB(t)
	mock := &mockArkServer{}
	mockServer := httptest.NewServer(mock)
	t.Cleanup(mockServer.Close)
	seedSeedanceE2EData(t, mockServer.URL)
	engine := seedanceE2ERouter()
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		return relay.GetTaskAdaptor(platform)
	}
	t.Cleanup(func() { service.GetTaskAdaptorFunc = nil })

	requestBody := seedance20MultimodalRequestBody
	status, submitResponse := performJSONRequest(t, engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", requestBody)
	require.Equal(t, http.StatusOK, status, string(submitResponse))
	t.Logf("客户端提交请求: %s", requestBody)
	t.Logf("网关提交响应: %s", submitResponse)

	var publicResponse map[string]interface{}
	require.NoError(t, common.Unmarshal(submitResponse, &publicResponse))
	require.Len(t, publicResponse, 1)
	publicID, ok := publicResponse["id"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(publicID, "task_"))
	assert.NotContains(t, string(submitResponse), upstreamTaskID)

	requests := mock.snapshot()
	require.Len(t, requests, 1)
	assert.Equal(t, http.MethodPost, requests[0].Method)
	assert.Equal(t, "/api/v3/contents/generations/tasks", requests[0].Path)
	assert.Equal(t, "Bearer mock-ark-key", requests[0].Authorization)
	t.Logf("mock ARK 提交请求: %s", requests[0].Body)
	var upstreamRequest map[string]interface{}
	require.NoError(t, common.Unmarshal(requests[0].Body, &upstreamRequest))
	assert.Equal(t, "doubao-seedance-2-0-260128", upstreamRequest["model"])
	assert.Equal(t, true, upstreamRequest["watermark"])
	_, hasResolution := upstreamRequest["resolution"]
	assert.False(t, hasResolution)
	content, ok := upstreamRequest["content"].([]interface{})
	require.True(t, ok)
	require.Len(t, content, 5)
	assert.Equal(t, "text", content[0].(map[string]interface{})["type"])
	assert.Equal(t, "image_url", content[1].(map[string]interface{})["type"])
	assert.Equal(t, "reference_image", content[1].(map[string]interface{})["role"])
	assert.Equal(t, "image_url", content[2].(map[string]interface{})["type"])
	assert.Equal(t, "reference_image", content[2].(map[string]interface{})["role"])
	assert.Equal(t, "video_url", content[3].(map[string]interface{})["type"])
	assert.Equal(t, "reference_video", content[3].(map[string]interface{})["role"])
	assert.Equal(t, "audio_url", content[4].(map[string]interface{})["type"])
	assert.Equal(t, "reference_audio", content[4].(map[string]interface{})["role"])

	var task model.Task
	require.NoError(t, model.DB.Where("task_id = ?", publicID).First(&task).Error)
	assert.Equal(t, constant.TaskPlatform("54"), task.Platform)
	assert.Equal(t, upstreamTaskID, task.PrivateData.UpstreamTaskID)
	assert.Equal(t, "doubao-seedance-2-0-260128", task.Properties.OriginModelName)
	assert.Equal(t, "doubao-seedance-2-0-260128", task.Properties.UpstreamModelName)
	require.NotNil(t, task.PrivateData.BillingContext)
	assert.True(t, task.PrivateData.BillingContext.HasVideoInput)
	require.NotNil(t, task.PrivateData.BillingContext.GenerateAudio)
	assert.True(t, *task.PrivateData.BillingContext.GenerateAudio)
	assert.Equal(t, "720p", task.PrivateData.BillingContext.Resolution)
	assert.Equal(t, "default", task.PrivateData.BillingContext.ServiceTier)
	assert.Contains(t, task.PrivateData.BillingContext.OtherRatios, "video_input")
	preConsumedQuota := task.Quota
	assert.Equal(t, 15217, preConsumedQuota)
	t.Logf("提交后内部任务状态: status=%s progress=%s platform=%s unfinished=%d", task.Status, task.Progress, task.Platform, len(model.GetAllUnFinishSyncTasks(100)))

	status, queryResponse := performJSONRequest(t, engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+publicID, "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(queryResponse))
	assert.Contains(t, string(queryResponse), `"id":"`+publicID+`"`)
	assert.NotContains(t, string(queryResponse), upstreamTaskID)
	t.Logf("轮询前查询响应: %s", queryResponse)

	summary := service.RunTaskPollingOnce(context.Background(), nil)
	assert.Equal(t, 1, summary.UnfinishedTasks)
	requests = mock.snapshot()
	require.Len(t, requests, 2)
	assert.Equal(t, http.MethodGet, requests[1].Method)
	assert.Equal(t, "/api/v3/contents/generations/tasks/"+upstreamTaskID, requests[1].Path)
	assert.Equal(t, "Bearer mock-ark-key", requests[1].Authorization)
	t.Logf("mock ARK 状态响应: %s", successUpstreamTaskResponse)

	require.NoError(t, model.DB.Where("task_id = ?", publicID).First(&task).Error)
	assert.Equal(t, string(model.TaskStatusSuccess), string(task.Status))
	assert.Equal(t, "100%", task.Progress)
	assert.Equal(t, "https://ark-content-generation-cn-beijing.tos-cn-beijing.volces.com/xxx", task.PrivateData.ResultURL)
	require.NotNil(t, task.PrivateData.BillingContext)
	assert.Equal(t, "720p", task.PrivateData.BillingContext.Resolution)
	assert.Equal(t, 108900, task.PrivateData.BillingContext.BillingTokens)
	assert.Equal(t, 6628, task.Quota)
	assert.Less(t, task.Quota, preConsumedQuota)
	t.Logf("轮询后任务数据: %s", task.Data)

	var billedUser model.User
	var billedChannel model.Channel
	var billedToken model.Token
	require.NoError(t, model.DB.First(&billedUser, e2eUserID).Error)
	require.NoError(t, model.DB.First(&billedChannel, e2eChannelID).Error)
	require.NoError(t, model.DB.First(&billedToken, 1).Error)
	assert.Equal(t, 2_000_000_000, billedUser.Quota+billedUser.UsedQuota)
	assert.Equal(t, task.Quota, billedUser.UsedQuota)
	assert.Equal(t, 1, billedUser.RequestCount)
	assert.Equal(t, int64(task.Quota), billedChannel.UsedQuota)
	assert.Equal(t, task.Quota, billedToken.UsedQuota)

	model.CacheQuotaDataLock.Lock()
	quotaDataSnapshot := make([]*model.QuotaData, 0, len(model.CacheQuotaData))
	for _, quotaData := range model.CacheQuotaData {
		copyQuotaData := *quotaData
		quotaDataSnapshot = append(quotaDataSnapshot, &copyQuotaData)
	}
	model.CacheQuotaDataLock.Unlock()
	require.Len(t, quotaDataSnapshot, 1)
	for _, quotaData := range quotaDataSnapshot {
		assert.Equal(t, 1, quotaData.Count)
		assert.Equal(t, task.Quota, quotaData.Quota)
		assert.Equal(t, 108900, quotaData.TokenUsed)
		assert.Equal(t, e2eUserID, quotaData.UserID)
		assert.Equal(t, e2eChannelID, quotaData.ChannelID)
		assert.Equal(t, 1, quotaData.TokenID)
	}
	t.Logf("计费结算: pre_consumed=%d actual=%d billing_tokens=%d user_used=%d channel_used=%d token_used_quota=%d", preConsumedQuota, task.Quota, task.PrivateData.BillingContext.BillingTokens, billedUser.UsedQuota, billedChannel.UsedQuota, billedToken.UsedQuota)

	status, queryResponse = performJSONRequest(t, engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+publicID, "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(queryResponse))
	assert.NotContains(t, string(queryResponse), upstreamTaskID)
	var successfulFields map[string]interface{}
	require.NoError(t, common.Unmarshal(queryResponse, &successfulFields))
	require.Len(t, successfulFields, 17)
	assert.Equal(t, publicID, successfulFields["id"])
	assert.Equal(t, "doubao-seedance-2-0-260128", successfulFields["model"])
	assert.Equal(t, "succeeded", successfulFields["status"])
	assert.Equal(t, map[string]interface{}{"video_url": "https://ark-content-generation-cn-beijing.tos-cn-beijing.volces.com/xxx"}, successfulFields["content"])
	assert.Equal(t, map[string]interface{}{"completion_tokens": float64(108900), "total_tokens": float64(108900)}, successfulFields["usage"])
	assert.Equal(t, float64(1779348818), successfulFields["created_at"])
	assert.Equal(t, float64(1779348874), successfulFields["updated_at"])
	assert.Equal(t, float64(78674), successfulFields["seed"])
	assert.Equal(t, "720p", successfulFields["resolution"])
	assert.Equal(t, "16:9", successfulFields["ratio"])
	assert.Equal(t, float64(5), successfulFields["duration"])
	assert.Equal(t, float64(24), successfulFields["framespersecond"])
	assert.Equal(t, "default", successfulFields["service_tier"])
	assert.Equal(t, float64(172800), successfulFields["execution_expires_after"])
	assert.Equal(t, true, successfulFields["generate_audio"])
	assert.Equal(t, false, successfulFields["draft"])
	assert.Equal(t, float64(0), successfulFields["priority"])
	t.Logf("轮询后公开查询响应: %s", queryResponse)

	status, listResponse := performJSONRequest(t, engine, http.MethodGet, "/api/v3/contents/generations/tasks?filter.task_ids="+publicID+"&filter.service_tier=default&page_size=20", "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(listResponse))
	assert.NotContains(t, string(listResponse), upstreamTaskID)
	var listFields struct {
		Items []map[string]interface{} `json:"items"`
		Total int                      `json:"total"`
	}
	require.NoError(t, common.Unmarshal(listResponse, &listFields))
	require.Equal(t, 1, listFields.Total)
	require.Len(t, listFields.Items, 1)
	assert.Equal(t, successfulFields, listFields.Items[0])
	t.Logf("公开任务列表响应: %s", listResponse)

	status, otherQuery := performJSONRequest(t, engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+publicID, "Bearer other-1", "")
	assert.Equal(t, http.StatusNotFound, status, string(otherQuery))
	assert.Contains(t, string(otherQuery), "task_not_exist")

	invalidAudioOnly := `{"model":"doubao-seedance-2-0-260128","content":[{"type":"audio_url","audio_url":{"url":"https://mock.example/reference-audio.wav"},"role":"reference_audio"},{"type":"text","text":"audio only should fail"}]}`
	status, invalidResponse := performJSONRequest(t, engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", invalidAudioOnly)
	assert.Equal(t, http.StatusBadRequest, status, string(invalidResponse))
	assert.Contains(t, string(invalidResponse), "audio input requires an image or video")
	assert.Contains(t, string(invalidResponse), `"code":"InvalidParameter.content"`)
	assert.Len(t, mock.snapshot(), 2)

	fast1080 := `{"model":"doubao-seedance-2-0-fast-260128","content":[{"type":"text","text":"unsupported resolution"}],"resolution":"1080p"}`
	status, fastResponse := performJSONRequest(t, engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", fast1080)
	assert.Equal(t, http.StatusBadRequest, status, string(fastResponse))
	assert.Contains(t, string(fastResponse), "resolution 1080p is not supported")
	assert.Contains(t, string(fastResponse), `"code":"InvalidParameter"`)
	assert.Len(t, mock.snapshot(), 2)

	status, legacyResponse := performJSONRequest(t, engine, http.MethodPost, "/seedance/api/v3/contents/generations/tasks", "Bearer e2e-1", requestBody)
	assert.Equal(t, http.StatusNotFound, status, string(legacyResponse))
	assert.Len(t, mock.snapshot(), 2)
}

func TestSeedanceNativeFailedTaskResponseAndRefundE2E(t *testing.T) {
	setupSeedanceE2EDB(t)
	mock := &mockArkServer{
		taskID:           failedUpstreamTaskID,
		terminalResponse: failedUpstreamTaskResponse,
	}
	mockServer := httptest.NewServer(mock)
	t.Cleanup(mockServer.Close)
	seedSeedanceE2EData(t, mockServer.URL)
	engine := seedanceE2ERouter()
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		return relay.GetTaskAdaptor(platform)
	}
	t.Cleanup(func() { service.GetTaskAdaptorFunc = nil })

	status, submitResponse := performJSONRequest(t, engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", seedance20MultimodalRequestBody)
	require.Equal(t, http.StatusOK, status, string(submitResponse))
	var submitFields map[string]interface{}
	require.NoError(t, common.Unmarshal(submitResponse, &submitFields))
	require.Len(t, submitFields, 1)
	publicID, ok := submitFields["id"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(publicID, "task_"))
	assert.NotContains(t, string(submitResponse), failedUpstreamTaskID)

	var task model.Task
	require.NoError(t, model.DB.Where("task_id = ?", publicID).First(&task).Error)
	preConsumedQuota := task.Quota
	require.Equal(t, 15217, preConsumedQuota)

	summary := service.RunTaskPollingOnce(context.Background(), nil)
	assert.Equal(t, 1, summary.UnfinishedTasks)
	requests := mock.snapshot()
	require.Len(t, requests, 2)
	assert.Equal(t, "/api/v3/contents/generations/tasks/"+failedUpstreamTaskID, requests[1].Path)

	status, failedResponse := performJSONRequest(t, engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+publicID, "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(failedResponse))
	assert.NotContains(t, string(failedResponse), failedUpstreamTaskID)
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
	require.NoError(t, common.Unmarshal(failedResponse, &response))
	assert.Equal(t, publicID, response.ID)
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

	require.NoError(t, model.DB.Where("task_id = ?", publicID).First(&task).Error)
	assert.Equal(t, string(model.TaskStatusFailure), string(task.Status))
	assert.Equal(t, "100%", task.Progress)
	assert.Equal(t, response.Error.Message, task.FailReason)
	assert.Equal(t, preConsumedQuota, task.Quota)

	var refundedUser model.User
	var refundedChannel model.Channel
	var refundedToken model.Token
	require.NoError(t, model.DB.First(&refundedUser, e2eUserID).Error)
	require.NoError(t, model.DB.First(&refundedChannel, e2eChannelID).Error)
	require.NoError(t, model.DB.First(&refundedToken, 1).Error)
	assert.Equal(t, 2_000_000_000, refundedUser.Quota)
	assert.Zero(t, refundedUser.UsedQuota)
	assert.Equal(t, 1, refundedUser.RequestCount)
	assert.Zero(t, refundedChannel.UsedQuota)
	assert.Zero(t, refundedToken.UsedQuota)

	model.CacheQuotaDataLock.Lock()
	quotaDataSnapshot := make([]*model.QuotaData, 0, len(model.CacheQuotaData))
	for _, quotaData := range model.CacheQuotaData {
		copyQuotaData := *quotaData
		quotaDataSnapshot = append(quotaDataSnapshot, &copyQuotaData)
	}
	model.CacheQuotaDataLock.Unlock()
	require.Len(t, quotaDataSnapshot, 1)
	assert.Equal(t, 1, quotaDataSnapshot[0].Count)
	assert.Zero(t, quotaDataSnapshot[0].Quota)
	assert.Zero(t, quotaDataSnapshot[0].TokenUsed)

	var refundLog model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeRefund).Order("id DESC").First(&refundLog).Error)
	assert.Equal(t, preConsumedQuota, refundLog.Quota)
	assert.Equal(t, "doubao-seedance-2-0-260128", refundLog.ModelName)
}

func TestSeedanceNativeUpstreamErrorUsesARKEnvelopeE2E(t *testing.T) {
	setupSeedanceE2EDB(t)
	mock := &mockArkServer{
		submitStatus: http.StatusInternalServerError,
		submitResponse: `{
			"error":{
				"code":"InternalServiceError",
				"message":"The service encountered an unexpected internal error. Please retry later. Request ID: mock"
			}
		}`,
	}
	mockServer := httptest.NewServer(mock)
	t.Cleanup(mockServer.Close)
	seedSeedanceE2EData(t, mockServer.URL)
	engine := seedanceE2ERouter()

	status, responseBody := performJSONRequest(t, engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e", seedance20MultimodalRequestBody)

	require.Equal(t, http.StatusInternalServerError, status, string(responseBody))
	var response map[string]map[string]string
	require.NoError(t, common.Unmarshal(responseBody, &response))
	require.Equal(t, map[string]map[string]string{
		"error": {
			"code":    "InternalServiceError",
			"message": "The service encountered an unexpected internal error. Please retry later. Request ID: mock",
		},
	}, response)
	assert.Len(t, mock.snapshot(), 1)

	var taskCount int64
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&taskCount).Error)
	assert.Zero(t, taskCount)
	var refundedUser model.User
	var refundedChannel model.Channel
	var refundedToken model.Token
	require.NoError(t, model.DB.First(&refundedUser, e2eUserID).Error)
	require.NoError(t, model.DB.First(&refundedChannel, e2eChannelID).Error)
	require.NoError(t, model.DB.First(&refundedToken, 1).Error)
	assert.Equal(t, 2_000_000_000, refundedUser.Quota)
	assert.Zero(t, refundedUser.UsedQuota)
	assert.Zero(t, refundedUser.RequestCount)
	assert.Zero(t, refundedChannel.UsedQuota)
	assert.Zero(t, refundedToken.UsedQuota)
}

func TestSeedanceNativeRetriesNextChannelWithARKResponseE2E(t *testing.T) {
	setupSeedanceE2EDB(t)
	channelA := &mockArkServer{
		submitStatus: http.StatusInternalServerError,
		submitResponse: `{
			"error":{
				"code":"InternalServiceError",
				"message":"channel A unavailable"
			}
		}`,
	}
	channelAServer := httptest.NewServer(channelA)
	t.Cleanup(channelAServer.Close)
	channelB := &mockArkServer{taskID: "cgt-channel-b-success"}
	channelBServer := httptest.NewServer(channelB)
	t.Cleanup(channelBServer.Close)
	seedSeedanceE2EData(t, channelAServer.URL)
	seedSecondSeedanceE2EChannel(t, channelBServer.URL)

	originalRetryTimes := common.RetryTimes
	common.RetryTimes = 1
	t.Cleanup(func() { common.RetryTimes = originalRetryTimes })
	engine := seedanceE2ERouter()

	status, responseBody := performJSONRequest(t, engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e", seedance20MultimodalRequestBody)

	require.Equal(t, http.StatusOK, status, string(responseBody))
	var response map[string]interface{}
	require.NoError(t, common.Unmarshal(responseBody, &response))
	require.Len(t, response, 1)
	publicID, ok := response["id"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(publicID, "task_"))
	assert.NotContains(t, string(responseBody), "channel A unavailable")
	assert.NotContains(t, string(responseBody), "cgt-channel-b-success")
	assert.Len(t, channelA.snapshot(), 1)
	assert.Len(t, channelB.snapshot(), 1)

	var task model.Task
	require.NoError(t, model.DB.Where("task_id = ?", publicID).First(&task).Error)
	assert.Equal(t, 2, task.ChannelId)
	assert.Equal(t, "cgt-channel-b-success", task.PrivateData.UpstreamTaskID)
	assert.Equal(t, 15217, task.Quota)
	var billedUser model.User
	var billedChannelA model.Channel
	var billedChannelB model.Channel
	require.NoError(t, model.DB.First(&billedUser, e2eUserID).Error)
	require.NoError(t, model.DB.First(&billedChannelA, e2eChannelID).Error)
	require.NoError(t, model.DB.First(&billedChannelB, 2).Error)
	assert.Equal(t, 1, billedUser.RequestCount)
	assert.Equal(t, task.Quota, billedUser.UsedQuota)
	assert.Zero(t, billedChannelA.UsedQuota)
	assert.Equal(t, int64(task.Quota), billedChannelB.UsedQuota)
}

func TestSeedanceNativeReturnsLastARKErrorAfterAllChannelsFailE2E(t *testing.T) {
	setupSeedanceE2EDB(t)
	channelA := &mockArkServer{
		submitStatus:   http.StatusInternalServerError,
		submitResponse: `{"error":{"code":"InternalServiceError","message":"channel A unavailable"}}`,
	}
	channelAServer := httptest.NewServer(channelA)
	t.Cleanup(channelAServer.Close)
	channelB := &mockArkServer{
		submitStatus:   http.StatusServiceUnavailable,
		submitResponse: `{"error":{"code":"ServiceUnavailable","message":"channel B unavailable"}}`,
	}
	channelBServer := httptest.NewServer(channelB)
	t.Cleanup(channelBServer.Close)
	seedSeedanceE2EData(t, channelAServer.URL)
	seedSecondSeedanceE2EChannel(t, channelBServer.URL)

	originalRetryTimes := common.RetryTimes
	common.RetryTimes = 1
	t.Cleanup(func() { common.RetryTimes = originalRetryTimes })
	engine := seedanceE2ERouter()

	status, responseBody := performJSONRequest(t, engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e", seedance20MultimodalRequestBody)

	require.Equal(t, http.StatusServiceUnavailable, status, string(responseBody))
	var response map[string]map[string]string
	require.NoError(t, common.Unmarshal(responseBody, &response))
	require.Equal(t, map[string]map[string]string{
		"error": {
			"code":    "ServiceUnavailable",
			"message": "channel B unavailable",
		},
	}, response)
	assert.Len(t, channelA.snapshot(), 1)
	assert.Len(t, channelB.snapshot(), 1)

	var taskCount int64
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&taskCount).Error)
	assert.Zero(t, taskCount)
	var refundedUser model.User
	var refundedChannelA model.Channel
	var refundedChannelB model.Channel
	var refundedToken model.Token
	require.NoError(t, model.DB.First(&refundedUser, e2eUserID).Error)
	require.NoError(t, model.DB.First(&refundedChannelA, e2eChannelID).Error)
	require.NoError(t, model.DB.First(&refundedChannelB, 2).Error)
	require.NoError(t, model.DB.First(&refundedToken, 1).Error)
	assert.Equal(t, 2_000_000_000, refundedUser.Quota)
	assert.Zero(t, refundedUser.UsedQuota)
	assert.Zero(t, refundedUser.RequestCount)
	assert.Zero(t, refundedChannelA.UsedQuota)
	assert.Zero(t, refundedChannelB.UsedQuota)
	assert.Zero(t, refundedToken.UsedQuota)
}

func TestSeedanceNativeNetworkTimeoutUsesARKEnvelopeE2E(t *testing.T) {
	setupSeedanceE2EDB(t)
	releaseUpstream := make(chan struct{})
	timeoutServer := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-releaseUpstream
	}))
	t.Cleanup(timeoutServer.Close)
	t.Cleanup(func() { close(releaseUpstream) })
	seedSeedanceE2EData(t, timeoutServer.URL)
	originalRetryTimes := common.RetryTimes
	common.RetryTimes = 0
	t.Cleanup(func() { common.RetryTimes = originalRetryTimes })
	originalRelayTimeout := common.RelayTimeout
	common.RelayTimeout = 1
	service.InitHttpClient()
	t.Cleanup(func() {
		common.RelayTimeout = originalRelayTimeout
		service.InitHttpClient()
	})
	engine := seedanceE2ERouter()

	status, responseBody := performJSONRequest(t, engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e", seedance20MultimodalRequestBody)

	require.Equal(t, http.StatusInternalServerError, status, string(responseBody))
	var response map[string]map[string]string
	require.NoError(t, common.Unmarshal(responseBody, &response))
	require.Equal(t, map[string]map[string]string{
		"error": {
			"code":    "InternalServiceError",
			"message": "The service encountered an unexpected internal error. Please retry later.",
		},
	}, response)
	assert.NotContains(t, string(responseBody), "Client.Timeout")
	assert.NotContains(t, string(responseBody), timeoutServer.URL)

	var taskCount int64
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&taskCount).Error)
	assert.Zero(t, taskCount)
	var refundedUser model.User
	require.NoError(t, model.DB.First(&refundedUser, e2eUserID).Error)
	assert.Equal(t, 2_000_000_000, refundedUser.Quota)
	assert.Zero(t, refundedUser.UsedQuota)
}
