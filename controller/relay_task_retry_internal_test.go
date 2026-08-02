package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestShouldRetryTaskRelayNeverRetriesPaipuSubmit(t *testing.T) {
	for _, status := range []int{
		http.StatusTooManyRequests,
		http.StatusTemporaryRedirect,
		http.StatusInternalServerError,
		http.StatusBadGateway,
	} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypePaipu)
			taskErr := service.TaskErrorWrapper(errors.New("upstream uncertain"), "upstream_error", status)
			assert.False(t, shouldRetryTaskRelay(c, constant.ChannelTypePaipu, taskErr, 3))
		})
	}
}

func TestShouldRetryTaskRelayStillRetriesOtherTaskChannels(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeCangyuan)
	taskErr := service.TaskErrorWrapper(errors.New("upstream uncertain"), "upstream_error", http.StatusInternalServerError)
	assert.True(t, shouldRetryTaskRelay(c, constant.ChannelTypeCangyuan, taskErr, 3))
}
