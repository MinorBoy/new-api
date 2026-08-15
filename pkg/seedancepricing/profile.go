// Package seedancepricing holds Seedance model-family and resolution profiles used
// for capability validation and token-usage estimation. User pricing is imported as
// explicit scenario duration prices and must not be derived from this package.
package seedancepricing

import (
	"strings"
)

// Seedance family identifiers let callers branch on family without re-parsing model
// names.
const (
	Family20     = "seedance-2.0"
	Family20Fast = "seedance-2.0-fast"
	Family20Mini = "seedance-2.0-mini"
	Family25     = "seedance-2.5"
	Family15Pro  = "seedance-1.5-pro"
)

const (
	// frameRateNum/frameRateDen together describe Seedance's published 24 fps output.
	frameRateNum int64 = 24
	frameRateDen int64 = 1
)

// ResolutionProfile describes the pixel dimensions and frame rate Seedance produces
// for a given output resolution. The dimensions are the landscape 16:9 reference
// values, which are the canonical output size used to estimate token consumption.
type ResolutionProfile struct {
	Name         string
	Width        int
	Height       int
	FrameRateNum int64
	FrameRateDen int64
}

var resolutionProfiles = map[string]ResolutionProfile{
	"480p":  {Name: "480p", Width: 864, Height: 496, FrameRateNum: frameRateNum, FrameRateDen: frameRateDen},
	"720p":  {Name: "720p", Width: 1280, Height: 720, FrameRateNum: frameRateNum, FrameRateDen: frameRateDen},
	"1080p": {Name: "1080p", Width: 1920, Height: 1080, FrameRateNum: frameRateNum, FrameRateDen: frameRateDen},
	"4k":    {Name: "4k", Width: 3840, Height: 2160, FrameRateNum: frameRateNum, FrameRateDen: frameRateDen},
}

var supportedResolutions = []string{"480p", "720p", "1080p", "4k"}

var familyResolutionSupport = map[string]map[string]bool{
	Family20:     {"480p": true, "720p": true, "1080p": true, "4k": true},
	Family20Fast: {"480p": true, "720p": true},
	Family20Mini: {"480p": true, "720p": true},
	Family25:     {"480p": true, "720p": true},
	Family15Pro:  {"480p": true, "720p": true, "1080p": true},
}

// Family resolves canonical and provider-specific Seedance model names to their
// capability family. Separators and provider prefixes do not affect the result.
func Family(modelName string) string {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	if !strings.Contains(modelName, "seedance") {
		return ""
	}
	compact := strings.NewReplacer("-", "", "_", "", ".", "", " ", "").Replace(modelName)
	switch {
	case strings.Contains(compact, "seedance20fast"):
		return Family20Fast
	case strings.Contains(compact, "seedance20mini"):
		return Family20Mini
	case strings.Contains(compact, "seedance15pro"):
		return Family15Pro
	case strings.Contains(compact, "seedance25"):
		return Family25
	case strings.Contains(compact, "seedance20"):
		return Family20
	default:
		return ""
	}
}

// normalizeResolution lowercases, trims and defaults an empty resolution to 720p
// (the documented Seedance default). It returns ok=false for unsupported values.
func normalizeResolution(resolution string) (string, bool) {
	resolution = strings.ToLower(strings.TrimSpace(resolution))
	if resolution == "" {
		resolution = "720p"
	}
	for _, supported := range supportedResolutions {
		if resolution == supported {
			return resolution, true
		}
	}
	return "", false
}

// Profile returns the canonical pixel dimensions and frame rate for a Seedance
// output resolution. The resolution is normalized (trimmed/lowercased, empty
// defaults to 720p). Unknown resolutions return ok=false.
func Profile(resolution string) (ResolutionProfile, bool) {
	normalized, ok := normalizeResolution(resolution)
	if !ok {
		return ResolutionProfile{}, false
	}
	profile, ok := resolutionProfiles[normalized]
	if !ok {
		return ResolutionProfile{}, false
	}
	return profile, true
}

// SupportsResolution reports whether a Seedance family supports an output
// resolution. This is a capability check only and has no pricing semantics.
func SupportsResolution(modelName, resolution string) bool {
	family := Family(modelName)
	resolutions, ok := familyResolutionSupport[family]
	if !ok {
		return false
	}
	normalized, ok := normalizeResolution(resolution)
	if !ok {
		return false
	}
	return resolutions[normalized]
}
