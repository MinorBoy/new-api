package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	secureAssetHTTPTimeout = 30 * time.Second
	secureAssetMaxResponse = 1 << 20
)

type SecureAssetProvider struct {
	httpClient *http.Client
}

func NewSecureAssetProvider(httpClient *http.Client) *SecureAssetProvider {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: secureAssetHTTPTimeout}
	} else if httpClient.Timeout == 0 {
		clonedClient := *httpClient
		clonedClient.Timeout = secureAssetHTTPTimeout
		httpClient = &clonedClient
	}
	return &SecureAssetProvider{httpClient: httpClient}
}

func (provider *SecureAssetProvider) Create(
	ctx context.Context,
	credential AssetProviderCredential,
	request CreateAssetRequest,
) (*ProviderAsset, error) {
	return provider.request(ctx, credential, "/v1/asset/create", map[string]string{
		"url": request.URL,
	}, request.IdempotencyKey)
}

func (provider *SecureAssetProvider) Get(
	ctx context.Context,
	credential AssetProviderCredential,
	upstreamAssetID string,
) (*ProviderAsset, error) {
	return provider.request(ctx, credential, "/v1/asset/get", map[string]string{
		"id": upstreamAssetID,
	}, "")
}

func (provider *SecureAssetProvider) request(
	ctx context.Context,
	credential AssetProviderCredential,
	path string,
	payload map[string]string,
	idempotencyKey string,
) (*ProviderAsset, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(credential.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("secure asset provider base URL is required")
	}
	apiKey := strings.TrimSpace(credential.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("secure asset provider API key is required")
	}

	body, err := common.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal secure asset request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create secure asset request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		httpRequest.Header.Set("Idempotency-Key", idempotencyKey)
	}

	response, err := provider.httpClient.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("send secure asset request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, secureAssetMaxResponse))
		return nil, fmt.Errorf("secure asset provider returned HTTP %d", response.StatusCode)
	}

	var result struct {
		Result struct {
			ID     string `json:"Id"`
			Status string `json:"Status"`
		} `json:"result"`
	}
	if err := common.DecodeJson(io.LimitReader(response.Body, secureAssetMaxResponse), &result); err != nil {
		return nil, fmt.Errorf("decode secure asset response: %w", err)
	}
	result.Result.ID = strings.TrimSpace(result.Result.ID)
	if result.Result.ID == "" {
		return nil, fmt.Errorf("secure asset response missing result.id")
	}

	providerStatus := strings.TrimSpace(result.Result.Status)
	status := ""
	switch strings.ToLower(providerStatus) {
	case "pending", "processing":
		status = model.AssetStatusProcessing
	case "active":
		status = model.AssetStatusActive
	case "failed":
		status = model.AssetStatusFailed
	case "unknown":
		status = model.AssetStatusUnknown
	default:
		return nil, fmt.Errorf("secure asset response has unsupported status %q", providerStatus)
	}

	return &ProviderAsset{
		ID:             result.Result.ID,
		Status:         status,
		ProviderStatus: providerStatus,
	}, nil
}
