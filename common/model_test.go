package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
)

func TestImage25ModelsUseImageGenerationEndpoint(t *testing.T) {
	for _, model := range []string{"gpt-image-2.5-flare", "gpt-image-2.5-sunburst"} {
		assert.True(t, IsImageGenerationModel(model))
		assert.Equal(t, constant.EndpointTypeImageGeneration, GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, model)[0])
	}
}
