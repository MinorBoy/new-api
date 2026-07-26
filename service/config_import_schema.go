package service

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
)

const (
	configImportDocumentKind             = "new-api.channel-config-import"
	configImportSchemaVersion            = 1
	configImportMaxInputBytes            = 10 * 1024 * 1024
	configImportMaxNestingDepth          = 32
	configImportMaxAuthoritativeEntities = 5000
	configImportMaxSourceIssues          = 10000
	configImportMaxStringBytes           = 4 * 1024
	configImportMaxNoteBytes             = 2 * 1024
)

var (
	configImportHashPattern            = regexp.MustCompile(`^[a-f0-9]{64}$`)
	configImportCredentialFieldPattern = regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|auth[_-]?token|authorization|cookie|secret|password)`)
)

// ConfigImportSchemaError is stable enough for API handlers to map into a
// validation response without inspecting an implementation-specific message.
type ConfigImportSchemaError struct {
	Code    string
	Message string
}

func (e *ConfigImportSchemaError) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

func configImportError(code string, format string, args ...any) error {
	return &ConfigImportSchemaError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// ParseConfigImportDocument reads and validates a complete, standalone channel
// configuration import document. The artifact is intentionally credential-free
// and uses only business identifiers; no database state is consulted here.
func ParseConfigImportDocument(reader io.Reader) (*types.ConfigImportDocument, error) {
	if reader == nil {
		return nil, configImportError("SCHEMA_READER", "reader is required")
	}

	body, err := io.ReadAll(io.LimitReader(reader, configImportMaxInputBytes+1))
	if err != nil {
		return nil, configImportError("SCHEMA_READ", "read document: %v", err)
	}
	if len(body) > configImportMaxInputBytes {
		return nil, configImportError("LIMIT_INPUT_BYTES", "document exceeds %d bytes", configImportMaxInputBytes)
	}
	if err := scanConfigImportJSON(body); err != nil {
		return nil, err
	}

	var document types.ConfigImportDocument
	if err := common.DecodeJsonStrict(bytes.NewReader(body), &document); err != nil {
		return nil, configImportError("SCHEMA_JSON", "invalid document: %v", err)
	}
	if err := validateConfigImportDocument(&document); err != nil {
		return nil, err
	}
	return &document, nil
}

func scanConfigImportJSON(body []byte) error {
	depth := 0
	for index := 0; index < len(body); {
		switch body[index] {
		case ' ', '\n', '\r', '\t', ',', ':':
			index++
		case '{', '[':
			depth++
			if depth > configImportMaxNestingDepth {
				return configImportError("LIMIT_NESTING_DEPTH", "document exceeds nesting depth %d", configImportMaxNestingDepth)
			}
			index++
		case '}', ']':
			depth--
			if depth < 0 {
				return configImportError("SCHEMA_JSON", "unexpected closing delimiter")
			}
			index++
		case '"':
			end, _, err := scanConfigImportJSONString(body, index)
			if err != nil {
				return configImportError("SCHEMA_JSON", "invalid JSON string: %v", err)
			}
			index = end
		default:
			index++
		}
	}
	if depth != 0 {
		return configImportError("SCHEMA_JSON", "unclosed JSON delimiter")
	}

	var value any
	if err := common.Unmarshal(body, &value); err != nil {
		return configImportError("SCHEMA_JSON", "invalid document: %v", err)
	}
	return validateConfigImportGenericValue(value, "")
}

func scanConfigImportJSONString(body []byte, start int) (int, string, error) {
	escaped := false
	for index := start + 1; index < len(body); index++ {
		switch body[index] {
		case '\\':
			escaped = !escaped
		case '"':
			if escaped {
				escaped = false
				continue
			}
			value, err := strconv.Unquote(string(body[start : index+1]))
			if err != nil {
				return 0, "", err
			}
			return index + 1, value, nil
		default:
			escaped = false
		}
	}
	return 0, "", io.ErrUnexpectedEOF
}

func isConfigImportNoteField(fieldName string) bool {
	return strings.Contains(strings.ToLower(fieldName), "note")
}

func isConfigImportDatabaseIDField(fieldName string) bool {
	normalized := strings.ToLower(fieldName)
	if normalized == "business_id" || normalized == "raw_business_id" {
		return false
	}
	return normalized == "id" || strings.HasSuffix(normalized, "_id")
}

func validateConfigImportGenericValue(value any, fieldName string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if len(key) > configImportMaxStringBytes {
				return configImportError("LIMIT_STRING_LENGTH", "field name exceeds %d bytes", configImportMaxStringBytes)
			}
			if configImportCredentialFieldPattern.MatchString(key) {
				return configImportError("SECURITY_CREDENTIAL_FIELD", "credential field %q is not allowed", key)
			}
			if isConfigImportDatabaseIDField(key) {
				return configImportError("SECURITY_DATABASE_ID", "database id field %q is not allowed", key)
			}
			if err := validateConfigImportGenericValue(child, key); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := validateConfigImportGenericValue(child, fieldName); err != nil {
				return err
			}
		}
	case string:
		maxBytes := configImportMaxStringBytes
		code := "LIMIT_STRING_LENGTH"
		if isConfigImportNoteField(fieldName) {
			maxBytes = configImportMaxNoteBytes
			code = "LIMIT_NOTE_LENGTH"
		}
		if len(typed) > maxBytes {
			return configImportError(code, "field %q exceeds %d bytes", fieldName, maxBytes)
		}
	}
	return nil
}

func validateConfigImportDocument(document *types.ConfigImportDocument) error {
	if document.Kind != configImportDocumentKind {
		return configImportError("SCHEMA_KIND", "kind must be %q", configImportDocumentKind)
	}
	if document.SchemaVersion != configImportSchemaVersion {
		return configImportError("SCHEMA_VERSION", "schema_version must be %d", configImportSchemaVersion)
	}
	if strings.TrimSpace(document.TemplateVersion) == "" {
		return configImportError("SCHEMA_TEMPLATE_VERSION", "template_version is required")
	}
	if err := validateConfigImportManifest(document.Manifest); err != nil {
		return err
	}
	if document.DerivedPreview == nil {
		return configImportError("SCHEMA_DERIVED_PREVIEW", "derived_preview is required")
	}
	if document.Issues == nil {
		return configImportError("SCHEMA_ISSUES", "issues is required")
	}
	if len(document.Issues) > configImportMaxSourceIssues {
		return configImportError("LIMIT_SOURCE_ISSUES", "issues exceeds %d entries", configImportMaxSourceIssues)
	}
	if err := validateConfigImportEntities(&document.Entities); err != nil {
		return err
	}
	if err := validateConfigImportIssues(document.Issues, configImportBusinessIDs(document.Entities)); err != nil {
		return err
	}

	actualPayloadSHA256, err := configImportPayloadSHA256(document.Entities)
	if err != nil {
		return configImportError("SCHEMA_PAYLOAD", "canonical payload: %v", err)
	}
	if document.Manifest.PayloadSHA256 != actualPayloadSHA256 {
		return configImportError("SCHEMA_PAYLOAD_HASH", "payload_sha256 does not match authoritative entities")
	}
	return nil
}

func validateConfigImportManifest(manifest types.ConfigImportManifest) error {
	if strings.TrimSpace(manifest.SourceFile) == "" {
		return configImportError("SCHEMA_MANIFEST_SOURCE_FILE", "manifest.source_file is required")
	}
	if strings.ContainsAny(manifest.SourceFile, `/\\`) {
		return configImportError("SCHEMA_MANIFEST_SOURCE_FILE", "manifest.source_file must not include a path")
	}
	if !configImportHashPattern.MatchString(manifest.SourceSHA256) {
		return configImportError("SCHEMA_SOURCE_HASH", "manifest.source_sha256 must be a lowercase SHA-256 hex digest")
	}
	if !configImportHashPattern.MatchString(manifest.PayloadSHA256) {
		return configImportError("SCHEMA_PAYLOAD_HASH", "manifest.payload_sha256 must be a lowercase SHA-256 hex digest")
	}
	if strings.TrimSpace(manifest.ConverterVersion) == "" {
		return configImportError("SCHEMA_MANIFEST_CONVERTER", "manifest.converter_version is required")
	}
	if strings.TrimSpace(manifest.TemplateMatch) == "" {
		return configImportError("SCHEMA_MANIFEST_TEMPLATE_MATCH", "manifest.template_match is required")
	}
	if _, err := time.Parse(time.RFC3339, manifest.GeneratedAt); err != nil {
		return configImportError("SCHEMA_MANIFEST_GENERATED", "manifest.generated_at must be RFC3339: %v", err)
	}
	return nil
}

func validateConfigImportEntities(entities *types.ConfigImportEntities) error {
	if entities.Channels == nil ||
		entities.ChannelLines == nil ||
		entities.ModelSKUs == nil ||
		entities.SaleProposals == nil ||
		entities.CostRuleDrafts == nil ||
		entities.ModelMappings == nil ||
		entities.RouteBlueprints == nil ||
		entities.Sources == nil ||
		entities.UnresolvedVariants == nil {
		return configImportError("SCHEMA_ENTITIES", "all entity collections are required")
	}

	count := len(entities.Channels) + len(entities.ChannelLines) + len(entities.ModelSKUs) +
		len(entities.SaleProposals) + len(entities.CostRuleDrafts) + len(entities.ModelMappings) +
		len(entities.RouteBlueprints) + len(entities.Sources) + len(entities.UnresolvedVariants)
	if count > configImportMaxAuthoritativeEntities {
		return configImportError("LIMIT_AUTHORITATIVE_ENTITIES", "entities exceeds %d entries", configImportMaxAuthoritativeEntities)
	}

	businessIDs := make(map[string]string, count)
	sourceIDs := make(map[string]struct{}, len(entities.Sources))
	for index := range entities.Sources {
		source := &entities.Sources[index]
		if err := validateConfigImportAuthoritativeEntity(&source.ConfigImportAuthoritativeEntity, "sources", index, businessIDs); err != nil {
			return err
		}
		if source.SourceRef != source.BusinessID {
			return configImportError("REFERENCE_SOURCE", "sources[%d].source_ref must equal its business_id", index)
		}
		if err := normalizeConfigImportSourceURL(source); err != nil {
			return err
		}
		sourceIDs[source.BusinessID] = struct{}{}
	}

	for index := range entities.Channels {
		if err := validateConfigImportAuthoritativeEntity(&entities.Channels[index].ConfigImportAuthoritativeEntity, "channels", index, businessIDs); err != nil {
			return err
		}
	}
	for index := range entities.ChannelLines {
		line := &entities.ChannelLines[index]
		if err := validateConfigImportAuthoritativeEntity(&line.ConfigImportAuthoritativeEntity, "channel_lines", index, businessIDs); err != nil {
			return err
		}
		if err := validateConfigImportDecimal("channel_lines.weight", line.Weight); err != nil {
			return err
		}
	}
	for index := range entities.ModelSKUs {
		if err := validateConfigImportAuthoritativeEntity(&entities.ModelSKUs[index].ConfigImportAuthoritativeEntity, "model_skus", index, businessIDs); err != nil {
			return err
		}
	}
	for index := range entities.SaleProposals {
		proposal := &entities.SaleProposals[index]
		if err := validateConfigImportAuthoritativeEntity(&proposal.ConfigImportAuthoritativeEntity, "sale_proposals", index, businessIDs); err != nil {
			return err
		}
		for field, value := range map[string]*string{
			"unit_price": proposal.UnitPrice, "price_per_unit": proposal.PricePerUnit, "margin_ratio": proposal.MarginRatio,
		} {
			if err := validateConfigImportDecimal("sale_proposals."+field, value); err != nil {
				return err
			}
		}
	}
	for index := range entities.CostRuleDrafts {
		draft := &entities.CostRuleDrafts[index]
		if err := validateConfigImportAuthoritativeEntity(&draft.ConfigImportAuthoritativeEntity, "cost_rule_drafts", index, businessIDs); err != nil {
			return err
		}
		for field, value := range map[string]*string{
			"unit_price": draft.UnitPrice, "price_per_second": draft.PricePerSecond, "input_per_million": draft.InputPerMillion,
			"output_per_million": draft.OutputPerMillion, "completion_per_million": draft.CompletionPerMillion,
			"total_per_million": draft.TotalPerMillion, "billing_multiplier": draft.BillingMultiplier, "fee_rate": draft.FeeRate,
		} {
			if err := validateConfigImportDecimal("cost_rule_drafts."+field, value); err != nil {
				return err
			}
		}
	}
	for index := range entities.ModelMappings {
		if err := validateConfigImportAuthoritativeEntity(&entities.ModelMappings[index].ConfigImportAuthoritativeEntity, "model_mappings", index, businessIDs); err != nil {
			return err
		}
	}
	for index := range entities.RouteBlueprints {
		blueprint := &entities.RouteBlueprints[index]
		if err := validateConfigImportAuthoritativeEntity(&blueprint.ConfigImportAuthoritativeEntity, "route_blueprints", index, businessIDs); err != nil {
			return err
		}
		if blueprint.MergeMode != "" && blueprint.MergeMode != types.ConfigImportRouteMergeModeReplace &&
			blueprint.MergeMode != types.ConfigImportRouteMergeModeAppend && blueprint.MergeMode != types.ConfigImportRouteMergeModeMerge {
			return configImportError("SCHEMA_ROUTE_MERGE_MODE", "route_blueprints[%d].merge_mode is invalid", index)
		}
		for targetIndex := range blueprint.Targets {
			if err := validateConfigImportDecimal("route_blueprints.targets.weight", blueprint.Targets[targetIndex].Weight); err != nil {
				return err
			}
		}
	}
	for index := range entities.UnresolvedVariants {
		if err := validateConfigImportAuthoritativeEntity(&entities.UnresolvedVariants[index].ConfigImportAuthoritativeEntity, "unresolved_variants", index, businessIDs); err != nil {
			return err
		}
	}

	if err := validateConfigImportEntityReferences(entities, sourceIDs, businessIDs); err != nil {
		return err
	}
	return nil
}

func validateConfigImportAuthoritativeEntity(entity *types.ConfigImportAuthoritativeEntity, collection string, index int, businessIDs map[string]string) error {
	if strings.TrimSpace(entity.BusinessID) == "" {
		return configImportError("SCHEMA_BUSINESS_ID", "%s[%d].business_id is required", collection, index)
	}
	if !configImportHashPattern.MatchString(entity.EntityHash) {
		return configImportError("SCHEMA_ENTITY_HASH", "%s[%d].entity_hash must be a lowercase SHA-256 hex digest", collection, index)
	}
	if strings.TrimSpace(entity.SourceRef) == "" {
		return configImportError("REFERENCE_SOURCE", "%s[%d].source_ref is required", collection, index)
	}
	if _, exists := businessIDs[entity.BusinessID]; exists {
		return configImportError("DUPLICATE_BUSINESS_ID", "%s[%d].business_id duplicates %q", collection, index, entity.BusinessID)
	}
	businessIDs[entity.BusinessID] = collection
	return nil
}

func normalizeConfigImportSourceURL(source *types.ConfigImportSource) error {
	if source.URL == "" {
		return nil
	}
	parsed, err := url.Parse(source.URL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Path == "" {
		return configImportError("SCHEMA_SOURCE_URL", "sources url must include scheme, host, and path")
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return configImportError("SCHEMA_SOURCE_URL", "sources url scheme must be http or https")
	}
	if parsed.User != nil {
		return configImportError("SECURITY_SOURCE_URL", "sources url must not include user information")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	source.URL = parsed.String()
	return nil
}

func validateConfigImportDecimal(field string, value *string) error {
	if value == nil {
		return nil
	}
	parsed, err := decimal.NewFromString(strings.TrimSpace(*value))
	if err != nil || parsed.IsNegative() {
		return configImportError("SCHEMA_DECIMAL", "%s must be a non-negative finite decimal string", field)
	}
	return nil
}

func validateConfigImportEntityReferences(entities *types.ConfigImportEntities, sourceIDs map[string]struct{}, businessIDs map[string]string) error {
	for index := range entities.Channels {
		if err := requireConfigImportSourceReference("channels", index, entities.Channels[index].SourceRef, sourceIDs); err != nil {
			return err
		}
	}
	for index := range entities.ChannelLines {
		line := entities.ChannelLines[index]
		if err := requireConfigImportSourceReference("channel_lines", index, line.SourceRef, sourceIDs); err != nil {
			return err
		}
		if err := requireConfigImportReference("channel_lines", index, "channel_ref", line.ChannelRef, businessIDs, "channels"); err != nil {
			return err
		}
	}
	for index := range entities.ModelSKUs {
		modelSKU := entities.ModelSKUs[index]
		if err := requireConfigImportSourceReference("model_skus", index, modelSKU.SourceRef, sourceIDs); err != nil {
			return err
		}
		if err := requireConfigImportReference("model_skus", index, "channel_line_ref", modelSKU.ChannelLineRef, businessIDs, "channel_lines"); err != nil {
			return err
		}
	}
	for index := range entities.SaleProposals {
		proposal := entities.SaleProposals[index]
		if err := requireConfigImportSourceReference("sale_proposals", index, proposal.SourceRef, sourceIDs); err != nil {
			return err
		}
		if err := requireConfigImportReference("sale_proposals", index, "model_sku_ref", proposal.ModelSKURef, businessIDs, "model_skus"); err != nil {
			return err
		}
	}
	for index := range entities.CostRuleDrafts {
		draft := entities.CostRuleDrafts[index]
		if err := requireConfigImportSourceReference("cost_rule_drafts", index, draft.SourceRef, sourceIDs); err != nil {
			return err
		}
		if err := requireConfigImportReference("cost_rule_drafts", index, "channel_line_ref", draft.ChannelLineRef, businessIDs, "channel_lines"); err != nil {
			return err
		}
		if err := requireConfigImportReference("cost_rule_drafts", index, "model_sku_ref", draft.ModelSKURef, businessIDs, "model_skus"); err != nil {
			return err
		}
	}
	for index := range entities.ModelMappings {
		mapping := entities.ModelMappings[index]
		if err := requireConfigImportSourceReference("model_mappings", index, mapping.SourceRef, sourceIDs); err != nil {
			return err
		}
		if err := requireConfigImportReference("model_mappings", index, "channel_ref", mapping.ChannelRef, businessIDs, "channels"); err != nil {
			return err
		}
		if err := requireConfigImportReference("model_mappings", index, "model_sku_ref", mapping.ModelSKURef, businessIDs, "model_skus"); err != nil {
			return err
		}
	}
	for index := range entities.RouteBlueprints {
		blueprint := entities.RouteBlueprints[index]
		if err := requireConfigImportSourceReference("route_blueprints", index, blueprint.SourceRef, sourceIDs); err != nil {
			return err
		}
		for targetIndex := range blueprint.Targets {
			target := blueprint.Targets[targetIndex]
			if err := requireConfigImportReference("route_blueprints", index, "targets.channel_line_ref", target.ChannelLineRef, businessIDs, "channel_lines"); err != nil {
				return err
			}
			if err := requireConfigImportReference("route_blueprints", index, "targets.model_sku_ref", target.ModelSKURef, businessIDs, "model_skus"); err != nil {
				return err
			}
		}
	}
	for index := range entities.UnresolvedVariants {
		variant := entities.UnresolvedVariants[index]
		if err := requireConfigImportSourceReference("unresolved_variants", index, variant.SourceRef, sourceIDs); err != nil {
			return err
		}
		if err := requireConfigImportReference("unresolved_variants", index, "channel_ref", variant.ChannelRef, businessIDs, "channels"); err != nil {
			return err
		}
		if variant.ModelSKURef != "" {
			if err := requireConfigImportReference("unresolved_variants", index, "model_sku_ref", variant.ModelSKURef, businessIDs, "model_skus"); err != nil {
				return err
			}
		}
	}
	return nil
}

func requireConfigImportSourceReference(collection string, index int, sourceRef string, sourceIDs map[string]struct{}) error {
	if _, exists := sourceIDs[sourceRef]; !exists {
		return configImportError("REFERENCE_SOURCE", "%s[%d].source_ref %q does not exist", collection, index, sourceRef)
	}
	return nil
}

func requireConfigImportReference(collection string, index int, field string, reference string, businessIDs map[string]string, expectedCollection string) error {
	actualCollection, exists := businessIDs[reference]
	if !exists {
		return configImportError("REFERENCE_NOT_FOUND", "%s[%d].%s %q does not exist", collection, index, field, reference)
	}
	if actualCollection != expectedCollection {
		return configImportError("REFERENCE_TYPE", "%s[%d].%s %q must reference %s", collection, index, field, reference, expectedCollection)
	}
	return nil
}

func validateConfigImportIssues(issues []types.ConfigImportSourceIssue, businessIDs map[string]string) error {
	for index := range issues {
		issue := issues[index]
		if strings.TrimSpace(issue.Code) == "" || strings.TrimSpace(issue.Message) == "" {
			return configImportError("SCHEMA_ISSUE", "issues[%d] requires code and message", index)
		}
		if issue.Severity != types.ConfigImportIssueSeverityInfo && issue.Severity != types.ConfigImportIssueSeverityWarning && issue.Severity != types.ConfigImportIssueSeverityError {
			return configImportError("SCHEMA_ISSUE_SEVERITY", "issues[%d].severity is invalid", index)
		}
		if issue.EntityRef != "" {
			if _, exists := businessIDs[issue.EntityRef]; !exists {
				return configImportError("REFERENCE_NOT_FOUND", "issues[%d].entity_ref %q does not exist", index, issue.EntityRef)
			}
		}
	}
	return nil
}

func configImportBusinessIDs(entities types.ConfigImportEntities) map[string]string {
	all := make(map[string]string, len(entities.Channels)+len(entities.ChannelLines)+len(entities.ModelSKUs)+len(entities.SaleProposals)+len(entities.CostRuleDrafts)+len(entities.ModelMappings)+len(entities.RouteBlueprints)+len(entities.Sources)+len(entities.UnresolvedVariants))
	for _, source := range entities.Sources {
		all[source.BusinessID] = "sources"
	}
	for _, channel := range entities.Channels {
		all[channel.BusinessID] = "channels"
	}
	for _, line := range entities.ChannelLines {
		all[line.BusinessID] = "channel_lines"
	}
	for _, modelSKU := range entities.ModelSKUs {
		all[modelSKU.BusinessID] = "model_skus"
	}
	for _, proposal := range entities.SaleProposals {
		all[proposal.BusinessID] = "sale_proposals"
	}
	for _, draft := range entities.CostRuleDrafts {
		all[draft.BusinessID] = "cost_rule_drafts"
	}
	for _, mapping := range entities.ModelMappings {
		all[mapping.BusinessID] = "model_mappings"
	}
	for _, blueprint := range entities.RouteBlueprints {
		all[blueprint.BusinessID] = "route_blueprints"
	}
	for _, variant := range entities.UnresolvedVariants {
		all[variant.BusinessID] = "unresolved_variants"
	}
	return all
}

// configImportPayloadSHA256 deliberately serializes a normalized generic value
// so map keys are deterministic. The payload includes only authoritative
// entities and their embedded source locations; manifest filename/generated
// metadata, source issues, and previews cannot affect this integrity check.
func configImportPayloadSHA256(entities types.ConfigImportEntities) (string, error) {
	entitiesJSON, err := common.Marshal(entities)
	if err != nil {
		return "", err
	}
	var canonicalEntities map[string]any
	if err := common.Unmarshal(entitiesJSON, &canonicalEntities); err != nil {
		return "", err
	}
	payload, err := common.Marshal(map[string]any{"entities": canonicalEntities})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return fmt.Sprintf("%x", digest), nil
}
