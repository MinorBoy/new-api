package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func TaskMediaProxy(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	index, err := strconv.Atoi(c.Param("index"))
	if taskID == "" || err != nil || index < 0 {
		videoProxyError(c, http.StatusBadRequest, "invalid_request_error", "invalid task media request")
		return
	}

	task, exists, err := model.GetByTaskId(c.GetInt("id"), taskID)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to query task media %s: %s", taskID, err.Error()))
		videoProxyError(c, http.StatusInternalServerError, "server_error", "Failed to query task")
		return
	}
	if !exists || task == nil {
		videoProxyError(c, http.StatusNotFound, "invalid_request_error", "Task not found")
		return
	}
	if task.Status != model.TaskStatusSuccess {
		videoProxyError(c, http.StatusBadRequest, "invalid_request_error", "Task is not completed")
		return
	}

	kind := strings.ToLower(strings.TrimSpace(c.Param("kind")))
	mediaURL, err := service.ResolvePublicTaskMediaURL(task, index, kind)
	if err != nil {
		videoProxyError(c, http.StatusNotFound, "invalid_request_error", "Task media not found")
		return
	}
	if strings.HasPrefix(mediaURL, "data:") {
		if err := writePublicMediaDataURL(c, mediaURL, kind); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to decode task media data URL for task %s: %s", taskID, err.Error()))
			videoProxyError(c, http.StatusBadGateway, "server_error", "Failed to fetch task media")
		}
		return
	}

	// Result URLs are untrusted; outbound proxies cannot preserve dial-time SSRF pinning.
	client := publicMediaHTTPClient("")
	err = service.ValidateSSRFProtectedFetchURL(mediaURL)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Task media URL blocked for task %s: %v", taskID, err))
		videoProxyError(c, http.StatusForbidden, "server_error", "Task media request blocked")
		return
	}

	parsedURL, err := url.Parse(mediaURL)
	if err != nil {
		videoProxyError(c, http.StatusBadGateway, "server_error", "Failed to fetch task media")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		videoProxyError(c, http.StatusBadGateway, "server_error", "Failed to fetch task media")
		return
	}
	if rangeHeader := strings.TrimSpace(c.GetHeader("Range")); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to fetch task media for task %s", taskID))
		videoProxyError(c, http.StatusBadGateway, "server_error", "Failed to fetch task media")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		videoProxyError(c, http.StatusBadGateway, "server_error", "Upstream media request failed")
		return
	}
	contentType, ok := publicMediaContentType(kind, resp.Header.Get("Content-Type"))
	if !ok {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Upstream returned an unsafe task media type for task %s", taskID))
		videoProxyError(c, http.StatusBadGateway, "server_error", "Upstream media type is not supported")
		return
	}
	resp.Header.Set("Content-Type", contentType)

	copyPublicMediaResponseHeaders(c.Writer.Header(), resp.Header)
	c.Writer.Header().Set("Cache-Control", "private, no-store")
	c.Writer.Header().Set("X-Content-Type-Options", "nosniff")
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to stream task media for task %s: %s", taskID, err.Error()))
	}
}
