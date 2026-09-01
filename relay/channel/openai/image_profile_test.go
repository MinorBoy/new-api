package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/imageprofile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRequestURLUsesImageProfilePathOverride(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ChannelBaseUrl: "https://provider.example",
		},
	}
	info.ChannelOtherSettings.ImageProfile = &imageprofile.Binding{
		Profile:        imageprofile.OpenAIImagesProfile,
		ProfileVersion: imageprofile.OpenAIImagesVersion,
		Paths:          map[imageprofile.Endpoint]string{imageprofile.EndpointGenerations: "/custom/generations"},
	}
	url, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://provider.example/custom/generations", url)

	info.ChannelOtherSettings.ImageProfile.Paths[imageprofile.EndpointGenerations] = "https://images.example/v1/images/generations"
	url, err = adaptor.GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://images.example/v1/images/generations", url)
}
