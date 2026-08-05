package service

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectPublicTaskUsesArkWhitelistAndProxyURL(t *testing.T) {
	previousServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://gateway.example"
	t.Cleanup(func() { system_setting.ServerAddress = previousServerAddress })

	task := &model.Task{
		ID:         91,
		CreatedAt:  1710000000,
		UpdatedAt:  1710000005,
		TaskID:     "task_public",
		Platform:   constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo)),
		UserId:     10,
		Group:      "internal-group",
		ChannelId:  40,
		Quota:      123,
		Action:     "generate",
		Status:     model.TaskStatusSuccess,
		SubmitTime: 1710000000,
		StartTime:  1710000001,
		FinishTime: 1710000005,
		Progress:   "100%",
		Properties: model.Properties{
			OriginModelName:   "doubao-seedance-2-0-260128",
			UpstreamModelName: "provider-model",
			RequestPath:       "/supplier/private/path",
		},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "cgt-secret",
			ResultURL:      "https://supplier.example/video.mp4",
			UserRequestData: []byte(`{
				"model":"doubao-seedance-2-0-260128",
				"duration":4,
				"resolution":"480p"
			}`),
			UpstreamResponseData: []byte(`{"id":"cgt-secret"}`),
			UserResponseData: []byte(`{
				"id":"cgt-secret",
				"model":"provider-model",
				"status":"succeeded",
				"content":{"video_url":"https://supplier.example/video.mp4"},
				"usage":{"completion_tokens":108900,"total_tokens":108900},
				"supplier_payload":"never-public"
			}`),
			BillingContext: &model.TaskBillingContext{
				UsageProfile:      model.TaskUsageProfileSeedance,
				OriginModelName:   "doubao-seedance-2-0-260128",
				UpstreamModelName: "provider-model",
			},
			Routing: &modelrouting.Audit{
				PolicyID:      7,
				TargetID:      8,
				TargetName:    "supplier-target",
				UpstreamModel: "provider-model",
			},
		},
	}

	public := ProjectPublicTask(task)
	require.NotNil(t, public)
	require.NotNil(t, public.UserResponseData)
	assert.Equal(t, "task_public", public.TaskID)
	assert.Equal(t, "doubao-seedance-2-0-260128", public.RequestModel)
	assert.Equal(t, "task_public", public.UserResponseData.ID)
	assert.Equal(t, "doubao-seedance-2-0-260128", public.UserResponseData.Model)
	assert.Equal(t, "succeeded", public.UserResponseData.Status)
	require.NotNil(t, public.UserResponseData.Content)
	assert.Equal(t, "https://gateway.example/v1/videos/task_public/content", public.UserResponseData.Content.VideoURL)
	assert.Equal(t, int64(108900), public.UserResponseData.Usage.CompletionTokens)
	assert.Equal(t, int64(4), public.UserResponseData.Duration)
	assert.Equal(t, "480p", public.UserResponseData.Resolution)
	assert.Equal(t, int64(24), public.UserResponseData.FramesPerSecond)

	payload, err := common.Marshal(public)
	require.NoError(t, err)
	body := string(payload)
	for _, forbidden := range []string{
		`"id":91`, `"user_id"`, `"platform"`, `"group"`, `"channel_id"`,
		`"properties"`, `"request_path"`, `"user_request_data"`,
		`"upstream_response_data"`, `"routing"`, `"billing_context"`,
		"provider-model", "cgt-secret", "supplier-target", "supplier.example",
		"supplier_payload", "never-public",
	} {
		assert.NotContains(t, body, forbidden)
	}
}

func TestProjectPublicTaskUsesFixedFailureError(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_failed_public",
		Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo)),
		Status:   model.TaskStatusFailure,
		Properties: model.Properties{
			OriginModelName: "doubao-seedance-2-0-260128",
		},
		FailReason: "supplier-x failed cgt-private https://supplier.example/error",
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "cgt-private",
			UserResponseData: []byte(`{
				"id":"cgt-private",
				"model":"provider-model",
				"status":"failed",
				"error":{"code":"supplier_specific","message":"supplier-x cgt-private"}
			}`),
			BillingContext: &model.TaskBillingContext{
				UsageProfile:    model.TaskUsageProfileSeedance,
				OriginModelName: "doubao-seedance-2-0-260128",
			},
		},
	}

	public := ProjectPublicTask(task)
	require.NotNil(t, public)
	assert.Equal(t, "task failed", public.FailReason)
	require.NotNil(t, public.UserResponseData)
	require.NotNil(t, public.UserResponseData.Error)
	assert.Equal(t, "task_failed", public.UserResponseData.Error.Code)
	assert.Equal(t, "task failed", public.UserResponseData.Error.Message)
	assert.Nil(t, public.UserResponseData.Content)

	payload, err := common.Marshal(public)
	require.NoError(t, err)
	body := string(payload)
	for _, forbidden := range []string{
		"supplier-x", "supplier_specific", "provider-model", "cgt-private", "supplier.example",
	} {
		assert.NotContains(t, body, forbidden)
	}
}

func TestProjectPublicTaskReturnsNilForNilInput(t *testing.T) {
	assert.Nil(t, ProjectPublicTask(nil))
}
