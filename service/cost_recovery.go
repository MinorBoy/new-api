package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
)

const (
	costPreparedRecoveryAge    = 5 * time.Minute
	costDispatchingRecoveryAge = 5 * time.Minute
	costRecoveryDefaultLimit   = 100
	costRecoveryMaxLimit       = 1000
	costRecoveryOrder          = "CASE status WHEN 'prepared' THEN prepared_at WHEN 'dispatching' THEN dispatching_at ELSE updated_at END ASC, id ASC"
)

type CostRecoverySummary struct {
	PreparedClosed     int `json:"prepared_closed"`
	DispatchingUnknown int `json:"dispatching_unknown"`
	AwaitingSettled    int `json:"awaiting_settled"`
	RevenueSettled     int `json:"revenue_settled"`
}

var errTaskRevenueNotReady = errors.New("task revenue is not ready")

func HasRecoverableCostAccounting(ctx context.Context, now time.Time) (bool, error) {
	if ctx == nil {
		return false, errors.New("cost recovery context is required")
	}
	var attempt model.CostAccountingAttempt
	err := costRecoveryQuery(ctx, now).
		Select("id").
		Order(costRecoveryOrder).
		Limit(1).
		Find(&attempt).Error
	if err != nil {
		return false, err
	}
	if attempt.ID != 0 {
		return true, nil
	}

	var request model.CostAccountingRequest
	err = recoverableTaskRevenueQuery(ctx).
		Select("cost_accounting_requests.id").
		Limit(1).
		Find(&request).Error
	if err != nil {
		return false, err
	}
	return request.ID != 0, nil
}

func RecoverStaleCostAccounting(ctx context.Context, now time.Time, limit int) (CostRecoverySummary, error) {
	var summary CostRecoverySummary
	if ctx == nil {
		return summary, errors.New("cost recovery context is required")
	}
	if err := ctx.Err(); err != nil {
		return summary, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	if limit <= 0 {
		limit = costRecoveryDefaultLimit
	}
	if limit > costRecoveryMaxLimit {
		limit = costRecoveryMaxLimit
	}

	var attempts []model.CostAccountingAttempt
	if err := costRecoveryQuery(ctx, now).
		Order(costRecoveryOrder).
		Limit(limit).
		Find(&attempts).Error; err != nil {
		return summary, err
	}

	nowUnix := now.Unix()
	for i := range attempts {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		attempt := &attempts[i]
		var err error
		switch types.CostAttemptStatus(attempt.Status) {
		case types.CostAttemptPrepared:
			err = model.TransitionCostAttemptWithContext(ctx, attempt.ID, types.CostAttemptPrepared, types.CostAttemptNotDispatched, map[string]any{
				"failure_code": "recovery_prepared_stale",
				"terminal_at":  nowUnix,
			})
			if err == nil {
				summary.PreparedClosed++
			}
		case types.CostAttemptDispatching:
			err = model.TransitionCostAttemptWithContext(ctx, attempt.ID, types.CostAttemptDispatching, types.CostAttemptUnknown, map[string]any{
				"failure_code": "recovery_dispatch_outcome_unknown",
				"terminal_at":  nowUnix,
			})
			if err == nil {
				summary.DispatchingUnknown++
			}
		case types.CostAttemptAwaitingMeter:
			var task *model.Task
			if attempt.FailureCode == taskSucceededPendingCostSettlement || attempt.FailureCode == taskFailedPendingCostSettlement {
				task, err = loadTaskForPendingCostSettlement(ctx, attempt)
				if err == nil {
					err = recognizeRecoverableTaskRevenue(ctx, attempt.CostRequestID, task)
				}
				if errors.Is(err, errTaskRevenueNotReady) {
					err = nil
					break
				}
				if errors.Is(err, errAsyncRevenueManualReconciliation) {
					err = nil
				}
				if err != nil {
					break
				}
			}
			if attempt.FailureCode == taskFailedPendingCostSettlement {
				err = SettleAsyncCostAttempt(ctx, attempt.CostRequestID, task, &relaycommon.TaskInfo{
					Status: string(model.TaskStatusFailure),
				})
				if err == nil {
					summary.AwaitingSettled++
				}
				break
			}
			meter, meterErr := recoverableCostMeter(attempt)
			if meterErr != nil {
				err = model.TransitionCostAttemptWithContext(ctx, attempt.ID, types.CostAttemptAwaitingMeter, types.CostAttemptSettlementFailed, map[string]any{
					"actual_meter_json": attempt.ActualMeterJSON,
					"failure_code":      "cost_meter_invalid",
					"terminal_at":       nowUnix,
				})
				if err != nil && !errors.Is(err, model.ErrCostStateConflict) {
					return summary, fmt.Errorf("mark recovered cost attempt %d settlement failed: %w", attempt.ID, err)
				}
				continue
			}
			err = settleCostAttemptFromSnapshot(ctx, attempt, types.CostAttemptAwaitingMeter, meter)
			if err == nil {
				summary.AwaitingSettled++
			}
		}
		if err != nil && !errors.Is(err, model.ErrCostStateConflict) {
			return summary, fmt.Errorf("recover cost attempt %d: %w", attempt.ID, err)
		}
	}

	remaining := limit - len(attempts)
	if remaining <= 0 {
		return summary, nil
	}
	var requests []model.CostAccountingRequest
	if err := recoverableTaskRevenueQuery(ctx).
		Order("cost_accounting_requests.updated_at ASC, cost_accounting_requests.id ASC").
		Limit(remaining).
		Find(&requests).Error; err != nil {
		return summary, err
	}
	for i := range requests {
		request := &requests[i]
		attempt := &model.CostAccountingAttempt{CostRequestID: request.ID}
		task, err := loadTaskForPendingCostSettlement(ctx, attempt)
		if err != nil {
			return summary, fmt.Errorf("load task revenue request %d: %w", request.ID, err)
		}
		if task.Status == model.TaskStatusFailure && task.Quota == 0 {
			if err := SettleAsyncCostAttempt(ctx, request.ID, task, &relaycommon.TaskInfo{
				Status: string(model.TaskStatusFailure), Reason: task.FailReason,
			}); err != nil && !errors.Is(err, model.ErrCostStateConflict) {
				return summary, fmt.Errorf("settle failed task cost request %d: %w", request.ID, err)
			}
		}
		if err := recognizeRecoverableTaskRevenue(ctx, request.ID, task); err != nil {
			if errors.Is(err, errTaskRevenueNotReady) {
				continue
			}
			return summary, fmt.Errorf("recover task revenue request %d: %w", request.ID, err)
		}
		summary.RevenueSettled++
	}
	return summary, nil
}

func recoverableTaskRevenueQuery(ctx context.Context) *gorm.DB {
	return model.DB.WithContext(ctx).
		Model(&model.CostAccountingRequest{}).
		Distinct("cost_accounting_requests.*").
		Joins("JOIN tasks ON tasks.task_id = cost_accounting_requests.task_id").
		Where("cost_accounting_requests.revenue_status = ? OR (cost_accounting_requests.revenue_status = ? AND cost_accounting_requests.failure_code = ?)",
			types.CostRevenuePending, types.CostRevenueFailed, costRevenueRecognitionFailureCode).
		Where("tasks.status IN ?", []model.TaskStatus{model.TaskStatusSuccess, model.TaskStatusFailure})
}

func recognizeRecoverableTaskRevenue(ctx context.Context, costRequestID int64, task *model.Task) error {
	if task == nil || (task.Status != model.TaskStatusSuccess && task.Status != model.TaskStatusFailure) {
		return errTaskRevenueNotReady
	}
	if task.Status == model.TaskStatusFailure && task.Quota != 0 {
		return errTaskRevenueNotReady
	}
	return recognizeAsyncBilledRevenue(ctx, costRequestID, task.Quota)
}

func loadTaskForPendingCostSettlement(ctx context.Context, attempt *model.CostAccountingAttempt) (*model.Task, error) {
	var request model.CostAccountingRequest
	if err := model.DB.WithContext(ctx).
		Select("id", "task_id").
		Where("id = ?", attempt.CostRequestID).
		First(&request).Error; err != nil {
		return nil, err
	}
	if request.TaskID == nil {
		return nil, errors.New("pending cost settlement request has no task ID")
	}

	var tasks []model.Task
	if err := model.DB.WithContext(ctx).Where("task_id = ?", *request.TaskID).Find(&tasks).Error; err != nil {
		return nil, err
	}
	for i := range tasks {
		if tasks[i].PrivateData.CostRequestID == request.ID {
			return &tasks[i], nil
		}
	}
	return nil, errors.New("pending cost settlement task was not found")
}

func ReconcileCostAttempt(ctx context.Context, attemptID int64, adminID int, action string, meter *types.CostMeter, reason string) error {
	if ctx == nil {
		return errors.New("cost reconciliation context is required")
	}
	if attemptID <= 0 || adminID <= 0 {
		return errors.New("cost reconciliation identity is invalid")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errors.New("reconciliation reason is required")
	}

	var attempt model.CostAccountingAttempt
	if err := model.DB.WithContext(ctx).Where("id = ?", attemptID).First(&attempt).Error; err != nil {
		return err
	}
	status := types.CostAttemptStatus(attempt.Status)
	if status != types.CostAttemptSettlementFailed && status != types.CostAttemptUnknown {
		return model.ErrCostStateConflict
	}

	input := model.ReconcileCostAttemptInput{
		AttemptID: attempt.ID,
		AdminID:   adminID,
		Reason:    reason,
		MeterJSON: "{}",
	}
	switch strings.TrimSpace(action) {
	case "settle":
		actualMeter, err := costReconciliationMeter(&attempt, meter)
		if err != nil {
			return err
		}
		var config types.CostRuleConfigV1
		if err := common.UnmarshalJsonStr(attempt.RuleConfigJSON, &config); err != nil {
			return fmt.Errorf("decode cost rule snapshot: %w", err)
		}
		originalCost, costNanoUSD, err := CalculateAttemptCost(types.CostMode(attempt.CostMode), config, actualMeter)
		if err != nil {
			return err
		}
		meterJSON, err := common.Marshal(actualMeter)
		if err != nil {
			return err
		}
		input.To = types.CostAttemptSettled
		input.MeterJSON = string(meterJSON)
		input.OriginalCost = originalCost
		input.CostNanoUSD = &costNanoUSD
	case "confirm_zero":
		if meter != nil {
			if strings.TrimSpace(attempt.MeterSource) == "" {
				return errors.New("attempt snapshot does not declare a meter source")
			}
			actualMeter, err := costReconciliationMeter(&attempt, meter)
			if err != nil {
				return err
			}
			meterJSON, err := common.Marshal(actualMeter)
			if err != nil {
				return err
			}
			input.MeterJSON = string(meterJSON)
		}
		zero := int64(0)
		input.To = types.CostAttemptConfirmedZero
		input.OriginalCost = "0"
		input.CostNanoUSD = &zero
	default:
		return errors.New("unsupported reconciliation action")
	}
	return model.ReconcileCostAttemptWithContext(ctx, input)
}

func ReconcileCostRevenue(ctx context.Context, requestID int64, adminID int, finalQuota int64, reason string) error {
	if ctx == nil {
		return errors.New("cost reconciliation context is required")
	}
	if requestID <= 0 || adminID <= 0 {
		return errors.New("cost reconciliation identity is invalid")
	}
	if finalQuota < 0 {
		return errors.New("final user quota cannot be negative")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errors.New("reconciliation reason is required")
	}

	var request model.CostAccountingRequest
	if err := model.DB.WithContext(ctx).Where("id = ?", requestID).First(&request).Error; err != nil {
		return err
	}
	if types.CostRevenueStatus(request.RevenueStatus) != types.CostRevenueFailed {
		return model.ErrCostStateConflict
	}
	revenueNanoUSD, err := RevenueEquivalentNanoUSD(finalQuota, request.QuotaPerUnitSnapshot)
	if err != nil {
		return err
	}
	detailsJSON, err := common.Marshal(map[string]any{
		"final_user_quota":        finalQuota,
		"quota_per_unit_snapshot": request.QuotaPerUnitSnapshot,
	})
	if err != nil {
		return err
	}
	target := types.CostRevenueSettled
	if finalQuota == 0 {
		target = types.CostRevenueConfirmedZero
	}
	return model.ReconcileCostRevenueWithContext(ctx, model.ReconcileCostRevenueInput{
		CostRequestID:        request.ID,
		AdminID:              adminID,
		To:                   target,
		FinalUserQuota:       finalQuota,
		QuotaPerUnitSnapshot: request.QuotaPerUnitSnapshot,
		RevenueNanoUSD:       revenueNanoUSD,
		MeterJSON:            string(detailsJSON),
		Reason:               reason,
	})
}

func costRecoveryQuery(ctx context.Context, now time.Time) *gorm.DB {
	preparedCutoff := now.Add(-costPreparedRecoveryAge).Unix()
	dispatchingCutoff := now.Add(-costDispatchingRecoveryAge).Unix()
	return model.DB.WithContext(ctx).Where(
		"(status = ? AND prepared_at <= ?) OR (status = ? AND dispatching_at IS NOT NULL AND dispatching_at <= ?) OR (status = ? AND actual_meter_json <> ?)",
		types.CostAttemptPrepared, preparedCutoff,
		types.CostAttemptDispatching, dispatchingCutoff,
		types.CostAttemptAwaitingMeter, "",
	)
}

func recoverableCostMeter(attempt *model.CostAccountingAttempt) (types.CostMeter, error) {
	if attempt == nil || strings.TrimSpace(attempt.ActualMeterJSON) == "" {
		return types.CostMeter{}, errors.New("actual cost meter is missing")
	}
	var meter types.CostMeter
	if err := common.UnmarshalJsonStr(attempt.ActualMeterJSON, &meter); err != nil {
		return types.CostMeter{}, fmt.Errorf("decode actual cost meter: %w", err)
	}
	actualMeter, err := costReconciliationMeter(attempt, &meter)
	if err != nil {
		return types.CostMeter{}, err
	}
	var config types.CostRuleConfigV1
	if err := common.UnmarshalJsonStr(attempt.RuleConfigJSON, &config); err != nil {
		return types.CostMeter{}, fmt.Errorf("decode cost rule snapshot: %w", err)
	}
	if _, _, err := CalculateAttemptCost(types.CostMode(attempt.CostMode), config, actualMeter); err != nil {
		return types.CostMeter{}, err
	}
	return actualMeter, nil
}

func costReconciliationMeter(attempt *model.CostAccountingAttempt, supplied *types.CostMeter) (types.CostMeter, error) {
	var config types.CostRuleConfigV1
	if err := common.UnmarshalJsonStr(attempt.RuleConfigJSON, &config); err != nil {
		return types.CostMeter{}, fmt.Errorf("decode cost rule snapshot: %w", err)
	}
	meter := types.CostMeter{}
	if supplied != nil {
		meter = *supplied
	}
	if config.MeterSource == types.CostMeterValidatedRequest {
		if strings.TrimSpace(attempt.RequestMeterJSON) == "" || attempt.RequestMeterJSON == "{}" {
			return types.CostMeter{}, errors.New("validated request meter snapshot is missing")
		}
		if err := common.UnmarshalJsonStr(attempt.RequestMeterJSON, &meter); err != nil {
			return types.CostMeter{}, fmt.Errorf("decode request cost meter snapshot: %w", err)
		}
	}
	mode := types.CostMode(attempt.CostMode)
	if (mode == types.CostModePerDuration || mode == types.CostModePerToken) && meter.Source != config.MeterSource {
		return types.CostMeter{}, errors.New("meter source does not match the attempt snapshot")
	}
	if err := validateCostMeterBounds(meter); err != nil {
		return types.CostMeter{}, err
	}
	return meter, nil
}
