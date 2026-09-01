package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/imageprofile"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/image_setting"
)

type ImageCompatibilityTestRequest struct {
	PublicModel string                `json:"model"`
	Endpoint    imageprofile.Endpoint `json:"endpoint"`
}

type ImageCompatibilityTestResult struct {
	Status         imageprofile.CompatibilityStatus `json:"status"`
	ProfileVersion int                              `json:"profile_version"`
	ContractHash   string                           `json:"contract_hash"`
	TestedAt       int64                            `json:"tested_at"`
	ErrorSummary   string                           `json:"error_summary,omitempty"`
}

// RunImageCompatibilityTest sends a minimal, bounded request to one channel.
// It returns only a redacted summary and a contract hash; upstream response
// bodies, credentials, and signed URLs are never copied into the result.
func RunImageCompatibilityTest(ctx context.Context, channel *model.Channel, request ImageCompatibilityTestRequest) (ImageCompatibilityTestResult, error) {
	result := ImageCompatibilityTestResult{Status: imageprofile.CompatibilityFailed, TestedAt: time.Now().Unix()}
	if channel == nil {
		return result, errors.New("channel is required")
	}
	publicModel := strings.TrimSpace(request.PublicModel)
	if publicModel == "" {
		return result, errors.New("image model is required")
	}
	if request.Endpoint != imageprofile.EndpointGenerations && request.Endpoint != imageprofile.EndpointEdits {
		return result, errors.New("image endpoint is not supported")
	}
	settings := channel.GetOtherSettings()
	if settings.ImageProfile == nil {
		return result, errors.New("channel has no image profile binding")
	}
	if err := settings.ImageProfile.Validate(); err != nil {
		return result, err
	}
	profile, ok := imageprofile.Lookup(settings.ImageProfile.Profile, settings.ImageProfile.ProfileVersion)
	if !ok {
		return result, errors.New("channel image profile is not registered")
	}
	if !model.SupportsOpenAIImagesChannelType(channel.Type) {
		return result, errors.New("channel is not OpenAI Images compatible")
	}
	endpointCatalog, err := imageTestEndpointCatalog(publicModel, request.Endpoint)
	if err != nil {
		return result, err
	}
	resolved, err := image_setting.Resolve(image_setting.Selection{
		Model: publicModel, Endpoint: request.Endpoint, Size: endpointCatalog.DefaultSize,
		Quality: endpointCatalog.DefaultQuality, ResponseFormat: endpointCatalog.DefaultResponseFormat, N: 1,
	})
	if err != nil {
		return result, err
	}
	eligibility, err := EvaluateImageChannel(channel, publicModel, ImageRequestContext{Resolved: resolved})
	if err != nil {
		return result, err
	}
	hash, err := ResolveImageContractHash(channel, publicModel, ImageRequestContext{Resolved: resolved})
	if err != nil {
		return result, err
	}
	result.ProfileVersion = profile.Version
	result.ContractHash = hash

	upstreamURL := settings.ImageProfile.Path(request.Endpoint)
	if strings.HasPrefix(upstreamURL, "/") {
		upstreamURL = relaycommon.GetFullRequestURL(channel.GetBaseURL(), upstreamURL, channel.Type)
	}
	var body io.Reader
	contentType := "application/json"
	if request.Endpoint == imageprofile.EndpointEdits {
		var buffer bytes.Buffer
		writer := multipart.NewWriter(&buffer)
		_ = writer.WriteField("model", eligibility.UpstreamModel)
		_ = writer.WriteField("size", resolved.Size)
		_ = writer.WriteField("quality", resolved.Quality)
		_ = writer.WriteField("response_format", resolved.ResponseFormat)
		_ = writer.WriteField("n", "1")
		part, partErr := writer.CreateFormFile("image", "compatibility.png")
		if partErr != nil {
			return result, partErr
		}
		if _, copyErr := part.Write(minimalPNG); copyErr != nil {
			return result, copyErr
		}
		if endpointCatalog.Capability.SupportsMask {
			maskPart, maskErr := writer.CreateFormFile("mask", "compatibility-mask.png")
			if maskErr != nil {
				return result, maskErr
			}
			if _, copyErr := maskPart.Write(minimalPNG); copyErr != nil {
				return result, copyErr
			}
		}
		if closeErr := writer.Close(); closeErr != nil {
			return result, closeErr
		}
		body = &buffer
		contentType = writer.FormDataContentType()
	} else {
		payload, marshalErr := common.Marshal(map[string]any{
			"model": eligibility.UpstreamModel, "size": resolved.Size, "quality": resolved.Quality,
			"response_format": resolved.ResponseFormat, "n": 1, "prompt": "compatibility test",
		})
		if marshalErr != nil {
			return result, marshalErr
		}
		payload, marshalErr = relaycommon.ApplyParamOverride(payload, channel.GetParamOverride(), nil)
		if marshalErr != nil {
			return result, errors.New("image compatibility parameter override is invalid")
		}
		body = bytes.NewReader(payload)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(testCtx, http.MethodPost, upstreamURL, body)
	if err != nil {
		return result, err
	}
	req.Header.Set("Content-Type", contentType)
	if channel.Type == constant.ChannelTypeAzure {
		req.Header.Set("api-key", channel.Key)
	} else {
		req.Header.Set("Authorization", "Bearer "+channel.Key)
		if channel.Type == constant.ChannelTypeOpenAI && channel.OpenAIOrganization != nil && strings.TrimSpace(*channel.OpenAIOrganization) != "" {
			req.Header.Set("OpenAI-Organization", strings.TrimSpace(*channel.OpenAIOrganization))
		}
	}
	for key, value := range channel.GetHeaderOverride() {
		str, ok := value.(string)
		if !ok {
			return result, errors.New("image compatibility header override is invalid")
		}
		if strings.Contains(str, "{client_header:") {
			continue
		}
		str = strings.ReplaceAll(str, "{api_key}", channel.Key)
		if strings.TrimSpace(str) != "" {
			req.Header.Set(key, str)
		}
	}
	client, err := GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return result, redactImageCompatibilityError(err)
	}
	response, err := client.Do(req)
	if err != nil {
		return result, redactImageCompatibilityError(err)
	}
	defer response.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if readErr != nil {
		return result, redactImageCompatibilityError(readErr)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		result.ErrorSummary = fmt.Sprintf("upstream returned HTTP %d", response.StatusCode)
		return result, nil
	}
	var payload struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := common.Unmarshal(responseBody, &payload); err != nil {
		result.ErrorSummary = "upstream returned invalid JSON"
		return result, nil
	}
	if len(payload.Data) == 0 {
		result.ErrorSummary = "upstream returned no images"
		return result, nil
	}
	for _, image := range payload.Data {
		if strings.TrimSpace(image.URL) == "" && strings.TrimSpace(image.B64JSON) == "" {
			result.ErrorSummary = "upstream image item is missing url and b64_json"
			return result, nil
		}
	}
	result.Status = imageprofile.CompatibilityPassed
	return result, nil
}

func imageTestEndpointCatalog(publicModel string, endpoint imageprofile.Endpoint) (image_setting.EndpointCatalog, error) {
	catalog := image_setting.Snapshot()
	modelEntry, ok := catalog.Models[strings.TrimSpace(publicModel)]
	if !ok {
		return image_setting.EndpointCatalog{}, fmt.Errorf("image model %q is not configured", publicModel)
	}
	endpointCatalog, ok := modelEntry.Endpoints[endpoint]
	if !ok || !endpointCatalog.Capability.Enabled {
		return image_setting.EndpointCatalog{}, fmt.Errorf("image model %q endpoint %q is not configured", publicModel, endpoint)
	}
	return endpointCatalog, nil
}

func redactImageCompatibilityError(err error) error {
	if err == nil {
		return nil
	}
	return errors.New("image compatibility request failed")
}

var minimalPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
	0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0xf0,
	0x1f, 0x00, 0x05, 0x00, 0x01, 0xff, 0x89, 0x99,
	0x3d, 0x1d, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45,
	0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}
