package relay

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskDurationQuota(t *testing.T) {
	tests := []struct {
		name      string
		rule      types.DurationPrice
		requested int
		group     float64
		ratios    map[string]float64
		wantQuota int
		wantSecs  int
	}{
		{
			name:      "seconds with resolution ratio",
			rule:      types.DurationPrice{Price: 0.1, Unit: types.DurationUnitSecond, RoundingStepSeconds: 1, MinimumDurationSeconds: 4},
			requested: 6,
			group:     1,
			ratios:    map[string]float64{"resolution": 2.5},
			wantQuota: 750_000,
			wantSecs:  6,
		},
		{
			name:      "minutes round to five second step",
			rule:      types.DurationPrice{Price: 6, Unit: types.DurationUnitMinute, RoundingStepSeconds: 5, MinimumDurationSeconds: 4},
			requested: 6,
			group:     0.8,
			wantQuota: 400_000,
			wantSecs:  10,
		},
		{
			name:      "minimum duration",
			rule:      types.DurationPrice{Price: 0.2, Unit: types.DurationUnitSecond, RoundingStepSeconds: 1, MinimumDurationSeconds: 4},
			requested: 1,
			group:     1,
			wantQuota: 400_000,
			wantSecs:  4,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			priceData := types.PriceData{
				BillingMode:    "per_duration",
				DurationPrice:  &test.rule,
				GroupRatioInfo: types.GroupRatioInfo{GroupRatio: test.group},
			}
			priceData.ReplaceOtherRatios(test.ratios)

			quota, billable, clamp, err := taskDurationQuota(priceData, test.requested)

			require.NoError(t, err)
			assert.Equal(t, test.wantQuota, quota)
			assert.Equal(t, test.wantSecs, billable)
			assert.Nil(t, clamp)
		})
	}
}

func TestTaskDurationQuotaRejectsReservedDurationRatios(t *testing.T) {
	rule := types.DurationPrice{Price: 0.1, Unit: types.DurationUnitSecond, RoundingStepSeconds: 1, MinimumDurationSeconds: 4}
	for _, key := range []string{"seconds", "duration"} {
		t.Run(key, func(t *testing.T) {
			priceData := types.PriceData{DurationPrice: &rule, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}}
			priceData.ReplaceOtherRatios(map[string]float64{key: 1})

			_, _, _, err := taskDurationQuota(priceData, 6)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "reserved duration ratio")
		})
	}
}

func TestTaskDurationQuotaSaturatesFinitePrice(t *testing.T) {
	rule := types.DurationPrice{Price: math.MaxFloat64, Unit: types.DurationUnitSecond, RoundingStepSeconds: 1, MinimumDurationSeconds: 4}
	priceData := types.PriceData{DurationPrice: &rule, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}}

	quota, billable, clamp, err := taskDurationQuota(priceData, 6)

	require.NoError(t, err)
	assert.Equal(t, common.MaxQuota, quota)
	assert.Equal(t, 6, billable)
	require.NotNil(t, clamp)
	assert.Equal(t, common.QuotaClampOverflow, clamp.Kind)

	info := &relaycommon.RelayInfo{}
	noteTaskQuotaClamp(info, clamp)
	assert.Same(t, clamp, info.QuotaClamp)
}

func TestTaskQuotaWithOtherRatiosUsesRawBase(t *testing.T) {
	tests := []struct {
		name      string
		priceData types.PriceData
		ratios    map[string]float64
		want      int
	}{
		{
			name: "ratio mode converts once after all multipliers",
			priceData: types.PriceData{
				ModelRatio:     46.0 / 14,
				GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
			},
			ratios: map[string]float64{
				"duration":     2,
				"service_tier": 0.5,
				"audio":        28.0 / 46,
			},
			want: 500_000,
		},
		{
			name: "fixed price mode",
			priceData: types.PriceData{
				UsePrice:       true,
				ModelPrice:     0.75,
				GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1.2},
			},
			ratios: map[string]float64{"duration": 2},
			want:   900_000,
		},
		{
			name: "no other ratios",
			priceData: types.PriceData{
				ModelRatio:     2,
				GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 0.5},
			},
			want: 250_000,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for name, ratio := range test.ratios {
				test.priceData.AddOtherRatio(name, ratio)
			}

			quota, clamp := taskQuotaWithOtherRatios(test.priceData)

			assert.Equal(t, test.want, quota)
			assert.Nil(t, clamp)
		})
	}
}

func TestTaskRecalcQuotaFromRatiosUsesRawBase(t *testing.T) {
	tests := []struct {
		name      string
		priceData types.PriceData
		ratios    map[string]float64
		want      int
	}{
		{
			name: "ratio mode converts once after adjusted multipliers",
			priceData: types.PriceData{
				Quota:          499_999,
				ModelRatio:     46.0 / 14,
				GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
			},
			ratios: map[string]float64{
				"duration":     2,
				"service_tier": 0.5,
				"audio":        28.0 / 46,
			},
			want: 500_000,
		},
		{
			name: "fixed price mode",
			priceData: types.PriceData{
				UsePrice:       true,
				ModelPrice:     0.75,
				Quota:          900_000,
				GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1.2},
			},
			ratios: map[string]float64{"duration": 2},
			want:   900_000,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{PriceData: test.priceData}
			info.PriceData.AddOtherRatio("estimated", 1.5)

			quota, ok := recalcQuotaFromRatios(info, test.ratios)

			require.True(t, ok)
			assert.Equal(t, test.want, quota)
		})
	}
}
