package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/modelrouting"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type routingInputError struct {
	Code    types.ErrorCode
	Message string
}

func (e *routingInputError) Error() string {
	return e.Message
}

func extractSeedanceRoutingInput(c *gin.Context, canonicalModel string) (*modelrouting.FactsInput, *routingInputError) {
	if !c.GetBool(common.KeySeedanceOfficialAPI) || c.Request.Method != http.MethodPost ||
		c.Request.URL.Path != "/v1/video/generations" || !containsRoutingString(modelrouting.CanonicalModels, canonicalModel) {
		return nil, nil
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, newRoutingInputError("InvalidParameter", err.Error())
	}
	body, err := storage.Bytes()
	if err != nil || common.GetJsonType(body) != "object" {
		return nil, newRoutingInputError("InvalidParameter", "request body must be a JSON object")
	}
	input, parseErr := parseSeedanceRoutingFields(body, canonicalModel)
	if parseErr != nil {
		return nil, parseErr
	}
	return &input, nil
}

func parseSeedanceRoutingFields(body []byte, canonicalModel string) (modelrouting.FactsInput, *routingInputError) {
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(body, &fields); err != nil || fields == nil {
		return modelrouting.FactsInput{}, newRoutingInputError("InvalidParameter", "request body contains invalid parameters")
	}
	input := modelrouting.FactsInput{CanonicalModel: canonicalModel}

	if raw, ok := fields["resolution"]; ok {
		value, err := routingStringField(raw, "resolution")
		if err != nil || !containsRoutingString([]string{"480p", "720p", "1080p", "4k"}, value) {
			return modelrouting.FactsInput{}, newRoutingInputError("InvalidParameter.resolution", "resolution is invalid")
		}
		input.OutputResolution = &value
	}
	if raw, ok := fields["duration"]; ok {
		if common.GetJsonType(raw) != "number" {
			return modelrouting.FactsInput{}, newRoutingInputError("InvalidParameter.duration", "duration must be an integer")
		}
		var value int
		if err := common.Unmarshal(raw, &value); err != nil || value == 0 || value < -1 || value > relaycommon.MaxTaskDurationSeconds {
			return modelrouting.FactsInput{}, newRoutingInputError("InvalidParameter.duration", fmt.Sprintf("duration must be -1 or between 1 and %d", relaycommon.MaxTaskDurationSeconds))
		}
		input.DurationSeconds = &value
	}
	if raw, ok := fields["ratio"]; ok {
		value, err := routingStringField(raw, "ratio")
		if err != nil || !containsRoutingString([]string{"16:9", "4:3", "1:1", "3:4", "9:16", "21:9", "adaptive"}, value) {
			return modelrouting.FactsInput{}, newRoutingInputError("InvalidParameter.ratio", "ratio is invalid")
		}
		input.AspectRatio = &value
	}
	if raw, ok := fields["routing"]; ok {
		if common.GetJsonType(raw) != "object" {
			return modelrouting.FactsInput{}, newRoutingInputError("InvalidParameter.routing", "routing must be an object")
		}
		var routingFields map[string]json.RawMessage
		if err := common.Unmarshal(raw, &routingFields); err != nil || routingFields == nil {
			return modelrouting.FactsInput{}, newRoutingInputError("InvalidParameter.routing", "routing must be an object")
		}
		keys := make([]string, 0, len(routingFields))
		for key := range routingFields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if key != "require_real_person" {
				return modelrouting.FactsInput{}, newRoutingInputError(types.ErrorCode("InvalidParameter.routing."+key), "routing field is not supported")
			}
		}
		if requireRaw, ok := routingFields["require_real_person"]; ok {
			if common.GetJsonType(requireRaw) != "boolean" || common.Unmarshal(requireRaw, &input.RequireRealPerson) != nil {
				return modelrouting.FactsInput{}, newRoutingInputError("InvalidParameter.routing.require_real_person", "require_real_person must be a boolean")
			}
		}
	}

	contentRaw, ok := fields["content"]
	if !ok || common.GetJsonType(contentRaw) != "array" {
		return modelrouting.FactsInput{}, newRoutingInputError("InvalidParameter.content", "content must be an array")
	}
	contentFacts, contentErr := extractSeedanceContentFacts(contentRaw)
	if contentErr != nil {
		return modelrouting.FactsInput{}, contentErr
	}
	if input.AspectRatio != nil && *input.AspectRatio == "adaptive" && (contentFacts.videos > 0 || contentFacts.audios > 0) {
		return modelrouting.FactsInput{}, newRoutingInputError("InvalidParameter.ratio", "adaptive ratio does not support video or audio input")
	}
	input.ReferenceImages = contentFacts.images
	input.ReferenceVideos = contentFacts.videos
	input.ReferenceAudios = contentFacts.audios
	return input, nil
}

type seedanceRoutingContentFacts struct {
	images          int
	videos          int
	audios          int
	texts           int
	firstFrames     int
	lastFrames      int
	referenceImages int
	firstIndex      int
	lastIndex       int
}

func extractSeedanceContentFacts(raw json.RawMessage) (seedanceRoutingContentFacts, *routingInputError) {
	var items []map[string]json.RawMessage
	if err := common.Unmarshal(raw, &items); err != nil || len(items) == 0 {
		return seedanceRoutingContentFacts{}, newRoutingInputError("InvalidParameter.content", "content must be a non-empty array")
	}
	facts := seedanceRoutingContentFacts{firstIndex: -1, lastIndex: -1}
	for index, item := range items {
		typeRaw, ok := item["type"]
		if !ok || common.GetJsonType(typeRaw) != "string" {
			return facts, newRoutingInputError("InvalidParameter.content", "content type is required")
		}
		var contentType string
		if err := common.Unmarshal(typeRaw, &contentType); err != nil {
			return facts, newRoutingInputError("InvalidParameter.content", "content type is invalid")
		}
		switch strings.TrimSpace(contentType) {
		case "text":
			textRaw, ok := item["text"]
			if !ok || common.GetJsonType(textRaw) != "string" {
				return facts, newRoutingInputError("InvalidParameter.content", "text content is invalid")
			}
			var text string
			if common.Unmarshal(textRaw, &text) != nil || strings.TrimSpace(text) == "" {
				return facts, newRoutingInputError("InvalidParameter.content", "text content is invalid")
			}
			facts.texts++
		case "image_url":
			if err := validateRoutingMedia(item["image_url"]); err != nil {
				return facts, err
			}
			role, err := optionalRoutingRole(item["role"])
			if err != nil {
				return facts, err
			}
			facts.images++
			switch role {
			case "", "first_frame":
				facts.firstFrames++
				if facts.firstIndex == -1 {
					facts.firstIndex = index
				}
			case "last_frame":
				facts.lastFrames++
				facts.lastIndex = index
			case "reference_image":
				facts.referenceImages++
			default:
				return facts, newRoutingInputError("InvalidParameter.content", "unsupported image role")
			}
		case "video_url":
			if err := validateRoutingMedia(item["video_url"]); err != nil {
				return facts, err
			}
			role, err := optionalRoutingRole(item["role"])
			if err != nil || role != "reference_video" {
				return facts, newRoutingInputError("InvalidParameter.content", "video role must be reference_video")
			}
			facts.videos++
		case "audio_url":
			if err := validateRoutingMedia(item["audio_url"]); err != nil {
				return facts, err
			}
			role, err := optionalRoutingRole(item["role"])
			if err != nil || role != "reference_audio" {
				return facts, newRoutingInputError("InvalidParameter.content", "audio role must be reference_audio")
			}
			facts.audios++
		default:
			return facts, newRoutingInputError("InvalidParameter.content", "unsupported content type")
		}
	}
	if facts.texts != 1 {
		return facts, newRoutingInputError("InvalidParameter.content", "exactly one non-empty text item is required")
	}
	if facts.images > 9 || facts.videos > 3 || facts.audios > 3 {
		return facts, newRoutingInputError("InvalidParameter.content", "reference media count exceeds Seedance 2.0 limits")
	}
	if facts.audios > 0 && facts.images == 0 && facts.videos == 0 {
		return facts, newRoutingInputError("InvalidParameter.content", "audio input requires an image or video")
	}
	if (facts.firstFrames > 0 || facts.lastFrames > 0) && (facts.referenceImages > 0 || facts.videos > 0 || facts.audios > 0) {
		return facts, newRoutingInputError("InvalidParameter.content", "first/last frame content cannot mix with reference media")
	}
	if facts.firstFrames > 1 || facts.lastFrames > 1 || (facts.lastFrames > 0 && (facts.firstFrames != 1 || facts.lastIndex < facts.firstIndex)) {
		return facts, newRoutingInputError("InvalidParameter.content", "first/last frames are invalid")
	}
	return facts, nil
}

func routingStringField(raw json.RawMessage, field string) (string, error) {
	if common.GetJsonType(raw) != "string" {
		return "", fmt.Errorf("%s must be a string", field)
	}
	var value string
	if err := common.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", fmt.Errorf("%s cannot be empty", field)
	}
	return value, nil
}

func optionalRoutingRole(raw json.RawMessage) (string, *routingInputError) {
	if len(raw) == 0 {
		return "", nil
	}
	if common.GetJsonType(raw) != "string" {
		return "", newRoutingInputError("InvalidParameter.content", "content role must be a string")
	}
	var role string
	if err := common.Unmarshal(raw, &role); err != nil {
		return "", newRoutingInputError("InvalidParameter.content", "content role is invalid")
	}
	return strings.TrimSpace(role), nil
}

func validateRoutingMedia(raw json.RawMessage) *routingInputError {
	if common.GetJsonType(raw) != "object" {
		return newRoutingInputError("InvalidParameter.content", "media URL must be an object")
	}
	var media map[string]json.RawMessage
	if err := common.Unmarshal(raw, &media); err != nil {
		return newRoutingInputError("InvalidParameter.content", "media URL is invalid")
	}
	urlRaw, ok := media["url"]
	if !ok || common.GetJsonType(urlRaw) != "string" {
		return newRoutingInputError("InvalidParameter.content", "media URL is required")
	}
	var value string
	if common.Unmarshal(urlRaw, &value) != nil {
		return newRoutingInputError("InvalidParameter.content", "media URL is invalid")
	}
	parsed, err := url.ParseRequestURI(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || parsed.Scheme != "http" && parsed.Scheme != "https" {
		return newRoutingInputError("InvalidParameter.content", "media URL is invalid")
	}
	return nil
}

func containsRoutingString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func newRoutingInputError(code types.ErrorCode, message string) *routingInputError {
	return &routingInputError{Code: code, Message: message}
}
