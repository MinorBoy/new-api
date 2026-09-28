package modelrouting

import "strings"

const (
	MiniMaxH3    = "minimax-h3"
	MiniMaxH3VIP = "minimax-h3-vip"
)

// VideoSeriesContract describes provider-independent capability facts used by
// routing and channel contract validation. Provider-specific billing remains in
// the cost-rule layer and is selected by CostVariantKey.
type VideoSeriesContract struct {
	Series             string
	UpstreamModels     []string
	OutputResolutions  []string
	AspectRatios       []string
	MinDurationSeconds int
	MaxDurationSeconds int
	DefaultResolution  string
	DefaultAspectRatio string
	DefaultDuration    int
	ReferenceLimits    ReferenceLimits
	ReferenceTotalMax  int
	InputModes         []InputMode
}

var miniMaxH3Contract = VideoSeriesContract{
	Series:             MiniMaxH3,
	UpstreamModels:     []string{MiniMaxH3VIP},
	OutputResolutions:  []string{"720p", "2k"},
	AspectRatios:       []string{"auto", "1:1", "16:9", "9:16", "3:4", "4:3"},
	MinDurationSeconds: 4,
	MaxDurationSeconds: 15,
	DefaultResolution:  "2k",
	DefaultAspectRatio: "16:9",
	DefaultDuration:    15,
	ReferenceLimits:    ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
	ReferenceTotalMax:  15,
	InputModes: []InputMode{
		InputModeText,
		InputModeFirstFrame,
		InputModeFirstLastFrames,
		InputModeOmniReference,
	},
}

func MiniMaxH3Contract(modelName string) (VideoSeriesContract, bool) {
	switch strings.ToLower(strings.TrimSpace(modelName)) {
	case MiniMaxH3, MiniMaxH3VIP:
		return miniMaxH3Contract, true
	default:
		return VideoSeriesContract{}, false
	}
}

func IsMiniMaxH3Model(modelName string) bool {
	_, ok := MiniMaxH3Contract(modelName)
	return ok
}

func VideoSeriesContractForModel(modelName string) (VideoSeriesContract, bool) {
	if contract, ok := MiniMaxH3Contract(modelName); ok {
		return contract, true
	}
	return VideoSeriesContract{}, false
}
