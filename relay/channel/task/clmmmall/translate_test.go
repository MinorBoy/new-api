package clmmmall

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArkToClmmUsesOrdinaryDefaults(t *testing.T) {
	request := arkRequest{
		Model:   "client-model",
		Content: []arkContent{{Type: "text", Text: "a prompt"}},
	}

	converted, billingSeconds, err := arkToClmm(request, "sh-video-basic")

	require.NoError(t, err)
	require.Equal(t, 5, billingSeconds)
	assert.JSONEq(t, `{"model":"sh-video-basic","prompt":"a prompt","aspect_ratio":"16:9","resolution":"480p","size":"1280x720","seconds":"5"}`, string(mustMarshalClmm(t, converted)))
}

func TestArkToClmmPreservesContentOrderAndDegradesImageRoles(t *testing.T) {
	duration := 8
	request := arkRequest{
		Model:      "client-model",
		Ratio:      "9:16",
		Resolution: "720p",
		Duration:   &duration,
		Content: []arkContent{
			{Type: "text", Text: "line one"},
			{Type: "image_url", Role: "first_frame", ImageURL: &arkMedia{URL: "https://example.com/first.png"}},
			{Type: "image_url", Role: "last_frame", ImageURL: &arkMedia{URL: "https://example.com/last.png"}},
			{Type: "image_url", Role: "reference_image", ImageURL: &arkMedia{URL: "https://example.com/reference.png"}},
			{Type: "image_url", ImageURL: &arkMedia{URL: "data:image/png;base64,AA=="}},
			{Type: "video_url", Role: "reference_video", VideoURL: &arkMedia{URL: "https://example.com/reference.mp4"}},
			{Type: "text", Text: " line two "},
		},
	}

	converted, billingSeconds, err := arkToClmm(request, "grok-video-pro")

	require.NoError(t, err)
	assert.Equal(t, 8, billingSeconds)
	assert.JSONEq(t, `{
		"model":"grok-video-pro",
		"prompt":"line one\nline two",
		"aspect_ratio":"9:16",
		"resolution":"720p",
		"size":"720x1280",
		"seconds":"8",
		"reference_image_urls":[
			"https://example.com/first.png",
			"https://example.com/last.png",
			"https://example.com/reference.png",
			"data:image/png;base64,AA=="
		],
		"reference_videos":["https://example.com/reference.mp4"]
	}`, string(mustMarshalClmm(t, converted)))
}

func TestArkToClmmUsesDurationLimitSuffix(t *testing.T) {
	tests := []struct {
		name            string
		model           string
		duration        *int
		expectedSeconds int
	}{
		{name: "suffix default", model: "me-videos-720P-10s", expectedSeconds: 10},
		{name: "explicit below ordinary minimum", model: "me-videos-10s", duration: intPointer(3), expectedSeconds: 3},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := arkRequest{
				Model:    "client-model",
				Duration: test.duration,
				Content:  []arkContent{{Type: "text", Text: "bounded"}},
			}

			converted, billingSeconds, err := arkToClmm(request, test.model)

			require.NoError(t, err)
			assert.Equal(t, test.expectedSeconds, billingSeconds)
			assert.Equal(t, "1", converted.Seconds)
			assert.Equal(t, fmt.Sprintf("%d", test.expectedSeconds), converted.MySeconds)
		})
	}
}

func TestArkToClmmUsesFixedDurationForEitherGzSuffixOrder(t *testing.T) {
	for _, modelName := range []string{"op-video-10s-gz", "op-video-gz-10s"} {
		t.Run(modelName, func(t *testing.T) {
			request := arkRequest{
				Model:    "client-model",
				Duration: intPointer(2),
				Content:  []arkContent{{Type: "text", Text: "fixed"}},
			}

			converted, billingSeconds, err := arkToClmm(request, modelName)

			require.NoError(t, err)
			assert.Equal(t, 10, billingSeconds)
			assert.Equal(t, "1", converted.Seconds)
			assert.Equal(t, "10", converted.MySeconds)
		})
	}
}

func TestArkToClmmAppliesResolutionImageAndVideoControlSuffixes(t *testing.T) {
	request := arkRequest{
		Model:      "client-model",
		Resolution: "720p",
		Content: []arkContent{
			{Type: "text", Text: "controlled"},
			{Type: "image_url", Role: "reference_image", ImageURL: &arkMedia{URL: "image-1"}},
			{Type: "image_url", Role: "reference_image", ImageURL: &arkMedia{URL: "image-2"}},
			{Type: "video_url", Role: "reference_video", VideoURL: &arkMedia{URL: "video-1"}},
		},
	}

	converted, _, err := arkToClmm(request, "SH-video-sr-nsp-nyp-nyy-480P-2img-nv")

	require.NoError(t, err)
	assert.Equal(t, "SH-video-sr-nsp-nyp-nyy-480P-2img-nv", converted.Model)
	assert.Equal(t, "480p", converted.Resolution)
	assert.Equal(t, []string{"image-1", "image-2"}, converted.ReferenceImageURLs)
	assert.Empty(t, converted.ReferenceVideos)
}

func TestArkToClmmAcceptsDocumentedChannelPrefixes(t *testing.T) {
	for _, modelName := range []string{
		"sh-model", "GROK-model", "veo-model", "bbv3-model", "bbv4-model",
		"me-model", "hj-model", "mowc-model", "op-model",
	} {
		t.Run(modelName, func(t *testing.T) {
			_, _, err := arkToClmm(arkRequest{
				Model:   "client-model",
				Content: []arkContent{{Type: "text", Text: "prompt"}},
			}, modelName)
			require.NoError(t, err)
		})
	}
}

func TestArkToClmmRejectsInvalidMappedModelControls(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		duration *int
		images   int
	}{
		{name: "unknown prefix", model: "unknown-video"},
		{name: "gz without duration suffix", model: "op-video-gz"},
		{name: "zero duration suffix", model: "sh-video-0s"},
		{name: "duration suffix above global bound", model: fmt.Sprintf("sh-video-%ds", common.MaxTaskDurationSeconds+1)},
		{name: "explicit duration above suffix", model: "sh-video-10s", duration: intPointer(11)},
		{name: "zero explicit bounded duration", model: "sh-video-10s", duration: intPointer(0)},
		{name: "insufficient reference images", model: "sh-video-3img", images: 2},
		{name: "zero image suffix", model: "sh-video-0img"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content := []arkContent{{Type: "text", Text: "prompt"}}
			for i := 0; i < test.images; i++ {
				content = append(content, arkContent{Type: "image_url", ImageURL: &arkMedia{URL: fmt.Sprintf("image-%d", i)}})
			}
			_, _, err := arkToClmm(arkRequest{Model: "client-model", Duration: test.duration, Content: content}, test.model)
			require.Error(t, err)
		})
	}
}

func TestArkToClmmRejectsUnsupportedArkInput(t *testing.T) {
	defaultTier := "default"
	priorityTier := "priority"
	falseValue := false
	tests := []struct {
		name   string
		mutate func(*arkRequest)
	}{
		{name: "missing model", mutate: func(request *arkRequest) { request.Model = "" }},
		{name: "missing content", mutate: func(request *arkRequest) { request.Content = nil }},
		{name: "missing prompt", mutate: func(request *arkRequest) {
			request.Content = []arkContent{{Type: "image_url", ImageURL: &arkMedia{URL: "image"}}}
		}},
		{name: "empty image url", mutate: func(request *arkRequest) {
			request.Content = append(request.Content, arkContent{Type: "image_url", ImageURL: &arkMedia{}})
		}},
		{name: "empty video url", mutate: func(request *arkRequest) {
			request.Content = append(request.Content, arkContent{Type: "video_url", Role: "reference_video", VideoURL: &arkMedia{}})
		}},
		{name: "invalid image role", mutate: func(request *arkRequest) {
			request.Content = append(request.Content, arkContent{Type: "image_url", Role: "mask", ImageURL: &arkMedia{URL: "image"}})
		}},
		{name: "invalid video role", mutate: func(request *arkRequest) {
			request.Content = append(request.Content, arkContent{Type: "video_url", VideoURL: &arkMedia{URL: "video"}})
		}},
		{name: "audio", mutate: func(request *arkRequest) {
			request.Content = append(request.Content, arkContent{Type: "audio_url", AudioURL: &arkMedia{URL: "audio"}})
		}},
		{name: "draft task", mutate: func(request *arkRequest) {
			request.Content = append(request.Content, arkContent{Type: "draft_task", DraftTask: map[string]any{"id": "draft"}})
		}},
		{name: "unsupported content", mutate: func(request *arkRequest) {
			request.Content = append(request.Content, arkContent{Type: "input_file"})
		}},
		{name: "invalid ratio", mutate: func(request *arkRequest) { request.Ratio = "1:1" }},
		{name: "invalid resolution", mutate: func(request *arkRequest) { request.Resolution = "1080p" }},
		{name: "ordinary duration below minimum", mutate: func(request *arkRequest) { request.Duration = intPointer(4) }},
		{name: "ordinary duration above maximum", mutate: func(request *arkRequest) { request.Duration = intPointer(16) }},
		{name: "non-default service tier", mutate: func(request *arkRequest) { request.ServiceTier = &priorityTier }},
		{name: "known unsupported field even false", mutate: func(request *arkRequest) { request.Watermark = &falseValue }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := arkRequest{Model: "client-model", Content: []arkContent{{Type: "text", Text: "x"}}}
			test.mutate(&request)
			_, _, err := arkToClmm(request, "sh-video")
			require.Error(t, err)
		})
	}

	_, _, err := arkToClmm(arkRequest{
		Model:       "client-model",
		ServiceTier: &defaultTier,
		Content:     []arkContent{{Type: "text", Text: "x"}},
	}, "sh-video")
	require.NoError(t, err)
}

func TestArkToClmmEnforcesMediaLimits(t *testing.T) {
	images := []arkContent{{Type: "text", Text: "prompt"}}
	for i := 0; i < 10; i++ {
		images = append(images, arkContent{Type: "image_url", ImageURL: &arkMedia{URL: fmt.Sprintf("image-%d", i)}})
	}
	_, _, err := arkToClmm(arkRequest{Model: "client-model", Content: images}, "sh-video")
	require.Error(t, err)

	videos := []arkContent{{Type: "text", Text: "prompt"}}
	for i := 0; i < 4; i++ {
		videos = append(videos, arkContent{Type: "video_url", Role: "reference_video", VideoURL: &arkMedia{URL: fmt.Sprintf("video-%d", i)}})
	}
	_, _, err = arkToClmm(arkRequest{Model: "client-model", Content: videos}, "sh-video")
	require.Error(t, err)
}

func TestArkToClmmAcceptsOrdinaryDurationAndMediaUpperBounds(t *testing.T) {
	content := []arkContent{{Type: "text", Text: "boundary prompt"}}
	for i := 0; i < 9; i++ {
		content = append(content, arkContent{Type: "image_url", ImageURL: &arkMedia{URL: fmt.Sprintf("image-%d", i)}})
	}
	for i := 0; i < 3; i++ {
		content = append(content, arkContent{Type: "video_url", Role: "reference_video", VideoURL: &arkMedia{URL: fmt.Sprintf("video-%d", i)}})
	}

	converted, billingSeconds, err := arkToClmm(arkRequest{
		Model:    "client-model",
		Duration: intPointer(15),
		Content:  content,
	}, "sh-video-9img")

	require.NoError(t, err)
	assert.Equal(t, 15, billingSeconds)
	assert.Equal(t, "15", converted.Seconds)
	assert.Len(t, converted.ReferenceImageURLs, 9)
	assert.Len(t, converted.ReferenceVideos, 3)
}

func TestArkToClmmFixedDurationExactOutput(t *testing.T) {
	converted, billingSeconds, err := arkToClmm(arkRequest{
		Model:    "client-model",
		Duration: intPointer(2),
		Content:  []arkContent{{Type: "text", Text: "fixed prompt"}},
	}, "op-video-gz-10s")

	require.NoError(t, err)
	assert.Equal(t, 10, billingSeconds)
	assert.JSONEq(t, `{"model":"op-video-gz-10s","prompt":"fixed prompt","aspect_ratio":"16:9","resolution":"480p","size":"1280x720","seconds":"1","mySeconds":"10"}`, string(mustMarshalClmm(t, converted)))
}

func mustMarshalClmm(t *testing.T, request clmmRequest) []byte {
	t.Helper()
	data, err := marshalClmmRequest(request)
	require.NoError(t, err)
	return data
}

func intPointer(value int) *int {
	return &value
}
