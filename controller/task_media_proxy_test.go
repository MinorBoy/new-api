package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskMediaProxyStreamsWhitelistedSunoAudioWithoutSupplierHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Content-Disposition", `inline; filename="preview.mp3"`)
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("ETag", "supplier-object-secret")
		w.Header().Set("Last-Modified", "supplier-internal-time")
		w.Header().Set("Server", "supplier-edge")
		w.Header().Set("Set-Cookie", "supplier_session=secret")
		w.Header().Set("X-Provider-Request-Id", "provider-request-secret")
		_, _ = w.Write([]byte("audio-data"))
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
	baseURL := server.URL
	channel := model.Channel{Type: constant.ChannelTypeSunoAPI, Name: "suno", Key: "supplier-key", BaseURL: &baseURL}
	channel.SetSetting(relaydto.ChannelSettings{Proxy: "http://127.0.0.1:1"})
	require.NoError(t, db.Create(&channel).Error)
	task := model.Task{
		TaskID:    "task_suno_public",
		UserId:    7,
		ChannelId: channel.Id,
		Platform:  constant.TaskPlatformSuno,
		Status:    model.TaskStatusSuccess,
		Data:      []byte(fmt.Sprintf(`[{"title":"Public title","audio_url":%q}]`, server.URL+"/audio.mp3")),
	}
	require.NoError(t, db.Create(&task).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/tasks/task_suno_public/media/0/audio", nil)
	c.Params = gin.Params{
		{Key: "task_id", Value: "task_suno_public"},
		{Key: "index", Value: "0"},
		{Key: "kind", Value: "audio"},
	}
	c.Set("id", 7)

	TaskMediaProxy(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "audio-data", recorder.Body.String())
	assert.Equal(t, "audio/mpeg", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "bytes", recorder.Header().Get("Accept-Ranges"))
	assert.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
	assert.Empty(t, recorder.Header().Get("Content-Disposition"))
	assert.Empty(t, recorder.Header().Get("ETag"))
	assert.Empty(t, recorder.Header().Get("Last-Modified"))
	assert.Empty(t, recorder.Header().Get("Server"))
	assert.Empty(t, recorder.Header().Get("Set-Cookie"))
	assert.Empty(t, recorder.Header().Get("X-Provider-Request-Id"))
}
