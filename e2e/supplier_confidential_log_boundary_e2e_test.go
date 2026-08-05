package e2e

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/controller"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	confidentialBoundaryUserID      = 4101
	confidentialBoundaryAdminID     = 4102
	confidentialBoundaryTokenID     = 4201
	confidentialBoundaryCostID      = 4301
	confidentialBoundaryAttemptID   = 4302
	confidentialBoundaryTaskDBID    = 4401
	confidentialBoundaryUserPAT     = "userboundaryaccess0000000000000001"
	confidentialBoundaryAdminPAT    = "adminboundaryaccess00000000000001"
	confidentialBoundaryAPIToken    = "boundarytoken"
	confidentialBoundaryRequestID   = "req-public-boundary"
	confidentialBoundaryTaskID      = "task-public-boundary"
	confidentialBoundaryPublicModel = "doubao-seedance-2-0-260128"
	confidentialBoundaryChannel     = "supplier-channel-secret"
	confidentialBoundaryModel       = "provider-model-secret"
	confidentialBoundaryGroup       = "internal-group-secret"
	confidentialBoundaryUpstreamID  = "cgt-private-secret"
)

func confidentialBoundaryRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/api/log/self", middleware.UserAuth(), controller.GetUserLogs)
	engine.GET("/api/log/token", middleware.TokenAuthReadOnly(), controller.GetLogByKey)
	engine.GET("/api/log/", middleware.AdminAuth(), controller.GetAllLogs)
	engine.GET("/api/task/self", middleware.UserAuth(), controller.GetUserTask)
	engine.GET("/api/task/", middleware.AdminAuth(), controller.GetAllTask)
	engine.GET(
		"/api/cost-accounting/requests/:id",
		middleware.AdminAuth(),
		middleware.RequirePermission(authz.CostAccountingRead),
		controller.GetCostAccountingRequest,
	)
	return engine
}

func seedConfidentialBoundaryData(t *testing.T) {
	t.Helper()
	setupSeedanceE2EDB(t)
	require.NoError(t, appI18n.Init())
	require.NoError(t, model.DB.AutoMigrate(
		&model.CostAccountingRequest{},
		&model.CostAccountingAttempt{},
		&model.CostAccountingAudit{},
	))

	previousServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "http://gateway.example"
	t.Cleanup(func() { system_setting.ServerAddress = previousServerAddress })

	userPAT := confidentialBoundaryUserPAT
	adminPAT := confidentialBoundaryAdminPAT
	users := []model.User{
		{
			Id: confidentialBoundaryUserID, Username: "boundary_user", Password: "e2e-password",
			AccessToken: &userPAT, Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
			Quota: 2_000_000_000, Group: "default", AffCode: "boundary-user-aff",
		},
		{
			Id: confidentialBoundaryAdminID, Username: "boundary_admin", Password: "e2e-password",
			AccessToken: &adminPAT, Role: common.RoleRootUser, Status: common.UserStatusEnabled,
			Quota: 2_000_000_000, Group: "default", AffCode: "boundary-admin-aff",
		},
	}
	require.NoError(t, model.DB.Create(&users).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		Id: confidentialBoundaryTokenID, UserId: confidentialBoundaryUserID,
		Key: confidentialBoundaryAPIToken, Name: "public token", Status: common.TokenStatusEnabled,
		RemainQuota: 2_000_000_000, UnlimitedQuota: true, Group: "default",
	}).Error)
	priority := int64(1)
	require.NoError(t, (&model.Channel{
		Id: 40, Type: constant.ChannelTypeNewAPIVideo,
		Key: "supplier-api-key-secret", BaseURL: common.GetPointer("https://supplier.example"),
		Status: common.ChannelStatusEnabled, Name: confidentialBoundaryChannel,
		Weight: common.GetPointer[uint](1), Priority: &priority,
		Models: confidentialBoundaryPublicModel, Group: "default",
		CreatedTime: 1785900000, OtherSettings: "{}",
	}).Insert())

	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId: confidentialBoundaryUserID, Username: "boundary_user",
		CreatedAt: 1785900000, Type: model.LogTypeConsume,
		Content: confidentialBoundaryModel + " https://supplier.example/private",
		TokenId: confidentialBoundaryTokenID, TokenName: "public token",
		ModelName: confidentialBoundaryPublicModel, Quota: 125000,
		PromptTokens: 10, CompletionTokens: 20, UseTime: 3,
		ChannelId: 40, ChannelName: confidentialBoundaryChannel,
		Group: confidentialBoundaryGroup, Ip: "10.0.0.8",
		RequestId: confidentialBoundaryRequestID, UpstreamRequestId: "upstream-request-secret",
		Other: common.MapToJsonStr(map[string]interface{}{
			"cache_tokens":        3,
			"model_price":         0.2,
			"group_ratio":         1.25,
			"upstream_model_name": confidentialBoundaryModel,
			"admin_info": map[string]interface{}{
				"cost_accounting_request_id": confidentialBoundaryCostID,
				"use_channel":                []int{40},
			},
		}),
	}).Error)

	platform := constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo))
	require.NoError(t, model.DB.Create(&model.Task{
		ID: confidentialBoundaryTaskDBID, TaskID: confidentialBoundaryTaskID,
		Platform: platform, UserId: confidentialBoundaryUserID,
		Group: confidentialBoundaryGroup, ChannelId: 40, Quota: 125000,
		Action: "generate", Status: model.TaskStatusSuccess,
		CreatedAt: 1785900000, UpdatedAt: 1785900005,
		SubmitTime: 1785900000, StartTime: 1785900001, FinishTime: 1785900005, Progress: "100%",
		Properties: model.Properties{
			OriginModelName:   confidentialBoundaryPublicModel,
			UpstreamModelName: confidentialBoundaryModel,
			RequestPath:       "/supplier/private/path",
		},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID:       confidentialBoundaryUpstreamID,
			ResultURL:            "https://supplier.example/private.mp4",
			UserRequestData:      []byte(`{"model":"doubao-seedance-2-0-260128","resolution":"720p","ratio":"16:9","duration":5,"generate_audio":true}`),
			UpstreamResponseData: []byte(`{"id":"cgt-private-secret","base_url":"https://supplier.example"}`),
			UserResponseData:     []byte(`{"id":"cgt-private-secret","model":"provider-model-secret","status":"succeeded","content":{"video_url":"https://supplier.example/private.mp4"},"usage":{"completion_tokens":108900,"total_tokens":108900},"created_at":1779348818,"updated_at":1779348874,"seed":78674,"resolution":"720p","ratio":"16:9","duration":5,"framespersecond":24,"service_tier":"default","execution_expires_after":172800,"generate_audio":true,"draft":false,"priority":0}`),
			BillingContext: &model.TaskBillingContext{
				UsageProfile:    model.TaskUsageProfileSeedance,
				OriginModelName: confidentialBoundaryPublicModel,
			},
		},
	}).Error)

	winningAttemptID := int64(confidentialBoundaryAttemptID)
	finalQuota := int64(125000)
	revenue := int64(724932000)
	profit := int64(327671726)
	margin := int64(452003)
	taskID := confidentialBoundaryTaskID
	upstreamTaskID := confidentialBoundaryUpstreamID
	require.NoError(t, model.DB.Create(&model.CostAccountingRequest{
		ID: confidentialBoundaryCostID, RequestID: confidentialBoundaryRequestID,
		TaskID: &taskID, UpstreamTaskID: &upstreamTaskID,
		UserID: confidentialBoundaryUserID, TokenID: confidentialBoundaryTokenID,
		UserGroup: "default", UsingGroup: confidentialBoundaryGroup,
		OriginModelName: confidentialBoundaryPublicModel, BillingSource: "quota",
		FinalUserQuota: &finalQuota, QuotaPerUnitSnapshot: "500000",
		BilledRevenueEquivalentNanoUSD: &revenue, ConfirmedCostNanoUSD: 397260274,
		AttemptCount: 1, WinningAttemptID: &winningAttemptID,
		BilledGrossProfitNanoUSD: &profit, GrossMarginPPM: &margin,
		RevenueStatus: "settled", ProfitStatus: "complete",
		RequestedAt: 1785900000, CreatedAt: 1785900000, UpdatedAt: 1785900005,
	}).Error)
	costNanoUSD := int64(397260274)
	require.NoError(t, model.DB.Create(&model.CostAccountingAttempt{
		ID: confidentialBoundaryAttemptID, CostRequestID: confidentialBoundaryCostID,
		AttemptNo: 1, ChannelID: 40, ChannelName: confidentialBoundaryChannel,
		ChannelType:            constant.ChannelTypeNewAPIVideo,
		PredictedUpstreamModel: confidentialBoundaryModel,
		BillableUpstreamModel:  confidentialBoundaryModel,
		CostVariantKey:         "default", RuleID: 99, RuleVersion: 2,
		CostMode: "per_token", SchemaVersion: 1,
		RuleConfigJSON: `{"currency":"CNY","unit_price":"2.9"}`,
		ChargeEvent:    "task_success", MeterSource: "validated_request",
		BillableRequestCount: 1,
		RequestMeterJSON:     `{"completion_tokens":108900}`,
		ActualMeterJSON:      `{"completion_tokens":108900}`,
		OriginalCost:         "2.9", CostNanoUSD: &costNanoUSD,
		UpstreamAccepted: true, HTTPStatus: http.StatusOK,
		Status: "settled", ReconciliationStatus: model.CostReconciliationNone,
		PreparedAt: 1785900000, CreatedAt: 1785900000, UpdatedAt: 1785900005,
	}).Error)
	attemptID := int64(confidentialBoundaryAttemptID)
	require.NoError(t, model.DB.Create(&model.CostAccountingAudit{
		CostRequestID: confidentialBoundaryCostID, CostAttemptID: &attemptID,
		AdminID: confidentialBoundaryAdminID, OldState: "prepared", NewState: "settled",
		MeterJSON: `{"completion_tokens":108900}`, RuleID: 99, RuleVersion: 2,
		Reason: "supplier-audit-secret", CreatedAt: 1785900005,
	}).Error)
}

func performConfidentialBoundaryRequest(
	t *testing.T,
	engine http.Handler,
	path string,
	authorization string,
) (int, []byte) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Authorization", "Bearer "+authorization)
	request.Header.Set("Accept", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder.Code, recorder.Body.Bytes()
}

func assertNoSupplierFacts(t *testing.T, payload []byte) {
	t.Helper()
	var value interface{}
	require.NoError(t, common.Unmarshal(payload, &value))
	forbiddenKeys := map[string]struct{}{
		"user_id": {}, "username": {}, "platform": {}, "group": {},
		"channel": {}, "channel_id": {}, "channel_name": {}, "ip": {},
		"token_id": {}, "upstream_request_id": {}, "upstream_model_name": {},
		"upstream_response_data": {}, "user_request_data": {}, "request_path": {},
		"model_price": {}, "duration_price": {}, "group_ratio": {},
		"admin_info": {}, "audit_info": {}, "cost_accounting_request_id": {},
		"rule_config_json": {}, "original_cost": {},
	}
	forbiddenValues := []string{
		confidentialBoundaryChannel,
		confidentialBoundaryModel,
		confidentialBoundaryGroup,
		confidentialBoundaryUpstreamID,
		"upstream-request-secret",
		"supplier.example",
		"supplier-audit-secret",
	}
	assertConfidentialBoundaryValue(t, value, "$", forbiddenKeys, forbiddenValues)
}

func assertConfidentialBoundaryValue(
	t *testing.T,
	value interface{},
	path string,
	forbiddenKeys map[string]struct{},
	forbiddenValues []string,
) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, child := range typed {
			_, forbidden := forbiddenKeys[key]
			assert.False(t, forbidden, "forbidden key %s at %s", key, path)
			assertConfidentialBoundaryValue(t, child, path+"."+key, forbiddenKeys, forbiddenValues)
		}
	case []interface{}:
		for index, child := range typed {
			assertConfidentialBoundaryValue(t, child, path+"["+strconv.Itoa(index)+"]", forbiddenKeys, forbiddenValues)
		}
	case string:
		for _, forbidden := range forbiddenValues {
			assert.NotContains(t, typed, forbidden, "forbidden value at %s", path)
		}
	}
}

func TestSupplierConfidentialLogBoundaryE2E(t *testing.T) {
	seedConfidentialBoundaryData(t)
	engine := confidentialBoundaryRouter()

	publicRequests := []struct {
		name          string
		path          string
		authorization string
	}{
		{
			name:          "usage self",
			path:          "/api/log/self?p=1&page_size=20&channel=40&group=" + confidentialBoundaryGroup + "&upstream_request_id=upstream-request-secret",
			authorization: confidentialBoundaryUserPAT,
		},
		{
			name:          "usage token",
			path:          "/api/log/token?p=1&page_size=20",
			authorization: confidentialBoundaryAPIToken,
		},
		{
			name:          "task self",
			path:          "/api/task/self?p=1&page_size=20&platform=probe&channel_id=40&group=" + confidentialBoundaryGroup,
			authorization: confidentialBoundaryUserPAT,
		},
	}

	for _, request := range publicRequests {
		t.Run(request.name, func(t *testing.T) {
			status, body := performConfidentialBoundaryRequest(t, engine, request.path, request.authorization)
			require.Equal(t, http.StatusOK, status, string(body))
			assertNoSupplierFacts(t, body)
		})
	}

	status, publicLogBody := performConfidentialBoundaryRequest(
		t, engine, "/api/log/self?p=1&page_size=20", confidentialBoundaryUserPAT,
	)
	require.Equal(t, http.StatusOK, status, string(publicLogBody))
	assert.Contains(t, string(publicLogBody), `"request_id":"`+confidentialBoundaryRequestID+`"`)
	assert.Contains(t, string(publicLogBody), `"model_name":"`+confidentialBoundaryPublicModel+`"`)
	assert.Contains(t, string(publicLogBody), `"content":""`)

	status, publicTaskBody := performConfidentialBoundaryRequest(
		t, engine, "/api/task/self?p=1&page_size=20", confidentialBoundaryUserPAT,
	)
	require.Equal(t, http.StatusOK, status, string(publicTaskBody))
	publicTaskText := string(publicTaskBody)
	for _, field := range []string{
		`"usage"`, `"created_at"`, `"updated_at"`, `"seed"`, `"resolution"`,
		`"ratio"`, `"duration"`, `"framespersecond"`, `"service_tier"`,
		`"execution_expires_after"`, `"generate_audio"`, `"draft"`, `"priority"`,
	} {
		assert.Contains(t, publicTaskText, field)
	}
	assert.Contains(t, publicTaskText, "http://gateway.example/v1/videos/"+confidentialBoundaryTaskID+"/content")

	status, adminLogBody := performConfidentialBoundaryRequest(
		t, engine, "/api/log/?p=1&page_size=20&request_id="+confidentialBoundaryRequestID, confidentialBoundaryAdminPAT,
	)
	require.Equal(t, http.StatusOK, status, string(adminLogBody))
	adminLogText := string(adminLogBody)
	assert.Contains(t, adminLogText, confidentialBoundaryChannel)
	assert.Contains(t, adminLogText, confidentialBoundaryGroup)
	assert.Contains(t, adminLogText, "upstream-request-secret")
	assert.Contains(t, adminLogText, confidentialBoundaryModel)

	status, adminTaskBody := performConfidentialBoundaryRequest(
		t, engine, "/api/task/?p=1&page_size=20&task_id="+confidentialBoundaryTaskID, confidentialBoundaryAdminPAT,
	)
	require.Equal(t, http.StatusOK, status, string(adminTaskBody))
	adminTaskText := string(adminTaskBody)
	assert.Contains(t, adminTaskText, confidentialBoundaryGroup)
	assert.Contains(t, adminTaskText, confidentialBoundaryModel)
	assert.Contains(t, adminTaskText, confidentialBoundaryUpstreamID)
	assert.Contains(t, adminTaskText, "supplier.example")

	status, deniedCostBody := performConfidentialBoundaryRequest(
		t, engine, "/api/cost-accounting/requests/"+strconv.Itoa(confidentialBoundaryCostID), confidentialBoundaryUserPAT,
	)
	require.Equal(t, http.StatusForbidden, status, string(deniedCostBody))

	status, adminCostBody := performConfidentialBoundaryRequest(
		t, engine, "/api/cost-accounting/requests/"+strconv.Itoa(confidentialBoundaryCostID), confidentialBoundaryAdminPAT,
	)
	require.Equal(t, http.StatusOK, status, string(adminCostBody))
	adminCostText := string(adminCostBody)
	assert.Contains(t, adminCostText, confidentialBoundaryChannel)
	assert.Contains(t, adminCostText, confidentialBoundaryModel)
	assert.Contains(t, adminCostText, "supplier-audit-secret")
	assert.Contains(t, adminCostText, "unit_price")
	assert.Contains(t, adminCostText, "2.9")
}
