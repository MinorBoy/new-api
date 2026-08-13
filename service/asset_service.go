package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"gorm.io/gorm"
)

const SecureAssetDefaultChannelOptionKey = "secure_asset.default_channel_id"

const (
	AssetErrorInvalidURL          = "asset_invalid_url"
	AssetErrorTypeUnsupported     = "asset_type_unsupported"
	AssetErrorNotFound            = "asset_not_found"
	AssetErrorNotActive           = "asset_not_active"
	AssetErrorChannelUnavailable  = "asset_channel_unavailable"
	AssetErrorChannelMismatch     = "asset_channel_mismatch"
	AssetErrorIdempotencyConflict = "asset_idempotency_conflict"
	AssetErrorUpstream            = "asset_upstream_error"
)

type AssetServiceError struct {
	Code string
	Err  error
}

func (err *AssetServiceError) Error() string {
	if err == nil || err.Err == nil {
		return "asset service error"
	}
	return err.Err.Error()
}

func (err *AssetServiceError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

func AssetErrorCode(err error) string {
	var serviceErr *AssetServiceError
	if errors.As(err, &serviceErr) {
		return serviceErr.Code
	}
	return ""
}

type AssetCreateInput struct {
	Type           string
	URL            string
	IdempotencyKey string
}

type AssetListInput struct {
	Type     string
	Page     int
	PageSize int
}

type AssetView struct {
	ID             string `json:"id"`
	Type           string `json:"type"`
	URL            string `json:"url"`
	Status         string `json:"status"`
	Provider       string `json:"provider"`
	ProviderStatus string `json:"provider_status,omitempty"`
	Reference      string `json:"reference,omitempty"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}

type AssetReferenceBinding struct {
	AssetID         string
	UpstreamAssetID string
	ChannelID       int
}

type AssetService struct {
	db       *gorm.DB
	provider AssetProviderAdapter
}

func NewAssetService(db *gorm.DB, provider AssetProviderAdapter) *AssetService {
	if db == nil {
		db = model.DB
	}
	if provider == nil {
		provider = NewSecureAssetProvider(GetHttpClient())
	}
	return &AssetService{db: db, provider: provider}
}

func (service *AssetService) Create(
	ctx context.Context,
	userID int,
	tokenID int,
	input AssetCreateInput,
) (*AssetView, error) {
	assetType := strings.TrimSpace(input.Type)
	if assetType != model.AssetTypeImage {
		return nil, &AssetServiceError{
			Code: AssetErrorTypeUnsupported,
			Err:  fmt.Errorf("role asset type %q is unsupported", assetType),
		}
	}
	sourceURL := strings.TrimSpace(input.URL)
	if err := ValidateRoleAssetURL(sourceURL); err != nil {
		return nil, &AssetServiceError{Code: AssetErrorInvalidURL, Err: err}
	}

	var idempotencyHash *string
	if idempotencyKey := strings.TrimSpace(input.IdempotencyKey); idempotencyKey != "" {
		hash := common.GenerateHMAC("role-asset-idempotency:" + idempotencyKey)
		idempotencyHash = &hash
		var existing model.Asset
		err := service.db.WithContext(ctx).
			Where("user_id = ? AND idempotency_key_hash = ?", userID, hash).
			First(&existing).Error
		if err == nil {
			return service.resumeCreation(ctx, &existing, assetType, sourceURL)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("find idempotent role asset: %w", err)
		}
	}

	channel, credential, fingerprint, err := service.defaultChannel(ctx)
	if err != nil {
		return nil, err
	}
	now := common.GetTimestamp()
	asset := model.Asset{
		ID:                 "asset-" + common.GetUUID(),
		UserID:             userID,
		CreatedByTokenID:   tokenID,
		Type:               assetType,
		SourceURL:          sourceURL,
		Status:             model.AssetStatusProcessing,
		IdempotencyKeyHash: idempotencyHash,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	binding := model.AssetProviderBinding{
		AssetID:                asset.ID,
		Provider:               model.AssetProviderSecure,
		ChannelID:              channel.Id,
		UpstreamStatus:         "Pending",
		UpstreamIdempotencyKey: "role-asset-" + common.GetUUID(),
		CredentialFingerprint:  fingerprint,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	if err := service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&asset).Error; err != nil {
			return err
		}
		return tx.Create(&binding).Error
	}); err != nil {
		if idempotencyHash != nil {
			var existing model.Asset
			findErr := service.db.WithContext(ctx).
				Where("user_id = ? AND idempotency_key_hash = ?", userID, *idempotencyHash).
				First(&existing).Error
			if findErr == nil {
				return service.resumeCreation(ctx, &existing, assetType, sourceURL)
			}
		}
		return nil, fmt.Errorf("persist role asset placeholder: %w", err)
	}

	return service.completeCreation(ctx, &asset, &binding, credential)
}

func (service *AssetService) Get(ctx context.Context, userID int, assetID string) (*AssetView, error) {
	return service.Refresh(ctx, userID, assetID)
}

func (service *AssetService) List(
	ctx context.Context,
	userID int,
	input AssetListInput,
) ([]AssetView, int64, error) {
	query := service.db.WithContext(ctx).Model(&model.Asset{}).Where("user_id = ?", userID)
	if assetType := strings.TrimSpace(input.Type); assetType != "" {
		query = query.Where("type = ?", assetType)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count role assets: %w", err)
	}
	page := input.Page
	if page < 1 {
		page = 1
	}
	pageSize := input.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	var assets []model.Asset
	if err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&assets).Error; err != nil {
		return nil, 0, fmt.Errorf("list role assets: %w", err)
	}
	views := make([]AssetView, 0, len(assets))
	for i := range assets {
		view, err := service.view(ctx, &assets[i])
		if err != nil {
			return nil, 0, err
		}
		views = append(views, *view)
	}
	return views, total, nil
}

func (service *AssetService) Refresh(ctx context.Context, userID int, assetID string) (*AssetView, error) {
	asset, binding, err := service.ownedAssetWithBinding(ctx, userID, assetID)
	if err != nil {
		return nil, err
	}
	credential, err := service.bindingCredential(ctx, binding)
	if err != nil {
		return service.viewWithBinding(asset, binding), err
	}
	if binding.UpstreamAssetID == nil || strings.TrimSpace(*binding.UpstreamAssetID) == "" {
		return service.completeCreation(ctx, asset, binding, credential)
	}

	providerAsset, providerErr := service.provider.Get(ctx, credential, *binding.UpstreamAssetID)
	if providerErr != nil {
		service.recordRefreshError(ctx, asset, binding, providerErr)
		return service.viewWithBinding(asset, binding), &AssetServiceError{
			Code: AssetErrorUpstream,
			Err:  fmt.Errorf("refresh Secure role asset: %w", providerErr),
		}
	}
	if providerAsset.ID != *binding.UpstreamAssetID {
		mismatchErr := fmt.Errorf("Secure returned unexpected asset id %q", providerAsset.ID)
		service.recordRefreshError(ctx, asset, binding, mismatchErr)
		return service.viewWithBinding(asset, binding), &AssetServiceError{
			Code: AssetErrorUpstream,
			Err:  mismatchErr,
		}
	}
	if err := service.persistProviderAsset(ctx, asset, binding, providerAsset); err != nil {
		return nil, err
	}
	return service.viewWithBinding(asset, binding), nil
}

func (service *AssetService) ResolveActiveReferences(
	ctx context.Context,
	userID int,
	assetIDs []string,
) ([]AssetReferenceBinding, error) {
	resolved := make([]AssetReferenceBinding, 0, len(assetIDs))
	channelID := 0
	for _, assetID := range assetIDs {
		view, err := service.Refresh(ctx, userID, assetID)
		if err != nil {
			return nil, err
		}
		if view.Status != model.AssetStatusActive {
			return nil, &AssetServiceError{
				Code: AssetErrorNotActive,
				Err:  fmt.Errorf("role asset %s is not active", assetID),
			}
		}
		_, binding, err := service.ownedAssetWithBinding(ctx, userID, assetID)
		if err != nil {
			return nil, err
		}
		if binding.UpstreamAssetID == nil || *binding.UpstreamAssetID == "" {
			return nil, &AssetServiceError{
				Code: AssetErrorNotActive,
				Err:  fmt.Errorf("role asset %s has no upstream binding", assetID),
			}
		}
		if channelID == 0 {
			channelID = binding.ChannelID
		} else if binding.ChannelID != channelID {
			return nil, &AssetServiceError{
				Code: AssetErrorChannelMismatch,
				Err:  fmt.Errorf("role assets are bound to different channels"),
			}
		}
		resolved = append(resolved, AssetReferenceBinding{
			AssetID:         assetID,
			UpstreamAssetID: *binding.UpstreamAssetID,
			ChannelID:       binding.ChannelID,
		})
	}
	return resolved, nil
}

func (service *AssetService) resumeCreation(
	ctx context.Context,
	asset *model.Asset,
	assetType string,
	sourceURL string,
) (*AssetView, error) {
	if asset.Type != assetType || asset.SourceURL != sourceURL {
		return nil, &AssetServiceError{
			Code: AssetErrorIdempotencyConflict,
			Err:  fmt.Errorf("idempotency key was already used with different asset input"),
		}
	}
	var binding model.AssetProviderBinding
	if err := service.db.WithContext(ctx).Where("asset_id = ?", asset.ID).First(&binding).Error; err != nil {
		return nil, fmt.Errorf("load idempotent role asset binding: %w", err)
	}
	if binding.UpstreamAssetID != nil && strings.TrimSpace(*binding.UpstreamAssetID) != "" {
		return service.viewWithBinding(asset, &binding), nil
	}
	credential, err := service.bindingCredential(ctx, &binding)
	if err != nil {
		return service.viewWithBinding(asset, &binding), err
	}
	return service.completeCreation(ctx, asset, &binding, credential)
}

func (service *AssetService) completeCreation(
	ctx context.Context,
	asset *model.Asset,
	binding *model.AssetProviderBinding,
	credential AssetProviderCredential,
) (*AssetView, error) {
	providerAsset, err := service.provider.Create(ctx, credential, CreateAssetRequest{
		URL:            asset.SourceURL,
		IdempotencyKey: binding.UpstreamIdempotencyKey,
	})
	if err != nil {
		service.recordRefreshError(ctx, asset, binding, err)
		return service.viewWithBinding(asset, binding), &AssetServiceError{
			Code: AssetErrorUpstream,
			Err:  fmt.Errorf("create Secure role asset: %w", err),
		}
	}
	if strings.TrimSpace(providerAsset.ID) == "" {
		invalidErr := fmt.Errorf("Secure role asset response is missing an asset id")
		service.recordRefreshError(ctx, asset, binding, invalidErr)
		return service.viewWithBinding(asset, binding), &AssetServiceError{
			Code: AssetErrorUpstream,
			Err:  invalidErr,
		}
	}
	if err := service.persistProviderAsset(ctx, asset, binding, providerAsset); err != nil {
		return nil, err
	}
	return service.viewWithBinding(asset, binding), nil
}

func (service *AssetService) persistProviderAsset(
	ctx context.Context,
	asset *model.Asset,
	binding *model.AssetProviderBinding,
	providerAsset *ProviderAsset,
) error {
	now := common.GetTimestamp()
	asset.Status = providerAsset.Status
	asset.LastError = ""
	asset.UpdatedAt = now
	binding.UpstreamAssetID = &providerAsset.ID
	binding.UpstreamStatus = providerAsset.ProviderStatus
	binding.LastCheckedAt = now
	binding.LastError = ""
	binding.UpdatedAt = now
	if binding.UpstreamCreatedAt == 0 {
		binding.UpstreamCreatedAt = now
	}
	if err := service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(asset).Error; err != nil {
			return err
		}
		return tx.Save(binding).Error
	}); err != nil {
		return fmt.Errorf("persist role asset provider state: %w", err)
	}
	return nil
}

func (service *AssetService) recordRefreshError(
	ctx context.Context,
	asset *model.Asset,
	binding *model.AssetProviderBinding,
	refreshErr error,
) {
	now := common.GetTimestamp()
	asset.LastError = refreshErr.Error()
	asset.UpdatedAt = now
	binding.LastError = refreshErr.Error()
	binding.LastCheckedAt = now
	binding.UpdatedAt = now
	_ = service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(asset).Error; err != nil {
			return err
		}
		return tx.Save(binding).Error
	})
}

func (service *AssetService) ownedAssetWithBinding(
	ctx context.Context,
	userID int,
	assetID string,
) (*model.Asset, *model.AssetProviderBinding, error) {
	var asset model.Asset
	if err := service.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", strings.TrimSpace(assetID), userID).
		First(&asset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, &AssetServiceError{
				Code: AssetErrorNotFound,
				Err:  fmt.Errorf("role asset not found"),
			}
		}
		return nil, nil, fmt.Errorf("load role asset: %w", err)
	}
	var binding model.AssetProviderBinding
	if err := service.db.WithContext(ctx).
		Where("asset_id = ? AND provider = ?", asset.ID, model.AssetProviderSecure).
		First(&binding).Error; err != nil {
		return nil, nil, fmt.Errorf("load role asset provider binding: %w", err)
	}
	return &asset, &binding, nil
}

func (service *AssetService) view(ctx context.Context, asset *model.Asset) (*AssetView, error) {
	var binding model.AssetProviderBinding
	if err := service.db.WithContext(ctx).
		Where("asset_id = ? AND provider = ?", asset.ID, model.AssetProviderSecure).
		First(&binding).Error; err != nil {
		return nil, fmt.Errorf("load role asset view binding: %w", err)
	}
	return service.viewWithBinding(asset, &binding), nil
}

func (service *AssetService) viewWithBinding(
	asset *model.Asset,
	binding *model.AssetProviderBinding,
) *AssetView {
	reference := ""
	if asset.Status == model.AssetStatusActive {
		reference = "asset://" + asset.ID
	}
	return &AssetView{
		ID:             asset.ID,
		Type:           asset.Type,
		URL:            asset.SourceURL,
		Status:         asset.Status,
		Provider:       binding.Provider,
		ProviderStatus: binding.UpstreamStatus,
		Reference:      reference,
		CreatedAt:      asset.CreatedAt,
		UpdatedAt:      asset.UpdatedAt,
	}
}

func (service *AssetService) defaultChannel(
	ctx context.Context,
) (*model.Channel, AssetProviderCredential, string, error) {
	var option model.Option
	if err := service.db.WithContext(ctx).
		Where("key = ?", SecureAssetDefaultChannelOptionKey).
		First(&option).Error; err != nil {
		return nil, AssetProviderCredential{}, "", &AssetServiceError{
			Code: AssetErrorChannelUnavailable,
			Err:  fmt.Errorf("default Secure role asset channel is not configured"),
		}
	}
	channelID, err := strconv.Atoi(strings.TrimSpace(option.Value))
	if err != nil || channelID <= 0 {
		return nil, AssetProviderCredential{}, "", &AssetServiceError{
			Code: AssetErrorChannelUnavailable,
			Err:  fmt.Errorf("default Secure role asset channel is invalid"),
		}
	}
	channel, credential, fingerprint, err := service.channelCredential(ctx, channelID)
	if err != nil {
		return nil, AssetProviderCredential{}, "", err
	}
	return channel, credential, fingerprint, nil
}

func (service *AssetService) bindingCredential(
	ctx context.Context,
	binding *model.AssetProviderBinding,
) (AssetProviderCredential, error) {
	if binding.Provider != model.AssetProviderSecure {
		return AssetProviderCredential{}, &AssetServiceError{
			Code: AssetErrorChannelUnavailable,
			Err:  fmt.Errorf("role asset provider %q is unsupported", binding.Provider),
		}
	}
	_, credential, fingerprint, err := service.channelCredential(ctx, binding.ChannelID)
	if err != nil {
		return AssetProviderCredential{}, err
	}
	if fingerprint != binding.CredentialFingerprint {
		return AssetProviderCredential{}, &AssetServiceError{
			Code: AssetErrorChannelUnavailable,
			Err:  fmt.Errorf("role asset channel credentials have changed"),
		}
	}
	return credential, nil
}

func (service *AssetService) channelCredential(
	ctx context.Context,
	channelID int,
) (*model.Channel, AssetProviderCredential, string, error) {
	var channel model.Channel
	if err := service.db.WithContext(ctx).First(&channel, "id = ?", channelID).Error; err != nil {
		return nil, AssetProviderCredential{}, "", &AssetServiceError{
			Code: AssetErrorChannelUnavailable,
			Err:  fmt.Errorf("Secure role asset channel is unavailable"),
		}
	}
	if channel.Type != constant.ChannelTypeSecure || channel.Status != common.ChannelStatusEnabled || channel.ChannelInfo.IsMultiKey {
		return nil, AssetProviderCredential{}, "", &AssetServiceError{
			Code: AssetErrorChannelUnavailable,
			Err:  fmt.Errorf("Secure role asset channel is unavailable"),
		}
	}
	var otherSettings relaydto.ChannelOtherSettings
	if err := common.UnmarshalJsonStr(channel.OtherSettings, &otherSettings); err != nil ||
		otherSettings.SecureVideoGroup != relaydto.SecureVideoGroupEnterprise {
		return nil, AssetProviderCredential{}, "", &AssetServiceError{
			Code: AssetErrorChannelUnavailable,
			Err:  fmt.Errorf("Secure role asset channel must use the enterprise video group"),
		}
	}
	keys := channel.GetKeys()
	if len(keys) != 1 || strings.TrimSpace(keys[0]) == "" {
		return nil, AssetProviderCredential{}, "", &AssetServiceError{
			Code: AssetErrorChannelUnavailable,
			Err:  fmt.Errorf("Secure role asset channel must use exactly one API key"),
		}
	}
	credential := AssetProviderCredential{
		BaseURL: channel.GetBaseURL(),
		APIKey:  strings.TrimSpace(keys[0]),
	}
	if strings.TrimSpace(credential.BaseURL) == "" {
		return nil, AssetProviderCredential{}, "", &AssetServiceError{
			Code: AssetErrorChannelUnavailable,
			Err:  fmt.Errorf("Secure role asset channel base URL is unavailable"),
		}
	}
	fingerprint := common.GenerateHMAC("secure-role-asset-credential:" + credential.APIKey)
	return &channel, credential, fingerprint, nil
}
