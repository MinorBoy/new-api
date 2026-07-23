package newapivideo

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const detailedSuccessBody = `{
  "code":"success",
  "message":"",
  "data":{
    "task_id":"upstream-secret",
    "status":"SUCCESS",
    "result_url":"https://example.com/outer.mp4",
    "submit_time":1784716214,
    "start_time":1784716220,
    "finish_time":1784716351,
    "progress":"100%",
    "user_id":59,
    "channel_id":14,
    "group":"secret-group",
    "platform":"54",
    "quota":2000000,
    "data":{
      "content":{"video_url":"https://example.com/video.mp4?one=1\u0026two=2"},
      "created_at":1784716214,
      "updated_at":1784716351,
      "draft":false,
      "duration":5,
      "execution_expires_after":172800,
      "framespersecond":24,
      "generate_audio":true,
      "id":"provider-secret",
      "model":"provider-model",
      "priority":0,
      "ratio":"16:9",
      "resolution":"720p",
      "seed":47347,
      "service_tier":"default",
      "status":"succeeded",
      "usage":{"completion_tokens":108900,"total_tokens":108900},
      "future_field":{"keep":true}
    }
  }
}`

func TestParseTaskResultDetailedReport(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(detailedSuccessBody))
	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusSuccess), result.Status)
	assert.Equal(t, "https://example.com/outer.mp4", result.Url)
	assert.Equal(t, 108900, result.CompletionTokens)
	assert.True(t, result.CompletionTokensPresent)
	assert.Equal(t, 108900, result.TotalTokens)
	assert.True(t, result.TotalTokensPresent)
	assert.Equal(t, "720p", result.Resolution)
	assert.Nil(t, result.BillingClamp)
}

func TestParseTaskResultPreservesZeroAndClampsOversizedUsage(t *testing.T) {
	zero, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{"status":"succeeded","metadata":{"url":"https://x/v.mp4"},"usage":{"completion_tokens":0,"total_tokens":0}}`))
	require.NoError(t, err)
	assert.Zero(t, zero.CompletionTokens)
	assert.True(t, zero.CompletionTokensPresent)
	assert.Zero(t, zero.TotalTokens)
	assert.True(t, zero.TotalTokensPresent)

	clamped, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{"status":"succeeded","metadata":{"url":"https://x/v.mp4"},"usage":{"completion_tokens":9999999999999999,"total_tokens":9999999999999999}}`))
	require.NoError(t, err)
	assert.Equal(t, common.MaxQuota, clamped.CompletionTokens)
	assert.Equal(t, common.MaxQuota, clamped.TotalTokens)
	require.NotNil(t, clamped.BillingClamp)
	assert.Equal(t, common.QuotaClampOverflow, clamped.BillingClamp.Kind)
}

func TestParseTaskResultRejectsFractionalOrNegativeUsage(t *testing.T) {
	for _, usage := range []string{
		`{"completion_tokens":1.5,"total_tokens":2}`,
		`{"completion_tokens":-1,"total_tokens":2}`,
	} {
		result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{"status":"succeeded","metadata":{"url":"https://x/v.mp4"},"usage":` + usage + `}`))
		require.Error(t, err)
		assert.Nil(t, result)
	}
}

func TestParseTaskResultStatusMappings(t *testing.T) {
	wrapperStatuses := map[string]model.TaskStatus{
		"NOT_START":   model.TaskStatusNotStart,
		"SUBMITTED":   model.TaskStatusSubmitted,
		"QUEUED":      model.TaskStatusQueued,
		"IN_PROGRESS": model.TaskStatusInProgress,
		"SUCCESS":     model.TaskStatusSuccess,
		"FAILURE":     model.TaskStatusFailure,
	}
	for upstream, expected := range wrapperStatuses {
		t.Run("wrapper "+upstream, func(t *testing.T) {
			url := ""
			if expected == model.TaskStatusSuccess {
				url = `,"result_url":"https://example.com/v.mp4"`
			}
			result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(fmt.Sprintf(`{"code":"success","data":{"status":%q%s,"data":{}}}`, upstream, url)))
			require.NoError(t, err)
			assert.Equal(t, string(expected), result.Status)
		})
	}

	directStatuses := map[string]model.TaskStatus{
		"queued":      model.TaskStatusQueued,
		"in_progress": model.TaskStatusInProgress,
		"running":     model.TaskStatusInProgress,
		"completed":   model.TaskStatusSuccess,
		"succeeded":   model.TaskStatusSuccess,
		"failed":      model.TaskStatusFailure,
		"cancelled":   model.TaskStatusFailure,
	}
	for upstream, expected := range directStatuses {
		t.Run("direct "+upstream, func(t *testing.T) {
			url := ""
			if expected == model.TaskStatusSuccess {
				url = `,"metadata":{"url":"https://example.com/v.mp4"}`
			}
			result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(fmt.Sprintf(`{"status":%q%s}`, upstream, url)))
			require.NoError(t, err)
			assert.Equal(t, string(expected), result.Status)
		})
	}
}

func TestParseTaskResultRejectsUnknownOrURLLessSuccess(t *testing.T) {
	for _, body := range []string{
		`{"status":"mystery"}`,
		`{"status":"succeeded"}`,
		`{"code":"success","data":{"status":"SUCCESS","data":{}}}`,
	} {
		result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(body))
		require.Error(t, err)
		assert.Nil(t, result)
	}
}

func TestParseTaskResultIgnoresCompletedAtForInProgress(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{"id":"upstream-secret","status":"in_progress","progress":50,"completed_at":1784716351,"metadata":{"url":""}}`))
	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusInProgress), result.Status)
	assert.Equal(t, "50%", result.Progress)
	assert.Empty(t, result.Url)
}

func TestParseTaskResultURLPrecedence(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "outer result", body: `{"code":"success","data":{"status":"SUCCESS","result_url":"https://x/outer.mp4","data":{"content":{"video_url":"https://x/nested.mp4"}}}}`, want: "https://x/outer.mp4"},
		{name: "nested content", body: `{"code":"success","data":{"status":"SUCCESS","data":{"content":{"video_url":"https://x/nested.mp4"}}}}`, want: "https://x/nested.mp4"},
		{name: "direct metadata", body: `{"status":"succeeded","metadata":{"url":"https://x/metadata.mp4"},"content":{"video_url":"https://x/content.mp4"},"data":{"url":"https://x/data.mp4"}}`, want: "https://x/metadata.mp4"},
		{name: "direct content", body: `{"status":"succeeded","content":{"video_url":"https://x/content.mp4"},"data":{"url":"https://x/data.mp4"}}`, want: "https://x/content.mp4"},
		{name: "direct data", body: `{"status":"succeeded","data":{"url":"https://x/data.mp4"}}`, want: "https://x/data.mp4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(tt.body))
			require.NoError(t, err)
			assert.Equal(t, tt.want, result.Url)
		})
	}
}

func TestParseTaskResultFailurePrecedence(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "outer reason", body: `{"code":"success","message":"envelope","data":{"status":"FAILURE","fail_reason":"outer","data":{"error":{"message":"nested"}}}}`, want: "outer"},
		{name: "nested error", body: `{"code":"success","message":"envelope","data":{"status":"FAILURE","data":{"error":{"code":"nested-code","message":"nested"}}}}`, want: "nested"},
		{name: "direct error", body: `{"status":"failed","error":{"code":"direct-code","message":"direct"}}`, want: "direct"},
		{name: "envelope message", body: `{"code":"success","message":"envelope","data":{"status":"FAILURE","data":{}}}`, want: "envelope"},
		{name: "fallback", body: `{"status":"failed"}`, want: "task failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(tt.body))
			require.NoError(t, err)
			assert.Equal(t, tt.want, result.Reason)
		})
	}
}

func TestParseTaskPollingHTTPError(t *testing.T) {
	adaptor := &TaskAdaptor{}
	for _, status := range []int{404, 410} {
		result := adaptor.ParseTaskPollingHTTPError([]byte(`{"error":{"code":"gone","message":"provider detail"}}`), status)
		require.NotNil(t, result)
		assert.Equal(t, string(model.TaskStatusFailure), result.Status)
		assert.Equal(t, "task not found or expired", result.Reason)
		assert.Equal(t, fmt.Sprintf("%d", status), result.ErrorCode)
	}

	result := adaptor.ParseTaskPollingHTTPError([]byte(`{"code":"bad_duration","message":"duration invalid"}`), 400)
	require.NotNil(t, result)
	assert.Equal(t, "duration invalid", result.Reason)
	assert.Equal(t, "bad_duration", result.ErrorCode)

	nested := adaptor.ParseTaskPollingHTTPError([]byte(`{"error":{"code":"policy","message":"blocked"}}`), 422)
	require.NotNil(t, nested)
	assert.Equal(t, "blocked", nested.Reason)
	assert.Equal(t, "policy", nested.ErrorCode)

	assert.Nil(t, adaptor.ParseTaskPollingHTTPError([]byte(`{}`), 503))
}

func TestConvertToOpenAIVideoUsesOnlyPublicTaskFacts(t *testing.T) {
	task := reportTaskFixture()
	body, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	assertPublicBody(t, body)

	var response dto.OpenAIVideo
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, "task_public", response.ID)
	assert.Equal(t, "task_public", response.TaskID)
	assert.Equal(t, "client-model", response.Model)
	assert.Equal(t, dto.VideoStatusCompleted, response.Status)
	assert.Equal(t, int64(1784716214), response.CreatedAt)
	assert.Equal(t, int64(1784716351), response.CompletedAt)
	assert.Equal(t, "https://example.com/outer.mp4", response.Metadata["url"])
}

func TestConvertToArkVideoTaskPreservesSafeZerosAndUsage(t *testing.T) {
	task := reportTaskFixture()
	body, err := (&TaskAdaptor{}).ConvertToArkVideoTask(task)
	require.NoError(t, err)
	assertPublicBody(t, body)
	assert.Contains(t, string(body), `"draft":false`)
	assert.Contains(t, string(body), `"priority":0`)

	var response arkTaskResponse
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, "task_public", response.ID)
	assert.Equal(t, "client-model", response.Model)
	assert.Equal(t, "succeeded", response.Status)
	require.NotNil(t, response.Usage)
	require.NotNil(t, response.Usage.CompletionTokens)
	assert.Equal(t, "108900", response.Usage.CompletionTokens.String())
	assert.Equal(t, "https://example.com/video.mp4?one=1&two=2", response.Content.VideoURL)
}

func TestConvertToArkVideoTaskPreservesExplicitZeroUsage(t *testing.T) {
	task := reportTaskFixture()
	task.Data = json.RawMessage(`{"code":"success","data":{"status":"SUCCESS","result_url":"https://x/v.mp4","data":{"draft":false,"priority":0,"usage":{"completion_tokens":0,"total_tokens":0}}}}`)
	body, err := (&TaskAdaptor{}).ConvertToArkVideoTask(task)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"completion_tokens":0`)
	assert.Contains(t, string(body), `"total_tokens":0`)
}

func TestPublicConvertersFallbackToLocalPollingFailure(t *testing.T) {
	task := reportTaskFixture()
	task.Status = model.TaskStatusFailure
	task.FailReason = "task not found or expired"
	task.Data = json.RawMessage(`{"code":"not_found","message":"provider task missing","user_id":59,"quota":2000000}`)

	openAIBody, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	assertPublicBody(t, openAIBody)
	var openAIResponse dto.OpenAIVideo
	require.NoError(t, common.Unmarshal(openAIBody, &openAIResponse))
	assert.Equal(t, dto.VideoStatusFailed, openAIResponse.Status)
	require.NotNil(t, openAIResponse.Error)
	assert.Equal(t, "not_found", openAIResponse.Error.Code)
	assert.Equal(t, "task not found or expired", openAIResponse.Error.Message)

	arkBody, err := (&TaskAdaptor{}).ConvertToArkVideoTask(task)
	require.NoError(t, err)
	assertPublicBody(t, arkBody)
	var arkResponse arkTaskResponse
	require.NoError(t, common.Unmarshal(arkBody, &arkResponse))
	assert.Equal(t, "failed", arkResponse.Status)
	require.NotNil(t, arkResponse.Error)
	assert.Equal(t, "not_found", arkResponse.Error.Code)
	assert.Equal(t, "task not found or expired", arkResponse.Error.Message)
}

func reportTaskFixture() *model.Task {
	return &model.Task{
		TaskID:     "task_public",
		Platform:   "60",
		UserId:     59,
		Group:      "secret-group",
		ChannelId:  14,
		Quota:      2000000,
		Status:     model.TaskStatusSuccess,
		SubmitTime: 1784716214,
		FinishTime: 1784716351,
		UpdatedAt:  1784716351,
		Progress:   "100%",
		Properties: model.Properties{OriginModelName: "client-model", UpstreamModelName: "provider-model"},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-secret",
			ResultURL:      "https://example.com/private.mp4",
		},
		Data: json.RawMessage(detailedSuccessBody),
	}
}

func assertPublicBody(t *testing.T, body []byte) {
	t.Helper()
	for _, privateValue := range []string{
		"upstream-secret", "provider-model", "provider-secret", "user_id", "channel_id",
		"secret-group", "platform", "quota",
	} {
		assert.NotContains(t, string(body), privateValue)
	}
}
