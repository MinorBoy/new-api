package newapivideo

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZ5APIParseTaskResultTreatsSecondsAsProcessingElapsed(t *testing.T) {
	adaptor := NewZ5APITaskAdaptor()

	pending, err := adaptor.ParseTaskResult([]byte(`{"id":"z5-private","status":"pending","progress":0}`))
	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusQueued), pending.Status)
	assert.Equal(t, "0%", pending.Progress)

	processing, err := adaptor.ParseTaskResult([]byte(`{"id":"z5-private","status":"processing","progress":42}`))
	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusInProgress), processing.Status)
	assert.Equal(t, "42%", processing.Progress)

	completed, err := adaptor.ParseTaskResult([]byte(`{"id":"z5-private","status":"completed","object":"https://assets.example/z5.mp4","seconds":"15"}`))
	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusSuccess), completed.Status)
	assert.Equal(t, "https://assets.example/z5.mp4", completed.Url)
	assert.False(t, completed.DurationPresent)
	assert.Zero(t, completed.DurationSeconds)
	assert.Nil(t, completed.CostMeter)
}

func TestZ5APIParseTaskResultBoundsSecondsAndSanitizesFailure(t *testing.T) {
	oversized, err := NewZ5APITaskAdaptor().ParseTaskResult([]byte(`{"id":"z5-private","status":"completed","object":"https://assets.example/z5.mp4","seconds":"999999999"}`))
	require.NoError(t, err)
	assert.False(t, oversized.DurationPresent)
	assert.Zero(t, oversized.DurationSeconds)
	assert.Nil(t, oversized.CostMeter)

	failed, err := NewZ5APITaskAdaptor().ParseTaskResult([]byte(`{"id":"z5-private","task_id":"z5-private","status":"failed","error":{"code":"provider_rejected","message":"task z5-private rejected Authorization: Bearer secret-token"}}`))
	require.NoError(t, err)
	assert.Equal(t, string(model.TaskStatusFailure), failed.Status)
	assert.Equal(t, "provider_rejected", failed.ErrorCode)
	assert.NotContains(t, failed.Reason, "z5-private")
	assert.NotContains(t, failed.Reason, "Bearer secret-token")
}

func TestZ5APIConvertToArkVideoTaskUsesPublicIdentity(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_public",
		Status:     model.TaskStatusSuccess,
		SubmitTime: 1785742254,
		Properties: model.Properties{
			OriginModelName:   "client-seedance",
			UpstreamModelName: "sd-2-c6",
		},
		Data: json.RawMessage(`{"id":"z5-private","task_id":"z5-private","status":"completed","object":"https://assets.example/z5.mp4","seconds":"15"}`),
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID:  "z5-private",
			ResultURL:       "https://assets.example/z5.mp4",
			UserRequestData: json.RawMessage(`{"duration":4}`),
		},
	}

	body, err := NewZ5APITaskAdaptor().ConvertToArkVideoTask(task)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"task_public","model":"client-seedance","status":"succeeded","content":{"video_url":"https://assets.example/z5.mp4"},"created_at":1785742254,"duration":4}`, string(body))
	assert.NotContains(t, string(body), "z5-private")
	assert.NotContains(t, string(body), "sd-2-c6")
}
