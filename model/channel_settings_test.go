package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/imageprofile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelValidateSettingsImageProfileBinding(t *testing.T) {
	binding := &imageprofile.Binding{
		Profile:        imageprofile.OpenAIImagesProfile,
		ProfileVersion: imageprofile.OpenAIImagesVersion,
	}
	tests := []struct {
		name        string
		channelType int
		wantErr     string
	}{
		{name: "openai compatible channel", channelType: constant.ChannelTypeOpenAI},
		{name: "gemini channel is not openai images compatible", channelType: constant.ChannelTypeGemini, wantErr: "OpenAI-compatible"},
		{name: "vertex channel is not openai images compatible", channelType: constant.ChannelTypeVertexAi, wantErr: "OpenAI-compatible"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &Channel{Type: tt.channelType}
			channel.SetOtherSettings(dto.ChannelOtherSettings{ImageProfile: binding})
			err := channel.ValidateSettings()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestChannelValidateSettingsImageProfileRejectsInvalidBinding(t *testing.T) {
	channel := &Channel{Type: constant.ChannelTypeOpenAI}
	channel.SetOtherSettings(dto.ChannelOtherSettings{ImageProfile: &imageprofile.Binding{
		Profile:        imageprofile.OpenAIImagesProfile,
		ProfileVersion: imageprofile.OpenAIImagesVersion,
		Paths: map[imageprofile.Endpoint]string{
			imageprofile.EndpointGenerations: "/v1/images/generations?key=secret",
		},
	}})
	err := channel.ValidateSettings()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "query")
}

func TestChannelValidateSettingsWithoutImageProfileKeepsLegacyBehavior(t *testing.T) {
	channel := &Channel{Type: constant.ChannelTypeGemini}
	channel.SetOtherSettings(dto.ChannelOtherSettings{})
	require.NoError(t, channel.ValidateSettings())
}

func TestAdvancedCustomChannelRequiresModelListRouteOnlyWhenUpdateChecksEnabled(t *testing.T) {
	inferenceRoute := dto.AdvancedCustomRoute{
		IncomingPath: "/v1/chat/completions",
		UpstreamPath: "/v1/chat/completions",
		Converter:    "none",
	}

	tests := []struct {
		name          string
		checksEnabled bool
		routes        []dto.AdvancedCustomRoute
		wantErr       string
	}{
		{
			name:   "legacy channel without discovery route remains valid",
			routes: []dto.AdvancedCustomRoute{inferenceRoute},
		},
		{
			name:          "enabled checks require discovery route",
			checksEnabled: true,
			routes:        []dto.AdvancedCustomRoute{inferenceRoute},
			wantErr:       dto.AdvancedCustomModelListPath,
		},
		{
			name:          "enabled checks accept discovery route",
			checksEnabled: true,
			routes: []dto.AdvancedCustomRoute{
				inferenceRoute,
				{
					IncomingPath: dto.AdvancedCustomModelListPath,
					UpstreamPath: dto.AdvancedCustomModelListPath,
					Converter:    "none",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &Channel{Type: constant.ChannelTypeAdvancedCustom}
			channel.SetOtherSettings(dto.ChannelOtherSettings{
				UpstreamModelUpdateCheckEnabled: tt.checksEnabled,
				AdvancedCustom: &dto.AdvancedCustomConfig{
					Routes: tt.routes,
				},
			})

			err := channel.ValidateSettings()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
