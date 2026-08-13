package controller

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type AssetControllerService interface {
	Create(ctx context.Context, userID int, tokenID int, input service.AssetCreateInput) (*service.AssetView, error)
	Get(ctx context.Context, userID int, assetID string) (*service.AssetView, error)
	List(ctx context.Context, userID int, input service.AssetListInput) ([]service.AssetView, int64, error)
	Refresh(ctx context.Context, userID int, assetID string) (*service.AssetView, error)
}

type AssetController struct {
	service AssetControllerService
}

func NewAssetController(assetService AssetControllerService) *AssetController {
	if assetService == nil {
		assetService = service.NewAssetService(nil, nil)
	}
	return &AssetController{service: assetService}
}

func (controller *AssetController) Create(c *gin.Context) {
	var request struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		writeAssetError(c, http.StatusBadRequest, "asset_invalid_request", "invalid asset request", nil)
		return
	}
	view, err := controller.service.Create(c.Request.Context(), c.GetInt("id"), c.GetInt("token_id"), service.AssetCreateInput{
		Type:           request.Type,
		URL:            request.URL,
		IdempotencyKey: c.GetHeader("Idempotency-Key"),
	})
	if err != nil {
		controller.writeServiceError(c, err, view)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "message": "", "data": view})
}

func (controller *AssetController) Get(c *gin.Context) {
	view, err := controller.service.Get(c.Request.Context(), c.GetInt("id"), c.Param("asset_id"))
	if err != nil {
		controller.writeServiceError(c, err, view)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": view})
}

func (controller *AssetController) List(c *gin.Context) {
	page, err := positiveQueryInt(c.Query("page"), 1)
	if err != nil {
		writeAssetError(c, http.StatusBadRequest, "asset_invalid_request", "page must be a positive integer", nil)
		return
	}
	pageSize, err := positiveQueryInt(c.Query("page_size"), 20)
	if err != nil {
		writeAssetError(c, http.StatusBadRequest, "asset_invalid_request", "page_size must be a positive integer", nil)
		return
	}
	views, total, err := controller.service.List(c.Request.Context(), c.GetInt("id"), service.AssetListInput{
		Type:     c.Query("type"),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		controller.writeServiceError(c, err, nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"items":     views,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

func (controller *AssetController) Refresh(c *gin.Context) {
	view, err := controller.service.Refresh(c.Request.Context(), c.GetInt("id"), c.Param("asset_id"))
	if err != nil {
		controller.writeServiceError(c, err, view)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": view})
}

func (controller *AssetController) writeServiceError(c *gin.Context, err error, view *service.AssetView) {
	code := service.AssetErrorCode(err)
	status := http.StatusInternalServerError
	switch code {
	case service.AssetErrorInvalidURL, service.AssetErrorTypeUnsupported:
		status = http.StatusBadRequest
	case service.AssetErrorNotFound:
		status = http.StatusNotFound
	case service.AssetErrorNotActive, service.AssetErrorChannelMismatch, service.AssetErrorIdempotencyConflict:
		status = http.StatusConflict
	case service.AssetErrorChannelUnavailable:
		status = http.StatusServiceUnavailable
	case service.AssetErrorUpstream:
		status = http.StatusBadGateway
	default:
		code = "asset_internal_error"
	}
	writeAssetError(c, status, code, err.Error(), view)
}

func writeAssetError(c *gin.Context, status int, code string, message string, data any) {
	payload := gin.H{"success": false, "code": code, "message": message}
	if data != nil {
		payload["data"] = data
	}
	c.JSON(status, payload)
}

func positiveQueryInt(raw string, defaultValue int) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, strconv.ErrSyntax
	}
	return value, nil
}
