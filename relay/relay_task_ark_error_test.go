package relay

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestARKTaskErrorFromResponsePreservesOfficialError(t *testing.T) {
	body := []byte(`{"error":{"code":"InternalServiceError","message":"The service encountered an unexpected internal error. Please retry later. Request ID: mock"}}`)

	taskErr := arkTaskErrorFromResponse(body, http.StatusInternalServerError)

	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusInternalServerError, taskErr.StatusCode)
	assert.Equal(t, "InternalServiceError", taskErr.Code)
	assert.Equal(t, "The service encountered an unexpected internal error. Please retry later. Request ID: mock", taskErr.Message)
}

func TestARKTaskErrorFromResponseRejectsInvalidEnvelope(t *testing.T) {
	taskErr := arkTaskErrorFromResponse([]byte(`{"message":"bad gateway"}`), http.StatusBadGateway)

	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadGateway, taskErr.StatusCode)
	assert.Equal(t, "fail_to_fetch_task", taskErr.Code)
}
