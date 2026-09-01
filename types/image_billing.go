package types

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/shopspring/decimal"
)

var (
	ErrInvalidImageBillingSnapshot = errors.New("invalid image billing snapshot")
	ErrInvalidImageCount           = errors.New("image count must be between 1 and 128")
)

// ImageBillingSnapshot freezes the public price inputs used by one image
// request. Settlement must use this snapshot instead of reading the mutable
// global catalog again.
type ImageBillingSnapshot struct {
	CatalogVersion   int    `json:"catalog_version"`
	Model            string `json:"model"`
	Endpoint         string `json:"endpoint"`
	SKUKey           string `json:"sku"`
	UnitSalePriceUSD string `json:"unit_sale_price_usd"`
	RequestedImages  int64  `json:"requested_images"`
	SettledImages    *int64 `json:"settled_images,omitempty"`
	MeterSource      string `json:"meter_source,omitempty"`
	GroupRatio       string `json:"group_ratio"`
	QuotaPerUnit     string `json:"quota_per_unit"`
}

// Quota computes the charge for imageCount using only decimal arithmetic.
// A non-nil clamp is returned when the final conversion reached the int32
// quota boundary; callers decide whether that is acceptable for the current
// billing phase and must retain the marker for audit logging.
func (s ImageBillingSnapshot) Quota(imageCount int64) (int, *common.QuotaClamp, error) {
	if imageCount < 1 || imageCount > int64(dto.MaxImageN) {
		return 0, nil, fmt.Errorf("%w: %d", ErrInvalidImageCount, imageCount)
	}
	price, err := parseImageBillingDecimal(s.UnitSalePriceUSD, "unit sale price")
	if err != nil {
		return 0, nil, err
	}
	groupRatio, err := parseImageBillingDecimal(s.GroupRatio, "group ratio")
	if err != nil {
		return 0, nil, err
	}
	quotaPerUnit, err := parseImageBillingDecimal(s.QuotaPerUnit, "quota per unit")
	if err != nil || !quotaPerUnit.GreaterThan(decimal.Zero) {
		if err != nil {
			return 0, nil, err
		}
		return 0, nil, fmt.Errorf("%w: quota per unit must be positive", ErrInvalidImageBillingSnapshot)
	}
	quota := price.Mul(decimal.NewFromInt(imageCount)).Mul(groupRatio).Mul(quotaPerUnit)
	if quota.IsNegative() {
		return 0, nil, fmt.Errorf("%w: calculated quota cannot be negative", ErrInvalidImageBillingSnapshot)
	}
	value, clamp := common.QuotaFromDecimalChecked(quota)
	return value, clamp, nil
}

func parseImageBillingDecimal(raw, name string) (decimal.Decimal, error) {
	value, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil || value.IsNegative() {
		return decimal.Zero, fmt.Errorf("%w: %s must be a non-negative decimal", ErrInvalidImageBillingSnapshot, name)
	}
	return value, nil
}
