package service

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsSeedanceTaskRequiresProtocolModelOrBillingEvidence(t *testing.T) {
	platform := constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeNewAPIVideo))
	tests := []struct {
		name string
		task *model.Task
		want bool
	}{
		{
			name: "shared platform alone is insufficient",
			task: &model.Task{Platform: platform, Properties: model.Properties{
				RequestPath: "/v1/video/generations", OriginModelName: "generic-video-model",
			}},
		},
		{
			name: "official Ark path",
			task: &model.Task{Platform: platform, Properties: model.Properties{
				RequestPath: "/api/v3/contents/generations/tasks", OriginModelName: "provider-alias",
			}},
			want: true,
		},
		{
			name: "Seedance billing profile",
			task: &model.Task{Platform: platform, Properties: model.Properties{
				RequestPath: "/v1/video/generations", OriginModelName: "provider-alias",
			}, PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
				UsageProfile: model.TaskUsageProfileSeedance,
			}}},
			want: true,
		},
		{
			name: "canonical Seedance model",
			task: &model.Task{Platform: platform, Properties: model.Properties{
				RequestPath: "/v1/video/generations", OriginModelName: "doubao-seedance-2-0-260128",
			}},
			want: true,
		},
		{
			name: "unsupported platform",
			task: &model.Task{Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeKling)), Properties: model.Properties{
				RequestPath: "/api/v3/contents/generations/tasks", OriginModelName: "doubao-seedance-2-0-260128",
			}},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.want, IsSeedanceTask(testCase.task))
		})
	}
}

func TestNormalizeSeedanceTaskResponseRejectsMissingSuccessVideoURL(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_public_missing_video_url",
		Status:     model.TaskStatusSuccess,
		Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"},
	}
	response := map[string]interface{}{"status": "completed"}

	require.ErrorContains(t, NormalizeSeedanceTaskResponse(task, response), "content.video_url")
}

func TestNormalizeSeedanceTaskResponseUsesFrozenPublicModel(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public_billing_model",
		Status: model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			UsageProfile:    model.TaskUsageProfileSeedance,
			OriginModelName: "doubao-seedance-2-0-260128",
		}, ResultURL: "https://example.com/video.mp4"},
	}
	response := map[string]interface{}{
		"model": "private-upstream-model",
		"error": map[string]interface{}{"code": "stale_error", "message": "private detail"},
	}

	require.NoError(t, NormalizeSeedanceTaskResponse(task, response))

	assert.Equal(t, "doubao-seedance-2-0-260128", response["model"])
	assert.NotContains(t, response, "error")
}

func TestZZoneIsSeedanceTaskPlatform(t *testing.T) {
	platform := constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeZZone))
	assert.True(t, IsSeedanceTaskPlatform(platform))
	assert.Contains(t, SeedanceTaskPlatformValues(), string(platform))
}
