package service

import "context"

type AssetProviderCredential struct {
	BaseURL string
	APIKey  string
}

type CreateAssetRequest struct {
	URL            string
	IdempotencyKey string
}

type ProviderAsset struct {
	ID             string
	Status         string
	ProviderStatus string
}

type AssetProviderAdapter interface {
	Create(ctx context.Context, credential AssetProviderCredential, request CreateAssetRequest) (*ProviderAsset, error)
	Get(ctx context.Context, credential AssetProviderCredential, upstreamAssetID string) (*ProviderAsset, error)
}
