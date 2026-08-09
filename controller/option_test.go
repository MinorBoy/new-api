package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpdateOptionRejectsUnavailableActiveGroupRoutingProfileBeforePersistence(t *testing.T) {
	fixture := setupGroupRoutingProfileControllerTest(t)
	const optionKey = "GroupRoutingRequirements"
	const originalValue = `{}`
	setGroupRoutingRequirementsOptionFixture(t, fixture.DB, originalValue)

	recorder := performGroupRoutingRequirementsOptionUpdate(t, `{
		"客户A":{"status":"active","routing_source":"default","allowed_cost_modes":["per_token"]}
	}`)
	response := decodeOptionUpdateResponse(t, recorder)
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "客户A")

	var persisted model.Option
	require.NoError(t, fixture.DB.First(&persisted, "key = ?", optionKey).Error)
	assert.Equal(t, originalValue, persisted.Value)
	assert.Equal(t, originalValue, ratio_setting.GroupRoutingRequirements2JSONString())
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, originalValue, common.OptionMap[optionKey])
	common.OptionMapRWMutex.RUnlock()
}

func TestUpdateOptionAuditsGroupRoutingProfileChangeSummaryWithoutTargetKeys(t *testing.T) {
	fixture := setupGroupRoutingProfileControllerTest(t)
	const optionKey = "GroupRoutingRequirements"
	originalValue := fmt.Sprintf(`{
		"客户A":{"status":"draft","routing_source":"default","allowed_cost_modes":["per_duration"],"excluded_target_keys":[%q,"grt_stale_old"]}
	}`, fixture.TargetKeys[0])
	setGroupRoutingRequirementsOptionFixture(t, fixture.DB, originalValue)
	require.NoError(t, fixture.DB.Create(&model.User{Id: 1, Username: "admin"}).Error)

	nextValue := fmt.Sprintf(`{
		"客户A":{"status":"active","routing_source":"default","allowed_cost_modes":["per_duration"],"excluded_target_keys":[%q,"grt_stale_new"]}
	}`, fixture.TargetKeys[1])
	recorder := performGroupRoutingRequirementsOptionUpdate(t, nextValue)
	response := decodeOptionUpdateResponse(t, recorder)
	require.True(t, response.Success)

	var log model.Log
	require.NoError(t, fixture.DB.Where("type = ?", model.LogTypeManage).Last(&log).Error)
	var other struct {
		Op struct {
			Action string         `json:"action"`
			Params map[string]any `json:"params"`
		} `json:"op"`
	}
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	assert.Equal(t, "option.update", other.Op.Action)
	assert.Equal(t, []any{"客户A"}, other.Op.Params["changed_groups"])
	assert.Equal(t, []any{"客户A"}, other.Op.Params["activated_groups"])
	assert.Equal(t, []any{}, other.Op.Params["draft_groups"])
	assert.Equal(t, float64(2), other.Op.Params["exclusions_added"])
	assert.Equal(t, float64(2), other.Op.Params["exclusions_removed"])
	assert.NotContains(t, log.Other, "grt_")
	assert.NotContains(t, log.Other, "excluded_target_keys")
}

func setGroupRoutingRequirementsOptionFixture(t *testing.T, db *gorm.DB, raw string) {
	t.Helper()
	previousProfiles := ratio_setting.GroupRoutingRequirements2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRoutingRequirementsByJSONString(raw))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRoutingRequirementsByJSONString(previousProfiles))
	})

	const optionKey = "GroupRoutingRequirements"
	require.NoError(t, db.Create(&model.Option{Key: optionKey, Value: raw}).Error)
	common.OptionMapRWMutex.Lock()
	optionMapWasNil := common.OptionMap == nil
	if optionMapWasNil {
		common.OptionMap = make(map[string]string)
	}
	previousValue, hadPreviousValue := common.OptionMap[optionKey]
	common.OptionMap[optionKey] = raw
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		if optionMapWasNil {
			common.OptionMap = nil
			return
		}
		if hadPreviousValue {
			common.OptionMap[optionKey] = previousValue
		} else {
			delete(common.OptionMap, optionKey)
		}
	})
}

func performGroupRoutingRequirementsOptionUpdate(t *testing.T, value string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Set("username", "admin")
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/option/", strings.NewReader(
		fmt.Sprintf(`{"key":"GroupRoutingRequirements","value":%q}`, value),
	))
	ctx.Request.Header.Set("Content-Type", "application/json")
	UpdateOption(ctx)
	return recorder
}
