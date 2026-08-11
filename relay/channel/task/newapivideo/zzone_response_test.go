package newapivideo

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZZoneParseTaskResultAllowsContentEndpointSuccess(t *testing.T) {
	result, err := NewZZoneTaskAdaptor().ParseTaskResult([]byte(`{
		"id":"zzone-private","status":"completed","progress":100,"seconds":"15"
	}`))

	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusSuccess), result.Status)
	assert.Equal(t, "100%", result.Progress)
	assert.Empty(t, result.Url)
	assert.True(t, result.DurationPresent)
	assert.Equal(t, 15, result.DurationSeconds)
}

func TestZZoneParseTaskResultMapsDocumentedStatusShapes(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		status model.TaskStatus
	}{
		{name: "queued", body: `{"id":"zzone-private","status":"pending","progress":0}`, status: model.TaskStatusQueued},
		{name: "running", body: `{"id":"zzone-private","status":"processing","progress":42}`, status: model.TaskStatusInProgress},
		{name: "failed", body: `{"id":"zzone-private","status":"failed","error":{"code":"provider_error","message":"generation failed"}}`, status: model.TaskStatusFailure},
		{name: "expired", body: `{"id":"zzone-private","status":"expired","error":{"code":"expired","message":"task expired"}}`, status: model.TaskStatusFailure},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := NewZZoneTaskAdaptor().ParseTaskResult([]byte(testCase.body))
			require.NoError(t, err)
			assert.Equal(t, string(testCase.status), result.Status)
		})
	}

	_, err := NewZZoneTaskAdaptor().ParseTaskResult([]byte(`{"id":"zzone-private","status":"waiting_for_magic"}`))
	require.ErrorContains(t, err, "unknown new-api video task status")
}

func TestZZoneParseTaskResultSanitizesFailure(t *testing.T) {
	result, err := NewZZoneTaskAdaptor().ParseTaskResult([]byte(`{
		"id":"zzone-private",
		"task_id":"zzone-private",
		"status":"failed",
		"error":{"code":"provider_rejected","message":"task zzone-private rejected Authorization: Bearer secret-token"}
	}`))

	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusFailure), result.Status)
	assert.Equal(t, "provider_rejected", result.ErrorCode)
	assert.NotContains(t, result.Reason, "zzone-private")
	assert.NotContains(t, result.Reason, "Bearer secret-token")
}

func TestZZoneConvertToArkVideoTaskUsesPublicIdentityAndProxyURL(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_public",
		Status:     model.TaskStatusSuccess,
		SubmitTime: 1785742254,
		Properties: model.Properties{
			OriginModelName:   "client-seedance",
			UpstreamModelName: "private-upstream-model",
		},
		Data: json.RawMessage(`{
			"id":"zzone-private","status":"completed","progress":100,"seconds":"15"
		}`),
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "zzone-private",
			ResultURL:      "https://gateway.example/v1/videos/task_public/content",
		},
	}

	body, err := NewZZoneTaskAdaptor().ConvertToArkVideoTask(task)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"id":"task_public",
		"model":"client-seedance",
		"status":"succeeded",
		"content":{"video_url":"https://gateway.example/v1/videos/task_public/content"},
		"created_at":1785742254,
		"duration":15
	}`, string(body))
	assert.NotContains(t, string(body), "zzone-private")
	assert.NotContains(t, string(body), "private-upstream-model")
}
