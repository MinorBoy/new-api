package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedanceTokenBilling720p() *types.SeedanceTokenBillingBreakdown {
	return &types.SeedanceTokenBillingBreakdown{
		Scenario:   types.SeedanceTokenScenarioNoVideo,
		Resolution: "720p",
		Width:      1280,
		Height:     720,
		FrameRate:  24,
	}
}

func TestCalculateSeedanceTaskUsage(t *testing.T) {
	tests := []struct {
		name      string
		context   model.TaskBillingContext
		facts     SeedanceTerminalFacts
		want      SeedanceTaskUsage
		wantError string
	}{
		{
			name: "request snapshot",
			context: model.TaskBillingContext{
				UsageProfile:             model.TaskUsageProfileSeedance,
				RequestedDurationSeconds: 5,
				Resolution:               "720p",
				SeedanceTokenBilling:     seedanceTokenBilling720p(),
			},
			want: SeedanceTaskUsage{InputTokens: 0, CompletionTokens: 108000, TotalTokens: 108000},
		},
		{
			name: "reference video",
			context: model.TaskBillingContext{
				UsageProfile:             model.TaskUsageProfileSeedance,
				RequestedDurationSeconds: 5,
				Resolution:               "720p",
				HasVideoInput:            true,
				InputVideoDurationMS:     3000,
				SeedanceTokenBilling:     seedanceTokenBilling720p(),
			},
			want: SeedanceTaskUsage{InputTokens: 0, CompletionTokens: 172800, TotalTokens: 172800},
		},
		{
			name: "terminal facts do not replace frozen pricing facts",
			context: model.TaskBillingContext{
				UsageProfile:             model.TaskUsageProfileSeedance,
				RequestedDurationSeconds: 5,
				Resolution:               "720p",
				SeedanceTokenBilling:     seedanceTokenBilling720p(),
			},
			facts: SeedanceTerminalFacts{
				DurationSeconds:        6,
				DurationPresent:        true,
				Resolution:             "720p",
				ResolutionPresent:      true,
				FramesPerSecond:        30,
				FramesPerSecondPresent: true,
			},
			want: SeedanceTaskUsage{InputTokens: 0, CompletionTokens: 108000, TotalTokens: 108000},
		},
		{
			name: "invalid terminal facts fall back to request snapshot",
			context: model.TaskBillingContext{
				UsageProfile:             model.TaskUsageProfileSeedance,
				RequestedDurationSeconds: 5,
				Resolution:               "720p",
				SeedanceTokenBilling:     seedanceTokenBilling720p(),
			},
			facts: SeedanceTerminalFacts{
				DurationPresent:        true,
				Resolution:             "unsupported",
				ResolutionPresent:      true,
				FramesPerSecond:        241,
				FramesPerSecondPresent: true,
			},
			want: SeedanceTaskUsage{InputTokens: 0, CompletionTokens: 108000, TotalTokens: 108000},
		},
		{
			name: "reference duration is required",
			context: model.TaskBillingContext{
				UsageProfile:             model.TaskUsageProfileSeedance,
				RequestedDurationSeconds: 5,
				Resolution:               "720p",
				HasVideoInput:            true,
				SeedanceTokenBilling:     seedanceTokenBilling720p(),
			},
			wantError: "reference video duration is unavailable",
		},
		{
			name: "request duration must be bounded",
			context: model.TaskBillingContext{
				UsageProfile:             model.TaskUsageProfileSeedance,
				RequestedDurationSeconds: 3601,
				Resolution:               "720p",
				SeedanceTokenBilling:     seedanceTokenBilling720p(),
			},
			wantError: "output duration is out of range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage, err := CalculateSeedanceTaskUsage(&tt.context, tt.facts)
			if tt.wantError != "" {
				require.EqualError(t, err, tt.wantError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, usage)
		})
	}
}

func TestCalculateSeedanceTaskUsageUsesFrozenOfficialPriceGeometry(t *testing.T) {
	price := &types.SeedanceTokenPrice{Scenarios: map[string]types.SeedanceTokenPriceScenario{
		"720p:no_video": {
			PricePerMillion: "2",
			Width:           640,
			Height:          360,
			FrameRate:       30,
			PricingVersion:  "official-token-v1",
			Source:          "official-sheet",
		},
	}}
	usage, err := CalculateSeedanceTaskUsage(&model.TaskBillingContext{
		UsageProfile:             model.TaskUsageProfileSeedance,
		RequestedDurationSeconds: 5,
		Resolution:               "720p",
		SeedanceTokenPrice:       price,
	}, SeedanceTerminalFacts{
		Resolution:             "1080p",
		ResolutionPresent:      true,
		FramesPerSecond:        60,
		FramesPerSecondPresent: true,
	})

	require.NoError(t, err)
	assert.Equal(t, SeedanceTaskUsage{
		InputTokens:      0,
		CompletionTokens: 33750,
		TotalTokens:      33750,
	}, usage)
}

func TestPersistedSeedanceTaskUsage(t *testing.T) {
	tests := []struct {
		name string
		bc   *model.TaskBillingContext
		want SeedanceTaskUsage
		ok   bool
	}{
		{
			name: "version one snapshot",
			bc: &model.TaskBillingContext{
				UsageSnapshotVersion:  model.TaskUsageSnapshotVersion1,
				UsageInputTokens:      0,
				UsageCompletionTokens: 172800,
				UsageTotalTokens:      172800,
			},
			want: SeedanceTaskUsage{InputTokens: 0, CompletionTokens: 172800, TotalTokens: 172800},
			ok:   true,
		},
		{
			name: "legacy version",
			bc: &model.TaskBillingContext{
				UsageCompletionTokens: 108000,
				UsageTotalTokens:      108000,
			},
		},
		{
			name: "partial snapshot",
			bc: &model.TaskBillingContext{
				UsageSnapshotVersion:  model.TaskUsageSnapshotVersion1,
				UsageCompletionTokens: 108000,
			},
		},
		{
			name: "invalid relation",
			bc: &model.TaskBillingContext{
				UsageSnapshotVersion:  model.TaskUsageSnapshotVersion1,
				UsageCompletionTokens: 108000,
				UsageTotalTokens:      100000,
			},
		},
		{
			name: "inconsistent input snapshot",
			bc: &model.TaskBillingContext{
				UsageSnapshotVersion:  model.TaskUsageSnapshotVersion1,
				UsageInputTokens:      64000,
				UsageCompletionTokens: 108000,
				UsageTotalTokens:      172800,
			},
		},
		{
			name: "zero values",
			bc: &model.TaskBillingContext{
				UsageSnapshotVersion: model.TaskUsageSnapshotVersion1,
			},
		},
		{
			name: "over limit",
			bc: &model.TaskBillingContext{
				UsageSnapshotVersion:  model.TaskUsageSnapshotVersion1,
				UsageCompletionTokens: relaycommon.MaxTokensLimit + 1,
				UsageTotalTokens:      relaycommon.MaxTokensLimit + 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := PersistedSeedanceTaskUsage(tt.bc)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeSeedanceTaskUsage(t *testing.T) {
	t.Run("calculates missing usage", func(t *testing.T) {
		task := &model.Task{
			Status: model.TaskStatusSuccess,
			PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
				UsageProfile:             model.TaskUsageProfileSeedance,
				RequestedDurationSeconds: 5,
				Resolution:               "720p",
				SeedanceTokenBilling:     seedanceTokenBilling720p(),
			}},
		}
		result := &relaycommon.TaskInfo{Status: string(model.TaskStatusSuccess)}

		require.NoError(t, NormalizeSeedanceTaskUsage(task, result))

		assert.Equal(t, 108000, result.CompletionTokens)
		assert.True(t, result.CompletionTokensPresent)
		assert.Equal(t, 108000, result.TotalTokens)
		assert.True(t, result.TotalTokensPresent)
		assert.Equal(t, model.TaskUsageSourceLocalCalculated, result.UsageSource)
		assert.Equal(t, model.TaskUsageSourceLocalCalculated, task.PrivateData.BillingContext.UsageSource)
		assert.Equal(t, 0, task.PrivateData.BillingContext.UsageInputTokens)
		assert.Equal(t, 108000, task.PrivateData.BillingContext.BillingTokens)
	})

	t.Run("preserves upstream usage matching frozen formula", func(t *testing.T) {
		task := &model.Task{
			Status: model.TaskStatusSuccess,
			PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
				UsageProfile:             model.TaskUsageProfileSeedance,
				RequestedDurationSeconds: 5,
				Resolution:               "720p",
				SeedanceTokenBilling:     seedanceTokenBilling720p(),
			}},
		}
		result := &relaycommon.TaskInfo{
			Status:                  string(model.TaskStatusSuccess),
			CompletionTokens:        108000,
			CompletionTokensPresent: true,
			TotalTokens:             108000,
			TotalTokensPresent:      true,
		}

		require.NoError(t, NormalizeSeedanceTaskUsage(task, result))

		assert.Equal(t, 108000, result.CompletionTokens)
		assert.Equal(t, 108000, result.TotalTokens)
		assert.Equal(t, model.TaskUsageSourceUpstream, result.UsageSource)
		assert.Equal(t, model.TaskUsageSourceUpstream, task.PrivateData.BillingContext.UsageSource)
		assert.Equal(t, 0, task.PrivateData.BillingContext.UsageInputTokens)
		assert.Equal(t, 108000, task.PrivateData.BillingContext.BillingTokens)
	})

	t.Run("recalculates upstream usage that differs from frozen formula", func(t *testing.T) {
		task := &model.Task{
			Status: model.TaskStatusSuccess,
			PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
				UsageProfile:             model.TaskUsageProfileSeedance,
				RequestedDurationSeconds: 5,
				Resolution:               "720p",
				SeedanceTokenBilling:     seedanceTokenBilling720p(),
			}},
		}
		result := &relaycommon.TaskInfo{
			Status:                  string(model.TaskStatusSuccess),
			CompletionTokens:        108900,
			CompletionTokensPresent: true,
			TotalTokens:             108900,
			TotalTokensPresent:      true,
		}

		require.NoError(t, NormalizeSeedanceTaskUsage(task, result))

		assert.Equal(t, 108000, result.CompletionTokens)
		assert.Equal(t, 108000, result.TotalTokens)
		assert.Equal(t, model.TaskUsageSourceLocalCalculated, result.UsageSource)
		assert.Equal(t, model.TaskUsageSourceLocalCalculated, task.PrivateData.BillingContext.UsageSource)
		assert.Equal(t, 0, task.PrivateData.BillingContext.UsageInputTokens)
		assert.Equal(t, 108000, task.PrivateData.BillingContext.BillingTokens)
	})

	t.Run("reference video uses combined duration as completion usage", func(t *testing.T) {
		task := &model.Task{
			Status: model.TaskStatusSuccess,
			PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
				UsageProfile:             model.TaskUsageProfileSeedance,
				RequestedDurationSeconds: 5,
				Resolution:               "720p",
				HasVideoInput:            true,
				InputVideoDurationMS:     3000,
				SeedanceTokenBilling:     seedanceTokenBilling720p(),
			}},
		}
		result := &relaycommon.TaskInfo{
			Status:                  string(model.TaskStatusSuccess),
			CompletionTokens:        172800,
			CompletionTokensPresent: true,
			TotalTokens:             172800,
			TotalTokensPresent:      true,
		}

		require.NoError(t, NormalizeSeedanceTaskUsage(task, result))

		assert.Equal(t, 172800, result.CompletionTokens)
		assert.Equal(t, 172800, result.TotalTokens)
		assert.Equal(t, model.TaskUsageSourceUpstream, result.UsageSource)
		assert.Equal(t, 0, task.PrivateData.BillingContext.UsageInputTokens)
		assert.Equal(t, 172800, task.PrivateData.BillingContext.BillingTokens)
	})

	t.Run("inconsistent upstream usage cannot replace persisted pair", func(t *testing.T) {
		task := &model.Task{PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			UsageProfile: model.TaskUsageProfileSeedance, UsageSnapshotVersion: model.TaskUsageSnapshotVersion1,
			UsageInputTokens: 0, UsageCompletionTokens: 172800, UsageTotalTokens: 172800,
			HasVideoInput: true, InputVideoDurationMS: 3000,
		}}}
		result := &relaycommon.TaskInfo{Status: string(model.TaskStatusSuccess), CompletionTokens: 108900, CompletionTokensPresent: true, TotalTokens: 109000, TotalTokensPresent: true}
		require.NoError(t, NormalizeSeedanceTaskUsage(task, result))
		assert.Equal(t, 172800, task.PrivateData.BillingContext.UsageCompletionTokens)
		assert.Equal(t, 172800, task.PrivateData.BillingContext.UsageTotalTokens)
		assert.Equal(t, 0, task.PrivateData.BillingContext.UsageInputTokens)
		assert.Equal(t, 172800, task.PrivateData.BillingContext.BillingTokens)
		assert.Equal(t, model.TaskUsageSourceLocalCalculated, task.PrivateData.BillingContext.UsageSource)
	})

	t.Run("terminal geometry cannot replace frozen pricing geometry", func(t *testing.T) {
		bc := &model.TaskBillingContext{
			UsageProfile: model.TaskUsageProfileSeedance, UsageSnapshotVersion: model.TaskUsageSnapshotVersion1,
			RequestedDurationSeconds: 5, Resolution: "720p", UsageCompletionTokens: 108000, UsageTotalTokens: 108000,
			SeedanceTokenBilling: seedanceTokenBilling720p(),
		}
		task := &model.Task{PrivateData: model.TaskPrivateData{BillingContext: bc}}
		result := &relaycommon.TaskInfo{Status: string(model.TaskStatusSuccess), FramesPerSecond: 30, FramesPerSecondPresent: true}
		require.NoError(t, NormalizeSeedanceTaskUsage(task, result))
		assert.Equal(t, 108000, bc.UsageCompletionTokens)
		assert.Equal(t, 108000, bc.UsageTotalTokens)
	})

	t.Run("broken request facts use persisted fallback", func(t *testing.T) {
		bc := &model.TaskBillingContext{
			UsageProfile: model.TaskUsageProfileSeedance, UsageSnapshotVersion: model.TaskUsageSnapshotVersion1,
			UsageInputTokens: 0, UsageCompletionTokens: 172800, UsageTotalTokens: 172800, HasVideoInput: true,
		}
		task := &model.Task{PrivateData: model.TaskPrivateData{BillingContext: bc}}
		result := &relaycommon.TaskInfo{Status: string(model.TaskStatusSuccess)}
		require.NoError(t, NormalizeSeedanceTaskUsage(task, result))
		assert.Equal(t, 172800, result.CompletionTokens)
		assert.Equal(t, 172800, result.TotalTokens)
	})

	t.Run("versioned task rejects missing fallback", func(t *testing.T) {
		bc := &model.TaskBillingContext{UsageProfile: model.TaskUsageProfileSeedance, UsageSnapshotVersion: model.TaskUsageSnapshotVersion1, HasVideoInput: true}
		task := &model.Task{PrivateData: model.TaskPrivateData{BillingContext: bc}}
		err := NormalizeSeedanceTaskUsage(task, &relaycommon.TaskInfo{Status: string(model.TaskStatusSuccess)})
		require.EqualError(t, err, "versioned Seedance usage snapshot is unavailable")
	})

	t.Run("legacy task tolerates unavailable usage", func(t *testing.T) {
		bc := &model.TaskBillingContext{UsageProfile: model.TaskUsageProfileSeedance, HasVideoInput: true}
		task := &model.Task{PrivateData: model.TaskPrivateData{BillingContext: bc}}
		result := &relaycommon.TaskInfo{Status: string(model.TaskStatusSuccess)}
		require.NoError(t, NormalizeSeedanceTaskUsage(task, result))
		assert.False(t, result.CompletionTokensPresent)
		assert.Empty(t, result.UsageSource)
	})

	t.Run("ignores non Seedance profile", func(t *testing.T) {
		task := &model.Task{
			Status:      model.TaskStatusSuccess,
			PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{RequestedDurationSeconds: 5, Resolution: "720p"}},
		}
		result := &relaycommon.TaskInfo{Status: string(model.TaskStatusSuccess)}

		require.NoError(t, NormalizeSeedanceTaskUsage(task, result))

		assert.False(t, result.CompletionTokensPresent)
		assert.Empty(t, result.UsageSource)
		assert.Empty(t, task.PrivateData.BillingContext.UsageSource)
	})
}
