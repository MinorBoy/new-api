package clmmmall

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArkToClmmUsesOrdinaryDefaults(t *testing.T) {
	request := ArkRequest{
		Model:   "client-model",
		Content: []ArkContent{{Type: "text", Text: "a prompt"}},
	}

	converted, billingSeconds, err := ArkToClmm(request, "sh-video-basic")

	require.NoError(t, err)
	require.Equal(t, 5, billingSeconds)
	assert.JSONEq(t, `{"model":"sh-video-basic","prompt":"a prompt","aspect_ratio":"16:9","resolution":"480p","size":"1280x720","seconds":"5"}`, string(mustMarshalClmm(t, converted)))
}

func TestArkToClmmPreservesContentOrderAndDegradesImageRoles(t *testing.T) {
	duration := 8
	request := ArkRequest{
		Model:      "client-model",
		Ratio:      "9:16",
		Resolution: "720p",
		Duration:   &duration,
		Content: []ArkContent{
			{Type: "text", Text: "line one"},
			{Type: "image_url", Role: "first_frame", ImageURL: &ArkMedia{URL: "https://example.com/first.png"}},
			{Type: "image_url", Role: "last_frame", ImageURL: &ArkMedia{URL: "https://example.com/last.png"}},
			{Type: "image_url", Role: "reference_image", ImageURL: &ArkMedia{URL: "https://example.com/reference.png"}},
			{Type: "image_url", ImageURL: &ArkMedia{URL: "data:image/png;base64,AA=="}},
			{Type: "video_url", Role: "reference_video", VideoURL: &ArkMedia{URL: "https://example.com/reference.mp4"}},
			{Type: "text", Text: " line two "},
		},
	}

	converted, billingSeconds, err := ArkToClmm(request, "grok-video-pro")

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
			request := ArkRequest{
				Model:    "client-model",
				Duration: test.duration,
				Content:  []ArkContent{{Type: "text", Text: "bounded"}},
			}

			converted, billingSeconds, err := ArkToClmm(request, test.model)

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
			request := ArkRequest{
				Model:    "client-model",
				Duration: intPointer(2),
				Content:  []ArkContent{{Type: "text", Text: "fixed"}},
			}

			converted, billingSeconds, err := ArkToClmm(request, modelName)

			require.NoError(t, err)
			assert.Equal(t, 10, billingSeconds)
			assert.Equal(t, "1", converted.Seconds)
			assert.Equal(t, "10", converted.MySeconds)
		})
	}
}

func TestArkToClmmAppliesResolutionImageAndVideoControlSuffixes(t *testing.T) {
	request := ArkRequest{
		Model:      "client-model",
		Resolution: "720p",
		Content: []ArkContent{
			{Type: "text", Text: "controlled"},
			{Type: "image_url", Role: "reference_image", ImageURL: &ArkMedia{URL: "image-1"}},
			{Type: "image_url", Role: "reference_image", ImageURL: &ArkMedia{URL: "image-2"}},
			{Type: "video_url", Role: "reference_video", VideoURL: &ArkMedia{URL: "video-1"}},
		},
	}

	converted, _, err := ArkToClmm(request, "SH-video-sr-nsp-nyp-nyy-480P-2img-nv")

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
			_, _, err := ArkToClmm(ArkRequest{
				Model:   "client-model",
				Content: []ArkContent{{Type: "text", Text: "prompt"}},
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
			content := []ArkContent{{Type: "text", Text: "prompt"}}
			for i := 0; i < test.images; i++ {
				content = append(content, ArkContent{Type: "image_url", ImageURL: &ArkMedia{URL: fmt.Sprintf("image-%d", i)}})
			}
			_, _, err := ArkToClmm(ArkRequest{Model: "client-model", Duration: test.duration, Content: content}, test.model)
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
		mutate func(*ArkRequest)
	}{
		{name: "missing model", mutate: func(request *ArkRequest) { request.Model = "" }},
		{name: "missing content", mutate: func(request *ArkRequest) { request.Content = nil }},
		{name: "missing prompt", mutate: func(request *ArkRequest) {
			request.Content = []ArkContent{{Type: "image_url", ImageURL: &ArkMedia{URL: "image"}}}
		}},
		{name: "empty image url", mutate: func(request *ArkRequest) {
			request.Content = append(request.Content, ArkContent{Type: "image_url", ImageURL: &ArkMedia{}})
		}},
		{name: "empty video url", mutate: func(request *ArkRequest) {
			request.Content = append(request.Content, ArkContent{Type: "video_url", Role: "reference_video", VideoURL: &ArkMedia{}})
		}},
		{name: "invalid image role", mutate: func(request *ArkRequest) {
			request.Content = append(request.Content, ArkContent{Type: "image_url", Role: "mask", ImageURL: &ArkMedia{URL: "image"}})
		}},
		{name: "invalid video role", mutate: func(request *ArkRequest) {
			request.Content = append(request.Content, ArkContent{Type: "video_url", VideoURL: &ArkMedia{URL: "video"}})
		}},
		{name: "audio", mutate: func(request *ArkRequest) {
			request.Content = append(request.Content, ArkContent{Type: "audio_url", AudioURL: &ArkMedia{URL: "audio"}})
		}},
		{name: "draft task", mutate: func(request *ArkRequest) {
			request.Content = append(request.Content, ArkContent{Type: "draft_task", DraftTask: map[string]any{"id": "draft"}})
		}},
		{name: "unsupported content", mutate: func(request *ArkRequest) {
			request.Content = append(request.Content, ArkContent{Type: "input_file"})
		}},
		{name: "invalid ratio", mutate: func(request *ArkRequest) { request.Ratio = "1:1" }},
		{name: "invalid resolution", mutate: func(request *ArkRequest) { request.Resolution = "1080p" }},
		{name: "ordinary duration below minimum", mutate: func(request *ArkRequest) { request.Duration = intPointer(4) }},
		{name: "ordinary duration above maximum", mutate: func(request *ArkRequest) { request.Duration = intPointer(16) }},
		{name: "non-default service tier", mutate: func(request *ArkRequest) { request.ServiceTier = &priorityTier }},
		{name: "known unsupported field even false", mutate: func(request *ArkRequest) { request.Watermark = &falseValue }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := ArkRequest{Model: "client-model", Content: []ArkContent{{Type: "text", Text: "x"}}}
			test.mutate(&request)
			_, _, err := ArkToClmm(request, "sh-video")
			require.Error(t, err)
		})
	}

	_, _, err := ArkToClmm(ArkRequest{
		Model:       "client-model",
		ServiceTier: &defaultTier,
		Content:     []ArkContent{{Type: "text", Text: "x"}},
	}, "sh-video")
	require.NoError(t, err)
}

func TestArkToClmmEnforcesMediaLimits(t *testing.T) {
	images := []ArkContent{{Type: "text", Text: "prompt"}}
	for i := 0; i < 10; i++ {
		images = append(images, ArkContent{Type: "image_url", ImageURL: &ArkMedia{URL: fmt.Sprintf("image-%d", i)}})
	}
	_, _, err := ArkToClmm(ArkRequest{Model: "client-model", Content: images}, "sh-video")
	require.Error(t, err)

	videos := []ArkContent{{Type: "text", Text: "prompt"}}
	for i := 0; i < 4; i++ {
		videos = append(videos, ArkContent{Type: "video_url", Role: "reference_video", VideoURL: &ArkMedia{URL: fmt.Sprintf("video-%d", i)}})
	}
	_, _, err = ArkToClmm(ArkRequest{Model: "client-model", Content: videos}, "sh-video")
	require.Error(t, err)
}

func mustMarshalClmm(t *testing.T, request ClmmRequest) []byte {
	t.Helper()
	data, err := marshalClmmRequest(request)
	require.NoError(t, err)
	return data
}

func intPointer(value int) *int {
	return &value
}
