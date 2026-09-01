package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/imageprofile"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// TestChannelImageProfile runs the administrator-triggered compatibility
// contract and persists its status. Ordinary channel CRUD cannot write a
// passed result.
func TestChannelImageProfile(c *gin.Context) {
	id, err := strconv.Atoi(strings.TrimSpace(c.Param("id")))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid channel id"})
		return
	}
	var request service.ImageCompatibilityTestRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid image compatibility request"})
		return
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil || channel == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "channel not found"})
		return
	}
	result, testErr := service.RunImageCompatibilityTest(c.Request.Context(), channel, request)
	if testErr != nil {
		// Validation, catalog, and transport setup errors do not describe a
		// completed compatibility probe. Return them without writing a key that
		// could later be mistaken for an upstream test result.
		result.Status = imageprofile.CompatibilityFailed
		result.ErrorSummary = "image compatibility test failed"
		c.JSON(http.StatusOK, gin.H{"success": false, "message": result.ErrorSummary, "data": result})
		return
	}

	settings := channel.GetOtherSettings()
	if settings.ImageProfile != nil {
		if settings.ImageProfile.Compatibility == nil {
			settings.ImageProfile.Compatibility = make(map[string]imageprofile.Compatibility)
		}
		key := fmt.Sprintf("%s:%s", strings.TrimSpace(request.PublicModel), request.Endpoint)
		settings.ImageProfile.Compatibility[key] = imageprofile.Compatibility{
			Status: result.Status, ProfileVersion: result.ProfileVersion,
			ContractHash: result.ContractHash, TestedAt: result.TestedAt,
		}
		channel.SetOtherSettings(settings)
		if err := model.DB.Model(&model.Channel{}).Where("id = ?", id).Update("settings", channel.OtherSettings).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		model.InitChannelCache()
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": result})
}
