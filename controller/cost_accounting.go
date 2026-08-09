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
	settings := cost_setting.Runtime()
	common.ApiSuccess(c, gin.H{
		"mode":                        settings.Mode,
		"minimum_expected_margin_bps": settings.MinimumExpectedMarginBPS,
	})
}

func UpdateCostAccountingSettings(c *gin.Context) {
	var request dto.UpdateCostAccountingSettingsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeCostAccountingError(c, err)
		return
	}
	if request.Mode == nil || request.MinimumExpectedMarginBPS == nil {
		writeCostAccountingError(c, errors.New("mode and minimum expected margin are required"))
		return
	}
	if err := cost_setting.ValidateMode(*request.Mode); err != nil {
		writeCostAccountingError(c, err)
		return
	}
	if err := cost_setting.ValidateMinimumExpectedMarginBPS(*request.MinimumExpectedMarginBPS); err != nil {
		writeCostAccountingError(c, err)
		return
	}
	if *request.Mode == types.CostAccountingStrict {
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
	if err := model.UpdateOptionsBulk(map[string]string{
		cost_setting.ConfigName + "." + cost_setting.KeyMode:                     string(*request.Mode),
		cost_setting.ConfigName + "." + cost_setting.KeyMinimumExpectedMarginBPS: strconv.Itoa(*request.MinimumExpectedMarginBPS),
	}); err != nil {
		writeCostAccountingError(c, err)
		return
	}
	recordManageAudit(c, "cost_accounting.mode_update", map[string]interface{}{
		"mode":                        *request.Mode,
		"minimum_expected_margin_bps": *request.MinimumExpectedMarginBPS,
	})
	settings := cost_setting.Runtime()
	common.ApiSuccess(c, gin.H{
		"mode":                        settings.Mode,
		"minimum_expected_margin_bps": settings.MinimumExpectedMarginBPS,
	})
}

func ListCostRules(c *gin.Context) {
	channelID, _ := strconv.Atoi(c.Query("channel_id"))
	rules, err := service.ListCostRules(channelID, c.Query("billable_upstream_model"), c.Query("cost_variant_key"))
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
		CostVariantKey:        request.CostVariantKey,
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
	channelID, err := optionalCostAccountingQueryInt(c, "channel_id")
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	results, err := service.CheckCostCoverage(service.CostCoverageFilter{
		ChannelID: channelID, OriginModel: c.Query("origin_model"),
		BillableUpstreamModel: c.Query("billable_upstream_model"),
		CostVariantKey:        c.Query("cost_variant_key"),
	})
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	response := make([]dto.CostCoverageItem, 0, len(results))
	for _, result := range results {
		response = append(response, costCoverageResponse(result))
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
	if err := service.ReconcileCostAttempt(c.Request.Context(), id, c.GetInt("id"), request.Action, request.Meter, request.Reason); err != nil {
		writeCostAccountingError(c, err)
		return
	}
	recordManageAudit(c, "cost_accounting.attempt_reconcile", map[string]interface{}{"attempt_id": id, "action": request.Action})
	common.ApiSuccess(c, nil)
}

func ReconcileCostRevenue(c *gin.Context) {
	id, ok := costAccountingID(c)
	if !ok {
		return
	}
	request, err := bindReconcileCostRevenueRequest(c)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	if request.QuotaPerUnitSnapshot != nil {
		writeCostAccountingError(c, errors.New("quota-per-unit snapshot cannot be overridden during reconciliation"))
		return
	}
	var finalQuota int64
	switch strings.TrimSpace(request.Action) {
	case "settle":
		if request.FinalUserQuota == nil {
			writeCostAccountingError(c, errors.New("final user quota is required"))
			return
		}
		finalQuota = *request.FinalUserQuota
	case "confirm_zero":
		if request.FinalUserQuota != nil && *request.FinalUserQuota != 0 {
			writeCostAccountingError(c, errors.New("confirm-zero final user quota must be zero"))
			return
		}
	default:
		writeCostAccountingError(c, errors.New("unsupported reconciliation action"))
		return
	}
	if err := service.ReconcileCostRevenue(c.Request.Context(), id, c.GetInt("id"), finalQuota, request.Reason); err != nil {
		writeCostAccountingError(c, err)
		return
	}
	recordManageAudit(c, "cost_accounting.revenue_reconcile", map[string]interface{}{"request_id": id, "action": request.Action})
	common.ApiSuccess(c, nil)
}

func GetCostAccountingRequest(c *gin.Context) {
	id, ok := costAccountingID(c)
	if !ok {
		return
	}
	detail, err := service.GetCostRequestDetail(id)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	common.ApiSuccess(c, costRequestDetailResponse(*detail))
}

func ListCostAnomalies(c *gin.Context) {
	page, err := optionalCostAccountingQueryInt(c, "page")
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	pageSize, err := optionalCostAccountingQueryInt(c, "page_size")
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	if page == 0 {
		page = 1
	}
	if pageSize == 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	channelID, err := optionalCostAccountingQueryInt(c, "channel_id")
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	startTime, err := optionalCostAccountingQueryInt64(c, "start_time")
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	endTime, err := optionalCostAccountingQueryInt64(c, "end_time")
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	rows, total, err := service.ListCostAnomalies(service.CostAnomalyFilter{
		Page: page, PageSize: pageSize, Kind: c.Query("kind"), ChannelID: channelID,
		StartTime: startTime, EndTime: endTime,
	})
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	responseRows := make([]costAnomalyAPIResponse, 0, len(rows))
	for _, row := range rows {
		responseRows = append(responseRows, costAnomalyResponse(row))
	}
	common.ApiSuccess(c, gin.H{"items": responseRows, "total": total, "page": page, "page_size": pageSize})
}

func GetCostReportSummary(c *gin.Context) {
	filter, err := costReportFilterFromQuery(c)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	summary, err := service.SummarizeCostProfit(filter)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	common.ApiSuccess(c, costProfitSummaryResponse(summary))
}

func GetCostReportBreakdown(c *gin.Context) {
	filter, err := costReportFilterFromQuery(c)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	rows, err := service.BreakDownCostProfit(filter)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	common.ApiSuccess(c, costProfitBreakdownResponses(rows))
}

func bindReconcileCostAttemptRequest(c *gin.Context) (dto.ReconcileCostAttemptRequest, error) {
	var request dto.ReconcileCostAttemptRequest
	err := c.ShouldBindJSON(&request)
	return request, err
}

func bindReconcileCostRevenueRequest(c *gin.Context) (dto.ReconcileCostRevenueRequest, error) {
	var request dto.ReconcileCostRevenueRequest
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

type costProfitSummaryAPIResponse struct {
	RealizedRevenueNanoUSD     string  `json:"realized_revenue_nano_usd"`
	RealizedCostNanoUSD        string  `json:"realized_cost_nano_usd"`
	RealizedProfitNanoUSD      string  `json:"realized_profit_nano_usd"`
	GrossMarginPPM             *string `json:"gross_margin_ppm,omitempty"`
	KnownIncompleteCostNanoUSD string  `json:"known_incomplete_cost_nano_usd"`
	CompleteRequestCount       int64   `json:"complete_request_count"`
	NegativeProfitRequestCount int64   `json:"negative_profit_request_count"`
	RetryAttemptCount          int64   `json:"retry_attempt_count"`
	AwaitingMeterCount         int64   `json:"awaiting_meter_count"`
	UnknownCostCount           int64   `json:"unknown_cost_count"`
	SettlementFailedCount      int64   `json:"settlement_failed_count"`
	RevenueFailedCount         int64   `json:"revenue_failed_count"`
}

type costProfitBreakdownAPIResponse struct {
	ChannelID                  int     `json:"channel_id"`
	ChannelName                string  `json:"channel_name"`
	BillableUpstreamModel      string  `json:"billable_upstream_model"`
	RealizedRevenueNanoUSD     string  `json:"realized_revenue_nano_usd"`
	RealizedCostNanoUSD        string  `json:"realized_cost_nano_usd"`
	RealizedProfitNanoUSD      string  `json:"realized_profit_nano_usd"`
	GrossMarginPPM             *string `json:"gross_margin_ppm,omitempty"`
	KnownIncompleteCostNanoUSD string  `json:"known_incomplete_cost_nano_usd"`
	CompleteRequestCount       int64   `json:"complete_request_count"`
	NegativeProfitRequestCount int64   `json:"negative_profit_request_count"`
	AttemptCount               int64   `json:"attempt_count"`
	RetryAttemptCount          int64   `json:"retry_attempt_count"`
	AwaitingMeterCount         int64   `json:"awaiting_meter_count"`
	UnknownCostCount           int64   `json:"unknown_cost_count"`
	SettlementFailedCount      int64   `json:"settlement_failed_count"`
	RevenueFailedCount         int64   `json:"revenue_failed_count"`
}

type costRequestLedgerAPIResponse struct {
	model.CostAccountingRequest
	BilledRevenueEquivalentNanoUSD *string `json:"billed_revenue_equivalent_nano_usd,omitempty"`
	ConfirmedCostNanoUSD           string  `json:"confirmed_cost_nano_usd"`
	BilledGrossProfitNanoUSD       *string `json:"billed_gross_profit_nano_usd,omitempty"`
}

type costAttemptLedgerAPIResponse struct {
	model.CostAccountingAttempt
	CostNanoUSD *string `json:"cost_nano_usd,omitempty"`
}

type costAuditAPIResponse struct {
	model.CostAccountingAudit
	OldAmountNanoUSD *string `json:"old_amount_nano_usd,omitempty"`
	NewAmountNanoUSD *string `json:"new_amount_nano_usd,omitempty"`
}

type costRequestAttemptAPIResponse struct {
	Attempt costAttemptLedgerAPIResponse `json:"attempt"`
	Winning bool                         `json:"winning"`
}

type costRequestDetailAPIResponse struct {
	Request  costRequestLedgerAPIResponse    `json:"request"`
	Attempts []costRequestAttemptAPIResponse `json:"attempts"`
	Audits   []costAuditAPIResponse          `json:"audits"`
}

type costAnomalyAPIResponse struct {
	Kind       string                        `json:"kind"`
	Request    costRequestLedgerAPIResponse  `json:"request"`
	Attempt    *costAttemptLedgerAPIResponse `json:"attempt,omitempty"`
	OccurredAt int64                         `json:"occurred_at"`
}

func costProfitSummaryResponse(summary service.CostProfitSummary) costProfitSummaryAPIResponse {
	return costProfitSummaryAPIResponse{
		RealizedRevenueNanoUSD:     strconv.FormatInt(summary.RealizedRevenueNanoUSD, 10),
		RealizedCostNanoUSD:        strconv.FormatInt(summary.RealizedCostNanoUSD, 10),
		RealizedProfitNanoUSD:      strconv.FormatInt(summary.RealizedProfitNanoUSD, 10),
		GrossMarginPPM:             costAccountingInt64StringPointer(summary.GrossMarginPPM),
		KnownIncompleteCostNanoUSD: strconv.FormatInt(summary.KnownIncompleteCostNanoUSD, 10),
		CompleteRequestCount:       summary.CompleteRequestCount,
		NegativeProfitRequestCount: summary.NegativeProfitRequestCount,
		RetryAttemptCount:          summary.RetryAttemptCount,
		AwaitingMeterCount:         summary.AwaitingMeterCount,
		UnknownCostCount:           summary.UnknownCostCount,
		SettlementFailedCount:      summary.SettlementFailedCount,
		RevenueFailedCount:         summary.RevenueFailedCount,
	}
}

func costProfitBreakdownResponses(rows []service.CostProfitBreakdownRow) []costProfitBreakdownAPIResponse {
	response := make([]costProfitBreakdownAPIResponse, 0, len(rows))
	for _, row := range rows {
		response = append(response, costProfitBreakdownAPIResponse{
			ChannelID: row.ChannelID, ChannelName: row.ChannelName,
			BillableUpstreamModel:      row.BillableUpstreamModel,
			RealizedRevenueNanoUSD:     strconv.FormatInt(row.RealizedRevenueNanoUSD, 10),
			RealizedCostNanoUSD:        strconv.FormatInt(row.RealizedCostNanoUSD, 10),
			RealizedProfitNanoUSD:      strconv.FormatInt(row.RealizedProfitNanoUSD, 10),
			GrossMarginPPM:             costAccountingInt64StringPointer(row.GrossMarginPPM),
			KnownIncompleteCostNanoUSD: strconv.FormatInt(row.KnownIncompleteCostNanoUSD, 10),
			CompleteRequestCount:       row.CompleteRequestCount,
			NegativeProfitRequestCount: row.NegativeProfitRequestCount,
			AttemptCount:               row.AttemptCount, RetryAttemptCount: row.RetryAttemptCount,
			AwaitingMeterCount: row.AwaitingMeterCount, UnknownCostCount: row.UnknownCostCount,
			SettlementFailedCount: row.SettlementFailedCount, RevenueFailedCount: row.RevenueFailedCount,
		})
	}
	return response
}

func costRequestDetailResponse(detail service.CostRequestDetail) costRequestDetailAPIResponse {
	attempts := make([]costRequestAttemptAPIResponse, 0, len(detail.Attempts))
	for _, attempt := range detail.Attempts {
		attempts = append(attempts, costRequestAttemptAPIResponse{
			Attempt: costAttemptLedgerResponse(attempt.Attempt), Winning: attempt.Winning,
		})
	}
	audits := make([]costAuditAPIResponse, 0, len(detail.Audits))
	for _, audit := range detail.Audits {
		audits = append(audits, costAuditAPIResponse{
			CostAccountingAudit: audit,
			OldAmountNanoUSD:    costAccountingInt64StringPointer(audit.OldAmountNanoUSD),
			NewAmountNanoUSD:    costAccountingInt64StringPointer(audit.NewAmountNanoUSD),
		})
	}
	return costRequestDetailAPIResponse{
		Request: costRequestLedgerResponse(detail.Request), Attempts: attempts, Audits: audits,
	}
}

func costAnomalyResponse(row service.CostAnomalyRow) costAnomalyAPIResponse {
	response := costAnomalyAPIResponse{
		Kind: row.Kind, Request: costRequestLedgerResponse(row.Request), OccurredAt: row.OccurredAt,
	}
	if row.Attempt != nil {
		attempt := costAttemptLedgerResponse(*row.Attempt)
		response.Attempt = &attempt
	}
	return response
}

func costRequestLedgerResponse(request model.CostAccountingRequest) costRequestLedgerAPIResponse {
	return costRequestLedgerAPIResponse{
		CostAccountingRequest:          request,
		BilledRevenueEquivalentNanoUSD: costAccountingInt64StringPointer(request.BilledRevenueEquivalentNanoUSD),
		ConfirmedCostNanoUSD:           strconv.FormatInt(request.ConfirmedCostNanoUSD, 10),
		BilledGrossProfitNanoUSD:       costAccountingInt64StringPointer(request.BilledGrossProfitNanoUSD),
	}
}

func costAttemptLedgerResponse(attempt model.CostAccountingAttempt) costAttemptLedgerAPIResponse {
	return costAttemptLedgerAPIResponse{
		CostAccountingAttempt: attempt,
		CostNanoUSD:           costAccountingInt64StringPointer(attempt.CostNanoUSD),
	}
}

func costCoverageResponse(row service.CostCoverageRow) dto.CostCoverageItem {
	return dto.CostCoverageItem{
		ChannelID: row.ChannelID, OriginModel: row.OriginModel,
		PredictedUpstreamModel: row.BillableUpstreamModel, CostVariantKey: row.CostVariantKey,
		Covered: row.Covered, Reason: row.Reason,
	}
}

func costAccountingInt64StringPointer(value *int64) *string {
	if value == nil {
		return nil
	}
	formatted := strconv.FormatInt(*value, 10)
	return &formatted
}

func costReportFilterFromQuery(c *gin.Context) (service.CostReportFilter, error) {
	channelID, err := optionalCostAccountingQueryInt(c, "channel_id")
	if err != nil {
		return service.CostReportFilter{}, err
	}
	startTime, err := optionalCostAccountingQueryInt64(c, "start_time")
	if err != nil {
		return service.CostReportFilter{}, err
	}
	endTime, err := optionalCostAccountingQueryInt64(c, "end_time")
	if err != nil {
		return service.CostReportFilter{}, err
	}
	if startTime > 0 && endTime > 0 && startTime > endTime {
		return service.CostReportFilter{}, errors.New("cost report start time must not exceed end time")
	}
	return service.CostReportFilter{
		TimeBasis: c.Query("time_basis"), StartTime: startTime, EndTime: endTime,
		ChannelID: channelID, BillableUpstreamModel: c.Query("billable_upstream_model"),
		OriginModelName: c.Query("origin_model"), UserGroup: c.Query("user_group"),
		UsingGroup: c.Query("using_group"), BillingSource: c.Query("billing_source"),
		Status: c.Query("status"),
	}, nil
}

func optionalCostAccountingQueryInt(c *gin.Context, key string) (int, error) {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, errors.New("invalid " + key)
	}
	return parsed, nil
}

func optionalCostAccountingQueryInt64(c *gin.Context, key string) (int64, error) {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, errors.New("invalid " + key)
	}
	return parsed, nil
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
		CostVariantKey: rule.CostVariantKey,
		Version:        rule.Version, Status: rule.Status, CostMode: types.CostMode(rule.CostMode),
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

func writeCostAccountingError(c *gin.Context, err error) {
	status := http.StatusBadRequest
	code := "invalid_request"
	if errors.Is(err, gorm.ErrRecordNotFound) {
		status = http.StatusNotFound
		code = "not_found"
	} else if errors.Is(err, model.ErrCostStateConflict) || errors.Is(err, model.ErrCostRuleStateConflict) || errors.Is(err, model.ErrCostActiveRuleConflict) {
		status = http.StatusConflict
		code = "state_conflict"
	} else if errors.Is(err, service.ErrCostReportOverflow) {
		status = http.StatusInternalServerError
		code = "cost_report_overflow"
	} else if errors.Is(err, service.ErrCostReportInconsistent) {
		status = http.StatusInternalServerError
		code = "cost_report_inconsistent"
	} else if errors.Is(err, service.ErrCostCatalogExportTooLarge) {
		status = http.StatusRequestEntityTooLarge
		code = "cost_catalog_export_too_large"
	} else if errors.Is(err, service.ErrCostCatalogUnavailable) {
		status = http.StatusInternalServerError
		code = "cost_catalog_unavailable"
	}
	message := strings.TrimSpace(err.Error())
	if status >= http.StatusInternalServerError {
		message = "cost accounting operation failed"
	}
	c.JSON(status, gin.H{"success": false, "message": message, "code": code})
}
