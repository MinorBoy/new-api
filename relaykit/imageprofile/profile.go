// Package imageprofile contains the versioned contracts for OpenAI-compatible
// image endpoints. It deliberately depends only on the relaykit module.
package imageprofile

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
)

const MaxImageN = 128

type Endpoint string

const (
	EndpointGenerations Endpoint = "generations"
	EndpointEdits       Endpoint = "edits"

	OpenAIImagesProfile = "openai_images"
	OpenAIImagesVersion = 1
)

type Capability struct {
	Enabled         bool     `json:"enabled"`
	Sizes           []string `json:"sizes,omitempty"`
	Qualities       []string `json:"qualities,omitempty"`
	ResponseFormats []string `json:"response_formats,omitempty"`
	MaxN            uint     `json:"max_n"`
	MaxInputImages  uint     `json:"max_input_images"`
	SupportsMask    bool     `json:"supports_mask"`
}

type Profile struct {
	Name         string                  `json:"name"`
	Version      int                     `json:"version"`
	Paths        map[Endpoint]string     `json:"paths"`
	Capabilities map[Endpoint]Capability `json:"capabilities"`
}

type ModelCapabilities struct {
	Generations     bool     `json:"generations,omitempty"`
	Edits           bool     `json:"edits,omitempty"`
	Sizes           []string `json:"sizes,omitempty"`
	Qualities       []string `json:"qualities,omitempty"`
	ResponseFormats []string `json:"response_formats,omitempty"`
	MaxN            uint     `json:"max_n,omitempty"`
	MaxInputImages  uint     `json:"max_input_images,omitempty"`
	SupportsMask    bool     `json:"supports_mask,omitempty"`

	// Presence bits preserve the distinction between an omitted optional
	// override and an explicit false/zero value after JSON decoding.
	generationsSet    bool
	editsSet          bool
	maxInputImagesSet bool
	supportsMaskSet   bool
}

func (c ModelCapabilities) GenerationsSet() bool { return c.generationsSet || c.Generations }
func (c ModelCapabilities) EditsSet() bool       { return c.editsSet || c.Edits }
func (c ModelCapabilities) MaxInputImagesSet() bool {
	return c.maxInputImagesSet || c.MaxInputImages > 0
}
func (c ModelCapabilities) SupportsMaskSet() bool {
	return c.supportsMaskSet || c.SupportsMask
}

func (c *ModelCapabilities) UnmarshalJSON(data []byte) error {
	type plain ModelCapabilities
	var decoded plain
	if err := kitutil.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := kitutil.Unmarshal(data, &fields); err != nil {
		return err
	}
	*c = ModelCapabilities(decoded)
	_, c.generationsSet = fields["generations"]
	_, c.editsSet = fields["edits"]
	_, c.maxInputImagesSet = fields["max_input_images"]
	_, c.supportsMaskSet = fields["supports_mask"]
	return nil
}

func (c ModelCapabilities) MarshalJSON() ([]byte, error) {
	type plain ModelCapabilities
	encoded, err := kitutil.Marshal(plain(c))
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	if err := kitutil.Unmarshal(encoded, &fields); err != nil {
		return nil, err
	}
	if c.generationsSet {
		fields["generations"] = c.Generations
	}
	if c.editsSet {
		fields["edits"] = c.Edits
	}
	if c.maxInputImagesSet {
		fields["max_input_images"] = c.MaxInputImages
	}
	if c.supportsMaskSet {
		fields["supports_mask"] = c.SupportsMask
	}
	return kitutil.Marshal(fields)
}

type CompatibilityStatus string

const (
	CompatibilityUntested CompatibilityStatus = "untested"
	CompatibilityPassed   CompatibilityStatus = "passed"
	CompatibilityFailed   CompatibilityStatus = "failed"
	StatusUntested                            = CompatibilityUntested
	StatusPassed                              = CompatibilityPassed
	StatusFailed                              = CompatibilityFailed
)

type Compatibility struct {
	Status         CompatibilityStatus `json:"status"`
	ProfileVersion int                 `json:"profile_version,omitempty"`
	ContractHash   string              `json:"contract_hash,omitempty"`
	TestedAt       int64               `json:"tested_at,omitempty"`
}

type Binding struct {
	Profile             string                       `json:"profile"`
	ProfileVersion      int                          `json:"profile_version"`
	Paths               map[Endpoint]string          `json:"paths,omitempty"`
	CapabilityOverrides map[string]ModelCapabilities `json:"capability_overrides,omitempty"`
	Compatibility       map[string]Compatibility     `json:"compatibility,omitempty"`
}

var profiles = map[string]Profile{
	profileKey(OpenAIImagesProfile, OpenAIImagesVersion): {
		Name:    OpenAIImagesProfile,
		Version: OpenAIImagesVersion,
		Paths: map[Endpoint]string{
			EndpointGenerations: "/v1/images/generations",
			EndpointEdits:       "/v1/images/edits",
		},
		Capabilities: map[Endpoint]Capability{
			EndpointGenerations: {
				Enabled:         true,
				Sizes:           []string{"256x256", "512x512", "1024x1024", "1536x1024", "1024x1536"},
				Qualities:       []string{"low", "medium", "high"},
				ResponseFormats: []string{"url", "b64_json"},
				MaxN:            MaxImageN,
			},
			EndpointEdits: {
				Enabled:         true,
				Sizes:           []string{"256x256", "512x512", "1024x1024", "1536x1024", "1024x1536"},
				Qualities:       []string{"low", "medium", "high"},
				ResponseFormats: []string{"url", "b64_json"},
				MaxN:            MaxImageN,
				MaxInputImages:  16,
				SupportsMask:    true,
			},
		},
	},
}

func profileKey(name string, version int) string {
	return fmt.Sprintf("%s@%d", name, version)
}

func Lookup(name string, version int) (Profile, bool) {
	profile, ok := profiles[profileKey(strings.TrimSpace(name), version)]
	if !ok {
		return Profile{}, false
	}
	profile.Paths = clonePaths(profile.Paths)
	profile.Capabilities = cloneCapabilities(profile.Capabilities)
	return profile, true
}

func (b Binding) Path(endpoint Endpoint) string {
	if path, ok := b.Paths[endpoint]; ok && strings.TrimSpace(path) != "" {
		return strings.TrimSpace(path)
	}
	profile, ok := Lookup(b.Profile, b.ProfileVersion)
	if !ok {
		return ""
	}
	return profile.Paths[endpoint]
}

func (b Binding) Validate() error {
	profile, ok := Lookup(strings.TrimSpace(b.Profile), b.ProfileVersion)
	if !ok {
		return fmt.Errorf("image profile %q version %d is not registered", strings.TrimSpace(b.Profile), b.ProfileVersion)
	}
	for endpoint, path := range b.Paths {
		if _, exists := profile.Paths[endpoint]; !exists {
			return fmt.Errorf("image profile path endpoint %q is not supported", endpoint)
		}
		if err := validatePath(path); err != nil {
			return fmt.Errorf("image profile path %q: %w", endpoint, err)
		}
	}
	for model, capabilities := range b.CapabilityOverrides {
		if strings.TrimSpace(model) == "" {
			return fmt.Errorf("image profile capability override model is required")
		}
		if err := capabilities.validate(); err != nil {
			return fmt.Errorf("image profile capability override %q: %w", model, err)
		}
	}
	for key, compatibility := range b.Compatibility {
		model, endpoint, ok := strings.Cut(strings.TrimSpace(key), ":")
		if !ok || model == "" || !validEndpoint(Endpoint(endpoint)) {
			return fmt.Errorf("image profile compatibility key %q must be model:endpoint", key)
		}
		switch compatibility.Status {
		case CompatibilityUntested, CompatibilityPassed, CompatibilityFailed:
		default:
			return fmt.Errorf("image profile compatibility %q has invalid status %q", key, compatibility.Status)
		}
		if compatibility.ProfileVersion != 0 && compatibility.ProfileVersion != profile.Version {
			return fmt.Errorf("image profile compatibility %q has profile version %d, want %d", key, compatibility.ProfileVersion, profile.Version)
		}
		if compatibility.TestedAt < 0 {
			return fmt.Errorf("image profile compatibility %q tested_at must not be negative", key)
		}
	}
	return nil
}

func (c ModelCapabilities) validate() error {
	if c.MaxN > MaxImageN {
		return fmt.Errorf("max_n must be between 1 and %d", MaxImageN)
	}
	if c.MaxInputImages > 16 {
		return fmt.Errorf("max_input_images must be between 0 and 16")
	}
	for field, values := range map[string][]string{
		"sizes": c.Sizes, "qualities": c.Qualities, "response_formats": c.ResponseFormats,
	} {
		seen := make(map[string]struct{}, len(values))
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				return fmt.Errorf("%s must not contain empty values", field)
			}
			if _, exists := seen[value]; exists {
				return fmt.Errorf("%s contains duplicate value %q", field, value)
			}
			seen[value] = struct{}{}
		}
	}
	return nil
}

func validatePath(raw string) error {
	path := strings.TrimSpace(raw)
	if path == "" {
		return fmt.Errorf("path is required")
	}
	parsed, err := url.Parse(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	if strings.HasPrefix(path, "/") {
		if strings.HasPrefix(path, "//") {
			return fmt.Errorf("relative path must start with a single slash")
		}
	} else {
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("full URL must use http or https")
		}
		if parsed.Host == "" {
			return fmt.Errorf("full URL must include a host")
		}
		if parsed.User != nil {
			return fmt.Errorf("full URL must not contain userinfo")
		}
	}
	if parsed.RawQuery != "" {
		return fmt.Errorf("path must not contain query")
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("path must not contain fragment")
	}
	return nil
}

func validEndpoint(endpoint Endpoint) bool {
	return endpoint == EndpointGenerations || endpoint == EndpointEdits
}

func clonePaths(paths map[Endpoint]string) map[Endpoint]string {
	clone := make(map[Endpoint]string, len(paths))
	for endpoint, path := range paths {
		clone[endpoint] = path
	}
	return clone
}

func cloneCapabilities(capabilities map[Endpoint]Capability) map[Endpoint]Capability {
	clone := make(map[Endpoint]Capability, len(capabilities))
	for endpoint, capability := range capabilities {
		capability.Sizes = append([]string(nil), capability.Sizes...)
		capability.Qualities = append([]string(nil), capability.Qualities...)
		capability.ResponseFormats = append([]string(nil), capability.ResponseFormats...)
		clone[endpoint] = capability
	}
	return clone
}
