package doubao

import (
	"strings"

	"github.com/QuantumNous/new-api/pkg/seedancepricing"
)

// Doubao family identifiers are retained as aliases over the shared Seedance
// capability profile.
const (
	seedance20Family     = seedancepricing.Family20
	seedance20FastFamily = seedancepricing.Family20Fast
	seedance20MiniFamily = seedancepricing.Family20Mini
	seedance25Family     = seedancepricing.Family25
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
	"doubao-seedance-2-5-260628",
}

var ChannelName = "doubao-video"

// seedancePricingFamily is the compatibility alias for seedancepricing.Family.
func seedancePricingFamily(modelName string) string {
	return seedancepricing.Family(modelName)
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
