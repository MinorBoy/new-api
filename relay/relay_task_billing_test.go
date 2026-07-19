package relay

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
