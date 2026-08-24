package modelrouting_test

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/stretchr/testify/assert"
)

func TestSeedanceSeriesContractForModel(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  modelrouting.SeedanceSeriesContract
	}{
		{name: "2.0", model: modelrouting.Seedance20, want: modelrouting.SeedanceSeriesContract{Series: "2.0", ReferenceLimits: modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, ReferenceTotalMax: 15, MaxDurationSeconds: 15}},
		{name: "2.0 fast", model: modelrouting.Seedance20Fast, want: modelrouting.SeedanceSeriesContract{Series: "2.0", ReferenceLimits: modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, ReferenceTotalMax: 15, MaxDurationSeconds: 15}},
		{name: "2.0 mini", model: modelrouting.Seedance20Mini, want: modelrouting.SeedanceSeriesContract{Series: "2.0", ReferenceLimits: modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, ReferenceTotalMax: 15, MaxDurationSeconds: 15}},
		{name: "2.5 public", model: modelrouting.Seedance25, want: modelrouting.SeedanceSeriesContract{Series: "2.5", ReferenceLimits: modelrouting.ReferenceLimits{Images: 30, Videos: 10, Audios: 10}, ReferenceTotalMax: 50, MaxDurationSeconds: 30}},
		{name: "2.5 config name", model: " seedance-2.5 ", want: modelrouting.SeedanceSeriesContract{Series: "2.5", ReferenceLimits: modelrouting.ReferenceLimits{Images: 30, Videos: 10, Audios: 10}, ReferenceTotalMax: 50, MaxDurationSeconds: 30}},
		{name: "unknown stays conservative", model: "legacy-provider-alias", want: modelrouting.SeedanceSeriesContract{Series: "2.0", ReferenceLimits: modelrouting.ReferenceLimits{Images: 9, Videos: 3, Audios: 3}, ReferenceTotalMax: 15, MaxDurationSeconds: 15}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, modelrouting.SeedanceSeriesContractForModel(test.model))
		})
	}
}
