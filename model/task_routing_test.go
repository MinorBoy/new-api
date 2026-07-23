package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitTaskKeepsCapabilityRoutingPrivate(t *testing.T) {
	routing := &modelrouting.Audit{
		PolicyID: 7, TargetID: 21, TargetName: "A1 1080p", UpstreamModel: "provider-1080p",
		Facts: modelrouting.Facts{
			GroupName: "分组A", CanonicalModel: modelrouting.Seedance20,
			OutputResolution: "1080p", DurationSeconds: 10, AspectRatio: "16:9",
		},
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: modelrouting.Seedance20,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeNewAPIVideo, ChannelId: 11,
			UpstreamModelName: "provider-1080p", Routing: routing,
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}

	task := InitTask(constant.TaskPlatform("49"), info)
	assert.Equal(t, modelrouting.Seedance20, task.Properties.OriginModelName)
	assert.Empty(t, task.Properties.UpstreamModelName)
	require.NotNil(t, task.PrivateData.Routing)
	assert.Equal(t, 21, task.PrivateData.Routing.TargetID)
	assert.Equal(t, "provider-1080p", task.PrivateData.Routing.UpstreamModel)
	assert.NotSame(t, routing, task.PrivateData.Routing)
}
