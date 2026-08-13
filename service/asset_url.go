package service

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

func ValidateRoleAssetURL(rawURL string) error {
	parsedURL, err := url.ParseRequestURI(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("invalid role asset URL: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("role asset URL must use http or https")
	}
	if parsedURL.User != nil {
		return fmt.Errorf("role asset URL must not contain credentials")
	}
	host := strings.ToLower(strings.TrimSuffix(parsedURL.Hostname(), "."))
	if host == "" {
		return fmt.Errorf("role asset URL must include a host")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("role asset URL must use a public host")
	}

	protection := common.SSRFProtection{
		AllowPrivateIp:         false,
		DomainFilterMode:       false,
		DomainList:             []string{"localhost", "*.localhost", "local", "*.local"},
		IpFilterMode:           false,
		ApplyIPFilterForDomain: true,
	}
	if err := protection.ValidateURL(parsedURL.String()); err != nil {
		return fmt.Errorf("role asset URL must be publicly reachable: %w", err)
	}
	return nil
}
