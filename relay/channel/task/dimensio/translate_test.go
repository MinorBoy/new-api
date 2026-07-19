package dimensio

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArkToDimensioMultimodal(t *testing.T) {
	arkReq := ArkRequest{
		Model: "doubao-seedance-2-0-260128",
		Content: []ArkContent{
			{Type: "image_url", Role: "reference_image", ImageURL: &ArkMedia{URL: "https://x/img1.jpg"}},
			{Type: "image_url", Role: "reference_image", ImageURL: &ArkMedia{URL: "https://x/img2.jpg"}},
			{Type: "video_url", Role: "reference_video", VideoURL: &ArkMedia{URL: "https://x/ref1.mp4"}},
			{Type: "video_url", Role: "reference_video", VideoURL: &ArkMedia{URL: "https://x/ref2.mp4"}},
			{Type: "video_url", Role: "reference_video", VideoURL: &ArkMedia{URL: "https://x/ref3.mp4"}},
			{Type: "audio_url", Role: "reference_audio", AudioURL: &ArkMedia{URL: "https://x/bg.mp3"}},
			{Type: "text", Text: "镜头缓慢推进"},
		},
		Duration:   common.GetPointer(10),
		Resolution: "720p",
		Ratio:      "16:9",
	}

	dim, err := ArkToDimensio(arkReq)
	require.NoError(t, err)
	assert.Equal(t, arkReq.Model, dim.Model)
	assert.Equal(t, "镜头缓慢推进", dim.Prompt)
	require.NotNil(t, dim.Duration)
	assert.Equal(t, 10, *dim.Duration)
	assert.Equal(t, "720p", dim.Resolution)
	assert.Equal(t, "16:9", dim.Ratio)
	assert.Equal(t, "omni_reference", dim.FunctionMode)
	assert.Equal(t, "https://x/img1.jpg", dim.ImageFiles["image_file_1"])
	assert.Equal(t, "https://x/img2.jpg", dim.ImageFiles["image_file_2"])
	assert.Equal(t, "https://x/ref1.mp4", dim.VideoFiles["video_file_1"])
	assert.Equal(t, "https://x/ref2.mp4", dim.VideoFiles["video_file_2"])
	assert.Equal(t, "https://x/ref3.mp4", dim.VideoFiles["video_file_3"])
	assert.Equal(t, "https://x/bg.mp3", dim.AudioFiles["audio_file_1"])
	assert.Equal(t, []string{"https://x/img1.jpg", "https://x/img2.jpg"}, dim.FilePaths)
}

func TestDeriveFunctionModeMatrix(t *testing.T) {
	cases := []struct {
		name     string
		content  []ArkContent
		expected string
	}{
		{"text_only", []ArkContent{{Type: "text", Text: "x"}}, "first_last_frames"},
		{"single_image", []ArkContent{{Type: "image_url", Role: "first_frame"}}, "first_last_frames"},
		{"two_frames", []ArkContent{{Type: "image_url", Role: "first_frame"}, {Type: "image_url", Role: "last_frame"}}, "first_last_frames"},
		{"two_reference_images", []ArkContent{{Type: "image_url", Role: "reference_image"}, {Type: "image_url", Role: "reference_image"}}, "omni_reference"},
		{"image_plus_video", []ArkContent{{Type: "image_url", Role: "reference_image"}, {Type: "video_url", Role: "reference_video"}}, "omni_reference"},
		{"image_plus_audio", []ArkContent{{Type: "image_url", Role: "reference_image"}, {Type: "audio_url", Role: "reference_audio"}}, "omni_reference"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, deriveFunctionMode(tc.content))
		})
	}
}

func TestArkToDimensioRejectsMediaLimits(t *testing.T) {
	tests := []struct {
		name    string
		content []ArkContent
		message string
	}{
		{
			name: "too many images",
			content: func() []ArkContent {
				content := []ArkContent{{Type: "text", Text: "hi"}}
				for i := 0; i < 10; i++ {
					content = append(content, ArkContent{Type: "image_url", Role: "reference_image", ImageURL: &ArkMedia{URL: "img"}})
				}
				return content
			}(),
			message: "too many images",
		},
		{
			name: "too many videos",
			content: []ArkContent{
				{Type: "text", Text: "hi"},
				{Type: "video_url", Role: "reference_video", VideoURL: &ArkMedia{URL: "v1"}},
				{Type: "video_url", Role: "reference_video", VideoURL: &ArkMedia{URL: "v2"}},
				{Type: "video_url", Role: "reference_video", VideoURL: &ArkMedia{URL: "v3"}},
				{Type: "video_url", Role: "reference_video", VideoURL: &ArkMedia{URL: "v4"}},
			},
			message: "too many videos",
		},
		{
			name: "too many audios",
			content: []ArkContent{
				{Type: "text", Text: "hi"},
				{Type: "audio_url", Role: "reference_audio", AudioURL: &ArkMedia{URL: "a1"}},
				{Type: "audio_url", Role: "reference_audio", AudioURL: &ArkMedia{URL: "a2"}},
				{Type: "audio_url", Role: "reference_audio", AudioURL: &ArkMedia{URL: "a3"}},
				{Type: "audio_url", Role: "reference_audio", AudioURL: &ArkMedia{URL: "a4"}},
			},
			message: "too many audios",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ArkToDimensio(ArkRequest{Model: "m", Content: tc.content})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.message)
		})
	}
}

func TestArkToDimensioRejectsInvalidContent(t *testing.T) {
	tests := []struct {
		name    string
		content []ArkContent
		message string
	}{
		{"empty prompt", []ArkContent{{Type: "image_url", ImageURL: &ArkMedia{URL: "i"}}}, "prompt is required"},
		{"audio only", []ArkContent{{Type: "audio_url", Role: "reference_audio", AudioURL: &ArkMedia{URL: "a"}}, {Type: "text", Text: "x"}}, "audio input requires"},
		{"reference and frames", []ArkContent{{Type: "image_url", Role: "reference_image", ImageURL: &ArkMedia{URL: "i"}}, {Type: "image_url", Role: "first_frame", ImageURL: &ArkMedia{URL: "f"}}, {Type: "text", Text: "x"}}, "cannot mix"},
		{"invalid video role", []ArkContent{{Type: "video_url", Role: "first_frame", VideoURL: &ArkMedia{URL: "v"}}, {Type: "text", Text: "x"}}, "video role must be reference_video"},
		{"invalid audio role", []ArkContent{{Type: "audio_url", Role: "reference_video", AudioURL: &ArkMedia{URL: "a"}}, {Type: "text", Text: "x"}}, "audio role must be reference_audio"},
		{"missing image url", []ArkContent{{Type: "image_url"}, {Type: "text", Text: "x"}}, "image_url.url is required"},
		{"unknown content type", []ArkContent{{Type: "file", Text: "x"}}, "unsupported content type"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ArkToDimensio(ArkRequest{Model: "m", Content: tc.content})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.message)
		})
	}
}

func TestArkToDimensioRejectsUnsupportedFields(t *testing.T) {
	base := ArkRequest{Model: "m", Content: []ArkContent{{Type: "text", Text: "x"}}}
	tests := []struct {
		name string
		set  func(*ArkRequest)
	}{
		{"seed", func(r *ArkRequest) { r.Seed = common.GetPointer(1) }},
		{"camera_fixed", func(r *ArkRequest) { r.CameraFixed = common.GetPointer(true) }},
		{"watermark", func(r *ArkRequest) { r.Watermark = common.GetPointer(true) }},
		{"generate_audio", func(r *ArkRequest) { r.GenerateAudio = common.GetPointer(true) }},
		{"frames", func(r *ArkRequest) { r.Frames = common.GetPointer(1) }},
		{"draft", func(r *ArkRequest) { r.Draft = common.GetPointer(true) }},
		{"priority", func(r *ArkRequest) { r.Priority = common.GetPointer(1) }},
		{"execution_expires_after", func(r *ArkRequest) { r.ExecutionExpiresAfter = common.GetPointer(1) }},
		{"return_last_frame", func(r *ArkRequest) { r.ReturnLastFrame = common.GetPointer(true) }},
		{"safety_identifier", func(r *ArkRequest) { r.SafetyIdentifier = common.GetPointer("x") }},
		{"tools", func(r *ArkRequest) {
			r.Tools = &[]struct {
				Type string `json:"type,omitempty"`
			}{{Type: "x"}}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			tc.set(&r)
			_, err := ArkToDimensio(r)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unsupported field")
		})
	}
}

func TestDimensioToArkTaskResponse(t *testing.T) {
	cases := []struct {
		name           string
		dimensioStatus string
		arkStatus      string
		resultURL      string
	}{
		{"pending", "pending", "queued", ""},
		{"processing", "processing", "running", ""},
		{"completed", "completed", "succeeded", "https://x/v.mp4"},
		{"failed", "failed", "failed", ""},
		{"not_found", "not_found", "failed", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dim := DimensioTaskResponse{
				TaskID: "dim-123", Status: tc.dimensioStatus, Progress: 50,
				Result: DimensioResult{URL: tc.resultURL}, Error: "审核不通过", ErrorCode: "2043",
			}
			ark, err := DimensioToArkTask(dim, "task_public", "doubao-seedance-2-0-260128", 1710000000, 1710000100)
			require.NoError(t, err)
			assert.Equal(t, "task_public", ark.ID)
			assert.NotEqual(t, dim.TaskID, ark.ID)
			assert.Equal(t, tc.arkStatus, ark.Status)
			assert.Equal(t, "doubao-seedance-2-0-260128", ark.Model)
			assert.Equal(t, int64(1710000000), ark.CreatedAt)
			assert.Equal(t, int64(1710000100), ark.UpdatedAt)
			assert.Equal(t, tc.resultURL, ark.Content.VideoURL)
			if tc.dimensioStatus == "failed" || tc.dimensioStatus == "not_found" {
				require.NotNil(t, ark.Error)
				assert.Equal(t, "2043", ark.Error.Code)
			}
		})
	}
}

func TestDimensioToArkTaskResponseDoesNotLeakProviderFields(t *testing.T) {
	ark, err := DimensioToArkTask(DimensioTaskResponse{
		TaskID: "dim-upstream", Status: "completed", Progress: 87,
	}, "task_public", "m", 1, 2)
	require.NoError(t, err)
	assert.Equal(t, "task_public", ark.ID)
	assert.Empty(t, ark.Content.VideoURL)
	assert.Empty(t, ark.Usage.CompletionTokens)

	_, err = DimensioToArkTask(DimensioTaskResponse{Status: "bogus"}, "task_public", "m", 1, 2)
	require.Error(t, err)
}

func TestDimensioToArkTaskFailedUsesStableMessage(t *testing.T) {
	ark, err := DimensioToArkTask(DimensioTaskResponse{Status: "failed", ErrorCode: "-2000"}, "task_public", "m", 1, 2)
	require.NoError(t, err)
	require.NotNil(t, ark.Error)
	assert.Equal(t, "task failed", ark.Error.Message)
}

func TestMarshalDimensioRequestExpandsMedia(t *testing.T) {
	duration := 5
	data, err := MarshalDimensioRequest(DimensioRequest{
		Model: "m", Prompt: "p", FunctionMode: "omni_reference", Duration: &duration,
		ImageFiles: map[string]string{"image_file_1": "i"},
		VideoFiles: map[string]string{"video_file_1": "v"},
		AudioFiles: map[string]string{"audio_file_1": "a"},
		FilePaths:  []string{"i"},
	})
	require.NoError(t, err)
	var payload map[string]interface{}
	require.NoError(t, common.Unmarshal(data, &payload))
	assert.Equal(t, "i", payload["image_file_1"])
	assert.Equal(t, "v", payload["video_file_1"])
	assert.Equal(t, "a", payload["audio_file_1"])
	assert.NotContains(t, payload, "file_paths")
	assert.Equal(t, "omni_reference", payload["functionMode"])
	assert.Equal(t, "m", payload["model"])
	assert.Equal(t, "p", payload["prompt"])
	assert.Equal(t, float64(5), payload["duration"])
}
