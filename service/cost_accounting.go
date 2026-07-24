package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type PrepareCostAttemptInput struct {
	RequestID              string
	TaskID                 *string
	UserID                 int
	TokenID                int
	UserGroup              string
	UsingGroup             string
	OriginModelName        string
	BillingSource          string
	SubscriptionID         int
	SubscriptionPlanID     int
	FinalUserQuota         *int64
	QuotaPerUnitSnapshot   string
	ChannelID              int
	ChannelName            string
	ChannelType            int
	PredictedUpstreamModel string
	BillableUpstreamModel  string
	RequestPath            string
	TaskPlatform           constant.TaskPlatform
	RequestMeter           *types.CostMeter
}

type CostCoverageError struct {
	ChannelID int
}

func (e *CostCoverageError) Error() string {
	return "channel cost coverage unavailable"
}

func PrepareCostAttempt(ctx context.Context, input PrepareCostAttemptInput) (*types.CostAttemptHandle, error) {
	if ctx == nil {
		return nil, errors.New("cost attempt context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	input.RequestID = strings.TrimSpace(input.RequestID)
	input.BillableUpstreamModel = strings.TrimSpace(input.BillableUpstreamModel)
	input.PredictedUpstreamModel = strings.TrimSpace(input.PredictedUpstreamModel)
	if input.RequestID == "" || len(input.RequestID) > 64 {
		return nil, errors.New("cost request ID is invalid")
	}
	if input.ChannelID <= 0 || input.BillableUpstreamModel == "" || len(input.BillableUpstreamModel) > 191 {
		return nil, &CostCoverageError{ChannelID: input.ChannelID}
	}
	if input.FinalUserQuota != nil && *input.FinalUserQuota < 0 {
		return nil, errors.New("final user quota cannot be negative")
	}

	channel, err := model.GetChannelById(input.ChannelID, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, &CostCoverageError{ChannelID: input.ChannelID}
		}
		return nil, err
	}
	capabilities, err := lookupCostCapabilities(channel.Type, input.RequestPath, input.TaskPlatform)
	if err != nil {
		return nil, &CostCoverageError{ChannelID: input.ChannelID}
	}

	requestMeterJSON := "{}"
	if input.RequestMeter != nil {
		if err := validateCostMeterBounds(*input.RequestMeter); err != nil {
			return nil, err
		}
		requestMeter, err := common.Marshal(input.RequestMeter)
		if err != nil {
			return nil, err
		}
		requestMeterJSON = string(requestMeter)
	}
	if input.TaskID != nil {
		taskID := strings.TrimSpace(*input.TaskID)
		if taskID == "" || len(taskID) > 191 {
			return nil, errors.New("cost task ID is invalid")
		}
		input.TaskID = &taskID
	}
	if input.PredictedUpstreamModel == "" {
		input.PredictedUpstreamModel = input.BillableUpstreamModel
	}
	channelName := strings.TrimSpace(input.ChannelName)
	if channelName == "" {
		channelName = channel.Name
	}

	request := &model.CostAccountingRequest{
		RequestID:            input.RequestID,
		TaskID:               input.TaskID,
		UserID:               input.UserID,
		TokenID:              input.TokenID,
		UserGroup:            strings.TrimSpace(input.UserGroup),
		UsingGroup:           strings.TrimSpace(input.UsingGroup),
		OriginModelName:      strings.TrimSpace(input.OriginModelName),
		BillingSource:        strings.TrimSpace(input.BillingSource),
		SubscriptionID:       input.SubscriptionID,
		SubscriptionPlanID:   input.SubscriptionPlanID,
		FinalUserQuota:       input.FinalUserQuota,
		QuotaPerUnitSnapshot: strings.TrimSpace(input.QuotaPerUnitSnapshot),
	}
	attempt := &model.CostAccountingAttempt{
		ChannelID:              input.ChannelID,
		ChannelName:            channelName,
		ChannelType:            channel.Type,
		PredictedUpstreamModel: input.PredictedUpstreamModel,
		BillableUpstreamModel:  input.BillableUpstreamModel,
		BillableRequestCount:   1,
		RequestMeterJSON:       requestMeterJSON,
	}
	err = model.PrepareCostAttemptWithRuleValidation(ctx, request, attempt, func(lockedRule *model.ChannelModelCostRule) error {
		config, err := validateCostRuleContract(lockedRule, capabilities)
		if err != nil {
			return &CostCoverageError{ChannelID: input.ChannelID}
		}
		if input.RequestMeter != nil && config.MeterSource != "" && input.RequestMeter.Source != config.MeterSource {
			return errors.New("request cost meter source does not match the rule snapshot")
		}
		if types.CostMode(lockedRule.CostMode) == types.CostModePerDuration && config.MeterSource == types.CostMeterValidatedRequest {
			if input.RequestMeter == nil {
				return errors.New("validated request duration meter is required")
			}
			if _, _, err := CalculateAttemptCost(types.CostMode(lockedRule.CostMode), config, *input.RequestMeter); err != nil {
				return err
			}
		}
		ruleConfigJSON, err := common.Marshal(config)
		if err != nil {
			return err
		}
		attempt.RuleID = lockedRule.ID
		attempt.RuleVersion = lockedRule.Version
		attempt.CostMode = lockedRule.CostMode
		attempt.SchemaVersion = lockedRule.SchemaVersion
		attempt.RuleConfigJSON = string(ruleConfigJSON)
		attempt.ChargeEvent = string(config.ChargeEvent)
		attempt.MeterSource = string(config.MeterSource)
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, &CostCoverageError{ChannelID: input.ChannelID}
		}
		return nil, err
	}

	return &types.CostAttemptHandle{
		CostRequestID: request.ID,
		AttemptID:     attempt.ID,
		AttemptNo:     attempt.AttemptNo,
		CostMode:      types.CostMode(attempt.CostMode),
		ChargeEvent:   types.CostChargeEvent(attempt.ChargeEvent),
	}, nil
}

func AuthorizeCostDispatch(ctx context.Context, handle *types.CostAttemptHandle) error {
	if err := validateCostAttemptHandle(handle); err != nil {
		return err
	}
	if ctx == nil {
		return errors.New("cost dispatch context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return model.TransitionCostAttemptWithContext(ctx, handle.AttemptID, types.CostAttemptPrepared, types.CostAttemptDispatching, nil)
}

func RecordCostDispatchOutcome(ctx context.Context, handle *types.CostAttemptHandle, outcome types.CostOutcome) error {
	if err := validateCostAttemptHandle(handle); err != nil {
		return err
	}
	persistenceCtx, cancel := costAccountingPersistenceContext(ctx)
	defer cancel()
	attempt, err := loadCostAttemptForHandle(persistenceCtx, handle)
	if err != nil {
		return err
	}
	from := types.CostAttemptStatus(attempt.Status)
	if from != types.CostAttemptDispatching && !(from == types.CostAttemptAwaitingMeter && outcome.Status == types.CostAttemptUnknown) {
		return model.ErrCostStateConflict
	}

	now := common.GetTimestamp()
	acceptedAt := attempt.AcceptedAt
	if outcome.UpstreamAccepted && acceptedAt == nil {
		acceptedAt = &now
	}
	failureCode := strings.TrimSpace(outcome.FailureCode)
	if len(failureCode) > 64 {
		return errors.New("cost outcome failure code is too long")
	}

	switch outcome.Status {
	case types.CostAttemptAwaitingMeter:
		if !outcome.UpstreamAccepted {
			return errors.New("awaiting-meter outcome requires upstream acceptance")
		}
		return model.TransitionCostAttemptWithContext(persistenceCtx, attempt.ID, types.CostAttemptDispatching, types.CostAttemptAwaitingMeter, map[string]any{
			"upstream_accepted": true,
			"accepted_at":       acceptedAt,
			"failure_code":      failureCode,
		})
	case types.CostAttemptSettled:
		return settleCostAttemptFromSnapshot(persistenceCtx, attempt, types.CostAttemptDispatching, types.CostMeter{})
	case types.CostAttemptConfirmedZero:
		return model.SettleCostAttemptWithContext(persistenceCtx, model.SettleCostAttemptInput{
			AttemptID:        attempt.ID,
			From:             types.CostAttemptDispatching,
			To:               types.CostAttemptConfirmedZero,
			ActualMeterJSON:  "{}",
			UpstreamAccepted: outcome.UpstreamAccepted,
			FailureCode:      failureCode,
			AcceptedAt:       acceptedAt,
			TerminalAt:       &now,
			SettledAt:        now,
		})
	case types.CostAttemptUnknown:
		if failureCode == "" {
			failureCode = "upstream_cost_unknown"
		}
		return model.TransitionCostAttemptWithContext(persistenceCtx, attempt.ID, from, types.CostAttemptUnknown, map[string]any{
			"upstream_accepted": outcome.UpstreamAccepted,
			"accepted_at":       acceptedAt,
			"terminal_at":       now,
			"failure_code":      failureCode,
		})
	default:
		return fmt.Errorf("unsupported post-dispatch cost outcome %q", outcome.Status)
	}
}

func SettleSyncCostAttempt(ctx context.Context, handle *types.CostAttemptHandle, meter types.CostMeter) error {
	if err := validateCostAttemptHandle(handle); err != nil {
		return err
	}
	persistenceCtx, cancel := costAccountingPersistenceContext(ctx)
	defer cancel()
	attempt, err := loadCostAttemptForHandle(persistenceCtx, handle)
	if err != nil {
		return err
	}
	from := types.CostAttemptStatus(attempt.Status)
	if from != types.CostAttemptDispatching && from != types.CostAttemptAwaitingMeter {
		return model.ErrCostStateConflict
	}
	if err := settleCostAttemptFromSnapshot(persistenceCtx, attempt, from, meter); err != nil {
		var stateErr error
		if !errors.Is(err, model.ErrCostStateConflict) {
			stateErr = markCostSettlementFailed(persistenceCtx, attempt, meter)
		}
		if stateErr != nil {
			return fmt.Errorf("settle cost attempt: %v; mark settlement failed: %w", err, stateErr)
		}
		return err
	}
	return nil
}

func MarkWinningCostAttempt(ctx context.Context, handle *types.CostAttemptHandle) error {
	if err := validateCostAttemptHandle(handle); err != nil {
		return err
	}
	persistenceCtx, cancel := costAccountingPersistenceContext(ctx)
	defer cancel()
	attempt, err := loadCostAttemptForHandle(persistenceCtx, handle)
	if err != nil {
		return err
	}
	switch types.CostAttemptStatus(attempt.Status) {
	case types.CostAttemptAwaitingMeter, types.CostAttemptSettled, types.CostAttemptConfirmedZero,
		types.CostAttemptUnknown, types.CostAttemptSettlementFailed:
	default:
		return model.ErrCostStateConflict
	}

	var request model.CostAccountingRequest
	db := model.DB.WithContext(persistenceCtx)
	if err := db.Select("id", "winning_attempt_id").Where("id = ?", handle.CostRequestID).First(&request).Error; err != nil {
		return err
	}
	if request.WinningAttemptID != nil {
		if *request.WinningAttemptID == attempt.ID {
			return nil
		}
		return model.ErrCostStateConflict
	}
	result := db.Model(&model.CostAccountingRequest{}).
		Where("id = ? AND winning_attempt_id IS NULL", request.ID).
		Updates(map[string]any{
			"winning_attempt_id": attempt.ID,
			"updated_at":         common.GetTimestamp(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return model.ErrCostStateConflict
	}
	return nil
}

func settleCostAttemptFromSnapshot(ctx context.Context, attempt *model.CostAccountingAttempt, from types.CostAttemptStatus, meter types.CostMeter) error {
	var config types.CostRuleConfigV1
	if err := common.UnmarshalJsonStr(attempt.RuleConfigJSON, &config); err != nil {
		return fmt.Errorf("decode cost rule snapshot: %w", err)
	}
	mode := types.CostMode(attempt.CostMode)
	if config.MeterSource == types.CostMeterValidatedRequest && attempt.RequestMeterJSON != "" && attempt.RequestMeterJSON != "{}" {
		if err := common.UnmarshalJsonStr(attempt.RequestMeterJSON, &meter); err != nil {
			return fmt.Errorf("decode request cost meter snapshot: %w", err)
		}
	}
	if mode == types.CostModePerDuration || mode == types.CostModePerToken {
		if meter.Source != config.MeterSource {
			return errors.New("cost meter source does not match the rule snapshot")
		}
	}
	if err := validateCostMeterBounds(meter); err != nil {
		return err
	}
	originalCost, costNanoUSD, err := CalculateAttemptCost(mode, config, meter)
	if err != nil {
		return err
	}
	if costNanoUSD < 0 {
		return errors.New("calculated cost cannot be negative")
	}
	meterJSON, err := common.Marshal(meter)
	if err != nil {
		return err
	}
	now := common.GetTimestamp()
	acceptedAt := attempt.AcceptedAt
	if acceptedAt == nil {
		acceptedAt = &now
	}
	targetStatus := types.CostAttemptSettled
	if mode == types.CostModeFree {
		targetStatus = types.CostAttemptConfirmedZero
	}
	return model.SettleCostAttemptWithContext(ctx, model.SettleCostAttemptInput{
		AttemptID:        attempt.ID,
		From:             from,
		To:               targetStatus,
		ActualMeterJSON:  string(meterJSON),
		OriginalCost:     originalCost,
		CostNanoUSD:      &costNanoUSD,
		UpstreamAccepted: true,
		AcceptedAt:       acceptedAt,
		TerminalAt:       &now,
		SettledAt:        now,
	})
}

func markCostSettlementFailed(ctx context.Context, attempt *model.CostAccountingAttempt, meter types.CostMeter) error {
	meterJSON, err := common.Marshal(meter)
	if err != nil {
		meterJSON = []byte("{}")
	}
	now := common.GetTimestamp()
	from := types.CostAttemptStatus(attempt.Status)
	if from == types.CostAttemptDispatching {
		if err := model.TransitionCostAttemptWithContext(ctx, attempt.ID, types.CostAttemptDispatching, types.CostAttemptAwaitingMeter, map[string]any{
			"upstream_accepted": true,
			"accepted_at":       now,
		}); err != nil {
			return err
		}
		from = types.CostAttemptAwaitingMeter
	}
	if from != types.CostAttemptAwaitingMeter {
		return model.ErrCostStateConflict
	}
	return model.TransitionCostAttemptWithContext(ctx, attempt.ID, from, types.CostAttemptSettlementFailed, map[string]any{
		"actual_meter_json": string(meterJSON),
		"terminal_at":       now,
		"failure_code":      "cost_meter_invalid",
	})
}

func validateCostAttemptHandle(handle *types.CostAttemptHandle) error {
	if handle == nil || handle.CostRequestID <= 0 || handle.AttemptID <= 0 || handle.AttemptNo <= 0 {
		return errors.New("cost attempt handle is invalid")
	}
	return nil
}

func loadCostAttemptForHandle(ctx context.Context, handle *types.CostAttemptHandle) (*model.CostAccountingAttempt, error) {
	var attempt model.CostAccountingAttempt
	if err := model.DB.WithContext(ctx).Where("id = ? AND cost_request_id = ?", handle.AttemptID, handle.CostRequestID).First(&attempt).Error; err != nil {
		return nil, err
	}
	if attempt.AttemptNo != handle.AttemptNo || types.CostMode(attempt.CostMode) != handle.CostMode {
		return nil, model.ErrCostStateConflict
	}
	return &attempt, nil
}

func costAccountingPersistenceContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	return context.WithTimeout(base, 10*time.Second)
}

func validateCostMeterBounds(meter types.CostMeter) error {
	for name, value := range map[string]*int64{
		"input tokens":      meter.InputTokens,
		"output tokens":     meter.OutputTokens,
		"completion tokens": meter.CompletionTokens,
		"total tokens":      meter.TotalTokens,
	} {
		if value == nil {
			continue
		}
		if *value < 0 || *value > int64(relaycommon.MaxTokensLimit) {
			return fmt.Errorf("%s must be between 0 and %d", name, relaycommon.MaxTokensLimit)
		}
	}
	if meter.DurationSeconds == nil {
		return nil
	}
	duration, err := decimal.NewFromString(strings.TrimSpace(*meter.DurationSeconds))
	if err != nil || duration.IsNegative() || duration.GreaterThan(decimal.NewFromInt(relaycommon.MaxTaskDurationSeconds)) {
		return fmt.Errorf("duration must be between 0 and %d seconds", relaycommon.MaxTaskDurationSeconds)
	}
	return nil
}
