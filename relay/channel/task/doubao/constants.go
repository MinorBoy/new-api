package doubao

import (
	"strings"

	"github.com/QuantumNous/new-api/pkg/seedancepricing"
)

// Doubao family identifiers are retained as aliases over the shared seedancepricing
// package so the billing adapter and its tests keep their existing branch logic and
// names without duplicating the official price table.
const (
	seedance20Family     = seedancepricing.Family20
	seedance20FastFamily = seedancepricing.Family20Fast
	seedance20MiniFamily = seedancepricing.Family20Mini
	seedance15ProFamily  = seedancepricing.Family15Pro
)

var ModelList = []string{
	"doubao-seedance-1-0-pro-250528",
	"doubao-seedance-1-0-lite-t2v",
	"doubao-seedance-1-0-lite-i2v",
	"doubao-seedance-1-5-pro-251215",
	"doubao-seedance-2-0-260128",
	"doubao-seedance-2-0-fast-260128",
	"doubao-seedance-2-0-mini-260615",
}

var ChannelName = "doubao-video"

// seedancePricingFamily is the compatibility alias for seedancepricing.Family,
// retained so existing billing-adapter call sites keep their names while delegating
// to the single shared table.
func seedancePricingFamily(modelName string) string {
	return seedancepricing.Family(modelName)
}

// GetVideoInputRatio returns the billing multiplier (actual unit price / family base
// price) for the given model at the given output resolution, depending on whether
// the request includes reference video input. It delegates to seedancepricing so the
// Doubao adapter and the profit predictor share one source of truth. A multiplier of
// 1.0 means the caller may omit the OtherRatio.
func GetVideoInputRatio(modelName, resolution string, hasVideo bool) (float64, bool) {
	return seedancepricing.VideoInputRatio(modelName, resolution, hasVideo)
}

// GetVideoBillingRatio is the descriptive alias used by billing callers.
func GetVideoBillingRatio(modelName, resolution string, hasVideo bool) (float64, bool) {
	return GetVideoInputRatio(modelName, resolution, hasVideo)
}

func GetSeedance15ProRatios(generateAudio, draft bool, serviceTier string) (map[string]float64, bool) {
	serviceTier = strings.ToLower(strings.TrimSpace(serviceTier))
	if serviceTier == "" {
		serviceTier = "default"
	}
	if serviceTier != "default" && serviceTier != "flex" {
		return nil, false
	}
	ratios := make(map[string]float64)
	if generateAudio {
		ratios["audio"] = 2
	}
	if serviceTier == "flex" {
		ratios["service_tier"] = 0.5
	}
	if draft {
		if generateAudio {
			ratios["draft_estimate"] = 0.6
		} else {
			ratios["draft_estimate"] = 0.7
		}
	}
	return ratios, true
}
