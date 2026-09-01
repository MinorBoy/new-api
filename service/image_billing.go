package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	hosttypes "github.com/QuantumNous/new-api/types"
)

// RecordImageSettlementCount records an authoritative upstream image count
// for a unified image request. Legacy image requests have no snapshot and are
// intentionally left on their existing PriceData path.
func RecordImageSettlementCount(info *relaycommon.RelayInfo, count int64, source hosttypes.CostMeterSource) error {
	if info == nil || info.ImageBillingSnapshot == nil {
		return nil
	}
	if count < 1 || count > int64(dto.MaxImageN) {
		return fmt.Errorf("image count must be between 1 and %d", dto.MaxImageN)
	}
	info.ImageBillingSnapshot.SettledImages = &count
	info.ImageBillingSnapshot.MeterSource = string(source)
	return nil
}

// ImageSettlementQuota returns the frozen image charge. When no reliable
// upstream count was recorded, the validated request count remains the meter.
func ImageSettlementQuota(info *relaycommon.RelayInfo) (int, *common.QuotaClamp, error) {
	if info == nil || info.ImageBillingSnapshot == nil {
		return 0, nil, nil
	}
	count := info.ImageBillingSnapshot.RequestedImages
	if info.ImageBillingSnapshot.SettledImages != nil {
		count = *info.ImageBillingSnapshot.SettledImages
	}
	quota, clamp, err := info.ImageBillingSnapshot.Quota(count)
	if err != nil {
		return 0, clamp, err
	}
	noteQuotaClamp(info, clamp)
	return quota, clamp, nil
}
