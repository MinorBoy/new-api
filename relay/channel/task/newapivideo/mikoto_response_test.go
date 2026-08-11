package newapivideo

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMikotoDirectTaskResponseProjection(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus model.TaskStatus
		wantURL    string
	}{
		{name: "queued", body: `{"id":"m1","status":"queued"}`, wantStatus: model.TaskStatusQueued},
		{name: "processing", body: `{"id":"m1","status":"processing","progress":40}`, wantStatus: model.TaskStatusInProgress},
		{name: "Seedance content URL", body: `{"id":"m1","status":"completed","content_url":"https://assets.example/seedance.mp4"}`, wantStatus: model.TaskStatusSuccess, wantURL: "https://assets.example/seedance.mp4"},
		{name: "Sora video URL", body: `{"id":"m1","status":"completed","video_url":"https://assets.example/sora.mp4"}`, wantStatus: model.TaskStatusSuccess, wantURL: "https://assets.example/sora.mp4"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := NewMikotoTaskAdaptor().ParseTaskResult([]byte(test.body))
			require.NoError(t, err)
			assert.Equal(t, string(test.wantStatus), result.Status)
			assert.Equal(t, test.wantURL, result.Url)
		})
	}
}
