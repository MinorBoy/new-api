package taskcommon

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

// UnmarshalMetadata converts a map[string]any metadata to a typed struct via JSON round-trip.
// This replaces the repeated pattern: json.Marshal(metadata) → json.Unmarshal(bytes, &target).
func UnmarshalMetadata(metadata map[string]any, target any) error {
	if metadata == nil {
		return nil
	}
	// Prevent metadata from overriding model fields to avoid billing bypass.
	delete(metadata, "model")
	metaBytes, err := common.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata failed: %w", err)
	}
	if err := common.Unmarshal(metaBytes, target); err != nil {
		return fmt.Errorf("unmarshal metadata failed: %w", err)
	}
	return nil
}

// DefaultString returns val if non-empty, otherwise fallback.
func DefaultString(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}

// DefaultInt returns val if non-zero, otherwise fallback.
func DefaultInt(val, fallback int) int {
	if val == 0 {
		return fallback
	}
	return val
}

// EncodeLocalTaskID encodes an upstream operation name to a URL-safe base64 string.
// Used by Gemini/Vertex to store upstream names as task IDs.
func EncodeLocalTaskID(name string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(name))
}

// DecodeLocalTaskID decodes a base64-encoded upstream operation name.
func DecodeLocalTaskID(id string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// BuildProxyURL constructs the video proxy URL using the public task ID.
// e.g., "https://your-server.com/v1/videos/task_xxxx/content"
func BuildProxyURL(taskID string) string {
	return fmt.Sprintf("%s/v1/videos/%s/content", system_setting.ServerAddress, taskID)
}

func BuildTaskMediaProxyURL(taskID string, index int, kind string) string {
	return fmt.Sprintf("%s/v1/tasks/%s/media/%d/%s", system_setting.ServerAddress, taskID, index, kind)
}

// Status-to-progress mapping constants for polling updates.
const (
	ProgressSubmitted  = "10%"
	ProgressQueued     = "20%"
	ProgressInProgress = "30%"
	ProgressComplete   = "100%"
)

// ---------------------------------------------------------------------------
// BaseBilling — embeddable no-op implementations for TaskAdaptor billing methods.
// Adaptors that do not need custom billing can embed this struct directly.
// ---------------------------------------------------------------------------

type BaseBilling struct{}

// EstimateBilling returns nil (no extra ratios; use base model price).
func (BaseBilling) EstimateBilling(_ *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	return nil
}

// AdjustBillingOnSubmit returns nil (no submit-time adjustment).
func (BaseBilling) AdjustBillingOnSubmit(_ *relaycommon.RelayInfo, _ []byte) map[string]float64 {
	return nil
}

// AdjustBillingOnComplete returns 0 (keep pre-charged amount).
func (BaseBilling) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return 0
}

func (BaseBilling) CostCapabilities(_ *relaycommon.RelayInfo) types.CostCapabilities {
	return TaskCostCapabilities()
}

func TaskCostCapabilities(meterSources ...types.CostMeterSource) types.CostCapabilities {
	return types.CostCapabilities{
		CanResolveBillableModel: true,
		ChargeEvents: []types.CostChargeEvent{
			types.CostChargeSubmitAccepted,
			types.CostChargeTaskSucceeded,
		},
		MeterSources: append([]types.CostMeterSource(nil), meterSources...),
	}
}

func (BaseBilling) ConfirmTaskCostIdentity(info *relaycommon.RelayInfo) error {
	if info == nil || info.ChannelMeta == nil {
		return fmt.Errorf("final billable task model is not confirmed")
	}
	modelName := strings.TrimSpace(info.ChannelMeta.UpstreamModelName)
	if modelName == "" {
		return fmt.Errorf("final billable task model is not confirmed")
	}
	info.BillableUpstreamModel = modelName
	return nil
}

func (BaseBilling) NormalizeTaskCostMeter(_ *model.Task, result *relaycommon.TaskInfo) (types.CostMeter, error) {
	if result == nil {
		return types.CostMeter{}, fmt.Errorf("authoritative task cost meter is unavailable")
	}
	if result.CostMeter != nil {
		meter := *result.CostMeter
		if err := validateTaskCostTokenBounds(meter); err != nil {
			return types.CostMeter{}, err
		}
		return meter, nil
	}
	if !result.CompletionTokensPresent && !result.TotalTokensPresent {
		return types.CostMeter{}, fmt.Errorf("authoritative task cost meter is unavailable")
	}

	meter := types.CostMeter{Source: types.CostMeterUpstreamUsage}
	if result.CompletionTokensPresent {
		completion := int64(result.CompletionTokens)
		meter.OutputTokens = &completion
		meter.CompletionTokens = &completion
	}
	if result.TotalTokensPresent {
		total := int64(result.TotalTokens)
		meter.TotalTokens = &total
	}
	if err := validateTaskCostTokenBounds(meter); err != nil {
		return types.CostMeter{}, err
	}
	return meter, nil
}

func validateTaskCostTokenBounds(meter types.CostMeter) error {
	for name, value := range map[string]*int64{
		"input tokens":      meter.InputTokens,
		"output tokens":     meter.OutputTokens,
		"completion tokens": meter.CompletionTokens,
		"total tokens":      meter.TotalTokens,
	} {
		if value != nil && (*value < 0 || *value > int64(relaycommon.MaxTokensLimit)) {
			return fmt.Errorf("%s exceeds the supported task cost meter range", name)
		}
	}
	return nil
}
