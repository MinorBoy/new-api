package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSeedanceRequestConvert(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		method   string
		path     string
		wantPath string
		wantMode int
	}{
		{
			name:     "video submit",
			method:   http.MethodPost,
			path:     "/api/v3/contents/generations/tasks",
			wantPath: "/v1/video/generations",
			wantMode: relayconstant.RelayModeVideoSubmit,
		},
		{
			name:     "image submit",
			method:   http.MethodPost,
			path:     "/api/v3/images/generations",
			wantPath: "/v1/images/generations",
			wantMode: relayconstant.RelayModeImagesGenerations,
		},
		{
			name:     "task fetch keeps path",
			method:   http.MethodGet,
			path:     "/api/v3/contents/generations/tasks/task_public",
			wantPath: "/api/v3/contents/generations/tasks/task_public",
			wantMode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(SeedanceRequestConvert())
			router.Any("/*path", func(c *gin.Context) {
				require.True(t, c.GetBool(common.KeySeedanceOfficialAPI))
				require.Equal(t, tt.wantPath, c.Request.URL.Path)
				require.Equal(t, tt.wantMode, c.GetInt("relay_mode"))
				c.Status(http.StatusNoContent)
			})

			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			router.ServeHTTP(recorder, req)
			require.Equal(t, http.StatusNoContent, recorder.Code)
		})
	}
}
