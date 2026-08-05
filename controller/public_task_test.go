package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupPublicTaskControllerDB(t *testing.T) {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))

	users := []model.User{
		{Id: 10, Username: "alice", Password: "password", Group: "default", Status: common.UserStatusEnabled, AffCode: "alice-public-task"},
		{Id: 11, Username: "bob", Password: "password", Group: "default", Status: common.UserStatusEnabled, AffCode: "bob-public-task"},
	}
	require.NoError(t, db.Create(&users).Error)

	platform := constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo))
	tasks := []model.Task{
		{
			ID: 91, TaskID: "task-public-a", Platform: platform, UserId: 10,
			Group: "internal-group-a", ChannelId: 40, Quota: 100,
			Action: "generate", Status: model.TaskStatusSuccess,
			CreatedAt: 100, UpdatedAt: 105, SubmitTime: 100, StartTime: 101, FinishTime: 105, Progress: "100%",
			Properties: model.Properties{
				OriginModelName:   "doubao-seedance-2-0-260128",
				UpstreamModelName: "provider-model-a",
				RequestPath:       "/supplier/private/path",
			},
			PrivateData: model.TaskPrivateData{
				UpstreamTaskID:       "cgt-secret-a",
				ResultURL:            "https://supplier.example/a.mp4",
				UserRequestData:      []byte(`{"model":"doubao-seedance-2-0-260128","duration":4}`),
				UpstreamResponseData: []byte(`{"id":"cgt-secret-a"}`),
				UserResponseData: []byte(`{
					"id":"cgt-secret-a",
					"model":"provider-model-a",
					"status":"succeeded",
					"content":{"video_url":"https://supplier.example/a.mp4"}
				}`),
				BillingContext: &model.TaskBillingContext{
					UsageProfile:    model.TaskUsageProfileSeedance,
					OriginModelName: "doubao-seedance-2-0-260128",
				},
			},
		},
		{
			ID: 92, TaskID: "task-public-b", Platform: platform, UserId: 10,
			Group: "internal-group-b", ChannelId: 41, Quota: 200,
			Action: "generate", Status: model.TaskStatusFailure,
			CreatedAt: 200, UpdatedAt: 205, SubmitTime: 200, FinishTime: 205, Progress: "100%",
			FailReason: "supplier failure cgt-secret-b",
			Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
			PrivateData: model.TaskPrivateData{
				UpstreamTaskID: "cgt-secret-b",
				UserResponseData: []byte(`{
					"id":"cgt-secret-b",
					"model":"provider-model-b",
					"status":"failed",
					"error":{"code":"provider_error","message":"supplier failure"}
				}`),
				BillingContext: &model.TaskBillingContext{
					UsageProfile:    model.TaskUsageProfileSeedance,
					OriginModelName: "doubao-seedance-2-0-260128",
				},
			},
		},
		{
			ID: 93, TaskID: "task-other-user", Platform: platform, UserId: 11,
			Group: "internal-group-c", ChannelId: 42, Action: "generate",
			Status: model.TaskStatusSuccess, SubmitTime: 300,
			Properties: model.Properties{OriginModelName: "other-public-model"},
		},
	}
	require.NoError(t, db.Create(&tasks).Error)
}

func assertPublicTaskPayload(t *testing.T, body string) {
	t.Helper()
	for _, forbidden := range []string{
		`"id":91`, `"id":92`, `"user_id"`, `"username"`, `"platform"`,
		`"group"`, `"channel_id"`, `"properties"`,
		`"request_path"`, `"user_request_data"`, `"upstream_response_data"`,
		"provider-model-a", "provider-model-b", "cgt-secret-a", "cgt-secret-b",
		"internal-group-a", "internal-group-b", "supplier.example", "supplier failure",
	} {
		assert.NotContains(t, body, forbidden)
	}
	assert.Contains(t, body, `"task_id":"task-public-a"`)
	assert.Contains(t, body, `"request_model":"doubao-seedance-2-0-260128"`)
	assert.Contains(t, body, `/v1/videos/task-public-a/content`)
	assert.Contains(t, body, `"user_response_data"`)
}

func TestGetUserTaskIgnoresAdminDimensionsAndReturnsPublicDTO(t *testing.T) {
	setupPublicTaskControllerDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 10)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/task/self?p=1&page_size=20&platform=probe&channel_id=999&user_id=11&group=probe",
		nil,
	)

	GetUserTask(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"total":2`)
	assertPublicTaskPayload(t, body)
}

func TestGetAllTaskKeepsAdministratorAuditDTO(t *testing.T) {
	setupPublicTaskControllerDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/task/?p=1&page_size=20&task_id=task-public-a",
		nil,
	)

	GetAllTask(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"platform":"`+string(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo)))+`"`)
	assert.Contains(t, body, `"channel_id":40`)
	assert.Contains(t, body, `"group":"internal-group-a"`)
	assert.Contains(t, body, `"request_path":"/supplier/private/path"`)
	assert.Contains(t, body, `user_request_data`)
	assert.Contains(t, body, `upstream_response_data`)
	assert.Contains(t, body, `cgt-secret-a`)
}
