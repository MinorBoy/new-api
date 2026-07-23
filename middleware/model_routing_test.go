package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractSeedanceRoutingInputExplicitFacts(t *testing.T) {
	body := `{
		"model":"doubao-seedance-2-0-260128",
		"content":[
			{"type":"text","text":"make a video"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://x/1.png"}},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://x/2.png"}},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://x/3.png"}},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://x/4.png"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://x/1.mp4"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://x/2.mp4"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://x/3.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://x/1.wav"}}
		],
		"resolution":"1080p","duration":10,"ratio":"16:9",
		"routing":{"require_real_person":true}
	}`
	c := seedanceRoutingContext(t, http.MethodPost, "/v1/video/generations", body, true)

	input, routeErr := extractSeedanceRoutingInput(c, modelrouting.Seedance20)
	require.Nil(t, routeErr)
	require.NotNil(t, input)
	require.NotNil(t, input.OutputResolution)
	require.NotNil(t, input.DurationSeconds)
	require.NotNil(t, input.AspectRatio)
	assert.Equal(t, "1080p", *input.OutputResolution)
	assert.Equal(t, 10, *input.DurationSeconds)
	assert.Equal(t, "16:9", *input.AspectRatio)
	assert.Equal(t, 4, input.ReferenceImages)
	assert.Equal(t, 3, input.ReferenceVideos)
	assert.Equal(t, 1, input.ReferenceAudios)
	assert.True(t, input.RequireRealPerson)

	storage, err := common.GetBodyStorage(c)
	require.NoError(t, err)
	stored, err := storage.Bytes()
	require.NoError(t, err)
	assert.JSONEq(t, body, string(stored))
}

func TestExtractSeedanceRoutingInputPreservesOmittedValuesAndSmartDuration(t *testing.T) {
	omitted := seedanceRoutingContext(t, http.MethodPost, "/v1/video/generations", `{
		"model":"doubao-seedance-2-0-260128",
		"content":[{"type":"text","text":"video"}]
	}`, true)
	input, routeErr := extractSeedanceRoutingInput(omitted, modelrouting.Seedance20)
	require.Nil(t, routeErr)
	require.NotNil(t, input)
	assert.Nil(t, input.OutputResolution)
	assert.Nil(t, input.DurationSeconds)
	assert.Nil(t, input.AspectRatio)

	smart := seedanceRoutingContext(t, http.MethodPost, "/v1/video/generations", `{
		"model":"doubao-seedance-2-0-260128","duration":-1,
		"content":[{"type":"text","text":"video"}]
	}`, true)
	input, routeErr = extractSeedanceRoutingInput(smart, modelrouting.Seedance20)
	require.Nil(t, routeErr)
	require.NotNil(t, input)
	require.NotNil(t, input.DurationSeconds)
	assert.Equal(t, -1, *input.DurationSeconds)
}

func TestExtractSeedanceRoutingInputRejectsInvalidFields(t *testing.T) {
	images := make([]string, 10)
	for index := range images {
		images[index] = `{"type":"image_url","role":"reference_image","image_url":{"url":"https://x/image.png"}}`
	}
	videos := make([]string, 4)
	for index := range videos {
		videos[index] = `{"type":"video_url","role":"reference_video","video_url":{"url":"https://x/video.mp4"}}`
	}
	audios := make([]string, 4)
	for index := range audios {
		audios[index] = `{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://x/audio.wav"}}`
	}
	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "resolution number", body: routingBody(`"resolution":720`), code: "InvalidParameter.resolution"},
		{name: "unsupported resolution", body: routingBody(`"resolution":"1440p"`), code: "InvalidParameter.resolution"},
		{name: "duration string", body: routingBody(`"duration":"10"`), code: "InvalidParameter.duration"},
		{name: "duration zero", body: routingBody(`"duration":0`), code: "InvalidParameter.duration"},
		{name: "duration too large", body: routingBody(`"duration":3601`), code: "InvalidParameter.duration"},
		{name: "invalid ratio", body: routingBody(`"ratio":"2:1"`), code: "InvalidParameter.ratio"},
		{name: "too many images", body: routingContentBody(append([]string{`{"type":"text","text":"video"}`}, images...)), code: "InvalidParameter.content"},
		{name: "too many videos", body: routingContentBody(append([]string{`{"type":"text","text":"video"}`}, videos...)), code: "InvalidParameter.content"},
		{name: "too many audios", body: routingContentBody(append([]string{`{"type":"text","text":"video"}`, `{"type":"image_url","role":"reference_image","image_url":{"url":"https://x/image.png"}}`}, audios...)), code: "InvalidParameter.content"},
		{name: "adaptive with video", body: `{"model":"doubao-seedance-2-0-260128","ratio":"adaptive","content":[{"type":"text","text":"video"},{"type":"video_url","role":"reference_video","video_url":{"url":"https://x/video.mp4"}}]}`, code: "InvalidParameter.ratio"},
		{name: "audio without image or video", body: routingContentBody([]string{`{"type":"text","text":"video"}`, `{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://x/audio.wav"}}`}), code: "InvalidParameter.content"},
		{name: "first frame mixed with reference", body: routingContentBody([]string{`{"type":"text","text":"video"}`, `{"type":"image_url","role":"first_frame","image_url":{"url":"https://x/first.png"}}`, `{"type":"image_url","role":"reference_image","image_url":{"url":"https://x/ref.png"}}`}), code: "InvalidParameter.content"},
		{name: "unsupported role", body: routingContentBody([]string{`{"type":"text","text":"video"}`, `{"type":"image_url","role":"style","image_url":{"url":"https://x/ref.png"}}`}), code: "InvalidParameter.content"},
		{name: "unsupported type", body: routingContentBody([]string{`{"type":"text","text":"video"}`, `{"type":"binary"}`}), code: "InvalidParameter.content"},
		{name: "routing array", body: routingBody(`"routing":[]`), code: "InvalidParameter.routing"},
		{name: "real person string", body: routingBody(`"routing":{"require_real_person":"yes"}`), code: "InvalidParameter.routing.require_real_person"},
		{name: "unknown routing key", body: routingBody(`"routing":{"cost":"low"}`), code: "InvalidParameter.routing.cost"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := seedanceRoutingContext(t, http.MethodPost, "/v1/video/generations", tt.body, true)
			input, routeErr := extractSeedanceRoutingInput(c, modelrouting.Seedance20)
			assert.Nil(t, input)
			require.NotNil(t, routeErr)
			assert.Equal(t, tt.code, string(routeErr.Code))
		})
	}
}

func TestExtractSeedanceRoutingInputSkipsLegacyRequests(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		path      string
		official  bool
		modelName string
	}{
		{name: "not official", method: http.MethodPost, path: "/v1/video/generations", modelName: modelrouting.Seedance20},
		{name: "not post", method: http.MethodGet, path: "/v1/video/generations", official: true, modelName: modelrouting.Seedance20},
		{name: "wrong path", method: http.MethodPost, path: "/v1/videos", official: true, modelName: modelrouting.Seedance20},
		{name: "non canonical", method: http.MethodPost, path: "/v1/video/generations", official: true, modelName: "seedance-custom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := seedanceRoutingContext(t, tt.method, tt.path, `{broken`, tt.official)
			input, routeErr := extractSeedanceRoutingInput(c, tt.modelName)
			assert.Nil(t, input)
			assert.Nil(t, routeErr)
			_, bodyRead := c.Get(common.KeyBodyStorage)
			assert.False(t, bodyRead)
		})
	}
}

func seedanceRoutingContext(t *testing.T, method, path, body string, official bool) *gin.Context {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.KeySeedanceOfficialAPI, official)
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	return c
}

func routingBody(field string) string {
	return `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"video"}],` + field + `}`
}

func routingContentBody(items []string) string {
	return `{"model":"doubao-seedance-2-0-260128","content":[` + strings.Join(items, ",") + `]}`
}
