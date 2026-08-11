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

func TestFFLinkVideoProxyForwardsRangeAndDropsBearerOnCrossOriginRedirect(t *testing.T) {
	var originPath, originRange, originAuthorization string
	var sinkAuthorization, sinkReferer string
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sinkAuthorization = r.Header.Get("Authorization")
		sinkReferer = r.Header.Get("Referer")
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Range", "bytes 10-19/100")
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("partial-data"))
	}))
	t.Cleanup(sink.Close)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originPath = r.URL.EscapedPath()
		originRange = r.Header.Get("Range")
		originAuthorization = r.Header.Get("Authorization")
		http.Redirect(w, r, sink.URL+"/content", http.StatusFound)
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
	t.Cleanup(func() { *fetchSetting = originalFetchSetting; service.InitHttpClient() })

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	baseURL := origin.URL
	channel := model.Channel{Type: constant.ChannelTypeFFLink, Name: "FYLink", Key: "fallback-key", BaseURL: &baseURL}
	require.NoError(t, db.Create(&channel).Error)
	task := model.Task{
		TaskID: "task_fflink_public", UserId: 7, ChannelId: channel.Id, Status: model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "job/private", Key: "selected-fflink-key"},
	}
	require.NoError(t, db.Create(&task).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task_fflink_public/content", nil)
	c.Request.Header.Set("Range", "bytes=10-19")
	c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	c.Set("id", 7)

	VideoProxy(c)

	assert.Equal(t, http.StatusPartialContent, recorder.Code)
	assert.Equal(t, "/v1/videos/jobs/job%2Fprivate/content", originPath)
	assert.Equal(t, "bytes=10-19", originRange)
	assert.Equal(t, "Bearer selected-fflink-key", originAuthorization)
	assert.Empty(t, sinkAuthorization)
	assert.Empty(t, sinkReferer)
	assert.Equal(t, "bytes 10-19/100", recorder.Header().Get("Content-Range"))
	assert.Equal(t, "bytes", recorder.Header().Get("Accept-Ranges"))
	assert.Equal(t, "partial-data", recorder.Body.String())
	assert.NotContains(t, recorder.Body.String(), "job/private")
}
