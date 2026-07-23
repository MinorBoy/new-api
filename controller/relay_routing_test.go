package controller

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoutingSelectionErrorPreservesStableRelayCodeAndStatus(t *testing.T) {
	selectionErr := &service.ChannelSelectionError{
		Code:       types.ErrorCodeCompatibleChannelUnavailable,
		StatusCode: http.StatusServiceUnavailable,
		Err:        errors.New("compatible channels are unavailable"),
	}

	mapped := routingSelectionErrorToAPI(selectionErr)
	require.NotNil(t, mapped)
	assert.Equal(t, types.ErrorCodeCompatibleChannelUnavailable, mapped.GetErrorCode())
	assert.Equal(t, http.StatusServiceUnavailable, mapped.StatusCode)
}

func TestRoutingSelectionErrorMapsToTaskError(t *testing.T) {
	selectionErr := &service.ChannelSelectionError{
		Code:       types.ErrorCodeNoCompatibleRoute,
		StatusCode: http.StatusBadRequest,
		Err:        errors.New("no compatible route supports this request"),
	}

	mapped := routingSelectionErrorToTaskError(selectionErr)
	require.NotNil(t, mapped)
	assert.Equal(t, string(types.ErrorCodeNoCompatibleRoute), mapped.Code)
	assert.Equal(t, http.StatusBadRequest, mapped.StatusCode)

	wrapped := routingSelectionErrorToAPI(selectionErr)
	mapped = routingSelectionErrorToTaskError(wrapped)
	require.NotNil(t, mapped)
	assert.Equal(t, string(types.ErrorCodeNoCompatibleRoute), mapped.Code)
	assert.Equal(t, http.StatusBadRequest, mapped.StatusCode)
}
