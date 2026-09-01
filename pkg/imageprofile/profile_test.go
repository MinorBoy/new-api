package imageprofile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIImagesProfileLookupProvidesStableContract(t *testing.T) {
	profile, ok := Lookup(OpenAIImagesProfile, OpenAIImagesVersion)
	require.True(t, ok)
	assert.Equal(t, OpenAIImagesProfile, profile.Name)
	assert.Equal(t, OpenAIImagesVersion, profile.Version)
	assert.Equal(t, "/v1/images/generations", profile.Paths[EndpointGenerations])
	assert.Equal(t, "/v1/images/edits", profile.Paths[EndpointEdits])
	assert.Contains(t, profile.Capabilities, EndpointGenerations)
	assert.Contains(t, profile.Capabilities, EndpointEdits)

	_, ok = Lookup(OpenAIImagesProfile, OpenAIImagesVersion+1)
	assert.False(t, ok)
}

func TestOpenAIImagesProfileBindingValidate(t *testing.T) {
	valid := Binding{
		Profile:        OpenAIImagesProfile,
		ProfileVersion: OpenAIImagesVersion,
		Paths: map[Endpoint]string{
			EndpointGenerations: "https://images.example/v1/images/generations",
			EndpointEdits:       "/v1/images/edits",
		},
		CapabilityOverrides: map[string]ModelCapabilities{
			"gpt-image-1": {
				Generations:     true,
				Sizes:           []string{"1024x1024"},
				Qualities:       []string{"medium"},
				ResponseFormats: []string{"url", "b64_json"},
				MaxN:            1,
				MaxInputImages:  2,
				SupportsMask:    true,
			},
		},
		Compatibility: map[string]Compatibility{
			"gpt-image-1:generations": {
				Status:         CompatibilityPassed,
				ProfileVersion: OpenAIImagesVersion,
				ContractHash:   "sha256:test",
				TestedAt:       1,
			},
		},
	}
	require.NoError(t, valid.Validate())
	assert.Equal(t, "https://images.example/v1/images/generations", valid.Path(EndpointGenerations))
	assert.Equal(t, "/v1/images/edits", valid.Path(EndpointEdits))

	tests := []struct {
		name    string
		binding Binding
		want    string
	}{
		{name: "unknown profile", binding: Binding{Profile: "custom", ProfileVersion: 1}, want: "profile"},
		{name: "query in path", binding: Binding{Profile: OpenAIImagesProfile, ProfileVersion: OpenAIImagesVersion, Paths: map[Endpoint]string{EndpointGenerations: "/v1/images/generations?key=secret"}}, want: "query"},
		{name: "userinfo in path", binding: Binding{Profile: OpenAIImagesProfile, ProfileVersion: OpenAIImagesVersion, Paths: map[Endpoint]string{EndpointGenerations: "https://user:pass@example.com/images"}}, want: "userinfo"},
		{name: "fragment in path", binding: Binding{Profile: OpenAIImagesProfile, ProfileVersion: OpenAIImagesVersion, Paths: map[Endpoint]string{EndpointGenerations: "https://example.com/images#fragment"}}, want: "fragment"},
		{name: "duplicate size", binding: Binding{Profile: OpenAIImagesProfile, ProfileVersion: OpenAIImagesVersion, CapabilityOverrides: map[string]ModelCapabilities{"gpt-image-1": {Sizes: []string{"1024x1024", "1024x1024"}, MaxN: 1}}}, want: "sizes"},
		{name: "invalid max n", binding: Binding{Profile: OpenAIImagesProfile, ProfileVersion: OpenAIImagesVersion, CapabilityOverrides: map[string]ModelCapabilities{"gpt-image-1": {MaxN: 129}}}, want: "max_n"},
		{name: "unknown compatibility status", binding: Binding{Profile: OpenAIImagesProfile, ProfileVersion: OpenAIImagesVersion, Compatibility: map[string]Compatibility{"gpt-image-1:generations": {Status: "unknown"}}}, want: "status"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.binding.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}
