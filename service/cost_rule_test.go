package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func prepareCostRuleServiceDB(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.ChannelModelCostRule{}))
	require.NoError(t, model.DB.Exec("DELETE FROM channel_model_cost_rules").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM channels").Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 7, Type: 1, Name: "supplier", Key: "test-key"}).Error)

	previousLookup := CostCapabilityLookup
	CostCapabilityLookup = func(int, string, constant.TaskPlatform) types.CostCapabilities {
		return completeCostCapabilities()
	}
	InvalidateCostCoverage(0, "", "")
	t.Cleanup(func() {
		CostCapabilityLookup = previousLookup
		InvalidateCostCoverage(0, "", "")
		model.DB.Exec("DELETE FROM channel_model_cost_rules")
		model.DB.Exec("DELETE FROM channels")
	})
}

func TestCreateCostRuleDraftRejectsInvalidConfigurations(t *testing.T) {
	prepareCostRuleServiceDB(t)

	tests := []struct {
		name   string
		mutate func(*CreateCostRuleInput)
	}{
		{name: "missing channel", mutate: func(input *CreateCostRuleInput) { input.ChannelID = 999 }},
		{name: "missing model", mutate: func(input *CreateCostRuleInput) { input.BillableUpstreamModel = "" }},
		{name: "unknown mode", mutate: func(input *CreateCostRuleInput) { input.CostMode = "unknown" }},
		{name: "unknown charge event", mutate: func(input *CreateCostRuleInput) { input.Config.ChargeEvent = "unknown" }},
		{name: "per request with meter source", mutate: func(input *CreateCostRuleInput) { input.Config.MeterSource = types.CostMeterValidatedRequest }},
		{name: "zero price", mutate: func(input *CreateCostRuleInput) { input.Config.UnitPrice = costStringPointer("0") }},
		{name: "negative price", mutate: func(input *CreateCostRuleInput) { input.Config.UnitPrice = costStringPointer("-1") }},
		{name: "invalid multiplier", mutate: func(input *CreateCostRuleInput) { input.Config.BillingMultiplier = "0" }},
		{name: "invalid exchange ratio", mutate: func(input *CreateCostRuleInput) { input.Config.RechargeExchangeRatio = "invalid" }},
		{name: "negative fee", mutate: func(input *CreateCostRuleInput) { input.Config.FeeRate = "-0.1" }},
		{name: "extra price field", mutate: func(input *CreateCostRuleInput) { input.Config.PricePerSecond = costStringPointer("1") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validCreateCostRuleInput()
			tt.mutate(&input)
			_, err := CreateCostRuleDraft(input)
			require.Error(t, err)
		})
	}
}

func TestCreateCostRuleDraftRejectsFreeWithoutReason(t *testing.T) {
	prepareCostRuleServiceDB(t)
	input := validCreateCostRuleInput()
	input.CostMode = types.CostModeFree
	input.Config = types.CostRuleConfigV1{}

	_, err := CreateCostRuleDraft(input)
	require.Error(t, err)
}

func TestCreateCostRuleDraftRejectsCapabilityMismatch(t *testing.T) {
	prepareCostRuleServiceDB(t)
	CostCapabilityLookup = func(int, string, constant.TaskPlatform) types.CostCapabilities {
		return types.CostCapabilities{
			CanResolveBillableModel: true,
			ChargeEvents:            []types.CostChargeEvent{types.CostChargeTaskSucceeded},
		}
	}

	_, err := CreateCostRuleDraft(validCreateCostRuleInput())
	require.Error(t, err)
}

func TestCreateCostRuleDraftValidatesDurationAndTokenContracts(t *testing.T) {
	prepareCostRuleServiceDB(t)

	tests := []struct {
		name   string
		mode   types.CostMode
		config types.CostRuleConfigV1
	}{
		{
			name:   "duration with token meter source",
			mode:   types.CostModePerDuration,
			config: validDurationCostConfig(types.CostMeterUpstreamUsage),
		},
		{
			name:   "token with duration meter source",
			mode:   types.CostModePerToken,
			config: validTokenCostConfig(types.CostTokenModeTotal, types.CostMeterValidatedRequest),
		},
		{
			name:   "unknown token mode",
			mode:   types.CostModePerToken,
			config: validTokenCostConfig("unknown", types.CostMeterUpstreamUsage),
		},
		{
			name: "split token price missing output",
			mode: types.CostModePerToken,
			config: func() types.CostRuleConfigV1 {
				config := validTokenCostConfig(types.CostTokenModeInputOutput, types.CostMeterUpstreamUsage)
				config.OutputPerMillion = nil
				return config
			}(),
		},
		{
			name: "token mode with extra price",
			mode: types.CostModePerToken,
			config: func() types.CostRuleConfigV1 {
				config := validTokenCostConfig(types.CostTokenModeTotal, types.CostMeterUpstreamUsage)
				config.CompletionPerMillion = costStringPointer("1")
				return config
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validCreateCostRuleInput()
			input.CostMode = tt.mode
			input.Config = tt.config
			_, err := CreateCostRuleDraft(input)
			require.Error(t, err)
		})
	}
}

func TestCreateCostRuleDraftValidatesSubmitAcceptedMeterAvailability(t *testing.T) {
	prepareCostRuleServiceDB(t)

	tests := []struct {
		name    string
		mode    types.CostMode
		config  types.CostRuleConfigV1
		wantErr bool
	}{
		{
			name: "duration upstream actual is unavailable at submit",
			mode: types.CostModePerDuration,
			config: func() types.CostRuleConfigV1 {
				config := validDurationCostConfig(types.CostMeterUpstreamActual)
				config.ChargeEvent = types.CostChargeSubmitAccepted
				return config
			}(),
			wantErr: true,
		},
		{
			name: "token usage is unavailable at submit",
			mode: types.CostModePerToken,
			config: func() types.CostRuleConfigV1 {
				config := validTokenCostConfig(types.CostTokenModeTotal, types.CostMeterUpstreamUsage)
				config.ChargeEvent = types.CostChargeSubmitAccepted
				return config
			}(),
			wantErr: true,
		},
		{
			name: "validated duration is available at submit",
			mode: types.CostModePerDuration,
			config: func() types.CostRuleConfigV1 {
				config := validDurationCostConfig(types.CostMeterValidatedRequest)
				config.ChargeEvent = types.CostChargeSubmitAccepted
				return config
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validCreateCostRuleInput()
			input.CostMode = tt.mode
			input.Config = tt.config
			rule, err := CreateCostRuleDraft(input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, string(types.CostRuleDraft), rule.Status)
		})
	}
}

func TestValidateCostRuleDraftRejectsInconsistentNormalizedPrice(t *testing.T) {
	prepareCostRuleServiceDB(t)
	config := validPerRequestCostConfig()
	normalized, err := NormalizeCostRuleConfig(types.CostModePerRequest, config)
	require.NoError(t, err)
	normalized.NormalizedUSDPrices.UnitPrice = costStringPointer("999")
	rule := costRuleWithConfig(t, types.CostModePerRequest, normalized)

	_, err = ValidateCostRuleDraft(&rule, completeCostCapabilities())
	require.Error(t, err)
}

func TestCostRuleLifecycleVersionsAndPreservesHistory(t *testing.T) {
	prepareCostRuleServiceDB(t)

	first, err := CreateCostRuleDraft(validCreateCostRuleInput())
	require.NoError(t, err)
	assert.Equal(t, 1, first.Version)
	firstConfig := first.ConfigJSON
	first, err = ActivateCostRule(first.ID, 41)
	require.NoError(t, err)
	cachedFirst, err := ActiveCostRule(7, "vendor-model", "default", false)
	require.NoError(t, err)
	assert.Equal(t, first.ID, cachedFirst.ID)

	secondInput := validCreateCostRuleInput()
	secondInput.Config.UnitPrice = costStringPointer("0.3")
	second, err := CreateCostRuleDraft(secondInput)
	require.NoError(t, err)
	assert.Equal(t, 2, second.Version)
	second, err = ActivateCostRule(second.ID, 42)
	require.NoError(t, err)

	require.NoError(t, model.DB.First(first, first.ID).Error)
	assert.Equal(t, string(types.CostRuleRetired), first.Status)
	require.NotNil(t, first.EffectiveTo)
	require.NotNil(t, second.EffectiveFrom)
	assert.Equal(t, *second.EffectiveFrom, *first.EffectiveTo)
	assert.Equal(t, firstConfig, first.ConfigJSON)
	activeSecond, err := ActiveCostRule(7, "vendor-model", "default", false)
	require.NoError(t, err)
	assert.Equal(t, second.ID, activeSecond.ID)

	_, err = UpdateCostRuleDraft(first.ID, UpdateCostRuleInput{
		CostMode: types.CostModePerRequest,
		Config:   validPerRequestCostConfig(),
	})
	assert.ErrorIs(t, err, model.ErrCostRuleStateConflict)

	require.NoError(t, RetireCostRule(second.ID, 43))
	_, err = ActiveCostRule(7, "vendor-model", "default", false)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestActivateCostRuleRejectsDuplicateActiveRows(t *testing.T) {
	prepareCostRuleServiceDB(t)

	for version := 1; version <= 2; version++ {
		rule := costRuleWithConfig(t, types.CostModePerRequest, normalizedPerRequestConfig(t, "0.2"))
		rule.Version = version
		rule.Status = string(types.CostRuleActive)
		rule.CostVariantKey = string(types.DefaultCostVariantKey)
		require.NoError(t, model.DB.Create(&rule).Error)
	}
	draft := costRuleWithConfig(t, types.CostModePerRequest, normalizedPerRequestConfig(t, "0.3"))
	draft.Version = 3
	require.NoError(t, model.CreateCostRuleDraft(&draft))

	_, err := ActivateCostRule(draft.ID, 42)
	assert.ErrorIs(t, err, model.ErrCostActiveRuleConflict)
}

func TestActiveCostRuleCacheInvalidationAndAuthoritativeFallback(t *testing.T) {
	prepareCostRuleServiceDB(t)

	_, err := ActiveCostRule(7, "vendor-model", "default", false)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

	rule := costRuleWithConfig(t, types.CostModePerRequest, normalizedPerRequestConfig(t, "0.2"))
	rule.Version = 1
	rule.Status = string(types.CostRuleActive)
	require.NoError(t, model.DB.Create(&rule).Error)

	cached, err := ActiveCostRule(7, "vendor-model", "default", false)
	require.NoError(t, err)
	assert.Equal(t, rule.ID, cached.ID)
	require.NoError(t, model.DB.Model(&model.ChannelModelCostRule{}).Where("id = ?", rule.ID).Update("status", types.CostRuleRetired).Error)

	cached, err = ActiveCostRule(7, "vendor-model", "default", false)
	require.NoError(t, err)
	assert.Equal(t, rule.ID, cached.ID)

	InvalidateCostCoverage(7, "vendor-model", "default")
	_, err = ActiveCostRule(7, "vendor-model", "default", false)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestCheckPredictedCostCoverageUsesPathCapabilityContract(t *testing.T) {
	prepareCostRuleServiceDB(t)
	rule := costRuleWithConfig(t, types.CostModePerRequest, normalizedPerRequestConfig(t, "0.2"))
	rule.Version = 1
	rule.Status = string(types.CostRuleActive)
	require.NoError(t, model.DB.Create(&rule).Error)

	CostCapabilityLookup = func(channelType int, requestPath string, platform constant.TaskPlatform) types.CostCapabilities {
		assert.Equal(t, 1, channelType)
		assert.Equal(t, "/v1/chat/completions", requestPath)
		assert.Empty(t, platform)
		return completeCostCapabilities()
	}

	covered, err := CheckPredictedCostCoverage(PredictedCoverageInput{
		ChannelID: 7, PredictedUpstreamModel: "vendor-model", RequestPath: "/v1/chat/completions",
	})
	require.NoError(t, err)
	assert.True(t, covered)
}

func TestCheckPredictedCostCoverageRejectsInconsistentPathContracts(t *testing.T) {
	prepareCostRuleServiceDB(t)
	rule := costRuleWithConfig(t, types.CostModePerRequest, normalizedPerRequestConfig(t, "0.2"))
	rule.Version = 1
	rule.Status = string(types.CostRuleActive)
	require.NoError(t, model.DB.Create(&rule).Error)

	CostCapabilityLookup = func(_ int, requestPath string, _ constant.TaskPlatform) types.CostCapabilities {
		if requestPath == "/v1/responses" {
			return types.CostCapabilities{CanResolveBillableModel: true}
		}
		return completeCostCapabilities()
	}

	covered, err := CheckPredictedCostCoverage(PredictedCoverageInput{
		ChannelID:              7,
		PredictedUpstreamModel: "vendor-model",
		ContractTargets: []CostContractTarget{
			{RequestPath: "/v1/chat/completions"},
			{RequestPath: "/v1/responses"},
		},
	})
	require.NoError(t, err)
	assert.False(t, covered)
}

func TestCheckAuthoritativeCostCoverageIgnoresAbilitiesForDeletedChannels(t *testing.T) {
	prepareCostRuleServiceDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Ability{}))
	require.NoError(t, model.DB.Exec("DELETE FROM abilities").Error)
	t.Cleanup(func() {
		require.NoError(t, model.DB.Exec("DELETE FROM abilities").Error)
	})
	require.NoError(t, model.DB.Create(&[]model.Ability{
		{Group: "default", Model: "valid-model", ChannelId: 7, Enabled: true},
		{Group: "default", Model: "orphan-model", ChannelId: 999, Enabled: true},
	}).Error)

	results, err := CheckAuthoritativeCostCoverage()

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 7, results[0].ChannelID)
	assert.Equal(t, "valid-model", results[0].OriginModel)
}

func TestTaskOnlyChannelCostRuleLifecycleUsesTaskAdaptorCapabilities(t *testing.T) {
	prepareCostRuleServiceDB(t)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 7).Update("type", 59).Error)
	CostCapabilityLookup = func(_ int, _ string, platform constant.TaskPlatform) types.CostCapabilities {
		if platform != "59" {
			return types.CostCapabilities{}
		}
		return types.CostCapabilities{
			CanResolveBillableModel: true,
			ChargeEvents:            []types.CostChargeEvent{types.CostChargeTaskSucceeded},
		}
	}
	input := validCreateCostRuleInput()
	input.Config.ChargeEvent = types.CostChargeTaskSucceeded

	rule, err := CreateCostRuleDraft(input)
	require.NoError(t, err)
	_, err = ValidateCostRuleByID(rule.ID)
	require.NoError(t, err)
	_, err = ActivateCostRule(rule.ID, 42)
	require.NoError(t, err)
	covered, err := CheckPredictedCostCoverage(PredictedCoverageInput{
		ChannelID:              7,
		PredictedUpstreamModel: "vendor-model",
		Authoritative:          true,
	})
	require.NoError(t, err)
	assert.True(t, covered)
}

func validCreateCostRuleInput() CreateCostRuleInput {
	return CreateCostRuleInput{
		ChannelID:             7,
		BillableUpstreamModel: "vendor-model",
		CostMode:              types.CostModePerRequest,
		Config:                validPerRequestCostConfig(),
		Note:                  "supplier price sheet",
		AdminID:               11,
	}
}

func validPerRequestCostConfig() types.CostRuleConfigV1 {
	return types.CostRuleConfigV1{
		Currency:              "USD",
		BillingMultiplier:     "1",
		PurchaseDiscountRatio: "1",
		RechargeExchangeRatio: "1",
		FeeRate:               "0",
		CurrencyToUSDRate:     "1",
		UnitPrice:             costStringPointer("0.2"),
		ChargeEvent:           types.CostChargeResponseSucceeded,
	}
}

func validDurationCostConfig(source types.CostMeterSource) types.CostRuleConfigV1 {
	config := validPerRequestCostConfig()
	config.UnitPrice = nil
	config.PricePerSecond = costStringPointer("0.1")
	config.MeterSource = source
	return config
}

func validTokenCostConfig(mode types.CostTokenMode, source types.CostMeterSource) types.CostRuleConfigV1 {
	config := validPerRequestCostConfig()
	config.UnitPrice = nil
	config.TokenMode = mode
	config.MeterSource = source
	switch mode {
	case types.CostTokenModeTotal:
		config.TotalPerMillion = costStringPointer("1")
	case types.CostTokenModeCompletion:
		config.CompletionPerMillion = costStringPointer("1")
	case types.CostTokenModeInputOutput:
		config.InputPerMillion = costStringPointer("1")
		config.OutputPerMillion = costStringPointer("1")
	}
	return config
}

func normalizedPerRequestConfig(t *testing.T, unitPrice string) types.CostRuleConfigV1 {
	t.Helper()
	config := validPerRequestCostConfig()
	config.UnitPrice = &unitPrice
	normalized, err := NormalizeCostRuleConfig(types.CostModePerRequest, config)
	require.NoError(t, err)
	return normalized
}

func costRuleWithConfig(t *testing.T, mode types.CostMode, config types.CostRuleConfigV1) model.ChannelModelCostRule {
	t.Helper()
	data, err := common.Marshal(config)
	require.NoError(t, err)
	return model.ChannelModelCostRule{
		ChannelID:             7,
		BillableUpstreamModel: "vendor-model",
		CostVariantKey:        string(types.DefaultCostVariantKey),
		Status:                string(types.CostRuleDraft),
		CostMode:              string(mode),
		SchemaVersion:         1,
		ConfigJSON:            string(data),
		Source:                "manual",
		CreatedBy:             11,
	}
}

func completeCostCapabilities() types.CostCapabilities {
	return types.CostCapabilities{
		CanResolveBillableModel: true,
		ChargeEvents: []types.CostChargeEvent{
			types.CostChargeResponseSucceeded,
			types.CostChargeSubmitAccepted,
			types.CostChargeTaskSucceeded,
		},
		MeterSources: []types.CostMeterSource{
			types.CostMeterValidatedRequest,
			types.CostMeterUpstreamActual,
			types.CostMeterUpstreamUsage,
			types.CostMeterLocalUsage,
		},
	}
}

func costStringPointer(value string) *string {
	return &value
}
