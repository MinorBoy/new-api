package service

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestConfigImportSchemaAcceptsCanonicalDocument(t *testing.T) {
	payload := configImportDocumentJSON(t, map[string]any{})

	document, err := ParseConfigImportDocument(strings.NewReader(payload))

	require.NoError(t, err)
	require.Equal(t, "new-api.channel-config-import", document.Kind)
	require.Equal(t, 1, document.SchemaVersion)
	require.Len(t, document.Entities.Sources, 1)
}

func TestConfigImportSchemaRejectsUnsupportedKind(t *testing.T) {
	payload := strings.Replace(configImportDocumentJSON(t, map[string]any{}), `"new-api.channel-config-import"`, `"other.import"`, 1)

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	requireCode(t, err, "SCHEMA_KIND")
}

func TestConfigImportSchemaRejectsUnsupportedVersion(t *testing.T) {
	payload := strings.Replace(configImportDocumentJSON(t, map[string]any{}), `"schema_version":1`, `"schema_version":2`, 1)

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	requireCode(t, err, "SCHEMA_VERSION")
}

func TestConfigImportSchemaRejectsMalformedSourceHash(t *testing.T) {
	payload := strings.Replace(configImportDocumentJSON(t, map[string]any{}), strings.Repeat("a", 64), "not-a-sha256", 1)

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	requireCode(t, err, "SCHEMA_SOURCE_HASH")
}

func TestConfigImportSchemaRejectsMismatchedPayloadHash(t *testing.T) {
	payload := configImportDocumentJSON(t, map[string]any{})
	firstHashIndex := strings.Index(payload, `"payload_sha256":"`)
	require.NotEqual(t, -1, firstHashIndex)
	hashStart := firstHashIndex + len(`"payload_sha256":"`)
	replacement := "0"
	if payload[hashStart] == '0' {
		replacement = "1"
	}
	payload = payload[:hashStart] + replacement + payload[hashStart+1:]

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	requireCode(t, err, "SCHEMA_PAYLOAD_HASH")
}

func TestConfigImportSchemaLimitsInputBytes(t *testing.T) {
	_, err := ParseConfigImportDocument(strings.NewReader(strings.Repeat(" ", configImportMaxInputBytes+1)))

	requireCode(t, err, "LIMIT_INPUT_BYTES")
}

func TestConfigImportSchemaLimitsNesting(t *testing.T) {
	payload := strings.Repeat("[", configImportMaxNestingDepth+1) + strings.Repeat("]", configImportMaxNestingDepth+1)

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	requireCode(t, err, "LIMIT_NESTING_DEPTH")
}

func TestConfigImportSchemaLimitsAuthoritativeEntities(t *testing.T) {
	channels := make([]any, configImportMaxAuthoritativeEntities+1)
	for i := range channels {
		channels[i] = map[string]any{
			"business_id": fmt.Sprintf("channel-%d", i),
			"entity_hash": strings.Repeat("a", 64),
			"source_ref":  "source-workbook",
		}
	}
	payload := configImportDocumentJSON(t, map[string]any{"channels": channels})

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	requireCode(t, err, "LIMIT_AUTHORITATIVE_ENTITIES")
}

func TestConfigImportSchemaAcceptsAuthoritativeEntityLimit(t *testing.T) {
	channels := make([]any, configImportMaxAuthoritativeEntities-1)
	for i := range channels {
		channels[i] = map[string]any{
			"business_id": fmt.Sprintf("channel-%d", i),
			"entity_hash": strings.Repeat("a", 64),
			"source_ref":  "source-workbook",
		}
	}
	payload := configImportDocumentJSON(t, map[string]any{"channels": channels})

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	require.NoError(t, err)
}

func TestConfigImportSchemaLimitsSourceIssues(t *testing.T) {
	issues := make([]any, configImportMaxSourceIssues+1)
	for i := range issues {
		issues[i] = map[string]any{"code": fmt.Sprintf("issue-%d", i), "severity": "warning", "message": "check"}
	}
	payload := configImportDocumentJSONWithIssues(t, map[string]any{}, issues)

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	requireCode(t, err, "LIMIT_SOURCE_ISSUES")
}

func TestConfigImportSchemaAcceptsSourceIssueLimit(t *testing.T) {
	issues := make([]any, configImportMaxSourceIssues)
	for i := range issues {
		issues[i] = map[string]any{"code": fmt.Sprintf("issue-%d", i), "severity": "warning", "message": "check"}
	}
	payload := configImportDocumentJSONWithIssues(t, map[string]any{}, issues)

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	require.NoError(t, err)
}

func TestConfigImportSchemaLimitsOrdinaryStringLength(t *testing.T) {
	payload := `{"name":"` + strings.Repeat("x", configImportMaxStringBytes+1) + `"}`

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	requireCode(t, err, "LIMIT_STRING_LENGTH")
}

func TestConfigImportSchemaLimitsNoteLength(t *testing.T) {
	payload := `{"audit_note":"` + strings.Repeat("x", configImportMaxNoteBytes+1) + `"}`

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	requireCode(t, err, "LIMIT_NOTE_LENGTH")
}

func TestConfigImportSchemaAcceptsStringAndNoteLimits(t *testing.T) {
	payload := configImportDocumentJSON(t, map[string]any{
		"channels": []any{map[string]any{
			"business_id":  "channel-one",
			"entity_hash":  strings.Repeat("c", 64),
			"source_ref":   "source-workbook",
			"display_name": strings.Repeat("x", configImportMaxStringBytes),
		}},
		"sources": []any{map[string]any{
			"business_id":     "source-workbook",
			"entity_hash":     strings.Repeat("b", 64),
			"source_ref":      "source-workbook",
			"sheet":           "Channels",
			"row":             4,
			"raw_business_id": "source-workbook",
			"audit_note":      strings.Repeat("n", configImportMaxNoteBytes),
			"url":             "https://example.test/template.xlsx",
		}},
	})

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	require.NoError(t, err)
}

func TestConfigImportSchemaRejectsCredentialFieldNames(t *testing.T) {
	for _, fieldName := range []string{
		"apiKey",
		"access_token",
		"auth-token",
		"authorization",
		"cookie",
		"secret",
		"password",
	} {
		t.Run(fieldName, func(t *testing.T) {
			_, err := ParseConfigImportDocument(strings.NewReader(`{"` + fieldName + `":"redacted"}`))

			requireCode(t, err, "SECURITY_CREDENTIAL_FIELD")
		})
	}
}

func TestConfigImportSchemaAllowsTokenBillingFieldNames(t *testing.T) {
	_, err := ParseConfigImportDocument(strings.NewReader(`{"input_tokens":"100"}`))

	require.Error(t, err)
	require.NotContains(t, err.Error(), "SECURITY_CREDENTIAL_FIELD")
}

func TestConfigImportSchemaStripsSourceURLQueryAndFragment(t *testing.T) {
	entities := mergeConfigImportEntities(map[string]any{
		"sources": []any{map[string]any{
			"business_id": "source-workbook",
			"entity_hash": strings.Repeat("b", 64),
			"source_ref":  "source-workbook",
			"url":         "https://example.test/template.xlsx?signature=private#sheet",
		}},
	})
	canonicalEntities := mergeConfigImportEntities(map[string]any{
		"sources": []any{map[string]any{
			"business_id": "source-workbook",
			"entity_hash": strings.Repeat("b", 64),
			"source_ref":  "source-workbook",
			"url":         "https://example.test/template.xlsx",
		}},
	})
	payload := configImportDocumentJSONForCanonicalEntities(t, entities, canonicalEntities)

	document, err := ParseConfigImportDocument(strings.NewReader(payload))

	require.NoError(t, err)
	require.Equal(t, "https://example.test/template.xlsx", document.Entities.Sources[0].URL)
}

func TestConfigImportSchemaRejectsUnsupportedSourceURL(t *testing.T) {
	payload := configImportDocumentJSON(t, map[string]any{
		"sources": []any{map[string]any{
			"business_id": "source-workbook",
			"entity_hash": strings.Repeat("b", 64),
			"source_ref":  "source-workbook",
			"url":         "file:///private/template.xlsx",
		}},
	})

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	requireCode(t, err, "SCHEMA_SOURCE_URL")
}

func TestConfigImportSchemaRejectsDuplicateBusinessID(t *testing.T) {
	payload := configImportDocumentJSON(t, map[string]any{
		"channels": []any{map[string]any{
			"business_id": "source-workbook",
			"entity_hash": strings.Repeat("c", 64),
			"source_ref":  "source-workbook",
		}},
	})

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	requireCode(t, err, "DUPLICATE_BUSINESS_ID")
}

func TestConfigImportSchemaRejectsMissingBusinessReference(t *testing.T) {
	payload := configImportDocumentJSON(t, map[string]any{
		"channel_lines": []any{map[string]any{
			"business_id": "line-one",
			"entity_hash": strings.Repeat("c", 64),
			"source_ref":  "source-workbook",
			"channel_ref": "missing-channel",
		}},
	})

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	requireCode(t, err, "REFERENCE_NOT_FOUND")
}

func TestConfigImportSchemaRejectsNegativeDecimalFact(t *testing.T) {
	payload := configImportDocumentJSON(t, map[string]any{
		"cost_rule_drafts": []any{map[string]any{
			"business_id":      "cost-one",
			"entity_hash":      strings.Repeat("c", 64),
			"source_ref":       "source-workbook",
			"channel_line_ref": "missing-line",
			"model_sku_ref":    "missing-sku",
			"unit_price":       "-1.00",
		}},
	})

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	requireCode(t, err, "SCHEMA_DECIMAL")
}

func TestConfigImportSchemaRejectsCredentialValueUnderAllowedField(t *testing.T) {
	payload := configImportDocumentJSON(t, map[string]any{
		"channels": []any{map[string]any{
			"business_id":  "channel-one",
			"entity_hash":  strings.Repeat("c", 64),
			"source_ref":   "source-workbook",
			"display_name": "Channel one",
			"api_key":      "must-not-be-accepted",
		}},
	})

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	requireCode(t, err, "SECURITY_CREDENTIAL_FIELD")
}

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	require.Error(t, err)
	require.Contains(t, err.Error(), code)
}

func configImportDocumentJSON(t *testing.T, extraEntities map[string]any) string {
	return configImportDocumentJSONForCanonicalEntities(t, mergeConfigImportEntities(extraEntities), nil)
}

func configImportDocumentJSONWithIssues(t *testing.T, extraEntities map[string]any, issues []any) string {
	entities := mergeConfigImportEntities(extraEntities)
	canonical := map[string]any{"entities": entities}
	canonicalJSON, err := common.Marshal(canonical)
	require.NoError(t, err)
	hash := sha256.Sum256(canonicalJSON)

	document := map[string]any{
		"kind":             "new-api.channel-config-import",
		"schema_version":   1,
		"template_version": "2026.07",
		"manifest": map[string]any{
			"source_file":       "channel-cost.xlsx",
			"source_sha256":     strings.Repeat("a", 64),
			"payload_sha256":    fmt.Sprintf("%x", hash),
			"generated_at":      "2026-07-27T00:00:00Z",
			"converter_version": "1.0.0",
			"template_match":    "exact",
			"counts":            map[string]any{},
		},
		"entities":        entities,
		"derived_preview": map[string]any{},
		"issues":          issues,
	}
	encoded, err := common.Marshal(document)
	require.NoError(t, err)
	return string(encoded)
}

func configImportDocumentJSONForCanonicalEntities(t *testing.T, documentEntities, canonicalEntities map[string]any) string {
	if canonicalEntities == nil {
		canonicalEntities = documentEntities
	}
	canonicalJSON, err := common.Marshal(map[string]any{"entities": canonicalEntities})
	require.NoError(t, err)
	hash := sha256.Sum256(canonicalJSON)

	document := map[string]any{
		"kind":             "new-api.channel-config-import",
		"schema_version":   1,
		"template_version": "2026.07",
		"manifest": map[string]any{
			"source_file":       "channel-cost.xlsx",
			"source_sha256":     strings.Repeat("a", 64),
			"payload_sha256":    fmt.Sprintf("%x", hash),
			"generated_at":      "2026-07-27T00:00:00Z",
			"converter_version": "1.0.0",
			"template_match":    "exact",
			"counts":            map[string]any{},
		},
		"entities":        documentEntities,
		"derived_preview": map[string]any{},
		"issues":          []any{},
	}
	encoded, err := common.Marshal(document)
	require.NoError(t, err)
	return string(encoded)
}

func mergeConfigImportEntities(extra map[string]any) map[string]any {
	entities := map[string]any{
		"channels":         []any{},
		"channel_lines":    []any{},
		"model_skus":       []any{},
		"sale_proposals":   []any{},
		"cost_rule_drafts": []any{},
		"model_mappings":   []any{},
		"route_blueprints": []any{},
		"sources": []any{map[string]any{
			"business_id":     "source-workbook",
			"entity_hash":     strings.Repeat("b", 64),
			"source_ref":      "source-workbook",
			"sheet":           "Channels",
			"row":             4,
			"raw_business_id": "source-workbook",
			"audit_note":      "source sheet",
			"url":             "https://example.test/template.xlsx",
		}},
		"unresolved_variants": []any{},
	}
	for key, value := range extra {
		entities[key] = value
	}
	return entities
}
