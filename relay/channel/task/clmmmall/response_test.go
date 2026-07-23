package clmmmall

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTaskResultMapsStatusAliasesCaseInsensitively(t *testing.T) {
	tests := []struct {
		status   string
		expected model.TaskStatus
		progress string
	}{
		{status: " queued ", expected: model.TaskStatusQueued, progress: "0%"},
		{status: "PENDING", expected: model.TaskStatusQueued, progress: "0%"},
		{status: "processing", expected: model.TaskStatusInProgress, progress: "50%"},
		{status: "RUNNING", expected: model.TaskStatusInProgress, progress: "50%"},
		{status: "In_Progress", expected: model.TaskStatusInProgress, progress: "50%"},
		{status: "completed", expected: model.TaskStatusSuccess, progress: "100%"},
		{status: "SUCCEEDED", expected: model.TaskStatusSuccess, progress: "100%"},
		{status: "success", expected: model.TaskStatusSuccess, progress: "100%"},
		{status: "failed", expected: model.TaskStatusFailure, progress: "100%"},
		{status: "ERROR", expected: model.TaskStatusFailure, progress: "100%"},
		{status: "cancelled", expected: model.TaskStatusFailure, progress: "100%"},
		{status: "CANCELED", expected: model.TaskStatusFailure, progress: "100%"},
	}
	for _, test := range tests {
		t.Run(test.status, func(t *testing.T) {
			body := `{"status":"` + test.status + `"}`
			if test.expected == model.TaskStatusSuccess {
				body = `{"status":"` + test.status + `","video_url":"video"}`
			}
			result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(body))
			require.NoError(t, err)
			assert.Equal(t, string(test.expected), result.Status)
			assert.Equal(t, test.progress, result.Progress)
		})
	}
}

func TestParseTaskResultClampsProgressAndPrioritizesURLs(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		progress string
		url      string
	}{
		{name: "lower clamp", body: `{"status":"processing","progress":-20}`, progress: "0%"},
		{name: "upper clamp", body: `{"status":"processing","progress":120}`, progress: "100%"},
		{name: "explicit zero", body: `{"status":"processing","progress":0}`, progress: "0%"},
		{name: "video url wins", body: `{"status":"completed","video_url":"video","url":"url","result_url":"result","metadata":{"url":"metadata"}}`, progress: "100%", url: "video"},
		{name: "url second", body: `{"status":"completed","url":"url","result_url":"result","metadata":{"url":"metadata"}}`, progress: "100%", url: "url"},
		{name: "result url third", body: `{"status":"completed","result_url":"result","metadata":{"url":"metadata"}}`, progress: "100%", url: "result"},
		{name: "metadata url last", body: `{"status":"completed","metadata":{"url":"metadata"}}`, progress: "100%", url: "metadata"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(test.body))
			require.NoError(t, err)
			assert.Equal(t, test.progress, result.Progress)
			assert.Equal(t, test.url, result.Url)
		})
	}
}

func TestParseTaskResultRetriesSuccessWithoutResultURL(t *testing.T) {
	for _, status := range []string{"completed", "succeeded", "success"} {
		t.Run(status, func(t *testing.T) {
			result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{"status":"` + status + `"}`))

			require.Error(t, err)
			assert.Nil(t, result)
		})
	}
}

func TestParseTaskResultExtractsFailureAndRetriesUnknownStatus(t *testing.T) {
	stringError, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{"status":"failed","error":"generation rejected"}`))
	require.NoError(t, err)
	assert.Equal(t, "task_failed", stringError.ErrorCode)
	assert.Equal(t, "generation rejected", stringError.Reason)

	objectError, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{"status":"error","error":{"code":"content_policy","message":"blocked"}}`))
	require.NoError(t, err)
	assert.Equal(t, "content_policy", objectError.ErrorCode)
	assert.Equal(t, "blocked", objectError.Reason)

	detailError, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{"status":"canceled","detail":"provider canceled the task"}`))
	require.NoError(t, err)
	assert.Equal(t, "task_failed", detailError.ErrorCode)
	assert.Equal(t, "provider canceled the task", detailError.Reason)

	for _, body := range []string{`{"status":"mystery"}`, `{"status":""}`, `{not-json}`} {
		result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(body))
		require.Error(t, err)
		assert.Nil(t, result)
	}
}

func TestConvertToArkVideoTaskUsesOnlyPublicFields(t *testing.T) {
	task := &model.Task{
		CreatedAt:  1709999999,
		TaskID:     "task_public",
		Status:     model.TaskStatusSuccess,
		SubmitTime: 1710000000,
		UpdatedAt:  1710000100,
		Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-250428"},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-private",
		},
		Data: []byte(`{"task_id":"upstream-private","id":"upstream-private","model":"provider-model","status":"completed","progress":100,"video_url":"https://example.com/video.mp4","size":"1280x720","quality":"secret-tier","created_at":999}`),
	}

	data, err := (&TaskAdaptor{}).ConvertToArkVideoTask(task)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "upstream-private")
	assert.NotContains(t, string(data), "provider-model")
	assert.NotContains(t, string(data), "secret-tier")
	assert.JSONEq(t, `{"id":"task_public","model":"doubao-seedance-2-0-250428","status":"succeeded","content":{"video_url":"https://example.com/video.mp4"},"created_at":1710000000,"updated_at":1710000100}`, string(data))
}

func TestConvertToArkVideoTaskMapsStableFailure(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_public",
		Status:     model.TaskStatusFailure,
		SubmitTime: 1,
		UpdatedAt:  2,
		Properties: model.Properties{OriginModelName: "origin-model"},
		Data:       []byte(`{"id":"upstream-private","status":"failed","error":{"code":"content_policy","message":"Authorization: Bearer secret-token https://internal.example/private encoded=c2VjcmV0"}}`),
	}
	data, err := (&TaskAdaptor{}).ConvertToArkVideoTask(task)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "secret-token")
	assert.NotContains(t, string(data), "internal.example")
	assert.NotContains(t, string(data), "c2VjcmV0")

	var response arkTaskResponse
	require.NoError(t, common.Unmarshal(data, &response))
	assert.Equal(t, "task_public", response.ID)
	assert.Equal(t, "origin-model", response.Model)
	assert.Equal(t, "failed", response.Status)
	require.NotNil(t, response.Error)
	assert.Equal(t, "task_failed", response.Error.Code)
	assert.Equal(t, "task failed", response.Error.Message)
}

func TestConvertToArkVideoTaskDoesNotExposePrivateIDInFailure(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_public",
		Status:     model.TaskStatusFailure,
		SubmitTime: 1,
		UpdatedAt:  2,
		Properties: model.Properties{OriginModelName: "origin-model"},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-private",
		},
		Data: []byte(`{"id":"upstream-private","status":"failed","error":{"code":"provider_error","message":"task upstream-private failed; raw private diagnostic"}}`),
	}

	data, err := (&TaskAdaptor{}).ConvertToArkVideoTask(task)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "upstream-private")
	assert.NotContains(t, string(data), "raw private diagnostic")

	var response arkTaskResponse
	require.NoError(t, common.Unmarshal(data, &response))
	require.NotNil(t, response.Error)
	assert.Equal(t, "task_failed", response.Error.Code)
	assert.Equal(t, "task failed", response.Error.Message)
}
