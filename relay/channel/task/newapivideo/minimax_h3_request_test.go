package newapivideo

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiniMaxH3RequestUsesProviderDefaults(t *testing.T) {
	request, err := parseARKRequest([]byte(`{"model":"minimax-h3","content":[{"type":"text","text":"cinematic"}]}`), minimaxH3ProtocolProfile())
	require.NoError(t, err)
	body, err := buildMiniMaxH3Request(request, modelrouting.MiniMaxH3VIP)
	require.NoError(t, err)
	// The upstream gateway needs an explicit mode to pick the generation path;
	// text-only content maps to mode=text2video with the provider defaults.
	assert.JSONEq(t, `{"model":"minimax-h3-vip","prompt":"cinematic","mode":"text2video","duration":15,"aspect_ratio":"16:9","resolution":"2k"}`, string(body))
}

// Reference media must map onto mode=reference2video, first/last frames onto
// mode=frames2video, single image onto image2video; otherwise the UniArt
// gateway rejects the submission without a recognized mode.
func TestMiniMaxH3RequestDerivesUpstreamMode(t *testing.T) {
	tests := []struct {
		name    string
		content string
		mode    string
	}{
		{
			name:    "reference image",
			content: `{"type":"text","text":"turn"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/a.png"}}`,
			mode:    "reference2video",
		},
		{
			name:    "reference video and audio",
			content: `{"type":"text","text":"dance"},{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.4.4/a.mp4"}},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://1.1.1.1/a.wav"}}`,
			mode:    "reference2video",
		},
		{
			name:    "first frame",
			content: `{"type":"text","text":"move"},{"type":"image_url","role":"first_frame","image_url":{"url":"https://8.8.8.8/f.png"}}`,
			mode:    "frames2video",
		},
		{
			name:    "first and last frames",
			content: `{"type":"text","text":"morph"},{"type":"image_url","role":"first_frame","image_url":{"url":"https://8.8.8.8/f.png"}},{"type":"image_url","role":"last_frame","image_url":{"url":"https://8.8.4.4/l.png"}}`,
			mode:    "frames2video",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := parseARKRequest([]byte(`{"model":"minimax-h3","content":[`+test.content+`]}`), minimaxH3ProtocolProfile())
			require.NoError(t, err)
			body, err := buildMiniMaxH3Request(request, "vendor-h3")
			require.NoError(t, err)
			assert.Contains(t, string(body), `"mode":"`+test.mode+`"`)
		})
	}
}

func TestMiniMaxH3RequestPreservesReferencesAndRejectsBounds(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"minimax-h3",
		"content":[
			{"type":"text","text":"reference"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.png"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.4.4/ref.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://1.1.1.1/ref.wav"}}
		],
		"duration":4,"ratio":"auto","resolution":"720p"
	}`), minimaxH3ProtocolProfile())
	require.NoError(t, err)
	body, err := buildMiniMaxH3Request(request, modelrouting.MiniMaxH3VIP)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"minimax-h3-vip","prompt":"reference","mode":"reference2video","duration":4,"aspect_ratio":"auto","resolution":"720p","images":["https://8.8.8.8/ref.png"],"videos":["https://8.8.4.4/ref.mp4"],"audios":["https://1.1.1.1/ref.wav"]}`, string(body))

	request.Duration = intPointer(3)
	err = validateMiniMaxH3Request(request, modelrouting.MiniMaxH3VIP)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duration")
}

func intPointer(value int) *int {
	return &value
}
