package service

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type fakeAssetProvider struct {
	createCalls []CreateAssetRequest
	getCalls    []string
	create      func(CreateAssetRequest) (*ProviderAsset, error)
	get         func(string) (*ProviderAsset, error)
}

func (provider *fakeAssetProvider) Create(
	_ context.Context,
	_ AssetProviderCredential,
	request CreateAssetRequest,
) (*ProviderAsset, error) {
	provider.createCalls = append(provider.createCalls, request)
	return provider.create(request)
}

func (provider *fakeAssetProvider) Get(
	_ context.Context,
	_ AssetProviderCredential,
	upstreamAssetID string,
) (*ProviderAsset, error) {
	provider.getCalls = append(provider.getCalls, upstreamAssetID)
	return provider.get(upstreamAssetID)
}

func prepareAssetService(t *testing.T, provider AssetProviderAdapter) (*AssetService, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(
		&model.Option{},
		&model.Channel{},
		&model.Asset{},
		&model.AssetProviderBinding{},
	))

	return NewAssetService(db, provider), db
}

func createSecureAssetChannel(t *testing.T, db *gorm.DB, mutate func(*model.Channel)) model.Channel {
	t.Helper()

	baseURL := "https://token.secure-skill.com"
	channel := model.Channel{
		Type:        constant.ChannelTypeSecure,
		Key:         "secure-key",
		Status:      common.ChannelStatusEnabled,
		Name:        "Secure enterprise",
		BaseURL:     &baseURL,
		Models:      "video-2.0-pro",
		Group:       "default",
		CreatedTime: common.GetTimestamp(),
	}
	channel.SetOtherSettings(relaydto.ChannelOtherSettings{
		SecureVideoGroup: relaydto.SecureVideoGroupEnterprise,
	})
	if mutate != nil {
		mutate(&channel)
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Option{
		Key:   SecureAssetDefaultChannelOptionKey,
		Value: strconv.Itoa(channel.Id),
	}).Error)
	return channel
}

func processingAssetProvider() *fakeAssetProvider {
	return &fakeAssetProvider{
		create: func(CreateAssetRequest) (*ProviderAsset, error) {
			return &ProviderAsset{
				ID:             "asset-local-created",
				Status:         model.AssetStatusProcessing,
				ProviderStatus: "Processing",
			}, nil
		},
		get: func(id string) (*ProviderAsset, error) {
			return &ProviderAsset{
				ID:             id,
				Status:         model.AssetStatusProcessing,
				ProviderStatus: "Processing",
			}, nil
		},
	}
}

func TestAssetServiceDefaultChannelLookupQuotesOptionKeyForMySQL(t *testing.T) {
	var logs bytes.Buffer
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(127.0.0.1:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DisableAutomaticPing: true,
		DryRun:               true,
		Logger:               logger.New(log.New(&logs, "", 0), logger.Config{LogLevel: logger.Info}),
	})
	require.NoError(t, err)

	service := NewAssetService(db, processingAssetProvider())
	_, _, _, err = service.defaultChannel(context.Background())
	require.Error(t, err)
	assert.Contains(t, logs.String(), "`options`.`key`")
	assert.NotContains(t, strings.ReplaceAll(logs.String(), "`options`.`key`", ""), "WHERE key")
}

func TestAssetServiceRejectsInvalidDefaultChannels(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*model.Channel)
		setup  func(*gorm.DB)
	}{
		{name: "missing option"},
		{
			name: "non secure",
			mutate: func(channel *model.Channel) {
				channel.Type = constant.ChannelTypeOpenAI
			},
		},
		{
			name: "disabled",
			mutate: func(channel *model.Channel) {
				channel.Status = common.ChannelStatusManuallyDisabled
			},
		},
		{
			name: "non enterprise",
			mutate: func(channel *model.Channel) {
				channel.SetOtherSettings(relaydto.ChannelOtherSettings{
					SecureVideoGroup: relaydto.SecureVideoGroupDiscount,
				})
			},
		},
		{
			name: "multi key",
			mutate: func(channel *model.Channel) {
				channel.Key = "secure-key-one\nsecure-key-two"
				channel.ChannelInfo.IsMultiKey = true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := processingAssetProvider()
			service, db := prepareAssetService(t, provider)
			if tt.name != "missing option" {
				createSecureAssetChannel(t, db, tt.mutate)
			}

			_, err := service.Create(context.Background(), 42, 7, AssetCreateInput{
				Type: model.AssetTypeImage,
				URL:  "https://8.8.8.8/character.png",
			})

			require.Error(t, err)
			assert.Equal(t, AssetErrorChannelUnavailable, AssetErrorCode(err))
			assert.Empty(t, provider.createCalls)
		})
	}
}

func TestAssetServiceCreatesAssetAndBinding(t *testing.T) {
	provider := processingAssetProvider()
	service, db := prepareAssetService(t, provider)
	channel := createSecureAssetChannel(t, db, nil)

	view, err := service.Create(context.Background(), 42, 7, AssetCreateInput{
		Type:           model.AssetTypeImage,
		URL:            "https://8.8.8.8/character.png",
		IdempotencyKey: "client-request-one",
	})

	require.NoError(t, err)
	assert.Regexp(t, `^asset-[0-9a-f]{32}$`, view.ID)
	assert.Equal(t, model.AssetTypeImage, view.Type)
	assert.Equal(t, model.AssetStatusProcessing, view.Status)
	assert.Equal(t, model.AssetProviderSecure, view.Provider)
	assert.Empty(t, view.Reference)
	require.Len(t, provider.createCalls, 1)
	assert.NotEmpty(t, provider.createCalls[0].IdempotencyKey)
	assert.NotEqual(t, "client-request-one", provider.createCalls[0].IdempotencyKey)

	var stored model.Asset
	var binding model.AssetProviderBinding
	require.NoError(t, db.First(&stored, "id = ?", view.ID).Error)
	require.NoError(t, db.First(&binding, "asset_id = ?", view.ID).Error)
	assert.Equal(t, 42, stored.UserID)
	assert.Equal(t, 7, stored.CreatedByTokenID)
	assert.NotEqual(t, "client-request-one", *stored.IdempotencyKeyHash)
	assert.Equal(t, channel.Id, binding.ChannelID)
	require.NotNil(t, binding.UpstreamAssetID)
	assert.Equal(t, "asset-local-created", *binding.UpstreamAssetID)
	assert.NotEmpty(t, binding.CredentialFingerprint)
	assert.NotContains(t, binding.CredentialFingerprint, "secure-key")
}

func TestAssetServiceIdempotentRetryReturnsOriginalAsset(t *testing.T) {
	provider := processingAssetProvider()
	service, db := prepareAssetService(t, provider)
	createSecureAssetChannel(t, db, nil)
	input := AssetCreateInput{
		Type:           model.AssetTypeImage,
		URL:            "https://8.8.8.8/character.png",
		IdempotencyKey: "client-request-one",
	}

	first, err := service.Create(context.Background(), 42, 7, input)
	require.NoError(t, err)
	second, err := service.Create(context.Background(), 42, 8, input)
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID)
	assert.Len(t, provider.createCalls, 1)
}

func TestAssetServiceRejectsIdempotencyConflict(t *testing.T) {
	provider := processingAssetProvider()
	service, db := prepareAssetService(t, provider)
	createSecureAssetChannel(t, db, nil)

	_, err := service.Create(context.Background(), 42, 7, AssetCreateInput{
		Type:           model.AssetTypeImage,
		URL:            "https://8.8.8.8/character-one.png",
		IdempotencyKey: "client-request-one",
	})
	require.NoError(t, err)
	_, err = service.Create(context.Background(), 42, 7, AssetCreateInput{
		Type:           model.AssetTypeImage,
		URL:            "https://8.8.8.8/character-two.png",
		IdempotencyKey: "client-request-one",
	})

	require.Error(t, err)
	assert.Equal(t, AssetErrorIdempotencyConflict, AssetErrorCode(err))
	assert.Len(t, provider.createCalls, 1)
}

func TestAssetServiceScopesAssetsToUser(t *testing.T) {
	provider := processingAssetProvider()
	service, db := prepareAssetService(t, provider)
	createSecureAssetChannel(t, db, nil)
	created, err := service.Create(context.Background(), 42, 7, AssetCreateInput{
		Type: model.AssetTypeImage,
		URL:  "https://8.8.8.8/character.png",
	})
	require.NoError(t, err)

	_, err = service.Get(context.Background(), 43, created.ID)
	require.Error(t, err)
	assert.Equal(t, AssetErrorNotFound, AssetErrorCode(err))
}

func TestAssetServiceRefreshPreservesStatusOnTemporaryProviderFailure(t *testing.T) {
	provider := processingAssetProvider()
	service, db := prepareAssetService(t, provider)
	createSecureAssetChannel(t, db, nil)
	created, err := service.Create(context.Background(), 42, 7, AssetCreateInput{
		Type: model.AssetTypeImage,
		URL:  "https://8.8.8.8/character.png",
	})
	require.NoError(t, err)
	provider.get = func(string) (*ProviderAsset, error) {
		return nil, errors.New("temporary upstream failure")
	}

	view, err := service.Refresh(context.Background(), 42, created.ID)
	require.Error(t, err)
	assert.Equal(t, AssetErrorUpstream, AssetErrorCode(err))
	require.NotNil(t, view)
	assert.Equal(t, model.AssetStatusProcessing, view.Status)

	var stored model.Asset
	require.NoError(t, db.First(&stored, "id = ?", created.ID).Error)
	assert.Equal(t, model.AssetStatusProcessing, stored.Status)
	assert.Contains(t, stored.LastError, "temporary upstream failure")
}

func TestAssetServiceOnlyResolvesActiveReferences(t *testing.T) {
	provider := processingAssetProvider()
	service, db := prepareAssetService(t, provider)
	channel := createSecureAssetChannel(t, db, nil)
	created, err := service.Create(context.Background(), 42, 7, AssetCreateInput{
		Type: model.AssetTypeImage,
		URL:  "https://8.8.8.8/character.png",
	})
	require.NoError(t, err)

	provider.get = func(id string) (*ProviderAsset, error) {
		return &ProviderAsset{ID: id, Status: model.AssetStatusActive, ProviderStatus: "Active"}, nil
	}
	bindings, err := service.ResolveActiveReferences(context.Background(), 42, []string{created.ID})
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	assert.Equal(t, created.ID, bindings[0].AssetID)
	assert.Equal(t, "asset-local-created", bindings[0].UpstreamAssetID)
	assert.Equal(t, channel.Id, bindings[0].ChannelID)

	provider.get = func(id string) (*ProviderAsset, error) {
		return &ProviderAsset{ID: id, Status: model.AssetStatusProcessing, ProviderStatus: "Processing"}, nil
	}
	_, err = service.ResolveActiveReferences(context.Background(), 42, []string{created.ID})
	require.Error(t, err)
	assert.Equal(t, AssetErrorNotActive, AssetErrorCode(err))
}
