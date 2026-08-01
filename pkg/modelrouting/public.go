package modelrouting

const PublicModelOwner = "doubao"

func IsPublicModel(modelName string) bool {
	switch modelName {
	case Seedance20, Seedance20Fast, Seedance20Mini:
		return true
	default:
		return false
	}
}

func FilterPublicModels(modelNames []string) []string {
	available := make(map[string]struct{}, len(modelNames))
	for _, modelName := range modelNames {
		available[modelName] = struct{}{}
	}

	filtered := make([]string, 0, len(CanonicalModels))
	for _, modelName := range CanonicalModels {
		if _, ok := available[modelName]; ok {
			filtered = append(filtered, modelName)
		}
	}
	return filtered
}
