package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/objectstorage"
	"github.com/QuantumNous/new-api/setting/config"
	object_storage "github.com/QuantumNous/new-api/setting/object_storage"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type objectStorageSettingsRequest struct {
	Enabled                   bool     `json:"enabled"`
	Endpoint                  string   `json:"endpoint"`
	PublicEndpoint            string   `json:"public_endpoint"`
	Region                    string   `json:"region"`
	Bucket                    string   `json:"bucket"`
	AccessKeyID               string   `json:"access_key_id"`
	SecretAccessKey           string   `json:"secret_access_key"`
	ClearSecret               bool     `json:"clear_secret"`
	UsePathStyle              bool     `json:"use_path_style"`
	MaxVideoSizeMB            int      `json:"max_video_size_mb"`
	ExpiresSeconds            int      `json:"expires_seconds"`
	TransferMode              *string  `json:"transfer_mode"`
	WhitelistEnabled          *bool    `json:"whitelist_enabled"`
	BlacklistEnabled          *bool    `json:"blacklist_enabled"`
	TransferDomainWhitelist   []string `json:"transfer_domain_whitelist"`
	NoTransferDomainBlacklist []string `json:"no_transfer_domain_blacklist"`
}

type objectStorageSettingsResponse struct {
	Enabled                   bool     `json:"enabled"`
	Endpoint                  string   `json:"endpoint"`
	PublicEndpoint            string   `json:"public_endpoint"`
	Region                    string   `json:"region"`
	Bucket                    string   `json:"bucket"`
	AccessKeyID               string   `json:"access_key_id"`
	SecretConfigured          bool     `json:"secret_configured"`
	UsePathStyle              bool     `json:"use_path_style"`
	MaxVideoSizeMB            int      `json:"max_video_size_mb"`
	ExpiresSeconds            int      `json:"expires_seconds"`
	TransferMode              string   `json:"transfer_mode"`
	WhitelistEnabled          bool     `json:"whitelist_enabled"`
	BlacklistEnabled          bool     `json:"blacklist_enabled"`
	TransferDomainWhitelist   []string `json:"transfer_domain_whitelist"`
	NoTransferDomainBlacklist []string `json:"no_transfer_domain_blacklist"`
}

func GetObjectStorageSettings(c *gin.Context) {
	common.ApiSuccess(c, objectStorageResponse(object_storage.Runtime().ObjectStorageConfig))
}

func UpdateObjectStorageSettings(c *gin.Context) {
	var request objectStorageSettingsRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		writeObjectStorageError(c, http.StatusBadRequest, "invalid object storage settings")
		return
	}
	candidate := objectStorageConfigFromRequest(request, object_storage.Runtime().ObjectStorageConfig)
	if err := object_storage.ValidateConfig(candidate); err != nil {
		writeObjectStorageError(c, http.StatusBadRequest, err.Error())
		return
	}
	candidate = object_storage.NormalizeConfig(candidate)
	flat, err := config.ConfigToMap(&candidate)
	if err != nil {
		writeObjectStorageError(c, http.StatusInternalServerError, "failed to serialize object storage settings")
		return
	}
	values := make(map[string]string, len(flat))
	for key, value := range flat {
		values[object_storage.ConfigName+"."+key] = value
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		return model.UpdateOptionsWithTx(tx, values)
	}); err != nil {
		writeObjectStorageError(c, http.StatusInternalServerError, "failed to save object storage settings")
		return
	}

	cfg := config.GlobalConfig.Get(object_storage.ConfigName)
	if err := config.UpdateConfigFromMap(cfg, flat); err != nil {
		writeObjectStorageError(c, http.StatusInternalServerError, "failed to publish object storage settings")
		return
	}
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	for key, value := range values {
		common.OptionMap[key] = value
	}
	common.OptionMapRWMutex.Unlock()
	object_storage.UpdateAndSync()
	recordManageAudit(c, "object_storage.update", map[string]interface{}{"enabled": candidate.Enabled})
	common.ApiSuccess(c, objectStorageResponse(candidate))
}

func TestObjectStorageSettings(c *gin.Context) {
	var request objectStorageSettingsRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		writeObjectStorageError(c, http.StatusBadRequest, "invalid object storage settings")
		return
	}
	candidate := objectStorageConfigFromRequest(request, object_storage.Runtime().ObjectStorageConfig)
	candidate.Enabled = true
	if err := object_storage.ValidateConfig(candidate); err != nil {
		writeObjectStorageError(c, http.StatusBadRequest, err.Error())
		return
	}
	store, err := objectstorage.New(objectstorage.Config{
		Endpoint:        candidate.Endpoint,
		PublicEndpoint:  candidate.PublicEndpoint,
		Region:          candidate.Region,
		Bucket:          candidate.Bucket,
		AccessKeyID:     candidate.AccessKeyID,
		SecretAccessKey: candidate.SecretAccessKey,
		UsePathStyle:    candidate.UsePathStyle,
	})
	if err != nil {
		writeObjectStorageError(c, http.StatusBadRequest, "invalid object storage connection settings")
		return
	}
	if err := store.Probe(c.Request.Context()); err != nil {
		writeObjectStorageError(c, http.StatusBadGateway, "object storage connection test failed")
		return
	}
	recordManageAudit(c, "object_storage.test", nil)
	common.ApiSuccess(c, gin.H{"connected": true})
}

func objectStorageConfigFromRequest(request objectStorageSettingsRequest, current object_storage.ObjectStorageConfig) object_storage.ObjectStorageConfig {
	secret := strings.TrimSpace(request.SecretAccessKey)
	if request.ClearSecret {
		secret = ""
	} else if secret == "" {
		secret = current.SecretAccessKey
	}
	transferMode := current.TransferMode
	if request.TransferMode != nil {
		transferMode = *request.TransferMode
	}
	whitelistEnabled := current.WhitelistEnabled
	if request.WhitelistEnabled != nil {
		whitelistEnabled = *request.WhitelistEnabled
	}
	blacklistEnabled := current.BlacklistEnabled
	if request.BlacklistEnabled != nil {
		blacklistEnabled = *request.BlacklistEnabled
	}
	return object_storage.ObjectStorageConfig{
		Enabled:                   request.Enabled,
		Endpoint:                  request.Endpoint,
		PublicEndpoint:            request.PublicEndpoint,
		Region:                    request.Region,
		Bucket:                    request.Bucket,
		AccessKeyID:               request.AccessKeyID,
		SecretAccessKey:           secret,
		UsePathStyle:              request.UsePathStyle,
		MaxVideoSizeMB:            request.MaxVideoSizeMB,
		ExpiresSeconds:            request.ExpiresSeconds,
		TransferMode:              transferMode,
		WhitelistEnabled:          whitelistEnabled,
		BlacklistEnabled:          blacklistEnabled,
		TransferDomainWhitelist:   request.TransferDomainWhitelist,
		NoTransferDomainBlacklist: request.NoTransferDomainBlacklist,
	}
}

func objectStorageResponse(cfg object_storage.ObjectStorageConfig) objectStorageSettingsResponse {
	cfg = object_storage.NormalizeConfig(cfg)
	return objectStorageSettingsResponse{
		Enabled:                   cfg.Enabled,
		Endpoint:                  cfg.Endpoint,
		PublicEndpoint:            cfg.PublicEndpoint,
		Region:                    cfg.Region,
		Bucket:                    cfg.Bucket,
		AccessKeyID:               cfg.AccessKeyID,
		SecretConfigured:          strings.TrimSpace(cfg.SecretAccessKey) != "",
		UsePathStyle:              cfg.UsePathStyle,
		MaxVideoSizeMB:            cfg.MaxVideoSizeMB,
		ExpiresSeconds:            cfg.ExpiresSeconds,
		TransferMode:              cfg.TransferMode,
		WhitelistEnabled:          cfg.WhitelistEnabled,
		BlacklistEnabled:          cfg.BlacklistEnabled,
		TransferDomainWhitelist:   append([]string{}, cfg.TransferDomainWhitelist...),
		NoTransferDomainBlacklist: append([]string{}, cfg.NoTransferDomainBlacklist...),
	}
}

func writeObjectStorageError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"success": false, "message": message})
}
