package helper

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
)

// NormalizeSeedreamNativeImageRequest validates the request fields that affect
// image-generation billing and converts the bounded output estimate into the
// canonical OpenAI request count used by pre-consume billing. The outbound ARK
// body is never changed by this normalization.
func NormalizeSeedreamNativeImageRequest(c *gin.Context, request *dto.ImageRequest) error {
	if c == nil || request == nil || !c.GetBool(common.KeySeedanceOfficialAPI) {
		return nil
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return err
	}
	rawBody, err := storage.Bytes()
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(rawBody, &fields); err != nil {
		return fmt.Errorf("invalid Seedream request body: %w", err)
	}
	inputImages := seedreamNativeInputImageCount(fields["image"])
	if inputImages >= 15 {
		return errors.New("input image count must be less than 15")
	}

	sequence := "disabled"
	if value, ok := fields["sequential_image_generation"]; ok {
		if err := common.Unmarshal(value, &sequence); err != nil {
			return errors.New("sequential_image_generation must be a string")
		}
		sequence = strings.ToLower(strings.TrimSpace(sequence))
		if sequence != "auto" && sequence != "disabled" {
			return errors.New("sequential_image_generation must be auto or disabled")
		}
	}
	options, hasOptions := fields["sequential_image_generation_options"]
	if sequence != "auto" && hasOptions {
		return errors.New("sequential_image_generation_options requires sequential_image_generation=auto")
	}
	if sequence == "disabled" {
		request.N = common.GetPointer(uint(1))
		return nil
	}

	maxImages := int64(15)
	if hasOptions {
		var optionFields map[string]json.RawMessage
		if err := common.Unmarshal(options, &optionFields); err != nil {
			return errors.New("sequential_image_generation_options must be an object")
		}
		if maxValue, ok := optionFields["max_images"]; ok {
			if err := common.Unmarshal(maxValue, &maxImages); err != nil || maxImages < 1 || maxImages > 15 {
				return errors.New("max_images must be between 1 and 15")
			}
		}
	}
	outputLimit := maxImages
	if available := int64(15 - inputImages); outputLimit > available {
		outputLimit = available
	}
	if outputLimit < 1 {
		return errors.New("input images leave no output image capacity")
	}
	request.N = common.GetPointer(uint(outputLimit))
	return nil
}

func seedreamNativeInputImageCount(raw json.RawMessage) int {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var value string
	if err := common.Unmarshal(raw, &value); err == nil {
		if strings.TrimSpace(value) != "" {
			return 1
		}
		return 0
	}
	var values []json.RawMessage
	if err := common.Unmarshal(raw, &values); err != nil {
		return 0
	}
	count := 0
	for _, item := range values {
		if len(item) > 0 && string(item) != "null" {
			count++
		}
	}
	return count
}

// ValidateSeedreamNativeModelRequest applies the documented Seedream family
// capability matrix after channel model mapping has selected the upstream ID.
func ValidateSeedreamNativeModelRequest(c *gin.Context, request *dto.ImageRequest, upstreamModel string) error {
	if c == nil || request == nil || !c.GetBool(common.KeySeedanceOfficialAPI) {
		return nil
	}
	family := seedreamModelFamily(upstreamModel)
	if family == "" {
		return nil
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return err
	}
	rawBody, err := storage.Bytes()
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(rawBody, &fields); err != nil {
		return fmt.Errorf("invalid Seedream request body: %w", err)
	}
	inputImages := seedreamNativeInputImageCount(fields["image"])
	maxInputImages := 14
	if family == "pro" {
		maxInputImages = 10
	}
	if inputImages > maxInputImages {
		return fmt.Errorf("%s supports at most %d input images", upstreamModel, maxInputImages)
	}

	sequenceRaw, hasSequence := fields["sequential_image_generation"]
	sequence := "disabled"
	if hasSequence {
		if err := common.Unmarshal(sequenceRaw, &sequence); err != nil {
			return errors.New("sequential_image_generation must be a string")
		}
		sequence = strings.ToLower(strings.TrimSpace(sequence))
	}
	if family == "pro" {
		if hasSequence || hasJSONField(fields, "sequential_image_generation_options") {
			return errors.New("Seedream Pro does not support sequential image generation")
		}
		if stream, ok, err := seedreamNativeBool(fields["stream"]); err != nil {
			return err
		} else if ok && stream {
			return errors.New("Seedream Pro does not support stream")
		}
		if request.N != nil && *request.N > 1 {
			return errors.New("Seedream Pro only supports one output image")
		}
	} else {
		if hasSequence && sequence != "auto" && sequence != "disabled" {
			return errors.New("sequential_image_generation must be auto or disabled")
		}
		if hasJSONField(fields, "sequential_image_generation_options") && sequence != "auto" {
			return errors.New("sequential_image_generation_options requires sequential_image_generation=auto")
		}
	}

	if hasJSONField(fields, "output_format") && family != "pro" && family != "lite" {
		return errors.New("output_format is supported only by Seedream Pro and Lite")
	}
	if hasJSONField(fields, "tools") && family != "lite" {
		return errors.New("tools is supported only by Seedream Lite")
	}
	if hasJSONField(fields, "optimize_prompt_options") {
		var options struct {
			Mode string `json:"mode"`
		}
		if err := common.Unmarshal(fields["optimize_prompt_options"], &options); err != nil {
			return errors.New("optimize_prompt_options must be an object")
		}
		if strings.EqualFold(options.Mode, "fast") && family != "4.0" {
			return errors.New("optimize_prompt_options.mode=fast is supported only by Seedream 4.0")
		}
	}
	return nil
}

func seedreamNativeBool(raw json.RawMessage) (bool, bool, error) {
	if len(raw) == 0 {
		return false, false, nil
	}
	var value bool
	if err := common.Unmarshal(raw, &value); err != nil {
		return false, true, errors.New("stream must be a boolean")
	}
	return value, true, nil
}

func hasJSONField(fields map[string]json.RawMessage, key string) bool {
	_, ok := fields[key]
	return ok
}

func seedreamModelFamily(modelName string) string {
	name := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(modelName), ".", "-"))
	switch {
	case strings.Contains(name, "seedream-5-0-lite") || strings.Contains(name, "seedream-5-lite"):
		return "lite"
	case strings.Contains(name, "seedream-5-0-pro") || strings.Contains(name, "seedream-5-pro") || strings.Contains(name, "seedream-5-0"):
		return "pro"
	case strings.Contains(name, "seedream-4-5"):
		return "4.5"
	case strings.Contains(name, "seedream-4-0"):
		return "4.0"
	default:
		return ""
	}
}

func GetAndValidateRequest(c *gin.Context, format types.RelayFormat) (request dto.Request, err error) {
	relayMode := relayconstant.Path2RelayMode(c.Request.URL.Path)

	switch format {
	case types.RelayFormatOpenAI:
		request, err = GetAndValidateTextRequest(c, relayMode)
	case types.RelayFormatGemini:
		if strings.Contains(c.Request.URL.Path, ":embedContent") {
			request, err = GetAndValidateGeminiEmbeddingRequest(c)
		} else if strings.Contains(c.Request.URL.Path, ":batchEmbedContents") {
			request, err = GetAndValidateGeminiBatchEmbeddingRequest(c)
		} else {
			request, err = GetAndValidateGeminiRequest(c)
		}
	case types.RelayFormatClaude:
		request, err = GetAndValidateClaudeRequest(c)
	case types.RelayFormatOpenAIResponses:
		request, err = GetAndValidateResponsesRequest(c)
	case types.RelayFormatOpenAIResponsesCompaction:
		request, err = GetAndValidateResponsesCompactionRequest(c)

	case types.RelayFormatOpenAIImage:
		request, err = GetAndValidOpenAIImageRequest(c, relayMode)
	case types.RelayFormatEmbedding:
		request, err = GetAndValidateEmbeddingRequest(c, relayMode)
	case types.RelayFormatRerank:
		request, err = GetAndValidateRerankRequest(c)
	case types.RelayFormatOpenAIAudio:
		request, err = GetAndValidAudioRequest(c, relayMode)
	case types.RelayFormatOpenAIRealtime:
		request = &dto.BaseRequest{}
	default:
		return nil, fmt.Errorf("unsupported relay format: %s", format)
	}
	return request, err
}

func GetAndValidAudioRequest(c *gin.Context, relayMode int) (*dto.AudioRequest, error) {
	audioRequest := &dto.AudioRequest{}
	err := common.UnmarshalBodyReusable(c, audioRequest)
	if err != nil {
		return nil, err
	}
	switch relayMode {
	case relayconstant.RelayModeAudioSpeech:
		if audioRequest.Model == "" {
			return nil, errors.New("model is required")
		}
	default:
		if audioRequest.Model == "" {
			return nil, errors.New("model is required")
		}
		if audioRequest.ResponseFormat == "" {
			audioRequest.ResponseFormat = "json"
		}
	}
	return audioRequest, nil
}

func GetAndValidateRerankRequest(c *gin.Context) (*dto.RerankRequest, error) {
	var rerankRequest *dto.RerankRequest
	err := common.UnmarshalBodyReusable(c, &rerankRequest)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("getAndValidateTextRequest failed: %s", err.Error()))
		return nil, types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	if rerankRequest.Query == "" {
		return nil, types.NewError(fmt.Errorf("query is empty"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	if len(rerankRequest.Documents) == 0 {
		return nil, types.NewError(fmt.Errorf("documents is empty"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	return rerankRequest, nil
}

func GetAndValidateEmbeddingRequest(c *gin.Context, relayMode int) (*dto.EmbeddingRequest, error) {
	var embeddingRequest *dto.EmbeddingRequest
	err := common.UnmarshalBodyReusable(c, &embeddingRequest)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("getAndValidateTextRequest failed: %s", err.Error()))
		return nil, types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	if embeddingRequest.Input == nil {
		return nil, fmt.Errorf("input is empty")
	}
	if relayMode == relayconstant.RelayModeModerations && embeddingRequest.Model == "" {
		embeddingRequest.Model = "omni-moderation-latest"
	}
	if relayMode == relayconstant.RelayModeEmbeddings && embeddingRequest.Model == "" {
		embeddingRequest.Model = c.Param("model")
	}
	return embeddingRequest, nil
}

// maxTokensLimit bounds user-supplied max token fields. These values feed
// pre-consume quota math (preConsumedTokens * ratio); an unbounded value can
// overflow the conversion and corrupt billing.
const maxTokensLimit = math.MaxInt32 / 2

func exceedsMaxTokensLimit(values ...*uint) bool {
	for _, v := range values {
		if lo.FromPtrOr(v, uint(0)) > maxTokensLimit {
			return true
		}
	}
	return false
}

func GetAndValidateResponsesRequest(c *gin.Context) (*dto.OpenAIResponsesRequest, error) {
	request := &dto.OpenAIResponsesRequest{}
	err := common.UnmarshalBodyReusable(c, request)
	if err != nil {
		return nil, err
	}
	if request.Model == "" {
		return nil, errors.New("model is required")
	}
	if request.Input == nil {
		return nil, errors.New("input is required")
	}
	if exceedsMaxTokensLimit(request.MaxOutputTokens) {
		return nil, errors.New("max_output_tokens is invalid")
	}
	return request, nil
}

func GetAndValidateResponsesCompactionRequest(c *gin.Context) (*dto.OpenAIResponsesCompactionRequest, error) {
	request := &dto.OpenAIResponsesCompactionRequest{}
	if err := common.UnmarshalBodyReusable(c, request); err != nil {
		return nil, err
	}
	if request.Model == "" {
		return nil, errors.New("model is required")
	}
	return request, nil
}

func GetAndValidOpenAIImageRequest(c *gin.Context, relayMode int) (*dto.ImageRequest, error) {
	imageRequest := &dto.ImageRequest{}

	switch relayMode {
	case relayconstant.RelayModeImagesEdits:
		if strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
			form, err := common.ParseMultipartFormReusable(c)
			if err != nil {
				return nil, fmt.Errorf("failed to parse image edit form request: %w", err)
			}
			formData := url.Values(form.Value)
			c.Request.MultipartForm = form
			c.Request.PostForm = formData
			imageRequest.Prompt = formData.Get("prompt")
			imageRequest.Model = formData.Get("model")
			if nValue := strings.TrimSpace(formData.Get("n")); nValue != "" {
				n, err := strconv.Atoi(nValue)
				if err != nil || n < 0 || n > dto.MaxImageN {
					return nil, fmt.Errorf("n must be an integer between 1 and %d", dto.MaxImageN)
				}
				imageRequest.N = common.GetPointer(uint(n))
			}
			imageRequest.Quality = formData.Get("quality")
			imageRequest.Size = formData.Get("size")
			if streamValue := strings.TrimSpace(formData.Get("stream")); streamValue != "" {
				stream, err := strconv.ParseBool(streamValue)
				if err != nil {
					return nil, fmt.Errorf("invalid stream value: %w", err)
				}
				imageRequest.Stream = common.GetPointer(stream)
			}
			if imageValue := formData.Get("image"); imageValue != "" {
				imageRequest.Image, _ = common.Marshal(imageValue)
			}

			if imageRequest.Model == "gpt-image-1" {
				if imageRequest.Quality == "" {
					imageRequest.Quality = "standard"
				}
			}
			if imageRequest.N == nil || *imageRequest.N == 0 {
				imageRequest.N = common.GetPointer(uint(1))
			}

			hasWatermark := formData.Has("watermark")
			if hasWatermark {
				watermark := formData.Get("watermark") == "true"
				imageRequest.Watermark = &watermark
			}
			break
		}
		fallthrough
	default:
		err := common.UnmarshalBodyReusable(c, imageRequest)
		if err != nil {
			return nil, err
		}

		if imageRequest.Model == "" {
			//imageRequest.Model = "dall-e-3"
			return nil, errors.New("model is required")
		}

		if strings.Contains(imageRequest.Size, "×") {
			return nil, errors.New("size an unexpected error occurred in the parameter, please use 'x' instead of the multiplication sign '×'")
		}

		if imageRequest.N != nil && *imageRequest.N > dto.MaxImageN {
			return nil, fmt.Errorf("n must be an integer between 1 and %d", dto.MaxImageN)
		}

		// Not "256x256", "512x512", or "1024x1024"
		if imageRequest.Model == "dall-e-2" || imageRequest.Model == "dall-e" {
			if imageRequest.Size != "" && imageRequest.Size != "256x256" && imageRequest.Size != "512x512" && imageRequest.Size != "1024x1024" {
				return nil, errors.New("size must be one of 256x256, 512x512, or 1024x1024 for dall-e-2 or dall-e")
			}
			if imageRequest.Size == "" {
				imageRequest.Size = "1024x1024"
			}
		} else if imageRequest.Model == "dall-e-3" {
			if imageRequest.Size != "" && imageRequest.Size != "1024x1024" && imageRequest.Size != "1024x1792" && imageRequest.Size != "1792x1024" {
				return nil, errors.New("size must be one of 1024x1024, 1024x1792 or 1792x1024 for dall-e-3")
			}
			if imageRequest.Quality == "" {
				imageRequest.Quality = "standard"
			}
			if imageRequest.Size == "" {
				imageRequest.Size = "1024x1024"
			}
		} else if imageRequest.Model == "gpt-image-1" {
			if imageRequest.Quality == "" {
				imageRequest.Quality = "auto"
			}
		}

		//if imageRequest.Prompt == "" {
		//	return nil, errors.New("prompt is required")
		//}

		if imageRequest.N == nil || *imageRequest.N == 0 {
			imageRequest.N = common.GetPointer(uint(1))
		}
	}

	return imageRequest, nil
}

func GetAndValidateClaudeRequest(c *gin.Context) (textRequest *dto.ClaudeRequest, err error) {
	textRequest = &dto.ClaudeRequest{}
	err = common.UnmarshalBodyReusable(c, textRequest)
	if err != nil {
		return nil, err
	}
	if textRequest.Messages == nil || len(textRequest.Messages) == 0 {
		return nil, errors.New("field messages is required")
	}
	if textRequest.Model == "" {
		return nil, errors.New("field model is required")
	}
	if exceedsMaxTokensLimit(textRequest.MaxTokens, textRequest.MaxTokensToSample) {
		return nil, errors.New("max_tokens is invalid")
	}

	//if textRequest.Stream {
	//	relayInfo.IsStream = true
	//}

	return textRequest, nil
}

func GetAndValidateTextRequest(c *gin.Context, relayMode int) (*dto.GeneralOpenAIRequest, error) {
	textRequest := &dto.GeneralOpenAIRequest{}
	err := common.UnmarshalBodyReusable(c, textRequest)
	if err != nil {
		return nil, err
	}

	if relayMode == relayconstant.RelayModeModerations && textRequest.Model == "" {
		textRequest.Model = "text-moderation-latest"
	}
	if relayMode == relayconstant.RelayModeEmbeddings && textRequest.Model == "" {
		textRequest.Model = c.Param("model")
	}

	if exceedsMaxTokensLimit(textRequest.MaxTokens, textRequest.MaxCompletionTokens) {
		return nil, errors.New("max_tokens is invalid")
	}
	if textRequest.Model == "" {
		return nil, errors.New("model is required")
	}
	if textRequest.WebSearchOptions != nil {
		if textRequest.WebSearchOptions.SearchContextSize != "" {
			validSizes := map[string]bool{
				"high":   true,
				"medium": true,
				"low":    true,
			}
			if !validSizes[textRequest.WebSearchOptions.SearchContextSize] {
				return nil, errors.New("invalid search_context_size, must be one of: high, medium, low")
			}
		} else {
			textRequest.WebSearchOptions.SearchContextSize = "medium"
		}
	}
	switch relayMode {
	case relayconstant.RelayModeCompletions:
		if textRequest.Prompt == "" {
			return nil, errors.New("field prompt is required")
		}
	case relayconstant.RelayModeChatCompletions:
		// For FIM (Fill-in-the-middle) requests with prefix/suffix, messages is optional
		// It will be filled by provider-specific adaptors if needed (e.g., SiliconFlow)。Or it is allowed by model vendor(s) (e.g., DeepSeek)
		if len(textRequest.Messages) == 0 && textRequest.Prefix == nil && textRequest.Suffix == nil {
			return nil, errors.New("field messages is required")
		}
	case relayconstant.RelayModeEmbeddings:
	case relayconstant.RelayModeModerations:
		if textRequest.Input == nil || textRequest.Input == "" {
			return nil, errors.New("field input is required")
		}
	case relayconstant.RelayModeEdits:
		if textRequest.Instruction == "" {
			return nil, errors.New("field instruction is required")
		}
	}
	return textRequest, nil
}

func GetAndValidateGeminiRequest(c *gin.Context) (*dto.GeminiChatRequest, error) {
	request := &dto.GeminiChatRequest{}
	err := common.UnmarshalBodyReusable(c, request)
	if err != nil {
		return nil, err
	}
	if len(request.Contents) == 0 && len(request.Requests) == 0 {
		return nil, errors.New("contents is required")
	}
	if exceedsMaxTokensLimit(request.GenerationConfig.MaxOutputTokens) {
		return nil, errors.New("maxOutputTokens is invalid")
	}

	//if c.Query("alt") == "sse" {
	//	relayInfo.IsStream = true
	//}

	return request, nil
}

func GetAndValidateGeminiEmbeddingRequest(c *gin.Context) (*dto.GeminiEmbeddingRequest, error) {
	request := &dto.GeminiEmbeddingRequest{}
	err := common.UnmarshalBodyReusable(c, request)
	if err != nil {
		return nil, err
	}
	return request, nil
}

func GetAndValidateGeminiBatchEmbeddingRequest(c *gin.Context) (*dto.GeminiBatchEmbeddingRequest, error) {
	request := &dto.GeminiBatchEmbeddingRequest{}
	err := common.UnmarshalBodyReusable(c, request)
	if err != nil {
		return nil, err
	}
	return request, nil
}
