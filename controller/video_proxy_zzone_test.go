package controller

import (
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

func TestZZoneVideoProxyUsesPrivateUpstreamTaskIDAndBearerKey(t *testing.T) {
	var upstreamPath string
	var upstreamAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamPath = request.URL.EscapedPath()
		upstreamAuthorization = request.Header.Get("Authorization")
		writer.Header().Set("Content-Type", "video/mp4")
		writer.Header().Set("Accept-Ranges", "bytes")
		writer.Header().Set("Set-Cookie", "supplier_session=secret")
		writer.Header().Set("X-Provider-Request-Id", "provider-request-secret")
		_, _ = writer.Write([]byte("mp4-data"))
	}))
	t.Cleanup(server.Close)
	allowVideoProxyTestServers(t, server.URL)

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	baseURL := server.URL + "/"
	channel := model.Channel{Type: constant.ChannelTypeZZone, Name: "ZZone", Key: "zzone-key", BaseURL: &baseURL}
	require.NoError(t, db.Create(&channel).Error)
	task := model.Task{
		TaskID:    "task_public",
		UserId:    7,
		ChannelId: channel.Id,
		Status:    model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream/private",
			ResultURL:      "https://gateway.example/v1/videos/task_public/content",
		},
	}
	require.NoError(t, db.Create(&task).Error)

	recorder := performZZoneVideoProxyRequest("task_public", 7)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "/v1/videos/upstream%2Fprivate/content", upstreamPath)
	assert.Equal(t, "Bearer zzone-key", upstreamAuthorization)
	assert.Equal(t, "mp4-data", recorder.Body.String())
	assert.Equal(t, "video/mp4", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "bytes", recorder.Header().Get("Accept-Ranges"))
	assert.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	assert.Empty(t, recorder.Header().Get("Set-Cookie"))
	assert.Empty(t, recorder.Header().Get("X-Provider-Request-Id"))
}

func TestZZoneVideoProxyStripsAuthorizationOnCrossOriginRedirect(t *testing.T) {
	redirectAuthorization := "not-called"
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		redirectAuthorization = request.Header.Get("Authorization")
		writer.Header().Set("Content-Type", "video/mp4")
		_, _ = writer.Write([]byte("redirected-video"))
	}))
	t.Cleanup(target.Close)

	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "Bearer zzone-key", request.Header.Get("Authorization"))
		http.Redirect(writer, request, target.URL+"/video.mp4", http.StatusFound)
	}))
	t.Cleanup(source.Close)
	allowVideoProxyTestServers(t, source.URL, target.URL)

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	baseURL := source.URL
	channel := model.Channel{Type: constant.ChannelTypeZZone, Name: "ZZone", Key: "zzone-key", BaseURL: &baseURL}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Task{
		TaskID:    "task_redirect",
		UserId:    7,
		ChannelId: channel.Id,
		Status:    model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "zzone-private",
			ResultURL:      "https://gateway.example/v1/videos/task_redirect/content",
		},
	}).Error)

	recorder := performZZoneVideoProxyRequest("task_redirect", 7)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "redirected-video", recorder.Body.String())
	assert.Empty(t, redirectAuthorization)
}

func allowVideoProxyTestServers(t *testing.T, rawURLs ...string) {
	t.Helper()
	fetchSetting := system_setting.GetFetchSetting()
	originalFetchSetting := *fetchSetting
	fetchSetting.EnableSSRFProtection = true
	fetchSetting.AllowPrivateIp = true
	fetchSetting.AllowedPorts = nil
	for _, rawURL := range rawURLs {
		parsedURL, err := url.Parse(rawURL)
		require.NoError(t, err)
		fetchSetting.AllowedPorts = append(fetchSetting.AllowedPorts, parsedURL.Port())
	}
	service.InitHttpClient()
	t.Cleanup(func() {
		*fetchSetting = originalFetchSetting
		service.InitHttpClient()
	})
}

func performZZoneVideoProxyRequest(taskID string, userID int) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/"+taskID+"/content", nil)
	context.Params = gin.Params{{Key: "task_id", Value: taskID}}
	context.Set("id", userID)
	VideoProxy(context)
	return recorder
}
