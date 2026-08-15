package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeVideoAssetService struct {
	refs      []service.AssetReferenceBinding
	err       error
	seen      []string
	seenToken int
}

func (fake *fakeVideoAssetService) ResolveActiveReferences(_ context.Context, _ int, tokenID int, assetIDs []string) ([]service.AssetReferenceBinding, error) {
	fake.seenToken = tokenID
	fake.seen = append(fake.seen, assetIDs...)
	return fake.refs, fake.err
}

func videoAssetContext(body string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", 42)
	ctx.Set("token_id", 7)
	return ctx, recorder
}

func TestVideoAssetRoutingAcceptsSupportedPublicAssetIDFormats(t *testing.T) {
	tests := []string{
		"asset-00000000000000000000000000000001",
		"asset-20260401123823-6d4x2",
	}

	for _, assetID := range tests {
		t.Run(assetID, func(t *testing.T) {
			fake := &fakeVideoAssetService{refs: []service.AssetReferenceBinding{{
				AssetID: assetID, UpstreamAssetID: "asset-local-one", ChannelID: 77,
			}}}
			ctx, recorder := videoAssetContext(fmt.Sprintf(
				`{"model":"video-2.0-pro","content":[{"type":"image_url","role":"reference_image","image_url":{"url":"asset://%s"}}]}`,
				assetID,
			))

			NewVideoAssetRouting(fake)(ctx)

			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, []string{assetID}, fake.seen)
			assert.Equal(t, 7, fake.seenToken)
		})
	}
}

func TestVideoAssetRoutingLocksSingleChannelAndStoresMappings(t *testing.T) {
	fake := &fakeVideoAssetService{refs: []service.AssetReferenceBinding{
		{AssetID: "asset-00000000000000000000000000000001", UpstreamAssetID: "asset-local-one", ChannelID: 77},
		{AssetID: "asset-00000000000000000000000000000002", UpstreamAssetID: "asset-local-two", ChannelID: 77},
	}}
	middleware := NewVideoAssetRouting(fake)
	ctx, recorder := videoAssetContext(`{"model":"video-2.0-pro","content":[{"type":"text","text":"a"},{"type":"image_url","role":"reference_image","image_url":{"url":"asset://asset-00000000000000000000000000000001"}},{"type":"image_url","role":"reference_image","image_url":{"url":"asset://asset-00000000000000000000000000000002"}}]}`)

	middleware(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, []string{"asset-00000000000000000000000000000001", "asset-00000000000000000000000000000002"}, fake.seen)
	assert.Equal(t, "77", common.GetContextKeyString(ctx, constant.ContextKeyTokenSpecificChannelId))
	mappings, ok := GetVideoAssetMappings(ctx)
	require.True(t, ok)
	assert.Equal(t, "asset-local-one", mappings["asset-00000000000000000000000000000001"])
	assert.Equal(t, "asset-local-two", mappings["asset-00000000000000000000000000000002"])
}

func TestVideoAssetRoutingLeavesPublicImagesUnchanged(t *testing.T) {
	fake := &fakeVideoAssetService{}
	middleware := NewVideoAssetRouting(fake)
	ctx, recorder := videoAssetContext(`{"model":"video-2.0-pro","content":[{"type":"image_url","role":"reference_image","image_url":{"url":"https://example.com/character.png"}}]}`)

	middleware(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Empty(t, fake.seen)
	_, ok := GetVideoAssetMappings(ctx)
	assert.False(t, ok)
}

func TestVideoAssetRoutingRejectsMixedReferencesAndMoreThanNineAssets(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{
			name: "mixed",
			body: `{"model":"video-2.0-pro","content":[{"type":"image_url","role":"reference_image","image_url":{"url":"asset://asset-00000000000000000000000000000001"}},{"type":"image_url","role":"reference_image","image_url":{"url":"https://example.com/ordinary.png"}}]}`,
			code: service.AssetErrorReferenceMixed,
		},
		{
			name: "limit",
			body: `{"model":"video-2.0-pro","content":[{"type":"image_url","role":"reference_image","image_url":{"url":"asset://asset-00000000000000000000000000000001"}},{"type":"image_url","role":"reference_image","image_url":{"url":"asset://asset-00000000000000000000000000000002"}},{"type":"image_url","role":"reference_image","image_url":{"url":"asset://asset-00000000000000000000000000000003"}},{"type":"image_url","role":"reference_image","image_url":{"url":"asset://asset-00000000000000000000000000000004"}},{"type":"image_url","role":"reference_image","image_url":{"url":"asset://asset-00000000000000000000000000000005"}},{"type":"image_url","role":"reference_image","image_url":{"url":"asset://asset-00000000000000000000000000000006"}},{"type":"image_url","role":"reference_image","image_url":{"url":"asset://asset-00000000000000000000000000000007"}},{"type":"image_url","role":"reference_image","image_url":{"url":"asset://asset-00000000000000000000000000000008"}},{"type":"image_url","role":"reference_image","image_url":{"url":"asset://asset-00000000000000000000000000000009"}},{"type":"image_url","role":"reference_image","image_url":{"url":"asset://asset-0000000000000000000000000000000a"}}]}`,
			code: service.AssetErrorLimitExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeVideoAssetService{}
			middleware := NewVideoAssetRouting(fake)
			ctx, recorder := videoAssetContext(tt.body)

			middleware(ctx)

			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.Contains(t, recorder.Body.String(), tt.code)
		})
	}
}

func TestVideoAssetRoutingPropagatesServiceErrors(t *testing.T) {
	fake := &fakeVideoAssetService{err: &service.AssetServiceError{Code: service.AssetErrorNotFound, Err: assert.AnError}}
	middleware := NewVideoAssetRouting(fake)
	ctx, recorder := videoAssetContext(`{"model":"video-2.0-pro","content":[{"type":"image_url","role":"reference_image","image_url":{"url":"asset://asset-00000000000000000000000000000001"}}]}`)

	middleware(ctx)

	assert.Equal(t, http.StatusNotFound, recorder.Code)
	assert.Contains(t, recorder.Body.String(), service.AssetErrorNotFound)
}
