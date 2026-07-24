package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveMappedModel(t *testing.T) {
	tests := []struct {
		name        string
		origin      string
		mappingJSON string
		want        string
		changed     bool
	}{
		{name: "no mapping", origin: "client", mappingJSON: `{}`, want: "client"},
		{name: "single mapping", origin: "client", mappingJSON: `{"client":"provider"}`, want: "provider", changed: true},
		{name: "mapping chain", origin: "client", mappingJSON: `{"client":"alias","alias":"provider"}`, want: "provider", changed: true},
		{name: "self mapping", origin: "client", mappingJSON: `{"client":"client"}`, want: "client"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapped, changed, err := ResolveMappedModel(tt.origin, tt.mappingJSON)
			require.NoError(t, err)
			assert.Equal(t, tt.want, mapped)
			assert.Equal(t, tt.changed, changed)
		})
	}
}

func TestResolveMappedModelRejectsCycle(t *testing.T) {
	_, _, err := ResolveMappedModel("client", `{"client":"alias","alias":"client"}`)
	require.Error(t, err)
}
