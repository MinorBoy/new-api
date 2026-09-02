package controller

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// GetImagePricingCostSummary exposes the safe, aggregated supplier-cost view
// consumed by the image pricing workbench.
func GetImagePricingCostSummary(c *gin.Context) {
	models := make([]string, 0)
	for _, modelName := range c.QueryArray("model") {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		if len(modelName) > 191 {
			writeCostAccountingError(c, errors.New("model query is too long"))
			return
		}
		models = append(models, modelName)
		if len(models) > 100 {
			writeCostAccountingError(c, errors.New("too many model query values"))
			return
		}
	}
	summary, err := service.ListImagePricingCostSummary(models)
	if err != nil {
		writeCostAccountingError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
}
