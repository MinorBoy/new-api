package object_storage

import (
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/setting/config"
)

const (
	ConfigName          = "object_storage"
	TransferModeDefault = "default"
	TransferModeAll     = "all"
	TransferModeRules   = "rules"

	DefaultRegion         = "us-east-1"
	DefaultMaxVideoSizeMB = 512
	DefaultExpiresSeconds = 86400
	MinMaxVideoSizeMB     = 1
	MaxMaxVideoSizeMB     = 2048
	MinExpiresSeconds     = 60
	MaxExpiresSeconds     = 604800
)

type ObjectStorageConfig struct {
	Enabled                   bool     `json:"enabled"`
	Endpoint                  string   `json:"endpoint"`
	PublicEndpoint            string   `json:"public_endpoint"`
	Region                    string   `json:"region"`
	Bucket                    string   `json:"bucket"`
	AccessKeyID               string   `json:"access_key_id"`
	SecretAccessKey           string   `json:"secret_access_key"`
	UsePathStyle              bool     `json:"use_path_style"`
	MaxVideoSizeMB            int      `json:"max_video_size_mb"`
	ExpiresSeconds            int      `json:"expires_seconds"`
	TransferMode              string   `json:"transfer_mode"`
	WhitelistEnabled          bool     `json:"whitelist_enabled"`
	BlacklistEnabled          bool     `json:"blacklist_enabled"`
	TransferDomainWhitelist   []string `json:"transfer_domain_whitelist"`
	NoTransferDomainBlacklist []string `json:"no_transfer_domain_blacklist"`
}

type RuntimeSnapshot struct {
	ObjectStorageConfig
}

var objectStorageConfig = DefaultConfig()
var runtimeSnapshot atomic.Value

func init() {
	config.GlobalConfig.Register(ConfigName, &objectStorageConfig)
	UpdateAndSync()
}

func DefaultConfig() ObjectStorageConfig {
	return ObjectStorageConfig{
		Region:                    DefaultRegion,
		MaxVideoSizeMB:            DefaultMaxVideoSizeMB,
		ExpiresSeconds:            DefaultExpiresSeconds,
		UsePathStyle:              false,
		TransferDomainWhitelist:   []string{},
		NoTransferDomainBlacklist: []string{},
	}
}

func Runtime() RuntimeSnapshot {
	if loaded := runtimeSnapshot.Load(); loaded != nil {
		if snapshot, ok := loaded.(RuntimeSnapshot); ok {
			return snapshot
		}
	}
	return RuntimeSnapshot{ObjectStorageConfig: NormalizeConfig(objectStorageConfig)}
}

func UpdateAndSync() {
	runtimeSnapshot.Store(RuntimeSnapshot{ObjectStorageConfig: NormalizeConfig(objectStorageConfig)})
}

func NormalizeConfig(cfg ObjectStorageConfig) ObjectStorageConfig {
	legacyTransferMode := strings.TrimSpace(cfg.TransferMode) == ""
	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	cfg.PublicEndpoint = strings.TrimSpace(cfg.PublicEndpoint)
	cfg.Region = strings.TrimSpace(cfg.Region)
	if cfg.Region == "" {
		cfg.Region = DefaultRegion
	}
	cfg.Bucket = strings.TrimSpace(cfg.Bucket)
	cfg.AccessKeyID = strings.TrimSpace(cfg.AccessKeyID)
	cfg.SecretAccessKey = strings.TrimSpace(cfg.SecretAccessKey)
	if cfg.MaxVideoSizeMB == 0 {
		cfg.MaxVideoSizeMB = DefaultMaxVideoSizeMB
	}
	if cfg.ExpiresSeconds == 0 {
		cfg.ExpiresSeconds = DefaultExpiresSeconds
	}
	cfg.TransferDomainWhitelist = normalizeDomains(cfg.TransferDomainWhitelist)
	cfg.NoTransferDomainBlacklist = normalizeDomains(cfg.NoTransferDomainBlacklist)
	if legacyTransferMode {
		if len(cfg.TransferDomainWhitelist) > 0 || len(cfg.NoTransferDomainBlacklist) > 0 {
			cfg.TransferMode = TransferModeRules
			cfg.WhitelistEnabled = len(cfg.TransferDomainWhitelist) > 0
			cfg.BlacklistEnabled = len(cfg.NoTransferDomainBlacklist) > 0
		} else {
			cfg.TransferMode = TransferModeDefault
		}
	} else {
		switch cfg.TransferMode {
		case TransferModeDefault, TransferModeAll, TransferModeRules:
		default:
			cfg.TransferMode = TransferModeDefault
		}
	}
	return cfg
}

func ValidateConfig(cfg ObjectStorageConfig) error {
	cfg = NormalizeConfig(cfg)
	if cfg.MaxVideoSizeMB < MinMaxVideoSizeMB || cfg.MaxVideoSizeMB > MaxMaxVideoSizeMB {
		return fmt.Errorf("max_video_size_mb must be between %d and %d", MinMaxVideoSizeMB, MaxMaxVideoSizeMB)
	}
	if cfg.ExpiresSeconds < MinExpiresSeconds || cfg.ExpiresSeconds > MaxExpiresSeconds {
		return fmt.Errorf("expires_seconds must be between %d and %d", MinExpiresSeconds, MaxExpiresSeconds)
	}
	if !cfg.Enabled {
		return nil
	}
	for field, value := range map[string]string{
		"endpoint":          cfg.Endpoint,
		"public_endpoint":   cfg.PublicEndpoint,
		"bucket":            cfg.Bucket,
		"access_key_id":     cfg.AccessKeyID,
		"secret_access_key": cfg.SecretAccessKey,
	} {
		if value == "" {
			return fmt.Errorf("%s is required when object storage is enabled", field)
		}
	}
	for field, value := range map[string]string{
		"endpoint":        cfg.Endpoint,
		"public_endpoint": cfg.PublicEndpoint,
	} {
		u, err := url.Parse(value)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("%s must be an absolute HTTP(S) URL", field)
		}
	}
	return nil
}

func normalizeDomains(domains []string) []string {
	result := make([]string, 0, len(domains))
	seen := make(map[string]struct{}, len(domains))
	for _, raw := range domains {
		domain := strings.ToLower(strings.TrimSpace(raw))
		domain = strings.TrimSuffix(domain, ".")
		if domain == "" {
			continue
		}
		if strings.HasPrefix(domain, "*.") {
			domain = "*." + strings.TrimSuffix(strings.TrimPrefix(domain, "*."), ".")
		}
		if host, _, ok := strings.Cut(domain, ":"); ok && !strings.Contains(host, "]") {
			domain = host
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		result = append(result, domain)
	}
	return result
}
