package newapivideo

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskAdaptorSubmitAndFetchTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		if r.Method == http.MethodGet {
			assert.Equal(t, "/v1/video/generations/upstream%2Ftask", r.URL.EscapedPath())
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "test-key"}}
	adaptor.Init(info)
	url, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, server.URL+"/v1/video/generations", url)

	req, err := http.NewRequest(http.MethodPost, url, nil)
	require.NoError(t, err)
	require.NoError(t, adaptor.BuildRequestHeader(nil, req, info))
	assert.Equal(t, "Bearer test-key", req.Header.Get("Authorization"))
	assert.Equal(t, "application/json", req.Header.Get("Accept"))
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))

	response, err := adaptor.FetchTask(server.URL, "test-key", map[string]any{"task_id": "upstream/task"}, "")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.NoError(t, response.Body.Close())
}

func TestTaskAdaptorDoResponse(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		ark        bool
		wantID     string
		wantCode   string
		wantStatus int
		wantBody   string
		wantEmpty  bool
	}{
		{
			name:       "matching ids openai",
			body:       `{"id":"upstream","task_id":"upstream","object":"task","model":"provider","status":"queued","progress":12,"created_at":123}`,
			wantID:     "upstream",
			wantStatus: http.StatusOK,
			wantBody:   `{"id":"public","task_id":"public","object":"video","model":"client","status":"queued","progress":12,"created_at":123}`,
		},
		{
			name:       "only id",
			body:       `{"id":"upstream","status":"queued"}`,
			wantID:     "upstream",
			wantStatus: http.StatusOK,
			wantBody:   `{"id":"public","task_id":"public","object":"video","model":"client","status":"queued","progress":0,"created_at":0}`,
		},
		{
			name:       "only task id",
			body:       `{"task_id":"upstream","status":"queued"}`,
			wantID:     "upstream",
			wantStatus: http.StatusOK,
			wantBody:   `{"id":"public","task_id":"public","object":"video","model":"client","status":"queued","progress":0,"created_at":0}`,
		},
		{
			name:       "ark response",
			body:       `{"id":"upstream","task_id":"upstream","model":"provider","status":"queued"}`,
			ark:        true,
			wantID:     "upstream",
			wantStatus: http.StatusOK,
			wantBody:   `{"id":"public"}`,
		},
		{
			name:       "conflicting ids",
			body:       `{"id":"one","task_id":"two"}`,
			wantCode:   "invalid_response",
			wantStatus: http.StatusBadGateway,
			wantEmpty:  true,
		},
		{
			name:       "missing ids",
			body:       `{"status":"queued"}`,
			wantCode:   "invalid_response",
			wantStatus: http.StatusBadGateway,
			wantEmpty:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			if tt.ark {
				c.Set(common.KeySeedanceOfficialAPI, true)
			}
			info := &relaycommon.RelayInfo{
				OriginModelName: "client",
				ChannelMeta:     &relaycommon.ChannelMeta{},
				TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "public"},
			}

			upstreamID, _, taskErr := (&TaskAdaptor{}).DoResponse(c, &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}, info)

			if tt.wantCode != "" {
				require.NotNil(t, taskErr)
				assert.Equal(t, tt.wantCode, taskErr.Code)
				assert.Equal(t, tt.wantStatus, taskErr.StatusCode)
				assert.Empty(t, recorder.Body.String())
				return
			}
			require.Nil(t, taskErr)
			assert.Equal(t, tt.wantID, upstreamID)
			assert.Equal(t, tt.wantStatus, recorder.Code)
			assert.JSONEq(t, tt.wantBody, recorder.Body.String())
		})
	}
}

func TestParseTaskError(t *testing.T) {
	adaptor := &TaskAdaptor{}
	nested := adaptor.ParseTaskError([]byte(`{"code":"top","message":"top message","error":{"code":"nested","message":"nested message"}}`), http.StatusTooManyRequests)
	require.NotNil(t, nested)
	assert.Equal(t, "nested", nested.Code)
	assert.Equal(t, "nested message", nested.Message)
	assert.Equal(t, http.StatusTooManyRequests, nested.StatusCode)
	assert.False(t, nested.LocalError)

	topLevel := adaptor.ParseTaskError([]byte(`{"code":"bad_request","message":"invalid request"}`), http.StatusBadRequest)
	require.NotNil(t, topLevel)
	assert.Equal(t, "bad_request", topLevel.Code)
	assert.Equal(t, "invalid request", topLevel.Message)

	invalid := adaptor.ParseTaskError([]byte(`not-json`), http.StatusBadGateway)
	require.NotNil(t, invalid)
	assert.Equal(t, "fail_to_fetch_task", invalid.Code)
	assert.Equal(t, http.StatusBadGateway, invalid.StatusCode)
}

func TestTaskAdaptorRejectsUnsupportedMediaType(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{"model":"m","prompt":"text"}`))
	c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	err := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}})
	require.NotNil(t, err)
	assert.Equal(t, http.StatusUnsupportedMediaType, err.StatusCode)
}

func TestTaskAdaptorUsesIntegerSecondsForDurationEstimator(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{"model":"m","prompt":"text","seconds":"10"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info))
	seconds, err := (&TaskAdaptor{}).EstimateDurationSeconds(c, info)
	require.Nil(t, err)
	assert.Equal(t, 10, seconds)
	assert.Nil(t, (&TaskAdaptor{}).EstimateBilling(c, info))
}

func TestTaskAdaptorRejectsDurationOnlyForDurationEstimator(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{"model":"m","prompt":"text","duration":10}`))
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info))
	seconds, err := (&TaskAdaptor{}).EstimateDurationSeconds(c, info)
	assert.Zero(t, seconds)
	require.NotNil(t, err)
	assert.Equal(t, "invalid_seconds", err.Code)
}

func TestTaskAdaptorIsTaskOnly(t *testing.T) {
	assert.Empty(t, (&TaskAdaptor{}).GetModelList())
	assert.Equal(t, ChannelName, (&TaskAdaptor{}).GetChannelName())
}
