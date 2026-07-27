package newapivideo

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/videometa"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecureProfiles(t *testing.T) {
	tests := []struct {
		group      dto.SecureVideoGroup
		submitPath string
		pollPath   string
		dialect    videoRequestDialect
	}{
		{dto.SecureVideoGroupDiscount, "/api/generate-video", "/api/task/{task_id}", videoRequestDialectSecureDiscount},
		{dto.SecureVideoGroupOverseas, "/api/generate-video", "/api/task/{task_id}", videoRequestDialectSecureOverseas},
		{dto.SecureVideoGroupEnterprise, "/v1/videos", "/v1/videos/{task_id}", videoRequestDialectSecureEnterprise},
	}
	for _, tt := range tests {
		t.Run(string(tt.group), func(t *testing.T) {
			profile, err := secureProtocolProfile(tt.group)
			require.NoError(t, err)
			assert.Equal(t, "Secure", profile.channelName)
			assert.Equal(t, tt.submitPath, profile.submitPath)
			assert.Equal(t, tt.pollPath, profile.pollPath)
			assert.Equal(t, tt.dialect, profile.requestDialect)
			require.NotNil(t, profile.secureRequest)
			assert.Equal(t, tt.group, profile.secureRequest.group)
		})
	}
	_, err := secureProtocolProfile("unknown")
	require.Error(t, err)

	adaptor := NewSecureTaskAdaptor()
	expected := []string{"video-2.0-fast", "video-2.0-mini", "video-2.0-pro"}
	models := adaptor.GetModelList()
	assert.Equal(t, expected, models)
	models[0] = "changed"
	assert.Equal(t, expected, adaptor.GetModelList())
}

func TestSecureInitSelectsTransportProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/task/upstream%2Ftask", r.URL.EscapedPath())
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	adaptor := NewSecureTaskAdaptor()
	info := secureRelayInfo(dto.SecureVideoGroupDiscount, "video-2.0-pro")
	info.ChannelBaseUrl = server.URL
	info.ApiKey = "key"
	adaptor.Init(info)
	requestURL, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, server.URL+"/api/generate-video", requestURL)
	response, err := adaptor.FetchTask(server.URL, "key", map[string]any{"task_id": "upstream/task"}, "")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())

	info.ChannelOtherSettings.SecureVideoGroup = "unknown"
	adaptor.Init(info)
	_, err = adaptor.BuildRequestURL(info)
	assert.ErrorContains(t, err, "invalid secure_video_group")
}

func TestSecureBuildDiscountRequest(t *testing.T) {
	body := `{
		"model":"client","content":[
			{"type":"text","text":"product close-up"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/a.jpg"}},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/b.jpg"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/ref.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/ref.mp3"}}
		],"duration":8,"ratio":"16:9","resolution":"720p"
	}`
	adaptor, c, info := secureValidationContext(t, dto.SecureVideoGroupDiscount, "video-2.0-pro", body)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	requestBody, contentType := secureBuildBody(t, adaptor, c, info)
	assert.Equal(t, [][2]string{
		{"model", "video-2.0-pro"},
		{"prompt", "product close-up"},
		{"duration", "8"},
		{"ratio", "16:9"},
		{"resolution", "720p"},
		{"files", "https://8.8.8.8/a.jpg"},
		{"files", "https://8.8.8.8/b.jpg"},
		{"video_urls", "https://8.8.8.8/ref.mp4"},
		{"audio_urls", "https://8.8.8.8/ref.mp3"},
	}, readSecureMultipart(t, contentType, requestBody))
}

func TestSecureBuildOverseasOmniRequest(t *testing.T) {
	service.SetVideoMetadataClient(secureVideoMetadataClient{durations: map[string]int64{"https://8.8.8.8/ref.mp4": 1000}})
	t.Cleanup(func() { service.SetVideoMetadataClient(nil) })
	body := `{
		"model":"client","content":[
			{"type":"text","text":"edit to the video rhythm"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/a.jpg"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/ref.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/ref.mp3"}}
		],"duration":8,"ratio":"16:9","resolution":"720p"
	}`
	adaptor, c, info := secureValidationContext(t, dto.SecureVideoGroupOverseas, "video-2.0-pro", body)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	requestBody, contentType := secureBuildBody(t, adaptor, c, info)
	assert.Equal(t, [][2]string{
		{"model", "video-2.0-pro"},
		{"prompt", "edit to the video rhythm @image_file_1 @video_file_1 @audio_file_1"},
		{"duration", "8"},
		{"ratio", "16:9"},
		{"resolution", "720p"},
		{"functionMode", "omni_reference"},
		{"image_file_1", "https://8.8.8.8/a.jpg"},
		{"video_file_1", "https://8.8.8.8/ref.mp4"},
		{"audio_file_1", "https://8.8.8.8/ref.mp3"},
	}, readSecureMultipart(t, contentType, requestBody))
}

func TestSecureOverseasDoesNotDuplicatePromptReferences(t *testing.T) {
	service.SetVideoMetadataClient(secureVideoMetadataClient{durations: map[string]int64{"https://8.8.8.8/ref.mp4": 1000}})
	t.Cleanup(func() { service.SetVideoMetadataClient(nil) })
	body := `{
		"model":"client","content":[
			{"type":"text","text":"use @video_file_1 timing"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/a.jpg"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/ref.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/ref.mp3"}}
		],"duration":8,"ratio":"16:9","resolution":"720p"
	}`
	adaptor, c, info := secureValidationContext(t, dto.SecureVideoGroupOverseas, "video-2.0-pro", body)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	requestBody, contentType := secureBuildBody(t, adaptor, c, info)
	fields := readSecureMultipart(t, contentType, requestBody)
	require.GreaterOrEqual(t, len(fields), 2)
	prompt := fields[1][1]
	assert.Equal(t, "use @video_file_1 timing @image_file_1 @audio_file_1", prompt)
	assert.Equal(t, 1, strings.Count(prompt, "@video_file_1"))
}

func TestSecureBuildOverseasFirstLastRequest(t *testing.T) {
	body := `{
		"model":"client","content":[
			{"type":"text","text":"transition"},
			{"type":"image_url","role":"first_frame","image_url":{"url":"https://8.8.8.8/first.jpg"}},
			{"type":"image_url","role":"last_frame","image_url":{"url":"https://8.8.8.8/last.jpg"}}
		],"duration":8,"ratio":"16:9","resolution":"720p"
	}`
	adaptor, c, info := secureValidationContext(t, dto.SecureVideoGroupOverseas, "video-2.0-pro", body)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	requestBody, contentType := secureBuildBody(t, adaptor, c, info)
	fields := readSecureMultipart(t, contentType, requestBody)
	assert.Contains(t, fields, [2]string{"functionMode", "first_last_frames"})
	assert.Contains(t, fields, [2]string{"image_file_1", "https://8.8.8.8/first.jpg"})
	assert.Contains(t, fields, [2]string{"image_file_2", "https://8.8.8.8/last.jpg"})
	assert.Equal(t, [2]string{"prompt", "transition"}, fields[1])
}

func TestSecureBuildEnterpriseRequest(t *testing.T) {
	body := `{
		"model":"client","content":[
			{"type":"text","text":"enterprise multimodal"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/main.jpg"}},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/extra.jpg"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/ref.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/ref.mp3"}}
		],"duration":8,"ratio":"16:9","resolution":"720p"
	}`
	adaptor, c, info := secureValidationContext(t, dto.SecureVideoGroupEnterprise, "video-2.0-pro", body)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	requestBody, contentType := secureBuildBody(t, adaptor, c, info)
	assert.Equal(t, "application/json", contentType)
	assert.JSONEq(t, `{
		"model":"video-2.0-pro","prompt":"enterprise multimodal","duration":8,
		"aspect_ratio":"16:9","image_url":"https://8.8.8.8/main.jpg",
		"extra_images":["https://8.8.8.8/extra.jpg"],
		"extra_videos":["https://8.8.8.8/ref.mp4"],
		"extra_audios":["https://8.8.8.8/ref.mp3"]
	}`, string(requestBody))
	for _, field := range []string{`"ratio"`, `"resolution"`, `"files"`, `"functionMode"`} {
		assert.NotContains(t, string(requestBody), field)
	}
}

func TestSecureRequestCapabilityMatrix(t *testing.T) {
	images10 := strings.Repeat(`,{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/a.jpg"}}`, 10)
	videos4 := strings.Repeat(`,{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/a.mp4"}}`, 4)
	audios4 := strings.Repeat(`,{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/a.mp3"}}`, 4)
	images9 := strings.Repeat(`,{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/a.jpg"}}`, 9)
	videos3 := strings.Repeat(`,{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/a.mp4"}}`, 3)
	tests := []struct {
		name          string
		group         dto.SecureVideoGroup
		upstreamModel string
		body          string
		code          string
	}{
		{name: "discount text only", group: dto.SecureVideoGroupDiscount, upstreamModel: "video-2.0-pro", body: `{"model":"m","content":[{"type":"text","text":"text"}],"duration":8,"ratio":"16:9","resolution":"720p"}`, code: "InvalidParameter.content"},
		{name: "discount last frame", group: dto.SecureVideoGroupDiscount, upstreamModel: "video-2.0-pro", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"first_frame","image_url":{"url":"https://8.8.8.8/a.jpg"}},{"type":"image_url","role":"last_frame","image_url":{"url":"https://8.8.8.8/b.jpg"}}],"duration":8,"ratio":"16:9","resolution":"720p"}`, code: "InvalidParameter.content"},
		{name: "discount media streams", group: dto.SecureVideoGroupDiscount, upstreamModel: "video-2.0-pro", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/a.jpg"}},{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/a.mp4"}},{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/b.mp4"}},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/a.mp3"}},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/b.mp3"}}],"duration":8,"ratio":"16:9","resolution":"720p"}`, code: "InvalidParameter.content"},
		{name: "discount short", group: dto.SecureVideoGroupDiscount, upstreamModel: "video-2.0-pro", body: secureImageBody(3, "16:9", "720p"), code: "InvalidParameter.duration"},
		{name: "discount long", group: dto.SecureVideoGroupDiscount, upstreamModel: "video-2.0-pro", body: secureImageBody(16, "16:9", "720p"), code: "InvalidParameter.duration"},
		{name: "discount ratio", group: dto.SecureVideoGroupDiscount, upstreamModel: "video-2.0-pro", body: secureImageBody(8, "1:1", "720p"), code: "InvalidParameter.ratio"},
		{name: "discount fast resolution", group: dto.SecureVideoGroupDiscount, upstreamModel: "video-2.0-fast", body: secureImageBody(8, "16:9", "1080p"), code: "InvalidParameter.resolution"},
		{name: "discount pro resolution", group: dto.SecureVideoGroupDiscount, upstreamModel: "video-2.0-pro", body: secureImageBody(8, "16:9", "480p"), code: "InvalidParameter.resolution"},
		{name: "overseas images", group: dto.SecureVideoGroupOverseas, upstreamModel: "video-2.0-pro", body: `{"model":"m","content":[{"type":"text","text":"text"}` + images10 + `],"duration":8,"ratio":"16:9","resolution":"720p"}`, code: "InvalidParameter.content"},
		{name: "overseas videos", group: dto.SecureVideoGroupOverseas, upstreamModel: "video-2.0-pro", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/a.jpg"}}` + videos4 + `],"duration":8,"ratio":"16:9","resolution":"720p"}`, code: "InvalidParameter.content"},
		{name: "overseas audios", group: dto.SecureVideoGroupOverseas, upstreamModel: "video-2.0-pro", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/a.jpg"}}` + audios4 + `],"duration":8,"ratio":"16:9","resolution":"720p"}`, code: "InvalidParameter.content"},
		{name: "overseas total media", group: dto.SecureVideoGroupOverseas, upstreamModel: "video-2.0-pro", body: `{"model":"m","content":[{"type":"text","text":"text"}` + images9 + videos3 + `,{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.8/a.mp3"}}],"duration":8,"ratio":"16:9","resolution":"720p"}`, code: "InvalidParameter.content"},
		{name: "overseas first frame mixed with reference video", group: dto.SecureVideoGroupOverseas, upstreamModel: "video-2.0-pro", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"first_frame","image_url":{"url":"https://8.8.8.8/a.jpg"}},{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/a.mp4"}}],"duration":8,"ratio":"16:9","resolution":"720p"}`, code: "InvalidParameter.content"},
		{name: "overseas strict mixed media", group: dto.SecureVideoGroupOverseas, upstreamModel: "video-2.0-pro", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"first_frame","image_url":{"url":"https://8.8.8.8/a.jpg"}},{"type":"image_url","role":"last_frame","image_url":{"url":"https://8.8.8.8/b.jpg"}},{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/a.mp4"}}],"duration":8,"ratio":"16:9","resolution":"720p"}`, code: "InvalidParameter.content"},
		{name: "overseas short", group: dto.SecureVideoGroupOverseas, upstreamModel: "video-2.0-pro", body: secureTextBody(3, "16:9", "720p"), code: "InvalidParameter.duration"},
		{name: "overseas long", group: dto.SecureVideoGroupOverseas, upstreamModel: "video-2.0-pro", body: secureTextBody(16, "16:9", "720p"), code: "InvalidParameter.duration"},
		{name: "overseas ratio", group: dto.SecureVideoGroupOverseas, upstreamModel: "video-2.0-pro", body: secureTextBody(8, "2:1", "720p"), code: "InvalidParameter.ratio"},
		{name: "overseas fast resolution", group: dto.SecureVideoGroupOverseas, upstreamModel: "video-2.0-fast", body: secureTextBody(8, "16:9", "1080p"), code: "InvalidParameter.resolution"},
		{name: "overseas 4k", group: dto.SecureVideoGroupOverseas, upstreamModel: "video-2.0-pro", body: secureTextBody(8, "16:9", "4k"), code: "InvalidParameter.resolution"},
		{name: "enterprise model", group: dto.SecureVideoGroupEnterprise, upstreamModel: "video-2.0-fast", body: secureTextBody(8, "16:9", "720p"), code: "InvalidParameter.model"},
		{name: "enterprise resolution", group: dto.SecureVideoGroupEnterprise, upstreamModel: "video-2.0-pro", body: secureTextBody(8, "16:9", "1080p"), code: "InvalidParameter.resolution"},
		{name: "enterprise duration required", group: dto.SecureVideoGroupEnterprise, upstreamModel: "video-2.0-pro", body: `{"model":"m","content":[{"type":"text","text":"text"}],"ratio":"16:9","resolution":"720p"}`, code: "MissingParameter.duration"},
		{name: "enterprise short", group: dto.SecureVideoGroupEnterprise, upstreamModel: "video-2.0-pro", body: secureTextBody(4, "16:9", "720p"), code: "InvalidParameter.duration"},
		{name: "enterprise long", group: dto.SecureVideoGroupEnterprise, upstreamModel: "video-2.0-pro", body: secureTextBody(16, "16:9", "720p"), code: "InvalidParameter.duration"},
		{name: "enterprise last frame", group: dto.SecureVideoGroupEnterprise, upstreamModel: "video-2.0-pro", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"first_frame","image_url":{"url":"https://8.8.8.8/a.jpg"}},{"type":"image_url","role":"last_frame","image_url":{"url":"https://8.8.8.8/b.jpg"}}],"duration":8,"ratio":"16:9","resolution":"720p"}`, code: "InvalidParameter.content"},
		{name: "watermark", group: dto.SecureVideoGroupEnterprise, upstreamModel: "video-2.0-pro", body: `{"model":"m","content":[{"type":"text","text":"text"}],"duration":8,"watermark":false}`, code: "InvalidParameter.watermark"},
		{name: "generate audio", group: dto.SecureVideoGroupEnterprise, upstreamModel: "video-2.0-pro", body: `{"model":"m","content":[{"type":"text","text":"text"}],"duration":8,"generate_audio":false}`, code: "InvalidParameter.generate_audio"},
		{name: "service tier", group: dto.SecureVideoGroupEnterprise, upstreamModel: "video-2.0-pro", body: `{"model":"m","content":[{"type":"text","text":"text"}],"duration":8,"service_tier":"flex"}`, code: "InvalidParameter.service_tier"},
		{name: "draft", group: dto.SecureVideoGroupEnterprise, upstreamModel: "video-2.0-pro", body: `{"model":"m","content":[{"type":"text","text":"text"}],"duration":8,"draft":false}`, code: "InvalidParameter.draft"},
		{name: "tools", group: dto.SecureVideoGroupEnterprise, upstreamModel: "video-2.0-pro", body: `{"model":"m","content":[{"type":"text","text":"text"}],"duration":8,"tools":[{"type":"camera_fixed"}]}`, code: "InvalidParameter.tools"},
		{name: "embedded data", group: dto.SecureVideoGroupDiscount, upstreamModel: "video-2.0-pro", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"reference_image","image_url":{"url":"data:image/png;base64,QUJDRA=="}}],"duration":8,"ratio":"16:9","resolution":"720p"}`, code: "InvalidParameter.content"},
		{name: "embedded asset", group: dto.SecureVideoGroupDiscount, upstreamModel: "video-2.0-pro", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"reference_image","image_url":{"url":"asset://image-1"}}],"duration":8,"ratio":"16:9","resolution":"720p"}`, code: "InvalidParameter.content"},
		{name: "loopback media", group: dto.SecureVideoGroupDiscount, upstreamModel: "video-2.0-pro", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"reference_image","image_url":{"url":"http://127.0.0.1/a.jpg"}}],"duration":8,"ratio":"16:9","resolution":"720p"}`, code: "InvalidParameter.content"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, err := secureProtocolProfile(tt.group)
			require.NoError(t, err)
			request, err := parseARKRequest([]byte(tt.body), profile)
			if err == nil {
				err = validateSecureRequest(request, *profile.secureRequest, tt.upstreamModel)
			}
			var requestErr *arkRequestError
			require.ErrorAs(t, err, &requestErr)
			assert.Equal(t, tt.code, requestErr.Code)
		})
	}
}

func TestSecureValidationAndBillingHooksRejectBeforeBuild(t *testing.T) {
	tests := []struct {
		name          string
		group         dto.SecureVideoGroup
		upstreamModel string
		body          string
		billing       bool
		code          string
	}{
		{name: "discount input", group: dto.SecureVideoGroupDiscount, upstreamModel: "video-2.0-pro", body: secureTextBody(8, "16:9", "720p"), code: "InvalidParameter.content"},
		{name: "enterprise duration", group: dto.SecureVideoGroupEnterprise, upstreamModel: "video-2.0-pro", body: `{"model":"m","content":[{"type":"text","text":"text"}],"ratio":"16:9","resolution":"720p"}`, code: "MissingParameter.duration"},
		{name: "discount mapped resolution", group: dto.SecureVideoGroupDiscount, upstreamModel: "video-2.0-fast", body: secureImageBody(8, "16:9", "1080p"), billing: true, code: "InvalidParameter.resolution"},
		{name: "overseas mapped resolution", group: dto.SecureVideoGroupOverseas, upstreamModel: "video-2.0-mini", body: secureTextBody(8, "16:9", "1080p"), billing: true, code: "InvalidParameter.resolution"},
		{name: "enterprise mapped model", group: dto.SecureVideoGroupEnterprise, upstreamModel: "video-2.0-fast", body: secureTextBody(8, "16:9", "720p"), billing: true, code: "InvalidParameter.model"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adaptor, c, info := secureValidationContext(t, tt.group, tt.upstreamModel, tt.body)
			taskErr := adaptor.ValidateRequestAndSetAction(c, info)
			if tt.billing {
				require.Nil(t, taskErr)
				taskErr = adaptor.ValidateBillingRequest(c, info)
			}
			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			assert.Equal(t, tt.code, taskErr.Code)
			_, buildErr := adaptor.BuildRequestBody(c, info)
			assert.Error(t, buildErr)
		})
	}
}

func TestSecureOverseasReferenceVideoDuration(t *testing.T) {
	tests := []struct {
		name       string
		durations  map[string]int64
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "exact maximum", durations: map[string]int64{"https://8.8.8.8/a.mp4": 9000, "https://8.8.8.8/b.mp4": 6000}},
		{name: "above maximum", durations: map[string]int64{"https://8.8.8.8/a.mp4": 9000, "https://8.8.8.8/b.mp4": 6001}, wantStatus: http.StatusBadRequest, wantCode: "InvalidParameter.content"},
		{name: "invalid media", err: &service.VideoMetadataError{Kind: service.VideoMetadataInvalidMedia}, wantStatus: http.StatusBadRequest, wantCode: "InvalidParameter.content"},
		{name: "metadata unavailable", err: &service.VideoMetadataError{Kind: service.VideoMetadataUnavailable}, wantStatus: http.StatusServiceUnavailable, wantCode: "reference_video_metadata_unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			service.SetVideoMetadataClient(secureVideoMetadataClient{durations: tt.durations, err: tt.err, calls: &calls})
			t.Cleanup(func() { service.SetVideoMetadataClient(nil) })
			body := `{"model":"m","content":[
				{"type":"text","text":"text"},
				{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/a.jpg"}},
				{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/a.mp4"}},
				{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.8/b.mp4"}}
			],"duration":8,"ratio":"16:9","resolution":"720p"}`
			adaptor, c, info := secureValidationContext(t, dto.SecureVideoGroupOverseas, "video-2.0-pro", body)
			taskErr := adaptor.ValidateRequestAndSetAction(c, info)
			if tt.wantCode == "" {
				require.Nil(t, taskErr)
				assert.Equal(t, int32(2), calls.Load())
				return
			}
			require.NotNil(t, taskErr)
			assert.Equal(t, tt.wantStatus, taskErr.StatusCode)
			assert.Equal(t, tt.wantCode, taskErr.Code)
			assert.Empty(t, adaptor.requestContentType)
		})
	}
}

func TestSecureInvalidConfigurationFailsClosed(t *testing.T) {
	adaptor := NewSecureTaskAdaptor()
	info := secureRelayInfo("unknown", "video-2.0-pro")
	adaptor.Init(info)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(secureTextBody(8, "16:9", "720p")))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.KeySeedanceOfficialAPI, true)
	taskErr := adaptor.ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusInternalServerError, taskErr.StatusCode)
	assert.Equal(t, "invalid_secure_channel_config", taskErr.Code)
}

type secureVideoMetadataClient struct {
	durations map[string]int64
	err       error
	calls     *atomic.Int32
}

func (f secureVideoMetadataClient) Metadata(_ context.Context, url string) (videometa.Metadata, error) {
	if f.calls != nil {
		f.calls.Add(1)
	}
	if f.err != nil {
		return videometa.Metadata{}, f.err
	}
	return videometa.Metadata{
		DurationMS: f.durations[url], Width: 1280, Height: 720,
		FrameRateNum: 24, FrameRateDen: 1, Container: "mp4", ContentLength: 1,
	}, nil
}

func secureValidationContext(t *testing.T, group dto.SecureVideoGroup, upstreamModel, body string) (*TaskAdaptor, *gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	adaptor := NewSecureTaskAdaptor()
	info := secureRelayInfo(group, upstreamModel)
	adaptor.Init(info)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.KeySeedanceOfficialAPI, true)
	return adaptor, c, info
}

func secureRelayInfo(group dto.SecureVideoGroup, upstreamModel string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:       "https://secure.example",
			ApiKey:               "key",
			UpstreamModelName:    upstreamModel,
			ChannelOtherSettings: dto.ChannelOtherSettings{SecureVideoGroup: group},
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
}

func secureBuildBody(t *testing.T, adaptor *TaskAdaptor, c *gin.Context, info *relaycommon.RelayInfo) ([]byte, string) {
	t.Helper()
	reader, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, "https://secure.example", nil)
	require.NoError(t, err)
	require.NoError(t, adaptor.BuildRequestHeader(c, req, info))
	return body, req.Header.Get("Content-Type")
}

func readSecureMultipart(t *testing.T, contentType string, body []byte) [][2]string {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(contentType)
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mediaType)
	require.NotEmpty(t, params["boundary"])
	reader := multipart.NewReader(strings.NewReader(string(body)), params["boundary"])
	fields := make([][2]string, 0)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		value, err := io.ReadAll(part)
		require.NoError(t, err)
		fields = append(fields, [2]string{part.FormName(), string(value)})
		require.NoError(t, part.Close())
	}
	return fields
}

func secureImageBody(duration int, ratio, resolution string) string {
	return `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/a.jpg"}}],"duration":` +
		strconv.Itoa(duration) + `,"ratio":"` + ratio + `","resolution":"` + resolution + `"}`
}

func secureTextBody(duration int, ratio, resolution string) string {
	return `{"model":"m","content":[{"type":"text","text":"text"}],"duration":` +
		strconv.Itoa(duration) + `,"ratio":"` + ratio + `","resolution":"` + resolution + `"}`
}
