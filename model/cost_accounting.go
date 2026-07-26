package model

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrCostStateConflict        = errors.New("cost accounting state conflict")
	ErrCostAmountOverflow       = errors.New("cost accounting amount overflow")
	ErrCostRuleSnapshotConflict = errors.New("cost rule snapshot conflict")
)

const (
	CostReconciliationNone       = "none"
	CostReconciliationReconciled = "reconciled"
)

var costAttemptAllocationLocks [256]sync.Mutex

func IsCostSnapshotTransactionConflict(err error) bool {
	for err != nil {
		message := strings.ToLower(err.Error())
		for _, marker := range []string{
			"database is locked",
			"database table is locked",
			"database is busy",
			"sqlite_busy",
			"sqlite_locked",
			"serialization failure",
			"could not serialize",
			"deadlock detected",
			"deadlock found",
			"lock wait timeout",
			"try restarting transaction",
		} {
			if strings.Contains(message, marker) {
				return true
			}
		}
		err = errors.Unwrap(err)
	}
	return false
}

type CostAccountingRequest struct {
	ID                             int64   `json:"id" gorm:"primaryKey"`
	RequestID                      string  `json:"request_id" gorm:"type:varchar(64);uniqueIndex"`
	TaskID                         *string `json:"task_id,omitempty" gorm:"type:varchar(191);uniqueIndex"`
	UpstreamTaskID                 *string `json:"upstream_task_id,omitempty" gorm:"type:varchar(191)"`
	UserID                         int     `json:"user_id" gorm:"index"`
	TokenID                        int     `json:"token_id"`
	UserGroup                      string  `json:"user_group" gorm:"type:varchar(64);index"`
	UsingGroup                     string  `json:"using_group" gorm:"type:varchar(64);index"`
	OriginModelName                string  `json:"origin_model_name" gorm:"type:varchar(191);index"`
	BillingSource                  string  `json:"billing_source" gorm:"type:varchar(32);index"`
	SubscriptionID                 int     `json:"subscription_id"`
	SubscriptionPlanID             int     `json:"subscription_plan_id"`
	FinalUserQuota                 *int64  `json:"final_user_quota,omitempty"`
	QuotaPerUnitSnapshot           string  `json:"quota_per_unit_snapshot" gorm:"type:varchar(64)"`
	BilledRevenueEquivalentNanoUSD *int64  `json:"billed_revenue_equivalent_nano_usd,omitempty"`
	ConfirmedCostNanoUSD           int64   `json:"confirmed_cost_nano_usd"`
	AttemptCount                   int     `json:"attempt_count"`
	WinningAttemptID               *int64  `json:"winning_attempt_id,omitempty"`
	BilledGrossProfitNanoUSD       *int64  `json:"billed_gross_profit_nano_usd,omitempty"`
	GrossMarginPPM                 *int64  `json:"gross_margin_ppm,omitempty"`
	RevenueStatus                  string  `json:"revenue_status" gorm:"type:varchar(32);index"`
	ProfitStatus                   string  `json:"profit_status" gorm:"type:varchar(32);index"`
	FailureCode                    string  `json:"failure_code" gorm:"type:varchar(64);index"`
	RequestedAt                    int64   `json:"requested_at" gorm:"index"`
	RevenueSettledAt               *int64  `json:"revenue_settled_at,omitempty"`
	ProfitRecognizedAt             *int64  `json:"profit_recognized_at,omitempty" gorm:"index"`
	CreatedAt                      int64   `json:"created_at"`
	UpdatedAt                      int64   `json:"updated_at"`
}

type CostAccountingAttempt struct {
	ID                     int64  `json:"id" gorm:"primaryKey"`
	CostRequestID          int64  `json:"cost_request_id" gorm:"uniqueIndex:idx_cost_attempt_no,priority:1;index"`
	AttemptNo              int    `json:"attempt_no" gorm:"uniqueIndex:idx_cost_attempt_no,priority:2"`
	ChannelID              int    `json:"channel_id" gorm:"index"`
	ChannelName            string `json:"channel_name" gorm:"type:varchar(191)"`
	ChannelType            int    `json:"channel_type"`
	PredictedUpstreamModel string `json:"predicted_upstream_model" gorm:"type:varchar(191);index"`
	BillableUpstreamModel  string `json:"billable_upstream_model" gorm:"type:varchar(191);index"`
	CostVariantKey         string `json:"cost_variant_key" gorm:"type:varchar(64);not null;index"`
	RuleID                 int64  `json:"rule_id" gorm:"index"`
	RuleVersion            int    `json:"rule_version"`
	CostMode               string `json:"cost_mode" gorm:"type:varchar(32)"`
	SchemaVersion          int    `json:"schema_version"`
	RuleConfigJSON         string `json:"rule_config_json" gorm:"type:text"`
	ChargeEvent            string `json:"charge_event" gorm:"type:varchar(32)"`
	MeterSource            string `json:"meter_source" gorm:"type:varchar(32)"`
	BillableRequestCount   int    `json:"billable_request_count"`
	RequestMeterJSON       string `json:"request_meter_json" gorm:"type:text"`
	ActualMeterJSON        string `json:"actual_meter_json" gorm:"type:text"`
	OriginalCost           string `json:"original_cost" gorm:"type:text"`
	CostNanoUSD            *int64 `json:"cost_nano_usd,omitempty"`
	UpstreamAccepted       bool   `json:"upstream_accepted"`
	HTTPStatus             int    `json:"http_status"`
	ResultCode             string `json:"result_code" gorm:"type:varchar(64);index"`
	FailureCode            string `json:"failure_code" gorm:"type:varchar(64);index"`
	Status                 string `json:"status" gorm:"type:varchar(32);index"`
	ReconciliationStatus   string `json:"reconciliation_status" gorm:"type:varchar(32);index"`
	PreparedAt             int64  `json:"prepared_at" gorm:"index"`
	DispatchingAt          *int64 `json:"dispatching_at,omitempty"`
	AcceptedAt             *int64 `json:"accepted_at,omitempty"`
	TerminalAt             *int64 `json:"terminal_at,omitempty"`
	SettledAt              *int64 `json:"settled_at,omitempty" gorm:"index"`
	CreatedAt              int64  `json:"created_at"`
	UpdatedAt              int64  `json:"updated_at"`
}

type CostAccountingAudit struct {
	ID               int64  `json:"id" gorm:"primaryKey"`
	CostRequestID    int64  `json:"cost_request_id" gorm:"index"`
	CostAttemptID    *int64 `json:"cost_attempt_id,omitempty" gorm:"index"`
	AdminID          int    `json:"admin_id" gorm:"index"`
	OldState         string `json:"old_state" gorm:"type:varchar(32)"`
	NewState         string `json:"new_state" gorm:"type:varchar(32)"`
	MeterJSON        string `json:"meter_json" gorm:"type:text"`
	RuleID           int64  `json:"rule_id"`
	RuleVersion      int    `json:"rule_version"`
	OldAmountNanoUSD *int64 `json:"old_amount_nano_usd,omitempty"`
	NewAmountNanoUSD *int64 `json:"new_amount_nano_usd,omitempty"`
	Reason           string `json:"reason" gorm:"type:text"`
	CreatedAt        int64  `json:"created_at" gorm:"index"`
}

type SettleCostAttemptInput struct {
	AttemptID        int64
	From             types.CostAttemptStatus
	To               types.CostAttemptStatus
	ActualMeterJSON  string
	OriginalCost     string
	CostNanoUSD      *int64
	UpstreamAccepted bool
	HTTPStatus       int
	ResultCode       string
	FailureCode      string
	AcceptedAt       *int64
	TerminalAt       *int64
	SettledAt        int64
}

type RecognizeCostRevenueInput struct {
	CostRequestID        int64
	From                 types.CostRevenueStatus
	To                   types.CostRevenueStatus
	FinalUserQuota       *int64
	QuotaPerUnitSnapshot string
	RevenueNanoUSD       *int64
	FailureCode          string
	SettledAt            int64
}

type ReconcileCostAttemptInput struct {
	AttemptID    int64
	AdminID      int
	To           types.CostAttemptStatus
	MeterJSON    string
	OriginalCost string
	CostNanoUSD  *int64
	Reason       string
	CreatedAt    int64
}

type ReconcileCostRevenueInput struct {
	CostRequestID        int64
	AdminID              int
	To                   types.CostRevenueStatus
	FinalUserQuota       int64
	QuotaPerUnitSnapshot string
	RevenueNanoUSD       int64
	MeterJSON            string
	Reason               string
	CreatedAt            int64
}

func PrepareCostAttempt(request *CostAccountingRequest, attempt *CostAccountingAttempt) error {
	return prepareCostAttempt(DB, request, attempt, nil, nil)
}

func PrepareCostAttemptWithRuleValidation(
	ctx context.Context,
	request *CostAccountingRequest,
	attempt *CostAccountingAttempt,
	profitSnapshot *types.CostProfitRecheckSnapshot,
	validateRule func(*ChannelModelCostRule) error,
) error {
	if ctx == nil {
		return errors.New("cost attempt context is required")
	}
	if validateRule == nil {
		return errors.New("cost rule validation is required")
	}
	return prepareCostAttempt(DB.WithContext(ctx), request, attempt, profitSnapshot, validateRule)
}

func prepareCostAttempt(
	db *gorm.DB,
	request *CostAccountingRequest,
	attempt *CostAccountingAttempt,
	profitSnapshot *types.CostProfitRecheckSnapshot,
	validateRule func(*ChannelModelCostRule) error,
) error {
	if request == nil || attempt == nil || strings.TrimSpace(request.RequestID) == "" {
		return errors.New("cost request and attempt are required")
	}
	if attempt.Status != "" && attempt.Status != string(types.CostAttemptPrepared) {
		return ErrCostStateConflict
	}
	if attempt.BillableRequestCount != 0 && attempt.BillableRequestCount != 1 {
		return errors.New("billable request count must be one")
	}
	if attempt.AttemptNo <= 0 {
		hash := fnv.New32a()
		_, _ = hash.Write([]byte(request.RequestID))
		allocationLock := &costAttemptAllocationLocks[hash.Sum32()%uint32(len(costAttemptAllocationLocks))]
		allocationLock.Lock()
		defer allocationLock.Unlock()
	}

	return db.Transaction(func(tx *gorm.DB) error {
		now := common.GetTimestamp()
		selectedVariant, err := types.NormalizeCostVariantKey(attempt.CostVariantKey)
		if err != nil {
			return err
		}
		if validateRule != nil {
			if profitSnapshot != nil {
				snapshotVariant, variantErr := types.NormalizeCostVariantKey(profitSnapshot.CostVariantKey)
				if variantErr != nil {
					return ErrCostRuleSnapshotConflict
				}
				if profitSnapshot.ChannelID != attempt.ChannelID || profitSnapshot.BillableUpstreamModel != attempt.BillableUpstreamModel {
					return ErrCostRuleSnapshotConflict
				}
				selectedVariant = snapshotVariant
				if profitSnapshot.RouteTarget != nil {
					targetSnapshot := profitSnapshot.RouteTarget
					var target RouteTarget
					err := lockForUpdate(tx).Where(
						"id = ? AND policy_id = ? AND channel_id = ? AND upstream_model = ? AND cost_variant_key = ? AND enabled = ?",
						targetSnapshot.TargetID,
						targetSnapshot.PolicyID,
						targetSnapshot.ChannelID,
						targetSnapshot.UpstreamModel,
						targetSnapshot.CostVariantKey,
						true,
					).First(&target).Error
					if errors.Is(err, gorm.ErrRecordNotFound) {
						return ErrCostRuleSnapshotConflict
					}
					if err != nil {
						return err
					}
					lockedVariant, variantErr := types.NormalizeCostVariantKey(target.CostVariantKey)
					if variantErr != nil || lockedVariant != selectedVariant ||
						target.TargetPriority != targetSnapshot.Priority ||
						!sameCostProfitMarginThreshold(target.MinimumExpectedMarginBPS, targetSnapshot.MinimumExpectedMarginBPS) {
						return ErrCostRuleSnapshotConflict
					}
				}
			}
			var activeRules []ChannelModelCostRule
			if err := lockForUpdate(tx).Where(
				"channel_id = ? AND billable_upstream_model = ? AND cost_variant_key = ? AND status = ?",
				attempt.ChannelID, attempt.BillableUpstreamModel, selectedVariant, types.CostRuleActive,
			).Order("version DESC").Limit(2).Find(&activeRules).Error; err != nil {
				return err
			}
			if len(activeRules) == 0 {
				return gorm.ErrRecordNotFound
			}
			if len(activeRules) > 1 {
				return ErrCostActiveRuleConflict
			}
			if profitSnapshot != nil &&
				(activeRules[0].ID != profitSnapshot.RuleID || activeRules[0].Version != profitSnapshot.RuleVersion) {
				return ErrCostRuleSnapshotConflict
			}
			if err := validateRule(&activeRules[0]); err != nil {
				return err
			}
		}

		var persisted CostAccountingRequest
		err = lockForUpdate(tx).Where("request_id = ?", request.RequestID).First(&persisted).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			persisted = *request
			if persisted.RevenueStatus == "" {
				persisted.RevenueStatus = string(types.CostRevenuePending)
			}
			if persisted.ProfitStatus == "" {
				persisted.ProfitStatus = string(types.CostProfitIncompleteRevenue)
			}
			if persisted.RequestedAt == 0 {
				persisted.RequestedAt = now
			}
			if persisted.CreatedAt == 0 {
				persisted.CreatedAt = now
			}
			persisted.UpdatedAt = now
			result := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "request_id"}},
				DoNothing: true,
			}).Create(&persisted)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				if err := lockForUpdate(tx).Where("request_id = ?", request.RequestID).First(&persisted).Error; err != nil {
					return err
				}
			}
		} else if err != nil {
			return err
		}
		if request.TaskID != nil && !sameCostTaskID(request.TaskID, persisted.TaskID) {
			return errors.New("cost request task ID does not match persisted request")
		}
		*request = persisted

		if attempt.CostRequestID != 0 && attempt.CostRequestID != persisted.ID {
			return errors.New("attempt belongs to another cost request")
		}
		attempt.CostRequestID = persisted.ID
		if attempt.AttemptNo <= 0 {
			var latestAttemptNo int
			if err := tx.Model(&CostAccountingAttempt{}).
				Where("cost_request_id = ?", persisted.ID).
				Select("COALESCE(MAX(attempt_no), 0)").
				Scan(&latestAttemptNo).Error; err != nil {
				return err
			}
			attempt.AttemptNo = latestAttemptNo + 1
		}
		// Persist the same normalized key used by the locked active-rule query.
		attempt.CostVariantKey = selectedVariant
		attempt.Status = string(types.CostAttemptPrepared)
		attempt.BillableRequestCount = 1
		if attempt.ReconciliationStatus == "" {
			attempt.ReconciliationStatus = CostReconciliationNone
		}
		if attempt.PreparedAt == 0 {
			attempt.PreparedAt = now
		}
		if attempt.CreatedAt == 0 {
			attempt.CreatedAt = now
		}
		attempt.UpdatedAt = now
		if err := tx.Create(attempt).Error; err != nil {
			return err
		}
		return recomputeCostAccountingRequest(tx, persisted.ID, now)
	})
}

func sameCostProfitMarginThreshold(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func TransitionCostAttempt(id int64, from, to types.CostAttemptStatus, updates map[string]any) error {
	return transitionCostAttempt(DB, id, from, to, updates)
}

func TransitionCostAttemptWithContext(ctx context.Context, id int64, from, to types.CostAttemptStatus, updates map[string]any) error {
	if ctx == nil {
		return errors.New("cost transition context is required")
	}
	return transitionCostAttempt(DB.WithContext(ctx), id, from, to, updates)
}

func RecordPendingCostMeterWithContext(ctx context.Context, attemptID int64, meterJSON string, failureCode string) error {
	if ctx == nil {
		return errors.New("cost meter context is required")
	}
	meterJSON = strings.TrimSpace(meterJSON)
	failureCode = strings.TrimSpace(failureCode)
	if attemptID <= 0 || meterJSON == "" {
		return errors.New("pending cost meter is required")
	}
	if failureCode == "" || len(failureCode) > 64 {
		return errors.New("pending cost meter failure code is invalid")
	}

	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var attempt CostAccountingAttempt
		if err := lockForUpdate(tx).Where("id = ?", attemptID).First(&attempt).Error; err != nil {
			return err
		}
		if types.CostAttemptStatus(attempt.Status) != types.CostAttemptAwaitingMeter {
			if (types.CostAttemptStatus(attempt.Status) == types.CostAttemptSettled ||
				types.CostAttemptStatus(attempt.Status) == types.CostAttemptConfirmedZero) &&
				attempt.ActualMeterJSON == meterJSON {
				return nil
			}
			return ErrCostStateConflict
		}
		if attempt.ActualMeterJSON != "" {
			if attempt.ActualMeterJSON == meterJSON && attempt.FailureCode == failureCode {
				return nil
			}
			return ErrCostStateConflict
		}

		result := tx.Model(&CostAccountingAttempt{}).
			Where("id = ? AND status = ? AND actual_meter_json = ?", attempt.ID, types.CostAttemptAwaitingMeter, "").
			Updates(map[string]any{
				"actual_meter_json": meterJSON,
				"failure_code":      failureCode,
				"updated_at":        common.GetTimestamp(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCostStateConflict
		}
		return nil
	})
}

func transitionCostAttempt(db *gorm.DB, id int64, from, to types.CostAttemptStatus, updates map[string]any) error {
	if !costAttemptTransitionAllowed(from, to) {
		return ErrCostStateConflict
	}
	allowedUpdates := map[string]struct{}{
		"upstream_accepted": {}, "http_status": {}, "result_code": {}, "failure_code": {},
		"actual_meter_json": {}, "dispatching_at": {}, "accepted_at": {}, "terminal_at": {},
		"settled_at": {}, "reconciliation_status": {},
	}
	casUpdates := make(map[string]any, len(updates)+2)
	for key, value := range updates {
		if _, ok := allowedUpdates[key]; !ok {
			return fmt.Errorf("unsupported cost attempt update %q", key)
		}
		casUpdates[key] = value
	}
	now := common.GetTimestamp()
	casUpdates["status"] = string(to)
	casUpdates["updated_at"] = now
	if to == types.CostAttemptDispatching {
		if _, ok := casUpdates["dispatching_at"]; !ok {
			casUpdates["dispatching_at"] = now
		}
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var attempt CostAccountingAttempt
		if err := tx.Select("id", "cost_request_id").Where("id = ?", id).First(&attempt).Error; err != nil {
			return err
		}
		result := tx.Model(&CostAccountingAttempt{}).
			Where("id = ? AND status = ?", id, from).
			Updates(casUpdates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCostStateConflict
		}
		return recomputeCostAccountingRequest(tx, attempt.CostRequestID, now)
	})
}

func SettleCostAttempt(input SettleCostAttemptInput) error {
	return settleCostAttempt(DB, input)
}

func SettleCostAttemptWithContext(ctx context.Context, input SettleCostAttemptInput) error {
	if ctx == nil {
		return errors.New("cost settlement context is required")
	}
	return settleCostAttempt(DB.WithContext(ctx), input)
}

func settleCostAttempt(db *gorm.DB, input SettleCostAttemptInput) error {
	if input.To != types.CostAttemptSettled && input.To != types.CostAttemptConfirmedZero {
		return ErrCostStateConflict
	}
	if !costAttemptTransitionAllowed(input.From, input.To) {
		return ErrCostStateConflict
	}
	if input.To == types.CostAttemptSettled {
		if input.CostNanoUSD == nil || *input.CostNanoUSD < 0 {
			return errors.New("settled cost must be non-negative")
		}
		amount, err := decimal.NewFromString(input.OriginalCost)
		if err != nil || amount.IsNegative() {
			return errors.New("original cost must be a non-negative decimal")
		}
	} else {
		zero := int64(0)
		input.CostNanoUSD = &zero
		input.OriginalCost = "0"
	}
	if input.SettledAt == 0 {
		input.SettledAt = common.GetTimestamp()
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var attempt CostAccountingAttempt
		if err := tx.Select("id", "cost_request_id").Where("id = ?", input.AttemptID).First(&attempt).Error; err != nil {
			return err
		}
		result := tx.Model(&CostAccountingAttempt{}).
			Where("id = ? AND status = ?", input.AttemptID, input.From).
			Updates(map[string]any{
				"status":            string(input.To),
				"actual_meter_json": input.ActualMeterJSON,
				"original_cost":     input.OriginalCost,
				"cost_nano_usd":     input.CostNanoUSD,
				"upstream_accepted": input.UpstreamAccepted,
				"http_status":       input.HTTPStatus,
				"result_code":       input.ResultCode,
				"failure_code":      input.FailureCode,
				"accepted_at":       input.AcceptedAt,
				"terminal_at":       input.TerminalAt,
				"settled_at":        input.SettledAt,
				"updated_at":        input.SettledAt,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCostStateConflict
		}
		return recomputeCostAccountingRequest(tx, attempt.CostRequestID, input.SettledAt)
	})
}

func RecognizeCostRevenue(input RecognizeCostRevenueInput) error {
	return recognizeCostRevenue(DB, input)
}

func RecognizeCostRevenueWithContext(ctx context.Context, input RecognizeCostRevenueInput) error {
	if ctx == nil {
		return errors.New("cost revenue context is required")
	}
	return recognizeCostRevenue(DB.WithContext(ctx), input)
}

func recognizeCostRevenue(db *gorm.DB, input RecognizeCostRevenueInput) error {
	if !costRevenueTransitionAllowed(input.From, input.To) {
		return ErrCostStateConflict
	}
	if input.SettledAt == 0 {
		input.SettledAt = common.GetTimestamp()
	}
	updates := map[string]any{
		"revenue_status": string(input.To),
		"failure_code":   input.FailureCode,
		"updated_at":     input.SettledAt,
	}
	if input.To == types.CostRevenueFailed {
		updates["revenue_settled_at"] = nil
	} else {
		if input.To == types.CostRevenueConfirmedZero {
			zero := int64(0)
			input.RevenueNanoUSD = &zero
			if input.FinalUserQuota == nil {
				input.FinalUserQuota = &zero
			}
		}
		if input.FinalUserQuota == nil || *input.FinalUserQuota < 0 || input.RevenueNanoUSD == nil || *input.RevenueNanoUSD < 0 {
			return errors.New("confirmed revenue values must be non-negative")
		}
		if strings.TrimSpace(input.QuotaPerUnitSnapshot) == "" {
			return errors.New("quota-per-unit snapshot is required")
		}
		updates["final_user_quota"] = input.FinalUserQuota
		updates["quota_per_unit_snapshot"] = input.QuotaPerUnitSnapshot
		updates["billed_revenue_equivalent_nano_usd"] = input.RevenueNanoUSD
		updates["revenue_settled_at"] = input.SettledAt
		updates["updated_at"] = input.SettledAt
	}

	return db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&CostAccountingRequest{}).
			Where("id = ? AND revenue_status = ?", input.CostRequestID, input.From).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCostStateConflict
		}
		return recomputeCostAccountingRequest(tx, input.CostRequestID, input.SettledAt)
	})
}

func ReconcileCostAttempt(input ReconcileCostAttemptInput) error {
	return reconcileCostAttempt(DB, input)
}

func ReconcileCostAttemptWithContext(ctx context.Context, input ReconcileCostAttemptInput) error {
	if ctx == nil {
		return errors.New("cost reconciliation context is required")
	}
	return reconcileCostAttempt(DB.WithContext(ctx), input)
}

func reconcileCostAttempt(db *gorm.DB, input ReconcileCostAttemptInput) error {
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Reason == "" {
		return errors.New("reconciliation reason is required")
	}
	if input.To != types.CostAttemptSettled && input.To != types.CostAttemptConfirmedZero {
		return ErrCostStateConflict
	}
	if input.To == types.CostAttemptSettled {
		if input.CostNanoUSD == nil || *input.CostNanoUSD < 0 {
			return errors.New("reconciled cost must be non-negative")
		}
		amount, err := decimal.NewFromString(input.OriginalCost)
		if err != nil || amount.IsNegative() {
			return errors.New("reconciled original cost must be a non-negative decimal")
		}
	} else {
		zero := int64(0)
		input.CostNanoUSD = &zero
		input.OriginalCost = "0"
	}
	if input.CreatedAt == 0 {
		input.CreatedAt = common.GetTimestamp()
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var attempt CostAccountingAttempt
		if err := lockForUpdate(tx).Where("id = ?", input.AttemptID).First(&attempt).Error; err != nil {
			return err
		}
		from := types.CostAttemptStatus(attempt.Status)
		if from != types.CostAttemptSettlementFailed && from != types.CostAttemptUnknown {
			return ErrCostStateConflict
		}
		var request CostAccountingRequest
		if err := lockForUpdate(tx).Where("id = ?", attempt.CostRequestID).First(&request).Error; err != nil {
			return err
		}

		result := tx.Model(&CostAccountingAttempt{}).
			Where("id = ? AND status = ?", attempt.ID, attempt.Status).
			Updates(map[string]any{
				"status":                string(input.To),
				"actual_meter_json":     input.MeterJSON,
				"original_cost":         input.OriginalCost,
				"cost_nano_usd":         input.CostNanoUSD,
				"failure_code":          "",
				"reconciliation_status": CostReconciliationReconciled,
				"settled_at":            input.CreatedAt,
				"updated_at":            input.CreatedAt,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCostStateConflict
		}

		auditAttemptID := attempt.ID
		audit := CostAccountingAudit{
			CostRequestID:    attempt.CostRequestID,
			CostAttemptID:    &auditAttemptID,
			AdminID:          input.AdminID,
			OldState:         attempt.Status,
			NewState:         string(input.To),
			MeterJSON:        input.MeterJSON,
			RuleID:           attempt.RuleID,
			RuleVersion:      attempt.RuleVersion,
			OldAmountNanoUSD: attempt.CostNanoUSD,
			NewAmountNanoUSD: input.CostNanoUSD,
			Reason:           input.Reason,
			CreatedAt:        input.CreatedAt,
		}
		if err := tx.Create(&audit).Error; err != nil {
			return err
		}
		return recomputeCostAccountingRequest(tx, request.ID, input.CreatedAt)
	})
}

func ReconcileCostRevenueWithContext(ctx context.Context, input ReconcileCostRevenueInput) error {
	if ctx == nil {
		return errors.New("cost reconciliation context is required")
	}
	return reconcileCostRevenue(DB.WithContext(ctx), input)
}

func reconcileCostRevenue(db *gorm.DB, input ReconcileCostRevenueInput) error {
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Reason == "" {
		return errors.New("reconciliation reason is required")
	}
	if input.To != types.CostRevenueSettled && input.To != types.CostRevenueConfirmedZero {
		return ErrCostStateConflict
	}
	if input.FinalUserQuota < 0 || input.RevenueNanoUSD < 0 {
		return errors.New("reconciled revenue values must be non-negative")
	}
	input.QuotaPerUnitSnapshot = strings.TrimSpace(input.QuotaPerUnitSnapshot)
	if input.QuotaPerUnitSnapshot == "" {
		return errors.New("quota-per-unit snapshot is required")
	}
	if input.To == types.CostRevenueConfirmedZero && (input.FinalUserQuota != 0 || input.RevenueNanoUSD != 0) {
		return errors.New("confirmed-zero revenue values must be zero")
	}
	if input.CreatedAt == 0 {
		input.CreatedAt = common.GetTimestamp()
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var request CostAccountingRequest
		if err := lockForUpdate(tx).Where("id = ?", input.CostRequestID).First(&request).Error; err != nil {
			return err
		}
		if types.CostRevenueStatus(request.RevenueStatus) != types.CostRevenueFailed ||
			request.QuotaPerUnitSnapshot != input.QuotaPerUnitSnapshot {
			return ErrCostStateConflict
		}

		result := tx.Model(&CostAccountingRequest{}).
			Where("id = ? AND revenue_status = ?", request.ID, types.CostRevenueFailed).
			Updates(map[string]any{
				"revenue_status":                     string(input.To),
				"final_user_quota":                   input.FinalUserQuota,
				"quota_per_unit_snapshot":            input.QuotaPerUnitSnapshot,
				"billed_revenue_equivalent_nano_usd": input.RevenueNanoUSD,
				"failure_code":                       "",
				"revenue_settled_at":                 input.CreatedAt,
				"updated_at":                         input.CreatedAt,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCostStateConflict
		}

		newAmount := input.RevenueNanoUSD
		audit := CostAccountingAudit{
			CostRequestID:    request.ID,
			AdminID:          input.AdminID,
			OldState:         request.RevenueStatus,
			NewState:         string(input.To),
			MeterJSON:        input.MeterJSON,
			OldAmountNanoUSD: request.BilledRevenueEquivalentNanoUSD,
			NewAmountNanoUSD: &newAmount,
			Reason:           input.Reason,
			CreatedAt:        input.CreatedAt,
		}
		if err := tx.Create(&audit).Error; err != nil {
			return err
		}
		return recomputeCostAccountingRequest(tx, request.ID, input.CreatedAt)
	})
}

func CreateCostAccountingAudit(audit *CostAccountingAudit) error {
	if audit == nil {
		return errors.New("cost accounting audit is required")
	}
	if audit.CreatedAt == 0 {
		audit.CreatedAt = common.GetTimestamp()
	}
	return DB.Create(audit).Error
}

func ListCostAccountingAudits(costRequestID int64) ([]CostAccountingAudit, error) {
	var audits []CostAccountingAudit
	err := DB.Where("cost_request_id = ?", costRequestID).Order("created_at ASC, id ASC").Find(&audits).Error
	return audits, err
}

func recomputeCostAccountingRequest(tx *gorm.DB, requestID int64, now int64) error {
	var request CostAccountingRequest
	if err := lockForUpdate(tx).Where("id = ?", requestID).First(&request).Error; err != nil {
		return err
	}
	var attempts []CostAccountingAttempt
	if err := tx.Where("cost_request_id = ?", requestID).Find(&attempts).Error; err != nil {
		return err
	}

	confirmedCost := int64(0)
	costComplete := true
	for _, attempt := range attempts {
		switch types.CostAttemptStatus(attempt.Status) {
		case types.CostAttemptSettled:
			if attempt.CostNanoUSD == nil || *attempt.CostNanoUSD < 0 {
				return errors.New("settled attempt has invalid cost")
			}
			var err error
			confirmedCost, err = checkedCostNanoAdd(confirmedCost, *attempt.CostNanoUSD)
			if err != nil {
				return err
			}
		case types.CostAttemptConfirmedZero, types.CostAttemptNotDispatched:
		default:
			costComplete = false
		}
	}

	updates := map[string]any{
		"confirmed_cost_nano_usd": confirmedCost,
		"attempt_count":           len(attempts),
		"updated_at":              now,
	}
	revenueComplete := request.RevenueStatus == string(types.CostRevenueSettled) || request.RevenueStatus == string(types.CostRevenueConfirmedZero)
	if !revenueComplete {
		updates["profit_status"] = string(types.CostProfitIncompleteRevenue)
		updates["billed_gross_profit_nano_usd"] = nil
		updates["gross_margin_ppm"] = nil
	} else if !costComplete {
		updates["profit_status"] = string(types.CostProfitIncompleteCost)
		updates["billed_gross_profit_nano_usd"] = nil
		updates["gross_margin_ppm"] = nil
	} else {
		if request.BilledRevenueEquivalentNanoUSD == nil || *request.BilledRevenueEquivalentNanoUSD < 0 {
			return errors.New("confirmed request has invalid revenue")
		}
		profit, err := checkedCostNanoSubtract(*request.BilledRevenueEquivalentNanoUSD, confirmedCost)
		if err != nil {
			return err
		}
		margin, err := costGrossMarginPPM(profit, *request.BilledRevenueEquivalentNanoUSD)
		if err != nil {
			return err
		}
		updates["profit_status"] = string(types.CostProfitComplete)
		updates["billed_gross_profit_nano_usd"] = profit
		updates["gross_margin_ppm"] = margin
		if request.ProfitRecognizedAt == nil {
			updates["profit_recognized_at"] = now
		}
	}

	result := tx.Model(&CostAccountingRequest{}).Where("id = ?", requestID).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	// The request row is locked above. MySQL reports zero affected rows for a
	// successful no-op update, so RowsAffected is not a second CAS here.
	return nil
}

func costAttemptTransitionAllowed(from, to types.CostAttemptStatus) bool {
	switch from {
	case types.CostAttemptPrepared:
		return to == types.CostAttemptDispatching || to == types.CostAttemptNotDispatched
	case types.CostAttemptDispatching:
		return to == types.CostAttemptAwaitingMeter || to == types.CostAttemptSettled || to == types.CostAttemptConfirmedZero || to == types.CostAttemptUnknown
	case types.CostAttemptAwaitingMeter:
		return to == types.CostAttemptSettled || to == types.CostAttemptConfirmedZero || to == types.CostAttemptSettlementFailed || to == types.CostAttemptUnknown
	case types.CostAttemptSettlementFailed, types.CostAttemptUnknown:
		return to == types.CostAttemptSettled || to == types.CostAttemptConfirmedZero
	default:
		return false
	}
}

func costRevenueTransitionAllowed(from, to types.CostRevenueStatus) bool {
	if from == types.CostRevenuePending {
		return to == types.CostRevenueSettled || to == types.CostRevenueConfirmedZero || to == types.CostRevenueFailed
	}
	if from == types.CostRevenueFailed {
		return to == types.CostRevenueSettled || to == types.CostRevenueConfirmedZero
	}
	return false
}

func sameCostTaskID(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func checkedCostNanoAdd(left, right int64) (int64, error) {
	if (right > 0 && left > math.MaxInt64-right) || (right < 0 && left < math.MinInt64-right) {
		return 0, ErrCostAmountOverflow
	}
	return left + right, nil
}

func checkedCostNanoSubtract(left, right int64) (int64, error) {
	if (right > 0 && left < math.MinInt64+right) || (right < 0 && left > math.MaxInt64+right) {
		return 0, ErrCostAmountOverflow
	}
	return left - right, nil
}

func costGrossMarginPPM(profitNanoUSD, revenueNanoUSD int64) (*int64, error) {
	if revenueNanoUSD == 0 {
		return nil, nil
	}
	margin := decimal.NewFromInt(profitNanoUSD).
		Div(decimal.NewFromInt(revenueNanoUSD)).
		Mul(decimal.NewFromInt(1_000_000)).
		Round(0)
	maxInt64 := decimal.NewFromInt(math.MaxInt64)
	minInt64 := decimal.NewFromInt(math.MinInt64)
	if margin.GreaterThan(maxInt64) || margin.LessThan(minInt64) {
		return nil, ErrCostAmountOverflow
	}
	value := margin.IntPart()
	return &value, nil
}
