package objectstorage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildVideoObjectKeyUsesLocalModelAndTaskID(t *testing.T) {
	key, err := BuildVideoObjectKey(
		"doubao-seedance-2-0-fast",
		"task_public",
		"video/mp4",
		"https://upstream.example/result.mp4?signature=secret",
	)
	require.NoError(t, err)
	assert.Equal(t, "doubao-seedance-2-0-fast/task_public.mp4", key)
}

func TestBuildVideoObjectKeySanitizesPathSegments(t *testing.T) {
	key, err := BuildVideoObjectKey("../model/name", "../task?id=1", "video/webm", "https://x/result.webm")
	require.NoError(t, err)
	assert.Equal(t, "model_name/task_id_1.webm", key)
}

func TestBuildVideoObjectKeyDefaultsToMP4(t *testing.T) {
	key, err := BuildVideoObjectKey("model", "task", "", "https://x/result")
	require.NoError(t, err)
	assert.Equal(t, "model/task.mp4", key)
}

func TestBuildVideoObjectKeyRejectsEmptySegments(t *testing.T) {
	for _, values := range [][2]string{{"", "task"}, {"model", ""}} {
		_, err := BuildVideoObjectKey(values[0], values[1], "video/mp4", "https://x/result.mp4")
		assert.Error(t, err)
	}
}
