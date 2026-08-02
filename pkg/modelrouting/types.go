package modelrouting

import "strings"

const (
	Seedance20     = "doubao-seedance-2-0-260128"
	Seedance20Fast = "doubao-seedance-2-0-fast-260128"
	Seedance20Mini = "doubao-seedance-2-0-mini-260615"
)

var CanonicalModels = []string{Seedance20, Seedance20Fast, Seedance20Mini}

// NormalizeCanonicalModel maps supported public aliases to the single routing
// policy identity for that model family. The caller's original model remains
// available for billing and audit; this normalization only selects a policy.
func NormalizeCanonicalModel(modelName string) string {
	modelName = strings.TrimSpace(modelName)
	if modelName == "doubao-seedance-2-0-mini-260128" {
		return Seedance20Mini
	}
	return modelName
}

type InputMode string

const (
	InputModeText            InputMode = "text"
	InputModeFirstFrame      InputMode = "first_frame"
	InputModeFirstLastFrames InputMode = "first_last_frames"
	InputModeOmniReference   InputMode = "omni_reference"
)

type Defaults struct {
	OutputResolution string `json:"output_resolution"`
	DurationSeconds  int    `json:"duration_seconds"`
	AspectRatio      string `json:"aspect_ratio"`
}

type FactsInput struct {
	CanonicalModel    string
	InputMode         InputMode
	OutputResolution  *string
	DurationSeconds   *int
	AspectRatio       *string
	ReferenceImages   int
	ReferenceVideos   int
	ReferenceAudios   int
	RequireRealPerson bool
	// ReferenceVideoURLs holds only normalized, fetchable HTTP(S) URLs from
	// reference_video inputs. ReferenceVideos remains the total count, including
	// embedded data: and asset:// media that cannot be inspected by the metadata
	// service. URLs carry signed query parameters and other sensitive material, so this
	// slice is never serialized: it stays in request memory and is excluded from Facts,
	// Audit, diagnostics and logs.
	ReferenceVideoURLs []string `json:"-"`
}

type Facts struct {
	GroupName         string          `json:"group_name"`
	CanonicalModel    string          `json:"canonical_model"`
	InputMode         InputMode       `json:"input_mode"`
	OutputResolution  string          `json:"output_resolution"`
	DurationSeconds   int             `json:"duration_seconds"`
	AspectRatio       string          `json:"aspect_ratio"`
	References        ReferenceLimits `json:"references"`
	RequireRealPerson bool            `json:"require_real_person"`
	// ReferenceVideoTotalDurationMS is request-local routing input. It is excluded
	// from audits and logs together with the reference video URLs used to resolve it.
	ReferenceVideoTotalDurationMS *int64 `json:"-"`
}

type DurationConstraint struct {
	Values []int `json:"values,omitempty"`
	Min    *int  `json:"min,omitempty"`
	Max    *int  `json:"max,omitempty"`
}

type ReferenceLimits struct {
	Images int `json:"images"`
	Videos int `json:"videos"`
	Audios int `json:"audios"`
}

type Constraints struct {
	OutputResolutions                  []string           `json:"output_resolutions"`
	Durations                          DurationConstraint `json:"durations"`
	AspectRatios                       []string           `json:"aspect_ratios,omitempty"`
	InputModes                         []InputMode        `json:"input_modes,omitempty"`
	ReferenceMinimums                  ReferenceLimits    `json:"reference_minimums,omitempty"`
	ReferenceLimits                    ReferenceLimits    `json:"reference_limits"`
	ReferenceTotalMax                  *int               `json:"reference_total_max,omitempty"`
	ReferenceVideoAudioTotalMax        *int               `json:"reference_video_audio_total_max,omitempty"`
	ReferenceVideoTotalDurationSeconds *int               `json:"reference_video_total_duration_seconds,omitempty"`
	ReferenceModes                     []string           `json:"reference_modes,omitempty"`
	SupportsRealPerson                 *bool              `json:"supports_real_person"`
}

type Target struct {
	ID                       int         `json:"id"`
	PolicyID                 int         `json:"policy_id"`
	ChannelID                int         `json:"channel_id"`
	Name                     string      `json:"name"`
	UpstreamModel            string      `json:"upstream_model"`
	CostVariantKey           string      `json:"cost_variant_key"`
	Priority                 int         `json:"target_priority"`
	MinimumExpectedMarginBPS *int        `json:"minimum_expected_margin_bps,omitempty"`
	Enabled                  bool        `json:"enabled"`
	Constraints              Constraints `json:"constraints"`
}

type PolicySnapshot struct {
	ID               int              `json:"id"`
	GroupName        string           `json:"group_name"`
	CanonicalModel   string           `json:"model"`
	Enabled          bool             `json:"enabled"`
	Defaults         Defaults         `json:"defaults"`
	TargetsByChannel map[int][]Target `json:"-"`
}

type MismatchReason string

const (
	MismatchResolution               MismatchReason = "resolution"
	MismatchDuration                 MismatchReason = "duration"
	MismatchAspectRatio              MismatchReason = "aspect_ratio"
	MismatchInputMode                MismatchReason = "input_mode"
	MismatchReferenceImages          MismatchReason = "reference_images"
	MismatchReferenceVideos          MismatchReason = "reference_videos"
	MismatchReferenceAudios          MismatchReason = "reference_audios"
	MismatchReferenceTotal           MismatchReason = "reference_total"
	MismatchReferenceVideoAudioTotal MismatchReason = "reference_video_audio_total"
	MismatchReferenceVideoDuration   MismatchReason = "reference_video_duration"
	MismatchRealPerson               MismatchReason = "real_person"
)

type Evaluation struct {
	CompatibleByChannel map[int]Target         `json:"-"`
	MismatchCounts      map[MismatchReason]int `json:"mismatch_counts"`
}

type Audit struct {
	PolicyID       int                    `json:"policy_id"`
	TargetID       int                    `json:"target_id"`
	TargetName     string                 `json:"target_name"`
	UpstreamModel  string                 `json:"upstream_model"`
	CostVariantKey string                 `json:"cost_variant_key"`
	Facts          Facts                  `json:"facts"`
	MismatchCounts map[MismatchReason]int `json:"mismatch_counts,omitempty"`
}
