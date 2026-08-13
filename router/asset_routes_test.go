package router

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/controller"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRegisterAssetRoutesExposesCompleteContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")
	registerAssetRoutes(api, controller.NewAssetController(nil), func(c *gin.Context) {
		c.Set("id", 42)
		c.Next()
	})

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v3/assets"},
		{http.MethodGet, "/api/v3/assets"},
		{http.MethodGet, "/api/v3/assets/:asset_id"},
		{http.MethodPost, "/api/v3/assets/:asset_id/refresh"},
	}
	registered := make(map[string]bool)
	for _, route := range engine.Routes() {
		registered[route.Method+route.Path] = true
	}
	for _, route := range routes {
		t.Run(route.method+route.path, func(t *testing.T) {
			assert.True(t, registered[route.method+route.path])
		})
	}
}
