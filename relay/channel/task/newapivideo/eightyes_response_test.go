package newapivideo

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEightYesSubmitResponseUsesPrivateUpstreamAndPublicTaskIDs(t *testing.T) {
	for _, body := range []string{
		`{"id":"upstream","status":"queued"}`,
		`{"task_id":"upstream","status":"queued"}`,
		`{"taskId":"upstream","status":"queued"}`,
	} {
		t.Run(body, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			info := &relaycommon.RelayInfo{OriginModelName: "client", ChannelMeta: &relaycommon.ChannelMeta{}, TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}}

			upstreamID, _, taskErr := NewEightYesTaskAdaptor().DoResponse(c, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, info)

			require.Nil(t, taskErr)
			assert.Equal(t, "upstream", upstreamID)
			var response dto.OpenAIVideo
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, "task_public", response.ID)
			assert.Equal(t, "task_public", response.TaskID)
			assert.Equal(t, "client", response.Model)
			assert.NotContains(t, recorder.Body.String(), "upstream")
		})
	}
}

func TestEightYesSubmitResponseRejectsConflictingIDs(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{OriginModelName: "client", ChannelMeta: &relaycommon.ChannelMeta{}, TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}}

	_, _, taskErr := NewEightYesTaskAdaptor().DoResponse(c, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":"one","task_id":"two"}`))}, info)

	require.NotNil(t, taskErr)
	assert.Equal(t, "invalid_response", taskErr.Code)
	assert.Equal(t, http.StatusBadGateway, taskErr.StatusCode)
	assert.Empty(t, recorder.Body.String())
}

func TestEightYesPollingAllowsContentProxyForURLLessSuccess(t *testing.T) {
	result, err := NewEightYesTaskAdaptor().ParseTaskResult([]byte(`{
		"id":"provider-secret",
		"model":"videos-4-mini-480p",
		"status":"completed",
		"progress":100,
		"created_at":1785652832,
		"completed_at":1785652862,
		"seconds":"5"
	}`))

	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusSuccess), result.Status)
	assert.Empty(t, result.Url)
	assert.Equal(t, "100%", result.Progress)

	_, err = (&TaskAdaptor{}).ParseTaskResult([]byte(`{"status":"completed"}`))
	require.ErrorContains(t, err, "no result URL")
}

func TestEightYesPollingStillPrefersDocumentedResultURLs(t *testing.T) {
	result, err := NewEightYesTaskAdaptor().ParseTaskResult([]byte(`{
		"status":"completed",
		"metadata":{"content_url":"https://cdn.example.com/video.mp4"}
	}`))

	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusSuccess), result.Status)
	assert.Equal(t, "https://cdn.example.com/video.mp4", result.Url)
}
