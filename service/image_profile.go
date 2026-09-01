package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/imageprofile"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/image_setting"
)

// ImageRequestContext is the immutable, normalized image contract carried by
// routing and billing for one request.
type ImageRequestContext struct {
	Resolved image_setting.ResolvedSKU
}

type ImageChannelEligibility struct {
	ChannelID      int
	Priority       int64
	Weight         int
	UpstreamModel  string
	CostVariantKey string
	ContractHash   string
}

func ResolveImageRequest(request *dto.ImageRequest, relayMode int) (ImageRequestContext, error) {
	if request == nil {
		return ImageRequestContext{}, errors.New("image request is required")
	}
	endpoint := imageprofile.Endpoint("")
	switch relayMode {
	case relayconstant.RelayModeImagesGenerations:
		endpoint = imageprofile.EndpointGenerations
	case relayconstant.RelayModeImagesEdits:
		endpoint = imageprofile.EndpointEdits
	default:
		return ImageRequestContext{}, fmt.Errorf("relay mode %d is not an OpenAI Images endpoint", relayMode)
	}
	n := uint(1)
	if request.N != nil {
		n = *request.N
	}
	resolved, err := image_setting.Resolve(image_setting.Selection{
		Model:          request.Model,
		Size:           request.Size,
		Quality:        request.Quality,
		ResponseFormat: request.ResponseFormat,
		Endpoint:       endpoint,
		N:              n,
		InputImages:    request.InputImageCount,
		HasMask:        request.HasMask,
	})
	if err != nil {
		return ImageRequestContext{}, err
	}
	return ImageRequestContext{Resolved: resolved}, nil
}

// HasModel reports whether a public model is managed by the unified image
// catalog. Legacy image models therefore remain on their existing path.
func HasImageModel(modelName string) bool {
	_, ok := image_setting.Snapshot().Models[strings.TrimSpace(modelName)]
	return ok
}

func EvaluateImageChannel(channel *model.Channel, publicModel string, request ImageRequestContext) (ImageChannelEligibility, error) {
	if channel == nil {
		return ImageChannelEligibility{}, errors.New("image channel is required")
	}
	if !model.SupportsOpenAIImagesChannelType(channel.Type) {
		return ImageChannelEligibility{}, errors.New("channel is not OpenAI Images compatible")
	}
	settings := channel.GetOtherSettings()
	if settings.ImageProfile == nil {
		return ImageChannelEligibility{}, errors.New("channel has no image profile binding")
	}
	if err := settings.ImageProfile.Validate(); err != nil {
		return ImageChannelEligibility{}, err
	}
	publicModel = strings.TrimSpace(publicModel)
	if publicModel == "" {
		publicModel = request.Resolved.Model
	}
	if !containsModel(channel.GetModels(), publicModel) {
		return ImageChannelEligibility{}, errors.New("channel does not advertise the public image model")
	}
	upstreamModel := publicModel
	if mapping := strings.TrimSpace(channel.GetModelMapping()); mapping != "" && mapping != "{}" {
		mapped, _, err := ResolveMappedModel(publicModel, mapping)
		if err != nil {
			return ImageChannelEligibility{}, err
		}
		upstreamModel = mapped
	}
	profile, ok := imageprofile.Lookup(settings.ImageProfile.Profile, settings.ImageProfile.ProfileVersion)
	if !ok {
		return ImageChannelEligibility{}, errors.New("channel image profile is not registered")
	}
	capability, ok := profile.Capabilities[request.Resolved.Endpoint]
	if !ok || !capability.Enabled {
		return ImageChannelEligibility{}, errors.New("channel image endpoint is disabled")
	}
	if override, exists := settings.ImageProfile.CapabilityOverrides[publicModel]; exists {
		if override.GenerationsSet() && !override.Generations && request.Resolved.Endpoint == imageprofile.EndpointGenerations {
			return ImageChannelEligibility{}, errors.New("channel generations capability is disabled")
		}
		if override.EditsSet() && !override.Edits && request.Resolved.Endpoint == imageprofile.EndpointEdits {
			return ImageChannelEligibility{}, errors.New("channel edits capability is disabled")
		}
		if len(override.Sizes) > 0 && !contains(override.Sizes, request.Resolved.Size) {
			return ImageChannelEligibility{}, errors.New("channel image size is not supported")
		}
		if len(override.Qualities) > 0 && !contains(override.Qualities, request.Resolved.Quality) {
			return ImageChannelEligibility{}, errors.New("channel image quality is not supported")
		}
		if len(override.ResponseFormats) > 0 && !contains(override.ResponseFormats, request.Resolved.ResponseFormat) {
			return ImageChannelEligibility{}, errors.New("channel image response format is not supported")
		}
		if override.MaxN > 0 && request.Resolved.N > override.MaxN {
			return ImageChannelEligibility{}, errors.New("channel image n exceeds max_n")
		}
		if override.MaxInputImagesSet() && request.Resolved.InputImages > override.MaxInputImages {
			return ImageChannelEligibility{}, errors.New("channel image input count exceeds max_input_images")
		}
		if request.Resolved.HasMask && override.SupportsMaskSet() && !override.SupportsMask {
			return ImageChannelEligibility{}, errors.New("channel image mask is not supported")
		}
	}
	return ImageChannelEligibility{
		ChannelID:      channel.Id,
		Priority:       channel.GetPriority(),
		Weight:         channel.GetWeight(),
		UpstreamModel:  upstreamModel,
		CostVariantKey: request.Resolved.SKUKey,
	}, nil
}

func ResolveImageContractHash(channel *model.Channel, publicModel string, request ImageRequestContext) (string, error) {
	eligibility, err := EvaluateImageChannel(channel, publicModel, request)
	if err != nil {
		return "", err
	}
	publicModel = strings.TrimSpace(publicModel)
	settings := channel.GetOtherSettings()
	profile, ok := imageprofile.Lookup(settings.ImageProfile.Profile, settings.ImageProfile.ProfileVersion)
	if !ok {
		return "", errors.New("channel image profile is not registered")
	}
	catalog := image_setting.Snapshot()
	modelEntry, ok := catalog.Models[publicModel]
	if !ok {
		return "", fmt.Errorf("image model %q is not configured", publicModel)
	}
	endpointCatalog, ok := modelEntry.Endpoints[request.Resolved.Endpoint]
	if !ok {
		return "", fmt.Errorf("image model %q endpoint %q is not configured", publicModel, request.Resolved.Endpoint)
	}
	effectiveURL, err := imageEffectiveEndpointURL(channel, request.Resolved.Endpoint)
	if err != nil {
		return "", err
	}
	headerDigest, err := imageOverrideDigest(channel.GetHeaderOverride())
	if err != nil {
		return "", err
	}
	paramDigest, err := imageOverrideDigest(channel.GetParamOverride())
	if err != nil {
		return "", err
	}
	organization := ""
	if channel.OpenAIOrganization != nil {
		organization = strings.TrimSpace(*channel.OpenAIOrganization)
	}
	document := struct {
		Profile            string                         `json:"profile"`
		ProfileVersion     int                            `json:"profile_version"`
		CatalogVersion     int                            `json:"catalog_version"`
		PublicModel        string                         `json:"public_model"`
		UpstreamModel      string                         `json:"upstream_model"`
		Endpoint           imageprofile.Endpoint          `json:"endpoint"`
		Path               string                         `json:"path"`
		EffectiveURL       string                         `json:"effective_url"`
		ChannelType        int                            `json:"channel_type"`
		Organization       string                         `json:"organization,omitempty"`
		Proxy              string                         `json:"proxy,omitempty"`
		HeaderOverrideHash string                         `json:"header_override_hash,omitempty"`
		ParamOverrideHash  string                         `json:"param_override_hash,omitempty"`
		ProfileCapability  imageprofile.Capability        `json:"profile_capability"`
		CatalogCapability  imageprofile.Capability        `json:"catalog_capability"`
		CapabilityOverride imageprofile.ModelCapabilities `json:"capability_override,omitempty"`
		SKUKey             string                         `json:"sku_key"`
	}{
		Profile:            profile.Name,
		ProfileVersion:     profile.Version,
		CatalogVersion:     catalog.Version,
		PublicModel:        publicModel,
		UpstreamModel:      eligibility.UpstreamModel,
		Endpoint:           request.Resolved.Endpoint,
		Path:               settings.ImageProfile.Path(request.Resolved.Endpoint),
		EffectiveURL:       effectiveURL,
		ChannelType:        channel.Type,
		Organization:       organization,
		Proxy:              imageContractProxy(channel.GetSetting().Proxy),
		HeaderOverrideHash: headerDigest,
		ParamOverrideHash:  paramDigest,
		ProfileCapability:  profile.Capabilities[request.Resolved.Endpoint],
		CatalogCapability:  endpointCatalog.Capability,
		SKUKey:             request.Resolved.SKUKey,
	}
	if override, exists := settings.ImageProfile.CapabilityOverrides[publicModel]; exists {
		document.CapabilityOverride = override
	}
	encoded, err := common.Marshal(document)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func imageEffectiveEndpointURL(channel *model.Channel, endpoint imageprofile.Endpoint) (string, error) {
	if channel == nil {
		return "", errors.New("image channel is required")
	}
	settings := channel.GetOtherSettings()
	if settings.ImageProfile == nil {
		return "", errors.New("channel has no image profile binding")
	}
	path := settings.ImageProfile.Path(endpoint)
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("image profile path is not configured for endpoint %s", endpoint)
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return imageContractURL(path), nil
	}
	return imageContractURL(relaycommon.GetFullRequestURL(channel.GetBaseURL(), path, channel.Type)), nil
}

func imageContractURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return strings.TrimSpace(raw)
	}
	parsed.User = nil
	query := parsed.Query()
	for key := range query {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "key") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "sig") {
			query.Set(key, "<redacted>")
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func imageContractProxy(raw string) string {
	return imageContractURL(raw)
}

func imageOverrideDigest(value map[string]interface{}) (string, error) {
	if len(value) == 0 {
		return "", nil
	}
	encoded, err := common.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

// MergeStoredImageCompatibility keeps compatibility results out of ordinary
// channel CRUD input. A result survives only when its stored contract hash
// still matches the submitted channel contract; otherwise it is invalidated.
func MergeStoredImageCompatibility(stored, submitted *model.Channel) error {
	if submitted == nil {
		return errors.New("submitted channel is required")
	}
	return mergeImageCompatibility(stored, submitted)
}

// ClearImageCompatibility removes caller-supplied compatibility results when a
// channel is created. Only the dedicated compatibility test endpoint may write
// a passed/failed result.
func ClearImageCompatibility(channel *model.Channel) error {
	return mergeImageCompatibility(nil, channel)
}

func mergeImageCompatibility(stored, submitted *model.Channel) error {
	if submitted == nil {
		return errors.New("submitted channel is required")
	}
	var submittedRaw map[string]json.RawMessage
	if strings.TrimSpace(submitted.OtherSettings) == "" {
		submittedRaw = map[string]json.RawMessage{}
	} else if err := common.UnmarshalJsonStr(submitted.OtherSettings, &submittedRaw); err != nil {
		return err
	}
	var submittedBinding imageprofile.Binding
	rawBinding, hasBinding := submittedRaw["image_profile"]
	if !hasBinding || common.Unmarshal(rawBinding, &submittedBinding) != nil || submittedBinding.Profile == "" {
		return nil
	}
	retained := map[string]imageprofile.Compatibility{}
	if stored != nil {
		var storedRaw map[string]json.RawMessage
		if strings.TrimSpace(stored.OtherSettings) != "" {
			if err := common.UnmarshalJsonStr(stored.OtherSettings, &storedRaw); err != nil {
				return err
			}
		}
		var storedBinding imageprofile.Binding
		if raw, ok := storedRaw["image_profile"]; ok && common.Unmarshal(raw, &storedBinding) == nil {
			for key, compatibility := range storedBinding.Compatibility {
				if compatibility.ContractHash == "" {
					continue
				}
				hash, err := imageContractHashForKey(submitted, key)
				if err == nil && hash == compatibility.ContractHash {
					retained[key] = compatibility
				}
			}
		}
	}
	submittedBinding.Compatibility = retained
	encoded, err := common.Marshal(submittedBinding)
	if err != nil {
		return err
	}
	submittedRaw["image_profile"] = encoded
	settings, err := common.Marshal(submittedRaw)
	if err != nil {
		return err
	}
	submitted.OtherSettings = string(settings)
	return nil
}

func imageContractHashForKey(channel *model.Channel, key string) (string, error) {
	publicModel, endpointName, ok := strings.Cut(strings.TrimSpace(key), ":")
	if !ok || publicModel == "" {
		return "", errors.New("invalid image compatibility key")
	}
	endpoint := imageprofile.Endpoint(endpointName)
	catalog := image_setting.Snapshot()
	entry, ok := catalog.Models[publicModel]
	if !ok {
		return "", errors.New("image model is not configured")
	}
	endpointCatalog, ok := entry.Endpoints[endpoint]
	if !ok {
		return "", errors.New("image endpoint is not configured")
	}
	resolved, err := image_setting.Resolve(image_setting.Selection{
		Model: publicModel, Endpoint: endpoint, Size: endpointCatalog.DefaultSize,
		Quality: endpointCatalog.DefaultQuality, ResponseFormat: endpointCatalog.DefaultResponseFormat, N: 1,
	})
	if err != nil {
		return "", err
	}
	return ResolveImageContractHash(channel, publicModel, ImageRequestContext{Resolved: resolved})
}

func containsModel(models []string, target string) bool {
	for _, modelName := range models {
		if strings.TrimSpace(modelName) == target {
			return true
		}
	}
	return false
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}
