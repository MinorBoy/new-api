// Package image_setting stores the global OpenAI Images model catalog.
package image_setting

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/imageprofile"
	"github.com/shopspring/decimal"
)

const (
	CatalogOptionKey = "ImageModelCatalog"
	CatalogVersion   = 1

	ResolutionTier1K ResolutionTier = "1k"
	ResolutionTier2K ResolutionTier = "2k"
	ResolutionTier4K ResolutionTier = "4k"
)

// OpenAIImage25Models are models sharing the OpenAI Images tier-quality
// matrix. They are kept as independent catalog entries for pricing and
// channel capability configuration.
var OpenAIImage25Models = [...]string{
	"gpt-image-2.5-flare",
	"gpt-image-2.5-sunburst",
}

type ResolutionTier string

const (
	max1KPixels uint64 = 1024 * 1024
	max2KPixels uint64 = 2048 * 2048
	max4KPixels uint64 = 2880 * 2880
)

type Catalog struct {
	Version int                   `json:"version"`
	Models  map[string]ModelEntry `json:"models"`
}

type ModelEntry struct {
	Profile        string                                    `json:"profile"`
	ProfileVersion int                                       `json:"profile_version"`
	Endpoints      map[imageprofile.Endpoint]EndpointCatalog `json:"endpoints"`
	SKUs           map[string]SKU                            `json:"skus"`
}

type EndpointCatalog struct {
	Capability            imageprofile.Capability `json:"capability"`
	DefaultSize           string                  `json:"default_size"`
	DefaultQuality        string                  `json:"default_quality"`
	DefaultResponseFormat string                  `json:"default_response_format"`
}

type SKU struct {
	Endpoint     imageprofile.Endpoint `json:"endpoint"`
	Tier         string                `json:"tier,omitempty"`
	Size         string                `json:"size"`
	Quality      string                `json:"quality"`
	Unit         string                `json:"unit"`
	SalePriceUSD string                `json:"sale_price_usd"`
}

type Selection struct {
	Model, Size, Quality, ResponseFormat string
	Endpoint                             imageprofile.Endpoint
	N, InputImages                       uint
	HasMask                              bool
}

type ResolvedSKU struct {
	CatalogVersion                                             int
	Model, SKUKey, Size, Quality, ResponseFormat, SalePriceUSD string
	Tier                                                       ResolutionTier
	Endpoint                                                   imageprofile.Endpoint
	N, InputImages                                             uint
	HasMask                                                    bool
}

var (
	catalogMu      sync.RWMutex
	currentCatalog = Catalog{Version: CatalogVersion, Models: map[string]ModelEntry{}}
	fixedDecimal   = regexp.MustCompile(`^(0|[0-9]+)(\.[0-9]+)?$`)
)

func ParseCatalogJSONString(raw string) (Catalog, error) {
	var catalog Catalog
	if common.GetJsonType([]byte(raw)) != "object" {
		return Catalog{}, errors.New("image model catalog must be a JSON object")
	}
	if err := common.UnmarshalJsonStr(raw, &catalog); err != nil {
		return Catalog{}, err
	}
	if err := ValidateCatalog(catalog); err != nil {
		return Catalog{}, err
	}
	return cloneCatalog(catalog), nil
}

func ValidateCatalog(catalog Catalog) error {
	if catalog.Version != CatalogVersion {
		return fmt.Errorf("image model catalog version must be %d", CatalogVersion)
	}
	if catalog.Models == nil {
		return errors.New("image model catalog models must be an object")
	}
	for modelName, model := range catalog.Models {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			return errors.New("image model catalog model name must not be empty")
		}
		if model.Profile != imageprofile.OpenAIImagesProfile || model.ProfileVersion != imageprofile.OpenAIImagesVersion {
			return fmt.Errorf("image model catalog model %q must use profile %q version %d", modelName, imageprofile.OpenAIImagesProfile, imageprofile.OpenAIImagesVersion)
		}
		profile, ok := imageprofile.Lookup(model.Profile, model.ProfileVersion)
		if !ok {
			return fmt.Errorf("image model catalog model %q references an unknown profile", modelName)
		}
		if len(model.Endpoints) == 0 {
			return fmt.Errorf("image model catalog model %q must define endpoints", modelName)
		}
		for endpoint, endpointCatalog := range model.Endpoints {
			profileCapability, ok := profile.Capabilities[endpoint]
			if !ok {
				return fmt.Errorf("image model catalog model %q has unsupported endpoint %q", modelName, endpoint)
			}
			if err := validateEndpointCatalog(modelName, endpoint, endpointCatalog, profileCapability); err != nil {
				return err
			}
		}
		if err := validateSKUs(modelName, model); err != nil {
			return err
		}
	}
	return nil
}

func validateEndpointCatalog(model string, endpoint imageprofile.Endpoint, value EndpointCatalog, profile imageprofile.Capability) error {
	capability := value.Capability
	if capability.Enabled {
		if capability.MaxN == 0 || capability.MaxN > imageprofile.MaxImageN {
			return fmt.Errorf("image model catalog model %q endpoint %q max_n must be between 1 and %d", model, endpoint, imageprofile.MaxImageN)
		}
	} else if capability.MaxN > imageprofile.MaxImageN {
		return fmt.Errorf("image model catalog model %q endpoint %q max_n exceeds %d", model, endpoint, imageprofile.MaxImageN)
	}
	if capability.MaxInputImages > profile.MaxInputImages {
		return fmt.Errorf("image model catalog model %q endpoint %q max_input_images exceeds %d", model, endpoint, profile.MaxInputImages)
	}
	if capability.SupportsMask && !profile.SupportsMask {
		return fmt.Errorf("image model catalog model %q endpoint %q cannot enable mask support", model, endpoint)
	}
	if len(capability.ResolutionTiers) > 0 {
		if err := validateResolutionTiers(capability.ResolutionTiers); err != nil {
			return fmt.Errorf("image model catalog model %q endpoint %q: %w", model, endpoint, err)
		}
	}
	if len(capability.Sizes) > 0 {
		if err := validateUniqueOptions("sizes", capability.Sizes); err != nil {
			return fmt.Errorf("image model catalog model %q endpoint %q: %w", model, endpoint, err)
		}
	}
	if err := validateUniqueOptions("qualities", capability.Qualities); err != nil {
		return fmt.Errorf("image model catalog model %q endpoint %q: %w", model, endpoint, err)
	}
	if err := validateUniqueOptions("response_formats", capability.ResponseFormats); err != nil {
		return fmt.Errorf("image model catalog model %q endpoint %q: %w", model, endpoint, err)
	}
	if capability.Enabled {
		if len(capability.Sizes) > 0 && value.DefaultSize != "" && !strings.EqualFold(value.DefaultSize, "auto") && !contains(capability.Sizes, value.DefaultSize) {
			return fmt.Errorf("image model catalog model %q endpoint %q default_size must be supported", model, endpoint)
		}
		if !contains(capability.Qualities, value.DefaultQuality) {
			return fmt.Errorf("image model catalog model %q endpoint %q default_quality must be supported", model, endpoint)
		}
		if !contains(capability.ResponseFormats, value.DefaultResponseFormat) {
			return fmt.Errorf("image model catalog model %q endpoint %q default_response_format must be supported", model, endpoint)
		}
	}
	return nil
}

func validateResolutionTiers(values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		tier := strings.ToLower(strings.TrimSpace(value))
		if !isResolutionTier(ResolutionTier(tier)) {
			return fmt.Errorf("resolution_tiers must contain only 1k, 2k, or 4k")
		}
		if _, exists := seen[tier]; exists {
			return fmt.Errorf("resolution_tiers contains duplicate value %q", value)
		}
		seen[tier] = struct{}{}
	}
	return nil
}

func validateSKUs(modelName string, model ModelEntry) error {
	if len(model.SKUs) == 0 {
		return fmt.Errorf("image model catalog model %q must define SKUs", modelName)
	}
	defaultSKUSeen := make(map[imageprofile.Endpoint]bool)
	for key, sku := range model.SKUs {
		endpointCatalog, ok := model.Endpoints[sku.Endpoint]
		if !ok {
			return fmt.Errorf("image model catalog model %q SKU %q references an undefined endpoint", modelName, key)
		}
		if !endpointCatalog.Capability.Enabled {
			return fmt.Errorf("image model catalog model %q SKU %q uses a disabled endpoint", modelName, key)
		}
		if sku.Unit != "image" {
			return fmt.Errorf("image model catalog model %q SKU %q unit must be image", modelName, key)
		}
		if strings.TrimSpace(sku.Quality) == "" || !contains(endpointCatalog.Capability.Qualities, sku.Quality) {
			return fmt.Errorf("image model catalog model %q SKU %q has unsupported quality", modelName, key)
		}
		skuTier := strings.TrimSpace(sku.Tier)
		if skuTier == "" {
			skuTier = string(tierFromSKUKey(key))
		}
		if skuTier != "" {
			if !isResolutionTier(ResolutionTier(skuTier)) {
				return fmt.Errorf("image model catalog %q SKU %q has invalid resolution tier %q", modelName, key, skuTier)
			}
			expectedKey := BuildTierSKUKey(sku.Endpoint, ResolutionTier(skuTier), sku.Quality)
			if key != expectedKey {
				return fmt.Errorf("image model catalog model %q SKU %q must be named %q", modelName, key, expectedKey)
			}
		} else {
			if strings.TrimSpace(sku.Size) == "" || (len(endpointCatalog.Capability.Sizes) > 0 && !contains(endpointCatalog.Capability.Sizes, sku.Size)) {
				return fmt.Errorf("image model catalog model %q SKU %q has unsupported size", modelName, key)
			}
			expectedKey := BuildSKUKey(sku.Endpoint, sku.Size, sku.Quality)
			if key != expectedKey {
				return fmt.Errorf("image model catalog model %q SKU %q must be named %q", modelName, key, expectedKey)
			}
		}
		if err := validateSalePrice(sku.SalePriceUSD); err != nil {
			return fmt.Errorf("image model catalog model %q SKU %q: %w", modelName, key, err)
		}
		defaultTier, _, _ := ResolveResolutionTier(endpointCatalog.DefaultSize)
		if (skuTier != "" && ResolutionTier(skuTier) == defaultTier || skuTier == "" && sku.Size == endpointCatalog.DefaultSize) && sku.Quality == endpointCatalog.DefaultQuality {
			defaultSKUSeen[sku.Endpoint] = true
		}
	}
	for endpoint, endpointCatalog := range model.Endpoints {
		if endpointCatalog.Capability.Enabled && !defaultSKUSeen[endpoint] {
			return fmt.Errorf("image model catalog model %q endpoint %q default SKU is missing", modelName, endpoint)
		}
	}
	return nil
}

func isResolutionTier(tier ResolutionTier) bool {
	return tier == ResolutionTier1K || tier == ResolutionTier2K || tier == ResolutionTier4K
}

func tierFromSKUKey(key string) ResolutionTier {
	parts := strings.Split(strings.TrimSpace(key), "-")
	if len(parts) != 3 {
		return ""
	}
	tier := ResolutionTier(parts[1])
	if !isResolutionTier(tier) {
		return ""
	}
	return tier
}

func validateUniqueOptions(name string, values []string) error {
	if len(values) == 0 {
		return fmt.Errorf("%s must not be empty", name)
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return fmt.Errorf("%s must not contain empty values", name)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("%s contains duplicate value %q", name, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateSalePrice(raw string) error {
	value := strings.TrimSpace(raw)
	if !fixedDecimal.MatchString(value) {
		return errors.New("sale_price_usd must be a non-negative fixed-point decimal string")
	}
	price, err := decimal.NewFromString(value)
	if err != nil || price.IsNegative() {
		return errors.New("sale_price_usd must be a non-negative fixed-point decimal string")
	}
	return nil
}

func BuildSKUKey(endpoint imageprofile.Endpoint, size, quality string) string {
	prefix := "gen"
	if endpoint == imageprofile.EndpointEdits {
		prefix = "edit"
	}
	return fmt.Sprintf("%s-%s-%s", prefix, strings.TrimSpace(size), strings.TrimSpace(quality))
}

// ResolveResolutionTier classifies a concrete image size by total pixel count.
// Empty and auto sizes use the 1K billing tier while retaining an empty
// normalized size so callers can apply their existing upstream default.
func ResolveResolutionTier(size string) (ResolutionTier, string, error) {
	normalized := strings.TrimSpace(size)
	if normalized == "" || strings.EqualFold(normalized, "auto") {
		return ResolutionTier1K, "", nil
	}
	parts := strings.Split(strings.ToLower(normalized), "x")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("image size %q must use widthxheight format", size)
	}
	width, err := parsePositiveDimension(parts[0])
	if err != nil {
		return "", "", fmt.Errorf("image size %q has invalid width", size)
	}
	height, err := parsePositiveDimension(parts[1])
	if err != nil {
		return "", "", fmt.Errorf("image size %q has invalid height", size)
	}
	if width > math.MaxUint64/height {
		return "", "", fmt.Errorf("image size %q exceeds supported pixel range", size)
	}
	pixels := width * height
	switch {
	case pixels <= max1KPixels:
		return ResolutionTier1K, normalized, nil
	case pixels <= max2KPixels:
		return ResolutionTier2K, normalized, nil
	case pixels <= max4KPixels:
		return ResolutionTier4K, normalized, nil
	default:
		return "", "", fmt.Errorf("image size %q exceeds the 4k pixel limit", size)
	}
}

// ResolveLegacyResolutionTier maps a previously stored concrete-size SKU to
// a billing tier. Legacy catalogs may contain sizes above the current 4K
// request limit (for example 4096x4096); those entries are retained as the
// historical 4K price while new requests are still validated by
// ResolveResolutionTier.
func ResolveLegacyResolutionTier(size string) (ResolutionTier, string, error) {
	tier, normalized, err := ResolveResolutionTier(size)
	if err == nil {
		return tier, normalized, nil
	}
	normalized = strings.TrimSpace(size)
	parts := strings.Split(strings.ToLower(normalized), "x")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", err
	}
	width, widthErr := parsePositiveDimension(parts[0])
	height, heightErr := parsePositiveDimension(parts[1])
	if widthErr != nil || heightErr != nil || width > math.MaxUint64/height {
		return "", "", err
	}
	return ResolutionTier4K, normalized, nil
}

func parsePositiveDimension(value string) (uint64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("dimension is empty")
	}
	var parsed uint64
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return 0, errors.New("dimension is not a positive integer")
		}
		if parsed > (math.MaxUint64-uint64(digit-'0'))/10 {
			return 0, errors.New("dimension overflows uint64")
		}
		parsed = parsed*10 + uint64(digit-'0')
	}
	if parsed == 0 {
		return 0, errors.New("dimension must be positive")
	}
	return parsed, nil
}

func BuildTierSKUKey(endpoint imageprofile.Endpoint, tier ResolutionTier, quality string) string {
	prefix := "gen"
	if endpoint == imageprofile.EndpointEdits {
		prefix = "edit"
	}
	return fmt.Sprintf("%s-%s-%s", prefix, strings.TrimSpace(string(tier)), strings.TrimSpace(quality))
}

func UpdateCatalogByJSONString(raw string) error {
	catalog, err := ParseCatalogJSONString(raw)
	if err != nil {
		return err
	}
	catalogMu.Lock()
	currentCatalog = catalog
	catalogMu.Unlock()
	return nil
}

func Catalog2JSONString() string {
	catalogMu.RLock()
	catalog := cloneCatalog(currentCatalog)
	catalogMu.RUnlock()
	encoded, err := common.Marshal(catalog)
	if err != nil {
		common.SysError("failed to marshal image model catalog: " + err.Error())
		return "{}"
	}
	return string(encoded)
}

func Snapshot() Catalog {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	return cloneCatalog(currentCatalog)
}

// EnsureOpenAIImage25Models adds missing Image 2.5 entries by deep-cloning
// the configured gpt-image-2 entry. Existing entries are never overwritten.
func EnsureOpenAIImage25Models(catalog Catalog) (Catalog, bool) {
	updated := cloneCatalog(catalog)
	source, ok := updated.Models["gpt-image-2"]
	if !ok {
		return updated, false
	}
	changed := false
	for _, modelName := range OpenAIImage25Models {
		if _, exists := updated.Models[modelName]; exists {
			continue
		}
		updated.Models[modelName] = cloneModelEntry(source)
		changed = true
	}
	return updated, changed
}

func Resolve(selection Selection) (ResolvedSKU, error) {
	modelName := strings.TrimSpace(selection.Model)
	if modelName == "" {
		return ResolvedSKU{}, errors.New("image model is required")
	}
	if selection.Endpoint != imageprofile.EndpointGenerations && selection.Endpoint != imageprofile.EndpointEdits {
		return ResolvedSKU{}, fmt.Errorf("image endpoint %q is not supported", selection.Endpoint)
	}
	if selection.N == 0 || selection.N > imageprofile.MaxImageN {
		return ResolvedSKU{}, fmt.Errorf("image n must be between 1 and %d", imageprofile.MaxImageN)
	}
	catalog := Snapshot()
	model, ok := catalog.Models[modelName]
	if !ok {
		return ResolvedSKU{}, fmt.Errorf("image model %q is not configured", modelName)
	}
	endpointCatalog, ok := model.Endpoints[selection.Endpoint]
	if !ok || !endpointCatalog.Capability.Enabled {
		return ResolvedSKU{}, fmt.Errorf("image model %q does not support endpoint %q", modelName, selection.Endpoint)
	}
	sizeInput := strings.TrimSpace(selection.Size)
	tier, normalizedSize, err := ResolveResolutionTier(sizeInput)
	if err != nil {
		return ResolvedSKU{}, err
	}
	size := normalizedSize
	// Missing and auto sizes are billing sentinels for the 1K tier. They must
	// never inherit a concrete catalog default because that would charge and
	// route the request as a different resolution.
	quality := strings.TrimSpace(selection.Quality)
	// The legacy OpenAI image validator fills an omitted gpt-image quality
	// with "auto". Treat that compatibility sentinel as an omitted value when
	// the configured catalog does not expose an explicit auto SKU, so catalog
	// defaults remain authoritative for unified image models.
	if quality == "auto" && !contains(endpointCatalog.Capability.Qualities, quality) {
		quality = ""
	}
	if quality == "" {
		quality = endpointCatalog.DefaultQuality
	}
	responseFormat := strings.TrimSpace(selection.ResponseFormat)
	if responseFormat == "" {
		responseFormat = endpointCatalog.DefaultResponseFormat
	}
	if !contains(endpointCatalog.Capability.Qualities, quality) {
		return ResolvedSKU{}, fmt.Errorf("image quality %q is not supported for model %q endpoint %q", quality, modelName, selection.Endpoint)
	}
	if !contains(endpointCatalog.Capability.ResponseFormats, responseFormat) {
		return ResolvedSKU{}, fmt.Errorf("image response format %q is not supported for model %q endpoint %q", responseFormat, modelName, selection.Endpoint)
	}
	capability := endpointCatalog.Capability
	if selection.N > capability.MaxN {
		return ResolvedSKU{}, fmt.Errorf("image n exceeds model %q endpoint %q max_n", modelName, selection.Endpoint)
	}
	if selection.Endpoint == imageprofile.EndpointGenerations && selection.InputImages > 0 {
		return ResolvedSKU{}, errors.New("image generations does not accept input images")
	}
	if selection.InputImages > capability.MaxInputImages {
		return ResolvedSKU{}, fmt.Errorf("image input_images exceeds model %q endpoint %q max_input_images", modelName, selection.Endpoint)
	}
	if selection.HasMask && !capability.SupportsMask {
		return ResolvedSKU{}, fmt.Errorf("image model %q endpoint %q does not support mask", modelName, selection.Endpoint)
	}
	skuKey := BuildTierSKUKey(selection.Endpoint, tier, quality)
	sku, ok := model.SKUs[skuKey]
	if !ok || sku.Endpoint != selection.Endpoint {
		// Keep existing catalogs usable until their concrete-size rules are
		// migrated. New catalogs always resolve the tier key above.
		legacyKey := BuildSKUKey(selection.Endpoint, size, quality)
		if size == "" {
			// A legacy catalog may use default_size=auto, so there is no
			// concrete key to construct. Find the first legacy SKU in the
			// requested tier instead of treating auto as a literal size.
			for candidateKey, candidate := range model.SKUs {
				if candidate.Endpoint != selection.Endpoint || candidate.Quality != quality {
					continue
				}
				candidateTier, _, candidateErr := ResolveResolutionTier(candidate.Size)
				if candidateErr == nil && candidateTier == tier {
					legacyKey, sku, ok = candidateKey, candidate, true
					break
				}
			}
		} else {
			sku, ok = model.SKUs[legacyKey]
		}
		if !ok || sku.Endpoint != selection.Endpoint {
			return ResolvedSKU{}, fmt.Errorf("image SKU %q is not configured for model %q", skuKey, modelName)
		}
		skuKey = legacyKey
	}
	return ResolvedSKU{
		CatalogVersion: catalog.Version,
		Model:          modelName,
		SKUKey:         skuKey,
		Tier:           tier,
		Size:           size,
		Quality:        quality,
		ResponseFormat: responseFormat,
		SalePriceUSD:   sku.SalePriceUSD,
		Endpoint:       selection.Endpoint,
		N:              selection.N,
		InputImages:    selection.InputImages,
		HasMask:        selection.HasMask,
	}, nil
}

func cloneCatalog(catalog Catalog) Catalog {
	clone := Catalog{Version: catalog.Version, Models: make(map[string]ModelEntry, len(catalog.Models))}
	for modelName, model := range catalog.Models {
		clone.Models[modelName] = cloneModelEntry(model)
	}
	return clone
}

func cloneModelEntry(model ModelEntry) ModelEntry {
	entry := ModelEntry{
		Profile:        model.Profile,
		ProfileVersion: model.ProfileVersion,
		Endpoints:      make(map[imageprofile.Endpoint]EndpointCatalog, len(model.Endpoints)),
		SKUs:           make(map[string]SKU, len(model.SKUs)),
	}
	for endpoint, endpointCatalog := range model.Endpoints {
		capability := endpointCatalog.Capability
		capability.ResolutionTiers = append([]string(nil), capability.ResolutionTiers...)
		capability.ResolutionQualities = append([]string(nil), capability.ResolutionQualities...)
		capability.Sizes = append([]string(nil), capability.Sizes...)
		capability.Qualities = append([]string(nil), capability.Qualities...)
		capability.ResponseFormats = append([]string(nil), capability.ResponseFormats...)
		endpointCatalog.Capability = capability
		entry.Endpoints[endpoint] = endpointCatalog
	}
	for key, sku := range model.SKUs {
		entry.SKUs[key] = sku
	}
	return entry
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// CatalogModels returns model names in stable order for admin APIs.
func CatalogModels() []string {
	catalog := Snapshot()
	models := make([]string, 0, len(catalog.Models))
	for model := range catalog.Models {
		models = append(models, model)
	}
	sort.Strings(models)
	return models
}
