package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRoutingPolicyControllerLifecycle(t *testing.T) {
	prepareRoutingPolicyControllerTest(t)
	request := controllerRoutingPolicyRequest(t)
	create := performRoutingPolicyRequest(t, http.MethodPost, "/", "", request, CreateRoutingPolicy)
	assert.Equal(t, http.StatusCreated, create.Code)
	var createBody struct {
		Success bool                      `json:"success"`
		Data    service.RoutingPolicyView `json:"data"`
	}
	require.NoError(t, common.Unmarshal(create.Body.Bytes(), &createBody))
	require.True(t, createBody.Success)
	require.NotZero(t, createBody.Data.ID)

	id := strconv.Itoa(createBody.Data.ID)
	get := performRoutingPolicyRequest(t, http.MethodGet, "/"+id, id, nil, GetRoutingPolicy)
	assert.Equal(t, http.StatusOK, get.Code)
	assert.Contains(t, get.Body.String(), modelrouting.Seedance20)
	assert.NotContains(t, get.Body.String(), "secret")

	request.Defaults.DurationSeconds = 15
	update := performRoutingPolicyRequest(t, http.MethodPut, "/"+id, id, request, UpdateRoutingPolicy)
	assert.Equal(t, http.StatusOK, update.Code)
	assert.Contains(t, update.Body.String(), `"duration_seconds":15`)

	status := performRoutingPolicyRequest(t, http.MethodPost, "/"+id+"/status", id, map[string]bool{"enabled": false}, UpdateRoutingPolicyStatus)
	assert.Equal(t, http.StatusOK, status.Code)
	assert.Contains(t, status.Body.String(), `"enabled":false`)

	deleteResponse := performRoutingPolicyRequest(t, http.MethodDelete, "/"+id, id, nil, DeleteRoutingPolicy)
	assert.Equal(t, http.StatusOK, deleteResponse.Code)
	var count int64
	require.NoError(t, model.DB.Model(&model.RoutingPolicy{}).Where("id = ?", createBody.Data.ID).Count(&count).Error)
	assert.Zero(t, count)
}

func TestRoutingPolicyControllerReturnsStructuredOverlapError(t *testing.T) {
	prepareRoutingPolicyControllerTest(t)
	request := controllerRoutingPolicyRequest(t)
	second := request.Targets[0]
	second.Name = "second"
	second.UpstreamModel = "provider-second"
	request.Targets = append(request.Targets, second)

	response := performRoutingPolicyRequest(t, http.MethodPost, "/", "", request, CreateRoutingPolicy)
	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.JSONEq(t, `{"success":false,"message":"targets overlap at the same channel priority","code":"routing_target_overlap","data":{"field":"targets","target_indexes":[0,1]}}`, response.Body.String())
}

func prepareRoutingPolicyControllerTest(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.RoutingPolicy{}, &model.RouteTarget{}, &model.Log{}))
	priority := int64(100)
	weight := uint(10)
	require.NoError(t, db.Create(&model.Channel{Id: 11, Name: "A1", Key: "secret", Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "分组A", Model: modelrouting.Seedance20, ChannelId: 11, Enabled: true, Priority: &priority, Weight: weight}).Error)
	require.NoError(t, model.InitRoutingPolicyCache())
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		require.NoError(t, sqlDB.Close())
	})
}

func controllerRoutingPolicyRequest(t *testing.T) service.RoutingPolicyWriteRequest {
	t.Helper()
	supportsRealPerson := true
	minDuration := 4
	maxDuration := 15
	return service.RoutingPolicyWriteRequest{
		GroupName: "分组A",
		Model:     modelrouting.Seedance20,
		Enabled:   true,
		Defaults:  modelrouting.Defaults{OutputResolution: "720p", DurationSeconds: 10, AspectRatio: "16:9"},
		Targets: []service.RouteTargetWriteRequest{{
			ChannelID: 11, Name: "A1", UpstreamModel: "provider-model", TargetPriority: 100, Enabled: true,
			Constraints: modelrouting.Constraints{
				OutputResolutions:  []string{"720p"},
				Durations:          modelrouting.DurationConstraint{Min: &minDuration, Max: &maxDuration},
				ReferenceLimits:    modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
				SupportsRealPerson: &supportsRealPerson,
			},
		}},
	}
}

func performRoutingPolicyRequest(t *testing.T, method, path, id string, body any, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	var requestBody *bytes.Reader
	if body == nil {
		requestBody = bytes.NewReader(nil)
	} else {
		encoded, err := common.Marshal(body)
		require.NoError(t, err)
		requestBody = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, requestBody)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = request
	c.Set("id", 1)
	c.Set("username", "admin")
	if id != "" {
		c.Params = gin.Params{{Key: "id", Value: id}}
	}
	handler(c)
	return recorder
}
