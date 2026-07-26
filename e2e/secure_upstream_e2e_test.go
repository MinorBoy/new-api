package e2e

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/videometa"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type secureE2ERequest struct {
	method        string
	path          string
	authorization string
	contentType   string
	body          []byte
}

type secureE2EMock struct {
	mu            sync.Mutex
	group         dto.SecureVideoGroup
	taskID        string
	videoURL      string
	requests      []secureE2ERequest
	pollResponses []string
	pollIndex     int
}

func (m *secureE2EMock) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	body, _ := io.ReadAll(request.Body)
	m.mu.Lock()
	m.requests = append(m.requests, secureE2ERequest{
		method:        request.Method,
		path:          request.URL.Path,
		authorization: request.Header.Get("Authorization"),
		contentType:   request.Header.Get("Content-Type"),
		body:          append([]byte(nil), body...),
	})
	submitPath, pollPath := "/api/generate-video", "/api/task/"+m.taskID
	if m.group == dto.SecureVideoGroupEnterprise {
		submitPath, pollPath = "/v1/videos", "/v1/videos/"+m.taskID
	}
	response := ""
	switch {
	case request.Method == http.MethodPost && request.URL.Path == submitPath:
		response = `{"task_id":"` + m.taskID + `","status":"queued"}`
	case request.Method == http.MethodGet && request.URL.Path == pollPath:
		if len(m.pollResponses) > 0 {
			index := min(m.pollIndex, len(m.pollResponses)-1)
			response = m.pollResponses[index]
			m.pollIndex++
		}
	}
	m.mu.Unlock()

	writer.Header().Set("Content-Type", "application/json")
	if response == "" {
		http.NotFound(writer, request)
		return
	}
	_, _ = writer.Write([]byte(response))
}

func (m *secureE2EMock) snapshot() []secureE2ERequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]secureE2ERequest(nil), m.requests...)
}

type secureE2EVideoMetadataClient struct{}

func (secureE2EVideoMetadataClient) Metadata(_ context.Context, _ string) (videometa.Metadata, error) {
	return videometa.Metadata{
		DurationMS: 6_000,
		Width:      1280,
		Height:     720,
		Container:  "mp4",
	}, nil
}

type secureE2EEnvironment struct {
	engine        *gin.Engine
	mock          *secureE2EMock
	upstreamModel string
	key           string
}

func setupSecureE2E(t *testing.T, group dto.SecureVideoGroup, upstreamModel string) *secureE2EEnvironment {
	t.Helper()
	setupSeedanceE2EDB(t)
	taskID := "secure-" + string(group) + "-private"
	videoURL := "https://assets.example/secure-" + string(group) + ".mp4?signature=playback"
	mock := &secureE2EMock{
		group:    group,
		taskID:   taskID,
		videoURL: videoURL,
		pollResponses: []string{
			`{"task_id":"` + taskID + `","status":"in_progress","progress":50}`,
			`{"task_id":"` + taskID + `","status":"completed","video_url":"` + videoURL + `"}`,
		},
	}
	server := httptest.NewServer(mock)
	t.Cleanup(server.Close)
	seedSeedanceE2EData(t, server.URL)

	channel, err := model.GetChannelById(e2eChannelID, true)
	require.NoError(t, err)
	key := "secure-" + string(group) + "-key"
	mapping := `{"client-video":"` + upstreamModel + `"}`
	channel.Type = constant.ChannelTypeSecure
	channel.Key = key
	channel.Models = "client-video"
	channel.ModelMapping = &mapping
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		DisableTaskPollingSleep: true,
		SecureVideoGroup:        group,
	})
	require.NoError(t, channel.Update())

	ratios := ratio_setting.GetModelRatioCopy()
	ratios["client-video"] = 0.1
	encoded, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(encoded)))

	service.SetVideoMetadataClient(secureE2EVideoMetadataClient{})
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		return relay.GetTaskAdaptor(platform)
	}
	t.Cleanup(func() {
		service.SetVideoMetadataClient(nil)
		service.GetTaskAdaptorFunc = nil
	})
	return &secureE2EEnvironment{
		engine:        seedanceE2ERouter(),
		mock:          mock,
		upstreamModel: upstreamModel,
		key:           key,
	}
}

func TestSecureGroupedARKLifecyclesE2E(t *testing.T) {
	tests := []struct {
		name          string
		group         dto.SecureVideoGroup
		upstreamModel string
		requestBody   string
		submitPath    string
		pollPath      string
		wantMultipart [][2]string
		wantJSON      string
	}{
		{
			name:          "discount",
			group:         dto.SecureVideoGroupDiscount,
			upstreamModel: "video-2.0-pro",
			requestBody: `{"model":"client-video","content":[
				{"type":"text","text":"discount product shot"},
				{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/first.jpg"}},
				{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/second.jpg"}}
			],"duration":8,"ratio":"16:9","resolution":"720p"}`,
			submitPath: "/api/generate-video",
			pollPath:   "/api/task/secure-discount-private",
			wantMultipart: [][2]string{
				{"model", "video-2.0-pro"},
				{"prompt", "discount product shot"},
				{"duration", "8"},
				{"ratio", "16:9"},
				{"resolution", "720p"},
				{"files", "https://8.8.8.8/first.jpg"},
				{"files", "https://8.8.8.8/second.jpg"},
			},
		},
		{
			name:          "overseas",
			group:         dto.SecureVideoGroupOverseas,
			upstreamModel: "video-2.0-fast",
			requestBody: `{"model":"client-video","content":[
				{"type":"text","text":"overseas omni"},
				{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.jpg"}},
				{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/ref.mp4"}},
				{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/ref.mp3"}}
			],"duration":8,"ratio":"16:9","resolution":"720p"}`,
			submitPath: "/api/generate-video",
			pollPath:   "/api/task/secure-overseas-private",
			wantMultipart: [][2]string{
				{"model", "video-2.0-fast"},
				{"prompt", "overseas omni @image_file_1 @video_file_1 @audio_file_1"},
				{"duration", "8"},
				{"ratio", "16:9"},
				{"resolution", "720p"},
				{"functionMode", "omni_reference"},
				{"image_file_1", "https://8.8.8.8/ref.jpg"},
				{"video_file_1", "https://8.8.8.8/ref.mp4"},
				{"audio_file_1", "https://8.8.8.8/ref.mp3"},
			},
		},
		{
			name:          "enterprise",
			group:         dto.SecureVideoGroupEnterprise,
			upstreamModel: "video-2.0-pro",
			requestBody:   `{"model":"client-video","content":[{"type":"text","text":"enterprise text"}],"duration":8,"ratio":"16:9","resolution":"720p"}`,
			submitPath:    "/v1/videos",
			pollPath:      "/v1/videos/secure-enterprise-private",
			wantJSON:      `{"model":"video-2.0-pro","prompt":"enterprise text","duration":8,"aspect_ratio":"16:9"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := setupSecureE2E(t, test.group, test.upstreamModel)
			status, submit := performJSONRequest(
				t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", test.requestBody,
			)
			require.Equal(t, http.StatusOK, status, string(submit))
			var submitResponse map[string]any
			require.NoError(t, common.Unmarshal(submit, &submitResponse))
			require.Len(t, submitResponse, 1)
			publicID, ok := submitResponse["id"].(string)
			require.True(t, ok)
			assert.True(t, strings.HasPrefix(publicID, "task_"))
			assertSecureE2EPublicBody(t, submit, env)

			requests := env.mock.snapshot()
			require.Len(t, requests, 1)
			assert.Equal(t, http.MethodPost, requests[0].method)
			assert.Equal(t, test.submitPath, requests[0].path)
			assert.Equal(t, "Bearer "+env.key, requests[0].authorization)
			if test.wantJSON != "" {
				assert.Equal(t, "application/json", requests[0].contentType)
				assert.JSONEq(t, test.wantJSON, string(requests[0].body))
			} else {
				assert.Equal(t, test.wantMultipart, secureE2EMultipartFields(t, requests[0]))
			}

			task := pollNewAPIVideoTask(t, publicID)
			assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), task.Status)
			task = pollNewAPIVideoTask(t, publicID)
			assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
			assert.Equal(t, env.mock.taskID, task.PrivateData.UpstreamTaskID)
			assert.Equal(t, env.mock.videoURL, task.PrivateData.ResultURL)

			requests = env.mock.snapshot()
			require.Len(t, requests, 3)
			assert.Equal(t, test.pollPath, requests[1].path)
			assert.Equal(t, test.pollPath, requests[2].path)

			status, single := performJSONRequest(
				t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+publicID, "Bearer e2e-1", "",
			)
			require.Equal(t, http.StatusOK, status, string(single))
			assertSecureE2ESucceededTask(t, single, publicID, env)

			status, list := performJSONRequest(
				t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks?filter.task_ids="+publicID+"&page_size=20", "Bearer e2e-1", "",
			)
			require.Equal(t, http.StatusOK, status, string(list))
			assertSecureE2EPublicBody(t, list, env)
			assert.Contains(t, string(list), publicID)
			assert.Contains(t, string(list), env.mock.videoURL)
		})
	}
}

func TestSecureGroupedFailuresRefundE2E(t *testing.T) {
	tests := []struct {
		name          string
		group         dto.SecureVideoGroup
		upstreamModel string
		requestBody   string
	}{
		{
			name:          "discount",
			group:         dto.SecureVideoGroupDiscount,
			upstreamModel: "video-2.0-pro",
			requestBody:   `{"model":"client-video","content":[{"type":"text","text":"refund"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.jpg"}}],"duration":8,"ratio":"16:9","resolution":"720p"}`,
		},
		{
			name:          "overseas",
			group:         dto.SecureVideoGroupOverseas,
			upstreamModel: "video-2.0-fast",
			requestBody:   `{"model":"client-video","content":[{"type":"text","text":"refund"}],"duration":8,"ratio":"16:9","resolution":"720p"}`,
		},
		{
			name:          "enterprise",
			group:         dto.SecureVideoGroupEnterprise,
			upstreamModel: "video-2.0-pro",
			requestBody:   `{"model":"client-video","content":[{"type":"text","text":"refund"}],"duration":8,"ratio":"16:9","resolution":"720p"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := setupSecureE2E(t, test.group, test.upstreamModel)
			failureCode := "secure_" + test.name + "_failed"
			env.mock.mu.Lock()
			env.mock.pollResponses = []string{`{"task_id":"` + env.mock.taskID + `","status":"failed","error":{"code":"` + failureCode + `","message":"generation failed"}}`}
			env.mock.mu.Unlock()

			var beforeUser model.User
			var beforeToken model.Token
			var beforeChannel model.Channel
			require.NoError(t, model.DB.First(&beforeUser, e2eUserID).Error)
			require.NoError(t, model.DB.First(&beforeToken, 1).Error)
			require.NoError(t, model.DB.First(&beforeChannel, e2eChannelID).Error)

			status, submit := performJSONRequest(
				t, env.engine, http.MethodPost, "/api/v3/contents/generations/tasks", "Bearer e2e-1", test.requestBody,
			)
			require.Equal(t, http.StatusOK, status, string(submit))
			var submitResponse map[string]any
			require.NoError(t, common.Unmarshal(submit, &submitResponse))
			publicID, ok := submitResponse["id"].(string)
			require.True(t, ok)

			var task model.Task
			require.NoError(t, model.DB.Where("task_id = ?", publicID).First(&task).Error)
			preConsumedQuota := task.Quota
			require.Positive(t, preConsumedQuota)
			task = pollNewAPIVideoTask(t, publicID)
			assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), task.Status)
			assert.Zero(t, task.Quota)

			status, failed := performJSONRequest(
				t, env.engine, http.MethodGet, "/api/v3/contents/generations/tasks/"+publicID, "Bearer e2e-1", "",
			)
			require.Equal(t, http.StatusOK, status, string(failed))
			assertSecureE2EPublicBody(t, failed, env)
			assert.Contains(t, string(failed), `"status":"failed"`)
			assert.Contains(t, string(failed), `"code":"`+failureCode+`"`)

			var afterUser model.User
			var afterToken model.Token
			var afterChannel model.Channel
			require.NoError(t, model.DB.First(&afterUser, e2eUserID).Error)
			require.NoError(t, model.DB.First(&afterToken, 1).Error)
			require.NoError(t, model.DB.First(&afterChannel, e2eChannelID).Error)
			assert.Equal(t, beforeUser.Quota, afterUser.Quota)
			assert.Equal(t, beforeUser.UsedQuota, afterUser.UsedQuota)
			assert.Equal(t, beforeToken.UsedQuota, afterToken.UsedQuota)
			assert.Equal(t, beforeChannel.UsedQuota, afterChannel.UsedQuota)

			var refundLog model.Log
			require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeRefund).Order("id DESC").First(&refundLog).Error)
			assert.Equal(t, preConsumedQuota, refundLog.Quota)
		})
	}
}

func secureE2EMultipartFields(t *testing.T, request secureE2ERequest) [][2]string {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(request.contentType)
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mediaType)
	reader := multipart.NewReader(bytes.NewReader(request.body), params["boundary"])
	fields := make([][2]string, 0)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		value, err := io.ReadAll(part)
		require.NoError(t, err)
		fields = append(fields, [2]string{part.FormName(), string(value)})
		require.NoError(t, part.Close())
	}
	return fields
}

func assertSecureE2ESucceededTask(t *testing.T, body []byte, publicID string, env *secureE2EEnvironment) {
	t.Helper()
	assertSecureE2EPublicBody(t, body, env)
	var response map[string]any
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, publicID, response["id"])
	assert.Equal(t, "client-video", response["model"])
	assert.Equal(t, "succeeded", response["status"])
	assert.Equal(t, map[string]any{"video_url": env.mock.videoURL}, response["content"])
}

func assertSecureE2EPublicBody(t *testing.T, body []byte, env *secureE2EEnvironment) {
	t.Helper()
	for _, privateValue := range []string{
		env.mock.taskID,
		env.upstreamModel,
		env.key,
		"secure_video_group",
		"user_id",
		"channel_id",
		`"group"`,
		`"quota"`,
		`"platform"`,
		"upstream_model_name",
	} {
		assert.NotContains(t, string(body), privateValue)
	}
}
