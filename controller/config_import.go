package controller

import (
	"bytes"
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func CreateConfigImportBatch(c *gin.Context) {
	var request dto.ConfigImportUploadRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeConfigImportError(c, err)
		return
	}
	encoded, err := common.Marshal(request.Document)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	detail, _, err := service.CreateConfigImportBatch(c, c.GetInt("id"), bytes.NewReader(encoded))
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	common.ApiSuccess(c, detail)
}

func ListConfigImportBatches(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	result, err := service.ListConfigImportBatches(c, page, pageSize)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func GetConfigImportBatch(c *gin.Context) {
	id, err := configImportID(c)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	result, err := service.GetConfigImportBatch(c, id)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func UpdateConfigImportBindings(c *gin.Context) {
	id, err := configImportID(c)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	request, err := service.DecodeConfigImportBindingRequest(c.Request.Body)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	result, err := service.UpdateConfigImportBindings(c, c.GetInt("id"), id, request.Bindings)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func UpdateConfigImportResolutions(c *gin.Context) {
	id, err := configImportID(c)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	request, err := service.DecodeConfigImportResolutionRequest(c.Request.Body)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	result, err := service.UpdateConfigImportResolutions(c, c.GetInt("id"), id, request.Resolutions)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func UpdateConfigImportRouteReviews(c *gin.Context) {
	id, err := configImportID(c)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	request, err := service.DecodeConfigImportRouteReviewRequest(c.Request.Body)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	result, err := service.UpdateConfigImportRouteReviews(c, c.GetInt("id"), id, request.Reviews)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func UpdateConfigImportPricingReview(c *gin.Context) {
	id, err := configImportID(c)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	request, err := service.DecodeConfigImportPricingReviewRequest(c.Request.Body)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	result, err := service.UpdateConfigImportPricingReview(c, c.GetInt("id"), id, request.SelectedGroups)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func StageConfigImportBatch(c *gin.Context) {
	id, err := configImportID(c)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	result, err := service.StageConfigImportBatch(c, c.GetInt("id"), id)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func ValidateConfigImportBatch(c *gin.Context) {
	StageConfigImportBatch(c)
}

func PublishConfigImportBatch(c *gin.Context) {
	id, err := configImportID(c)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	if err := service.PublishConfigImportBatch(c, id, c.GetInt("id")); err != nil {
		writeConfigImportError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"batch_id": id, "status": "published"})
}

func ActivateConfigImportBatch(c *gin.Context) {
	id, err := configImportID(c)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	detail, err := service.ActivateConfigImportBatch(c, id, c.GetInt("id"))
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	common.ApiSuccess(c, detail)
}

func RefreshConfigImportBatchCache(c *gin.Context) {
	id, err := configImportID(c)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	if err := service.RetryConfigImportBatchCache(c, id, c.GetInt("id")); err != nil {
		writeConfigImportError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"batch_id": id, "status": "published"})
}

func PreviewConfigImportRouteOwnershipBackfill(c *gin.Context) {
	report, err := service.PreviewConfigImportRouteOwnershipBackfill(c)
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	common.ApiSuccess(c, report)
}

func ApplyConfigImportRouteOwnershipBackfill(c *gin.Context) {
	report, err := service.ApplyConfigImportRouteOwnershipBackfill(c, c.GetInt("id"))
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	common.ApiSuccess(c, report)
}

func RollbackConfigImportRouteOwnershipBackfill(c *gin.Context) {
	report, err := service.RollbackConfigImportRouteOwnershipBackfill(c, c.GetInt("id"), c.Param("operation_id"))
	if err != nil {
		writeConfigImportError(c, err)
		return
	}
	common.ApiSuccess(c, report)
}

func configImportID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, &service.ConfigImportSchemaError{
			Code:    "SCHEMA_BATCH_ID",
			Message: "invalid config import batch id",
		}
	}
	return id, nil
}

func writeConfigImportError(c *gin.Context, err error) {
	status := http.StatusBadRequest
	var schemaErr *service.ConfigImportSchemaError
	if errors.As(err, &schemaErr) {
		switch schemaErr.Code {
		case "STALE_BASE_VERSION", "ACTIVATION_BLOCKED", "ACTIVATION_CONCURRENT", "ROUTE_OWNERSHIP_ROLLBACK_CONFLICT":
			status = http.StatusConflict
		case "ACTIVATION_CACHE_REFRESH_PENDING":
			status = http.StatusServiceUnavailable
		}
		payload := gin.H{"success": false, "code": schemaErr.Code, "message": schemaErr.Message}
		if schemaErr.Data != nil {
			data := schemaErr.Data
			if schemaErr.Code == "ACTIVATION_BLOCKED" {
				switch preview := schemaErr.Data.(type) {
				case dto.ConfigImportActivationPreview:
					data = gin.H{"blockers": preview.Blockers}
				case *dto.ConfigImportActivationPreview:
					data = gin.H{"blockers": preview.Blockers}
				}
			}
			payload["data"] = data
		}
		c.JSON(status, payload)
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "CONFIG_IMPORT_ERROR", "message": err.Error()})
}
