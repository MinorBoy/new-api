package relay

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	taskclmmmall "github.com/QuantumNous/new-api/relay/channel/task/clmmmall"
	taskdimensio "github.com/QuantumNous/new-api/relay/channel/task/dimensio"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

type VideoRouteContractError struct {
	Code    string
	Message string
}

func (e *VideoRouteContractError) Error() string {
	return e.Message
}

func ValidateVideoRouteTargetContract(channel *model.Channel, target modelrouting.Target) error {
	if channel == nil {
		return newVideoRouteContractError("route_contract_channel", "channel is required")
	}
	switch channel.Type {
	case constant.ChannelTypeCangyuan:
		return validateTextOnlyVideoRoute(target, nil)
	case constant.ChannelTypePaipu:
		return validatePaipuVideoRoute(target)
	case constant.ChannelTypeClmmMall:
		return validateClmmVideoRoute(target)
	case constant.ChannelTypeDimensio:
		return validateDimensioVideoRoute(target)
	case constant.ChannelTypeSecure:
		return validateSecureVideoRoute(channel.GetOtherSettings().SecureVideoGroup, target)
	default:
		return nil
	}
}

func validatePaipuVideoRoute(target modelrouting.Target) error {
	if strings.TrimSpace(target.UpstreamModel) == "" {
		return newVideoRouteContractError("route_contract_model", "Paipu mapped upstream model is required")
	}
	if !routeDurationWithin(target.Constraints.Durations, 1, relaycommon.MaxTaskDurationSeconds) {
		return newVideoRouteContractError("route_contract_duration", "Paipu route duration exceeds the task protocol limit")
	}
	limits := target.Constraints.ReferenceLimits
	minimums := target.Constraints.ReferenceMinimums
	if limits.Images > 9 || limits.Videos > 3 || limits.Audios > 3 ||
		minimums.Images > limits.Images || minimums.Videos > limits.Videos || minimums.Audios > limits.Audios {
		return newVideoRouteContractError("route_contract_references", "Paipu route reference limits exceed the protocol")
	}
	return nil
}

func validateTextOnlyVideoRoute(target modelrouting.Target, allowedModels []string) error {
	if len(target.Constraints.InputModes) != 1 || target.Constraints.InputModes[0] != modelrouting.InputModeText {
		return newVideoRouteContractError("route_contract_input_mode", "channel route must declare text-only input mode")
	}
	if target.Constraints.ReferenceMinimums != (modelrouting.ReferenceLimits{}) || target.Constraints.ReferenceLimits != (modelrouting.ReferenceLimits{}) {
		return newVideoRouteContractError("route_contract_references", "text-only channel route cannot advertise reference media")
	}
	if len(allowedModels) > 0 && !common.StringsContains(allowedModels, strings.TrimSpace(target.UpstreamModel)) {
		return newVideoRouteContractError("route_contract_model", "mapped upstream model is not verified for this channel")
	}
	if required := modelResolutionSuffix(target.UpstreamModel); required != "" && !allRouteResolutions(target.Constraints.OutputResolutions, required) {
		return newVideoRouteContractError("route_contract_resolution", fmt.Sprintf("mapped model requires %s", required))
	}
	return nil
}

func validateClmmVideoRoute(target modelrouting.Target) error {
	if !routeResolutionsWithin(target.Constraints.OutputResolutions, "480p", "720p") {
		return newVideoRouteContractError("route_contract_resolution", "CLMM routes support only 480p and 720p")
	}
	if err := taskclmmmall.ValidateRouteModel(target.UpstreamModel); err != nil {
		return newVideoRouteContractError("route_contract_model", err.Error())
	}
	limits := target.Constraints.ReferenceLimits
	if limits.Images > 9 || limits.Videos > 3 || limits.Audios != 0 || limits.Images+limits.Videos > 12 {
		return newVideoRouteContractError("route_contract_references", "CLMM route reference limits exceed the verified protocol")
	}
	if !clmmModelControlsDuration(target.UpstreamModel) && !routeDurationWithin(target.Constraints.Durations, 5, 15) {
		return newVideoRouteContractError("route_contract_duration", "ordinary CLMM routes require durations from 5 to 15 seconds")
	}
	return nil
}

func validateDimensioVideoRoute(target modelrouting.Target) error {
	modelName := strings.TrimSpace(target.UpstreamModel)
	if !common.StringsContains(taskdimensio.ModelList, modelName) {
		return newVideoRouteContractError("route_contract_model", "mapped upstream model is not verified for Dimensio")
	}
	if !routeResolutionsWithin(target.Constraints.OutputResolutions, "720p", "1080p") {
		return newVideoRouteContractError("route_contract_resolution", "Dimensio routes support only 720p and 1080p")
	}
	if containsRouteResolution(target.Constraints.OutputResolutions, "1080p") && modelName != "jimeng-video-seedance-2.0-vip" {
		return newVideoRouteContractError("route_contract_resolution", "Dimensio 1080p requires jimeng-video-seedance-2.0-vip")
	}
	limits := target.Constraints.ReferenceLimits
	if limits.Images > 9 || limits.Videos > 3 || limits.Audios > 3 || limits.Images+limits.Videos+limits.Audios > 12 {
		return newVideoRouteContractError("route_contract_references", "Dimensio route reference limits exceed the verified protocol")
	}
	if !routeDurationWithin(target.Constraints.Durations, 4, 15) {
		return newVideoRouteContractError("route_contract_duration", "Dimensio routes require durations from 4 to 15 seconds")
	}
	return nil
}

func validateSecureVideoRoute(group dto.SecureVideoGroup, target modelrouting.Target) error {
	modelName := strings.TrimSpace(target.UpstreamModel)
	if modelName != "video-2.0-fast" && modelName != "video-2.0-mini" && modelName != "video-2.0-pro" {
		return newVideoRouteContractError("route_contract_model", "mapped upstream model is not verified for Secure")
	}
	limits := target.Constraints.ReferenceLimits
	minimums := target.Constraints.ReferenceMinimums
	switch group {
	case dto.SecureVideoGroupDiscount:
		if !routeDurationWithin(target.Constraints.Durations, 4, 15) {
			return newVideoRouteContractError("route_contract_duration", "Secure discount routes require durations from 4 to 15 seconds")
		}
		if minimums.Images < 1 || limits.Images > 9 || limits.Videos > 3 || limits.Audios > 3 || limits.Videos+limits.Audios > 3 {
			return newVideoRouteContractError("route_contract_references", "Secure discount route reference limits exceed the verified protocol")
		}
		if !routeResolutionsWithin(target.Constraints.OutputResolutions, "720p", "1080p", "4k") {
			return newVideoRouteContractError("route_contract_resolution", "Secure discount route resolution is unsupported")
		}
	case dto.SecureVideoGroupOverseas:
		if !routeDurationWithin(target.Constraints.Durations, 4, 15) {
			return newVideoRouteContractError("route_contract_duration", "Secure overseas routes require durations from 4 to 15 seconds")
		}
		if limits.Images > 9 || limits.Videos > 3 || limits.Audios > 3 || limits.Images+limits.Videos+limits.Audios > 12 {
			return newVideoRouteContractError("route_contract_references", "Secure overseas route reference limits exceed the verified protocol")
		}
		if !routeResolutionsWithin(target.Constraints.OutputResolutions, "720p", "1080p") {
			return newVideoRouteContractError("route_contract_resolution", "Secure overseas route resolution is unsupported")
		}
	case dto.SecureVideoGroupEnterprise:
		if modelName != "video-2.0-pro" {
			return newVideoRouteContractError("route_contract_model", "Secure enterprise routes require video-2.0-pro")
		}
		if !routeDurationWithin(target.Constraints.Durations, 5, 15) {
			return newVideoRouteContractError("route_contract_duration", "Secure enterprise routes require durations from 5 to 15 seconds")
		}
		if !allRouteResolutions(target.Constraints.OutputResolutions, "720p") {
			return newVideoRouteContractError("route_contract_resolution", "Secure enterprise routes require 720p")
		}
		if limits.Images > 9 || limits.Videos > 3 || limits.Audios > 3 {
			return newVideoRouteContractError("route_contract_references", "Secure enterprise route reference limits exceed the verified protocol")
		}
	default:
		return newVideoRouteContractError("route_contract_channel", "Secure channel group is invalid")
	}
	if (modelName == "video-2.0-fast" || modelName == "video-2.0-mini") && !allRouteResolutions(target.Constraints.OutputResolutions, "720p") {
		return newVideoRouteContractError("route_contract_resolution", modelName+" supports only 720p")
	}
	return nil
}

func routeDurationWithin(constraint modelrouting.DurationConstraint, minimum, maximum int) bool {
	if len(constraint.Values) > 0 {
		for _, value := range constraint.Values {
			if value < minimum || value > maximum {
				return false
			}
		}
		return true
	}
	return constraint.Min != nil && constraint.Max != nil && *constraint.Min >= minimum && *constraint.Max <= maximum && *constraint.Min <= *constraint.Max
}

func routeResolutionsWithin(values []string, allowed ...string) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if !common.StringsContains(allowed, strings.ToLower(strings.TrimSpace(value))) {
			return false
		}
	}
	return true
}

func allRouteResolutions(values []string, expected string) bool {
	return len(values) > 0 && routeResolutionsWithin(values, expected)
}

func containsRouteResolution(values []string, expected string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), expected) {
			return true
		}
	}
	return false
}

func modelResolutionSuffix(modelName string) string {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	for _, resolution := range []string{"480p", "720p", "1080p"} {
		if strings.HasSuffix(modelName, "-"+resolution) {
			return resolution
		}
	}
	return ""
}

func clmmModelControlsDuration(modelName string) bool {
	for _, segment := range strings.Split(strings.ToLower(strings.TrimSpace(modelName)), "-") {
		if !strings.HasSuffix(segment, "s") {
			continue
		}
		value, err := strconv.Atoi(strings.TrimSuffix(segment, "s"))
		if err == nil && value > 0 {
			return true
		}
	}
	return false
}

func newVideoRouteContractError(code, message string) *VideoRouteContractError {
	return &VideoRouteContractError{Code: code, Message: message}
}
