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
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpdateGroupSettingsRejectsReferencedDeletionWithoutPersistence(t *testing.T) {
	db := setupGroupSettingsControllerTest(t)
	require.NoError(t, db.Create(&model.User{Username: "paused-user", Group: "paused", AffCode: "paused-aff"}).Error)

	recorder := performGroupSettingsUpdate(t, validGroupSettingsBody(`{"default":1}`, `{"default":true}`))
	response := decodeGroupSettingsResponse(t, recorder)
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "paused")

	var option model.Option
	require.NoError(t, db.First(&option, "key = ?", "GroupRatio").Error)
	assert.JSONEq(t, `{"default":1,"paused":1}`, option.Value)
	assert.JSONEq(t, `{"default":1,"paused":1}`, ratio_setting.GroupRatio2JSONString())
}

func TestUpdateGroupSettingsRejectsOrphanStatus(t *testing.T) {
	setupGroupSettingsControllerTest(t)

	recorder := performGroupSettingsUpdate(t, validGroupSettingsBody(
		`{"default":1}`,
		`{"default":true,"orphan":false}`,
	))
	response := decodeGroupSettingsResponse(t, recorder)
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "orphan")
}

func TestUpdateGroupSettingsPersistsCompleteSnapshot(t *testing.T) {
	db := setupGroupSettingsControllerTest(t)

	recorder := performGroupSettingsUpdate(t, validGroupSettingsBody(
		`{"default":1,"paused":1}`,
		`{"default":true,"paused":false}`,
	))
	response := decodeGroupSettingsResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	assert.False(t, ratio_setting.IsGroupEnabled("paused"))

	expected := map[string]string{
		"GroupRatio":                       `{"default":1,"paused":1}`,
		"group_ratio_setting.group_status": `{"default":true,"paused":false}`,
		"TopupGroupRatio":                  `{}`,
		"UserUsableGroups":                 `{}`,
		"GroupGroupRatio":                  `{}`,
		"AutoGroups":                       `[]`,
		"DefaultUseAutoGroup":              `false`,
		"group_ratio_setting.group_special_usable_group": `{}`,
		"GroupRoutingRequirements":                       `{}`,
	}
	for key, value := range expected {
		var option model.Option
		require.NoError(t, db.First(&option, "key = ?", key).Error)
		assert.JSONEq(t, value, option.Value, key)
	}
}

func setupGroupSettingsControllerTest(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.User{}, &model.Token{}, &model.Log{}))

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalRedisEnabled := common.RedisEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalRatio := ratio_setting.GroupRatio2JSONString()
	originalStatus := ratio_setting.GroupStatus2JSONString()
	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "admin", Group: "default", AffCode: "admin-aff"}).Error)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"paused":1}`))
	require.NoError(t, ratio_setting.UpdateGroupStatusByJSONString(`{"default":true,"paused":true}`))
	require.NoError(t, model.UpdateOptionsWithTx(db, map[string]string{
		"GroupRatio":                       `{"default":1,"paused":1}`,
		"group_ratio_setting.group_status": `{"default":true,"paused":true}`,
	}))
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.RedisEnabled = originalRedisEnabled
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatio))
		require.NoError(t, ratio_setting.UpdateGroupStatusByJSONString(originalStatus))
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func validGroupSettingsBody(groupRatio, groupStatus string) string {
	return fmt.Sprintf(`{
		"group_ratio":%q,
		"group_status":%q,
		"topup_group_ratio":"{}",
		"user_usable_groups":"{}",
		"group_group_ratio":"{}",
		"auto_groups":"[]",
		"default_use_auto_group":false,
		"group_special_usable_group":"{}",
		"group_routing_requirements":"{}"
	}`, groupRatio, groupStatus)
}

func performGroupSettingsUpdate(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Set("username", "admin")
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/option/group-settings", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	UpdateGroupSettings(ctx)
	return recorder
}

func decodeGroupSettingsResponse(t *testing.T, recorder *httptest.ResponseRecorder) struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
} {
	t.Helper()
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}
