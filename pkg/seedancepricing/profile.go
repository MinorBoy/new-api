// Package seedancepricing holds the read-only pricing and resolution profile shared
// between the Doubao billing adapter and the profit-aware routing predictor. Both
// paths must agree on the official Seedance unit prices, resolution pixel dimensions
// and canonical frame rate, otherwise the predicted user revenue and the charged cost
// drift apart.
//
// The data captured here is the published Doubao Seedance price matrix
// (元/百万 token) plus the landscape 16:9 output dimensions used to estimate the
// token count for a video. Keeping the table in one place prevents the billing
// adapter and the profit predictor from maintaining two parallel copies.
package seedancepricing

import (
	"math"
	"strings"
)

// Seedance family identifiers. They double as the keys into the official price
// matrix and are returned by Family so callers can branch on family without
// re-parsing the model name.
const (
	Family20     = "seedance-2.0"
	Family20Fast = "seedance-2.0-fast"
	Family20Mini = "seedance-2.0-mini"
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

// priceKey indexes the official price matrix by output resolution tier (480p/720p
// share the base tier) and whether the request includes reference video input.
type priceKey struct {
	is1080p  bool
	is4k     bool
	hasVideo bool
}

// priceTable holds each family's official unit price (元/百万 token) per
// (output resolution tier, has-video) cell. The Doubao adapter divides by the
// family's base price to obtain a billing OtherRatio, while the profit predictor
// uses the same table to estimate user revenue at the official price.
var priceTable = map[string]map[priceKey]float64{
	Family20: {
		{hasVideo: false}:                46.0,
		{hasVideo: true}:                 28.0,
		{is1080p: true, hasVideo: false}: 51.0,
		{is1080p: true, hasVideo: true}:  31.0,
		{is4k: true, hasVideo: false}:    26.0,
		{is4k: true, hasVideo: true}:     16.0,
	},
	Family20Fast: {
		{hasVideo: false}: 37.0,
		{hasVideo: true}:  22.0,
	},
	Family20Mini: {
		{hasVideo: false}: 23.0,
		{hasVideo: true}:  14.0,
	},
}

// basePrice is each family's 480p/720p text-only (base) unit price. The Doubao
// billing adapter exposes this value to administrators as the model's ModelRatio.
var basePrice = map[string]float64{
	Family20:     46,
	Family20Fast: 37,
	Family20Mini: 23,
}

var supportedResolutions = []string{"480p", "720p", "1080p", "4k"}

// Family resolves a canonical Seedance model name to its pricing family. The
// matching mirrors the Doubao adapter's prefix rules, so legacy "doubao-seedance-*"
// names and forward-dated suffixes both classify consistently.
func Family(modelName string) string {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	switch {
	case strings.HasPrefix(modelName, "doubao-seedance-2-0-fast"):
		return Family20Fast
	case strings.HasPrefix(modelName, "doubao-seedance-2-0-mini"):
		return Family20Mini
	case strings.HasPrefix(modelName, "doubao-seedance-2-0"):
		return Family20
	case strings.HasPrefix(modelName, "doubao-seedance-1-5-pro"):
		return Family15Pro
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

// VideoInputRatio returns the billing multiplier (actual unit price / family base
// price) for the given model at the given output resolution, depending on whether
// the request includes reference video input. A multiplier of 1.0 means the cell
// matches the base price and the caller can omit the OtherRatio.
//
// The multiplier must still pass types.PriceData.AddOtherRatio's positive/finite
// guard before entering the billing path. ok=false means the combination is
// unsupported (e.g. mini at 1080p, or a non-Seedance model).
func VideoInputRatio(modelName, resolution string, hasVideo bool) (float64, bool) {
	family := Family(modelName)
	prices, ok := priceTable[family]
	if !ok {
		return 0, false
	}
	base := basePrice[family]
	if base <= 0 {
		return 0, false
	}
	normalized, ok := normalizeResolution(resolution)
	if !ok {
		return 0, false
	}
	price, ok := prices[priceKey{is1080p: normalized == "1080p", is4k: normalized == "4k", hasVideo: hasVideo}]
	if !ok {
		return 0, false
	}
	return price / base, true
}

// DurationMultiplier scales the 720p text-to-video base duration price for a
// request's official unit price, output pixel rate, and total processed video
// duration. Reference-video requests require a positive inspected input
// duration, so callers cannot silently undercharge an unmetered asset.
func DurationMultiplier(modelName, resolution string, hasVideo bool, inputDurationMS int64, outputDurationSeconds int) (float64, bool) {
	if outputDurationSeconds <= 0 || inputDurationMS < 0 || (hasVideo && inputDurationMS <= 0) {
		return 0, false
	}
	profile, ok := Profile(resolution)
	if !ok {
		return 0, false
	}
	baseProfile, ok := Profile("720p")
	if !ok {
		return 0, false
	}
	unitPriceRatio, ok := VideoInputRatio(modelName, resolution, hasVideo)
	if !ok {
		return 0, false
	}
	outputSeconds := float64(outputDurationSeconds)
	inputSeconds := float64(inputDurationMS) / 1000
	pixelRate := float64(profile.Width) * float64(profile.Height) * float64(profile.FrameRateNum) / float64(profile.FrameRateDen)
	basePixelRate := float64(baseProfile.Width) * float64(baseProfile.Height) * float64(baseProfile.FrameRateNum) / float64(baseProfile.FrameRateDen)
	multiplier := unitPriceRatio * (pixelRate / basePixelRate) * ((inputSeconds + outputSeconds) / outputSeconds)
	if multiplier <= 0 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
		return 0, false
	}
	return multiplier, true
}

// OfficialUnitPrice returns the raw published unit price (元/百万 token) for the
// given model, output resolution and video-input flag. It is the authoritative
// source both the billing adapter and the profit predictor must reference, and is
// exposed so acceptance tests can assert the table matches the official matrix.
func OfficialUnitPrice(modelName, resolution string, hasVideo bool) (float64, bool) {
	family := Family(modelName)
	prices, ok := priceTable[family]
	if !ok {
		return 0, false
	}
	normalized, ok := normalizeResolution(resolution)
	if !ok {
		return 0, false
	}
	price, ok := prices[priceKey{is1080p: normalized == "1080p", is4k: normalized == "4k", hasVideo: hasVideo}]
	if !ok {
		return 0, false
	}
	return price, true
}
