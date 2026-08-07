package objectstorage

import (
	"fmt"
	"net/url"
	"strings"
)

func ShouldTransfer(rawURL string, whitelist, blacklist []string) (bool, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return false, fmt.Errorf("video result URL must be an absolute HTTP(S) URL")
	}
	host := normalizeHost(u.Hostname())
	for _, pattern := range blacklist {
		if domainMatches(host, pattern) {
			return false, nil
		}
	}
	for _, pattern := range whitelist {
		if domainMatches(host, pattern) {
			return true, nil
		}
	}
	return false, nil
}

func normalizeHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

func domainMatches(host, pattern string) bool {
	pattern = normalizeHost(pattern)
	if pattern == "" {
		return false
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*.")
		return host != suffix && strings.HasSuffix(host, "."+suffix)
	}
	return host == pattern
}
