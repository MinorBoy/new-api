package middleware

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

var projectAssetIDPattern = regexp.MustCompile(
	`^asset-(?:[0-9a-f]{32}|[0-9]{14}-[a-z0-9]{5})$`,
)

type VideoAssetRoutingService interface {
	ResolveActiveReferences(ctx context.Context, userID int, assetIDs []string) ([]service.AssetReferenceBinding, error)
}

func NewVideoAssetRouting(assetService VideoAssetRoutingService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request struct {
			Content []struct {
				Type     string `json:"type"`
				Role     string `json:"role"`
				ImageURL struct {
					URL string `json:"url"`
				} `json:"image_url"`
			} `json:"content"`
		}
		if err := common.UnmarshalBodyReusable(c, &request); err != nil {
			c.Next()
			return
		}
		assetIDs, hasPublicImages, err := parseVideoAssetReferences(request.Content)
		if err != nil {
			abortVideoAssetError(c, http.StatusBadRequest, service.AssetErrorCode(err), err)
			return
		}
		if len(assetIDs) == 0 {
			c.Next()
			return
		}
		if hasPublicImages {
			abortVideoAssetError(c, http.StatusBadRequest, service.AssetErrorReferenceMixed, fmt.Errorf("role assets cannot be mixed with public image URLs"))
			return
		}
		if len(assetIDs) > 9 {
			abortVideoAssetError(c, http.StatusBadRequest, service.AssetErrorLimitExceeded, fmt.Errorf("Secure enterprise video supports at most nine role assets"))
			return
		}
		resolved, err := assetService.ResolveActiveReferences(c.Request.Context(), c.GetInt("id"), assetIDs)
		if err != nil {
			code := service.AssetErrorCode(err)
			status := videoAssetErrorStatus(code)
			abortVideoAssetError(c, status, code, err)
			return
		}
		mappings := make(map[string]string, len(resolved))
		channelID := 0
		for _, binding := range resolved {
			if channelID == 0 {
				channelID = binding.ChannelID
			} else if channelID != binding.ChannelID {
				abortVideoAssetError(c, http.StatusConflict, service.AssetErrorChannelMismatch, fmt.Errorf("role assets are bound to different channels"))
				return
			}
			mappings[binding.AssetID] = binding.UpstreamAssetID
		}
		common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, fmt.Sprintf("%d", channelID))
		common.SetContextKey(c, constant.ContextKeyVideoAssetMappings, mappings)
		c.Next()
	}
}

func parseVideoAssetReferences(content []struct {
	Type     string `json:"type"`
	Role     string `json:"role"`
	ImageURL struct {
		URL string `json:"url"`
	} `json:"image_url"`
}) ([]string, bool, error) {
	assetIDs := make([]string, 0)
	seen := make(map[string]struct{})
	hasPublicImages := false
	for _, item := range content {
		if item.Type != "image_url" || strings.TrimSpace(item.Role) != "reference_image" {
			continue
		}
		rawURL := strings.TrimSpace(item.ImageURL.URL)
		if strings.HasPrefix(rawURL, "asset://") {
			assetID := strings.TrimPrefix(rawURL, "asset://")
			if !projectAssetIDPattern.MatchString(assetID) {
				return nil, false, &service.AssetServiceError{Code: service.AssetErrorNotFound, Err: fmt.Errorf("invalid project asset reference")}
			}
			if _, ok := seen[assetID]; ok {
				return nil, false, &service.AssetServiceError{Code: service.AssetErrorLimitExceeded, Err: fmt.Errorf("duplicate role asset reference")}
			}
			seen[assetID] = struct{}{}
			assetIDs = append(assetIDs, assetID)
			continue
		}
		if rawURL != "" {
			hasPublicImages = true
		}
	}
	return assetIDs, hasPublicImages, nil
}

func GetVideoAssetMappings(c *gin.Context) (map[string]string, bool) {
	mappings, ok := common.GetContextKeyType[map[string]string](c, constant.ContextKeyVideoAssetMappings)
	return mappings, ok
}

func videoAssetErrorStatus(code string) int {
	switch code {
	case service.AssetErrorNotFound:
		return http.StatusNotFound
	case service.AssetErrorNotActive, service.AssetErrorChannelMismatch:
		return http.StatusConflict
	case service.AssetErrorChannelUnavailable:
		return http.StatusServiceUnavailable
	case service.AssetErrorUpstream:
		return http.StatusBadGateway
	default:
		return http.StatusBadRequest
	}
}

func abortVideoAssetError(c *gin.Context, status int, code string, err error) {
	if status == 0 {
		status = http.StatusBadRequest
	}
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": code, "message": err.Error()}})
}
