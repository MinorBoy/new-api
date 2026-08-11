package newapivideo

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFFLinkCancelTaskUsesEscapedPrivateIDAndBearerKey(t *testing.T) {
	var method, escapedPath, authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		escapedPath = r.URL.EscapedPath()
		authorization = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	response, err := NewFYLinkTaskAdaptor().CancelTask(context.Background(), server.URL, "selected-key", "job/private", "")
	require.NoError(t, err)
	require.NotNil(t, response)
	_, _ = io.Copy(io.Discard, response.Body)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, http.MethodDelete, method)
	assert.Equal(t, "/v1/videos/jobs/job%2Fprivate", escapedPath)
	assert.Equal(t, "Bearer selected-key", authorization)
}
