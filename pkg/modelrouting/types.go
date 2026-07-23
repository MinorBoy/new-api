package modelrouting

const (
	Seedance20     = "doubao-seedance-2-0-260128"
	Seedance20Fast = "doubao-seedance-2-0-fast-260128"
	Seedance20Mini = "doubao-seedance-2-0-mini-260615"
)

var CanonicalModels = []string{Seedance20, Seedance20Fast, Seedance20Mini}

type Defaults struct {
	OutputResolution string `json:"output_resolution"`
	DurationSeconds  int    `json:"duration_seconds"`
	AspectRatio      string `json:"aspect_ratio"`
}

type FactsInput struct {
	CanonicalModel    string
	OutputResolution  *string
	DurationSeconds   *int
	AspectRatio       *string
	ReferenceImages   int
	ReferenceVideos   int
	ReferenceAudios   int
	RequireRealPerson bool
}

type Facts struct {
	GroupName         string          `json:"group_name"`
	CanonicalModel    string          `json:"canonical_model"`
	OutputResolution  string          `json:"output_resolution"`
	DurationSeconds   int             `json:"duration_seconds"`
	AspectRatio       string          `json:"aspect_ratio"`
	References        ReferenceLimits `json:"references"`
	RequireRealPerson bool            `json:"require_real_person"`
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
	OutputResolutions  []string           `json:"output_resolutions"`
	Durations          DurationConstraint `json:"durations"`
	AspectRatios       []string           `json:"aspect_ratios,omitempty"`
	ReferenceLimits    ReferenceLimits    `json:"reference_limits"`
	SupportsRealPerson *bool              `json:"supports_real_person"`
}

type Target struct {
	ID            int         `json:"id"`
	PolicyID      int         `json:"policy_id"`
	ChannelID     int         `json:"channel_id"`
	Name          string      `json:"name"`
	UpstreamModel string      `json:"upstream_model"`
	Priority      int         `json:"target_priority"`
	Enabled       bool        `json:"enabled"`
	Constraints   Constraints `json:"constraints"`
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
	MismatchResolution      MismatchReason = "resolution"
	MismatchDuration        MismatchReason = "duration"
	MismatchAspectRatio     MismatchReason = "aspect_ratio"
	MismatchReferenceImages MismatchReason = "reference_images"
	MismatchReferenceVideos MismatchReason = "reference_videos"
	MismatchReferenceAudios MismatchReason = "reference_audios"
	MismatchRealPerson      MismatchReason = "real_person"
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
	Facts          Facts                  `json:"facts"`
	MismatchCounts map[MismatchReason]int `json:"mismatch_counts,omitempty"`
}
