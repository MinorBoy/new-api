package video_setting

import (
	"fmt"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

const (
	ConfigName = "video_setting"

	KeyBase64InputEnabled   = "base64_input_enabled"
	KeyJSONRequestBodyMaxMB = "json_request_body_max_mb"

	DefaultJSONRequestBodyMaxMB = 16
	MinJSONRequestBodyMaxMB     = 1
	MaxJSONRequestBodyMaxMB     = 128
)

type VideoSetting struct {
	Base64InputEnabled   bool `json:"base64_input_enabled"`
	JSONRequestBodyMaxMB int  `json:"json_request_body_max_mb"`
}

type RuntimeSnapshot struct {
	Base64InputEnabled      bool
	JSONRequestBodyMaxMB    int
	JSONRequestBodyMaxBytes int64
}

var videoSetting = VideoSetting{
	Base64InputEnabled:   false,
	JSONRequestBodyMaxMB: DefaultJSONRequestBodyMaxMB,
}

var runtimeSnapshot atomic.Value

func init() {
	config.GlobalConfig.Register(ConfigName, &videoSetting)
	UpdateAndSync()
}

func ValidateJSONRequestBodyMaxMB(value int) error {
	if value < MinJSONRequestBodyMaxMB || value > MaxJSONRequestBodyMaxMB {
		return fmt.Errorf("video JSON request body limit must be between %d and %d MB", MinJSONRequestBodyMaxMB, MaxJSONRequestBodyMaxMB)
	}
	return nil
}

func normalizedJSONRequestBodyMaxMB(value int) int {
	if err := ValidateJSONRequestBodyMaxMB(value); err != nil {
		common.SysError(err.Error() + "; using safe default")
		return DefaultJSONRequestBodyMaxMB
	}
	return value
}

func Runtime() RuntimeSnapshot {
	if loaded := runtimeSnapshot.Load(); loaded != nil {
		if snapshot, ok := loaded.(RuntimeSnapshot); ok {
			return snapshot
		}
	}
	return buildRuntimeSnapshot(videoSetting)
}

func UpdateAndSync() {
	runtimeSnapshot.Store(buildRuntimeSnapshot(videoSetting))
}

func buildRuntimeSnapshot(setting VideoSetting) RuntimeSnapshot {
	limitMB := normalizedJSONRequestBodyMaxMB(setting.JSONRequestBodyMaxMB)
	return RuntimeSnapshot{
		Base64InputEnabled:      setting.Base64InputEnabled,
		JSONRequestBodyMaxMB:    limitMB,
		JSONRequestBodyMaxBytes: int64(limitMB) << 20,
	}
}
