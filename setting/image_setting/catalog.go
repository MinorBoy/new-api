// Package image_setting stores the global OpenAI Images model catalog.
package image_setting

import (
	"errors"
	"fmt"
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
	if err := validateUniqueOptions("sizes", capability.Sizes); err != nil {
		return fmt.Errorf("image model catalog model %q endpoint %q: %w", model, endpoint, err)
	}
	if err := validateUniqueOptions("qualities", capability.Qualities); err != nil {
		return fmt.Errorf("image model catalog model %q endpoint %q: %w", model, endpoint, err)
	}
	if err := validateUniqueOptions("response_formats", capability.ResponseFormats); err != nil {
		return fmt.Errorf("image model catalog model %q endpoint %q: %w", model, endpoint, err)
	}
	if capability.Enabled {
		if !contains(capability.Sizes, value.DefaultSize) {
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
		if strings.TrimSpace(sku.Size) == "" || !contains(endpointCatalog.Capability.Sizes, sku.Size) {
			return fmt.Errorf("image model catalog model %q SKU %q has unsupported size", modelName, key)
		}
		if strings.TrimSpace(sku.Quality) == "" || !contains(endpointCatalog.Capability.Qualities, sku.Quality) {
			return fmt.Errorf("image model catalog model %q SKU %q has unsupported quality", modelName, key)
		}
		expectedKey := BuildSKUKey(sku.Endpoint, sku.Size, sku.Quality)
		if key != expectedKey {
			return fmt.Errorf("image model catalog model %q SKU %q must be named %q", modelName, key, expectedKey)
		}
		if err := validateSalePrice(sku.SalePriceUSD); err != nil {
			return fmt.Errorf("image model catalog model %q SKU %q: %w", modelName, key, err)
		}
		if sku.Size == endpointCatalog.DefaultSize && sku.Quality == endpointCatalog.DefaultQuality {
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
	size := strings.TrimSpace(selection.Size)
	if size == "" {
		size = endpointCatalog.DefaultSize
	}
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
	if !contains(endpointCatalog.Capability.Sizes, size) {
		return ResolvedSKU{}, fmt.Errorf("image size %q is not supported for model %q endpoint %q", size, modelName, selection.Endpoint)
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
	skuKey := BuildSKUKey(selection.Endpoint, size, quality)
	sku, ok := model.SKUs[skuKey]
	if !ok || sku.Endpoint != selection.Endpoint {
		return ResolvedSKU{}, fmt.Errorf("image SKU %q is not configured for model %q", skuKey, modelName)
	}
	return ResolvedSKU{
		CatalogVersion: catalog.Version,
		Model:          modelName,
		SKUKey:         skuKey,
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
		entry := ModelEntry{
			Profile:        model.Profile,
			ProfileVersion: model.ProfileVersion,
			Endpoints:      make(map[imageprofile.Endpoint]EndpointCatalog, len(model.Endpoints)),
			SKUs:           make(map[string]SKU, len(model.SKUs)),
		}
		for endpoint, endpointCatalog := range model.Endpoints {
			capability := endpointCatalog.Capability
			capability.Sizes = append([]string(nil), capability.Sizes...)
			capability.Qualities = append([]string(nil), capability.Qualities...)
			capability.ResponseFormats = append([]string(nil), capability.ResponseFormats...)
			endpointCatalog.Capability = capability
			entry.Endpoints[endpoint] = endpointCatalog
		}
		for key, sku := range model.SKUs {
			entry.SKUs[key] = sku
		}
		clone.Models[modelName] = entry
	}
	return clone
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
