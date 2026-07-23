package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func ListRoutingPolicies(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	channelID, _ := strconv.Atoi(c.Query("channel_id"))
	items, total, err := service.ListRoutingPolicyViews(
		c.Query("group_name"), c.Query("model"), channelID,
		pageInfo.GetStartIdx(), pageInfo.GetPageSize(),
	)
	if err != nil {
		writeRoutingPolicyError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"items": items, "total": total,
		"page": pageInfo.GetPage(), "page_size": pageInfo.GetPageSize(),
	})
}

func ListRoutingPolicyCandidates(c *gin.Context) {
	candidates, err := model.ListRoutingCandidates(c.Query("group_name"), c.Query("model"))
	if err != nil {
		writeRoutingPolicyError(c, err)
		return
	}
	common.ApiSuccess(c, candidates)
}

func GetRoutingPolicy(c *gin.Context) {
	id, ok := routingPolicyID(c)
	if !ok {
		return
	}
	policy, err := service.GetRoutingPolicyView(id)
	if err != nil {
		writeRoutingPolicyError(c, err)
		return
	}
	common.ApiSuccess(c, policy)
}

func CreateRoutingPolicy(c *gin.Context) {
	var request service.RoutingPolicyWriteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeRoutingPolicyError(c, &service.RoutingPolicyServiceError{Code: "invalid_request", Err: err})
		return
	}
	policy, err := service.SaveRoutingPolicy(0, request)
	if err != nil {
		writeRoutingPolicyError(c, err)
		return
	}
	recordManageAudit(c, "routing_policy.create", routingPolicyAuditParams(policy))
	c.JSON(http.StatusCreated, gin.H{"success": true, "message": "", "data": policy})
}

func UpdateRoutingPolicy(c *gin.Context) {
	id, ok := routingPolicyID(c)
	if !ok {
		return
	}
	var request service.RoutingPolicyWriteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeRoutingPolicyError(c, &service.RoutingPolicyServiceError{Code: "invalid_request", Err: err})
		return
	}
	policy, err := service.SaveRoutingPolicy(id, request)
	if err != nil {
		writeRoutingPolicyError(c, err)
		return
	}
	recordManageAudit(c, "routing_policy.update", routingPolicyAuditParams(policy))
	common.ApiSuccess(c, policy)
}

func UpdateRoutingPolicyStatus(c *gin.Context) {
	id, ok := routingPolicyID(c)
	if !ok {
		return
	}
	var request struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.Enabled == nil {
		if err == nil {
			err = errors.New("enabled is required")
		}
		writeRoutingPolicyError(c, &service.RoutingPolicyServiceError{Code: "invalid_request", Field: "enabled", Err: err})
		return
	}
	policy, err := service.SetRoutingPolicyStatus(id, *request.Enabled)
	if err != nil {
		writeRoutingPolicyError(c, err)
		return
	}
	recordManageAudit(c, "routing_policy.status_update", routingPolicyAuditParams(policy))
	common.ApiSuccess(c, policy)
}

func DeleteRoutingPolicy(c *gin.Context) {
	id, ok := routingPolicyID(c)
	if !ok {
		return
	}
	policy, err := service.GetRoutingPolicyView(id)
	if err != nil {
		writeRoutingPolicyError(c, err)
		return
	}
	if err := service.RemoveRoutingPolicy(id); err != nil {
		writeRoutingPolicyError(c, err)
		return
	}
	recordManageAudit(c, "routing_policy.delete", routingPolicyAuditParams(policy))
	common.ApiSuccess(c, nil)
}

func routingPolicyID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		writeRoutingPolicyError(c, &service.RoutingPolicyServiceError{Code: "invalid_request", Field: "id", Err: errors.New("invalid routing policy id")})
		return 0, false
	}
	return id, true
}

func writeRoutingPolicyError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "routing policy not found", "code": "not_found"})
		return
	}
	var serviceErr *service.RoutingPolicyServiceError
	if errors.As(err, &serviceErr) {
		status := http.StatusBadRequest
		if serviceErr.Code == "routing_policy_error" {
			status = http.StatusInternalServerError
		}
		response := gin.H{"success": false, "message": serviceErr.Error(), "code": serviceErr.Code}
		data := gin.H{}
		if serviceErr.Field != "" {
			data["field"] = serviceErr.Field
		}
		if len(serviceErr.TargetIndexes) > 0 {
			data["target_indexes"] = serviceErr.TargetIndexes
		}
		if len(data) > 0 {
			response["data"] = data
		}
		c.JSON(status, response)
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error(), "code": "routing_policy_error"})
}

func routingPolicyAuditParams(policy *service.RoutingPolicyView) map[string]interface{} {
	return map[string]interface{}{
		"policy_id": policy.ID, "group_name": policy.GroupName, "model": policy.Model,
	}
}
