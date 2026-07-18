package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/relay"
	"github.com/gin-gonic/gin"
)

// RelaySeedanceTaskFetch serves the ARK-compatible single-task and list APIs.
func RelaySeedanceTaskFetch(c *gin.Context) {
	responseBody, taskErr := relay.SeedanceTaskFetch(c)
	if taskErr != nil {
		respondTaskError(c, taskErr)
		return
	}
	c.Data(http.StatusOK, "application/json", responseBody)
}
