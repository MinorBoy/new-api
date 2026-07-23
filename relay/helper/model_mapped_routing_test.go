package helper

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelMappedHelperPrefersCapabilityTarget(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("model_mapping", `{"doubao-seedance-2-0-260128":"ordinary-map","provider-1080p":"must-not-chain"}`)
	request := &dto.GeneralOpenAIRequest{Model: modelrouting.Seedance20}
	info := &relaycommon.RelayInfo{
		OriginModelName: modelrouting.Seedance20,
		ChannelMeta: &relaycommon.ChannelMeta{Routing: &modelrouting.Audit{
			PolicyID: 7, TargetID: 21, UpstreamModel: "provider-1080p",
		}},
	}

	require.NoError(t, ModelMappedHelper(c, info, request))
	assert.Equal(t, modelrouting.Seedance20, info.OriginModelName)
	assert.Equal(t, "provider-1080p", info.UpstreamModelName)
	assert.Equal(t, "provider-1080p", request.Model)
	assert.True(t, info.IsModelMapped)
}
