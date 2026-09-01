package controller

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/imageprofile"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/image_setting"
	"github.com/gin-gonic/gin"
)

type ImageRoutingPreviewRequest struct {
	Group          string `json:"group" binding:"required"`
	Model          string `json:"model" binding:"required"`
	Endpoint       string `json:"endpoint" binding:"required"`
	Size           string `json:"size"`
	Quality        string `json:"quality"`
	ResponseFormat string `json:"response_format"`
	N              uint   `json:"n"`
	InputImages    uint   `json:"input_images"`
	HasMask        bool   `json:"has_mask"`
}

type ImageRoutingPreviewResponse struct {
	Strategy          string                        `json:"strategy"`
	SKU               string                        `json:"sku"`
	SelectedChannelID *int                          `json:"selected_channel_id,omitempty"`
	Candidates        []service.ImageRouteCandidate `json:"candidates"`
}

func PreviewImageRouting(c *gin.Context) {
	var request ImageRoutingPreviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid image routing preview request"})
		return
	}
	endpoint := imageprofile.Endpoint(strings.TrimSpace(request.Endpoint))
	if endpoint != imageprofile.EndpointGenerations && endpoint != imageprofile.EndpointEdits {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "image endpoint is not supported"})
		return
	}
	if request.N == 0 {
		request.N = 1
	}
	resolved, err := image_setting.Resolve(image_setting.Selection{
		Model: request.Model, Endpoint: endpoint, Size: request.Size, Quality: request.Quality,
		ResponseFormat: request.ResponseFormat, N: request.N, InputImages: request.InputImages, HasMask: request.HasMask,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	decision, err := service.PreviewImageRoute(c, request.Group, request.Model, "/v1/images/"+string(endpoint), service.ImageRequestContext{Resolved: resolved})
	if err != nil && decision.Selected == nil {
		if errors.Is(err, service.ErrNoCompatibleImageChannel) || errors.Is(err, service.ErrNoEligibleImageChannel) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": err.Error(), "data": decision})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "image routing preview failed", "data": decision})
		return
	}
	response := ImageRoutingPreviewResponse{Strategy: string(decision.Strategy), SKU: resolved.SKUKey, Candidates: decision.Candidates}
	if decision.Selected != nil {
		response.SelectedChannelID = &decision.Selected.ChannelID
	}
	common.ApiSuccess(c, response)
}
