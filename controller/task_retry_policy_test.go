package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestShouldRetryTaskRelayNeverRetriesClmmCreate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	for _, statusCode := range []int{
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
	} {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			taskErr := &taskdto.TaskError{StatusCode: statusCode, Error: errors.New("upstream create failed")}
			assert.False(t, shouldRetryTaskRelay(c, constant.ChannelTypeClmmMall, taskErr, 1))
		})
	}
}

func TestShouldRetryTaskRelayKeepsOtherTaskChannelPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	for _, statusCode := range []int{http.StatusTooManyRequests, http.StatusInternalServerError} {
		taskErr := &taskdto.TaskError{StatusCode: statusCode, Error: errors.New("upstream create failed")}
		assert.True(t, shouldRetryTaskRelay(c, constant.ChannelTypeMegaByAI, taskErr, 1))
	}
}
