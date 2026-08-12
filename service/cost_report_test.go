package service

import (
	"errors"
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProfitAttributionKeepsRetryCostsOnTheirChannelsAndRevenueOnWinner(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	now := common.GetTimestamp()
	request, retry, winner := seedCompleteCostReportRequest(t, "attribution", now, now)

	summary, err := SummarizeCostProfit(CostReportFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(1_000_000_000), summary.RealizedRevenueNanoUSD)
	assert.Equal(t, int64(300_000_000), summary.RealizedCostNanoUSD)
	assert.Equal(t, int64(700_000_000), summary.RealizedProfitNanoUSD)
	require.NotNil(t, summary.GrossMarginPPM)
	assert.Equal(t, int64(700_000), *summary.GrossMarginPPM)
	assert.Equal(t, int64(1), summary.CompleteRequestCount)
	assert.Equal(t, int64(1), summary.RetryAttemptCount)

	rows, err := BreakDownCostProfit(CostReportFilter{})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	retryRow := costBreakdownRowForChannel(t, rows, retry.ChannelID)
	assert.Equal(t, int64(0), retryRow.RealizedRevenueNanoUSD)
	assert.Equal(t, int64(100_000_000), retryRow.RealizedCostNanoUSD)
	assert.Equal(t, int64(-100_000_000), retryRow.RealizedProfitNanoUSD)
	assert.Nil(t, retryRow.GrossMarginPPM)
	winnerRow := costBreakdownRowForChannel(t, rows, winner.ChannelID)
	assert.Equal(t, int64(1_000_000_000), winnerRow.RealizedRevenueNanoUSD)
	assert.Equal(t, int64(200_000_000), winnerRow.RealizedCostNanoUSD)
	assert.Equal(t, int64(800_000_000), winnerRow.RealizedProfitNanoUSD)
	require.NotNil(t, request.WinningAttemptID)
	assert.Equal(t, winner.ID, *request.WinningAttemptID)

	var revenueTotal, costTotal, profitTotal int64
	for _, row := range rows {
		revenueTotal += row.RealizedRevenueNanoUSD
		costTotal += row.RealizedCostNanoUSD
		profitTotal += row.RealizedProfitNanoUSD
	}
	assert.Equal(t, summary.RealizedRevenueNanoUSD, revenueTotal)
	assert.Equal(t, summary.RealizedCostNanoUSD, costTotal)
	assert.Equal(t, summary.RealizedProfitNanoUSD, profitTotal)

	retrySummary, err := SummarizeCostProfit(CostReportFilter{ChannelID: retry.ChannelID})
	require.NoError(t, err)
	assert.Equal(t, int64(0), retrySummary.RealizedRevenueNanoUSD)
	assert.Equal(t, int64(100_000_000), retrySummary.RealizedCostNanoUSD)
	assert.Equal(t, int64(-100_000_000), retrySummary.RealizedProfitNanoUSD)
	assert.Nil(t, retrySummary.GrossMarginPPM)
}

func TestCostReportDefaultsToProfitRecognizedTimeAndExcludesIncompleteAmounts(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	now := common.GetTimestamp()
	seedCompleteCostReportRequest(t, "inside", now-100, now)
	outside, _, _ := seedCompleteCostReportRequest(t, "outside-profit-window", now, now-10_000)
	outsideRevenue := int64(2_000_000_000)
	outsideProfit := int64(1_700_000_000)
	require.NoError(t, model.DB.Model(&model.CostAccountingRequest{}).Where("id = ?", outside.ID).Updates(map[string]any{
		"billed_revenue_equivalent_nano_usd": outsideRevenue,
		"billed_gross_profit_nano_usd":       outsideProfit,
	}).Error)
	seedIncompleteCostReportRequest(t, "incomplete", now)

	filter := CostReportFilter{StartTime: now - 10, EndTime: now + 10}
	summary, err := SummarizeCostProfit(filter)
	require.NoError(t, err)
	assert.Equal(t, int64(1_000_000_000), summary.RealizedRevenueNanoUSD)
	assert.Equal(t, int64(300_000_000), summary.RealizedCostNanoUSD)
	assert.Equal(t, int64(700_000_000), summary.RealizedProfitNanoUSD)
	assert.Equal(t, int64(100_000_000), summary.KnownIncompleteCostNanoUSD)
	assert.Equal(t, int64(1), summary.UnknownCostCount)

	filter.TimeBasis = CostReportTimeRequested
	summary, err = SummarizeCostProfit(filter)
	require.NoError(t, err)
	assert.Equal(t, int64(2_000_000_000), summary.RealizedRevenueNanoUSD)
	assert.Equal(t, int64(300_000_000), summary.RealizedCostNanoUSD)
	assert.Equal(t, int64(1_700_000_000), summary.RealizedProfitNanoUSD)
}

func TestCostRequestDetailAndAnomalyQueueExposeLedgerTimeline(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	now := common.GetTimestamp()
	request, retry, winner := seedCompleteCostReportRequest(t, "detail", now, now)
	auditAttemptID := retry.ID
	require.NoError(t, model.DB.Create(&model.CostAccountingAudit{
		CostRequestID: request.ID, CostAttemptID: &auditAttemptID, AdminID: 91,
		OldState: string(types.CostAttemptUnknown), NewState: string(types.CostAttemptSettled),
		Reason: "supplier invoice", CreatedAt: now,
	}).Error)

	detail, err := GetCostRequestDetail(request.ID)
	require.NoError(t, err)
	assert.Equal(t, request.ID, detail.Request.ID)
	require.Len(t, detail.Attempts, 2)
	assert.False(t, detail.Attempts[0].Winning)
	assert.Equal(t, retry.ID, detail.Attempts[0].Attempt.ID)
	assert.True(t, detail.Attempts[1].Winning)
	assert.Equal(t, winner.ID, detail.Attempts[1].Attempt.ID)
	require.Len(t, detail.Audits, 1)

	unknownRequest, unknownAttempt := seedCostAnomalyAttempt(t, "unknown", types.CostAttemptUnknown, now+1)
	revenueFailed := seedRevenueAnomalyRequest(t, "revenue", "revenue_settlement_failed", now+2)
	orphan := seedRevenueAnomalyRequest(t, "orphan", "orphaned_task_insert_failed", now+3)
	rows, total, err := ListCostAnomalies(CostAnomalyFilter{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	require.Len(t, rows, 3)
	assert.True(t, hasCostAnomaly(rows, "cost_unknown", unknownRequest.ID, unknownAttempt.ID))
	assert.True(t, hasCostAnomaly(rows, "revenue_failed", revenueFailed.ID, 0))
	assert.True(t, hasCostAnomaly(rows, "orphaned_task", orphan.ID, 0))
	summary, err := SummarizeCostProfit(CostReportFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), summary.UnknownCostCount)
	assert.Equal(t, int64(1), summary.RevenueFailedCount)
}

func TestCostReportReturnsStableOverflowError(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	now := common.GetTimestamp()
	seedCostReportRequestAmount(t, "overflow-max", math.MaxInt64, now)
	seedCostReportRequestAmount(t, "overflow-one", 1, now)

	_, err := SummarizeCostProfit(CostReportFilter{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCostReportOverflow))
	assert.Equal(t, "cost_report_overflow", err.Error())
}

func TestProfitAttributionRejectsCompleteRevenueWithoutPersistedWinner(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	now := common.GetTimestamp()
	request, _, _ := seedCompleteCostReportRequest(t, "missing-winner", now, now)
	require.NoError(t, model.DB.Model(&model.CostAccountingRequest{}).Where("id = ?", request.ID).
		Update("winning_attempt_id", nil).Error)

	_, err := BreakDownCostProfit(CostReportFilter{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCostReportInconsistent)
	assert.Equal(t, "cost_report_inconsistent", err.Error())
}

func TestCostReportFilterOptionsRespectFiltersAndReturnStableCandidates(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	now := common.GetTimestamp()
	request := model.CostAccountingRequest{
		RequestID: "filter-options-1", OriginModelName: " client-model ", UserGroup: "group-a", UsingGroup: "using-a",
		BillingSource: "wallet", ProfitStatus: string(types.CostProfitComplete), RequestedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(&request).Error)
	request2 := model.CostAccountingRequest{
		RequestID: "filter-options-2", OriginModelName: "other-model", UserGroup: "group-b", UsingGroup: "using-b",
		BillingSource: "wallet", ProfitStatus: string(types.CostProfitComplete), RequestedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(&request2).Error)
	for _, attempt := range []model.CostAccountingAttempt{
		{CostRequestID: request.ID, AttemptNo: 1, ChannelID: 22, ChannelName: "channel-b", BillableUpstreamModel: "vendor-b", CreatedAt: now, UpdatedAt: now},
		{CostRequestID: request.ID, AttemptNo: 2, ChannelID: 11, ChannelName: "channel-a", BillableUpstreamModel: "vendor-a", CreatedAt: now, UpdatedAt: now},
		{CostRequestID: request.ID, AttemptNo: 3, ChannelID: 0, ChannelName: " ", BillableUpstreamModel: " ", CreatedAt: now, UpdatedAt: now},
		{CostRequestID: request2.ID, AttemptNo: 1, ChannelID: 22, ChannelName: "channel-b-renamed", BillableUpstreamModel: "vendor-b", CreatedAt: now, UpdatedAt: now},
	} {
		require.NoError(t, model.DB.Create(&attempt).Error)
	}

	options, err := ListCostReportFilterOptions(CostReportFilter{UserGroup: "group-a"})
	require.NoError(t, err)
	assert.Equal(t, []CostReportFilterChannel{{ID: 11, Name: "channel-a"}, {ID: 22, Name: "channel-b"}}, options.Channels)
	assert.Equal(t, []string{"vendor-a", "vendor-b"}, options.BillableUpstreamModels)
	assert.Equal(t, []string{"client-model"}, options.OriginModels)
	assert.Equal(t, []string{"group-a", "group-b"}, options.UserGroups)
	assert.Equal(t, []string{"using-a"}, options.UsingGroups)

	options, err = ListCostReportFilterOptions(CostReportFilter{UserGroup: "group-a", BillableUpstreamModel: "vendor-b"})
	require.NoError(t, err)
	assert.Equal(t, []string{"vendor-a", "vendor-b"}, options.BillableUpstreamModels)
	assert.Equal(t, []string{"client-model"}, options.OriginModels)

	options, err = ListCostReportFilterOptions(CostReportFilter{StartTime: now + 1, EndTime: now + 2})
	require.NoError(t, err)
	assert.Empty(t, options.Channels)
	assert.NotNil(t, options.Channels)
}

func TestCostReportFilterOptionsReturnCandidatesThatMatchLedgerFilters(t *testing.T) {
	prepareCostAttemptServiceDB(t)
	now := common.GetTimestamp()
	profitAt := now + 100
	request := model.CostAccountingRequest{
		RequestID: "filter-options-whitespace", OriginModelName: " client-model ",
		UserGroup: " group-a ", UsingGroup: " using-a ", BillingSource: "wallet",
		ProfitStatus: string(types.CostProfitComplete), RequestedAt: now,
		ProfitRecognizedAt: &profitAt, CreatedAt: now, UpdatedAt: profitAt,
	}
	require.NoError(t, model.DB.Create(&request).Error)
	require.NoError(t, model.DB.Create(&model.CostAccountingAttempt{
		CostRequestID: request.ID, AttemptNo: 1, ChannelID: 7, ChannelName: "supplier",
		BillableUpstreamModel: " vendor-model ", CreatedAt: now, UpdatedAt: now,
	}).Error)

	options, err := ListCostReportFilterOptions(CostReportFilter{})
	require.NoError(t, err)
	require.Equal(t, []string{"client-model"}, options.OriginModels)
	require.Equal(t, []string{"group-a"}, options.UserGroups)
	require.Equal(t, []string{"using-a"}, options.UsingGroups)
	require.Equal(t, []string{"vendor-model"}, options.BillableUpstreamModels)

	filtered, err := ListCostReportFilterOptions(CostReportFilter{
		OriginModelName: options.OriginModels[0], UserGroup: options.UserGroups[0],
		UsingGroup: options.UsingGroups[0], BillableUpstreamModel: options.BillableUpstreamModels[0],
	})
	require.NoError(t, err)
	assert.Equal(t, []CostReportFilterChannel{{ID: 7, Name: "supplier"}}, filtered.Channels)
}

func seedCompleteCostReportRequest(t *testing.T, suffix string, requestedAt, profitAt int64) (model.CostAccountingRequest, model.CostAccountingAttempt, model.CostAccountingAttempt) {
	t.Helper()
	revenue := int64(1_000_000_000)
	profit := int64(700_000_000)
	margin := int64(700_000)
	request := model.CostAccountingRequest{
		RequestID: "cost-report-" + suffix, UserGroup: "default", UsingGroup: "default",
		OriginModelName: "client-model", BillingSource: "wallet",
		BilledRevenueEquivalentNanoUSD: &revenue, ConfirmedCostNanoUSD: 300_000_000,
		AttemptCount: 2, BilledGrossProfitNanoUSD: &profit, GrossMarginPPM: &margin,
		RevenueStatus: string(types.CostRevenueSettled), ProfitStatus: string(types.CostProfitComplete),
		RequestedAt: requestedAt, ProfitRecognizedAt: &profitAt, CreatedAt: requestedAt, UpdatedAt: profitAt,
	}
	require.NoError(t, model.DB.Create(&request).Error)
	retryCost := int64(100_000_000)
	retry := model.CostAccountingAttempt{
		CostRequestID: request.ID, AttemptNo: 1, ChannelID: 11, ChannelName: "channel-a",
		BillableUpstreamModel: "vendor-a", CostNanoUSD: &retryCost, OriginalCost: "0.1",
		Status: string(types.CostAttemptSettled), ReconciliationStatus: model.CostReconciliationNone,
		PreparedAt: requestedAt, SettledAt: &profitAt, CreatedAt: requestedAt, UpdatedAt: profitAt,
	}
	require.NoError(t, model.DB.Create(&retry).Error)
	winnerCost := int64(200_000_000)
	winner := model.CostAccountingAttempt{
		CostRequestID: request.ID, AttemptNo: 2, ChannelID: 22, ChannelName: "channel-b",
		BillableUpstreamModel: "vendor-b", CostNanoUSD: &winnerCost, OriginalCost: "0.2",
		Status: string(types.CostAttemptSettled), ReconciliationStatus: model.CostReconciliationNone,
		PreparedAt: requestedAt + 1, SettledAt: &profitAt, CreatedAt: requestedAt + 1, UpdatedAt: profitAt,
	}
	require.NoError(t, model.DB.Create(&winner).Error)
	require.NoError(t, model.DB.Model(&model.CostAccountingRequest{}).Where("id = ?", request.ID).
		Update("winning_attempt_id", winner.ID).Error)
	request.WinningAttemptID = &winner.ID
	return request, retry, winner
}

func seedIncompleteCostReportRequest(t *testing.T, suffix string, requestedAt int64) {
	t.Helper()
	revenue := int64(1_000_000_000)
	request := model.CostAccountingRequest{
		RequestID: "cost-report-" + suffix, OriginModelName: "client-model", BillingSource: "wallet",
		BilledRevenueEquivalentNanoUSD: &revenue, ConfirmedCostNanoUSD: 100_000_000, AttemptCount: 2,
		RevenueStatus: string(types.CostRevenueSettled), ProfitStatus: string(types.CostProfitIncompleteCost),
		RequestedAt: requestedAt, CreatedAt: requestedAt, UpdatedAt: requestedAt,
	}
	require.NoError(t, model.DB.Create(&request).Error)
	known := int64(100_000_000)
	require.NoError(t, model.DB.Create(&model.CostAccountingAttempt{
		CostRequestID: request.ID, AttemptNo: 1, ChannelID: 11, ChannelName: "channel-a",
		BillableUpstreamModel: "vendor-a", CostNanoUSD: &known, Status: string(types.CostAttemptSettled),
		PreparedAt: requestedAt, CreatedAt: requestedAt, UpdatedAt: requestedAt,
	}).Error)
	require.NoError(t, model.DB.Create(&model.CostAccountingAttempt{
		CostRequestID: request.ID, AttemptNo: 2, ChannelID: 22, ChannelName: "channel-b",
		BillableUpstreamModel: "vendor-b", Status: string(types.CostAttemptUnknown),
		PreparedAt: requestedAt, CreatedAt: requestedAt, UpdatedAt: requestedAt,
	}).Error)
}

func seedCostAnomalyAttempt(t *testing.T, suffix string, status types.CostAttemptStatus, timestamp int64) (model.CostAccountingRequest, model.CostAccountingAttempt) {
	t.Helper()
	request := model.CostAccountingRequest{
		RequestID: "cost-anomaly-" + suffix, RevenueStatus: string(types.CostRevenuePending),
		ProfitStatus: string(types.CostProfitIncompleteRevenue), RequestedAt: timestamp, CreatedAt: timestamp, UpdatedAt: timestamp,
	}
	require.NoError(t, model.DB.Create(&request).Error)
	attempt := model.CostAccountingAttempt{
		CostRequestID: request.ID, AttemptNo: 1, ChannelID: 33, ChannelName: "channel-c",
		BillableUpstreamModel: "vendor-c", Status: string(status), FailureCode: string(status),
		PreparedAt: timestamp, CreatedAt: timestamp, UpdatedAt: timestamp,
	}
	require.NoError(t, model.DB.Create(&attempt).Error)
	return request, attempt
}

func seedRevenueAnomalyRequest(t *testing.T, suffix, failureCode string, timestamp int64) model.CostAccountingRequest {
	t.Helper()
	status := types.CostRevenueFailed
	if failureCode == "orphaned_task_insert_failed" {
		status = types.CostRevenueSettled
	}
	request := model.CostAccountingRequest{
		RequestID: "cost-anomaly-" + suffix, RevenueStatus: string(status),
		ProfitStatus: string(types.CostProfitIncompleteRevenue), FailureCode: failureCode,
		RequestedAt: timestamp, CreatedAt: timestamp, UpdatedAt: timestamp,
	}
	require.NoError(t, model.DB.Create(&request).Error)
	return request
}

func seedCostReportRequestAmount(t *testing.T, suffix string, amount, timestamp int64) {
	t.Helper()
	profit := amount
	request := model.CostAccountingRequest{
		RequestID: "cost-report-" + suffix, BilledRevenueEquivalentNanoUSD: &amount,
		BilledGrossProfitNanoUSD: &profit, RevenueStatus: string(types.CostRevenueSettled),
		ProfitStatus: string(types.CostProfitComplete), RequestedAt: timestamp,
		ProfitRecognizedAt: &timestamp, CreatedAt: timestamp, UpdatedAt: timestamp,
	}
	require.NoError(t, model.DB.Create(&request).Error)
}

func costBreakdownRowForChannel(t *testing.T, rows []CostProfitBreakdownRow, channelID int) CostProfitBreakdownRow {
	t.Helper()
	for _, row := range rows {
		if row.ChannelID == channelID {
			return row
		}
	}
	t.Fatalf("missing cost breakdown row for channel %d", channelID)
	return CostProfitBreakdownRow{}
}

func hasCostAnomaly(rows []CostAnomalyRow, kind string, requestID, attemptID int64) bool {
	for _, row := range rows {
		if row.Kind == kind && row.Request.ID == requestID {
			if attemptID == 0 {
				return row.Attempt == nil
			}
			return row.Attempt != nil && row.Attempt.ID == attemptID
		}
	}
	return false
}
