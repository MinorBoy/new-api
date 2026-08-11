package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	ffLinkE2EClientModel = "doubao-seedance-2-0-260128"
	ffLinkE2EKey         = "mock-fflink-key"
)

type ffLinkE2ERequest struct {
	Method        string
	Path          string
	Authorization string
	Prefer        string
	Range         string
	Body          []byte
}

type ffLinkE2EMock struct {
	mu            sync.Mutex
	jobID         string
	pollResponses []string
	pollIndex     int
	requests      []ffLinkE2ERequest
}

func (m *ffLinkE2EMock) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	body, _ := io.ReadAll(request.Body)
	m.mu.Lock()
	m.requests = append(m.requests, ffLinkE2ERequest{
		Method: request.Method, Path: request.URL.Path,
		Authorization: request.Header.Get("Authorization"), Prefer: request.Header.Get("Prefer"),
		Range: request.Header.Get("Range"), Body: append([]byte(nil), body...),
	})
	jobID := m.jobID
	response := ""
	switch {
	case request.Method == http.MethodPost && request.URL.Path == "/v1/videos/generations":
		response = fmt.Sprintf(`{"job_id":%q,"status":"pending","provider_body":"private-submit"}`, jobID)
	case request.Method == http.MethodGet && request.URL.Path == "/v1/videos/jobs/"+jobID:
		if len(m.pollResponses) > 0 {
			index := min(m.pollIndex, len(m.pollResponses)-1)
			response = m.pollResponses[index]
			m.pollIndex++
		}
	case request.Method == http.MethodGet && request.URL.Path == "/v1/videos/jobs/"+jobID+"/content":
		m.mu.Unlock()
		writer.Header().Set("Content-Type", "video/mp4")
		writer.Header().Set("Content-Range", "bytes 10-19/100")
		writer.Header().Set("Accept-Ranges", "bytes")
		writer.WriteHeader(http.StatusPartialContent)
		_, _ = writer.Write([]byte("partial-video"))
		return
	case request.Method == http.MethodDelete && request.URL.Path == "/v1/videos/jobs/"+jobID:
		m.mu.Unlock()
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	m.mu.Unlock()

	writer.Header().Set("Content-Type", "application/json")
	if response == "" {
		http.NotFound(writer, request)
		return
	}
	if request.Method == http.MethodPost {
		writer.WriteHeader(http.StatusAccepted)
	}
	_, _ = writer.Write([]byte(response))
}

func (m *ffLinkE2EMock) snapshot() []ffLinkE2ERequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	requests := make([]ffLinkE2ERequest, len(m.requests))
	copy(requests, m.requests)
	return requests
}

type ffLinkE2EEnvironment struct {
	engine *gin.Engine
	mock   *ffLinkE2EMock
	server *httptest.Server
}

func setupFFLinkE2E(t *testing.T, jobID string, pollResponses ...string) *ffLinkE2EEnvironment {
	t.Helper()
	require.NoError(t, i18n.Init())
	setupSeedanceE2EDB(t)
	setupSeedanceE2EVideoMetadata(t)
	mock := &ffLinkE2EMock{jobID: jobID, pollResponses: pollResponses}
	server := httptest.NewServer(mock)
	t.Cleanup(server.Close)
	seedSeedanceE2EData(t, server.URL)

	channel, err := model.GetChannelById(e2eChannelID, true)
	require.NoError(t, err)
	mapping := `{"` + ffLinkE2EClientModel + `":"seedance-2.0"}`
	channel.Type = constant.ChannelTypeFFLink
	channel.Key = ffLinkE2EKey
	channel.Name = "fflink-e2e-mock"
	channel.Models = ffLinkE2EClientModel
	channel.ModelMapping = &mapping
	// Keep the pre-acceptance gate visible in the fixture. The test route below
	// injects this selected channel directly, while normal distributor routing
	// must continue to reject it until real FYLink acceptance.
	channel.Status = common.ChannelStatusManuallyDisabled
	require.NoError(t, channel.Update())
	// The direct route injector still needs a selectable ability. This is test
	// scaffolding only; the production distributor checks channel status before
	// selecting a channel.
	require.NoError(t, model.UpdateAbilityStatus(e2eChannelID, true))

	ratios := ratio_setting.GetModelRatioCopy()
	ratios[ffLinkE2EClientModel] = 0.1
	encoded, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(encoded)))

	previousFactory := service.GetTaskAdaptorFunc
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		return relay.GetTaskAdaptor(platform)
	}
	t.Cleanup(func() { service.GetTaskAdaptorFunc = previousFactory })
	return &ffLinkE2EEnvironment{engine: ffLinkE2ERouter(), mock: mock, server: server}
}

// ffLinkE2ERouter preserves the production auth, request conversion, task
// handlers, and content proxy while injecting the selected pre-acceptance
// channel through the test fixture's enabled ability. The real distributor is
// intentionally omitted here because it must reject manually disabled FYLink.
func ffLinkE2ERouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	seedance := engine.Group("/api/v3/contents/generations")
	seedance.Use(middleware.RouteTag("relay"), middleware.SeedanceRequestConvert(), middleware.VideoRequestPolicy(), middleware.TokenAuth())
	{
		seedance.POST("/tasks", ffLinkSelectChannel, controller.RelayTask)
		seedance.GET("/tasks", controller.RelaySeedanceTaskFetch)
		seedance.GET("/tasks/:task_id", controller.RelaySeedanceTaskFetch)
		seedance.DELETE("/tasks/:task_id", controller.RelaySeedanceTaskCancel)
	}
	video := engine.Group("/v1")
	video.Use(middleware.RouteTag("relay"), middleware.TokenOrUserAuth())
	video.GET("/videos/:task_id/content", controller.VideoProxy)
	return engine
}

func ffLinkSelectChannel(c *gin.Context) {
	channel, err := model.GetChannelById(e2eChannelID, true)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	modelName := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	if modelName == "" {
		modelName = ffLinkE2EClientModel
	}
	if setupErr := middleware.SetupContextForSelectedChannel(c, channel, modelName); setupErr != nil {
		c.AbortWithStatusJSON(setupErr.StatusCode, gin.H{"error": setupErr.Error()})
		return
	}
	c.Set("platform", strconv.Itoa(constant.ChannelTypeFFLink))
	c.Next()
}

func TestFFLinkArkLifecycleAndContentProxyE2E(t *testing.T) {
	env := setupFFLinkE2E(t, "job_1",
		`{"job_id":"job_1","status":"running","progress":35}`,
		`{"job_id":"job_1","status":"settling","progress":80}`,
		`{"job_id":"job_1","status":"completed","duration":8,"resolution":"720p","provider_body":"private-terminal"}`,
	)
	beforeBilling := ffLinkBillingSnapshot(t)
	requestBody := `{
		"model":"doubao-seedance-2-0-260128",
		"content":[
			{"type":"text","text":"FYLink public URL acceptance"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/reference.png"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://1.1.1.1/reference.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://9.9.9.9/reference.mp3"}}
		],
		"duration":8,
		"ratio":"16:9",
		"resolution":"720p",
		"generate_audio":true
	}`

	status, submit := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", requestBody)
	require.Equal(t, http.StatusOK, status, string(submit))
	var submitted struct {
		ID string `json:"id"`
	}
	require.NoError(t, common.Unmarshal(submit, &submitted))
	require.True(t, strings.HasPrefix(submitted.ID, "task_"))
	assertFFLinkPublicBody(t, submit)

	requests := env.mock.snapshot()
	require.Len(t, requests, 1)
	assert.Equal(t, http.MethodPost, requests[0].Method)
	assert.Equal(t, "/v1/videos/generations", requests[0].Path)
	assert.Equal(t, "Bearer "+ffLinkE2EKey, requests[0].Authorization)
	assert.Equal(t, "respond-async", requests[0].Prefer)
	assert.JSONEq(t, `{
		"model":"seedance-2.0",
		"prompt":"FYLink public URL acceptance",
		"resolution":"720p",
		"duration":8,
		"aspect_ratio":"16:9",
		"audio":true,
		"guidances":{
			"image_reference":[{"image":{"url":"https://8.8.8.8/reference.png","type":"UPLOADED"}}],
			"video_reference_base":[{"video":{"url":"https://1.1.1.1/reference.mp4","type":"UPLOADED"}}],
			"audio_reference":[{"audio":{"url":"https://9.9.9.9/reference.mp3","type":"UPLOADED"}}]
		}
	}`, string(requests[0].Body))

	var task model.Task
	require.NoError(t, model.DB.Where("task_id = ?", submitted.ID).First(&task).Error)
	require.Positive(t, task.Quota)
	assert.Equal(t, constant.TaskPlatform("214"), task.Platform)
	assert.Equal(t, "job_1", task.PrivateData.UpstreamTaskID)
	assert.Equal(t, ffLinkE2EKey, task.PrivateData.Key)

	task = pollNewAPIVideoTask(t, submitted.ID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), task.Status)
	task = pollNewAPIVideoTask(t, submitted.ID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), task.Status)
	task = pollNewAPIVideoTask(t, submitted.ID)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
	assert.Contains(t, task.PrivateData.ResultURL, "/v1/videos/"+submitted.ID+"/content")
	afterBilling := ffLinkBillingSnapshot(t, submitted.ID)
	assert.Equal(t, beforeBilling.UserQuota+beforeBilling.UserUsedQuota, afterBilling.UserQuota+afterBilling.UserUsedQuota)
	assert.Equal(t, int64(task.Quota), afterBilling.ChannelUsedQuota-beforeBilling.ChannelUsedQuota)
	assert.Equal(t, task.Quota, afterBilling.TokenUsedQuota-beforeBilling.TokenUsedQuota)
	assert.Equal(t, task.Quota, afterBilling.QuotaDataQuota-beforeBilling.QuotaDataQuota)
	assert.Equal(t, 0, afterBilling.RefundLogCount-beforeBilling.RefundLogCount)
	assert.GreaterOrEqual(t, afterBilling.ConsumeLogCount-beforeBilling.ConsumeLogCount, 1)
	assert.GreaterOrEqual(t, afterBilling.LogCount-beforeBilling.LogCount, int64(1))

	// Terminal polling is idempotent: it must not settle or consume a second time.
	service.RunTaskPollingOnce(context.Background(), nil)
	service.RunTaskPollingOnce(context.Background(), nil)
	repeatedBilling := ffLinkBillingSnapshot(t, submitted.ID)
	assert.Equal(t, afterBilling, repeatedBilling)

	status, single := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+submitted.ID, "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(single))
	assertFFLinkSucceededTask(t, single, submitted.ID)

	status, list := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks?filter.task_ids="+submitted.ID+"&page_size=20", "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(list))
	assertFFLinkPublicBody(t, list)
	assert.Contains(t, string(list), submitted.ID)

	fetchSetting := system_setting.GetFetchSetting()
	originalFetchSetting := *fetchSetting
	serverURL, err := url.Parse(env.server.URL)
	require.NoError(t, err)
	fetchSetting.EnableSSRFProtection = true
	fetchSetting.AllowPrivateIp = true
	fetchSetting.AllowedPorts = []string{serverURL.Port()}
	service.InitHttpClient()
	t.Cleanup(func() {
		*fetchSetting = originalFetchSetting
		service.InitHttpClient()
	})

	contentRequest := httptest.NewRequest(http.MethodGet, "/v1/videos/"+submitted.ID+"/content", nil)
	contentRequest.Header.Set("Authorization", "Bearer e2e-1")
	contentRequest.Header.Set("Range", "bytes=10-19")
	contentRecorder := httptest.NewRecorder()
	env.engine.ServeHTTP(contentRecorder, contentRequest)
	require.Equal(t, http.StatusPartialContent, contentRecorder.Code, contentRecorder.Body.String())
	assert.Equal(t, "bytes 10-19/100", contentRecorder.Header().Get("Content-Range"))
	assert.Equal(t, "bytes", contentRecorder.Header().Get("Accept-Ranges"))
	assert.Equal(t, "partial-video", contentRecorder.Body.String())

	requests = env.mock.snapshot()
	require.Len(t, requests, 5)
	contentUpstream := requests[4]
	assert.Equal(t, http.MethodGet, contentUpstream.Method)
	assert.Equal(t, "/v1/videos/jobs/job_1/content", contentUpstream.Path)
	assert.Equal(t, "Bearer "+ffLinkE2EKey, contentUpstream.Authorization)
	assert.Equal(t, "bytes=10-19", contentUpstream.Range)
	assertFFLinkPublicBody(t, contentRecorder.Body.Bytes())
}

func TestFFLinkManualDisableBlocksNormalDistributorE2E(t *testing.T) {
	env := setupFFLinkE2E(t, "job_disabled")
	status, response := performJSONRequest(t, seedanceE2ERouter(), http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"disabled"}],"duration":8,"resolution":"720p"}`)
	assert.Equal(t, http.StatusForbidden, status, string(response))
	assert.Empty(t, env.mock.snapshot())
	var channel model.Channel
	require.NoError(t, model.DB.First(&channel, e2eChannelID).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, channel.Status)
}

func TestFFLinkCancelRefundsExactlyOnceE2E(t *testing.T) {
	env := setupFFLinkE2E(t, "job_2")
	requestBody := `{
		"model":"doubao-seedance-2-0-260128",
		"content":[
			{"type":"text","text":"FYLink frame cancellation"},
			{"type":"image_url","role":"first_frame","image_url":{"url":"https://8.8.8.8/first.png"}},
			{"type":"image_url","role":"last_frame","image_url":{"url":"https://8.8.4.4/last.png"}}
		],
		"duration":8,
		"resolution":"720p"
	}`
	var before model.User
	require.NoError(t, model.DB.First(&before, e2eUserID).Error)
	status, submit := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", requestBody)
	require.Equal(t, http.StatusOK, status, string(submit))
	var submitted struct {
		ID string `json:"id"`
	}
	require.NoError(t, common.Unmarshal(submit, &submitted))

	requests := env.mock.snapshot()
	require.Len(t, requests, 1)
	assert.JSONEq(t, `{
		"model":"seedance-2.0",
		"prompt":"FYLink frame cancellation",
		"resolution":"720p",
		"duration":8,
		"start_frame_url":"https://8.8.8.8/first.png",
		"end_frame_url":"https://8.8.4.4/last.png"
	}`, string(requests[0].Body))

	var pending model.Task
	require.NoError(t, model.DB.Where("task_id = ?", submitted.ID).First(&pending).Error)
	require.Positive(t, pending.Quota)
	var afterSubmit model.User
	require.NoError(t, model.DB.First(&afterSubmit, e2eUserID).Error)
	assert.Equal(t, before.Quota-pending.Quota, afterSubmit.Quota)
	assert.Equal(t, before.UsedQuota+pending.Quota, afterSubmit.UsedQuota)

	status, canceled := performJSONRequest(t, env.engine, http.MethodDelete, "/api/v3/contents/generations/tasks/"+submitted.ID, "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(canceled))
	assertFFLinkPublicBody(t, canceled)
	require.NoError(t, model.DB.Where("task_id = ?", submitted.ID).First(&pending).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), pending.Status)
	assert.Zero(t, pending.Quota)

	var afterCancel model.User
	require.NoError(t, model.DB.First(&afterCancel, e2eUserID).Error)
	assert.Equal(t, before.Quota, afterCancel.Quota)
	assert.Equal(t, before.UsedQuota, afterCancel.UsedQuota)
	var refundCount int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeRefund).Count(&refundCount).Error)
	assert.Equal(t, int64(1), refundCount)

	status, second := performJSONRequest(t, env.engine, http.MethodDelete, "/api/v3/contents/generations/tasks/"+submitted.ID, "Bearer e2e-1", "")
	assert.Equal(t, http.StatusConflict, status, string(second))
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeRefund).Count(&refundCount).Error)
	assert.Equal(t, int64(1), refundCount)
	requests = env.mock.snapshot()
	require.Len(t, requests, 2)
	assert.Equal(t, http.MethodDelete, requests[1].Method)
	assert.Equal(t, "/v1/videos/jobs/job_2", requests[1].Path)
	assert.Equal(t, "Bearer "+ffLinkE2EKey, requests[1].Authorization)
}

func TestFFLinkFailedTaskRefundsExactlyOnceE2E(t *testing.T) {
	env := setupFFLinkE2E(t, "job_failed",
		`{"job_id":"job_failed","status":"failed","error":{"code":"provider_rejected","message":"job_failed rejected mock-fflink-key private-upstream-body"}}`,
	)
	var before model.User
	require.NoError(t, model.DB.First(&before, e2eUserID).Error)
	status, submit := performJSONRequest(t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"FYLink failed refund"}],"duration":8,"resolution":"720p"}`)
	require.Equal(t, http.StatusOK, status, string(submit))
	var submitted struct {
		ID string `json:"id"`
	}
	require.NoError(t, common.Unmarshal(submit, &submitted))

	service.RunTaskPollingOnce(context.Background(), nil)
	service.RunTaskPollingOnce(context.Background(), nil)
	var failed model.Task
	require.NoError(t, model.DB.Where("task_id = ?", submitted.ID).First(&failed).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), failed.Status)
	assert.Zero(t, failed.Quota)

	status, public := performJSONRequest(t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+submitted.ID, "Bearer e2e-1", "")
	require.Equal(t, http.StatusOK, status, string(public))
	assert.Contains(t, string(public), `"status":"failed"`)
	assertFFLinkPublicBody(t, public)

	var after model.User
	require.NoError(t, model.DB.First(&after, e2eUserID).Error)
	assert.Equal(t, before.Quota, after.Quota)
	assert.Equal(t, before.UsedQuota, after.UsedQuota)
	var refundCount int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeRefund).Count(&refundCount).Error)
	assert.Equal(t, int64(1), refundCount)
}

func assertFFLinkSucceededTask(t *testing.T, body []byte, publicID string) {
	t.Helper()
	assertFFLinkPublicBody(t, body)
	var response map[string]any
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, publicID, response["id"])
	assert.Equal(t, ffLinkE2EClientModel, response["model"])
	assert.Equal(t, "succeeded", response["status"])
	content, ok := response["content"].(map[string]any)
	require.True(t, ok)
	videoURL, ok := content["video_url"].(string)
	require.True(t, ok)
	assert.Contains(t, videoURL, "/v1/videos/"+publicID+"/content")
}

func assertFFLinkPublicBody(t *testing.T, body []byte) {
	t.Helper()
	for _, privateValue := range []string{
		"job_1", "job_2", "job_failed", ffLinkE2EKey, "seedance-2.0", "private-submit", "private-terminal",
		"user_id", "channel_id", `"group"`, `"quota"`, `"platform"`, `"properties"`, "upstream_model_name",
	} {
		assert.NotContains(t, string(body), privateValue)
	}
}

func ffLinkBillingSnapshot(t *testing.T, taskIDs ...string) seedanceBillingDomainSnapshot {
	t.Helper()
	return seedanceBillingDomainSnapshotFor(t, &seedanceBillingE2EEnv{
		User:    &model.User{Id: e2eUserID},
		Token:   &model.Token{Id: 1},
		Channel: &model.Channel{Id: e2eChannelID},
	}, taskIDs...)
}
