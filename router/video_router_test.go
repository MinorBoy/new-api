package router

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/video_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withRouterVideoPolicyDefaults(t *testing.T) {
	t.Helper()

	cfg := config.GlobalConfig.Get(video_setting.ConfigName)
	require.NotNil(t, cfg)
	saved := video_setting.Runtime()
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{
			video_setting.KeyBase64InputEnabled:   strconv.FormatBool(saved.Base64InputEnabled),
			video_setting.KeyJSONRequestBodyMaxMB: strconv.Itoa(saved.JSONRequestBodyMaxMB),
		}))
		video_setting.UpdateAndSync()
	})

	require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{
		video_setting.KeyBase64InputEnabled:   "false",
		video_setting.KeyJSONRequestBodyMaxMB: "1",
	}))
	video_setting.UpdateAndSync()
}

func performVideoRouterRequest(router http.Handler, path string, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestVideoRouterRejectsBase64BeforePreAuthConverters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withRouterVideoPolicyDefaults(t)

	router := gin.New()
	SetVideoRouter(router)

	tests := []struct {
		name     string
		path     string
		wantCode string
	}{
		{name: "kling text2video", path: "/kling/v1/videos/text2video", wantCode: "video_base64_input_disabled"},
		{name: "kling image2video", path: "/kling/v1/videos/image2video", wantCode: "video_base64_input_disabled"},
		{name: "jimeng submit", path: "/jimeng/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31", wantCode: "video_base64_input_disabled"},
		{name: "seedance submit", path: "/api/v3/contents/generations/tasks", wantCode: "InvalidParameter.content"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := performVideoRouterRequest(router, tt.path, `{"model":"m","image":"data:image/png;base64,`+strings.Repeat("A", 80)+`"}`)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.Contains(t, recorder.Body.String(), tt.wantCode)
			assert.NotContains(t, recorder.Body.String(), strings.Repeat("A", 80))
		})
	}
}

func TestVideoRouterSkipsJimengPostQueryAction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withRouterVideoPolicyDefaults(t)

	router := gin.New()
	SetVideoRouter(router)

	recorder := performVideoRouterRequest(router, "/jimeng/?Action=CVSync2AsyncGetResult&Version=2022-08-31", `{"task_id":"task_1","image":"data:image/png;base64,`+strings.Repeat("A", 80)+`"}`)

	assert.NotEqual(t, http.StatusBadRequest, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "video_base64_input_disabled")
}
