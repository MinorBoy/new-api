package newapivideo

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskAdaptorSwitchesMappedMiniMaxH3ProfileBeforeSubmit(t *testing.T) {
	adaptor := &TaskAdaptor{}
	adaptor.profile = genericProtocolProfile()
	adaptor.mappedModel = modelrouting.MiniMaxH3VIP
	adaptor.baseURL = "https://uniart.example"
	url, err := adaptor.BuildRequestURL(&relaycommon.RelayInfo{})
	require.NoError(t, err)
	assert.Equal(t, "https://uniart.example/v1/videos", url)
	assert.Equal(t, videoRequestDialectMiniMaxH3, adaptor.activeProfile().requestDialect)
}

// A provider upstream ID cannot be recognised from a fixed name, so the H3
// dialect is selected from the canonical client model instead.
func TestTaskAdaptorSelectsH3ProfileFromCanonicalClientModel(t *testing.T) {
	adaptor := &TaskAdaptor{}
	adaptor.mappedModel = "vendor-h3-provider-model"
	adaptor.usesMiniMaxH3 = true
	adaptor.baseURL = "https://vendor.example"
	url, err := adaptor.BuildRequestURL(&relaycommon.RelayInfo{})
	require.NoError(t, err)
	assert.Equal(t, "https://vendor.example/v1/videos", url)
	assert.Equal(t, videoRequestDialectMiniMaxH3, adaptor.activeProfile().requestDialect)
}

// H3 requests arrive on the new-api video entry without the Seedance flag, so the
// dialect must be chosen from the canonical model rather than the mapped ID.
func TestMiniMaxH3SubmitWithoutOfficialFlagUsesH3Dialect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{"model":"MiniMax-H3","content":[{"type":"text","text":"cinematic"}]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		OriginModelName: "MiniMax-H3",
		ChannelMeta:     &relaycommon.ChannelMeta{},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}
	adaptor := &TaskAdaptor{}

	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	require.Nil(t, adaptor.ValidateBillingRequest(c, info))
	state, err := getRequestState(c)
	require.NoError(t, err)
	assert.True(t, state.ProviderValidationComplete)

	// The provider upstream model arrives only after capability routing.
	info.UpstreamModelName = "vendor-h3-2k"
	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	encoded, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"vendor-h3-2k","prompt":"cinematic","mode":"text2video","duration":15,"aspect_ratio":"16:9","resolution":"2k"}`, string(encoded))
}

func TestMiniMaxH3MappedRequestCompletesProviderValidationBeforeBuild(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"minimax-h3","content":[{"type":"text","text":"cinematic"}]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.KeySeedanceOfficialAPI, true)
	info := &relaycommon.RelayInfo{
		OriginModelName: modelrouting.MiniMaxH3,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: modelrouting.MiniMaxH3VIP,
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	adaptor := &TaskAdaptor{profile: genericProtocolProfile()}

	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	require.Nil(t, adaptor.ValidateBillingRequest(c, info))
	state, err := getRequestState(c)
	require.NoError(t, err)
	assert.True(t, state.ProviderValidationComplete)

	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	encoded, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"minimax-h3-vip","prompt":"cinematic","mode":"text2video","duration":15,"aspect_ratio":"16:9","resolution":"2k"}`, string(encoded))
}

func TestMiniMaxH3PollingUsesPersistedOrPassedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name string
		body map[string]any
	}{
		{name: "persisted upstream model", body: map[string]any{"task_id": "task/one", "upstream_model": modelrouting.MiniMaxH3VIP}},
		{name: "passed model", body: map[string]any{"task_id": "task/one", "model": modelrouting.MiniMaxH3}},
		{name: "provider upstream id with canonical origin", body: map[string]any{"task_id": "task/one", "upstream_model": "vendor-h3-2k", "origin_model": modelrouting.MiniMaxH3}},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/videos/task%2Fone", r.URL.EscapedPath())
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			adaptor := &TaskAdaptor{profile: genericProtocolProfile()}
			response, err := adaptor.FetchTask(server.URL, "key", test.body, "")
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, response.StatusCode)
			require.NoError(t, response.Body.Close())
		})
	}
}

func TestSecureInitClearsPreviousProfileError(t *testing.T) {
	adaptor := NewSecureTaskAdaptor()
	adaptor.Init(secureRelayInfo("invalid", "video-2.0-pro"))
	_, err := adaptor.BuildRequestURL(nil)
	require.Error(t, err)

	adaptor.Init(secureRelayInfo("enterprise", "video-2.0-pro"))
	_, err = adaptor.BuildRequestURL(nil)
	require.NoError(t, err)

	adaptor.Init(nil)
	_, err = adaptor.BuildRequestURL(nil)
	require.NoError(t, err)
	assert.Equal(t, ChannelName, adaptor.GetChannelName())
}
