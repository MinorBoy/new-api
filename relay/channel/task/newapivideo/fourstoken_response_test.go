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
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFourSTokenSubmitResponseAcceptsTaskIDVariants(t *testing.T) {
	for _, body := range []string{
		`{"task_id":"upstream"}`,
		`{"taskId":"upstream"}`,
		`{"id":"upstream"}`,
	} {
		t.Run(body, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set(common.KeySeedanceOfficialAPI, true)
			info := &relaycommon.RelayInfo{OriginModelName: "client", ChannelMeta: &relaycommon.ChannelMeta{}, TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "public"}}
			upstreamID, _, taskErr := (&TaskAdaptor{}).DoResponse(c, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, info)
			require.Nil(t, taskErr)
			assert.Equal(t, "upstream", upstreamID)
			assert.JSONEq(t, `{"id":"public"}`, recorder.Body.String())
		})
	}
}

func TestFourSTokenSubmitResponseRejectsConflictingIDs(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	info := &relaycommon.RelayInfo{OriginModelName: "client", ChannelMeta: &relaycommon.ChannelMeta{}, TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "public"}}
	_, _, taskErr := (&TaskAdaptor{}).DoResponse(c, &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"task_id":"one","taskId":"two","id":"one"}`))}, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, "invalid_response", taskErr.Code)
	assert.Equal(t, http.StatusBadGateway, taskErr.StatusCode)
	assert.Empty(t, recorder.Body.String())
}

func TestParseFourSTokenTaskProjection(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{
		"id":"provider-secret",
		"model":"4sdance_v2.0_900",
		"status":"succeeded",
		"created_at":1784716214,
		"updated_at":1784716351,
		"content":{"video_url":"https://example.com/video.mp4","last_frame_url":"https://example.com/last.jpg"},
		"usage":{"completion_tokens":108900,"total_tokens":108900}
	}`))
	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusSuccess), result.Status)
	assert.Equal(t, "https://example.com/video.mp4", result.Url)
	assert.Equal(t, 108900, result.TotalTokens)
	assert.True(t, result.TotalTokensPresent)
}

func TestParseFourSTokenTaskStatusesAndFailureSanitization(t *testing.T) {
	for _, upstream := range []string{"queued", "running", "succeeded", "failed", "cancelled", "expired"} {
		t.Run(upstream, func(t *testing.T) {
			body := `{"id":"provider-secret","status":"` + upstream + `"}`
			if upstream == "succeeded" {
				body = `{"id":"provider-secret","status":"succeeded","content":{"video_url":"https://example.com/video.mp4"}}`
			}
			if upstream == "failed" || upstream == "cancelled" || upstream == "expired" {
				body = `{"id":"provider-secret","status":"` + upstream + `","error":{"code":"provider-secret","message":"failed provider-secret"}}`
			}
			result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(body))
			require.NoError(t, err)
			if upstream == "queued" {
				assert.Equal(t, string(model.TaskStatusQueued), result.Status)
			} else if upstream == "running" {
				assert.Equal(t, string(model.TaskStatusInProgress), result.Status)
			} else if upstream == "succeeded" {
				assert.Equal(t, string(model.TaskStatusSuccess), result.Status)
			} else {
				assert.Equal(t, string(model.TaskStatusFailure), result.Status)
				assert.NotContains(t, result.Reason, "provider-secret")
			}
		})
	}
	_, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{"status":"succeeded"}`))
	require.Error(t, err)
}
