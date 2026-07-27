package service

import (
	"crypto/sha256"
	"fmt"
	"sort"
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

func TestConfigImportSchemaRejectsLegacySourceFileField(t *testing.T) {
	payload := strings.Replace(configImportDocumentJSON(t, map[string]any{}), `"source_file_name"`, `"source_file"`, 1)

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	requireCode(t, err, "SCHEMA_JSON")
}

func TestConfigImportSchemaRequiresManifestCounts(t *testing.T) {
	payload := configImportDocumentJSON(t, map[string]any{})
	var document map[string]any
	require.NoError(t, common.Unmarshal([]byte(payload), &document))
	delete(document["manifest"].(map[string]any), "counts")
	encoded, err := common.Marshal(document)
	require.NoError(t, err)

	_, err = ParseConfigImportDocument(strings.NewReader(string(encoded)))

	requireCode(t, err, "SCHEMA_MANIFEST_COUNTS")
}

func TestConfigImportSchemaRejectsInvalidManifestCounts(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		count int
		code  string
	}{
		{name: "negative", count: -1, code: "LIMIT_MANIFEST_COUNTS"},
		{name: "mismatch", count: 2, code: "SCHEMA_MANIFEST_COUNTS"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			payload := configImportDocumentJSON(t, map[string]any{})
			var document map[string]any
			require.NoError(t, common.Unmarshal([]byte(payload), &document))
			document["manifest"].(map[string]any)["counts"].(map[string]any)["channels"] = testCase.count
			encoded, err := common.Marshal(document)
			require.NoError(t, err)

			_, err = ParseConfigImportDocument(strings.NewReader(string(encoded)))

			requireCode(t, err, testCase.code)
		})
	}
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
			"business_id":        "line-one",
			"entity_hash":        strings.Repeat("c", 64),
			"source_ref":         "source-workbook",
			"line_ref":           "line-one",
			"channel_ref":        "missing-channel",
			"display_name":       "Line one",
			"provider_type_hint": "openai",
			"region":             "global",
			"protocol":           "openai",
			"status_proposal":    "disabled",
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
			"line_ref":         "missing-line",
			"cost_variant_key": "default",
			"route_target_ref": "missing-target",
			"unit_price":       "-1.00",
		}},
	})

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	requireCode(t, err, "SCHEMA_DECIMAL")
}

func TestConfigImportSchemaRejectsCostDraftWithoutUpstreamModel(t *testing.T) {
	entities := configImportReferenceTupleEntities("line-one", "model-one", "line-one", "model-one", nil, nil)
	entities["cost_rule_drafts"] = []any{map[string]any{
		"business_id": "cost-one", "entity_hash": strings.Repeat("2", 64), "source_ref": "source-workbook",
		"line_ref": "line-one", "cost_variant_key": "default", "route_target_ref": "target-one",
	}}

	_, err := ParseConfigImportDocument(strings.NewReader(configImportDocumentJSON(t, entities)))

	requireCode(t, err, "SCHEMA_COST_UPSTREAM_MODEL")
}

func TestConfigImportSchemaRejectsUnsupportedRouteAppendMode(t *testing.T) {
	entities := configImportReferenceTupleEntities("line-one", "model-one", "line-one", "model-one", nil, nil)
	blueprint := entities["route_blueprints"].([]any)[0].(map[string]any)
	blueprint["merge_mode"] = "append"

	_, err := ParseConfigImportDocument(strings.NewReader(configImportDocumentJSON(t, entities)))

	requireCode(t, err, "SCHEMA_ROUTE_MERGE_MODE")
}

func TestConfigImportSchemaRejectsNonCanonicalCostVariantKeys(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		value string
	}{
		{name: "uppercase cost draft", value: "STANDARD"},
		{name: "surrounding whitespace cost draft", value: " standard "},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			payload := configImportDocumentJSON(t, map[string]any{
				"cost_rule_drafts": []any{map[string]any{
					"business_id": "cost-one", "entity_hash": strings.Repeat("c", 64), "source_ref": "source-workbook",
					"line_ref": "missing-line", "cost_variant_key": testCase.value, "route_target_ref": "missing-target",
				}},
			})

			_, err := ParseConfigImportDocument(strings.NewReader(payload))

			requireCode(t, err, "SCHEMA_COST_VARIANT_KEY")
		})
	}
}

func TestConfigImportSchemaRejectsNonCanonicalRouteTargetCostVariantKey(t *testing.T) {
	for _, value := range []string{"DEFAULT", " default "} {
		t.Run(value, func(t *testing.T) {
			entities := configImportReferenceTupleEntities("line-one", "model-one", "line-one", "model-one", nil, nil)
			target := entities["route_blueprints"].([]any)[0].(map[string]any)["targets"].([]any)[0].(map[string]any)
			target["cost_variant_key"] = value
			payload := configImportDocumentJSON(t, entities)

			_, err := ParseConfigImportDocument(strings.NewReader(payload))

			requireCode(t, err, "SCHEMA_COST_VARIANT_KEY")
		})
	}
}

func TestConfigImportSchemaRequiresRouteTargetRealPersonConstraintWhenLineSetsIt(t *testing.T) {
	lineSupportsRealPerson := true
	entities := configImportReferenceTupleEntities("line-one", "model-one", "line-one", "model-one", &lineSupportsRealPerson, nil)
	payload := configImportDocumentJSON(t, entities)

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	requireCode(t, err, "ROUTING_REAL_PERSON_MISMATCH")
}

func TestConfigImportSchemaRejectsConflictingRouteTargetRealPersonConstraint(t *testing.T) {
	lineSupportsRealPerson := true
	targetSupportsRealPerson := false
	entities := configImportReferenceTupleEntities("line-one", "model-one", "line-one", "model-one", &lineSupportsRealPerson, &targetSupportsRealPerson)
	payload := configImportDocumentJSON(t, entities)

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	requireCode(t, err, "ROUTING_REAL_PERSON_MISMATCH")
}

func TestConfigImportSchemaAcceptsGlobalSKUAcrossDifferentLineMappings(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		targetLine   string
		targetModel  string
		mappingLine  string
		mappingModel string
	}{
		{name: "target line differs from sku", targetLine: "line-two", targetModel: "model-one", mappingLine: "line-one", mappingModel: "model-one"},
		{name: "target model differs from sku", targetLine: "line-one", targetModel: "model-other", mappingLine: "line-one", mappingModel: "model-one"},
		{name: "mapping line differs from sku", targetLine: "line-one", targetModel: "model-one", mappingLine: "line-two", mappingModel: "model-one"},
		{name: "mapping model differs from sku", targetLine: "line-one", targetModel: "model-one", mappingLine: "line-one", mappingModel: "model-other"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			entities := configImportReferenceTupleEntities(testCase.targetLine, testCase.targetModel, testCase.mappingLine, testCase.mappingModel, nil, nil)
			sku := entities["model_skus"].([]any)[0].(map[string]any)
			delete(sku, "line_ref")
			delete(sku, "upstream_model")
			payload := configImportDocumentJSON(t, entities)

			_, err := ParseConfigImportDocument(strings.NewReader(payload))

			require.NoError(t, err)
		})
	}
}

func TestConfigImportSchemaRejectsTamperedEntityHash(t *testing.T) {
	payload := configImportDocumentJSON(t, map[string]any{})
	var document map[string]any
	require.NoError(t, common.Unmarshal([]byte(payload), &document))
	document["entities"].(map[string]any)["sources"].([]any)[0].(map[string]any)["entity_hash"] = strings.Repeat("0", 64)
	encoded, err := common.Marshal(document)
	require.NoError(t, err)

	_, err = ParseConfigImportDocument(strings.NewReader(string(encoded)))

	requireCode(t, err, "SCHEMA_ENTITY_HASH")
}

func TestConfigImportSchemaAcceptsStructuredV2LineCostAndRoute(t *testing.T) {
	payload := configImportDocumentJSON(t, map[string]any{
		"channels": []any{map[string]any{
			"business_id": "channel-main", "entity_hash": strings.Repeat("c", 64), "source_ref": "source-workbook",
		}},
		"channel_lines": []any{map[string]any{
			"business_id": "line-main", "entity_hash": strings.Repeat("d", 64), "source_ref": "source-workbook",
			"line_ref": "line-main", "channel_ref": "channel-main", "display_name": "Primary line",
			"provider_type_hint": "openai", "region": "global", "protocol": "openai", "supports_real_person": true,
			"status_proposal": "disabled", "note": "account capability",
		}},
		"model_skus": []any{map[string]any{
			"business_id": "sku-video", "entity_hash": strings.Repeat("e", 64), "source_ref": "source-workbook",
			"line_ref": "line-main", "upstream_model": "video-v2", "output_resolutions": []any{"720p", "1080p"},
		}},
		"model_mappings": []any{map[string]any{
			"business_id": "mapping-video", "entity_hash": strings.Repeat("f", 64), "source_ref": "source-workbook",
			"canonical_model": "video-v2", "client_model": "video-v2", "line_ref": "line-main",
			"upstream_model": "video-v2", "sku_ref": "sku-video",
		}},
		"route_blueprints": []any{map[string]any{
			"business_id": "route-video", "entity_hash": strings.Repeat("1", 64), "source_ref": "source-workbook",
			"canonical_model": "video-v2", "client_model": "video-v2", "model_mapping_refs": []any{"mapping-video"}, "merge_mode": "merge",
			"targets": []any{map[string]any{
				"route_target_ref": "target-video", "line_ref": "line-main", "upstream_model": "video-v2", "sku_ref": "sku-video",
				"cost_variant_key": "standard", "output_resolutions": []any{"720p", "1080p"}, "duration_values": []any{5, 10},
				"aspect_ratios": []any{"16:9"}, "input_modes": []any{"text_to_video"},
				"reference_minimums":   map[string]any{"images": 0, "videos": 0, "audios": 0},
				"reference_limits":     map[string]any{"images": 1, "videos": 1, "audios": 0},
				"supports_real_person": true, "priority": 10, "enabled": false,
			}},
		}},
		"cost_rule_drafts": []any{map[string]any{
			"business_id": "cost-video", "entity_hash": strings.Repeat("2", 64), "source_ref": "source-workbook",
			"line_ref": "line-main", "upstream_model": "video-v2", "cost_variant_key": "standard", "route_target_ref": "target-video",
			"scenario": "text_to_video", "cost_mode": "per_request", "currency": "CNY", "unit_price": "3.000",
			"billing_multiplier": "1", "purchase_discount_ratio": "1", "recharge_exchange_ratio": "1", "fee_rate": "0", "currency_to_usd_rate": "0.14",
			"normalized_usd_unit_price": "0.420",
		}},
	})

	document, err := ParseConfigImportDocument(strings.NewReader(payload))

	require.NoError(t, err)
	require.Equal(t, "line-main", document.Entities.ChannelLines[0].LineRef)
	require.Equal(t, "standard", document.Entities.CostRuleDrafts[0].CostVariantKey)
	require.Equal(t, "target-video", document.Entities.RouteBlueprints[0].Targets[0].RouteTargetRef)
}

func TestConfigImportSchemaAcceptsGlobalModelSKUReferencedByALineMapping(t *testing.T) {
	entities := configImportReferenceTupleEntities("line-one", "model-one", "line-one", "model-one", nil, nil)
	sku := entities["model_skus"].([]any)[0].(map[string]any)
	delete(sku, "line_ref")
	delete(sku, "upstream_model")

	_, err := ParseConfigImportDocument(strings.NewReader(configImportDocumentJSON(t, entities)))

	require.NoError(t, err)
}

func TestConfigImportSchemaAcceptsUnresolvedVariantWithoutVerifiedLine(t *testing.T) {
	payload := configImportDocumentJSON(t, map[string]any{
		"unresolved_variants": []any{map[string]any{
			"business_id": "supplier/model", "entity_hash": strings.Repeat("d", 64), "source_ref": "source-workbook",
			"line_ref": "", "upstream_model": "model", "reason": "line identity is not verified",
		}},
	})

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	require.NoError(t, err)
}

func TestConfigImportSchemaRejectsChannelLineBaseURL(t *testing.T) {
	payload := configImportDocumentJSON(t, map[string]any{
		"channels": []any{map[string]any{
			"business_id": "channel-main", "entity_hash": strings.Repeat("c", 64), "source_ref": "source-workbook",
		}},
		"channel_lines": []any{map[string]any{
			"business_id": "line-main", "entity_hash": strings.Repeat("d", 64), "source_ref": "source-workbook",
			"line_ref": "line-main", "channel_ref": "channel-main", "status_proposal": "disabled",
			"base_url": "https://runtime.example.test",
		}},
	})

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	requireCode(t, err, "SCHEMA_JSON")
}

func TestConfigImportSchemaCanonicalHashIgnoresEntityOrder(t *testing.T) {
	firstPayload := configImportDocumentJSON(t, map[string]any{
		"channels": []any{
			map[string]any{"business_id": "channel-b", "entity_hash": strings.Repeat("b", 64), "source_ref": "source-workbook"},
			map[string]any{"business_id": "channel-a", "entity_hash": strings.Repeat("a", 64), "source_ref": "source-workbook"},
		},
	})
	secondPayload := configImportDocumentJSON(t, map[string]any{
		"channels": []any{
			map[string]any{"business_id": "channel-a", "entity_hash": strings.Repeat("a", 64), "source_ref": "source-workbook"},
			map[string]any{"business_id": "channel-b", "entity_hash": strings.Repeat("b", 64), "source_ref": "source-workbook"},
		},
	})

	first, err := ParseConfigImportDocument(strings.NewReader(firstPayload))
	require.NoError(t, err)
	second, err := ParseConfigImportDocument(strings.NewReader(secondPayload))
	require.NoError(t, err)

	require.Equal(t, first.Manifest.PayloadSHA256, second.Manifest.PayloadSHA256)
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

func TestConfigImportSchemaRejectsHighConfidenceCredentialValue(t *testing.T) {
	payload := configImportDocumentJSON(t, map[string]any{
		"channels": []any{map[string]any{
			"business_id":  "channel-one",
			"entity_hash":  strings.Repeat("c", 64),
			"source_ref":   "source-workbook",
			"display_name": "sk-abcdefghijklmnopqrstuvwxyz0123456789",
		}},
	})

	_, err := ParseConfigImportDocument(strings.NewReader(payload))

	requireCode(t, err, "SECURITY_CREDENTIAL_VALUE")
}

func configImportReferenceTupleEntities(targetLine, targetModel, mappingLine, mappingModel string, lineSupportsRealPerson, targetSupportsRealPerson *bool) map[string]any {
	lineOne := map[string]any{
		"business_id": "line-one", "entity_hash": strings.Repeat("c", 64), "source_ref": "source-workbook",
		"line_ref": "line-one", "channel_ref": "channel-one", "display_name": "Line one",
		"provider_type_hint": "openai", "region": "global", "protocol": "openai", "status_proposal": "disabled",
	}
	if lineSupportsRealPerson != nil {
		lineOne["supports_real_person"] = *lineSupportsRealPerson
	}
	target := map[string]any{
		"route_target_ref": "target-one", "line_ref": targetLine, "upstream_model": targetModel, "sku_ref": "sku-one",
		"cost_variant_key": "default", "enabled": false,
	}
	if targetSupportsRealPerson != nil {
		target["supports_real_person"] = *targetSupportsRealPerson
	}
	return mergeConfigImportEntities(map[string]any{
		"channels": []any{
			map[string]any{"business_id": "channel-one", "entity_hash": strings.Repeat("a", 64), "source_ref": "source-workbook"},
			map[string]any{"business_id": "channel-two", "entity_hash": strings.Repeat("b", 64), "source_ref": "source-workbook"},
		},
		"channel_lines": []any{
			lineOne,
			map[string]any{
				"business_id": "line-two", "entity_hash": strings.Repeat("d", 64), "source_ref": "source-workbook",
				"line_ref": "line-two", "channel_ref": "channel-two", "display_name": "Line two",
				"provider_type_hint": "openai", "region": "global", "protocol": "openai", "status_proposal": "disabled",
			},
		},
		"model_skus": []any{map[string]any{
			"business_id": "sku-one", "entity_hash": strings.Repeat("e", 64), "source_ref": "source-workbook",
			"line_ref": "line-one", "upstream_model": "model-one",
		}},
		"model_mappings": []any{map[string]any{
			"business_id": "mapping-one", "entity_hash": strings.Repeat("f", 64), "source_ref": "source-workbook",
			"canonical_model": "model-one", "client_model": "model-one", "line_ref": mappingLine,
			"upstream_model": mappingModel, "sku_ref": "sku-one",
		}},
		"route_blueprints": []any{map[string]any{
			"business_id": "route-one", "entity_hash": strings.Repeat("1", 64), "source_ref": "source-workbook",
			"canonical_model": "model-one", "client_model": "model-one", "model_mapping_refs": []any{"mapping-one"},
			"targets": []any{target},
		}},
	})
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
	canonicalEntities := prepareConfigImportEntitiesForHash(t, entities, entities)
	canonical := map[string]any{"entities": canonicalEntities}
	canonicalJSON, err := common.Marshal(canonical)
	require.NoError(t, err)
	hash := sha256.Sum256(canonicalJSON)

	document := map[string]any{
		"kind":             "new-api.channel-config-import",
		"schema_version":   1,
		"template_version": "2026.07",
		"manifest": map[string]any{
			"source_file_name":  "channel-cost.xlsx",
			"source_sha256":     strings.Repeat("a", 64),
			"payload_sha256":    fmt.Sprintf("%x", hash),
			"generated_at":      "2026-07-27T00:00:00Z",
			"converter_version": "1.0.0",
			"template_match":    "exact",
			"counts":            configImportManifestCounts(entities),
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
	canonicalEntities = prepareConfigImportEntitiesForHash(t, documentEntities, canonicalEntities)
	canonicalJSON, err := common.Marshal(map[string]any{"entities": canonicalEntities})
	require.NoError(t, err)
	hash := sha256.Sum256(canonicalJSON)

	document := map[string]any{
		"kind":             "new-api.channel-config-import",
		"schema_version":   1,
		"template_version": "2026.07",
		"manifest": map[string]any{
			"source_file_name":  "channel-cost.xlsx",
			"source_sha256":     strings.Repeat("a", 64),
			"payload_sha256":    fmt.Sprintf("%x", hash),
			"generated_at":      "2026-07-27T00:00:00Z",
			"converter_version": "1.0.0",
			"template_match":    "exact",
			"counts":            configImportManifestCounts(documentEntities),
		},
		"entities":        documentEntities,
		"derived_preview": map[string]any{},
		"issues":          []any{},
	}
	encoded, err := common.Marshal(document)
	require.NoError(t, err)
	return string(encoded)
}

func cloneConfigImportEntitiesForHash(t *testing.T, entities map[string]any) map[string]any {
	t.Helper()
	encoded, err := common.Marshal(entities)
	require.NoError(t, err)
	var cloned map[string]any
	require.NoError(t, common.Unmarshal(encoded, &cloned))
	return cloned
}

func prepareConfigImportEntitiesForHash(t *testing.T, documentEntities, canonicalEntities map[string]any) map[string]any {
	t.Helper()
	prepared := cloneConfigImportEntitiesForHash(t, canonicalEntities)
	canonicalizeConfigImportEntitiesForHash(prepared)
	for _, collection := range []string{
		"channels", "channel_lines", "model_skus", "sale_proposals", "cost_rule_drafts",
		"model_mappings", "route_blueprints", "sources", "unresolved_variants",
	} {
		for _, item := range prepared[collection].([]any) {
			entity := item.(map[string]any)
			entity["entity_hash"] = configImportEntityHashForTest(t, entity)
		}
	}
	for _, collection := range []string{
		"channels", "channel_lines", "model_skus", "sale_proposals", "cost_rule_drafts",
		"model_mappings", "route_blueprints", "sources", "unresolved_variants",
	} {
		canonicalHashes := make(map[string]string)
		for _, item := range prepared[collection].([]any) {
			entity := item.(map[string]any)
			canonicalHashes[entity["business_id"].(string)] = entity["entity_hash"].(string)
		}
		for _, item := range documentEntities[collection].([]any) {
			entity := item.(map[string]any)
			entity["entity_hash"] = canonicalHashes[entity["business_id"].(string)]
		}
	}
	return prepared
}

func configImportEntityHashForTest(t *testing.T, entity map[string]any) string {
	t.Helper()
	canonical := cloneConfigImportEntitiesForHash(t, map[string]any{"entity": entity})["entity"].(map[string]any)
	delete(canonical, "entity_hash")
	canonicalizeConfigImportGenericValueForHash(canonical)
	encoded, err := common.Marshal(canonical)
	require.NoError(t, err)
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", digest)
}

func canonicalizeConfigImportEntitiesForHash(entities map[string]any) {
	for _, collection := range []string{
		"channels", "channel_lines", "model_skus", "sale_proposals", "cost_rule_drafts",
		"model_mappings", "route_blueprints", "sources", "unresolved_variants",
	} {
		items, ok := entities[collection].([]any)
		if !ok {
			continue
		}
		sort.Slice(items, func(left, right int) bool {
			return items[left].(map[string]any)["business_id"].(string) < items[right].(map[string]any)["business_id"].(string)
		})
		if collection == "model_skus" {
			for _, item := range items {
				canonicalizeConfigImportConstraintMap(item.(map[string]any))
			}
		}
	}
	blueprints, ok := entities["route_blueprints"].([]any)
	if !ok {
		return
	}
	for _, item := range blueprints {
		blueprint := item.(map[string]any)
		if references, ok := blueprint["model_mapping_refs"].([]any); ok {
			sort.Slice(references, func(left, right int) bool { return references[left].(string) < references[right].(string) })
		}
		if targets, ok := blueprint["targets"].([]any); ok {
			sort.Slice(targets, func(left, right int) bool {
				return targets[left].(map[string]any)["route_target_ref"].(string) < targets[right].(map[string]any)["route_target_ref"].(string)
			})
			for _, target := range targets {
				canonicalizeConfigImportConstraintMap(target.(map[string]any))
			}
		}
	}
}

func canonicalizeConfigImportConstraintMap(item map[string]any) {
	for _, field := range []string{"output_resolutions", "aspect_ratios", "input_modes"} {
		values, ok := item[field].([]any)
		if !ok {
			continue
		}
		sort.Slice(values, func(left, right int) bool { return values[left].(string) < values[right].(string) })
	}
	values, ok := item["duration_values"].([]any)
	if !ok {
		return
	}
	sort.Slice(values, func(left, right int) bool {
		return configImportDurationForHash(values[left]) < configImportDurationForHash(values[right])
	})
}

func configImportDurationForHash(value any) float64 {
	switch typed := value.(type) {
	case int:
		return float64(typed)
	case float64:
		return typed
	default:
		return 0
	}
}

func canonicalizeConfigImportGenericValueForHash(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			canonicalizeConfigImportGenericValueForHash(child)
		}
	case []any:
		for _, child := range typed {
			canonicalizeConfigImportGenericValueForHash(child)
		}
		if len(typed) < 2 {
			return
		}
		if configImportAllStringsForHash(typed) {
			sort.Slice(typed, func(left, right int) bool { return typed[left].(string) < typed[right].(string) })
			return
		}
		if configImportAllNumbersForHash(typed) {
			sort.Slice(typed, func(left, right int) bool {
				return configImportDurationForHash(typed[left]) < configImportDurationForHash(typed[right])
			})
			return
		}
		if configImportAllObjectsWithKeyForHash(typed, "business_id") {
			sort.Slice(typed, func(left, right int) bool {
				return typed[left].(map[string]any)["business_id"].(string) < typed[right].(map[string]any)["business_id"].(string)
			})
			return
		}
		if configImportAllObjectsWithKeyForHash(typed, "route_target_ref") {
			sort.Slice(typed, func(left, right int) bool {
				return typed[left].(map[string]any)["route_target_ref"].(string) < typed[right].(map[string]any)["route_target_ref"].(string)
			})
		}
	}
}

func configImportAllStringsForHash(values []any) bool {
	for _, value := range values {
		if _, ok := value.(string); !ok {
			return false
		}
	}
	return true
}

func configImportAllNumbersForHash(values []any) bool {
	for _, value := range values {
		switch value.(type) {
		case int, float64:
		default:
			return false
		}
	}
	return true
}

func configImportAllObjectsWithKeyForHash(values []any, key string) bool {
	for _, value := range values {
		object, ok := value.(map[string]any)
		if !ok {
			return false
		}
		if _, ok := object[key].(string); !ok {
			return false
		}
	}
	return true
}

func configImportManifestCounts(entities map[string]any) map[string]any {
	return map[string]any{
		"channels":            len(entities["channels"].([]any)),
		"channel_lines":       len(entities["channel_lines"].([]any)),
		"model_skus":          len(entities["model_skus"].([]any)),
		"sale_proposals":      len(entities["sale_proposals"].([]any)),
		"cost_rule_drafts":    len(entities["cost_rule_drafts"].([]any)),
		"model_mappings":      len(entities["model_mappings"].([]any)),
		"route_blueprints":    len(entities["route_blueprints"].([]any)),
		"sources":             len(entities["sources"].([]any)),
		"unresolved_variants": len(entities["unresolved_variants"].([]any)),
	}
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
