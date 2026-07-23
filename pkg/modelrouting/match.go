package modelrouting

import (
	"fmt"
	"sort"
	"strings"
)

func ResolveFacts(group string, input FactsInput, defaults Defaults) (Facts, error) {
	resolution := defaults.OutputResolution
	if input.OutputResolution != nil {
		resolution = *input.OutputResolution
	}
	duration := defaults.DurationSeconds
	if input.DurationSeconds != nil {
		duration = *input.DurationSeconds
	}
	ratio := defaults.AspectRatio
	if input.AspectRatio != nil {
		ratio = *input.AspectRatio
	}

	facts := Facts{
		GroupName:         strings.TrimSpace(group),
		CanonicalModel:    strings.TrimSpace(input.CanonicalModel),
		OutputResolution:  strings.ToLower(strings.TrimSpace(resolution)),
		DurationSeconds:   duration,
		AspectRatio:       strings.ToLower(strings.TrimSpace(ratio)),
		References:        ReferenceLimits{Images: input.ReferenceImages, Videos: input.ReferenceVideos, Audios: input.ReferenceAudios},
		RequireRealPerson: input.RequireRealPerson,
	}
	if facts.GroupName == "" || facts.CanonicalModel == "" || facts.OutputResolution == "" || facts.DurationSeconds == 0 || facts.AspectRatio == "" {
		return Facts{}, fmt.Errorf("routing facts are incomplete")
	}
	return facts, nil
}

func Evaluate(snapshot PolicySnapshot, facts Facts) Evaluation {
	result := Evaluation{
		CompatibleByChannel: map[int]Target{},
		MismatchCounts:      map[MismatchReason]int{},
	}
	for channelID, targets := range snapshot.TargetsByChannel {
		ordered := append([]Target(nil), targets...)
		sort.SliceStable(ordered, func(i, j int) bool {
			if ordered[i].Priority != ordered[j].Priority {
				return ordered[i].Priority > ordered[j].Priority
			}
			return ordered[i].ID < ordered[j].ID
		})
		for _, target := range ordered {
			if !target.Enabled {
				continue
			}
			reasons := Match(target.Constraints, facts)
			if len(reasons) == 0 {
				if _, selected := result.CompatibleByChannel[channelID]; !selected {
					result.CompatibleByChannel[channelID] = target
				}
				continue
			}
			for _, reason := range reasons {
				result.MismatchCounts[reason]++
			}
		}
	}
	return result
}

func Match(constraints Constraints, facts Facts) []MismatchReason {
	reasons := make([]MismatchReason, 0, 7)
	if !containsString(constraints.OutputResolutions, facts.OutputResolution) {
		reasons = append(reasons, MismatchResolution)
	}
	if !durationMatches(constraints.Durations, facts.DurationSeconds) {
		reasons = append(reasons, MismatchDuration)
	}
	if len(constraints.AspectRatios) > 0 && !containsString(constraints.AspectRatios, facts.AspectRatio) {
		reasons = append(reasons, MismatchAspectRatio)
	}
	if facts.References.Images > constraints.ReferenceLimits.Images {
		reasons = append(reasons, MismatchReferenceImages)
	}
	if facts.References.Videos > constraints.ReferenceLimits.Videos {
		reasons = append(reasons, MismatchReferenceVideos)
	}
	if facts.References.Audios > constraints.ReferenceLimits.Audios {
		reasons = append(reasons, MismatchReferenceAudios)
	}
	if facts.RequireRealPerson && (constraints.SupportsRealPerson == nil || !*constraints.SupportsRealPerson) {
		reasons = append(reasons, MismatchRealPerson)
	}
	return reasons
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func durationMatches(constraint DurationConstraint, duration int) bool {
	if len(constraint.Values) > 0 {
		for _, value := range constraint.Values {
			if value == duration {
				return true
			}
		}
		return false
	}
	if constraint.Min == nil || constraint.Max == nil {
		return false
	}
	return duration >= *constraint.Min && duration <= *constraint.Max
}
