package newapivideo

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/service"
)

func (a *FYLinkTaskAdaptor) CancelTask(ctx context.Context, baseURL string, key string, taskID string, proxy string) (*http.Response, error) {
	if strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task id")
	}
	requestURL := strings.TrimRight(baseURL, "/") + "/v1/videos/jobs/" + url.PathEscape(taskID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}
