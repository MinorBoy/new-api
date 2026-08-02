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

func TestEightYesVideoProxyUsesPrivateUpstreamTaskIDAndBearerKey(t *testing.T) {
	var upstreamPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.EscapedPath()
		assert.Equal(t, "Bearer eightyes-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("mp4-data"))
	}))
	t.Cleanup(server.Close)

	fetchSetting := system_setting.GetFetchSetting()
	originalFetchSetting := *fetchSetting
	parsedServerURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	fetchSetting.EnableSSRFProtection = true
	fetchSetting.AllowPrivateIp = true
	fetchSetting.AllowedPorts = []string{parsedServerURL.Port()}
	service.InitHttpClient()
	t.Cleanup(func() { *fetchSetting = originalFetchSetting })

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	baseURL := server.URL + "/"
	channel := model.Channel{Type: constant.ChannelTypeEightYes, Name: "8yes", Key: "eightyes-key", BaseURL: &baseURL}
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

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task_public/content", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "task_public"}}
	c.Set("id", 7)

	VideoProxy(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "/v1/videos/upstream%2Fprivate/content", upstreamPath)
	assert.Equal(t, "mp4-data", recorder.Body.String())
}
