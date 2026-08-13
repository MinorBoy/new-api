package e2e

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSecureRoleAssetLifecycleE2E(t *testing.T) {
	var createCalls, getCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "Bearer upstream-key", request.Header.Get("Authorization"))
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/v1/asset/create" {
			createCalls++
			_, _ = writer.Write([]byte(`{"result":{"Id":"asset-local-e2e","Status":"Processing"}}`))
			return
		}
		getCalls++
		_, _ = writer.Write([]byte(`{"result":{"Id":"asset-local-e2e","Status":"Active"}}`))
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Channel{}, &model.Asset{}, &model.AssetProviderBinding{}))
	baseURL := upstream.URL
	channel := model.Channel{Type: constant.ChannelTypeSecure, Key: "upstream-key", Status: common.ChannelStatusEnabled, Name: "secure-e2e", BaseURL: &baseURL, Group: "default", Models: "video-2.0-pro"}
	channel.SetOtherSettings(dto.ChannelOtherSettings{SecureVideoGroup: dto.SecureVideoGroupEnterprise})
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Option{Key: service.SecureAssetDefaultChannelOptionKey, Value: strconv.Itoa(channel.Id)}).Error)

	assetService := service.NewAssetService(db, service.NewSecureAssetProvider(&http.Client{}))
	created, err := assetService.Create(context.Background(), 1001, 7, service.AssetCreateInput{Type: model.AssetTypeImage, URL: "https://8.8.8.8/character.png", IdempotencyKey: "e2e-create"})
	require.NoError(t, err)
	assert.Regexp(t, `^asset-[0-9a-f]{32}$`, created.ID)
	assert.Equal(t, model.AssetStatusProcessing, created.Status)
	assert.Equal(t, 1, createCalls)

	refs, err := assetService.ResolveActiveReferences(context.Background(), 1001, []string{created.ID})
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, "asset-local-e2e", refs[0].UpstreamAssetID)
	assert.Equal(t, "asset://"+created.ID, (func() string {
		view, _ := assetService.Get(context.Background(), 1001, created.ID)
		return view.Reference
	})())
	assert.Positive(t, getCalls)

	_, err = assetService.Get(context.Background(), 1002, created.ID)
	assert.Equal(t, service.AssetErrorNotFound, service.AssetErrorCode(err))
}
