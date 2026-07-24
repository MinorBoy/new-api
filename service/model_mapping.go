package service

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

func ResolveMappedModel(originModel, mappingJSON string) (string, bool, error) {
	modelMap := make(map[string]string)
	if err := common.UnmarshalJsonStr(mappingJSON, &modelMap); err != nil {
		return "", false, err
	}

	normalizedOrigin := strings.TrimSpace(originModel)
	currentModel := normalizedOrigin
	visitedModels := map[string]struct{}{currentModel: {}}
	for {
		mappedModel, exists := modelMap[currentModel]
		mappedModel = strings.TrimSpace(mappedModel)
		if !exists || mappedModel == "" {
			return currentModel, currentModel != normalizedOrigin, nil
		}
		if mappedModel == currentModel {
			return currentModel, currentModel != normalizedOrigin, nil
		}
		if _, visited := visitedModels[mappedModel]; visited {
			return "", false, errors.New("model_mapping_contains_cycle")
		}
		visitedModels[mappedModel] = struct{}{}
		currentModel = mappedModel
	}
}
