package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetGroupsExcludesDisabledGroups(t *testing.T) {
	restoreGroupStatusControllerState(t)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"paused":1}`))
	require.NoError(t, ratio_setting.UpdateGroupStatusByJSONString(`{"paused":false}`))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/group/", nil)
	GetGroups(ctx)

	var response struct {
		Success bool     `json:"success"`
		Data    []string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.Contains(t, response.Data, "default")
	assert.NotContains(t, response.Data, "paused")
}

func TestValidateAssignableGroupRejectsDisabledAndUnknownGroups(t *testing.T) {
	restoreGroupStatusControllerState(t)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"paused":1}`))
	require.NoError(t, ratio_setting.UpdateGroupStatusByJSONString(`{"paused":false}`))

	require.NoError(t, validateAssignableGroup(""))
	require.NoError(t, validateAssignableGroup("auto"))
	require.NoError(t, validateAssignableGroup("default"))
	require.ErrorContains(t, validateAssignableGroup("paused"), "paused")
	require.ErrorContains(t, validateAssignableGroup("missing"), "missing")
}

func TestAddTokenRejectsDisabledGroupBeforePersistence(t *testing.T) {
	restoreGroupStatusControllerState(t)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"paused":1}`))
	require.NoError(t, ratio_setting.UpdateGroupStatusByJSONString(`{"paused":false}`))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 7)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/token/", strings.NewReader(`{
		"name":"paused token","group":"paused","unlimited_quota":true,"expired_time":-1
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	AddToken(ctx)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "paused")
}

func restoreGroupStatusControllerState(t *testing.T) {
	t.Helper()
	originalRatio := ratio_setting.GroupRatio2JSONString()
	originalStatus := ratio_setting.GroupStatus2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
		require.NoError(t, ratio_setting.UpdateGroupStatusByJSONString(originalStatus))
	})
}
