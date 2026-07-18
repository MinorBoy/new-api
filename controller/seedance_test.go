package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedanceTaskErrorUsesARKEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(common.KeySeedanceOfficialAPI, true)

	respondTaskError(c, &dto.TaskError{Code: "task_not_exist", Message: "task_not_exist", StatusCode: http.StatusNotFound})

	assert.Equal(t, http.StatusNotFound, recorder.Code)
	var response struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "task_not_exist", response.Error.Code)
	assert.Equal(t, "task_not_exist", response.Error.Message)
}

func TestSeedanceTaskErrorNormalizesGatewayFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		taskError   *dto.TaskError
		wantCode    string
		wantMessage string
	}{
		{
			name:        "network failure",
			taskError:   &dto.TaskError{Code: "do_request_failed", Message: "do request failed: upstream error: do request failed", StatusCode: http.StatusInternalServerError},
			wantCode:    "InternalServiceError",
			wantMessage: "The service encountered an unexpected internal error. Please retry later.",
		},
		{
			name:        "invalid upstream response",
			taskError:   &dto.TaskError{Code: "fail_to_fetch_task", Message: "upstream returned invalid response", StatusCode: http.StatusBadGateway},
			wantCode:    "InternalServiceError",
			wantMessage: "The service encountered an unexpected internal error. Please retry later.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set(common.KeySeedanceOfficialAPI, true)

			respondTaskError(c, tt.taskError)

			var response struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, tt.wantCode, response.Error.Code)
			assert.Equal(t, tt.wantMessage, response.Error.Message)
		})
	}
}
