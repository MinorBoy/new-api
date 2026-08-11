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

func TestFFLinkDoResponseExtractsJobID(t *testing.T) {
	adaptor := NewFYLinkTaskAdaptor()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set(common.KeySeedanceOfficialAPI, true)
	info := &relaycommon.RelayInfo{
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
		OriginModelName: "client-model",
	}
	jobID, body, taskErr := adaptor.DoResponse(ctx, &http.Response{
		StatusCode: http.StatusAccepted,
		Body:       io.NopCloser(strings.NewReader(`{"job_id":"job_1","status":"pending"}`)),
	}, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "job_1", jobID)
	assert.Contains(t, string(body), "job_1")
}

func TestFFLinkStatusProjectionSupportsSettlingAndMissingContentURL(t *testing.T) {
	adaptor := NewFYLinkTaskAdaptor()
	for _, status := range []string{"pending", "running", "settling", "completed", "failed", "canceled"} {
		t.Run(status, func(t *testing.T) {
			body := `{"status":"` + status + `","duration":8,"resolution":"720p"}`
			if status == "failed" || status == "canceled" {
				body = `{"status":"` + status + `","error":{"code":"provider_rejected","message":"job_1 rejected Bearer secret-token"}}`
			}
			result, err := adaptor.ParseTaskResult([]byte(body))
			require.NoError(t, err)
			switch status {
			case "pending":
				assert.Equal(t, string(model.TaskStatusQueued), result.Status)
			case "running", "settling":
				assert.Equal(t, string(model.TaskStatusInProgress), result.Status)
			case "completed":
				assert.Equal(t, string(model.TaskStatusSuccess), result.Status)
				assert.Empty(t, result.Url)
			default:
				assert.Equal(t, string(model.TaskStatusFailure), result.Status)
				assert.NotContains(t, result.Reason, "secret-token")
			}
		})
	}
	_, err := adaptor.ParseTaskResult([]byte(`{"status":"unknown_state"}`))
	assert.Error(t, err)
}

func TestFFLinkConvertToArkVideoTaskUsesPublicProxyWhenContentURLIsAbsent(t *testing.T) {
	task := &model.Task{
		TaskID:      "task_public",
		Platform:    "214",
		Status:      model.TaskStatusSuccess,
		Properties:  model.Properties{OriginModelName: "client-model"},
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "job_private"},
		Data:        []byte(`{"status":"completed","duration":8,"resolution":"720p"}`),
	}
	body, err := NewFYLinkTaskAdaptor().ConvertToArkVideoTask(task)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"id":"task_public"`)
	assert.Contains(t, string(body), `"status":"succeeded"`)
	assert.Contains(t, string(body), "/v1/videos/task_public/content")
	assert.NotContains(t, string(body), "job_private")
}
