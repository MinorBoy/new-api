package clmmmall

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskAdaptorValidatesNativeArkRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	validBody := `{"model":"ark-model","content":[{"type":"text","text":"a prompt"}]}`
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	c := newArkContext(validBody, true)

	require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info))
	assert.Equal(t, constant.TaskActionGenerate, info.Action)
	stored, err := relaycommon.GetTaskRequest(c)
	require.NoError(t, err)
	assert.Equal(t, "ark-model", stored.Model)
	assert.Equal(t, "a prompt", stored.Prompt)
	assert.Equal(t, 5, stored.Duration)
	assert.Equal(t, "480p", stored.Metadata["resolution"])

	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(newArkContext(validBody, false), &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}})
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)

	unknown := `{"model":"ark-model","content":[{"type":"text","text":"a prompt"}],"private_option":true}`
	taskErr = (&TaskAdaptor{}).ValidateRequestAndSetAction(newArkContext(unknown, true), &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}})
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	assert.Contains(t, taskErr.Message, "private_option")

	knownUnsupported := `{"model":"ark-model","content":[{"type":"text","text":"a prompt"}],"watermark":false}`
	taskErr = (&TaskAdaptor{}).ValidateRequestAndSetAction(newArkContext(knownUnsupported, true), &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}})
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
}

func TestTaskAdaptorValidatesMappedModelAfterMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newArkContext(`{"model":"ark-model","duration":3,"content":[{"type":"text","text":"a prompt"}]}`, true)
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info))

	info.ChannelMeta = &relaycommon.ChannelMeta{UpstreamModelName: "ME-videos-720P-10s"}
	require.Nil(t, (&TaskAdaptor{}).ValidateBillingRequest(c, info))
	stored, err := relaycommon.GetTaskRequest(c)
	require.NoError(t, err)
	assert.Equal(t, 3, stored.Duration)
	assert.Equal(t, "720p", stored.Metadata["resolution"])

	for _, modelName := range []string{"unknown-video", "op-video-gz", "sh-video-2s"} {
		t.Run(modelName, func(t *testing.T) {
			badInfo := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: modelName}}
			taskErr := (&TaskAdaptor{}).ValidateBillingRequest(c, badInfo)
			require.NotNil(t, taskErr)
			assert.Equal(t, "invalid_model", taskErr.Code)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
		})
	}
}

func TestTaskAdaptorSubmitsExactClmmRequestAndReturnsPublicID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
	var requestBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/videos", r.URL.Path)
		assert.Equal(t, "Bearer upstream-secret", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		var err error
		requestBody, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"task_id":"upstream-private","id":"fallback-private","status":"queued"}`))
	}))
	defer server.Close()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", bytes.NewBufferString(`{"model":"ark-model","ratio":"9:16","resolution":"480p","duration":3,"content":[{"type":"text","text":"line one"},{"type":"text","text":"line two"},{"type":"image_url","role":"last_frame","image_url":{"url":"https://example.com/ref.png"}}]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.KeySeedanceOfficialAPI, true)
	info := &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeClmmMall, ChannelBaseUrl: server.URL, ApiKey: "upstream-secret", UpstreamModelName: "me-videos-720P-10s"},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	require.Nil(t, adaptor.ValidateBillingRequest(c, info))
	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	resp, err := adaptor.DoRequest(c, info, body)
	require.NoError(t, err)
	upstreamID, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "upstream-private", upstreamID)
	assert.JSONEq(t, `{"task_id":"upstream-private","id":"fallback-private","status":"queued"}`, string(taskData))
	assert.JSONEq(t, `{"id":"task_public"}`, recorder.Body.String())
	assert.NotContains(t, recorder.Body.String(), "upstream-private")
	assert.JSONEq(t, `{"model":"me-videos-720P-10s","prompt":"line one\nline two","aspect_ratio":"9:16","resolution":"720p","size":"720x1280","seconds":"1","mySeconds":"3","reference_image_urls":["https://example.com/ref.png"]}`, string(requestBody))
}

func TestTaskAdaptorSubmitResponseUsesTaskIDThenID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		body       string
		expectedID string
		wantError  bool
	}{
		{name: "task id preferred", body: `{"task_id":"primary-private","id":"fallback-private"}`, expectedID: "primary-private"},
		{name: "id fallback", body: `{"id":"fallback-private"}`, expectedID: "fallback-private"},
		{name: "missing id", body: `{"status":"queued"}`, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(test.body))}
			upstreamID, taskData, taskErr := (&TaskAdaptor{}).DoResponse(c, resp, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}})
			assert.JSONEq(t, test.body, string(taskData))
			if test.wantError {
				require.NotNil(t, taskErr)
				assert.Empty(t, upstreamID)
				return
			}
			require.Nil(t, taskErr)
			assert.Equal(t, test.expectedID, upstreamID)
			assert.JSONEq(t, `{"id":"task_public"}`, recorder.Body.String())
		})
	}
}

func TestTaskAdaptorFetchesEscapedPrivateID(t *testing.T) {
	var requestURI string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestURI = r.RequestURI
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "Bearer upstream-secret", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		_, _ = w.Write([]byte(`{"status":"queued"}`))
	}))
	defer server.Close()

	resp, err := (&TaskAdaptor{}).FetchTask(server.URL, "upstream-secret", map[string]any{"task_id": "private/id with space"}, "")
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, "/v1/videos/private%2Fid%20with%20space", requestURI)
}

func TestTaskAdaptorFetchTaskKeepsTemporaryHTTPFailuresRetryable(t *testing.T) {
	service.InitHttpClient()
	for _, statusCode := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(statusCode)
				_, _ = w.Write([]byte(`{"status":"error","error":"temporary private diagnostic"}`))
			}))
			defer server.Close()

			response, err := (&TaskAdaptor{}).FetchTask(server.URL, "upstream-secret", map[string]any{"task_id": "private-task"}, "")

			require.Error(t, err)
			assert.Nil(t, response)
			assert.NotContains(t, err.Error(), "temporary private diagnostic")
			assert.NotContains(t, err.Error(), "upstream-secret")
			assert.NotContains(t, err.Error(), "private-task")
		})
	}
}

func TestTaskAdaptorParseTaskErrorUsesStableMappings(t *testing.T) {
	adaptor := &TaskAdaptor{}
	for _, test := range []struct {
		name, body, code, message  string
		responseStatus, taskStatus int
	}{
		{
			name:           "client error",
			responseStatus: http.StatusBadRequest,
			taskStatus:     http.StatusBadRequest,
			body:           `{"error":{"code":"upstream-private-id","message":"Authorization: Bearer fake-upstream-secret diagnostic=private"}}`,
			code:           "invalid_request",
			message:        "CLMM Mall rejected the request",
		},
		{
			name:           "unprocessable client error",
			responseStatus: http.StatusUnprocessableEntity,
			taskStatus:     http.StatusBadRequest,
			body:           `{"error":{"code":"upstream-private-id","message":"Authorization: Bearer fake-upstream-secret diagnostic=private"}}`,
			code:           "invalid_request",
			message:        "CLMM Mall rejected the request",
		},
		{
			name:           "rate limit",
			responseStatus: http.StatusTooManyRequests,
			taskStatus:     http.StatusTooManyRequests,
			body:           `{"detail":"upstream-private-id Authorization: Bearer fake-upstream-secret diagnostic=private"}`,
			code:           "rate_limit_exceeded",
			message:        "CLMM Mall rate limit exceeded",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.responseStatus)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			response, err := http.Get(server.URL)
			require.NoError(t, err)
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())

			taskErr := adaptor.ParseTaskError(body, response.StatusCode)

			require.NotNil(t, taskErr)
			assert.Equal(t, test.taskStatus, taskErr.StatusCode)
			assert.Equal(t, test.code, taskErr.Code)
			assert.Equal(t, test.message, taskErr.Message)
			assert.NotContains(t, taskErr.Code, "upstream-private-id")
			assert.NotContains(t, taskErr.Message, "upstream-private-id")
			assert.NotContains(t, taskErr.Message, "fake-upstream-secret")
			assert.NotContains(t, taskErr.Message, "diagnostic")
		})
	}

	for _, statusCode := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusTeapot, http.StatusInternalServerError} {
		gatewayErr := adaptor.ParseTaskError([]byte(`{"message":"secret upstream diagnostic token=abc"}`), statusCode)
		require.NotNil(t, gatewayErr)
		assert.Equal(t, http.StatusBadGateway, gatewayErr.StatusCode)
		assert.Equal(t, "upstream_error", gatewayErr.Code)
		assert.Equal(t, "CLMM Mall upstream request failed", gatewayErr.Message)
		assert.NotContains(t, gatewayErr.Message, "abc")
	}
}

func TestTaskAdaptorImplementsRequiredInterfaces(t *testing.T) {
	var _ channel.TaskAdaptor = (*TaskAdaptor)(nil)
	var _ channel.TaskBillingRequestValidator = (*TaskAdaptor)(nil)
	var _ channel.ArkVideoTaskConverter = (*TaskAdaptor)(nil)
	var _ channel.TaskErrorParser = (*TaskAdaptor)(nil)
	var _ interface {
		ConvertToArkVideoTask(*model.Task) ([]byte, error)
	} = (*TaskAdaptor)(nil)
}

func newArkContext(body string, ark bool) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.KeySeedanceOfficialAPI, ark)
	return c
}
