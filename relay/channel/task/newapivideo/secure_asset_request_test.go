package newapivideo

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSecureEnterpriseRequestTranslatesRoleAssets(t *testing.T) {
	ratio := "16:9"
	duration := 8
	request := arkRequest{
		Model:    "video-2.0-pro",
		Ratio:    &ratio,
		Duration: &duration,
		Content: []arkContent{
			{Type: "text", Text: "a character runs"},
			{Type: "image_url", Role: "reference_image", ImageURL: &arkMedia{URL: "asset://asset-one"}},
			{Type: "image_url", Role: "reference_image", ImageURL: &arkMedia{URL: "asset://asset-two"}},
		},
	}

	body, err := buildSecureEnterpriseRequestWithAssets(request, "video-2.0-pro", secureRequestProfile{group: dto.SecureVideoGroupEnterprise}, map[string]string{
		"asset-one": "asset-local-one",
		"asset-two": "asset-local-two",
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"video-2.0-pro","prompt":"a character runs","duration":8,"aspect_ratio":"16:9","use_person_character":true,"extra_images":["asset://asset-local-one","asset://asset-local-two"]}`, string(body))
}

func TestSecureEnterpriseValidationPreservesRoleAssetsForRequestBuild(t *testing.T) {
	ratio := "9:16"
	duration := 5
	request := arkRequest{
		Model:    "doubao-seedance-2-0-260128",
		Ratio:    &ratio,
		Duration: &duration,
		Content: []arkContent{
			{Type: "text", Text: "a character dances"},
			{Type: "image_url", Role: "reference_image", ImageURL: &arkMedia{URL: "asset://asset-project"}},
		},
	}
	profile := secureRequestProfile{group: dto.SecureVideoGroupEnterprise}
	mappings := map[string]string{"asset-project": "asset-local-upstream"}

	require.NoError(t, validateSecureRequestWithAssets(request, profile, "video-2.0-pro", mappings))
	body, err := buildSecureEnterpriseRequestWithAssets(request, "video-2.0-pro", profile, mappings)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"video-2.0-pro","prompt":"a character dances","duration":5,"aspect_ratio":"9:16","use_person_character":true,"extra_images":["asset://asset-local-upstream"]}`, string(body))
}

func TestBuildSecureEnterpriseRequestRejectsMixedRoleAndPublicImages(t *testing.T) {
	duration := 8
	request := arkRequest{
		Model:    "video-2.0-pro",
		Duration: &duration,
		Content: []arkContent{
			{Type: "text", Text: "a character runs"},
			{Type: "image_url", Role: "reference_image", ImageURL: &arkMedia{URL: "asset://asset-one"}},
			{Type: "image_url", Role: "reference_image", ImageURL: &arkMedia{URL: "https://example.com/other.png"}},
		},
	}

	_, err := buildSecureEnterpriseRequestWithAssets(request, "video-2.0-pro", secureRequestProfile{group: dto.SecureVideoGroupEnterprise}, map[string]string{"asset-one": "asset-local-one"})
	assert.Error(t, err)
}
