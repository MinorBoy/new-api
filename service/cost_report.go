package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
)

const (
	CostReportTimeProfitRecognized = "profit_recognized_at"
	CostReportTimeRequested        = "requested_at"
)

var (
	ErrCostReportOverflow     = errors.New("cost_report_overflow")
	ErrCostReportInconsistent = errors.New("cost_report_inconsistent")
)

type CostRequestAttemptDetail struct {
	Attempt model.CostAccountingAttempt `json:"attempt"`
	Winning bool                        `json:"winning"`
}

type CostRequestDetail struct {
	Request  model.CostAccountingRequest `json:"request"`
	Attempts []CostRequestAttemptDetail  `json:"attempts"`
	Audits   []model.CostAccountingAudit `json:"audits"`
}

type CostAnomalyFilter struct {
	Page      int
	PageSize  int
	Kind      string
	ChannelID int
	StartTime int64
	EndTime   int64
}

type CostAnomalyRow struct {
	Kind       string                       `json:"kind"`
	Request    model.CostAccountingRequest  `json:"request"`
	Attempt    *model.CostAccountingAttempt `json:"attempt,omitempty"`
	OccurredAt int64                        `json:"occurred_at"`
}

type CostCoverageFilter struct {
	ChannelID             int
	OriginModel           string
	BillableUpstreamModel string
	CostVariantKey        string
}

type CostCoverageRow struct {
	ChannelID             int    `json:"channel_id"`
	OriginModel           string `json:"origin_model"`
	BillableUpstreamModel string `json:"billable_upstream_model"`
	CostVariantKey        string `json:"cost_variant_key"`
	Covered               bool   `json:"covered"`
	Reason                string `json:"reason,omitempty"`
}

type CostReportFilter struct {
	TimeBasis             string
	StartTime             int64
	EndTime               int64
	ChannelID             int
	BillableUpstreamModel string
	OriginModelName       string
	UserGroup             string
	UsingGroup            string
	BillingSource         string
	Status                string
}

type CostReportFilterChannel struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type CostReportFilterOptions struct {
	Channels               []CostReportFilterChannel `json:"channels"`
	BillableUpstreamModels []string                  `json:"billable_upstream_models"`
	OriginModels           []string                  `json:"origin_models"`
	UserGroups             []string                  `json:"user_groups"`
	UsingGroups            []string                  `json:"using_groups"`
}

type CostProfitSummary struct {
	RealizedRevenueNanoUSD     int64  `json:"realized_revenue_nano_usd"`
	RealizedCostNanoUSD        int64  `json:"realized_cost_nano_usd"`
	RealizedProfitNanoUSD      int64  `json:"realized_profit_nano_usd"`
	GrossMarginPPM             *int64 `json:"gross_margin_ppm,omitempty"`
	KnownIncompleteCostNanoUSD int64  `json:"known_incomplete_cost_nano_usd"`
	CompleteRequestCount       int64  `json:"complete_request_count"`
	NegativeProfitRequestCount int64  `json:"negative_profit_request_count"`
	RetryAttemptCount          int64  `json:"retry_attempt_count"`
	AwaitingMeterCount         int64  `json:"awaiting_meter_count"`
	UnknownCostCount           int64  `json:"unknown_cost_count"`
	SettlementFailedCount      int64  `json:"settlement_failed_count"`
	RevenueFailedCount         int64  `json:"revenue_failed_count"`
}

type CostProfitBreakdownRow struct {
	ChannelID                  int    `json:"channel_id"`
	ChannelName                string `json:"channel_name"`
	BillableUpstreamModel      string `json:"billable_upstream_model"`
	RealizedRevenueNanoUSD     int64  `json:"realized_revenue_nano_usd"`
	RealizedCostNanoUSD        int64  `json:"realized_cost_nano_usd"`
	RealizedProfitNanoUSD      int64  `json:"realized_profit_nano_usd"`
	GrossMarginPPM             *int64 `json:"gross_margin_ppm,omitempty"`
	KnownIncompleteCostNanoUSD int64  `json:"known_incomplete_cost_nano_usd"`
	CompleteRequestCount       int64  `json:"complete_request_count"`
	NegativeProfitRequestCount int64  `json:"negative_profit_request_count"`
	AttemptCount               int64  `json:"attempt_count"`
	RetryAttemptCount          int64  `json:"retry_attempt_count"`
	AwaitingMeterCount         int64  `json:"awaiting_meter_count"`
	UnknownCostCount           int64  `json:"unknown_cost_count"`
	SettlementFailedCount      int64  `json:"settlement_failed_count"`
	RevenueFailedCount         int64  `json:"revenue_failed_count"`
}

type costBreakdownKey struct {
	ChannelID int
	Model     string
}

type costAttributedAmount struct {
	Revenue int64
	Cost    int64
}

type costReportJoinedRow struct {
	RequestID            int64  `gorm:"column:request_id"`
	WinningAttemptID     *int64 `gorm:"column:winning_attempt_id"`
	RequestRevenue       *int64 `gorm:"column:request_revenue"`
	RequestProfit        *int64 `gorm:"column:request_profit"`
	RequestConfirmedCost int64  `gorm:"column:request_confirmed_cost"`
	RequestProfitStatus  string `gorm:"column:request_profit_status"`
	RequestRevenueStatus string `gorm:"column:request_revenue_status"`
	AttemptID            int64  `gorm:"column:attempt_id"`
	AttemptNo            int    `gorm:"column:attempt_no"`
	ChannelID            int    `gorm:"column:channel_id"`
	ChannelName          string `gorm:"column:channel_name"`
	BillableModel        string `gorm:"column:billable_upstream_model"`
	AttemptStatus        string `gorm:"column:attempt_status"`
	AttemptCost          *int64 `gorm:"column:attempt_cost"`
}

type costAnomalyCandidate struct {
	Kind       string `gorm:"column:kind"`
	RequestID  int64  `gorm:"column:request_id"`
	AttemptID  *int64 `gorm:"column:attempt_id"`
	OccurredAt int64  `gorm:"column:occurred_at"`
}

func GetCostRequestDetail(id int64) (*CostRequestDetail, error) {
	if id <= 0 {
		return nil, errors.New("cost request ID is invalid")
	}
	var request model.CostAccountingRequest
	if err := model.DB.Where("id = ?", id).First(&request).Error; err != nil {
		return nil, err
	}
	var attempts []model.CostAccountingAttempt
	if err := model.DB.Where("cost_request_id = ?", request.ID).Order("attempt_no ASC, id ASC").Find(&attempts).Error; err != nil {
		return nil, err
	}
	details := make([]CostRequestAttemptDetail, 0, len(attempts))
	for _, attempt := range attempts {
		details = append(details, CostRequestAttemptDetail{
			Attempt: attempt,
			Winning: request.WinningAttemptID != nil && *request.WinningAttemptID == attempt.ID,
		})
	}
	audits, err := model.ListCostAccountingAudits(request.ID)
	if err != nil {
		return nil, err
	}
	return &CostRequestDetail{Request: request, Attempts: details, Audits: audits}, nil
}

func ListCostAnomalies(filter CostAnomalyFilter) ([]CostAnomalyRow, int64, error) {
	if filter.StartTime > 0 && filter.EndTime > 0 && filter.StartTime > filter.EndTime {
		return nil, 0, errors.New("cost anomaly start time must not exceed end time")
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	filter.Kind = strings.TrimSpace(filter.Kind)

	unionSQL, args := costAnomalyUnionQuery(filter)
	var total int64
	if err := model.DB.Raw("SELECT COUNT(*) FROM ("+unionSQL+") AS cost_anomalies", args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	pageIndex := int64(filter.Page - 1)
	pageSize := int64(filter.PageSize)
	if pageIndex > math.MaxInt64/pageSize {
		return []CostAnomalyRow{}, total, nil
	}
	offset := pageIndex * pageSize
	pageArgs := append(append([]any{}, args...), filter.PageSize, offset)
	var candidates []costAnomalyCandidate
	pageSQL := unionSQL + " ORDER BY occurred_at DESC, request_id DESC, sort_attempt_id DESC LIMIT ? OFFSET ?"
	if err := model.DB.Raw(pageSQL, pageArgs...).Scan(&candidates).Error; err != nil {
		return nil, 0, err
	}
	if len(candidates) == 0 {
		return []CostAnomalyRow{}, total, nil
	}

	requestIDs := make([]int64, 0, len(candidates))
	attemptIDs := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		requestIDs = append(requestIDs, candidate.RequestID)
		if candidate.AttemptID != nil {
			attemptIDs = append(attemptIDs, *candidate.AttemptID)
		}
	}
	var requests []model.CostAccountingRequest
	if err := model.DB.Where("id IN ?", requestIDs).Find(&requests).Error; err != nil {
		return nil, 0, err
	}
	requestByID := make(map[int64]model.CostAccountingRequest, len(requests))
	for _, request := range requests {
		requestByID[request.ID] = request
	}
	attemptByID := map[int64]model.CostAccountingAttempt{}
	if len(attemptIDs) > 0 {
		var attempts []model.CostAccountingAttempt
		if err := model.DB.Where("id IN ?", attemptIDs).Find(&attempts).Error; err != nil {
			return nil, 0, err
		}
		for _, attempt := range attempts {
			attemptByID[attempt.ID] = attempt
		}
	}

	rows := make([]CostAnomalyRow, 0, len(candidates))
	for _, candidate := range candidates {
		request, ok := requestByID[candidate.RequestID]
		if !ok {
			return nil, 0, costReportInconsistent("anomaly request is missing", candidate.RequestID)
		}
		row := CostAnomalyRow{Kind: candidate.Kind, Request: request, OccurredAt: candidate.OccurredAt}
		if candidate.AttemptID != nil {
			attempt, ok := attemptByID[*candidate.AttemptID]
			if !ok {
				return nil, 0, costReportInconsistent("anomaly attempt is missing", *candidate.AttemptID)
			}
			row.Attempt = &attempt
		}
		rows = append(rows, row)
	}
	return rows, total, nil
}

func CheckCostCoverage(filter CostCoverageFilter) ([]CostCoverageRow, error) {
	results, err := CheckAuthoritativeCostCoverage()
	if err != nil {
		return nil, err
	}
	originModel := strings.TrimSpace(filter.OriginModel)
	billableModel := strings.TrimSpace(filter.BillableUpstreamModel)
	costVariantKey := strings.TrimSpace(filter.CostVariantKey)
	if costVariantKey != "" {
		costVariantKey, err = types.NormalizeCostVariantKey(costVariantKey)
		if err != nil {
			return nil, err
		}
	}
	rows := make([]CostCoverageRow, 0, len(results))
	for _, result := range results {
		if filter.ChannelID > 0 && result.ChannelID != filter.ChannelID {
			continue
		}
		if originModel != "" && result.OriginModel != originModel {
			continue
		}
		if billableModel != "" && result.PredictedUpstreamModel != billableModel {
			continue
		}
		if costVariantKey != "" && result.CostVariantKey != costVariantKey {
			continue
		}
		rows = append(rows, CostCoverageRow{
			ChannelID:             result.ChannelID,
			OriginModel:           result.OriginModel,
			BillableUpstreamModel: result.PredictedUpstreamModel,
			CostVariantKey:        result.CostVariantKey,
			Covered:               result.Covered,
			Reason:                result.Reason,
		})
	}
	return rows, nil
}

func SummarizeCostProfit(filter CostReportFilter) (CostProfitSummary, error) {
	var summary CostProfitSummary
	attemptFiltered := filter.ChannelID > 0 || strings.TrimSpace(filter.BillableUpstreamModel) != ""
	var err error
	if attemptFiltered {
		summary, err = summarizeFilteredCostProfit(filter)
	} else {
		summary, err = summarizeRequestLedgerCostProfit(filter)
	}
	if err != nil {
		return summary, err
	}
	summary.GrossMarginPPM, err = costReportMargin(summary.RealizedProfitNanoUSD, summary.RealizedRevenueNanoUSD)
	return summary, err
}

func BreakDownCostProfit(filter CostReportFilter) ([]CostProfitBreakdownRow, error) {
	if err := validateCostReportWinners(filter); err != nil {
		return nil, err
	}
	query, err := costReportAttemptQuery(filter)
	if err != nil {
		return nil, err
	}
	sqlRows, err := query.Order("requests.id ASC, attempts.attempt_no ASC, attempts.id ASC").Rows()
	if err != nil {
		return nil, err
	}
	defer sqlRows.Close()

	rows := map[costBreakdownKey]*CostProfitBreakdownRow{}
	currentRequestID := int64(0)
	currentRequest := costReportJoinedRow{}
	currentAmounts := map[costBreakdownKey]*costAttributedAmount{}
	flushRequest := func() error {
		if currentRequestID == 0 {
			return nil
		}
		for key, amounts := range currentAmounts {
			row := rows[key]
			if currentRequest.RequestRevenueStatus == string(types.CostRevenueFailed) {
				row.RevenueFailedCount++
			}
			if currentRequest.RequestProfitStatus != string(types.CostProfitComplete) {
				continue
			}
			row.CompleteRequestCount++
			var err error
			row.RealizedRevenueNanoUSD, err = costReportAdd(row.RealizedRevenueNanoUSD, amounts.Revenue)
			if err != nil {
				return err
			}
			row.RealizedCostNanoUSD, err = costReportAdd(row.RealizedCostNanoUSD, amounts.Cost)
			if err != nil {
				return err
			}
			profit, err := costReportSubtract(amounts.Revenue, amounts.Cost)
			if err != nil {
				return err
			}
			if profit < 0 {
				row.NegativeProfitRequestCount++
			}
			row.RealizedProfitNanoUSD, err = costReportAdd(row.RealizedProfitNanoUSD, profit)
			if err != nil {
				return err
			}
		}
		return nil
	}

	for sqlRows.Next() {
		var joined costReportJoinedRow
		if err := query.ScanRows(sqlRows, &joined); err != nil {
			return nil, err
		}
		if currentRequestID != joined.RequestID {
			if err := flushRequest(); err != nil {
				return nil, err
			}
			currentRequestID = joined.RequestID
			currentRequest = joined
			currentAmounts = map[costBreakdownKey]*costAttributedAmount{}
		}
		key := costBreakdownKey{ChannelID: joined.ChannelID, Model: joined.BillableModel}
		row := rows[key]
		if row == nil {
			row = &CostProfitBreakdownRow{
				ChannelID: joined.ChannelID, ChannelName: joined.ChannelName,
				BillableUpstreamModel: joined.BillableModel,
			}
			rows[key] = row
		}
		amounts := currentAmounts[key]
		if amounts == nil {
			amounts = &costAttributedAmount{}
			currentAmounts[key] = amounts
		}
		row.AttemptCount++
		if joined.AttemptNo > 1 {
			row.RetryAttemptCount++
		}
		incrementCostAttemptCounts(&row.AwaitingMeterCount, &row.UnknownCostCount, &row.SettlementFailedCount, joined.AttemptStatus)
		cost, known, err := knownJoinedCostAttemptAmount(joined)
		if err != nil {
			return nil, err
		}
		if joined.RequestProfitStatus != string(types.CostProfitComplete) {
			if known {
				row.KnownIncompleteCostNanoUSD, err = costReportAdd(row.KnownIncompleteCostNanoUSD, cost)
				if err != nil {
					return nil, err
				}
			}
			continue
		}
		if !known {
			return nil, costReportInconsistent("complete request has an incomplete attempt", joined.RequestID)
		}
		amounts.Cost, err = costReportAdd(amounts.Cost, cost)
		if err != nil {
			return nil, err
		}
		if joined.WinningAttemptID != nil && *joined.WinningAttemptID == joined.AttemptID {
			if joined.RequestRevenue == nil {
				return nil, costReportInconsistent("complete request has missing revenue", joined.RequestID)
			}
			amounts.Revenue, err = costReportAdd(amounts.Revenue, *joined.RequestRevenue)
			if err != nil {
				return nil, err
			}
		}
	}
	if err := sqlRows.Err(); err != nil {
		return nil, err
	}
	if err := flushRequest(); err != nil {
		return nil, err
	}

	result := make([]CostProfitBreakdownRow, 0, len(rows))
	for _, row := range rows {
		row.GrossMarginPPM, err = costReportMargin(row.RealizedProfitNanoUSD, row.RealizedRevenueNanoUSD)
		if err != nil {
			return nil, err
		}
		result = append(result, *row)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ChannelID == result[j].ChannelID {
			return result[i].BillableUpstreamModel < result[j].BillableUpstreamModel
		}
		return result[i].ChannelID < result[j].ChannelID
	})
	return result, nil
}

// ListCostReportFilterOptions returns distinct values available under the
// submitted report filters. The field being populated is intentionally
// ignored so users can replace an existing value with another option.
func ListCostReportFilterOptions(filter CostReportFilter) (CostReportFilterOptions, error) {
	options := CostReportFilterOptions{
		Channels:               make([]CostReportFilterChannel, 0),
		BillableUpstreamModels: make([]string, 0),
		OriginModels:           make([]string, 0),
		UserGroups:             make([]string, 0),
		UsingGroups:            make([]string, 0),
	}
	if _, err := costReportRequestQuery(filter); err != nil {
		return options, err
	}

	attemptRows, err := costReportAttemptFilterRows(filter, "channel_id")
	if err != nil {
		return options, err
	}
	channelByID := map[int]string{}
	billableSet := map[string]struct{}{}
	for _, row := range attemptRows {
		if row.ChannelID > 0 {
			if _, exists := channelByID[row.ChannelID]; !exists && strings.TrimSpace(row.ChannelName) != "" {
				channelByID[row.ChannelID] = strings.TrimSpace(row.ChannelName)
			}
		}
	}
	for id, name := range channelByID {
		options.Channels = append(options.Channels, CostReportFilterChannel{ID: id, Name: name})
	}
	sort.Slice(options.Channels, func(i, j int) bool { return options.Channels[i].ID < options.Channels[j].ID })

	attemptRows, err = costReportAttemptFilterRows(filter, "billable_upstream_model")
	if err != nil {
		return options, err
	}
	for _, row := range attemptRows {
		if value := strings.TrimSpace(row.BillableModel); value != "" {
			billableSet[value] = struct{}{}
		}
	}
	options.BillableUpstreamModels = sortedCostReportStrings(billableSet)

	for _, candidate := range []struct {
		ignore string
		name   string
		target *[]string
	}{
		{ignore: "origin_model", name: "origin_model_name", target: &options.OriginModels},
		{ignore: "user_group", name: "user_group", target: &options.UserGroups},
		{ignore: "using_group", name: "using_group", target: &options.UsingGroups},
	} {
		query, err := costReportRequestQueryIgnoring(filter, candidate.ignore)
		if err != nil {
			return options, err
		}
		query = applyCostReportAttemptSelectionToRequestQuery(query, filter, candidate.ignore)
		rows, err := query.Select("requests." + candidate.name).Order("requests." + candidate.name + " ASC").Rows()
		if err != nil {
			return options, err
		}
		values := map[string]struct{}{}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				rows.Close()
				return options, err
			}
			if value = strings.TrimSpace(value); value != "" {
				values[value] = struct{}{}
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return options, err
		}
		rows.Close()
		*candidate.target = sortedCostReportStrings(values)
	}
	return options, nil
}

type costReportFilterAttemptOptionRow struct {
	ChannelID     int    `gorm:"column:channel_id"`
	ChannelName   string `gorm:"column:channel_name"`
	BillableModel string `gorm:"column:billable_upstream_model"`
}

func costReportAttemptFilterRows(filter CostReportFilter, ignored string) ([]costReportFilterAttemptOptionRow, error) {
	requestQuery, err := costReportRequestQueryIgnoring(filter, "")
	if err != nil {
		return nil, err
	}
	query := model.DB.Table("cost_accounting_attempts AS attempts").
		Select("attempts.channel_id, attempts.channel_name, attempts.billable_upstream_model").
		Joins("JOIN cost_accounting_requests AS requests ON requests.id = attempts.cost_request_id")
	query = applyCostReportRequestQuery(query, requestQuery)
	if ignored != "channel_id" && filter.ChannelID > 0 {
		query = query.Where("attempts.channel_id = ?", filter.ChannelID)
	}
	if ignored != "billable_upstream_model" {
		if value := strings.TrimSpace(filter.BillableUpstreamModel); value != "" {
			query = query.Where("TRIM(attempts.billable_upstream_model) = ?", value)
		}
	}
	rows, err := query.Order("attempts.channel_id ASC, attempts.id ASC").Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]costReportFilterAttemptOptionRow, 0)
	for rows.Next() {
		var row costReportFilterAttemptOptionRow
		if err := query.ScanRows(rows, &row); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func sortedCostReportStrings(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func summarizeRequestLedgerCostProfit(filter CostReportFilter) (CostProfitSummary, error) {
	var summary CostProfitSummary
	requestQuery, err := costReportRequestQuery(filter)
	if err != nil {
		return summary, err
	}
	requestRows, err := requestQuery.Select("requests.*").Order("requests.id ASC").Rows()
	if err != nil {
		return summary, err
	}
	for requestRows.Next() {
		var request model.CostAccountingRequest
		if err := requestQuery.ScanRows(requestRows, &request); err != nil {
			requestRows.Close()
			return summary, err
		}
		if request.RevenueStatus == string(types.CostRevenueFailed) {
			summary.RevenueFailedCount++
		}
		if request.ProfitStatus != string(types.CostProfitComplete) {
			continue
		}
		if request.BilledRevenueEquivalentNanoUSD == nil || request.BilledGrossProfitNanoUSD == nil {
			requestRows.Close()
			return summary, costReportInconsistent("complete request has missing realized amounts", request.ID)
		}
		summary.CompleteRequestCount++
		if *request.BilledGrossProfitNanoUSD < 0 {
			summary.NegativeProfitRequestCount++
		}
		summary.RealizedRevenueNanoUSD, err = costReportAdd(summary.RealizedRevenueNanoUSD, *request.BilledRevenueEquivalentNanoUSD)
		if err != nil {
			requestRows.Close()
			return summary, err
		}
		summary.RealizedCostNanoUSD, err = costReportAdd(summary.RealizedCostNanoUSD, request.ConfirmedCostNanoUSD)
		if err != nil {
			requestRows.Close()
			return summary, err
		}
		summary.RealizedProfitNanoUSD, err = costReportAdd(summary.RealizedProfitNanoUSD, *request.BilledGrossProfitNanoUSD)
		if err != nil {
			requestRows.Close()
			return summary, err
		}
	}
	if err := requestRows.Err(); err != nil {
		requestRows.Close()
		return summary, err
	}
	requestRows.Close()

	attemptQuery, err := costReportAttemptQuery(filter)
	if err != nil {
		return summary, err
	}
	attemptRows, err := attemptQuery.Order("requests.id ASC, attempts.attempt_no ASC, attempts.id ASC").Rows()
	if err != nil {
		return summary, err
	}
	defer attemptRows.Close()
	for attemptRows.Next() {
		var joined costReportJoinedRow
		if err := attemptQuery.ScanRows(attemptRows, &joined); err != nil {
			return summary, err
		}
		incrementCostAttemptCounts(&summary.AwaitingMeterCount, &summary.UnknownCostCount, &summary.SettlementFailedCount, joined.AttemptStatus)
		if joined.AttemptNo > 1 {
			summary.RetryAttemptCount++
		}
		if joined.RequestProfitStatus == string(types.CostProfitComplete) {
			continue
		}
		cost, known, err := knownJoinedCostAttemptAmount(joined)
		if err != nil {
			return summary, err
		}
		if known {
			summary.KnownIncompleteCostNanoUSD, err = costReportAdd(summary.KnownIncompleteCostNanoUSD, cost)
			if err != nil {
				return summary, err
			}
		}
	}
	return summary, attemptRows.Err()
}

func summarizeFilteredCostProfit(filter CostReportFilter) (CostProfitSummary, error) {
	var summary CostProfitSummary
	if err := validateCostReportWinners(filter); err != nil {
		return summary, err
	}
	query, err := costReportAttemptQuery(filter)
	if err != nil {
		return summary, err
	}
	sqlRows, err := query.Order("requests.id ASC, attempts.attempt_no ASC, attempts.id ASC").Rows()
	if err != nil {
		return summary, err
	}
	defer sqlRows.Close()

	currentID := int64(0)
	current := costReportJoinedRow{}
	amounts := costAttributedAmount{}
	knownIncomplete := int64(0)
	flush := func() error {
		if currentID == 0 {
			return nil
		}
		if current.RequestRevenueStatus == string(types.CostRevenueFailed) {
			summary.RevenueFailedCount++
		}
		if current.RequestProfitStatus != string(types.CostProfitComplete) {
			var err error
			summary.KnownIncompleteCostNanoUSD, err = costReportAdd(summary.KnownIncompleteCostNanoUSD, knownIncomplete)
			return err
		}
		summary.CompleteRequestCount++
		profit, err := costReportSubtract(amounts.Revenue, amounts.Cost)
		if err != nil {
			return err
		}
		if profit < 0 {
			summary.NegativeProfitRequestCount++
		}
		summary.RealizedRevenueNanoUSD, err = costReportAdd(summary.RealizedRevenueNanoUSD, amounts.Revenue)
		if err != nil {
			return err
		}
		summary.RealizedCostNanoUSD, err = costReportAdd(summary.RealizedCostNanoUSD, amounts.Cost)
		if err != nil {
			return err
		}
		summary.RealizedProfitNanoUSD, err = costReportAdd(summary.RealizedProfitNanoUSD, profit)
		return err
	}

	for sqlRows.Next() {
		var joined costReportJoinedRow
		if err := query.ScanRows(sqlRows, &joined); err != nil {
			return summary, err
		}
		if currentID != joined.RequestID {
			if err := flush(); err != nil {
				return summary, err
			}
			currentID = joined.RequestID
			current = joined
			amounts = costAttributedAmount{}
			knownIncomplete = 0
		}
		incrementCostAttemptCounts(&summary.AwaitingMeterCount, &summary.UnknownCostCount, &summary.SettlementFailedCount, joined.AttemptStatus)
		if joined.AttemptNo > 1 {
			summary.RetryAttemptCount++
		}
		cost, known, err := knownJoinedCostAttemptAmount(joined)
		if err != nil {
			return summary, err
		}
		if joined.RequestProfitStatus != string(types.CostProfitComplete) {
			if known {
				knownIncomplete, err = costReportAdd(knownIncomplete, cost)
				if err != nil {
					return summary, err
				}
			}
			continue
		}
		if !known {
			return summary, costReportInconsistent("complete request has an incomplete attempt", joined.RequestID)
		}
		amounts.Cost, err = costReportAdd(amounts.Cost, cost)
		if err != nil {
			return summary, err
		}
		if joined.WinningAttemptID != nil && *joined.WinningAttemptID == joined.AttemptID {
			if joined.RequestRevenue == nil {
				return summary, costReportInconsistent("complete request has missing revenue", joined.RequestID)
			}
			amounts.Revenue, err = costReportAdd(amounts.Revenue, *joined.RequestRevenue)
			if err != nil {
				return summary, err
			}
		}
	}
	if err := sqlRows.Err(); err != nil {
		return summary, err
	}
	return summary, flush()
}

func costReportRequestQuery(filter CostReportFilter) (*gorm.DB, error) {
	return costReportRequestQueryIgnoring(filter, "")
}

func costReportRequestQueryIgnoring(filter CostReportFilter, ignored string) (*gorm.DB, error) {
	if filter.StartTime > 0 && filter.EndTime > 0 && filter.StartTime > filter.EndTime {
		return nil, errors.New("cost report start time must not exceed end time")
	}
	timeBasis := strings.TrimSpace(filter.TimeBasis)
	if timeBasis == "" {
		timeBasis = CostReportTimeProfitRecognized
	}
	if timeBasis != CostReportTimeProfitRecognized && timeBasis != CostReportTimeRequested {
		return nil, errors.New("unsupported cost report time basis")
	}
	query := model.DB.Table("cost_accounting_requests AS requests")
	if ignored != "origin_model" {
		query = applyCostReportTrimmedTextFilter(query, "requests.origin_model_name", filter.OriginModelName)
	}
	if ignored != "user_group" {
		query = applyCostReportTrimmedTextFilter(query, "requests.user_group", filter.UserGroup)
	}
	if ignored != "using_group" {
		query = applyCostReportTrimmedTextFilter(query, "requests.using_group", filter.UsingGroup)
	}
	if value := strings.TrimSpace(filter.BillingSource); value != "" {
		query = query.Where("requests.billing_source = ?", value)
	}
	if value := strings.TrimSpace(filter.Status); value != "" {
		query = query.Where("requests.profit_status = ?", value)
	}
	if timeBasis == CostReportTimeRequested {
		if filter.StartTime > 0 {
			query = query.Where("requests.requested_at >= ?", filter.StartTime)
		}
		if filter.EndTime > 0 {
			query = query.Where("requests.requested_at <= ?", filter.EndTime)
		}
	} else {
		if filter.StartTime > 0 {
			query = query.Where(
				"(requests.profit_status = ? AND requests.profit_recognized_at >= ?) OR (requests.profit_status <> ? AND requests.requested_at >= ?)",
				types.CostProfitComplete, filter.StartTime, types.CostProfitComplete, filter.StartTime,
			)
		}
		if filter.EndTime > 0 {
			query = query.Where(
				"(requests.profit_status = ? AND requests.profit_recognized_at <= ?) OR (requests.profit_status <> ? AND requests.requested_at <= ?)",
				types.CostProfitComplete, filter.EndTime, types.CostProfitComplete, filter.EndTime,
			)
		}
	}
	return query, nil
}

func applyCostReportAttemptSelectionToRequestQuery(query *gorm.DB, filter CostReportFilter, ignored string) *gorm.DB {
	conditions := ""
	args := make([]any, 0, 2)
	if ignored != "channel_id" && filter.ChannelID > 0 {
		conditions += " AND selected_attempts.channel_id = ?"
		args = append(args, filter.ChannelID)
	}
	if ignored != "billable_upstream_model" {
		if value := strings.TrimSpace(filter.BillableUpstreamModel); value != "" {
			conditions += " AND TRIM(selected_attempts.billable_upstream_model) = ?"
			args = append(args, value)
		}
	}
	if conditions == "" {
		return query
	}
	return query.Where("EXISTS (SELECT 1 FROM cost_accounting_attempts AS selected_attempts WHERE selected_attempts.cost_request_id = requests.id"+conditions+")", args...)
}

func applyCostReportTrimmedTextFilter(query *gorm.DB, column, rawValue string) *gorm.DB {
	value := strings.TrimSpace(rawValue)
	if value == "" {
		return query
	}
	return query.Where("TRIM("+column+") = ?", value)
}

func costReportAttemptQuery(filter CostReportFilter) (*gorm.DB, error) {
	requestQuery, err := costReportRequestQuery(filter)
	if err != nil {
		return nil, err
	}
	query := model.DB.Table("cost_accounting_attempts AS attempts").
		Select("requests.id AS request_id, requests.winning_attempt_id, requests.billed_revenue_equivalent_nano_usd AS request_revenue, requests.billed_gross_profit_nano_usd AS request_profit, requests.confirmed_cost_nano_usd AS request_confirmed_cost, requests.profit_status AS request_profit_status, requests.revenue_status AS request_revenue_status, attempts.id AS attempt_id, attempts.attempt_no, attempts.channel_id, attempts.channel_name, attempts.billable_upstream_model, attempts.status AS attempt_status, attempts.cost_nano_usd AS attempt_cost").
		Joins("JOIN cost_accounting_requests AS requests ON requests.id = attempts.cost_request_id")
	query = applyCostReportRequestQuery(query, requestQuery)
	if filter.ChannelID > 0 {
		query = query.Where("attempts.channel_id = ?", filter.ChannelID)
	}
	if value := strings.TrimSpace(filter.BillableUpstreamModel); value != "" {
		query = query.Where("TRIM(attempts.billable_upstream_model) = ?", value)
	}
	return query, nil
}

func applyCostReportRequestQuery(query, requestQuery *gorm.DB) *gorm.DB {
	// GORM does not expose a portable way to copy a built WHERE clause between
	// aliased tables, so request filters are applied once more to the joined
	// query using the same structured predicates.
	return query.Where(requestQuery.Statement.Clauses["WHERE"].Expression)
}

func validateCostReportWinners(filter CostReportFilter) error {
	query, err := costReportRequestQuery(filter)
	if err != nil {
		return err
	}
	if filter.ChannelID > 0 || strings.TrimSpace(filter.BillableUpstreamModel) != "" {
		selected := "EXISTS (SELECT 1 FROM cost_accounting_attempts AS selected_attempts WHERE selected_attempts.cost_request_id = requests.id"
		args := []any{}
		if filter.ChannelID > 0 {
			selected += " AND selected_attempts.channel_id = ?"
			args = append(args, filter.ChannelID)
		}
		if value := strings.TrimSpace(filter.BillableUpstreamModel); value != "" {
			selected += " AND TRIM(selected_attempts.billable_upstream_model) = ?"
			args = append(args, value)
		}
		selected += ")"
		query = query.Where(selected, args...)
	}
	invalidWinner := "requests.profit_status = ? AND (requests.billed_revenue_equivalent_nano_usd IS NULL OR (requests.billed_revenue_equivalent_nano_usd <> 0 AND (requests.winning_attempt_id IS NULL OR NOT EXISTS (SELECT 1 FROM cost_accounting_attempts AS winner_attempt WHERE winner_attempt.id = requests.winning_attempt_id AND winner_attempt.cost_request_id = requests.id))))"
	query = query.Where(invalidWinner, types.CostProfitComplete)
	var requestID int64
	if err := query.Select("requests.id").Limit(1).Scan(&requestID).Error; err != nil {
		return err
	}
	if requestID != 0 {
		return costReportInconsistent("complete request cannot be attributed to a persisted winner", requestID)
	}
	return nil
}

func (row costReportJoinedRow) attempt() model.CostAccountingAttempt {
	return model.CostAccountingAttempt{
		ID: row.AttemptID, AttemptNo: row.AttemptNo, ChannelID: row.ChannelID,
		ChannelName: row.ChannelName, BillableUpstreamModel: row.BillableModel,
		Status: row.AttemptStatus, CostNanoUSD: row.AttemptCost,
	}
}

func knownJoinedCostAttemptAmount(row costReportJoinedRow) (int64, bool, error) {
	return knownCostAttemptAmount(row.attempt())
}

func costReportInconsistent(message string, id int64) error {
	logger.LogWarn(context.Background(), fmt.Sprintf("cost report data inconsistency: %s id=%d", message, id))
	return ErrCostReportInconsistent
}

func costAnomalyUnionQuery(filter CostAnomalyFilter) (string, []any) {
	requestBase := []string{}
	requestArgs := []any{}
	if filter.ChannelID > 0 {
		requestBase = append(requestBase, "EXISTS (SELECT 1 FROM cost_accounting_attempts AS channel_attempt WHERE channel_attempt.cost_request_id = requests.id AND channel_attempt.channel_id = ?)")
		requestArgs = append(requestArgs, filter.ChannelID)
	}
	if filter.StartTime > 0 {
		requestBase = append(requestBase, "requests.updated_at >= ?")
		requestArgs = append(requestArgs, filter.StartTime)
	}
	if filter.EndTime > 0 {
		requestBase = append(requestBase, "requests.updated_at <= ?")
		requestArgs = append(requestArgs, filter.EndTime)
	}
	requestBranches := []string{}
	requestBranchArgs := []any{}
	if filter.Kind == "" || filter.Kind == "orphaned_task" {
		conditions := append([]string{"requests.failure_code = ?"}, requestBase...)
		args := append([]any{"orphaned_task_insert_failed"}, requestArgs...)
		requestBranches = append(requestBranches, "SELECT 'orphaned_task' AS kind, requests.id AS request_id, NULL AS attempt_id, requests.updated_at AS occurred_at, 0 AS sort_attempt_id FROM cost_accounting_requests AS requests WHERE "+strings.Join(conditions, " AND "))
		requestBranchArgs = append(requestBranchArgs, args...)
	}
	if filter.Kind == "" || filter.Kind == "revenue_failed" {
		conditions := append([]string{"requests.revenue_status = ? AND requests.failure_code <> ?"}, requestBase...)
		args := append([]any{types.CostRevenueFailed, "orphaned_task_insert_failed"}, requestArgs...)
		requestBranches = append(requestBranches, "SELECT 'revenue_failed' AS kind, requests.id AS request_id, NULL AS attempt_id, requests.updated_at AS occurred_at, 0 AS sort_attempt_id FROM cost_accounting_requests AS requests WHERE "+strings.Join(conditions, " AND "))
		requestBranchArgs = append(requestBranchArgs, args...)
	}

	attemptConditions := []string{"attempts.status IN (?, ?)"}
	attemptArgs := []any{types.CostAttemptUnknown, types.CostAttemptSettlementFailed}
	if filter.Kind == string(types.CostAttemptUnknown) {
		attemptConditions = []string{"attempts.status = ?"}
		attemptArgs = []any{types.CostAttemptUnknown}
	} else if filter.Kind == string(types.CostAttemptSettlementFailed) {
		attemptConditions = []string{"attempts.status = ?"}
		attemptArgs = []any{types.CostAttemptSettlementFailed}
	} else if filter.Kind != "" {
		attemptConditions = []string{"1 = 0"}
		attemptArgs = nil
	}
	if filter.ChannelID > 0 {
		attemptConditions = append(attemptConditions, "attempts.channel_id = ?")
		attemptArgs = append(attemptArgs, filter.ChannelID)
	}
	if filter.StartTime > 0 {
		attemptConditions = append(attemptConditions, "attempts.updated_at >= ?")
		attemptArgs = append(attemptArgs, filter.StartTime)
	}
	if filter.EndTime > 0 {
		attemptConditions = append(attemptConditions, "attempts.updated_at <= ?")
		attemptArgs = append(attemptArgs, filter.EndTime)
	}
	attemptSQL := "SELECT attempts.status AS kind, attempts.cost_request_id AS request_id, attempts.id AS attempt_id, attempts.updated_at AS occurred_at, attempts.id AS sort_attempt_id FROM cost_accounting_attempts AS attempts WHERE " + strings.Join(attemptConditions, " AND ")
	branches := append(requestBranches, attemptSQL)
	args := append(requestBranchArgs, attemptArgs...)
	return strings.Join(branches, " UNION ALL "), args
}

func knownCostAttemptAmount(attempt model.CostAccountingAttempt) (int64, bool, error) {
	switch types.CostAttemptStatus(attempt.Status) {
	case types.CostAttemptSettled:
		if attempt.CostNanoUSD == nil || *attempt.CostNanoUSD < 0 {
			return 0, false, errors.New("settled cost attempt has an invalid amount")
		}
		return *attempt.CostNanoUSD, true, nil
	case types.CostAttemptConfirmedZero, types.CostAttemptNotDispatched:
		return 0, true, nil
	default:
		return 0, false, nil
	}
}

func incrementCostAttemptCounts(awaiting, unknown, failed *int64, status string) {
	switch types.CostAttemptStatus(status) {
	case types.CostAttemptAwaitingMeter:
		(*awaiting)++
	case types.CostAttemptUnknown:
		(*unknown)++
	case types.CostAttemptSettlementFailed:
		(*failed)++
	}
}

func costReportAdd(left, right int64) (int64, error) {
	value, err := CheckedNanoAdd(left, right)
	if err != nil {
		return 0, costReportOverflow("addition", left, right, err)
	}
	return value, nil
}

func costReportSubtract(left, right int64) (int64, error) {
	value, err := CheckedNanoSubtract(left, right)
	if err != nil {
		return 0, costReportOverflow("subtraction", left, right, err)
	}
	return value, nil
}

func costReportMargin(profit, revenue int64) (*int64, error) {
	margin, err := GrossMarginPPM(profit, revenue)
	if err != nil {
		return nil, costReportOverflow("margin", profit, revenue, err)
	}
	return margin, nil
}

func costReportOverflow(operation string, left, right int64, cause error) error {
	logger.LogWarn(context.Background(), fmt.Sprintf("cost report arithmetic overflow: operation=%s left=%d right=%d error=%v", operation, left, right, cause))
	return ErrCostReportOverflow
}
