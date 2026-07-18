package relay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSeedanceTaskDB(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&model.Task{}))
	model.DB = database
	t.Cleanup(func() { model.DB = originalDB })
}

func TestSeedanceTaskFetchUsesPublicIDAndOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSeedanceTaskDB(t)
	now := time.Now().Unix()
	task := model.Task{
		TaskID:      "task_public",
		Platform:    constant.TaskPlatform("54"),
		UserId:      7,
		Status:      model.TaskStatusSuccess,
		SubmitTime:  now,
		UpdatedAt:   now + 10,
		Properties:  model.Properties{OriginModelName: "seedance-alias", UpstreamModelName: "doubao-seedance-2-0-260128"},
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-secret", ResultURL: "https://example.com/video.mp4"},
		Data:        json.RawMessage(`{"id":"cgt-secret","status":"succeeded","content":{}}`),
	}
	require.NoError(t, model.DB.Create(&task).Error)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_public", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "task_public"}}
	c.Set("id", 7)
	body, taskErr := SeedanceTaskFetch(c)
	require.Nil(t, taskErr)
	assert.NotContains(t, string(body), "cgt-secret")
	var response map[string]interface{}
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, "task_public", response["id"])
	assert.Equal(t, "succeeded", response["status"])
	assert.Equal(t, "seedance-alias", response["model"])
	content := response["content"].(map[string]interface{})
	assert.Equal(t, "https://example.com/video.mp4", content["video_url"])

	other, _ := gin.CreateTestContext(httptest.NewRecorder())
	other.Request = c.Request
	other.Params = c.Params
	other.Set("id", 8)
	_, taskErr = SeedanceTaskFetch(other)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusNotFound, taskErr.StatusCode)
}

func TestSeedanceTaskFetchPreservesOfficialFailedTaskFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSeedanceTaskDB(t)
	task := model.Task{
		TaskID:     "task_public",
		Platform:   constant.TaskPlatform("54"),
		UserId:     7,
		Status:     model.TaskStatusFailure,
		SubmitTime: 111,
		UpdatedAt:  222,
		Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "cgt-20260717171624-cr2n9",
		},
		Data: json.RawMessage(`{
			"id":"cgt-20260717171624-cr2n9",
			"model":"doubao-seedance-2-0-260128",
			"status":"failed",
			"error":{
				"code":"OutputVideoSensitiveContentDetected.PolicyViolation",
				"message":"The request failed because the output video may be related to copyright restrictions. Request id: 02178427978698300000000000000000000ffffac1923a9fc42b8"
			},
			"created_at":1784279786,
			"updated_at":1784280145,
			"service_tier":"default",
			"execution_expires_after":172800,
			"generate_audio":true,
			"draft":false,
			"priority":0
		}`),
	}
	require.NoError(t, model.DB.Create(&task).Error)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_public", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "task_public"}}
	c.Set("id", 7)
	body, taskErr := SeedanceTaskFetch(c)
	require.Nil(t, taskErr)
	assert.NotContains(t, string(body), "cgt-20260717171624-cr2n9")

	var response struct {
		ID                    string `json:"id"`
		Model                 string `json:"model"`
		Status                string `json:"status"`
		CreatedAt             int64  `json:"created_at"`
		UpdatedAt             int64  `json:"updated_at"`
		ServiceTier           string `json:"service_tier"`
		ExecutionExpiresAfter int    `json:"execution_expires_after"`
		GenerateAudio         bool   `json:"generate_audio"`
		Draft                 bool   `json:"draft"`
		Priority              int    `json:"priority"`
		Error                 struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, "task_public", response.ID)
	assert.Equal(t, "doubao-seedance-2-0-260128", response.Model)
	assert.Equal(t, "failed", response.Status)
	assert.Equal(t, int64(1784279786), response.CreatedAt)
	assert.Equal(t, int64(1784280145), response.UpdatedAt)
	assert.Equal(t, "default", response.ServiceTier)
	assert.Equal(t, 172800, response.ExecutionExpiresAfter)
	assert.True(t, response.GenerateAudio)
	assert.False(t, response.Draft)
	assert.Zero(t, response.Priority)
	assert.Equal(t, "OutputVideoSensitiveContentDetected.PolicyViolation", response.Error.Code)
	assert.Contains(t, response.Error.Message, "copyright restrictions")
}

func TestSeedanceTaskFetchPreservesOfficialExpiredAndCancelledStatuses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSeedanceTaskDB(t)
	for index, status := range []string{"expired", "cancelled"} {
		task := model.Task{
			TaskID:     "task_" + status,
			Platform:   constant.TaskPlatform("54"),
			UserId:     7,
			Status:     model.TaskStatusFailure,
			SubmitTime: 1784279786,
			Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
			Data:       json.RawMessage(`{"id":"cgt-secret-` + status + `","model":"doubao-seedance-2-0-260128","status":"` + status + `","created_at":1784279786,"updated_at":1784280145}`),
		}
		task.ID = int64(index + 1)
		require.NoError(t, model.DB.Create(&task).Error)

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/"+task.TaskID, nil)
		c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
		c.Set("id", 7)
		body, taskErr := SeedanceTaskFetch(c)
		require.Nil(t, taskErr)
		var response map[string]interface{}
		require.NoError(t, common.Unmarshal(body, &response))
		assert.Equal(t, status, response["status"])
	}
}

func TestSeedanceTaskListFiltersAndPaginatesJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSeedanceTaskDB(t)
	now := time.Now().Unix()
	tasks := make([]model.Task, 0, seedanceTaskScanBatchSize+5)
	for i := 0; i < seedanceTaskScanBatchSize+5; i++ {
		modelName := "other-model"
		if i%2 == 0 {
			modelName = "wanted-model"
		}
		tasks = append(tasks, model.Task{
			TaskID:     "task_" + time.Unix(int64(i), 0).Format("150405"),
			Platform:   constant.TaskPlatform("45"),
			UserId:     7,
			Status:     model.TaskStatusQueued,
			SubmitTime: now,
			Properties: model.Properties{OriginModelName: modelName},
			Data:       json.RawMessage(`{"service_tier":"default"}`),
		})
	}
	require.NoError(t, model.DB.CreateInBatches(&tasks, 50).Error)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks?filter.model=wanted-model&filter.service_tier=default&page_num=2&page_size=3", nil)
	c.Set("id", 7)
	body, taskErr := SeedanceTaskFetch(c)
	require.Nil(t, taskErr)
	var response seedanceTaskListResponse
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, 103, response.Total)
	assert.Len(t, response.Items, 3)
}

func TestSeedanceTaskListDefaultsMissingServiceTier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupSeedanceTaskDB(t)
	now := time.Now().Unix()
	task := model.Task{
		TaskID:     "task_default_tier",
		Platform:   constant.TaskPlatform("54"),
		UserId:     7,
		Status:     model.TaskStatusQueued,
		SubmitTime: now,
		Properties: model.Properties{OriginModelName: "wanted-model"},
		Data:       json.RawMessage(`{"id":"cgt-secret"}`),
	}
	require.NoError(t, model.DB.Create(&task).Error)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks?filter.service_tier=default", nil)
	c.Set("id", 7)
	body, taskErr := SeedanceTaskFetch(c)
	require.Nil(t, taskErr)
	var response seedanceTaskListResponse
	require.NoError(t, common.Unmarshal(body, &response))
	require.Equal(t, 1, response.Total)
	require.Len(t, response.Items, 1)
}

func TestSeedreamNativeImageBodyMapsOnlyModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/images/generations", strings.NewReader(`{"model":"alias","prompt":"x","watermark":false,"seed":0,"unknown":{"value":0}}`))
	c.Request.Header.Set("Content-Type", "application/json")
	_, err := common.GetBodyStorage(c)
	require.NoError(t, err)
	body, err := buildSeedreamImageRequestBody(c, "doubao-seedream-5-0-lite-260128")
	require.NoError(t, err)
	var fields map[string]interface{}
	require.NoError(t, common.Unmarshal(body, &fields))
	assert.Equal(t, "doubao-seedream-5-0-lite-260128", fields["model"])
	assert.Equal(t, false, fields["watermark"])
	assert.Equal(t, float64(0), fields["seed"])
	assert.Equal(t, map[string]interface{}{"value": float64(0)}, fields["unknown"])
}
