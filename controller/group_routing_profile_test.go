package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type groupRoutingProfileControllerFixture struct {
	DB         *gorm.DB
	TargetKeys []string
}

func TestPreviewGroupRoutingProfileTargetsReturnsNonSensitivePage(t *testing.T) {
	setupGroupRoutingProfileControllerTest(t)
	recorder := performGroupRoutingProfileRequest(t, PreviewGroupRoutingProfileTargets, `{
		"group_name":"客户A",
		"profile":{"status":"draft","routing_source":"default","allowed_cost_modes":["per_duration"]},
		"page":1,
		"page_size":25
	}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool                                  `json:"success"`
		Data    service.GroupRoutingProfileTargetPage `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Len(t, response.Data.Items, 2)
	assert.Equal(t, "supplier", response.Data.Items[0].ChannelName)
	assert.Equal(t, types.CostModePerDuration, response.Data.Items[0].CostMode)
	assert.NotZero(t, response.Data.Items[0].CostRuleID)
	assert.NotEmpty(t, response.Data.Items[0].TargetKey)
	assert.NotContains(t, recorder.Body.String(), "secret-channel-key")
	assert.NotContains(t, recorder.Body.String(), "secret-cost-payload")
	assert.NotContains(t, recorder.Body.String(), "config_json")
}

func TestPreviewGroupRoutingProfileTargetsRejectsInvalidPageSize(t *testing.T) {
	setupGroupRoutingProfileControllerTest(t)
	recorder := performGroupRoutingProfileRequest(t, PreviewGroupRoutingProfileTargets, `{
		"group_name":"客户A",
		"profile":{"status":"draft","routing_source":"default"},
		"page":1,
		"page_size":20
	}`)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Equal(t, service.GroupRoutingProfileErrorInvalid, response.Code)
}

func TestPreviewGroupRoutingProfileSummariesReturnsAllDynamicProfiles(t *testing.T) {
	setupGroupRoutingProfileControllerTest(t)
	recorder := performGroupRoutingProfileRequest(t, PreviewGroupRoutingProfileSummaries, `{
		"profiles":{
			"客户A":{"status":"draft","routing_source":"default","allowed_cost_modes":["per_duration"]},
			"客户B":{"status":"draft","routing_source":"default","allowed_cost_modes":["per_token"]},
			"legacy":{"require_real_person":true}
		}
	}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool                                          `json:"success"`
		Data    map[string]service.GroupRoutingProfileSummary `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.Equal(t, 2, response.Data["客户A"].MatchedTargets)
	assert.Zero(t, response.Data["客户B"].MatchedTargets)
	assert.NotContains(t, response.Data, "legacy")
}

func TestGroupRoutingProfileErrorsUseStableCodes(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		code       string
	}{
		{name: "invalid", err: &service.GroupRoutingProfileError{Code: service.GroupRoutingProfileErrorInvalid, Err: errors.New("invalid")}, statusCode: http.StatusBadRequest, code: service.GroupRoutingProfileErrorInvalid},
		{name: "unavailable", err: &service.GroupRoutingProfileError{Code: service.GroupRoutingProfileErrorUnavailable, Err: errors.New("unavailable")}, statusCode: http.StatusConflict, code: service.GroupRoutingProfileErrorUnavailable},
		{name: "internal", err: errors.New("database unavailable"), statusCode: http.StatusInternalServerError, code: service.GroupRoutingProfileErrorPreview},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			writeGroupRoutingProfileError(ctx, test.err)
			assert.Equal(t, test.statusCode, recorder.Code)
			var response struct {
				Code string `json:"code"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, test.code, response.Code)
		})
	}
}

func performGroupRoutingProfileRequest(t *testing.T, handler gin.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/routing-policies/group-profile/preview", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	handler(ctx)
	return recorder
}

func setupGroupRoutingProfileControllerTest(t *testing.T) groupRoutingProfileControllerFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalRedisEnabled := common.RedisEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	previousUsableGroups := setting.UserUsableGroups2JSONString()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","客户A":"客户 A","客户B":"客户 B"}`))
	require.NoError(t, db.AutoMigrate(
		&model.Channel{}, &model.Ability{}, &model.RoutingPolicy{}, &model.RouteTarget{},
		&model.ChannelModelCostRule{}, &model.Option{}, &model.User{}, &model.Log{},
	))
	t.Cleanup(func() {
		service.InvalidateCostCoverage(0, "", "")
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.RedisEnabled = originalRedisEnabled
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousUsableGroups))
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.Create(&model.Channel{
		Id: 7, Type: 1, Name: "supplier", Key: "secret-channel-key", Status: common.ChannelStatusEnabled,
	}).Error)
	priority := int64(100)
	require.NoError(t, db.Create(&model.Ability{
		Group: ratio_setting.GroupRoutingSourceDefault, Model: modelrouting.Seedance20,
		ChannelId: 7, Enabled: true, Priority: &priority, Weight: 10,
	}).Error)
	policy := model.RoutingPolicy{
		GroupName: ratio_setting.GroupRoutingSourceDefault, Model: modelrouting.Seedance20, Enabled: true,
		DefaultResolution: "720p", DefaultDuration: 10, DefaultRatio: "16:9",
	}
	require.NoError(t, db.Create(&policy).Error)
	supportsRealPerson := true
	minimumDuration := 4
	maximumDuration := 15
	constraints, err := common.Marshal(modelrouting.Constraints{
		OutputResolutions:  []string{"720p"},
		Durations:          modelrouting.DurationConstraint{Min: &minimumDuration, Max: &maximumDuration},
		AspectRatios:       []string{"16:9"},
		ReferenceLimits:    modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
		SupportsRealPerson: &supportsRealPerson,
	})
	require.NoError(t, err)
	targets := []model.RouteTarget{
		{PolicyID: policy.ID, ChannelID: 7, Name: "primary", UpstreamModel: "duration-primary", CostVariantKey: string(types.DefaultCostVariantKey), TargetPriority: 100, Enabled: true, Constraints: string(constraints)},
		{PolicyID: policy.ID, ChannelID: 7, Name: "secondary", UpstreamModel: "duration-secondary", CostVariantKey: string(types.DefaultCostVariantKey), TargetPriority: 50, Enabled: true, Constraints: string(constraints)},
	}
	require.NoError(t, db.Create(&targets).Error)
	now := common.GetTimestamp()
	for _, target := range targets {
		require.NoError(t, db.Create(&model.ChannelModelCostRule{
			ChannelID: 7, BillableUpstreamModel: target.UpstreamModel, CostVariantKey: string(types.DefaultCostVariantKey),
			Version: 1, Status: string(types.CostRuleActive), CostMode: string(types.CostModePerDuration), SchemaVersion: 1,
			ConfigJSON: `{"source":"secret-cost-payload"}`, Source: "manual", EffectiveFrom: &now, CreatedAt: now, UpdatedAt: now,
		}).Error)
	}
	service.InvalidateCostCoverage(0, "", "")
	targetKeys := make([]string, 0, len(targets))
	for _, target := range targets {
		targetKeys = append(targetKeys, service.GroupRoutingTargetKey(ratio_setting.GroupRoutingSourceDefault, modelrouting.Seedance20, modelrouting.Target{
			ChannelID: target.ChannelID, Name: target.Name, UpstreamModel: target.UpstreamModel, CostVariantKey: target.CostVariantKey,
		}))
	}
	return groupRoutingProfileControllerFixture{DB: db, TargetKeys: targetKeys}
}
