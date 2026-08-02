package newapivideo

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaipuProfileUsesDynamicModelsAndArrayDialect(t *testing.T) {
	adaptor := NewPaipuTaskAdaptor()
	profile := adaptor.activeProfile()
	assert.Equal(t, ChannelNamePaipu, adaptor.GetChannelName())
	assert.Empty(t, adaptor.GetModelList())
	assert.Equal(t, "/v1/videos", profile.submitPath)
	assert.Equal(t, "/v1/videos/{task_id}", profile.pollPath)
	assert.Equal(t, videoRequestDialectPaipuMediaArrays, profile.requestDialect)
	assert.True(t, profile.allowEmbeddedMedia)
	assert.True(t, profile.requirePublicHTTPMedia)
}

func TestBuildPaipuRequestPreservesMultimodalArrays(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"doubao-seedance-2-0-260128",
		"content":[
			{"type":"text","text":"人物跟拍"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.png"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.4.4/ref.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"data:audio/wav;base64,UklGRg=="}}
		],
		"duration":5,"ratio":"16:9","resolution":"720p"
	}`), paipuProtocolProfile())
	require.NoError(t, err)
	body, err := buildPaipuRequest(request, "imported-paipu-model")
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"imported-paipu-model","prompt":"人物跟拍",
		"duration":5,"aspect_ratio":"16:9","resolution":"720p",
		"images":["https://8.8.8.8/ref.png"],
		"videos":["https://8.8.4.4/ref.mp4"],
		"audios":["data:audio/wav;base64,UklGRg=="]
	}`, string(body))
}

func TestBuildPaipuRequestOmitsAbsentScalarsAndAcceptsAnyImportedModel(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"doubao-seedance-2-0-260128",
		"content":[{"type":"text","text":"minimal"}]
	}`), paipuProtocolProfile())
	require.NoError(t, err)
	body, err := buildPaipuRequest(request, "vendor-model-from-import-v9")
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"vendor-model-from-import-v9","prompt":"minimal"}`, string(body))
}

func TestBuildPaipuRequestKeepsReferenceOrder(t *testing.T) {
	request, err := parseARKRequest([]byte(`{
		"model":"doubao-seedance-2-0-260128",
		"content":[
			{"type":"text","text":"ordered"},
			{"type":"image_url","image_url":{"url":"https://8.8.8.8/a.png"}},
			{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.4.4/b.png"}}
		]
	}`), paipuProtocolProfile())
	require.NoError(t, err)
	body, err := buildPaipuRequest(request, "imported-paipu-model")
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, common.Unmarshal(body, &got))
	assert.Equal(t, []any{"https://8.8.8.8/a.png", "https://8.8.4.4/b.png"}, got["images"])
}

func TestBuildPaipuRequestEnforcesMediaBoundariesRolesAndUrlSafety(t *testing.T) {
	imageURL := func(i int) string {
		return fmt.Sprintf("https://8.8.8.%d/img-%d.png", i%250+1, i)
	}
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{name: "nine images pass", body: paipuImageContent(9, imageURL)},
		{name: "ten images fail", body: paipuImageContent(10, imageURL), wantErr: true},
		{name: "three videos pass", body: paipuVideoContent(3)},
		{name: "four videos fail", body: paipuVideoContent(4), wantErr: true},
		{name: "three audios pass", body: paipuAudioContent(3)},
		{name: "four audios fail", body: paipuAudioContent(4), wantErr: true},
		{name: "first frame rejected", body: `{"model":"m","content":[{"type":"text","text":"t"},{"type":"image_url","role":"first_frame","image_url":{"url":"https://8.8.8.8/f.png"}}]}`, wantErr: true},
		{name: "last frame rejected", body: `{"model":"m","content":[{"type":"text","text":"t"},{"type":"image_url","role":"last_frame","image_url":{"url":"https://8.8.8.8/l.png"}}]}`, wantErr: true},
		{name: "wrong mime data uri", body: `{"model":"m","content":[{"type":"text","text":"t"},{"type":"video_url","role":"reference_video","video_url":{"url":"data:image/png;base64,iVBORw0KGgo="}}]}`, wantErr: true},
		{name: "asset uri rejected", body: `{"model":"m","content":[{"type":"text","text":"t"},{"type":"image_url","role":"reference_image","image_url":{"url":"asset://bucket/ref.png"}}]}`, wantErr: true},
		{name: "file uri rejected", body: `{"model":"m","content":[{"type":"text","text":"t"},{"type":"image_url","role":"reference_image","image_url":{"url":"file:///etc/ref.png"}}]}`, wantErr: true},
		{name: "private url rejected", body: `{"model":"m","content":[{"type":"text","text":"t"},{"type":"image_url","role":"reference_image","image_url":{"url":"http://192.168.1.1/ref.png"}}]}`, wantErr: true},
		{name: "multiple text rejected", body: `{"model":"m","content":[{"type":"text","text":"a"},{"type":"text","text":"b"}]}`, wantErr: true},
		{name: "http image passes", body: `{"model":"m","content":[{"type":"text","text":"t"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://8.8.8.8/ref.png"}}]}`},
		{name: "matching mime data uri passes", body: `{"model":"m","content":[{"type":"text","text":"t"},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"data:audio/wav;base64,UklGRg=="}}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, parseErr := parseARKRequest([]byte(tt.body), paipuProtocolProfile())
			if parseErr == nil {
				_, buildErr := buildPaipuRequest(request, "imported-paipu-model")
				if tt.wantErr {
					require.Error(t, buildErr)
					var requestErr *arkRequestError
					require.ErrorAs(t, buildErr, &requestErr)
					assert.Equal(t, "InvalidParameter.content", requestErr.Code)
					return
				}
				require.NoError(t, buildErr)
				return
			}
			if tt.wantErr {
				var requestErr *arkRequestError
				require.ErrorAs(t, parseErr, &requestErr)
				assert.Equal(t, "InvalidParameter.content", requestErr.Code)
				return
			}
			require.NoError(t, parseErr)
		})
	}
}

func paipuImageContent(count int, url func(int) string) string {
	items := `[{"type":"text","text":"t"}`
	for i := 0; i < count; i++ {
		items += fmt.Sprintf(`,{"type":"image_url","role":"reference_image","image_url":{"url":%q}}`, url(i))
	}
	items += "]"
	return fmt.Sprintf(`{"model":"m","content":%s}`, items)
}

func paipuVideoContent(count int) string {
	items := `[{"type":"text","text":"t"}`
	for i := 0; i < count; i++ {
		items += fmt.Sprintf(`,{"type":"video_url","role":"reference_video","video_url":{"url":"https://8.8.8.%d/ref.mp4"}}`, i%250+1)
	}
	items += "]"
	return fmt.Sprintf(`{"model":"m","content":%s}`, items)
}

func paipuAudioContent(count int) string {
	items := `[{"type":"text","text":"t"}`
	for i := 0; i < count; i++ {
		items += fmt.Sprintf(`,{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://8.8.8.%d/ref.mp3"}}`, i%250+1)
	}
	items += "]"
	return fmt.Sprintf(`{"model":"m","content":%s}`, items)
}

func TestPaipuRejectsUnsupportedScalarsBeforeBuild(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "image untyped still reference", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"image_url","image_url":{"url":"https://8.8.8.8/ref.png"}}]}`, code: ""},
		{name: "generate audio", body: `{"model":"m","content":[{"type":"text","text":"text"}],"generate_audio":true}`, code: "InvalidParameter.generate_audio"},
		{name: "watermark", body: `{"model":"m","content":[{"type":"text","text":"text"}],"watermark":false}`, code: "InvalidParameter.watermark"},
		{name: "seed", body: `{"model":"m","content":[{"type":"text","text":"text"}],"seed":7}`, code: "InvalidParameter.seed"},
		{name: "callback url", body: `{"model":"m","content":[{"type":"text","text":"text"}],"callback_url":"https://8.8.8.8/hook"}`, code: "InvalidParameter.callback_url"},
		{name: "draft", body: `{"model":"m","content":[{"type":"text","text":"text"}],"draft":true}`, code: "InvalidParameter.draft"},
		{name: "tools", body: `{"model":"m","content":[{"type":"text","text":"text"}],"tools":[{"type":"camera_fixed"}]}`, code: "InvalidParameter.tools"},
		{name: "draft task", body: `{"model":"m","content":[{"type":"text","text":"text"},{"type":"draft_task","draft_task":{"id":"task"}}]}`, code: "InvalidParameter.content"},
		{name: "service tier", body: `{"model":"m","content":[{"type":"text","text":"text"}],"service_tier":"flex"}`, code: "InvalidParameter.service_tier"},
		{name: "duration overflow", body: fmt.Sprintf(`{"model":"m","content":[{"type":"text","text":"text"}],"duration":%d}`, relaycommon.MaxTaskDurationSeconds+1), code: "InvalidParameter.duration"},
		{name: "empty ratio", body: `{"model":"m","content":[{"type":"text","text":"text"}],"ratio":"  "}`, code: "InvalidParameter.ratio"},
		{name: "empty resolution", body: `{"model":"m","content":[{"type":"text","text":"text"}],"resolution":""}`, code: "InvalidParameter.resolution"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set(common.KeySeedanceOfficialAPI, true)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "imported-paipu-model"}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
			adaptor := NewPaipuTaskAdaptor()
			taskErr := adaptor.ValidateRequestAndSetAction(c, info)
			if tt.code == "" {
				require.Nil(t, taskErr)
				return
			}
			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			assert.Equal(t, tt.code, taskErr.Code)
		})
	}
}

func TestPaipuAllowsDefaultServiceTierAndOmitsItUpstream(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{"model":"client","content":[{"type":"text","text":"text"}],"service_tier":"default"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.KeySeedanceOfficialAPI, true)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "imported-paipu-model"}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	adaptor := NewPaipuTaskAdaptor()
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	require.Nil(t, adaptor.ValidateBillingRequest(c, info))
	reader, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"imported-paipu-model","prompt":"text"}`, string(body))
}

func TestPaipuBuildRequiresCompletedProviderValidation(t *testing.T) {
	request, err := parseARKRequest([]byte(`{"model":"client","content":[{"type":"text","text":"text"}]}`), paipuProtocolProfile())
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(common.KeySeedanceOfficialAPI, true)
	c.Set(requestStateContextKey, requestState{ARK: &request})
	_, err = NewPaipuTaskAdaptor().BuildRequestBody(c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "imported-paipu-model"}})
	assert.ErrorContains(t, err, "validation")
}
