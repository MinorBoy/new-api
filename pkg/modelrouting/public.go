package modelrouting

import (
	"strings"
	"sync/atomic"
)

const PublicModelOwner = "doubao"

var hiddenSeedanceModels atomic.Value

func init() {
	hiddenSeedanceModels.Store(map[string]struct{}{})
}

func IsPublicSeedanceModel(modelName string) bool {
	switch modelName {
	case Seedance20, Seedance20Fast, Seedance20Mini, Seedance25:
		return true
	default:
		return false
	}
}

func IsHiddenSeedanceModel(modelName string) bool {
	if IsPublicSeedanceModel(modelName) {
		return false
	}
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	if strings.Contains(modelName, "seedance") {
		return true
	}
	_, ok := hiddenSeedanceModels.Load().(map[string]struct{})[modelName]
	return ok
}

// SetHiddenSeedanceModels replaces the upstream model IDs associated with the
// current public Seedance routing policies.
func SetHiddenSeedanceModels(modelNames []string) {
	next := make(map[string]struct{}, len(modelNames))
	for _, modelName := range modelNames {
		modelName = strings.ToLower(strings.TrimSpace(modelName))
		if modelName == "" || IsPublicSeedanceModel(modelName) {
			continue
		}
		next[modelName] = struct{}{}
	}
	hiddenSeedanceModels.Store(next)
}

func FilterPublicModels(modelNames []string) []string {
	seen := make(map[string]struct{}, len(modelNames))
	filtered := make([]string, 0, len(modelNames))
	for _, modelName := range modelNames {
		if IsHiddenSeedanceModel(modelName) {
			continue
		}
		if _, ok := seen[modelName]; ok {
			continue
		}
		seen[modelName] = struct{}{}
		filtered = append(filtered, modelName)
	}
	return filtered
}

// OrderPublicModels keeps non-Seedance models in their existing slots while
// presenting canonical public Seedance models in CanonicalModels order.
func OrderPublicModels(modelNames []string) []string {
	ordered := append([]string(nil), modelNames...)
	canonicalPositions := make([]int, 0, len(CanonicalModels))
	for index, modelName := range ordered {
		if IsPublicSeedanceModel(modelName) {
			canonicalPositions = append(canonicalPositions, index)
		}
	}
	if len(canonicalPositions) < 2 {
		return ordered
	}

	canonicalPresent := make(map[string]struct{}, len(canonicalPositions))
	for _, modelName := range ordered {
		if IsPublicSeedanceModel(modelName) {
			canonicalPresent[modelName] = struct{}{}
		}
	}
	canonicalIndex := 0
	for _, modelName := range CanonicalModels {
		if _, present := canonicalPresent[modelName]; !present {
			continue
		}
		ordered[canonicalPositions[canonicalIndex]] = modelName
		canonicalIndex++
	}
	return ordered
}
