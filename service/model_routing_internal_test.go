package service

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluateGroupRoutingRejectsMalformedCachedSnapshot(t *testing.T) {
	previousLookup := routingPolicySnapshotLookup
	routingPolicySnapshotLookup = func(group, modelName string) (modelrouting.PolicySnapshot, bool) {
		return modelrouting.PolicySnapshot{
			ID: 7, GroupName: "wrong-group", CanonicalModel: modelName, Enabled: true,
			TargetsByChannel: map[int][]modelrouting.Target{},
		}, true
	}
	t.Cleanup(func() { routingPolicySnapshotLookup = previousLookup })
	resolution, duration, ratio := "1080p", 10, "16:9"
	input := modelrouting.FactsInput{
		CanonicalModel: modelrouting.Seedance20, OutputResolution: &resolution,
		DurationSeconds: &duration, AspectRatio: &ratio,
	}

	_, err := evaluateGroupRouting("分组A", modelrouting.Seedance20, &input)
	var selectionErr *ChannelSelectionError
	require.ErrorAs(t, err, &selectionErr)
	assert.Equal(t, types.ErrorCodeRoutingPolicyError, selectionErr.Code)
	assert.Equal(t, http.StatusInternalServerError, selectionErr.StatusCode)
}
