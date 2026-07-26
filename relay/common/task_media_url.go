package common

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

type TaskMediaURLKind string

const (
	TaskMediaURLHTTP  TaskMediaURLKind = "http"
	TaskMediaURLData  TaskMediaURLKind = "data"
	TaskMediaURLAsset TaskMediaURLKind = "asset"
)

type TaskMediaURL struct {
	Value string
	Kind  TaskMediaURLKind
}

func (u TaskMediaURL) FetchableHTTP() bool {
	return u.Kind == TaskMediaURLHTTP
}

func ParseTaskMediaURL(raw string) (TaskMediaURL, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return TaskMediaURL{}, fmt.Errorf("media URL is empty")
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil {
		return TaskMediaURL{}, fmt.Errorf("invalid media URL: %w", err)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		if parsed.Host == "" {
			return TaskMediaURL{}, fmt.Errorf("HTTP media URL requires a host")
		}
		return TaskMediaURL{Value: value, Kind: TaskMediaURLHTTP}, nil
	case "data":
		comma := strings.IndexByte(value, ',')
		if comma < 0 || !strings.Contains(strings.ToLower(value[:comma]), ";base64") || comma == len(value)-1 {
			return TaskMediaURL{}, fmt.Errorf("data media URL must contain a base64 payload")
		}
		if _, err := base64.StdEncoding.DecodeString(value[comma+1:]); err != nil {
			return TaskMediaURL{}, fmt.Errorf("data media URL contains invalid base64 payload: %w", err)
		}
		return TaskMediaURL{Value: value, Kind: TaskMediaURLData}, nil
	case "asset":
		if parsed.Host == "" {
			return TaskMediaURL{}, fmt.Errorf("asset media URL requires an identifier")
		}
		return TaskMediaURL{Value: value, Kind: TaskMediaURLAsset}, nil
	default:
		return TaskMediaURL{}, fmt.Errorf("unsupported media URL scheme")
	}
}
