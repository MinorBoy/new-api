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
	assert.JSONEq(t, `{"model":"minimax-h3-vip","prompt":"cinematic","duration":15,"aspect_ratio":"16:9","resolution":"2k"}`, string(encoded))
}

func TestMiniMaxH3PollingUsesPersistedOrPassedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name string
		body map[string]any
	}{
		{name: "persisted upstream model", body: map[string]any{"task_id": "task/one", "upstream_model": modelrouting.MiniMaxH3VIP}},
		{name: "passed model", body: map[string]any{"task_id": "task/one", "model": modelrouting.MiniMaxH3}},
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
