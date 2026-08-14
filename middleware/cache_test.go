package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestCacheDisablesCachingForMissingAPIRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(Cache())
	engine.NoRoute(func(c *gin.Context) {
		c.Status(http.StatusNotFound)
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v3/assets?type=image", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	assert.Equal(t, "no-store, no-cache, must-revalidate, private, max-age=0", recorder.Header().Get("Cache-Control"))
}
