package controller

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetGeminiVideoURLRejectsUntrustedMediaOrigin(t *testing.T) {
	baseURL := "https://generativelanguage.googleapis.com"
	channel := &model.Channel{Type: constant.ChannelTypeGemini, BaseURL: &baseURL}
	task := &model.Task{Data: []byte(`{"uri":"https://supplier.example/private-video.mp4"}`)}

	videoURL, err := getGeminiVideoURL(channel, task, "gemini-secret-key")

	require.Error(t, err)
	assert.Empty(t, videoURL)
	assert.NotContains(t, err.Error(), "gemini-secret-key")
}

func TestGeminiVideoProxyDoesNotForwardAPIKeyAcrossMediaRedirect(t *testing.T) {
	var initialAPIKeyHeader string
	var initialAPIKeyQuery string
	var redirectedAPIKeyHeader string
	var redirectedAPIKeyQuery string
	var redirectedReferer string
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectedAPIKeyHeader = r.Header.Get("x-goog-api-key")
		redirectedAPIKeyQuery = r.URL.Query().Get("key")
		redirectedReferer = r.Header.Get("Referer")
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("video-data"))
	}))
	t.Cleanup(sink.Close)

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		initialAPIKeyHeader = r.Header.Get("x-goog-api-key")
		initialAPIKeyQuery = r.URL.Query().Get("key")
		http.Redirect(w, r, sink.URL+"/video.mp4", http.StatusFound)
	}))
	t.Cleanup(origin.Close)

	fetchSetting := system_setting.GetFetchSetting()
	originalFetchSetting := *fetchSetting
	originURL, err := url.Parse(origin.URL)
	require.NoError(t, err)
	sinkURL, err := url.Parse(sink.URL)
	require.NoError(t, err)
	fetchSetting.EnableSSRFProtection = true
	fetchSetting.AllowPrivateIp = true
	fetchSetting.AllowedPorts = []string{originURL.Port(), sinkURL.Port()}
	service.InitHttpClient()
	t.Cleanup(func() {
		*fetchSetting = originalFetchSetting
		service.InitHttpClient()
	})

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	baseURL := origin.URL
	channel := model.Channel{Type: constant.ChannelTypeGemini, Name: "gemini", BaseURL: &baseURL}
	require.NoError(t, db.Create(&channel).Error)
	task := model.Task{
		TaskID:    "task_gemini_public",
		UserId:    7,
		ChannelId: channel.Id,
		Status:    model.TaskStatusSuccess,
		Data:      []byte(`{"uri":"` + origin.URL + `/generated-video"}`),
		PrivateData: model.TaskPrivateData{
			Key: "gemini-secret-key",
		},
	}
	require.NoError(t, db.Create(&task).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task_gemini_public/content", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "task_gemini_public"}}
	c.Set("id", 7)

	VideoProxy(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "video-data", recorder.Body.String())
	assert.Equal(t, "gemini-secret-key", initialAPIKeyHeader)
	assert.Empty(t, initialAPIKeyQuery)
	assert.Empty(t, redirectedAPIKeyHeader)
	assert.Empty(t, redirectedAPIKeyQuery)
	assert.Empty(t, redirectedReferer)
}

func TestPublicMediaContentTypeAllowlist(t *testing.T) {
	tests := []struct {
		name        string
		kind        string
		contentType string
		want        string
		allowed     bool
	}{
		{name: "video", kind: "video", contentType: "video/mp4; charset=binary", want: "video/mp4", allowed: true},
		{name: "audio", kind: "audio", contentType: "audio/mpeg", want: "audio/mpeg", allowed: true},
		{name: "image", kind: "image", contentType: "image/png", want: "image/png", allowed: true},
		{name: "opaque binary", kind: "video", contentType: "application/octet-stream", want: "application/octet-stream", allowed: true},
		{name: "html", kind: "video", contentType: "text/html", allowed: false},
		{name: "svg", kind: "image", contentType: "image/svg+xml", allowed: false},
		{name: "wrong media kind", kind: "audio", contentType: "video/mp4", allowed: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contentType, allowed := publicMediaContentType(test.kind, test.contentType)
			assert.Equal(t, test.allowed, allowed)
			assert.Equal(t, test.want, contentType)
		})
	}
}

func TestWriteVideoDataURLRejectsExecutableContent(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	payload := base64.StdEncoding.EncodeToString([]byte("<script>alert(1)</script>"))

	err := writeVideoDataURL(c, "data:text/html;base64,"+payload)

	require.Error(t, err)
	assert.Empty(t, recorder.Body.String())
}

func TestVideoProxyDoesNotExposeBlockedSupplierURL(t *testing.T) {
	fetchSetting := system_setting.GetFetchSetting()
	originalFetchSetting := *fetchSetting
	fetchSetting.EnableSSRFProtection = true
	fetchSetting.AllowPrivateIp = false
	service.InitHttpClient()
	t.Cleanup(func() {
		*fetchSetting = originalFetchSetting
		service.InitHttpClient()
	})

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	baseURL := "http://127.0.0.1:1"
	channel := model.Channel{Type: constant.ChannelTypeEightYes, Name: "private-provider", Key: "private-key", BaseURL: &baseURL}
	require.NoError(t, db.Create(&channel).Error)
	task := model.Task{
		TaskID:    "task_blocked_public",
		UserId:    7,
		ChannelId: channel.Id,
		Status:    model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "private-upstream-task",
		},
	}
	require.NoError(t, db.Create(&task).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task_blocked_public/content", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "task_blocked_public"}}
	c.Set("id", 7)

	VideoProxy(c)

	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "127.0.0.1")
	assert.NotContains(t, recorder.Body.String(), "private-provider")
	assert.NotContains(t, recorder.Body.String(), "private-upstream-task")
}
