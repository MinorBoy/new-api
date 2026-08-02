package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			},
			want: SeedanceTaskUsage{CompletionTokens: 108000, TotalTokens: 108000},
		},
		{
			name: "reference video",
			context: model.TaskBillingContext{
				UsageProfile:             model.TaskUsageProfileSeedance,
				RequestedDurationSeconds: 5,
				Resolution:               "720p",
				HasVideoInput:            true,
				InputVideoDurationMS:     3000,
			},
			want: SeedanceTaskUsage{CompletionTokens: 108000, TotalTokens: 172800},
		},
		{
			name: "valid terminal facts take precedence",
			context: model.TaskBillingContext{
				UsageProfile:             model.TaskUsageProfileSeedance,
				RequestedDurationSeconds: 5,
				Resolution:               "720p",
			},
			facts: SeedanceTerminalFacts{
				DurationSeconds:        5,
				DurationPresent:        true,
				Resolution:             "720p",
				ResolutionPresent:      true,
				FramesPerSecond:        30,
				FramesPerSecondPresent: true,
			},
			want: SeedanceTaskUsage{CompletionTokens: 135000, TotalTokens: 135000},
		},
		{
			name: "invalid terminal facts fall back to request snapshot",
			context: model.TaskBillingContext{
				UsageProfile:             model.TaskUsageProfileSeedance,
				RequestedDurationSeconds: 5,
				Resolution:               "720p",
			},
			facts: SeedanceTerminalFacts{
				DurationPresent:        true,
				Resolution:             "unsupported",
				ResolutionPresent:      true,
				FramesPerSecond:        241,
				FramesPerSecondPresent: true,
			},
			want: SeedanceTaskUsage{CompletionTokens: 108000, TotalTokens: 108000},
		},
		{
			name: "reference duration is required",
			context: model.TaskBillingContext{
				UsageProfile:             model.TaskUsageProfileSeedance,
				RequestedDurationSeconds: 5,
				Resolution:               "720p",
				HasVideoInput:            true,
			},
			wantError: "reference video duration is unavailable",
		},
		{
			name: "request duration must be bounded",
			context: model.TaskBillingContext{
				UsageProfile:             model.TaskUsageProfileSeedance,
				RequestedDurationSeconds: 3601,
				Resolution:               "720p",
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
				UsageCompletionTokens: 108000,
				UsageTotalTokens:      172800,
			},
			want: SeedanceTaskUsage{CompletionTokens: 108000, TotalTokens: 172800},
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
		assert.Equal(t, 108000, task.PrivateData.BillingContext.BillingTokens)
	})

	t.Run("preserves authoritative usage", func(t *testing.T) {
		task := &model.Task{
			Status: model.TaskStatusSuccess,
			PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
				UsageProfile:             model.TaskUsageProfileSeedance,
				RequestedDurationSeconds: 5,
				Resolution:               "720p",
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

		assert.Equal(t, 108900, result.CompletionTokens)
		assert.Equal(t, 108900, result.TotalTokens)
		assert.Equal(t, model.TaskUsageSourceUpstream, result.UsageSource)
		assert.Equal(t, model.TaskUsageSourceUpstream, task.PrivateData.BillingContext.UsageSource)
		assert.Equal(t, 108900, task.PrivateData.BillingContext.BillingTokens)
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
