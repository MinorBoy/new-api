package relay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSeedanceTaskCancelCancelsPendingFFLinkTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	t.Cleanup(func() { model.DB = originalDB; _ = sqlDB.Close() })
	require.NoError(t, database.AutoMigrate(&model.Task{}, &model.Channel{}))
	model.DB = database

	var cancelCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancelCalls++
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "Bearer selected-key", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	baseURL := server.URL
	channel := &model.Channel{Type: constant.ChannelTypeFFLink, Key: "fallback-key", BaseURL: &baseURL}
	require.NoError(t, model.DB.Create(channel).Error)
	task := &model.Task{
		TaskID: "task_cancel_public", Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeFFLink)), UserId: 7,
		ChannelId: channel.Id, Status: model.TaskStatusQueued, Progress: "20%", Quota: 0,
		Properties:  model.Properties{RequestPath: "/api/v3/contents/generations/tasks"},
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "job-private", Key: "selected-key"},
		Data:        json.RawMessage(`{"status":"pending"}`), SubmitTime: time.Now().Unix(),
	}
	require.NoError(t, model.DB.Create(task).Error)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v3/contents/generations/tasks/task_cancel_public", nil)
	c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	c.Set("id", 7)
	body, taskErr := SeedanceTaskCancel(c)
	require.Nil(t, taskErr)
	assert.Contains(t, string(body), `"status":"failed"`)
	assert.Equal(t, 1, cancelCalls)

	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.Equal(t, string(model.TaskStatusFailure), string(reloaded.Status))
	assert.Equal(t, "100%", reloaded.Progress)
	assert.Equal(t, "task canceled", reloaded.FailReason)
	assert.Empty(t, reloaded.PrivateData.UpstreamTaskID)
	assert.Empty(t, reloaded.PrivateData.Key)

	secondContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	secondContext.Request = httptest.NewRequest(http.MethodDelete, "/api/v3/contents/generations/tasks/task_cancel_public", nil)
	secondContext.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	secondContext.Set("id", 7)
	_, secondErr := SeedanceTaskCancel(secondContext)
	require.NotNil(t, secondErr)
	assert.Equal(t, "task_not_cancellable", secondErr.Code)
	assert.Equal(t, 1, cancelCalls)

	otherUserContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	otherUserContext.Request = httptest.NewRequest(http.MethodDelete, "/api/v3/contents/generations/tasks/task_cancel_public", nil)
	otherUserContext.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	otherUserContext.Set("id", 99)
	_, otherUserErr := SeedanceTaskCancel(otherUserContext)
	require.NotNil(t, otherUserErr)
	assert.Equal(t, "task_not_exist", otherUserErr.Code)

	unsupported := &model.Task{
		TaskID: "task_unsupported_cancel", Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeZ5API)), UserId: 7,
		ChannelId: channel.Id, Status: model.TaskStatusQueued, Progress: "20%", Properties: model.Properties{RequestPath: "/api/v3/contents/generations/tasks"},
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "z5-job"}, Data: json.RawMessage(`{"status":"pending"}`), SubmitTime: time.Now().Unix(),
	}
	require.NoError(t, model.DB.Create(unsupported).Error)
	unsupportedContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	unsupportedContext.Request = httptest.NewRequest(http.MethodDelete, "/api/v3/contents/generations/tasks/task_unsupported_cancel", nil)
	unsupportedContext.Params = gin.Params{{Key: "task_id", Value: unsupported.TaskID}}
	unsupportedContext.Set("id", 7)
	_, unsupportedErr := SeedanceTaskCancel(unsupportedContext)
	require.NotNil(t, unsupportedErr)
	assert.Equal(t, "task_cancellation_not_supported", unsupportedErr.Code)
}
