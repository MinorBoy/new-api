package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	object_storage "github.com/QuantumNous/new-api/setting/object_storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskModel2DtoUsesFreshObjectStorageURL(t *testing.T) {
	cfg := config.GlobalConfig.Get(object_storage.ConfigName)
	require.NotNil(t, cfg)
	original, err := config.ConfigToMap(cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(cfg, original))
		object_storage.UpdateAndSync()
	})
	require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{
		"enabled":           "true",
		"endpoint":          "https://s3.example.com",
		"public_endpoint":   "https://cdn.example.com",
		"region":            "us-east-1",
		"bucket":            "videos",
		"access_key_id":     "access",
		"secret_access_key": "secret",
		"use_path_style":    "true",
		"expires_seconds":   "86400",
		"max_video_size_mb": "512",
	}))
	object_storage.UpdateAndSync()

	task := &model.Task{
		ID:     1,
		TaskID: "task_public",
		Status: model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{
			ResultObjectKey: "doubao-seedance-2-0-fast/task_public.mp4",
			ResultURL:       "https://supplier.example/original.mp4",
		},
	}
	dto := TaskModel2Dto(task, false)

	require.NotNil(t, dto)
	assert.Contains(t, dto.ResultURL, "https://cdn.example.com/videos/doubao-seedance-2-0-fast/task_public.mp4")
	assert.Contains(t, dto.ResultURL, "X-Amz-Expires=86400")
	assert.NotContains(t, dto.ResultURL, "supplier.example")
}

func TestSeedanceTaskResponseUsesFreshObjectStorageURL(t *testing.T) {
	cfg := config.GlobalConfig.Get(object_storage.ConfigName)
	require.NotNil(t, cfg)
	original, err := config.ConfigToMap(cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(cfg, original))
		object_storage.UpdateAndSync()
	})
	require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{
		"enabled":           "true",
		"endpoint":          "https://s3.example.com",
		"public_endpoint":   "https://cdn.example.com",
		"region":            "us-east-1",
		"bucket":            "videos",
		"access_key_id":     "access",
		"secret_access_key": "secret",
		"use_path_style":    "true",
		"expires_seconds":   "86400",
		"max_video_size_mb": "512",
	}))
	object_storage.UpdateAndSync()

	task := &model.Task{
		TaskID:   "task_public",
		Platform: constant.TaskPlatform("doubao"),
		Status:   model.TaskStatusSuccess,
		Data:     mustMarshalTaskPayload(t, map[string]any{"content": map[string]any{"video_url": "https://supplier.example/original.mp4"}}),
		PrivateData: model.TaskPrivateData{
			ResultObjectKey: "doubao-seedance-2-0-fast/task_public.mp4",
		},
	}
	response, err := seedanceTaskResponse(task)
	require.NoError(t, err)
	content, ok := response["content"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, content["video_url"], "https://cdn.example.com/videos/")
	assert.Contains(t, content["video_url"], "X-Amz-Expires=86400")
	assert.NotContains(t, content["video_url"], "supplier.example")
}

func mustMarshalTaskPayload(t *testing.T, value any) []byte {
	t.Helper()
	data, err := common.Marshal(value)
	require.NoError(t, err)
	return data
}
