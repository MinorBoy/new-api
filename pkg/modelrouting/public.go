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
	case Seedance20, Seedance20Fast, Seedance20Mini:
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
