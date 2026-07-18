package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
)

// SeedanceRequestConvert marks ARK-native requests and maps submission paths
// onto the existing relay routes. Query paths stay untouched because their
// task IDs are resolved by the native task controller.
func SeedanceRequestConvert() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(common.KeySeedanceOfficialAPI, true)

		if c.Request.Method == http.MethodPost {
			switch c.Request.URL.Path {
			case "/api/v3/contents/generations/tasks":
				c.Request.URL.Path = "/v1/video/generations"
				c.Set("relay_mode", relayconstant.RelayModeVideoSubmit)
			case "/api/v3/images/generations":
				c.Request.URL.Path = "/v1/images/generations"
				c.Set("relay_mode", relayconstant.RelayModeImagesGenerations)
			}
		}

		c.Next()
	}
}
