package modelrouting

import (
	"fmt"
	"sort"
	"strings"
)

type ValidationCode string

const (
	ValidationInvalidModel                 ValidationCode = "invalid_model"
	ValidationInvalidGroup                 ValidationCode = "invalid_group"
	ValidationInvalidOutputResolution      ValidationCode = "invalid_output_resolution"
	ValidationInvalidDuration              ValidationCode = "invalid_duration"
	ValidationInvalidAspectRatio           ValidationCode = "invalid_aspect_ratio"
	ValidationInvalidInputMode             ValidationCode = "invalid_input_mode"
	ValidationInvalidReferenceLimit        ValidationCode = "invalid_reference_limit"
	ValidationInvalidMinimumExpectedMargin ValidationCode = "invalid_minimum_expected_margin"
	ValidationDefaultRouteUnavailable      ValidationCode = "default_route_unavailable"
	ValidationTargetOverlap                ValidationCode = "routing_target_overlap"
)

type ValidationError struct {
	Code      ValidationCode
	Field     string
	TargetIDs []int
	Message   string
}

func (e *ValidationError) Error() string {
	return e.Message
}

var allowedResolutions = []string{"480p", "720p", "1080p", "4k"}
var allowedRatios = []string{"16:9", "4:3", "1:1", "3:4", "9:16", "21:9", "adaptive"}
var allowedInputModes = []InputMode{InputModeText, InputModeFirstFrame, InputModeFirstLastFrames, InputModeOmniReference}
var allowedReferenceModes = []string{"first_last_frames", "omni_reference", "agentic"}

func ValidatePolicy(policy PolicySnapshot, maxDuration int) error {
	if !containsString(CanonicalModels, policy.CanonicalModel) {
		return newValidationError(ValidationInvalidModel, "model", "model must be a supported canonical Seedance model")
	}
	groupName := strings.TrimSpace(policy.GroupName)
	if groupName == "" || strings.EqualFold(groupName, "auto") {
		return newValidationError(ValidationInvalidGroup, "group_name", "group_name must be a concrete group")
	}
	if !containsString(allowedResolutions, policy.Defaults.OutputResolution) {
		return newValidationError(ValidationInvalidOutputResolution, "defaults.output_resolution", "default output resolution is invalid")
	}
	if policy.Defaults.DurationSeconds < 1 || policy.Defaults.DurationSeconds > maxDuration {
		return newValidationError(ValidationInvalidDuration, "defaults.duration_seconds", "default duration is invalid")
	}
	if !containsString(allowedRatios, policy.Defaults.AspectRatio) {
		return newValidationError(ValidationInvalidAspectRatio, "defaults.aspect_ratio", "default aspect ratio is invalid")
	}

	for _, targets := range policy.TargetsByChannel {
		for _, target := range targets {
			if target.MinimumExpectedMarginBPS != nil && (*target.MinimumExpectedMarginBPS < 0 || *target.MinimumExpectedMarginBPS > 10_000) {
				return newValidationError(ValidationInvalidMinimumExpectedMargin, "targets.minimum_expected_margin_bps", "minimum expected margin must be between 0 and 10000 basis points")
			}
			if err := validateConstraints(target.Constraints, maxDuration, policy.CanonicalModel); err != nil {
				return err
			}
		}
	}
	if err := validateOverlaps(policy); err != nil {
		return err
	}
	if !policy.Enabled {
		return nil
	}

	for _, targets := range policy.TargetsByChannel {
		for _, target := range targets {
			if !target.Enabled {
				continue
			}
			for _, input := range representativeFactsInputs(target.Constraints) {
				input.CanonicalModel = policy.CanonicalModel
				facts, err := ResolveFacts(policy.GroupName, input, policy.Defaults)
				if err != nil {
					continue
				}
				if len(Evaluate(policy, facts).CompatibleByChannel) > 0 {
					return nil
				}
			}
		}
	}
	return &ValidationError{
		Code:    ValidationDefaultRouteUnavailable,
		Field:   "defaults",
		Message: "no enabled target matches the policy defaults",
	}
}

func validateConstraints(constraints Constraints, maxDuration int, canonicalModel string) error {
	if len(constraints.OutputResolutions) == 0 {
		return newValidationError(ValidationInvalidOutputResolution, "targets.constraints.output_resolutions", "at least one output resolution is required")
	}
	allowedModelResolutions := allowedResolutions
	modelMaxDuration := maxDuration
	maxImages, maxVideos, maxAudios := 9, 3, 3
	if canonicalModel == Seedance25 {
		allowedModelResolutions = []string{"480p", "720p"}
		if modelMaxDuration > 30 {
			modelMaxDuration = 30
		}
		maxImages, maxVideos, maxAudios = 30, 10, 10
	}
	for _, resolution := range constraints.OutputResolutions {
		if !containsString(allowedModelResolutions, resolution) {
			return newValidationError(ValidationInvalidOutputResolution, "targets.constraints.output_resolutions", "output resolution is invalid")
		}
	}

	if err := validateDurationConstraint(constraints.Durations, modelMaxDuration); err != nil {
		return err
	}
	for _, ratio := range constraints.AspectRatios {
		if !containsString(allowedRatios, ratio) {
			return newValidationError(ValidationInvalidAspectRatio, "targets.constraints.aspect_ratios", "aspect ratio is invalid")
		}
	}
	for _, mode := range constraints.InputModes {
		if !containsInputMode(allowedInputModes, mode) {
			return newValidationError(ValidationInvalidInputMode, "targets.constraints.input_modes", "input mode is invalid")
		}
	}
	limits := constraints.ReferenceLimits
	if limits.Images < 0 || limits.Images > maxImages || limits.Videos < 0 || limits.Videos > maxVideos || limits.Audios < 0 || limits.Audios > maxAudios {
		return newValidationError(ValidationInvalidReferenceLimit, "targets.constraints.reference_limits", "reference limits are invalid")
	}
	minimums := constraints.ReferenceMinimums
	if minimums.Images < 0 || minimums.Images > limits.Images ||
		minimums.Videos < 0 || minimums.Videos > limits.Videos ||
		minimums.Audios < 0 || minimums.Audios > limits.Audios {
		return newValidationError(ValidationInvalidReferenceLimit, "targets.constraints.reference_minimums", "reference minimums are invalid")
	}
	if constraints.ReferenceTotalMax != nil && (*constraints.ReferenceTotalMax < 0 || *constraints.ReferenceTotalMax > limits.Images+limits.Videos+limits.Audios) {
		return newValidationError(ValidationInvalidReferenceLimit, "targets.constraints.reference_total_max", "reference total maximum is invalid")
	}
	if constraints.ReferenceVideoAudioTotalMax != nil {
		if *constraints.ReferenceVideoAudioTotalMax < 0 || *constraints.ReferenceVideoAudioTotalMax > limits.Videos+limits.Audios {
			return newValidationError(ValidationInvalidReferenceLimit, "targets.constraints.reference_video_audio_total_max", "reference video and audio total maximum is invalid")
		}
		if constraints.ReferenceTotalMax != nil && *constraints.ReferenceTotalMax > limits.Images+*constraints.ReferenceVideoAudioTotalMax {
			return newValidationError(ValidationInvalidReferenceLimit, "targets.constraints.reference_total_max", "reference aggregate maxima conflict")
		}
	}
	if constraints.ReferenceVideoTotalDurationSeconds != nil && (*constraints.ReferenceVideoTotalDurationSeconds < 0 || (limits.Videos == 0 && *constraints.ReferenceVideoTotalDurationSeconds != 0)) {
		return newValidationError(ValidationInvalidReferenceLimit, "targets.constraints.reference_video_total_duration_seconds", "reference video duration limit is invalid")
	}
	for _, mode := range constraints.ReferenceModes {
		if !containsString(allowedReferenceModes, mode) {
			return newValidationError(ValidationInvalidReferenceLimit, "targets.constraints.reference_modes", "reference mode is invalid")
		}
	}
	if len(representativeFactsInputs(constraints)) == 0 {
		return newValidationError(ValidationInvalidReferenceLimit, "targets.constraints.reference_minimums", "reference minimums cannot produce a supported input mode")
	}
	return nil
}

func representativeFactsInputs(constraints Constraints) []FactsInput {
	modes := constraints.InputModes
	if len(modes) == 0 {
		modes = []InputMode{InputModeText, InputModeFirstFrame, InputModeFirstLastFrames, InputModeOmniReference}
	}
	inputs := make([]FactsInput, 0, len(modes))
	for _, mode := range modes {
		refs := constraints.ReferenceMinimums
		switch mode {
		case InputModeText:
			if refs.Images > 0 || refs.Videos > 0 || refs.Audios > 0 {
				continue
			}
			refs = ReferenceLimits{}
		case InputModeFirstFrame:
			if refs.Images > 1 || refs.Videos > 0 || refs.Audios > 0 {
				continue
			}
			refs.Images = 1
		case InputModeFirstLastFrames:
			if refs.Images > 2 || refs.Videos > 0 || refs.Audios > 0 {
				continue
			}
			refs.Images = 2
		case InputModeOmniReference:
			if refs.Images+refs.Videos+refs.Audios == 0 {
				refs.Images = 1
			}
			if refs.Audios > 0 && refs.Images+refs.Videos == 0 {
				refs.Images = 1
			}
		default:
			continue
		}
		if refs.Images > constraints.ReferenceLimits.Images || refs.Videos > constraints.ReferenceLimits.Videos || refs.Audios > constraints.ReferenceLimits.Audios {
			continue
		}
		inputs = append(inputs, FactsInput{
			InputMode:       mode,
			ReferenceImages: refs.Images,
			ReferenceVideos: refs.Videos,
			ReferenceAudios: refs.Audios,
		})
	}
	return inputs
}

func validateDurationConstraint(constraint DurationConstraint, maxDuration int) error {
	hasValues := len(constraint.Values) > 0
	hasMin := constraint.Min != nil
	hasMax := constraint.Max != nil
	if hasValues == (hasMin || hasMax) || hasMin != hasMax {
		return newValidationError(ValidationInvalidDuration, "targets.constraints.durations", "duration must use either values or an inclusive range")
	}
	if hasValues {
		for _, duration := range constraint.Values {
			if duration < 1 || duration > maxDuration {
				return newValidationError(ValidationInvalidDuration, "targets.constraints.durations.values", "duration value is out of range")
			}
		}
		return nil
	}
	if *constraint.Min < 1 || *constraint.Max > maxDuration || *constraint.Min > *constraint.Max {
		return newValidationError(ValidationInvalidDuration, "targets.constraints.durations", "duration range is invalid")
	}
	return nil
}

func validateOverlaps(policy PolicySnapshot) error {
	for _, targets := range policy.TargetsByChannel {
		for left := 0; left < len(targets); left++ {
			for right := left + 1; right < len(targets); right++ {
				a, b := targets[left], targets[right]
				if !a.Enabled || !b.Enabled || a.Priority != b.Priority || !constraintsOverlap(a.Constraints, b.Constraints) {
					continue
				}
				ids := []int{a.ID, b.ID}
				if a.ID == 0 || b.ID == 0 {
					ids = []int{-(left + 1), -(right + 1)}
				} else {
					sort.Ints(ids)
				}
				return &ValidationError{
					Code:      ValidationTargetOverlap,
					Field:     "targets",
					TargetIDs: ids,
					Message:   fmt.Sprintf("targets %d and %d overlap at the same channel priority", ids[0], ids[1]),
				}
			}
		}
	}
	return nil
}

func constraintsOverlap(a, b Constraints) bool {
	return stringSetsOverlap(a.OutputResolutions, b.OutputResolutions, false) &&
		durationsOverlap(a.Durations, b.Durations) &&
		stringSetsOverlap(a.AspectRatios, b.AspectRatios, true) &&
		inputModeSetsOverlap(a.InputModes, b.InputModes) &&
		referenceRangesOverlap(a.ReferenceMinimums, a.ReferenceLimits, b.ReferenceMinimums, b.ReferenceLimits)
}

func inputModeSetsOverlap(a, b []InputMode) bool {
	if len(a) == 0 || len(b) == 0 {
		return true
	}
	for _, left := range a {
		if containsInputMode(b, left) {
			return true
		}
	}
	return false
}

func referenceRangesOverlap(aMinimums, aMaximums, bMinimums, bMaximums ReferenceLimits) bool {
	return integerRangesOverlap(aMinimums.Images, aMaximums.Images, bMinimums.Images, bMaximums.Images) &&
		integerRangesOverlap(aMinimums.Videos, aMaximums.Videos, bMinimums.Videos, bMaximums.Videos) &&
		integerRangesOverlap(aMinimums.Audios, aMaximums.Audios, bMinimums.Audios, bMaximums.Audios)
}

func integerRangesOverlap(aMin, aMax, bMin, bMax int) bool {
	return aMin <= bMax && bMin <= aMax
}

func stringSetsOverlap(a, b []string, emptyMeansAny bool) bool {
	if emptyMeansAny && (len(a) == 0 || len(b) == 0) {
		return true
	}
	for _, left := range a {
		if containsString(b, left) {
			return true
		}
	}
	return false
}

func durationsOverlap(a, b DurationConstraint) bool {
	if len(a.Values) > 0 && len(b.Values) > 0 {
		for _, value := range a.Values {
			for _, other := range b.Values {
				if value == other {
					return true
				}
			}
		}
		return false
	}
	if len(a.Values) > 0 {
		return valuesOverlapRange(a.Values, b.Min, b.Max)
	}
	if len(b.Values) > 0 {
		return valuesOverlapRange(b.Values, a.Min, a.Max)
	}
	return a.Min != nil && a.Max != nil && b.Min != nil && b.Max != nil && *a.Min <= *b.Max && *b.Min <= *a.Max
}

func valuesOverlapRange(values []int, minValue, maxValue *int) bool {
	if minValue == nil || maxValue == nil {
		return false
	}
	for _, value := range values {
		if value >= *minValue && value <= *maxValue {
			return true
		}
	}
	return false
}

func newValidationError(code ValidationCode, field, message string) error {
	return &ValidationError{Code: code, Field: field, Message: message}
}
