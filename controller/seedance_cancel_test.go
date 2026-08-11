package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestSeedanceCancelHandlerReturnsJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupControllerTaskCostDB(t)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/v3/contents/generations/tasks/task_public", nil)
	ctx.Params = gin.Params{{Key: "task_id", Value: "task_public"}}
	ctx.Set("id", 7)
	RelaySeedanceTaskCancel(ctx)
	assert.Equal(t, http.StatusNotFound, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "task_not_exist")
}
