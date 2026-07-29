package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskUserResponseAuditWriterCapturesThePublicTaskResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	originalWriter := c.Writer
	auditWriter := &taskUserResponseAuditWriter{ResponseWriter: originalWriter}
	c.Writer = auditWriter

	c.JSON(http.StatusAccepted, gin.H{"id": "task-public", "status": "queued"})
	c.Writer = originalWriter

	require.Equal(t, http.StatusAccepted, recorder.Code)
	assert.JSONEq(t, recorder.Body.String(), string(auditWriter.Bytes()))
	assert.JSONEq(t, `{"id":"task-public","status":"queued"}`, string(auditWriter.Bytes()))
}
