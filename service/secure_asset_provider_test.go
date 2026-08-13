package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecureAssetProviderCreateSendsRequiredRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/asset/create", r.URL.Path)
		assert.Equal(t, "Bearer secure-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "request-unique-key", r.Header.Get("Idempotency-Key"))

		var request struct {
			URL string `json:"url"`
		}
		require.NoError(t, common.DecodeJson(r.Body, &request))
		assert.Equal(t, "https://example.com/character.png", request.URL)

		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"result":{"Id":"asset-local-create","Status":"Pending"}}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	provider := NewSecureAssetProvider(server.Client())
	asset, err := provider.Create(context.Background(), AssetProviderCredential{
		BaseURL: server.URL,
		APIKey:  "secure-key",
	}, CreateAssetRequest{
		URL:            "https://example.com/character.png",
		IdempotencyKey: "request-unique-key",
	})

	require.NoError(t, err)
	assert.Equal(t, "asset-local-create", asset.ID)
	assert.Equal(t, model.AssetStatusProcessing, asset.Status)
	assert.Equal(t, "Pending", asset.ProviderStatus)
}

func TestSecureAssetProviderGetSendsAssetIDAndMapsStatuses(t *testing.T) {
	tests := []struct {
		providerStatus string
		wantStatus     string
	}{
		{providerStatus: "Pending", wantStatus: model.AssetStatusProcessing},
		{providerStatus: "Processing", wantStatus: model.AssetStatusProcessing},
		{providerStatus: "Active", wantStatus: model.AssetStatusActive},
		{providerStatus: "Failed", wantStatus: model.AssetStatusFailed},
		{providerStatus: "Unknown", wantStatus: model.AssetStatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.providerStatus, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/v1/asset/get", r.URL.Path)
				assert.Equal(t, "Bearer secure-key", r.Header.Get("Authorization"))
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				var request struct {
					ID string `json:"id"`
				}
				require.NoError(t, common.DecodeJson(r.Body, &request))
				assert.Equal(t, "asset-local-get", request.ID)

				w.Header().Set("Content-Type", "application/json")
				response, err := common.Marshal(map[string]any{
					"result": map[string]any{
						"Id":     "asset-local-get",
						"Status": tt.providerStatus,
					},
				})
				require.NoError(t, err)
				_, err = w.Write(response)
				require.NoError(t, err)
			}))
			defer server.Close()

			provider := NewSecureAssetProvider(server.Client())
			asset, err := provider.Get(context.Background(), AssetProviderCredential{
				BaseURL: server.URL + "/",
				APIKey:  "secure-key",
			}, "asset-local-get")

			require.NoError(t, err)
			assert.Equal(t, "asset-local-get", asset.ID)
			assert.Equal(t, tt.wantStatus, asset.Status)
			assert.Equal(t, tt.providerStatus, asset.ProviderStatus)
		})
	}
}

func TestSecureAssetProviderRejectsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	provider := NewSecureAssetProvider(server.Client())
	_, err := provider.Get(context.Background(), AssetProviderCredential{
		BaseURL: server.URL,
		APIKey:  "secure-key",
	}, "asset-local-get")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "502")
}

func TestSecureAssetProviderRejectsMissingResultID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"result":{"Status":"Active"}}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	provider := NewSecureAssetProvider(server.Client())
	_, err := provider.Create(context.Background(), AssetProviderCredential{
		BaseURL: server.URL,
		APIKey:  "secure-key",
	}, CreateAssetRequest{
		URL:            "https://example.com/character.png",
		IdempotencyKey: "request-unique-key",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "result.id")
}

func TestSecureAssetProviderRejectsUnsupportedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"result":{"Id":"asset-local-get","Status":"Queued"}}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	provider := NewSecureAssetProvider(server.Client())
	_, err := provider.Get(context.Background(), AssetProviderCredential{
		BaseURL: server.URL,
		APIKey:  "secure-key",
	}, "asset-local-get")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Queued")
}
