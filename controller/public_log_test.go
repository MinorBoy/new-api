package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupPublicLogControllerDB(t *testing.T) {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	logs := []model.Log{
		{
			UserId: 10, CreatedAt: 200, Type: model.LogTypeConsume,
			TokenId: 7, TokenName: "user-token", ModelName: "public-model",
			ChannelId: 40, Group: "internal-a", RequestId: "req-a",
			UpstreamRequestId: "upstream-a", Quota: 100,
			Other: common.MapToJsonStr(map[string]interface{}{
				"cache_tokens": 3,
				"model_price":  0.2,
				"admin_info": map[string]interface{}{
					"cost_accounting_request_id": 91,
				},
			}),
		},
		{
			UserId: 10, CreatedAt: 100, Type: model.LogTypeConsume,
			Username: "alice", TokenId: 7, TokenName: "user-token", ModelName: "public-model-b",
			ChannelId: 41, Group: "internal-b", RequestId: "req-b",
			UpstreamRequestId: "upstream-b", Quota: 200,
		},
		{
			UserId: 11, CreatedAt: 300, Type: model.LogTypeConsume,
			TokenId: 8, TokenName: "other-token", ModelName: "other-model",
			ChannelId: 42, Group: "internal-c", RequestId: "req-c",
			UpstreamRequestId: "upstream-c", Quota: 300,
		},
	}
	require.NoError(t, db.Create(&logs).Error)
}

func TestGetLogsSelfStatIgnoresSupplierDimensions(t *testing.T) {
	setupPublicLogControllerDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("username", "alice")
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/log/self/stat?type=2&channel=40&group=internal-a",
		nil,
	)

	GetLogsSelfStat(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"quota":200`)
}

func TestGetLogSelfModelsIgnoresSupplierDimensions(t *testing.T) {
	setupPublicLogControllerDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 10)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/log/self/models?type=2&channel=40&group=internal-a",
		nil,
	)

	GetLogSelfModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"public-model"`)
	assert.Contains(t, body, `"public-model-b"`)
}

func assertPublicLogPayload(t *testing.T, body string) {
	t.Helper()
	for _, forbidden := range []string{
		`"user_id"`, `"username"`, `"token_id"`, `"channel"`,
		`"channel_name"`, `"group"`, `"ip"`, `"upstream_request_id"`,
		`"model_price"`, `"admin_info"`, `"cost_accounting_request_id"`,
		"upstream-a", "upstream-b", "internal-a", "internal-b",
	} {
		assert.NotContains(t, body, forbidden)
	}
	assert.Contains(t, body, `"request_id":"req-a"`)
	assert.Contains(t, body, `"model_name":"public-model"`)
}

func TestGetUserLogsIgnoresSupplierFiltersAndReturnsPublicContract(t *testing.T) {
	setupPublicLogControllerDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 10)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/log/self?p=1&page_size=20&channel=999&group=internal-a&upstream_request_id=upstream-a",
		nil,
	)

	GetUserLogs(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"total":2`)
	assertPublicLogPayload(t, body)
}

func TestGetLogByKeyReturnsPublicContract(t *testing.T) {
	setupPublicLogControllerDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("token_id", 7)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log/token", nil)

	GetLogByKey(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assertPublicLogPayload(t, body)
}

func TestGetAllLogsKeepsAdministratorAuditContract(t *testing.T) {
	setupPublicLogControllerDB(t)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/log/?p=1&page_size=20&group=internal-a&upstream_request_id=upstream-a",
		nil,
	)

	GetAllLogs(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.True(t, strings.Contains(body, `"channel":40`), body)
	assert.Contains(t, body, `"group":"internal-a"`)
	assert.Contains(t, body, `"upstream_request_id":"upstream-a"`)
	assert.Contains(t, body, `model_price`)
}
