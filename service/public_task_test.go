package service

import (
	"strconv"
	"testing"
	"time"

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
				"resolution":"https://supplier.example/private",
				"ratio":"supplier-z5",
				"service_tier":"provider-model-secret",
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
	assert.Equal(t, "16:9", public.UserResponseData.Ratio)
	assert.Equal(t, "default", public.UserResponseData.ServiceTier)
	assert.Equal(t, int64(24), public.UserResponseData.FramesPerSecond)

	payload, err := common.Marshal(public)
	require.NoError(t, err)
	body := string(payload)
	for _, forbidden := range []string{
		`"id":91`, `"user_id"`, `"platform"`, `"group"`, `"channel_id"`,
		`"properties"`, `"request_path"`, `"user_request_data"`,
		`"upstream_response_data"`, `"routing"`, `"billing_context"`,
		"provider-model", "cgt-secret", "supplier-target", "supplier.example",
		"supplier_payload", "never-public", "supplier-z5", "provider-model-secret",
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

func TestProjectPublicTaskRejectsUntrustedSeedanceEnumStrings(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_untrusted_enums",
		Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo)),
		Status:   model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{
			UserRequestData: []byte(`{
				"resolution":"https://supplier.example/private",
				"ratio":"supplier-ratio-secret",
				"service_tier":"provider-tier-secret"
			}`),
			BillingContext: &model.TaskBillingContext{
				UsageProfile: model.TaskUsageProfileSeedance,
			},
		},
	}

	public := ProjectPublicTask(task)
	require.NotNil(t, public)
	require.NotNil(t, public.UserResponseData)
	assert.Equal(t, "720p", public.UserResponseData.Resolution)
	assert.Equal(t, "16:9", public.UserResponseData.Ratio)
	assert.Equal(t, "default", public.UserResponseData.ServiceTier)

	payload, err := common.Marshal(public)
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "supplier.example")
	assert.NotContains(t, string(payload), "supplier-ratio-secret")
	assert.NotContains(t, string(payload), "provider-tier-secret")
}

func TestProjectPublicTaskReturnsWhitelistedSunoOutputsWithMediaProxies(t *testing.T) {
	previousServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://gateway.example"
	t.Cleanup(func() { system_setting.ServerAddress = previousServerAddress })

	task := &model.Task{
		TaskID:   "task_suno_public",
		Platform: constant.TaskPlatformSuno,
		Status:   model.TaskStatusSuccess,
		Data: []byte(`[{
			"id":"provider-clip-secret",
			"model_name":"supplier-model-secret",
			"title":"Public title",
			"text":"Public lyrics",
			"audio_url":"https://supplier.example/audio.mp3",
			"video_url":"https://supplier.example/video.mp4",
			"image_url":"https://supplier.example/image.jpg",
			"metadata":{"api_key":"secret","provider":"supplier-z5"}
		}]`),
	}

	public := ProjectPublicTask(task)
	require.NotNil(t, public)
	payload, err := common.Marshal(public)
	require.NoError(t, err)

	var body map[string]interface{}
	require.NoError(t, common.Unmarshal(payload, &body))
	outputs, ok := body["data"].([]interface{})
	require.True(t, ok)
	require.Len(t, outputs, 1)
	output, ok := outputs[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "Public title", output["title"])
	assert.Equal(t, "Public lyrics", output["text"])
	assert.Equal(t, "https://gateway.example/v1/tasks/task_suno_public/media/0/audio", output["audio_url"])
	assert.Equal(t, "https://gateway.example/v1/tasks/task_suno_public/media/0/video", output["video_url"])
	assert.Equal(t, "https://gateway.example/v1/tasks/task_suno_public/media/0/image", output["image_url"])

	serialized := string(payload)
	for _, forbidden := range []string{
		"provider-clip-secret", "supplier-model-secret", "supplier.example",
		"api_key", "supplier-z5", "metadata",
	} {
		assert.NotContains(t, serialized, forbidden)
	}
}

func TestProjectPublicTaskReturnsLocalResultURLForNonSeedanceVideo(t *testing.T) {
	previousServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://gateway.example"
	t.Cleanup(func() { system_setting.ServerAddress = previousServerAddress })

	task := &model.Task{
		TaskID:   "task_video_public",
		Platform: constant.TaskPlatform("kling"),
		Action:   constant.TaskActionGenerate,
		Status:   model.TaskStatusSuccess,
	}

	public := ProjectPublicTask(task)
	require.NotNil(t, public)
	payload, err := common.Marshal(public)
	require.NoError(t, err)
	assert.Contains(t, string(payload), `"result_url":"https://gateway.example/v1/videos/task_video_public/content"`)
	assert.NotContains(t, string(payload), "supplier.example")
}

func TestProjectPublicTaskReturnsFreshObjectStorageURL(t *testing.T) {
	configureVideoResultStorage(t, enabledVideoStorageValues("provider.example"))
	store := &fakeVideoResultStore{presignURL: "https://cdn.example.com/model/task_video_public.mp4?X-Amz-Expires=86400"}
	installVideoResultDependencies(t, store, nil, func(string) error { return nil })
	task := &model.Task{
		TaskID:   "task_video_public",
		Platform: constant.TaskPlatform("kling"),
		Action:   constant.TaskActionGenerate,
		Status:   model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{
			ResultObjectKey: "model/task_video_public.mp4",
		},
	}

	public := ProjectPublicTask(task)
	require.NotNil(t, public)
	assert.Equal(t, store.presignURL, public.ResultURL)
	assert.Equal(t, 86400*time.Second, store.lastExpiry)
}

func TestProjectPublicTaskReturnsNilForNilInput(t *testing.T) {
	assert.Nil(t, ProjectPublicTask(nil))
}
