package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/relay"
	"github.com/gin-gonic/gin"
)

func RelaySeedanceTaskCancel(c *gin.Context) {
	responseBody, taskErr := relay.SeedanceTaskCancel(c)
	if taskErr != nil {
		respondTaskError(c, taskErr)
		return
	}
	c.Data(http.StatusOK, "application/json", responseBody)
}
