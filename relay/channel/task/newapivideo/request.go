package newapivideo

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func validateOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, body []byte) *dto.TaskError {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return newRequestError("request must use application/json", "unsupported_media_type", http.StatusUnsupportedMediaType)
	}
	if common.GetJsonType(body) != "object" {
		return newRequestError("request body must be a JSON object", "invalid_request", http.StatusBadRequest)
	}

	var fields map[string]json.RawMessage
	if err := common.Unmarshal(body, &fields); err != nil {
		return newRequestError(fmt.Sprintf("invalid JSON body: %v", err), "invalid_json", http.StatusBadRequest)
	}
	if fields == nil {
		return newRequestError("request body must be a JSON object", "invalid_request", http.StatusBadRequest)
	}

	clientModel, err := requiredStringField(fields, "model")
	if err != nil {
		return newRequestError("field model is required", "missing_model", http.StatusBadRequest)
	}
	prompt, err := requiredStringField(fields, "prompt")
	if err != nil {
		return newRequestError("field prompt must be a non-empty string", "invalid_request", http.StatusBadRequest)
	}

	state := requestState{OpenAIFields: fields}
	if raw, exists := fields["duration"]; exists {
		value, err := parseRequestDecimal(raw, false)
		if err != nil || !validDuration(value, false) {
			return newRequestError(fmt.Sprintf("duration must be a number between 1 and %d", relaycommon.MaxTaskDurationSeconds), "invalid_duration", http.StatusBadRequest)
		}
		state.Duration = &value
	}
	if raw, exists := fields["seconds"]; exists {
		value, err := parseRequestDecimal(raw, true)
		if err != nil || !validDuration(value, true) {
			return newRequestError(fmt.Sprintf("seconds must be an integer string between 1 and %d", relaycommon.MaxTaskDurationSeconds), "invalid_seconds", http.StatusBadRequest)
		}
		state.Seconds = &value
	}
	if state.Duration != nil && state.Seconds != nil && !state.Duration.Equal(*state.Seconds) {
		return newRequestError("duration and seconds must have the same value", "invalid_duration", http.StatusBadRequest)
	}

	if raw, exists := fields["n"]; exists {
		value, err := parseRequestDecimal(raw, false)
		if err != nil || !value.Equal(decimal.NewFromInt(1)) {
			return newRequestError("n must be the integer 1", "invalid_n", http.StatusBadRequest)
		}
	}
	if raw, exists := fields["seed"]; exists {
		value, err := parseRequestDecimal(raw, false)
		if err != nil || !value.Equal(value.Truncate(0)) {
			return newRequestError("seed must be an integer", "invalid_seed", http.StatusBadRequest)
		}
	}

	if raw, exists := fields["metadata"]; exists {
		if common.GetJsonType(raw) != "object" {
			return newRequestError("metadata must be a JSON object", "invalid_request", http.StatusBadRequest)
		}
		var metadata map[string]json.RawMessage
		if err := common.Unmarshal(raw, &metadata); err != nil || metadata == nil {
			return newRequestError("metadata must be a JSON object", "invalid_request", http.StatusBadRequest)
		}
		for _, field := range []string{"duration", "seconds", "n"} {
			metadataRaw, metadataExists := metadata[field]
			if !metadataExists {
				continue
			}
			topLevelRaw, topLevelExists := fields[field]
			if !topLevelExists {
				return newRequestError(fmt.Sprintf("metadata.%s requires the same top-level field", field), "invalid_"+field, http.StatusBadRequest)
			}
			metadataValue, metadataErr := parseMetadataDecimal(metadataRaw)
			topLevelValue, topLevelErr := parseMetadataDecimal(topLevelRaw)
			if metadataErr != nil || topLevelErr != nil || !metadataValue.Equal(topLevelValue) {
				return newRequestError(fmt.Sprintf("metadata.%s must match the top-level field", field), "invalid_"+field, http.StatusBadRequest)
			}
		}
	}

	c.Set(requestStateContextKey, state)
	relaycommon.StoreTaskRequest(c, info, constant.TaskActionGenerate, relaycommon.TaskSubmitReq{
		Model:  clientModel,
		Prompt: prompt,
	})
	return nil
}

func buildOpenAIRequestBody(c *gin.Context, upstreamModel string) ([]byte, error) {
	state, err := getRequestState(c)
	if err != nil {
		return nil, err
	}
	fields := make(map[string]json.RawMessage, len(state.OpenAIFields))
	for key, value := range state.OpenAIFields {
		fields[key] = append(json.RawMessage(nil), value...)
	}
	modelJSON, err := common.Marshal(upstreamModel)
	if err != nil {
		return nil, err
	}
	fields["model"] = modelJSON
	return common.Marshal(fields)
}

func requiredStringField(fields map[string]json.RawMessage, name string) (string, error) {
	raw, exists := fields[name]
	if !exists || common.GetJsonType(raw) != "string" {
		return "", fmt.Errorf("field %s is required", name)
	}
	var value string
	if err := common.Unmarshal(raw, &value); err != nil || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("field %s is required", name)
	}
	return value, nil
}

func parseRequestDecimal(raw json.RawMessage, requireString bool) (decimal.Decimal, error) {
	expectedType := "number"
	if requireString {
		expectedType = "string"
	}
	if common.GetJsonType(raw) != expectedType {
		return decimal.Zero, fmt.Errorf("expected JSON %s", expectedType)
	}
	return decimal.NewFromString(common.JsonRawMessageToString(raw))
}

func parseMetadataDecimal(raw json.RawMessage) (decimal.Decimal, error) {
	jsonType := common.GetJsonType(raw)
	if jsonType != "number" && jsonType != "string" {
		return decimal.Zero, fmt.Errorf("expected a number or numeric string")
	}
	return decimal.NewFromString(common.JsonRawMessageToString(raw))
}

func validDuration(value decimal.Decimal, requireInteger bool) bool {
	if value.LessThanOrEqual(decimal.Zero) || value.GreaterThan(decimal.NewFromInt(relaycommon.MaxTaskDurationSeconds)) {
		return false
	}
	return !requireInteger || value.Equal(value.Truncate(0))
}

func newRequestError(message, code string, status int) *dto.TaskError {
	return service.TaskErrorWrapperLocal(fmt.Errorf("%s", message), code, status)
}
