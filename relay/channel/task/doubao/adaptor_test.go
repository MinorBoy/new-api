package doubao

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoResponseUsesARKTaskIDShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	response := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"id":"cgt-secret"}`)),
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-2-0-260128",
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID: "task_public",
		},
	}

	taskID, taskData, taskErr := (&TaskAdaptor{}).DoResponse(c, response, info)

	require.Nil(t, taskErr)
	assert.Equal(t, "cgt-secret", taskID)
	assert.JSONEq(t, `{"id":"cgt-secret"}`, string(taskData))
	assert.JSONEq(t, `{"id":"task_public"}`, recorder.Body.String())
}

func TestDoResponsePreservesARKTaskIDOptionalFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	response := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"id":"cgt-secret","safety_identifier":"user-hash"}`)),
	}
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}}

	_, _, taskErr := (&TaskAdaptor{}).DoResponse(c, response, info)

	require.Nil(t, taskErr)
	assert.JSONEq(t, `{"id":"task_public","safety_identifier":"user-hash"}`, recorder.Body.String())
}

func TestParseTaskResultPreservesUsageResolutionAndTerminalStates(t *testing.T) {
	adaptor := &TaskAdaptor{}

	result, err := adaptor.ParseTaskResult([]byte("{\"status\":\"succeeded\",\"resolution\":\"1080p\",\"content\":{\"video_url\":\"https://example.com/video.mp4\"},\"usage\":{\"completion_tokens\":1000,\"total_tokens\":1200}}"))
	require.NoError(t, err)
	require.Equal(t, model.TaskStatusSuccess, result.Status)
	require.Equal(t, "1080p", result.Resolution)
	require.Equal(t, 1000, result.CompletionTokens)
	require.Equal(t, 1200, result.TotalTokens)
	require.True(t, result.CompletionTokensPresent)

	zero, err := adaptor.ParseTaskResult([]byte("{\"status\":\"succeeded\",\"usage\":{\"completion_tokens\":0,\"total_tokens\":1200}}"))
	require.NoError(t, err)
	require.True(t, zero.CompletionTokensPresent)
	require.Zero(t, zero.CompletionTokens)

	missing, err := adaptor.ParseTaskResult([]byte("{\"status\":\"succeeded\",\"usage\":{\"total_tokens\":1200}}"))
	require.NoError(t, err)
	require.False(t, missing.CompletionTokensPresent)

	null, err := adaptor.ParseTaskResult([]byte("{\"status\":\"succeeded\",\"usage\":{\"completion_tokens\":null,\"total_tokens\":1200}}"))
	require.NoError(t, err)
	require.False(t, null.CompletionTokensPresent)

	for _, status := range []string{"expired", "cancelled"} {
		failed, err := adaptor.ParseTaskResult([]byte("{\"status\":\"" + status + "\"}"))
		require.NoError(t, err)
		require.Equal(t, model.TaskStatusFailure, failed.Status)
		require.Equal(t, status, failed.Reason)
	}
}

func TestAdjustBillingOnCompleteUsesTerminalFacts(t *testing.T) {
	t.Run("updates only video price ratio", func(t *testing.T) {
		task := &model.Task{PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			UpstreamModelName: "doubao-seedance-2-0-260128",
			HasVideoInput:     true,
			OtherRatios: map[string]float64{
				"video_input": 1,
				"duration":    5,
			},
		}}}

		(&TaskAdaptor{}).AdjustBillingOnComplete(task, &relaycommon.TaskInfo{Resolution: "1080p", TotalTokens: 1000})

		ratios := task.PrivateData.BillingContext.OtherRatios
		assert.InDelta(t, 31.0/46.0, ratios["video_input"], 1e-9)
		assert.Equal(t, 5.0, ratios["duration"])
		assert.Equal(t, "1080p", task.PrivateData.BillingContext.Resolution)
	})

	t.Run("total tokens remove draft estimate", func(t *testing.T) {
		task := &model.Task{PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			UpstreamModelName: "doubao-seedance-1-5-pro-251215",
			OtherRatios: map[string]float64{
				"audio":          2,
				"draft_estimate": 0.6,
				"service_tier":   0.5,
			},
		}}}

		(&TaskAdaptor{}).AdjustBillingOnComplete(task, &relaycommon.TaskInfo{TotalTokens: 1000})

		ratios := task.PrivateData.BillingContext.OtherRatios
		assert.Equal(t, 2.0, ratios["audio"])
		assert.Equal(t, 0.5, ratios["service_tier"])
		assert.NotContains(t, ratios, "draft_estimate")
	})
}

func TestEstimateBillingAcceptsStringBooleanMetadata(t *testing.T) {
	c := newNativeTaskContext(t, `{}`)
	c.Set(common.KeySeedanceOfficialAPI, false)
	info := &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-1-5-pro-251215",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "doubao-seedance-1-5-pro-251215",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	relaycommon.StoreTaskRequest(c, info, "generate", relaycommon.TaskSubmitReq{
		Model: "doubao-seedance-1-5-pro-251215",
		Metadata: map[string]interface{}{
			"generate_audio": "false",
		},
	})

	ratio := (&TaskAdaptor{}).EstimateBilling(c, info)
	assert.NotContains(t, ratio, "audio")
	assert.False(t, c.GetBool(string(constant.ContextKeyTaskGenerateAudio)))
}
