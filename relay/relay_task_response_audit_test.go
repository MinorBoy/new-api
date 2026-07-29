package relay

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersistTerminalTaskUserResponseStoresOnlyTerminalResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSeedanceTaskDB(t)
	task := &model.Task{
		TaskID: "task-public",
		UserId: 1,
		Status: model.TaskStatusInProgress,
	}
	require.NoError(t, model.DB.Create(task).Error)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	response := []byte(`{"id":"task-public","status":"succeeded"}`)
	persistTerminalTaskUserResponse(c, task, response)

	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Empty(t, stored.PrivateData.UserResponseData)

	task.Status = model.TaskStatusSuccess
	require.NoError(t, model.DB.Model(task).Update("status", task.Status).Error)
	persistTerminalTaskUserResponse(c, task, response)

	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.JSONEq(t, string(response), string(stored.PrivateData.UserResponseData))
}
