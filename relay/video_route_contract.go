package relay

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	taskclmmmall "github.com/QuantumNous/new-api/relay/channel/task/clmmmall"
	tasknewapivideo "github.com/QuantumNous/new-api/relay/channel/task/newapivideo"
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
		return validateCangyuanVideoRoute(target)
	case constant.ChannelTypeEightYes:
		if required := modelResolutionSuffix(target.UpstreamModel); required != "" && !allRouteResolutions(target.Constraints.OutputResolutions, required) {
			return newVideoRouteContractError("route_contract_resolution", fmt.Sprintf("mapped model requires %s", required))
		}
		return nil
	case constant.ChannelTypeMegaByAI:
		if !routeResolutionsWithin(target.Constraints.OutputResolutions, "480p", "720p", "1080p", "4k") {
			return newVideoRouteContractError("route_contract_resolution", "MegaByAI routes support 480p, 720p, 1080p, and 4k")
		}
		return nil
	case constant.ChannelTypeOmegaAI:
		return validateOmegaAIVideoRoute(target)
	case constant.ChannelTypePaipu:
		return validatePaipuVideoRoute(target)
	case constant.ChannelTypeZ5API:
		return validateZ5APIVideoRoute(target)
	case constant.ChannelTypeZZone:
		return validateZZoneVideoRoute(target)
	case constant.ChannelTypeMikoto:
		return validateMikotoVideoRoute(target)
	case constant.ChannelTypeClmmMall:
		return validateClmmVideoRoute(target)
	case constant.ChannelTypeDimensio:
		return validateDimensioVideoRoute(target)
	case constant.ChannelTypeSecure:
		return validateSecureVideoRoute(channel.GetOtherSettings().SecureVideoGroup, target)
	case constant.ChannelTypeFFLink:
		return validateFFLinkVideoRoute(target)
	case constant.ChannelTypeWxArt:
		return validateWxArtVideoRoute(target)
	default:
		return nil
	}
}

func validateWxArtVideoRoute(target modelrouting.Target) error {
	modelName, ok := tasknewapivideo.AnalyzeWxArtModel(target.UpstreamModel)
	if !ok {
		return newVideoRouteContractError("route_contract_model", "mapped upstream model is not verified for WxArt")
	}
	maxDuration := 15
	maxImages, maxVideos, maxAudios, maxTotal := 9, 3, 3, 12
	allowedResolutions := []string{"480p", "720p", "1080p", "4k"}
	switch modelName {
	case "seedance2.0":
	case "seedance2.5":
		maxDuration = 30
		maxImages, maxVideos, maxAudios, maxTotal = 30, 10, 10, 50
		allowedResolutions = []string{"480p", "720p"}
	default:
		return newVideoRouteContractError("route_contract_model", "mapped upstream model is not verified for WxArt")
	}
	if !routeResolutionsWithin(target.Constraints.OutputResolutions, allowedResolutions...) {
		return newVideoRouteContractError("route_contract_resolution", "WxArt route resolution is unsupported")
	}
	if !routeDurationWithin(target.Constraints.Durations, 4, maxDuration) {
		return newVideoRouteContractError("route_contract_duration", fmt.Sprintf("WxArt routes require durations from 4 to %d seconds", maxDuration))
	}
	limits := target.Constraints.ReferenceLimits
	minimums := target.Constraints.ReferenceMinimums
	if limits.Images > maxImages || limits.Videos > maxVideos || limits.Audios > maxAudios ||
		minimums.Images > limits.Images || minimums.Videos > limits.Videos || minimums.Audios > limits.Audios ||
		routeReferenceTotalMax(target.Constraints) > maxTotal {
		return newVideoRouteContractError("route_contract_references", "WxArt route reference limits exceed the verified protocol")
	}
	return nil
}

func validateMikotoVideoRoute(target modelrouting.Target) error {
	contract, ok := tasknewapivideo.AnalyzeMikotoModel(target.UpstreamModel)
	if !ok {
		return newVideoRouteContractError("route_contract_model", "mapped upstream model is not verified for Mikoto")
	}
	if !allRouteResolutions(target.Constraints.OutputResolutions, contract.OutputResolution) {
		return newVideoRouteContractError("route_contract_resolution", fmt.Sprintf("Mikoto model requires %s", contract.OutputResolution))
	}
	if !routeDurationWithin(target.Constraints.Durations, 4, 15) {
		return newVideoRouteContractError("route_contract_duration", "Mikoto routes require durations from 4 to 15 seconds")
	}
	limits := target.Constraints.ReferenceLimits
	minimums := target.Constraints.ReferenceMinimums
	if limits.Images > 9 || limits.Videos > 3 || limits.Audios > 3 ||
		minimums.Images > limits.Images || minimums.Videos > limits.Videos || minimums.Audios > limits.Audios ||
		(contract.ReferenceTotalLimit > 0 && routeReferenceTotalMax(target.Constraints) > contract.ReferenceTotalLimit) {
		return newVideoRouteContractError("route_contract_references", "Mikoto route reference limits exceed the verified protocol")
	}
	return nil
}

func validateFFLinkVideoRoute(target modelrouting.Target) error {
	modelName := strings.TrimSpace(strings.ToLower(target.UpstreamModel))
	if modelName != "seedance-2.0" && modelName != "seedance-2.0-fast" && modelName != "seedance-2.0-mini" {
		return newVideoRouteContractError("route_contract_model", "mapped upstream model is not verified for FYLink")
	}

	switch modelName {
	case "seedance-2.0":
		if !routeResolutionsWithin(target.Constraints.OutputResolutions, "480p", "720p", "1080p") {
			return newVideoRouteContractError("route_contract_resolution", "FYLink seedance-2.0 routes support 480p, 720p, and 1080p")
		}
	case "seedance-2.0-fast":
		if !routeResolutionsWithin(target.Constraints.OutputResolutions, "480p", "720p") {
			return newVideoRouteContractError("route_contract_resolution", "FYLink seedance-2.0-fast routes support 480p and 720p")
		}
	case "seedance-2.0-mini":
		if !allRouteResolutions(target.Constraints.OutputResolutions, "720p") {
			return newVideoRouteContractError("route_contract_resolution", "FYLink seedance-2.0-mini routes require 720p")
		}
	}

	if !routeDurationWithin(target.Constraints.Durations, 4, 15) {
		return newVideoRouteContractError("route_contract_duration", "FYLink routes require durations from 4 to 15 seconds")
	}
	if modelName == "seedance-2.0" && containsRouteResolution(target.Constraints.OutputResolutions, "1080p") && !routeDurationWithin(target.Constraints.Durations, 4, 12) {
		return newVideoRouteContractError("route_contract_duration", "FYLink seedance-2.0 1080p routes support durations up to 12 seconds")
	}

	limits := target.Constraints.ReferenceLimits
	minimums := target.Constraints.ReferenceMinimums
	if limits.Images > 4 || limits.Videos > 3 || limits.Audios > 1 ||
		minimums.Images > limits.Images || minimums.Videos > limits.Videos || minimums.Audios > limits.Audios ||
		routeReferenceTotalMax(target.Constraints) > 8 ||
		(target.Constraints.ReferenceVideoTotalDurationSeconds != nil && *target.Constraints.ReferenceVideoTotalDurationSeconds > 15) {
		return newVideoRouteContractError("route_contract_references", "FYLink route reference limits exceed the verified protocol")
	}
	return nil
}

func validateOmegaAIVideoRoute(target modelrouting.Target) error {
	maxImages, maxVideos, maxAudios, ok := tasknewapivideo.OmegaAIModelMediaLimits(target.UpstreamModel)
	if !ok {
		return newVideoRouteContractError("route_contract_model", "mapped upstream model is not verified for OmegaAI")
	}
	if !allRouteResolutions(target.Constraints.OutputResolutions, "720p") {
		return newVideoRouteContractError("route_contract_resolution", "OmegaAI routes require 720p")
	}
	if routeDurationDeclared(target.Constraints.Durations) && !routeDurationWithin(target.Constraints.Durations, 1, relaycommon.MaxTaskDurationSeconds) {
		return newVideoRouteContractError("route_contract_duration", "OmegaAI route duration exceeds the task protocol limit")
	}
	limits := target.Constraints.ReferenceLimits
	minimums := target.Constraints.ReferenceMinimums
	if limits.Images > maxImages || limits.Videos > maxVideos || limits.Audios > maxAudios ||
		minimums.Images > limits.Images || minimums.Videos > limits.Videos || minimums.Audios > limits.Audios ||
		routeReferenceTotalMax(target.Constraints) > maxImages+maxVideos+maxAudios {
		return newVideoRouteContractError("route_contract_references", "OmegaAI route reference limits exceed the verified model capability")
	}
	return nil
}

func validateCangyuanVideoRoute(target modelrouting.Target) error {
	if strings.TrimSpace(target.UpstreamModel) == "" {
		return newVideoRouteContractError("route_contract_model", "Cangyuan mapped upstream model is required")
	}
	if !routeResolutionsWithin(target.Constraints.OutputResolutions, "480p", "720p") {
		return newVideoRouteContractError("route_contract_resolution", "Cangyuan routes support only 480p and 720p")
	}
	maxImages, maxVideos, maxAudios, maxTotal, maxVideoAudio := cangyuanReferenceLimits(target.UpstreamModel)
	if !routeDurationWithin(target.Constraints.Durations, 4, 15) {
		return newVideoRouteContractError("route_contract_duration", "Cangyuan routes require durations from 4 to 15 seconds")
	}
	limits := target.Constraints.ReferenceLimits
	minimums := target.Constraints.ReferenceMinimums
	if limits.Images > maxImages || limits.Videos > maxVideos || limits.Audios > maxAudios ||
		minimums.Images > limits.Images || minimums.Videos > limits.Videos || minimums.Audios > limits.Audios ||
		routeReferenceTotalMax(target.Constraints) > maxTotal ||
		(maxVideoAudio > 0 && target.Constraints.ReferenceVideoAudioTotalMax != nil && *target.Constraints.ReferenceVideoAudioTotalMax > maxVideoAudio) {
		return newVideoRouteContractError("route_contract_references", "Cangyuan route reference limits exceed the verified protocol")
	}
	if target.Constraints.ReferenceVideoTotalDurationSeconds != nil && *target.Constraints.ReferenceVideoTotalDurationSeconds > 15 {
		return newVideoRouteContractError("route_contract_references", "Cangyuan reference videos may total at most 15 seconds")
	}
	return nil
}

func cangyuanReferenceLimits(upstreamModel string) (images, videos, audios, total, videoAudio int) {
	images, videos, audios, total = 4, 3, 1, 8
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(upstreamModel)), "sd5-seedance-") {
		images, audios, total, videoAudio = 9, 3, 12, 3
	}
	return images, videos, audios, total, videoAudio
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

func validateZ5APIVideoRoute(target modelrouting.Target) error {
	if strings.TrimSpace(target.UpstreamModel) == "" {
		return newVideoRouteContractError("route_contract_model", "Z5API mapped upstream model is required")
	}
	if !routeDurationWithin(target.Constraints.Durations, 1, relaycommon.MaxTaskDurationSeconds) {
		return newVideoRouteContractError("route_contract_duration", "Z5API route duration exceeds the task protocol limit")
	}
	limits := target.Constraints.ReferenceLimits
	minimums := target.Constraints.ReferenceMinimums
	if limits.Images > 9 || limits.Videos > 3 || limits.Audios > 3 ||
		minimums.Images > limits.Images || minimums.Videos > limits.Videos || minimums.Audios > limits.Audios {
		return newVideoRouteContractError("route_contract_references", "Z5API route reference limits exceed the protocol")
	}
	return nil
}

func validateZZoneVideoRoute(target modelrouting.Target) error {
	if strings.TrimSpace(target.UpstreamModel) == "" {
		return newVideoRouteContractError("route_contract_model", "ZZone mapped upstream model is required")
	}
	if !routeDurationWithin(target.Constraints.Durations, 1, relaycommon.MaxTaskDurationSeconds) {
		return newVideoRouteContractError("route_contract_duration", "ZZone route duration exceeds the task protocol limit")
	}
	for _, ratio := range target.Constraints.AspectRatios {
		switch strings.ToLower(strings.TrimSpace(ratio)) {
		case "16:9", "9:16", "1:1":
		default:
			return newVideoRouteContractError("route_contract_ratio", "ZZone route aspect ratio is unsupported")
		}
	}
	limits := target.Constraints.ReferenceLimits
	minimums := target.Constraints.ReferenceMinimums
	if limits.Images > 4 || limits.Videos > 3 || limits.Audios > 1 ||
		minimums.Images > limits.Images || minimums.Videos > limits.Videos || minimums.Audios > limits.Audios {
		return newVideoRouteContractError("route_contract_references", "ZZone route reference limits exceed the documented protocol")
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
	modelContract, err := taskclmmmall.AnalyzeRouteModel(target.UpstreamModel)
	if err != nil {
		return newVideoRouteContractError("route_contract_model", err.Error())
	}
	limits := target.Constraints.ReferenceLimits
	if limits.Images > 9 || limits.Videos > 3 || limits.Audios > 3 || routeReferenceTotalMax(target.Constraints) > 15 {
		return newVideoRouteContractError("route_contract_references", "CLMM route reference limits exceed the verified protocol")
	}
	if !modelContract.ControlsDuration && !routeDurationWithin(target.Constraints.Durations, 4, 15) {
		return newVideoRouteContractError("route_contract_duration", "ordinary CLMM routes require durations from 4 to 15 seconds")
	}
	return nil
}

func validateDimensioVideoRoute(target modelrouting.Target) error {
	modelName := strings.TrimSpace(target.UpstreamModel)
	if modelName == "" {
		return newVideoRouteContractError("route_contract_model", "Dimensio route requires an upstream model")
	}
	limits := target.Constraints.ReferenceLimits
	totalLimit := 15
	if strings.HasPrefix(modelName, "jmg-") {
		totalLimit = 12
	}
	if limits.Images > 9 || limits.Videos > 3 || limits.Audios > 3 || routeReferenceTotalMax(target.Constraints) > totalLimit {
		return newVideoRouteContractError("route_contract_references", "Dimensio route reference limits exceed the verified protocol")
	}
	if strings.HasPrefix(modelName, "jmg-") {
		if !routeResolutionsWithin(target.Constraints.OutputResolutions, "720p", "1080p") ||
			(containsRouteResolution(target.Constraints.OutputResolutions, "1080p") && modelName != "jmg-video-seedance-2.0-vip") {
			return newVideoRouteContractError("route_contract_resolution", "Dimensio JMG route resolution is unsupported")
		}
	} else if modelName == "pxv-seedance-2.0-fast" || modelName == "pxv-seedance-2.0-mini" {
		if !routeResolutionsWithin(target.Constraints.OutputResolutions, "480p", "720p") {
			return newVideoRouteContractError("route_contract_resolution", "Dimensio PXV fast and mini support only 480p and 720p")
		}
	} else if !routeResolutionsWithin(target.Constraints.OutputResolutions, "480p", "720p", "1080p", "4k") {
		return newVideoRouteContractError("route_contract_resolution", "Dimensio PXV standard route resolution is unsupported")
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
		if minimums.Images < 1 || limits.Images > 9 || limits.Videos > 3 || limits.Audios > 3 || routeReferenceVideoAudioTotalMax(target.Constraints) > 3 || routeReferenceTotalMax(target.Constraints) > 12 {
			return newVideoRouteContractError("route_contract_references", "Secure discount route reference limits exceed the verified protocol")
		}
		if !routeResolutionsWithin(target.Constraints.OutputResolutions, "720p", "1080p", "4k") {
			return newVideoRouteContractError("route_contract_resolution", "Secure discount route resolution is unsupported")
		}
	case dto.SecureVideoGroupOverseas:
		if !routeDurationWithin(target.Constraints.Durations, 4, 15) {
			return newVideoRouteContractError("route_contract_duration", "Secure overseas routes require durations from 4 to 15 seconds")
		}
		if limits.Images > 9 || limits.Videos > 3 || limits.Audios > 3 || routeReferenceTotalMax(target.Constraints) > 12 {
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
		if limits.Images > 9 || limits.Videos != 0 || limits.Audios > 3 || routeReferenceTotalMax(target.Constraints) > 12 {
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

func routeReferenceTotalMax(constraints modelrouting.Constraints) int {
	if constraints.ReferenceTotalMax != nil {
		return *constraints.ReferenceTotalMax
	}
	return constraints.ReferenceLimits.Images + constraints.ReferenceLimits.Videos + constraints.ReferenceLimits.Audios
}

func routeReferenceVideoAudioTotalMax(constraints modelrouting.Constraints) int {
	if constraints.ReferenceVideoAudioTotalMax != nil {
		return *constraints.ReferenceVideoAudioTotalMax
	}
	return constraints.ReferenceLimits.Videos + constraints.ReferenceLimits.Audios
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

func routeDurationDeclared(constraint modelrouting.DurationConstraint) bool {
	return len(constraint.Values) > 0 || constraint.Min != nil || constraint.Max != nil
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

func newVideoRouteContractError(code, message string) *VideoRouteContractError {
	return &VideoRouteContractError{Code: code, Message: message}
}
