package model

import "gorm.io/gorm"

const (
	AssetTypeImage = "image"

	AssetStatusPending    = "pending"
	AssetStatusProcessing = "processing"
	AssetStatusActive     = "active"
	AssetStatusFailed     = "failed"
	AssetStatusUnknown    = "unknown"

	AssetProviderSecure = "secure"
)

type Asset struct {
	ID                 string  `json:"id" gorm:"primaryKey;size:64"`
	UserID             int     `json:"user_id" gorm:"not null;index;uniqueIndex:idx_assets_user_idempotency,priority:1"`
	CreatedByTokenID   int     `json:"created_by_token_id" gorm:"not null;index"`
	Type               string  `json:"type" gorm:"size:32;not null;index"`
	SourceURL          string  `json:"source_url" gorm:"type:text;not null"`
	Status             string  `json:"status" gorm:"size:32;not null;index"`
	LastError          string  `json:"last_error,omitempty" gorm:"type:text"`
	IdempotencyKeyHash *string `json:"-" gorm:"size:128;uniqueIndex:idx_assets_user_idempotency,priority:2"`
	CreatedAt          int64   `json:"created_at" gorm:"bigint;not null"`
	UpdatedAt          int64   `json:"updated_at" gorm:"bigint;not null"`
}

type AssetProviderBinding struct {
	ID                     uint    `json:"-" gorm:"primaryKey"`
	AssetID                string  `json:"-" gorm:"size:64;not null;index;uniqueIndex:idx_asset_provider_channel,priority:1"`
	Provider               string  `json:"provider" gorm:"size:32;not null;uniqueIndex:idx_asset_provider_channel,priority:2"`
	ChannelID              int     `json:"-" gorm:"not null;index;uniqueIndex:idx_asset_provider_channel,priority:3"`
	UpstreamAssetID        *string `json:"-" gorm:"size:128;uniqueIndex"`
	UpstreamStatus         string  `json:"-" gorm:"size:32"`
	UpstreamIdempotencyKey string  `json:"-" gorm:"size:128;not null"`
	CredentialFingerprint  string  `json:"-" gorm:"size:128;not null"`
	UpstreamCreatedAt      int64   `json:"-" gorm:"bigint"`
	LastCheckedAt          int64   `json:"-" gorm:"bigint"`
	LastError              string  `json:"-" gorm:"type:text"`
	CreatedAt              int64   `json:"-" gorm:"bigint;not null"`
	UpdatedAt              int64   `json:"-" gorm:"bigint;not null"`
}

func HasAssetProviderBindings(channelID int) (bool, error) {
	var count int64
	err := DB.Model(&AssetProviderBinding{}).Where("channel_id = ?", channelID).Count(&count).Error
	return count > 0, err
}

func ValidateChannelAssetDeletion(channelID int) error {
	bound, err := HasAssetProviderBindings(channelID)
	if err != nil {
		// Older installations may not have the optional asset table yet; the
		// regular channel deletion path remains backwards compatible until the
		// migration creates it.
		return nil
	}
	if bound {
		return gorm.ErrInvalidData
	}
	return nil
}
