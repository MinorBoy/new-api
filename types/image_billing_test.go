package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageBillingSnapshotQuotaUsesFrozenDecimalPriceAndGroupRatio(t *testing.T) {
	snapshot := ImageBillingSnapshot{
		CatalogVersion:   1,
		Model:            "gpt-image-1",
		Endpoint:         "generations",
		SKUKey:           "gen-1024x1024-medium",
		UnitSalePriceUSD: "0.125",
		RequestedImages:  3,
		GroupRatio:       "0.8",
		QuotaPerUnit:     "500000",
	}

	quota, clamp, err := snapshot.Quota(3)
	require.NoError(t, err)
	assert.Nil(t, clamp)
	assert.Equal(t, 150000, quota)
}

func TestImageBillingSnapshotQuotaRejectsInvalidImageCount(t *testing.T) {
	snapshot := ImageBillingSnapshot{
		UnitSalePriceUSD: "1",
		GroupRatio:       "1",
		QuotaPerUnit:     "500000",
	}
	for _, count := range []int64{0, -1, 129} {
		t.Run(string(rune('0'+count)), func(t *testing.T) {
			_, _, err := snapshot.Quota(count)
			require.Error(t, err)
		})
	}
}

func TestImageBillingSnapshotQuotaRejectsInvalidDecimal(t *testing.T) {
	snapshot := ImageBillingSnapshot{
		UnitSalePriceUSD: "NaN",
		GroupRatio:       "1",
		QuotaPerUnit:     "500000",
	}
	_, _, err := snapshot.Quota(1)
	require.Error(t, err)
}
