package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
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
