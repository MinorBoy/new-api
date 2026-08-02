package relay

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/pkg/videometa"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type taskUsageMetadataClient struct {
	durationMS int64
	err        error
	calls      atomic.Int32
}

func (c *taskUsageMetadataClient) Metadata(_ context.Context, _ string) (videometa.Metadata, error) {
	c.calls.Add(1)
	if c.err != nil {
		return videometa.Metadata{}, c.err
	}
	return videometa.Metadata{DurationMS: c.durationMS}, nil
}

func TestPrepareSeedanceUsageSnapshot(t *testing.T) {
	tests := []struct {
		name                 string
		inputVideoDurationMS int64
		resolution           string
		hasVideoInput        bool
		wantCompletionTokens int
		wantTotalTokens      int
		wantErr              string
	}{
		{
			name:                 "text or image input",
			resolution:           "720p",
			wantCompletionTokens: 108000,
			wantTotalTokens:      108000,
		},
		{
			name:                 "reference video input",
			inputVideoDurationMS: 3000,
			resolution:           "720p",
			hasVideoInput:        true,
			wantCompletionTokens: 108000,
			wantTotalTokens:      172800,
		},
		{
			name:          "missing reference video duration",
			resolution:    "720p",
			hasVideoInput: true,
			wantErr:       "reference video duration is unavailable",
		},
		{
			name:       "unsupported resolution",
			resolution: "bad",
			wantErr:    "output resolution is unsupported",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				TaskRelayInfo: &relaycommon.TaskRelayInfo{
					InputVideoDurationMS: test.inputVideoDurationMS,
				},
			}

			err := prepareSeedanceUsageSnapshot(info, 5, test.resolution, test.hasVideoInput)

			if test.wantErr != "" {
				require.EqualError(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.wantCompletionTokens, info.TaskRelayInfo.UsageCompletionTokens)
			assert.Equal(t, test.wantTotalTokens, info.TaskRelayInfo.UsageTotalTokens)
		})
	}
}

func TestPrepareSeedanceUsageInputsStoresAggregateDurationForEveryCostMode(t *testing.T) {
	for _, costMode := range []types.CostMode{"", types.CostModeFree, types.CostModePerToken, types.CostModePerRequest, types.CostModePerDuration} {
		t.Run(string(costMode), func(t *testing.T) {
			client := &taskUsageMetadataClient{durationMS: 2500}
			service.SetVideoMetadataClient(client)
			t.Cleanup(func() { service.SetVideoMetadataClient(nil) })
			retryParam := &service.RetryParam{RoutingInput: &modelrouting.FactsInput{
				ReferenceVideos:    2,
				ReferenceVideoURLs: []string{"https://assets.example/a.mp4", "https://assets.example/b.mp4"},
			}}
			info := &relaycommon.RelayInfo{
				TaskRelayInfo: &relaycommon.TaskRelayInfo{},
			}

			taskErr := prepareSeedanceUsageInputs(t.Context(), retryParam, info)

			require.Nil(t, taskErr)
			assert.Equal(t, int64(5000), info.TaskRelayInfo.InputVideoDurationMS)
			assert.Equal(t, int32(2), client.calls.Load())
		})
	}
}

func TestPrepareSeedanceUsageInputsSkipsRequestsWithoutReferenceVideo(t *testing.T) {
	client := &taskUsageMetadataClient{durationMS: 2500}
	service.SetVideoMetadataClient(client)
	t.Cleanup(func() { service.SetVideoMetadataClient(nil) })
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}

	taskErr := prepareSeedanceUsageInputs(t.Context(), &service.RetryParam{}, info)

	require.Nil(t, taskErr)
	assert.Zero(t, info.TaskRelayInfo.InputVideoDurationMS)
	assert.Zero(t, client.calls.Load())
}

func TestPrepareSeedanceUsageInputsRejectsInvalidMedia(t *testing.T) {
	client := &taskUsageMetadataClient{err: &service.VideoMetadataError{Kind: service.VideoMetadataInvalidMedia}}
	service.SetVideoMetadataClient(client)
	t.Cleanup(func() { service.SetVideoMetadataClient(nil) })
	retryParam := &service.RetryParam{RoutingInput: &modelrouting.FactsInput{
		ReferenceVideos:    1,
		ReferenceVideoURLs: []string{"https://assets.example/a.mp4"},
	}}
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}

	taskErr := prepareSeedanceUsageInputs(t.Context(), retryParam, info)

	require.NotNil(t, taskErr)
	assert.Equal(t, 400, taskErr.StatusCode)
	assert.Equal(t, "invalid_reference_video", taskErr.Code)
}

func TestPrepareSeedanceUsageInputsFailsClosedWhenMetadataUnavailable(t *testing.T) {
	client := &taskUsageMetadataClient{err: &service.VideoMetadataError{Kind: service.VideoMetadataUnavailable}}
	service.SetVideoMetadataClient(client)
	t.Cleanup(func() { service.SetVideoMetadataClient(nil) })
	retryParam := &service.RetryParam{RoutingInput: &modelrouting.FactsInput{
		ReferenceVideos:    1,
		ReferenceVideoURLs: []string{"https://assets.example/a.mp4"},
	}}
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}

	taskErr := prepareSeedanceUsageInputs(t.Context(), retryParam, info)

	require.NotNil(t, taskErr)
	assert.Equal(t, 503, taskErr.StatusCode)
	assert.Equal(t, "video_metadata_unavailable", taskErr.Code)
}
