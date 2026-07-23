package clmmmall

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClmmMallDurationEstimatorUsesConvertedBillingSeconds(t *testing.T) {
	var _ channel.TaskDurationEstimator = (*TaskAdaptor)(nil)

	tests := []struct {
		name            string
		mappedModel     string
		duration        *int
		expectedSeconds int
	}{
		{name: "ordinary explicit duration", mappedModel: "sh-video", duration: intPointer(8), expectedSeconds: 8},
		{name: "ordinary omitted duration", mappedModel: "sh-video", expectedSeconds: 5},
		{name: "bounded explicit shorter duration", mappedModel: "me-videos-720P-10s", duration: intPointer(6), expectedSeconds: 6},
		{name: "bounded omitted duration", mappedModel: "me-videos-720P-10s", expectedSeconds: 10},
		{name: "fixed gz duration before suffix", mappedModel: "op-video-gz-10s", duration: intPointer(6), expectedSeconds: 10},
		{name: "fixed gz duration after suffix", mappedModel: "op-video-10s-gz", duration: intPointer(6), expectedSeconds: 10},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := `{"model":"client-model","content":[{"type":"text","text":"prompt"}]}`
			if test.duration != nil {
				body = fmt.Sprintf(`{"model":"client-model","duration":%d,"content":[{"type":"text","text":"prompt"}]}`, *test.duration)
			}
			c := newArkContext(body, true)
			info := &relaycommon.RelayInfo{
				ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: test.mappedModel},
				TaskRelayInfo: &relaycommon.TaskRelayInfo{},
			}
			adaptor := &TaskAdaptor{}
			require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))

			seconds, taskErr := adaptor.EstimateDurationSeconds(c, info)

			require.Nil(t, taskErr)
			assert.Equal(t, test.expectedSeconds, seconds)
		})
	}
}

func TestClmmMallDurationEstimatorRejectsInvalidDurationThroughBillingValidation(t *testing.T) {
	tests := []struct {
		name        string
		mappedModel string
		duration    int
	}{
		{name: "above suffix", mappedModel: "me-videos-720P-10s", duration: 11},
		{name: "above ordinary bound", mappedModel: "sh-video", duration: 16},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"model":"client-model","duration":%d,"content":[{"type":"text","text":"prompt"}]}`, test.duration)
			c := newArkContext(body, true)
			info := &relaycommon.RelayInfo{
				ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: test.mappedModel},
				TaskRelayInfo: &relaycommon.TaskRelayInfo{},
			}
			adaptor := &TaskAdaptor{}
			require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
			validationErr := adaptor.ValidateBillingRequest(c, info)
			require.NotNil(t, validationErr)

			seconds, taskErr := adaptor.EstimateDurationSeconds(c, info)

			assert.Zero(t, seconds)
			require.NotNil(t, taskErr)
			assert.Equal(t, validationErr.Code, taskErr.Code)
			assert.Equal(t, validationErr.Message, taskErr.Message)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
		})
	}
}
