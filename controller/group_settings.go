package controller

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

type GroupSettingsUpdateRequest struct {
	GroupRatio               string `json:"group_ratio"`
	GroupStatus              string `json:"group_status"`
	TopupGroupRatio          string `json:"topup_group_ratio"`
	UserUsableGroups         string `json:"user_usable_groups"`
	GroupGroupRatio          string `json:"group_group_ratio"`
	AutoGroups               string `json:"auto_groups"`
	DefaultUseAutoGroup      bool   `json:"default_use_auto_group"`
	GroupSpecialUsableGroup  string `json:"group_special_usable_group"`
	GroupRoutingRequirements string `json:"group_routing_requirements"`
}

func UpdateGroupSettings(c *gin.Context) {
	var request GroupSettingsUpdateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "无效的分组设置")
		return
	}

	if err := ratio_setting.CheckGroupRatio(request.GroupRatio); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	groupRatios := make(map[string]float64)
	if err := common.UnmarshalJsonStr(request.GroupRatio, &groupRatios); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	groupStatuses, err := ratio_setting.ParseGroupStatusJSONString(request.GroupStatus)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	for group := range groupStatuses {
		if _, exists := groupRatios[group]; !exists {
			common.ApiErrorMsg(c, fmt.Sprintf("group status references unknown group: %s", group))
			return
		}
	}

	if err := validateGroupSettingsJSON(request); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := ratio_setting.CheckGroupRoutingRequirements(request.GroupRoutingRequirements); err != nil {
		writeOptionValidationError(c, err)
		return
	}
	if err := service.ValidateActiveGroupRoutingProfiles(request.GroupRoutingRequirements); err != nil {
		writeOptionValidationError(c, err)
		return
	}

	removedGroups := make([]string, 0)
	for group := range ratio_setting.GetGroupRatioCopy() {
		if _, exists := groupRatios[group]; !exists {
			removedGroups = append(removedGroups, group)
		}
	}
	sort.Strings(removedGroups)
	references, err := model.FindGroupReferences(model.DB, removedGroups)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if references.Users > 0 || references.Tokens > 0 {
		common.ApiErrorMsg(c, fmt.Sprintf(
			"cannot delete groups %v: referenced by %d users and %d tokens",
			removedGroups,
			references.Users,
			references.Tokens,
		))
		return
	}

	values := map[string]string{
		"GroupRatio":                       request.GroupRatio,
		"group_ratio_setting.group_status": request.GroupStatus,
		"TopupGroupRatio":                  request.TopupGroupRatio,
		"UserUsableGroups":                 request.UserUsableGroups,
		"GroupGroupRatio":                  request.GroupGroupRatio,
		"AutoGroups":                       request.AutoGroups,
		"DefaultUseAutoGroup":              strconv.FormatBool(request.DefaultUseAutoGroup),
		"group_ratio_setting.group_special_usable_group": request.GroupSpecialUsableGroup,
		"GroupRoutingRequirements":                       request.GroupRoutingRequirements,
	}
	if err := model.UpdateOptionsBulk(values); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "group_settings.update", map[string]interface{}{
		"removed_groups": removedGroups,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

func validateGroupSettingsJSON(request GroupSettingsUpdateRequest) error {
	values := []struct {
		raw    string
		target any
	}{
		{request.TopupGroupRatio, &map[string]float64{}},
		{request.UserUsableGroups, &map[string]string{}},
		{request.GroupGroupRatio, &map[string]map[string]float64{}},
		{request.AutoGroups, &[]string{}},
		{request.GroupSpecialUsableGroup, &map[string]map[string]string{}},
	}
	for _, value := range values {
		if err := common.UnmarshalJsonStr(value.raw, value.target); err != nil {
			return err
		}
	}
	return nil
}
