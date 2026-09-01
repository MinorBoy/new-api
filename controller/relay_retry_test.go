package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestShouldRetryImageAfterCostOutcomeOnlyWhenUpstreamRejectedBeforeAcceptance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	retryable := types.NewErrorWithStatusCode(assert.AnError, types.ErrorCodeDoRequestFailed, http.StatusTooManyRequests)

	assert.True(t, shouldRetryImageAfterCostOutcome(c, retryable, 1, &hosttypes.CostOutcome{Status: hosttypes.CostAttemptNotDispatched}))
	assert.False(t, shouldRetryImageAfterCostOutcome(c, retryable, 1, &hosttypes.CostOutcome{Status: hosttypes.CostAttemptUnknown}))
	assert.False(t, shouldRetryImageAfterCostOutcome(c, retryable, 1, &hosttypes.CostOutcome{Status: hosttypes.CostAttemptNotDispatched, UpstreamAccepted: true}))
}

func TestShouldRetryImageAfterCostOutcomeAllowsRetryableFiveHundredRejection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	retryable := types.NewErrorWithStatusCode(assert.AnError, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)

	assert.True(t, shouldRetryImageAfterCostOutcome(c, retryable, 1, &hosttypes.CostOutcome{Status: hosttypes.CostAttemptNotDispatched}))
	assert.False(t, shouldRetryImageAfterCostOutcome(c, retryable, 1, &hosttypes.CostOutcome{Status: hosttypes.CostAttemptUnknown, FailureCode: "upstream_transport_ambiguous"}))
}
