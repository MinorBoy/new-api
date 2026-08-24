package modelrouting

import "strings"

// SeedanceSeriesContract contains the shared capability limits for a public
// Seedance model family. Channel-specific contracts may impose stricter limits
// but must not exceed these values.
type SeedanceSeriesContract struct {
	Series             string
	ReferenceLimits    ReferenceLimits
	ReferenceTotalMax  int
	MaxDurationSeconds int
}

var seedance20SeriesContract = SeedanceSeriesContract{
	Series:             "2.0",
	ReferenceLimits:    ReferenceLimits{Images: 9, Videos: 3, Audios: 3},
	ReferenceTotalMax:  15,
	MaxDurationSeconds: 15,
}

var seedance25SeriesContract = SeedanceSeriesContract{
	Series:             "2.5",
	ReferenceLimits:    ReferenceLimits{Images: 30, Videos: 10, Audios: 10},
	ReferenceTotalMax:  50,
	MaxDurationSeconds: 30,
}

// SeedanceSeriesContractForModel resolves canonical and imported stable model
// names. Unknown names deliberately use the conservative 2.0 contract.
func SeedanceSeriesContractForModel(modelName string) SeedanceSeriesContract {
	switch strings.TrimSpace(modelName) {
	case Seedance25, "seedance-2.5":
		return seedance25SeriesContract
	default:
		return seedance20SeriesContract
	}
}
