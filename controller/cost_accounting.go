package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/cost_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetCostAccountingSettings(c *gin.Context) {
	common.ApiSuccess(c, gin.H{"mode": cost_setting.Runtime().Mode})
}

func UpdateCostAccountingSettings(c *gin.Context) {
	var request dto.UpdateCostAccountingModeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeCostAccountingError(c, err)
		return
	}
	if err := cost_setting.ValidateMode(request.Mode); err != nil {
		writeCostAccountingError(c, err)
		return
	}
	if request.Mode == types.CostAccountingStrict {
		coverage, err := service.CheckAuthoritativeCostCoverage()
		if err != nil {
			writeCostAccountingError(c, err)
			return
		}
		for _, item := range coverage {
			if item.Covered {
				continue
			}
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"message": "cost coverage is incomplete for the selected channel and model",
				"code":    "cost_coverage_incomplete",
				"data": gin.H{
					"channel_id": item.ChannelID,
					"model":      item.PredictedUpstreamModel,
				},
			})
			return
		}
	}
	if err := model.UpdateOption(cost_setting.ConfigName+"."+cost_setting.KeyMode, string(request.Mode)); err != nil {
		writeCostAccountingError(c, err)
		return
	}
	recordManageAudit(c, "cost_accounting.mode_update", map[string]interface{}{"mode": request.Mode})
	common.ApiSuccess(c, gin.H{"mode": cost_setting.Runtime().Mode})
}

func ListCostRules(c *gin.Context) {
	channelID, _ := strconv.Atoi(c.Query("channel_id"))
	rules, err := service.ListCostRules(channelID, c.Query("billable_upstream_model"))
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	responses, err := costRuleResponses(rules)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	common.ApiSuccess(c, responses)
}

func CreateCostRule(c *gin.Context) {
	var request dto.CostRuleWriteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeCostAccountingError(c, err)
		return
	}
	rule, err := service.CreateCostRuleDraft(service.CreateCostRuleInput{
		ChannelID:             request.ChannelID,
		BillableUpstreamModel: request.BillableUpstreamModel,
		CostMode:              request.CostMode,
		Config:                request.Config,
		Note:                  request.Note,
		AdminID:               c.GetInt("id"),
		RequestPath:           request.RequestPath,
		TaskPlatform:          request.TaskPlatform,
	})
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	response, err := costRuleResponse(*rule)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	recordManageAudit(c, "cost_accounting.rule_create", map[string]interface{}{"rule_id": rule.ID})
	c.JSON(http.StatusCreated, gin.H{"success": true, "message": "", "data": response})
}

func UpdateCostRule(c *gin.Context) {
	id, ok := costAccountingID(c)
	if !ok {
		return
	}
	var request dto.CostRuleUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeCostAccountingError(c, err)
		return
	}
	rule, err := service.UpdateCostRuleDraft(id, service.UpdateCostRuleInput{
		CostMode: request.CostMode, Config: request.Config, Note: request.Note,
		RequestPath: request.RequestPath, TaskPlatform: request.TaskPlatform,
	})
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	response, err := costRuleResponse(*rule)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	recordManageAudit(c, "cost_accounting.rule_update", map[string]interface{}{"rule_id": rule.ID})
	common.ApiSuccess(c, response)
}

func ValidateCostRule(c *gin.Context) {
	id, ok := costAccountingID(c)
	if !ok {
		return
	}
	config, err := service.ValidateCostRuleByID(id)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"valid": true, "config": config})
}

func ActivateCostRule(c *gin.Context) {
	id, ok := costAccountingID(c)
	if !ok {
		return
	}
	rule, err := service.ActivateCostRule(id, c.GetInt("id"))
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	response, err := costRuleResponse(*rule)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	recordManageAudit(c, "cost_accounting.rule_activate", map[string]interface{}{"rule_id": rule.ID})
	common.ApiSuccess(c, response)
}

func RetireCostRule(c *gin.Context) {
	id, ok := costAccountingID(c)
	if !ok {
		return
	}
	if err := service.RetireCostRule(id, c.GetInt("id")); err != nil {
		writeCostAccountingError(c, err)
		return
	}
	recordManageAudit(c, "cost_accounting.rule_retire", map[string]interface{}{"rule_id": id})
	common.ApiSuccess(c, nil)
}

func GetCostRuleHistory(c *gin.Context) {
	id, ok := costAccountingID(c)
	if !ok {
		return
	}
	rules, err := service.CostRuleHistory(id)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	responses, err := costRuleResponses(rules)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	common.ApiSuccess(c, responses)
}

func PreviewCostAccounting(c *gin.Context) {
	var request dto.CostPreviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeCostAccountingError(c, err)
		return
	}
	if request.Meter == nil {
		writeCostAccountingError(c, errors.New("cost meter is required"))
		return
	}
	normalized, err := service.NormalizeCostRuleConfig(request.CostMode, request.Config)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	finalQuota, quotaPerUnitSnapshot, err := helper.PreviewUserBillingQuota(c, request)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	preview, err := service.PreviewChannelModelCost(service.PreviewCostInput{
		FinalUserQuota: finalQuota, QuotaPerUnitSnapshot: quotaPerUnitSnapshot,
		CostMode: request.CostMode, Config: normalized, Meter: *request.Meter,
	})
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	common.ApiSuccess(c, costPreviewResponse(preview))
}

func GetCostCoverage(c *gin.Context) {
	results, err := service.CheckAuthoritativeCostCoverage()
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	response := make([]dto.CostCoverageItem, 0, len(results))
	for _, result := range results {
		response = append(response, dto.CostCoverageItem{
			ChannelID: result.ChannelID, OriginModel: result.OriginModel,
			PredictedUpstreamModel: result.PredictedUpstreamModel,
			Covered:                result.Covered, Reason: result.Reason,
		})
	}
	common.ApiSuccess(c, response)
}

func ReconcileCostAttempt(c *gin.Context) {
	id, ok := costAccountingID(c)
	if !ok {
		return
	}
	request, err := bindReconcileCostAttemptRequest(c)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	var attempt model.CostAccountingAttempt
	if err := model.DB.Where("id = ?", id).First(&attempt).Error; err != nil {
		writeCostAccountingError(c, err)
		return
	}
	var config types.CostRuleConfigV1
	if err := common.UnmarshalJsonStr(attempt.RuleConfigJSON, &config); err != nil {
		writeCostAccountingError(c, err)
		return
	}

	input := model.ReconcileCostAttemptInput{
		AttemptID: id, AdminID: c.GetInt("id"), Reason: request.Reason,
	}
	switch request.Action {
	case "settle":
		mode := types.CostMode(attempt.CostMode)
		meterRequired := mode == types.CostModePerDuration || mode == types.CostModePerToken
		if request.Meter == nil && meterRequired {
			writeCostAccountingError(c, errors.New("meter is required for settlement"))
			return
		}
		meter := types.CostMeter{}
		if request.Meter != nil {
			meter = *request.Meter
		}
		if attempt.MeterSource != "" && meter.Source != types.CostMeterSource(attempt.MeterSource) {
			writeCostAccountingError(c, errors.New("meter source does not match the attempt snapshot"))
			return
		}
		originalCost, costNanoUSD, err := service.CalculateAttemptCost(mode, config, meter)
		if err != nil {
			writeCostAccountingError(c, err)
			return
		}
		if request.Meter != nil {
			meterJSON, err := common.Marshal(request.Meter)
			if err != nil {
				writeCostAccountingError(c, err)
				return
			}
			input.MeterJSON = string(meterJSON)
		}
		input.To = types.CostAttemptSettled
		input.OriginalCost = originalCost
		input.CostNanoUSD = &costNanoUSD
	case "confirm_zero":
		zero := int64(0)
		input.To = types.CostAttemptConfirmedZero
		input.OriginalCost = "0"
		input.CostNanoUSD = &zero
		if request.Meter != nil {
			meterJSON, err := common.Marshal(request.Meter)
			if err != nil {
				writeCostAccountingError(c, err)
				return
			}
			input.MeterJSON = string(meterJSON)
		}
	default:
		writeCostAccountingError(c, errors.New("unsupported reconciliation action"))
		return
	}
	if err := model.ReconcileCostAttempt(input); err != nil {
		writeCostAccountingError(c, err)
		return
	}
	recordManageAudit(c, "cost_accounting.attempt_reconcile", map[string]interface{}{"attempt_id": id, "action": request.Action})
	common.ApiSuccess(c, nil)
}

func ReconcileCostRevenue(c *gin.Context) {
	writeCostAccountingNotImplemented(c)
}

func GetCostAccountingRequest(c *gin.Context) {
	writeCostAccountingNotImplemented(c)
}

func ListCostAnomalies(c *gin.Context) {
	writeCostAccountingNotImplemented(c)
}

func GetCostReportSummary(c *gin.Context) {
	writeCostAccountingNotImplemented(c)
}

func GetCostReportBreakdown(c *gin.Context) {
	writeCostAccountingNotImplemented(c)
}

func bindReconcileCostAttemptRequest(c *gin.Context) (dto.ReconcileCostAttemptRequest, error) {
	var request dto.ReconcileCostAttemptRequest
	err := c.ShouldBindJSON(&request)
	return request, err
}

func costPreviewResponse(preview service.CostPreview) dto.CostPreviewResponse {
	response := dto.CostPreviewResponse{
		Estimated:      preview.Estimated,
		OriginalCost:   preview.OriginalCost,
		RevenueNanoUSD: strconv.FormatInt(preview.RevenueNanoUSD, 10),
		CostNanoUSD:    strconv.FormatInt(preview.CostNanoUSD, 10),
		ProfitNanoUSD:  strconv.FormatInt(preview.ProfitNanoUSD, 10),
	}
	if preview.MarginPPM != nil {
		margin := strconv.FormatInt(*preview.MarginPPM, 10)
		response.MarginPPM = &margin
	}
	return response
}

func costRuleResponses(rules []model.ChannelModelCostRule) ([]dto.CostRuleResponse, error) {
	responses := make([]dto.CostRuleResponse, 0, len(rules))
	for _, rule := range rules {
		response, err := costRuleResponse(rule)
		if err != nil {
			return nil, err
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func costRuleResponse(rule model.ChannelModelCostRule) (dto.CostRuleResponse, error) {
	var config types.CostRuleConfigV1
	if err := common.UnmarshalJsonStr(rule.ConfigJSON, &config); err != nil {
		return dto.CostRuleResponse{}, err
	}
	return dto.CostRuleResponse{
		ID: rule.ID, ChannelID: rule.ChannelID, BillableUpstreamModel: rule.BillableUpstreamModel,
		Version: rule.Version, Status: rule.Status, CostMode: types.CostMode(rule.CostMode),
		SchemaVersion: rule.SchemaVersion, Config: config, Source: rule.Source, Note: rule.Note,
		CreatedBy: rule.CreatedBy, ActivatedBy: rule.ActivatedBy,
		EffectiveFrom: rule.EffectiveFrom, EffectiveTo: rule.EffectiveTo,
		CreatedAt: rule.CreatedAt, UpdatedAt: rule.UpdatedAt,
	}, nil
}

func costAccountingID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeCostAccountingError(c, errors.New("invalid cost accounting ID"))
		return 0, false
	}
	return id, true
}

func writeCostAccountingNotImplemented(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"success": false,
		"message": "this cost accounting query is not implemented yet",
		"code":    "not_implemented",
	})
}

func writeCostAccountingError(c *gin.Context, err error) {
	status := http.StatusBadRequest
	code := "invalid_request"
	if errors.Is(err, gorm.ErrRecordNotFound) {
		status = http.StatusNotFound
		code = "not_found"
	} else if errors.Is(err, model.ErrCostStateConflict) || errors.Is(err, model.ErrCostRuleStateConflict) || errors.Is(err, model.ErrCostActiveRuleConflict) {
		status = http.StatusConflict
		code = "state_conflict"
	}
	message := strings.TrimSpace(err.Error())
	if status >= http.StatusInternalServerError {
		message = "cost accounting operation failed"
	}
	c.JSON(status, gin.H{"success": false, "message": message, "code": code})
}
