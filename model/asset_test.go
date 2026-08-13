package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func prepareAssetDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		require.NoError(t, sqlDB.Close())
	})

	require.NoError(t, db.AutoMigrate(&Asset{}, &AssetProviderBinding{}))
}

func TestAssetPersistsOwnershipAndProviderBinding(t *testing.T) {
	prepareAssetDB(t)

	idempotencyHash := "sha256:request-one"
	upstreamAssetID := "asset-local-upstream-one"
	asset := Asset{
		ID:                 "asset-project-one",
		UserID:             42,
		CreatedByTokenID:   7,
		Type:               "image",
		SourceURL:          "https://example.com/character.png",
		Status:             "processing",
		IdempotencyKeyHash: &idempotencyHash,
		CreatedAt:          1_723_000_000,
		UpdatedAt:          1_723_000_001,
	}
	binding := AssetProviderBinding{
		AssetID:                asset.ID,
		Provider:               "secure",
		ChannelID:              99,
		UpstreamAssetID:        &upstreamAssetID,
		UpstreamStatus:         "Processing",
		UpstreamIdempotencyKey: "secure-idempotency-one",
		CredentialFingerprint:  "fingerprint-one",
		LastCheckedAt:          1_723_000_002,
	}

	require.NoError(t, DB.Create(&asset).Error)
	require.NoError(t, DB.Create(&binding).Error)

	var loadedAsset Asset
	var loadedBinding AssetProviderBinding
	require.NoError(t, DB.First(&loadedAsset, "id = ?", asset.ID).Error)
	require.NoError(t, DB.First(&loadedBinding, "asset_id = ?", asset.ID).Error)
	assert.Equal(t, asset.UserID, loadedAsset.UserID)
	assert.Equal(t, asset.CreatedByTokenID, loadedAsset.CreatedByTokenID)
	assert.Equal(t, asset.SourceURL, loadedAsset.SourceURL)
	assert.Equal(t, asset.Status, loadedAsset.Status)
	require.NotNil(t, loadedAsset.IdempotencyKeyHash)
	assert.Equal(t, idempotencyHash, *loadedAsset.IdempotencyKeyHash)
	assert.Equal(t, binding.ChannelID, loadedBinding.ChannelID)
	require.NotNil(t, loadedBinding.UpstreamAssetID)
	assert.Equal(t, upstreamAssetID, *loadedBinding.UpstreamAssetID)
	assert.Equal(t, binding.UpstreamIdempotencyKey, loadedBinding.UpstreamIdempotencyKey)
	assert.Equal(t, binding.CredentialFingerprint, loadedBinding.CredentialFingerprint)
}

func TestAssetIdempotencyHashIsUniquePerUser(t *testing.T) {
	prepareAssetDB(t)

	idempotencyHash := "sha256:same-request"
	first := Asset{
		ID:                 "asset-first",
		UserID:             42,
		Type:               "image",
		SourceURL:          "https://example.com/first.png",
		Status:             "processing",
		IdempotencyKeyHash: &idempotencyHash,
	}
	require.NoError(t, DB.Create(&first).Error)

	duplicateForUser := first
	duplicateForUser.ID = "asset-duplicate"
	require.Error(t, DB.Create(&duplicateForUser).Error)

	sameHashForAnotherUser := first
	sameHashForAnotherUser.ID = "asset-another-user"
	sameHashForAnotherUser.UserID = 43
	require.NoError(t, DB.Create(&sameHashForAnotherUser).Error)
}

func TestAssetWithoutIdempotencyKeyCanBeCreatedRepeatedly(t *testing.T) {
	prepareAssetDB(t)

	first := Asset{
		ID:        "asset-without-key-one",
		UserID:    42,
		Type:      "image",
		SourceURL: "https://example.com/first.png",
		Status:    "processing",
	}
	second := first
	second.ID = "asset-without-key-two"

	require.NoError(t, DB.Create(&first).Error)
	require.NoError(t, DB.Create(&second).Error)
}
