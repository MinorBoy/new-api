package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type taskFilterChannelOption struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type taskFilterOptionsData struct {
	Channels      []taskFilterChannelOption  `json:"channels"`
	Statuses      []string                   `json:"statuses"`
	RequestModels []string                   `json:"request_models"`
	Users         []dto.TaskFilterUserOption `json:"users"`
}

type taskFilterOptionsResponse struct {
	Success bool                  `json:"success"`
	Message string                `json:"message"`
	Data    taskFilterOptionsData `json:"data"`
}

type taskListResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Total int            `json:"total"`
		Items []*dto.TaskDto `json:"items"`
	} `json:"data"`
}

func setupTaskFilterControllerTestDB(t *testing.T) {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))

	users := []model.User{
		{Id: 10, Username: "alice", Password: "password", Group: "default", Status: common.UserStatusEnabled, AffCode: "alice-code"},
		{Id: 11, Username: "bob", Password: "password", Group: "default", Status: common.UserStatusEnabled, AffCode: "bob-code"},
	}
	require.NoError(t, db.Create(&users).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 29, Name: "paipu", Key: "test-key"}).Error)

	tasks := []model.Task{
		{TaskID: "range-a", UserId: 10, ChannelId: 40, SubmitTime: 200, Status: model.TaskStatusSuccess, Properties: model.Properties{OriginModelName: "model-b"}},
		{TaskID: "range-b", UserId: 11, ChannelId: 29, SubmitTime: 250, Status: model.TaskStatusFailure, Properties: model.Properties{OriginModelName: "model-a"}},
		{TaskID: "duplicate", UserId: 10, ChannelId: 40, SubmitTime: 300, Status: model.TaskStatusSuccess, Properties: model.Properties{OriginModelName: "model-b"}},
		{TaskID: "outside", UserId: 11, ChannelId: 99, SubmitTime: 401, Status: model.TaskStatusQueued, Properties: model.Properties{OriginModelName: "model-z"}},
	}
	require.NoError(t, db.Create(&tasks).Error)
}

func decodeTaskFilterOptionsResponse(t *testing.T, recorder *httptest.ResponseRecorder) taskFilterOptionsResponse {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code)
	var payload taskFilterOptionsResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success, payload.Message)
	return payload
}

func TestGetAllTaskFilterOptionsUsesAllLogsInTimeRange(t *testing.T) {
	setupTaskFilterControllerTestDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/task/filter-options?start_timestamp=100&end_timestamp=400", nil)

	GetAllTaskFilterOptions(ctx)

	payload := decodeTaskFilterOptionsResponse(t, recorder)
	assert.Equal(t, []taskFilterChannelOption{
		{ID: 29, Name: "paipu"},
		{ID: 40, Name: ""},
	}, payload.Data.Channels)
	assert.Equal(t, []string{string(model.TaskStatusFailure), string(model.TaskStatusSuccess)}, payload.Data.Statuses)
	assert.Equal(t, []string{"model-a", "model-b"}, payload.Data.RequestModels)
	assert.Equal(t, []dto.TaskFilterUserOption{
		{ID: 10, Username: "alice"},
		{ID: 11, Username: "bob"},
	}, payload.Data.Users)
}

func TestGetUserTaskFilterOptionsRestrictsDimensionsAndUserScope(t *testing.T) {
	setupTaskFilterControllerTestDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 10)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/task/self/filter-options?start_timestamp=100&end_timestamp=400", nil)

	GetUserTaskFilterOptions(ctx)

	payload := decodeTaskFilterOptionsResponse(t, recorder)
	assert.Empty(t, payload.Data.Channels)
	assert.Equal(t, []string{string(model.TaskStatusSuccess)}, payload.Data.Statuses)
	assert.Equal(t, []string{"model-b"}, payload.Data.RequestModels)
	assert.Empty(t, payload.Data.Users)
}

func TestGetAllTaskAppliesUserAndRequestModelFilters(t *testing.T) {
	setupTaskFilterControllerTestDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/task/?p=1&page_size=10&user_id=10&request_model=model-b", nil)

	GetAllTask(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload taskListResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	assert.Equal(t, 2, payload.Data.Total)
	require.Len(t, payload.Data.Items, 2)
	assert.Equal(t, 10, payload.Data.Items[0].UserId)
	assert.Equal(t, "model-b", payload.Data.Items[0].RequestModel)
}
