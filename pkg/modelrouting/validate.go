package modelrouting

import (
	"fmt"
	"sort"
	"strings"
)

type ValidationCode string

const (
	ValidationInvalidModel            ValidationCode = "invalid_model"
	ValidationInvalidGroup            ValidationCode = "invalid_group"
	ValidationInvalidOutputResolution ValidationCode = "invalid_output_resolution"
	ValidationInvalidDuration         ValidationCode = "invalid_duration"
	ValidationInvalidAspectRatio      ValidationCode = "invalid_aspect_ratio"
	ValidationInvalidReferenceLimit   ValidationCode = "invalid_reference_limit"
	ValidationInvalidUpscale          ValidationCode = "invalid_upscale"
	ValidationDefaultRouteUnavailable ValidationCode = "default_route_unavailable"
	ValidationTargetOverlap           ValidationCode = "routing_target_overlap"
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
			if err := validateConstraints(target.Constraints, maxDuration); err != nil {
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

	facts, err := ResolveFacts(policy.GroupName, FactsInput{CanonicalModel: policy.CanonicalModel}, policy.Defaults)
	if err != nil {
		return &ValidationError{
			Code:    ValidationDefaultRouteUnavailable,
			Field:   "defaults",
			Message: err.Error(),
		}
	}
	if len(Evaluate(policy, facts).CompatibleByChannel) == 0 {
		return &ValidationError{
			Code:    ValidationDefaultRouteUnavailable,
			Field:   "defaults",
			Message: "no enabled target matches the policy defaults",
		}
	}
	return nil
}

func validateConstraints(constraints Constraints, maxDuration int) error {
	if len(constraints.OutputResolutions) == 0 {
		return newValidationError(ValidationInvalidOutputResolution, "targets.constraints.output_resolutions", "at least one output resolution is required")
	}
	for _, resolution := range constraints.OutputResolutions {
		if !containsString(allowedResolutions, resolution) {
			return newValidationError(ValidationInvalidOutputResolution, "targets.constraints.output_resolutions", "output resolution is invalid")
		}
	}

	if err := validateDurationConstraint(constraints.Durations, maxDuration); err != nil {
		return err
	}
	for _, ratio := range constraints.AspectRatios {
		if !containsString(allowedRatios, ratio) {
			return newValidationError(ValidationInvalidAspectRatio, "targets.constraints.aspect_ratios", "aspect ratio is invalid")
		}
	}
	limits := constraints.ReferenceLimits
	if limits.Images < 0 || limits.Images > 9 || limits.Videos < 0 || limits.Videos > 3 || limits.Audios < 0 || limits.Audios > 3 {
		return newValidationError(ValidationInvalidReferenceLimit, "targets.constraints.reference_limits", "reference limits are invalid")
	}

	generationResolution := strings.TrimSpace(constraints.GenerationResolution)
	if constraints.Upscaled {
		if len(constraints.OutputResolutions) != 1 ||
			!containsString(allowedResolutions, generationResolution) ||
			generationResolution == constraints.OutputResolutions[0] {
			return newValidationError(ValidationInvalidUpscale, "targets.constraints.upscaled", "upscaled targets require one distinct generation resolution")
		}
		return nil
	}
	if generationResolution != "" {
		return newValidationError(ValidationInvalidUpscale, "targets.constraints.generation_resolution", "native targets cannot set a generation resolution")
	}
	return nil
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
		stringSetsOverlap(a.AspectRatios, b.AspectRatios, true)
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
