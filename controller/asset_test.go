package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAssetControllerService struct {
	createInput  service.AssetCreateInput
	createUser   int
	createToken  int
	createView   *service.AssetView
	createErr    error
	getView      *service.AssetView
	getErr       error
	getToken     int
	listViews    []service.AssetView
	listTotal    int64
	listErr      error
	listToken    int
	refreshView  *service.AssetView
	refreshErr   error
	refreshToken int
}

func (fake *fakeAssetControllerService) Create(
	_ context.Context,
	userID int,
	tokenID int,
	input service.AssetCreateInput,
) (*service.AssetView, error) {
	fake.createUser = userID
	fake.createToken = tokenID
	fake.createInput = input
	return fake.createView, fake.createErr
}

func (fake *fakeAssetControllerService) Get(_ context.Context, _ int, tokenID int, _ string) (*service.AssetView, error) {
	fake.getToken = tokenID
	return fake.getView, fake.getErr
}

func (fake *fakeAssetControllerService) List(
	_ context.Context,
	_ int,
	tokenID int,
	_ service.AssetListInput,
) ([]service.AssetView, int64, error) {
	fake.listToken = tokenID
	return fake.listViews, fake.listTotal, fake.listErr
}

func (fake *fakeAssetControllerService) Refresh(_ context.Context, _ int, tokenID int, _ string) (*service.AssetView, error) {
	fake.refreshToken = tokenID
	return fake.refreshView, fake.refreshErr
}

func assetControllerContext(method string, path string, body string) (*httptest.ResponseRecorder, *gin.Context) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", 42)
	ctx.Set("token_id", 7)
	return recorder, ctx
}

func TestAssetControllerCreatePassesIdentityAndIdempotencyKey(t *testing.T) {
	fake := &fakeAssetControllerService{createView: &service.AssetView{
		ID:       "asset-project-one",
		Type:     "image",
		URL:      "https://example.com/character.png",
		Status:   "processing",
		Provider: "secure",
	}}
	controller := NewAssetController(fake)
	recorder, ctx := assetControllerContext(
		http.MethodPost,
		"/api/v3/assets",
		`{"type":"image","url":"https://example.com/character.png"}`,
	)
	ctx.Request.Header.Set("Idempotency-Key", "client-request-one")

	controller.Create(ctx)

	require.Equal(t, http.StatusCreated, recorder.Code)
	assert.Equal(t, 42, fake.createUser)
	assert.Equal(t, 7, fake.createToken)
	assert.Equal(t, "client-request-one", fake.createInput.IdempotencyKey)
	assert.Equal(t, "https://example.com/character.png", fake.createInput.URL)
	assert.NotContains(t, recorder.Body.String(), "asset-local")
	assert.NotContains(t, recorder.Body.String(), "channel_id")
	assert.NotContains(t, recorder.Body.String(), "secure-key")

	var response struct {
		Success bool              `json:"success"`
		Data    service.AssetView `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, "asset-project-one", response.Data.ID)
}

func TestAssetControllerRejectsMalformedCreateBody(t *testing.T) {
	controller := NewAssetController(&fakeAssetControllerService{})
	recorder, ctx := assetControllerContext(http.MethodPost, "/api/v3/assets", `{`)

	controller.Create(ctx)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"code":"asset_invalid_request"`)
}

func TestAssetControllerMapsServiceErrors(t *testing.T) {
	tests := []struct {
		code       string
		wantStatus int
	}{
		{service.AssetErrorInvalidURL, http.StatusBadRequest},
		{service.AssetErrorTypeUnsupported, http.StatusBadRequest},
		{service.AssetErrorNotFound, http.StatusNotFound},
		{service.AssetErrorNotActive, http.StatusConflict},
		{service.AssetErrorIdempotencyConflict, http.StatusConflict},
		{service.AssetErrorChannelUnavailable, http.StatusServiceUnavailable},
		{service.AssetErrorUpstream, http.StatusBadGateway},
		{service.AssetErrorTokenRequired, http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			fake := &fakeAssetControllerService{getErr: &service.AssetServiceError{
				Code: tt.code,
				Err:  errors.New("service failure"),
			}}
			controller := NewAssetController(fake)
			recorder, ctx := assetControllerContext(http.MethodGet, "/api/v3/assets/missing", "")
			ctx.Params = gin.Params{{Key: "asset_id", Value: "missing"}}

			controller.Get(ctx)

			assert.Equal(t, tt.wantStatus, recorder.Code)
			assert.Equal(t, 7, fake.getToken)
			assert.Contains(t, recorder.Body.String(), `"code":"`+tt.code+`"`)
		})
	}
}

func TestAssetControllerListAndRefresh(t *testing.T) {
	view := service.AssetView{ID: "asset-project-one", Type: "image", Status: "active", Provider: "secure"}
	fake := &fakeAssetControllerService{
		listViews:   []service.AssetView{view},
		listTotal:   1,
		refreshView: &view,
	}
	controller := NewAssetController(fake)

	listRecorder, listContext := assetControllerContext(http.MethodGet, "/api/v3/assets?page=1&page_size=20", "")
	controller.List(listContext)
	assert.Equal(t, http.StatusOK, listRecorder.Code)
	assert.Contains(t, listRecorder.Body.String(), `"total":1`)
	assert.Equal(t, 7, fake.listToken)

	refreshRecorder, refreshContext := assetControllerContext(http.MethodPost, "/api/v3/assets/asset-project-one/refresh", "")
	refreshContext.Params = gin.Params{{Key: "asset_id", Value: "asset-project-one"}}
	controller.Refresh(refreshContext)
	assert.Equal(t, http.StatusOK, refreshRecorder.Code)
	assert.Contains(t, refreshRecorder.Body.String(), `"id":"asset-project-one"`)
	assert.Equal(t, 7, fake.refreshToken)
}
