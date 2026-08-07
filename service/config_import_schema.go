package service

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
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
	configImportHashPattern             = regexp.MustCompile(`^[a-f0-9]{64}$`)
	configImportCredentialFieldPattern  = regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|auth[_-]?token|authorization|cookie|secret|password)`)
	configImportCredentialValuePatterns = []*regexp.Regexp{
		regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}\b`),
		regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`),
		regexp.MustCompile(`(?:^|[^A-Za-z0-9_-])AIza[A-Za-z0-9_-]{35}(?:$|[^A-Za-z0-9_-])`),
	}
)

// ConfigImportSchemaError is stable enough for API handlers to map into a
// validation response without inspecting an implementation-specific message.
type ConfigImportSchemaError struct {
	Code    string
	Message string
	Data    any
}

func (e *ConfigImportSchemaError) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

func configImportError(code string, format string, args ...any) error {
	return configImportErrorWithData(code, nil, format, args...)
}

func configImportErrorWithData(code string, data any, format string, args ...any) error {
	return &ConfigImportSchemaError{Code: code, Message: fmt.Sprintf(format, args...), Data: data}
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

// normalizeConfigImportDocument sorts authoritative collections and their
// constraint values before they are persisted as canonical entity rows. The
// parser has already verified each entity hash against this normalization.
func normalizeConfigImportDocument(document *types.ConfigImportDocument) {
	for index := range document.Entities.RouteBlueprints {
		if strings.TrimSpace(document.Entities.RouteBlueprints[index].GroupName) == "" {
			document.Entities.RouteBlueprints[index].GroupName = "default"
		}
	}
	canonicalizeConfigImportEntities(&document.Entities)
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
		for _, credentialPattern := range configImportCredentialValuePatterns {
			if credentialPattern.MatchString(typed) {
				return configImportError("SECURITY_CREDENTIAL_VALUE", "credential-like value is not allowed in field %q", fieldName)
			}
		}
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
	if err := validateConfigImportManifestCounts(document.Manifest.Counts, document.Entities); err != nil {
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
	if strings.TrimSpace(manifest.SourceFileName) == "" {
		return configImportError("SCHEMA_MANIFEST_SOURCE_FILE_NAME", "manifest.source_file_name is required")
	}
	if strings.ContainsAny(manifest.SourceFileName, `/\\`) {
		return configImportError("SCHEMA_MANIFEST_SOURCE_FILE_NAME", "manifest.source_file_name must not include a path")
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
	if manifest.Counts == nil {
		return configImportError("SCHEMA_MANIFEST_COUNTS", "manifest.counts is required")
	}
	return nil
}

func validateConfigImportManifestCounts(counts *types.ConfigImportManifestCounts, entities types.ConfigImportEntities) error {
	actual := types.ConfigImportEntityCounts{
		Channels:                 len(entities.Channels),
		ChannelLines:             len(entities.ChannelLines),
		ModelSKUs:                len(entities.ModelSKUs),
		SaleProposals:            len(entities.SaleProposals),
		CostRuleDrafts:           len(entities.CostRuleDrafts),
		ModelMappings:            len(entities.ModelMappings),
		RouteBlueprints:          len(entities.RouteBlueprints),
		GroupRoutingRequirements: len(entities.GroupRoutingRequirements),
		Sources:                  len(entities.Sources),
		UnresolvedVariants:       len(entities.UnresolvedVariants),
	}
	for _, count := range []struct {
		name     string
		declared *int
		actual   int
	}{
		{"channels", counts.Channels, actual.Channels},
		{"channel_lines", counts.ChannelLines, actual.ChannelLines},
		{"model_skus", counts.ModelSKUs, actual.ModelSKUs},
		{"sale_proposals", counts.SaleProposals, actual.SaleProposals},
		{"cost_rule_drafts", counts.CostRuleDrafts, actual.CostRuleDrafts},
		{"model_mappings", counts.ModelMappings, actual.ModelMappings},
		{"route_blueprints", counts.RouteBlueprints, actual.RouteBlueprints},
		{"sources", counts.Sources, actual.Sources},
		{"unresolved_variants", counts.UnresolvedVariants, actual.UnresolvedVariants},
	} {
		if count.declared == nil {
			return configImportError("SCHEMA_MANIFEST_COUNTS", "manifest.counts.%s is required", count.name)
		}
		if *count.declared < 0 {
			return configImportError("LIMIT_MANIFEST_COUNTS", "manifest.counts.%s cannot be negative", count.name)
		}
		if *count.declared != count.actual {
			return configImportError("SCHEMA_MANIFEST_COUNTS", "manifest.counts.%s must equal %d", count.name, count.actual)
		}
	}
	if counts.GroupRoutingRequirements != nil {
		if *counts.GroupRoutingRequirements < 0 {
			return configImportError("LIMIT_MANIFEST_COUNTS", "manifest.counts.group_routing_requirements cannot be negative")
		}
		if *counts.GroupRoutingRequirements != actual.GroupRoutingRequirements {
			return configImportError("SCHEMA_MANIFEST_COUNTS", "manifest.counts.group_routing_requirements must equal %d", actual.GroupRoutingRequirements)
		}
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
		len(entities.RouteBlueprints) + len(entities.GroupRoutingRequirements) + len(entities.Sources) + len(entities.UnresolvedVariants)
	if count > configImportMaxAuthoritativeEntities {
		return configImportError("LIMIT_AUTHORITATIVE_ENTITIES", "entities exceeds %d entries", configImportMaxAuthoritativeEntities)
	}

	businessIDs := make(map[string]string, count)
	sourceIDs := make(map[string]struct{}, len(entities.Sources))
	for index := range entities.Sources {
		source := &entities.Sources[index]
		if err := normalizeConfigImportSourceURL(source); err != nil {
			return err
		}
		if err := validateConfigImportAuthoritativeEntity(&source.ConfigImportAuthoritativeEntity, "sources", index, businessIDs); err != nil {
			return err
		}
		if err := validateConfigImportEntityHash("sources", index, source, source.EntityHash); err != nil {
			return err
		}
		if source.SourceRef != source.BusinessID {
			return configImportError("REFERENCE_SOURCE", "sources[%d].source_ref must equal its business_id", index)
		}
		sourceIDs[source.BusinessID] = struct{}{}
	}

	for index := range entities.Channels {
		channel := &entities.Channels[index]
		if err := validateConfigImportAuthoritativeEntity(&channel.ConfigImportAuthoritativeEntity, "channels", index, businessIDs); err != nil {
			return err
		}
		if err := validateConfigImportEntityHash("channels", index, channel, channel.EntityHash); err != nil {
			return err
		}
	}
	for index := range entities.ChannelLines {
		line := &entities.ChannelLines[index]
		if err := validateConfigImportAuthoritativeEntity(&line.ConfigImportAuthoritativeEntity, "channel_lines", index, businessIDs); err != nil {
			return err
		}
		if err := validateConfigImportEntityHash("channel_lines", index, line, line.EntityHash); err != nil {
			return err
		}
		if line.LineRef == "" || line.LineRef != line.BusinessID {
			return configImportError("SCHEMA_LINE_REF", "channel_lines[%d].line_ref must equal its business_id", index)
		}
		if strings.TrimSpace(line.ChannelRef) == "" || strings.TrimSpace(line.DisplayName) == "" ||
			strings.TrimSpace(line.ProviderTypeHint) == "" || strings.TrimSpace(line.Region) == "" || strings.TrimSpace(line.Protocol) == "" {
			return configImportError("SCHEMA_CHANNEL_LINE", "channel_lines[%d] requires channel_ref, display_name, provider_type_hint, region, and protocol", index)
		}
		if line.StatusProposal != "disabled" {
			return configImportError("SCHEMA_CHANNEL_LINE_STATUS", "channel_lines[%d].status_proposal must be disabled", index)
		}
	}
	for index := range entities.ModelSKUs {
		modelSKU := &entities.ModelSKUs[index]
		if err := validateConfigImportAuthoritativeEntity(&modelSKU.ConfigImportAuthoritativeEntity, "model_skus", index, businessIDs); err != nil {
			return err
		}
		if err := validateConfigImportEntityHash("model_skus", index, modelSKU, modelSKU.EntityHash); err != nil {
			return err
		}
	}
	for index := range entities.SaleProposals {
		proposal := &entities.SaleProposals[index]
		if err := validateConfigImportAuthoritativeEntity(&proposal.ConfigImportAuthoritativeEntity, "sale_proposals", index, businessIDs); err != nil {
			return err
		}
		if err := validateConfigImportEntityHash("sale_proposals", index, proposal, proposal.EntityHash); err != nil {
			return err
		}
		for field, value := range map[string]*string{
			"unit_price": proposal.UnitPrice, "price_per_unit": proposal.PricePerUnit, "margin_ratio": proposal.MarginRatio,
			"input_per_million": proposal.InputPerMillion, "output_per_million": proposal.OutputPerMillion,
			"completion_per_million": proposal.CompletionPerMillion, "total_per_million": proposal.TotalPerMillion,
		} {
			if err := validateConfigImportDecimal("sale_proposals."+field, value); err != nil {
				return err
			}
		}
		if proposal.DurationPrice != nil {
			if err := validateConfigImportDecimal("sale_proposals.duration_price.price", &proposal.DurationPrice.Price); err != nil {
				return err
			}
		}
		if proposal.SeedanceTokenPrice != nil {
			if err := validateConfigImportDecimal("sale_proposals.seedance_token_price.price_per_million", &proposal.SeedanceTokenPrice.PricePerMillion); err != nil {
				return err
			}
		}
	}
	for index := range entities.CostRuleDrafts {
		draft := &entities.CostRuleDrafts[index]
		if err := validateConfigImportAuthoritativeEntity(&draft.ConfigImportAuthoritativeEntity, "cost_rule_drafts", index, businessIDs); err != nil {
			return err
		}
		if draft.Enabled == nil {
			return configImportError("SCHEMA_COST_ENABLED", "cost_rule_drafts[%d].enabled is required", index)
		}
		if err := validateConfigImportEntityHash("cost_rule_drafts", index, draft, draft.EntityHash); err != nil {
			return err
		}
		for field, value := range map[string]*string{
			"unit_price": draft.UnitPrice, "price_per_second": draft.PricePerSecond, "input_per_million": draft.InputPerMillion,
			"output_per_million": draft.OutputPerMillion, "completion_per_million": draft.CompletionPerMillion,
			"total_per_million": draft.TotalPerMillion, "billing_multiplier": draft.BillingMultiplier,
			"purchase_discount_ratio": draft.PurchaseDiscountRatio, "recharge_exchange_ratio": draft.RechargeExchangeRatio,
			"fee_rate": draft.FeeRate, "currency_to_usd_rate": draft.CurrencyToUSDRate,
			"normalized_usd_unit_price":             draft.NormalizedUSDUnitPrice,
			"normalized_usd_price_per_second":       draft.NormalizedUSDPricePerSecond,
			"normalized_usd_input_per_million":      draft.NormalizedUSDInputPerMillion,
			"normalized_usd_output_per_million":     draft.NormalizedUSDOutputPerMillion,
			"normalized_usd_completion_per_million": draft.NormalizedUSDCompletionPerMillion,
			"normalized_usd_total_per_million":      draft.NormalizedUSDTotalPerMillion,
		} {
			if err := validateConfigImportDecimal("cost_rule_drafts."+field, value); err != nil {
				return err
			}
		}
		canonicalCostVariantKey, err := types.NormalizeCostVariantKey(draft.CostVariantKey)
		if err != nil || draft.CostVariantKey == "" || canonicalCostVariantKey != draft.CostVariantKey {
			return configImportError("SCHEMA_COST_VARIANT_KEY", "cost_rule_drafts[%d].cost_variant_key is invalid", index)
		}
	}
	for index := range entities.ModelMappings {
		mapping := &entities.ModelMappings[index]
		if err := validateConfigImportAuthoritativeEntity(&mapping.ConfigImportAuthoritativeEntity, "model_mappings", index, businessIDs); err != nil {
			return err
		}
		if err := validateConfigImportEntityHash("model_mappings", index, mapping, mapping.EntityHash); err != nil {
			return err
		}
		if strings.TrimSpace(mapping.CanonicalModel) == "" || strings.TrimSpace(mapping.ClientModel) == "" ||
			strings.TrimSpace(mapping.LineRef) == "" || strings.TrimSpace(mapping.UpstreamModel) == "" || strings.TrimSpace(mapping.SKURef) == "" {
			return configImportError("SCHEMA_MODEL_MAPPING", "model_mappings[%d] requires canonical_model, client_model, line_ref, upstream_model, and sku_ref", index)
		}
	}
	for index := range entities.RouteBlueprints {
		blueprint := &entities.RouteBlueprints[index]
		if err := validateConfigImportAuthoritativeEntity(&blueprint.ConfigImportAuthoritativeEntity, "route_blueprints", index, businessIDs); err != nil {
			return err
		}
		if err := validateConfigImportEntityHash("route_blueprints", index, blueprint, blueprint.EntityHash); err != nil {
			return err
		}
		if strings.TrimSpace(blueprint.CanonicalModel) == "" || strings.TrimSpace(blueprint.ClientModel) == "" {
			return configImportError("SCHEMA_ROUTE_BLUEPRINT", "route_blueprints[%d] requires canonical_model and client_model", index)
		}
		if blueprint.MergeMode != "" && blueprint.MergeMode != types.ConfigImportRouteMergeModeReplace &&
			blueprint.MergeMode != types.ConfigImportRouteMergeModeMerge && blueprint.MergeMode != types.ConfigImportRouteMergeModeSkip {
			return configImportError("SCHEMA_ROUTE_MERGE_MODE", "route_blueprints[%d].merge_mode is invalid", index)
		}
		for targetIndex := range blueprint.Targets {
			if err := validateConfigImportRouteTarget(&blueprint.Targets[targetIndex], index, targetIndex); err != nil {
				return err
			}
		}
	}
	seenRequirementGroups := make(map[string]struct{}, len(entities.GroupRoutingRequirements))
	for index := range entities.GroupRoutingRequirements {
		requirement := &entities.GroupRoutingRequirements[index]
		if err := validateConfigImportAuthoritativeEntity(&requirement.ConfigImportAuthoritativeEntity, "group_routing_requirements", index, businessIDs); err != nil {
			return err
		}
		if err := validateConfigImportEntityHash("group_routing_requirements", index, requirement, requirement.EntityHash); err != nil {
			return err
		}
		requirement.GroupName = strings.TrimSpace(requirement.GroupName)
		if requirement.GroupName == "" {
			return configImportError("SCHEMA_GROUP_ROUTING_REQUIREMENT", "group_routing_requirements[%d].group_name is required", index)
		}
		if _, exists := seenRequirementGroups[requirement.GroupName]; exists {
			return configImportError("DUPLICATE_GROUP_ROUTING_REQUIREMENT", "group_routing_requirements[%d].group_name %q is duplicated", index, requirement.GroupName)
		}
		seenRequirementGroups[requirement.GroupName] = struct{}{}
	}
	for index := range entities.UnresolvedVariants {
		variant := &entities.UnresolvedVariants[index]
		if err := validateConfigImportAuthoritativeEntity(&variant.ConfigImportAuthoritativeEntity, "unresolved_variants", index, businessIDs); err != nil {
			return err
		}
		if err := validateConfigImportEntityHash("unresolved_variants", index, variant, variant.EntityHash); err != nil {
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

// configImportEntitySHA256 hashes an entity's canonical authoritative content:
// its typed JSON fields (including source location) become a key-sorted object,
// and unordered contract arrays are normalized. entity_hash itself is removed
// before hashing, so the converter can produce a stable digest without a
// self-referential fixed-point calculation. Source URLs are normalized before
// this function is called.
func validateConfigImportEntityHash(collection string, index int, entity any, suppliedHash string) error {
	actualHash, err := configImportEntitySHA256(entity)
	if err != nil {
		return configImportError("SCHEMA_ENTITY_HASH", "%s[%d] canonicalization failed: %v", collection, index, err)
	}
	if suppliedHash != actualHash {
		return configImportError("SCHEMA_ENTITY_HASH", "%s[%d].entity_hash does not match canonical entity content", collection, index)
	}
	return nil
}

func configImportEntitySHA256(entity any) (string, error) {
	encoded, err := common.Marshal(entity)
	if err != nil {
		return "", err
	}
	var canonical map[string]any
	if err := common.Unmarshal(encoded, &canonical); err != nil {
		return "", err
	}
	delete(canonical, "entity_hash")
	canonicalizeConfigImportGenericValue(canonical)
	payload, err := common.Marshal(canonical)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return fmt.Sprintf("%x", digest), nil
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

func validateConfigImportRouteTarget(target *types.ConfigImportRouteTarget, blueprintIndex int, targetIndex int) error {
	if strings.TrimSpace(target.RouteTargetRef) == "" || strings.TrimSpace(target.LineRef) == "" ||
		strings.TrimSpace(target.UpstreamModel) == "" || strings.TrimSpace(target.SKURef) == "" {
		return configImportError("SCHEMA_ROUTE_TARGET", "route_blueprints[%d].targets[%d] requires route_target_ref, line_ref, upstream_model, and sku_ref", blueprintIndex, targetIndex)
	}
	canonicalCostVariantKey, err := types.NormalizeCostVariantKey(target.CostVariantKey)
	if err != nil || target.CostVariantKey == "" || canonicalCostVariantKey != target.CostVariantKey {
		return configImportError("SCHEMA_COST_VARIANT_KEY", "route_blueprints[%d].targets[%d].cost_variant_key is invalid", blueprintIndex, targetIndex)
	}
	if target.Enabled == nil || *target.Enabled {
		return configImportError("SCHEMA_ROUTE_TARGET_ENABLED", "route_blueprints[%d].targets[%d].enabled must be false", blueprintIndex, targetIndex)
	}
	if target.DurationMin != nil && *target.DurationMin < 0 || target.DurationMax != nil && *target.DurationMax < 0 {
		return configImportError("SCHEMA_ROUTE_DURATION", "route_blueprints[%d].targets[%d] duration cannot be negative", blueprintIndex, targetIndex)
	}
	if target.DurationMin != nil && target.DurationMax != nil && *target.DurationMin > *target.DurationMax {
		return configImportError("SCHEMA_ROUTE_DURATION", "route_blueprints[%d].targets[%d] duration_min cannot exceed duration_max", blueprintIndex, targetIndex)
	}
	for _, duration := range target.DurationValues {
		if duration < 0 {
			return configImportError("SCHEMA_ROUTE_DURATION", "route_blueprints[%d].targets[%d] duration_values cannot be negative", blueprintIndex, targetIndex)
		}
	}
	for _, bounds := range []*types.ConfigImportReferenceBounds{target.ReferenceMinimums, target.ReferenceLimits} {
		if bounds == nil || bounds.Images == nil || bounds.Videos == nil || bounds.Audios == nil {
			return configImportError("SCHEMA_ROUTE_REFERENCE", "route_blueprints[%d].targets[%d] requires complete reference minimums and limits", blueprintIndex, targetIndex)
		}
		for _, value := range []*int{bounds.Images, bounds.Videos, bounds.Audios} {
			if *value < 0 {
				return configImportError("SCHEMA_ROUTE_REFERENCE", "route_blueprints[%d].targets[%d] reference bounds cannot be negative", blueprintIndex, targetIndex)
			}
		}
	}
	limits := target.ReferenceLimits
	if target.ReferenceTotalMax != nil {
		if *target.ReferenceTotalMax < 0 || *target.ReferenceTotalMax > *limits.Images+*limits.Videos+*limits.Audios {
			return configImportError("SCHEMA_ROUTE_REFERENCE", "route_blueprints[%d].targets[%d] reference_total_max is invalid", blueprintIndex, targetIndex)
		}
	}
	if target.ReferenceVideoAudioTotalMax != nil {
		if *target.ReferenceVideoAudioTotalMax < 0 || *target.ReferenceVideoAudioTotalMax > *limits.Videos+*limits.Audios {
			return configImportError("SCHEMA_ROUTE_REFERENCE", "route_blueprints[%d].targets[%d] reference_video_audio_total_max is invalid", blueprintIndex, targetIndex)
		}
		if target.ReferenceTotalMax != nil && *target.ReferenceTotalMax > *limits.Images+*target.ReferenceVideoAudioTotalMax {
			return configImportError("SCHEMA_ROUTE_REFERENCE", "route_blueprints[%d].targets[%d] aggregate reference limits conflict", blueprintIndex, targetIndex)
		}
	}
	if target.ReferenceVideoTotalDurationSeconds != nil {
		if *target.ReferenceVideoTotalDurationSeconds < 0 || (*limits.Videos == 0 && *target.ReferenceVideoTotalDurationSeconds != 0) {
			return configImportError("SCHEMA_ROUTE_REFERENCE", "route_blueprints[%d].targets[%d] reference_video_total_duration_seconds is invalid", blueprintIndex, targetIndex)
		}
	}
	for _, mode := range target.ReferenceModes {
		switch mode {
		case "first_last_frames", "omni_reference", "agentic":
		default:
			return configImportError("SCHEMA_ROUTE_REFERENCE", "route_blueprints[%d].targets[%d] reference_modes contains an invalid value", blueprintIndex, targetIndex)
		}
	}
	return nil
}

func validateConfigImportEntityReferences(entities *types.ConfigImportEntities, sourceIDs map[string]struct{}, businessIDs map[string]string) error {
	routeTargets := make(map[string]*types.ConfigImportRouteTarget)
	linesByRef := make(map[string]*types.ConfigImportChannelLine, len(entities.ChannelLines))
	for index := range entities.ChannelLines {
		line := &entities.ChannelLines[index]
		linesByRef[line.LineRef] = line
	}
	for blueprintIndex := range entities.RouteBlueprints {
		for targetIndex := range entities.RouteBlueprints[blueprintIndex].Targets {
			target := &entities.RouteBlueprints[blueprintIndex].Targets[targetIndex]
			if _, exists := routeTargets[target.RouteTargetRef]; exists {
				return configImportError("DUPLICATE_ROUTE_TARGET_REF", "route_target_ref %q is duplicated", target.RouteTargetRef)
			}
			routeTargets[target.RouteTargetRef] = target
		}
	}

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
		if err := requireConfigImportReference("channel_lines", index, "line_ref", line.LineRef, businessIDs, "channel_lines"); err != nil {
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
		if err := requireConfigImportReference("cost_rule_drafts", index, "line_ref", draft.LineRef, businessIDs, "channel_lines"); err != nil {
			return err
		}
		target, exists := routeTargets[draft.RouteTargetRef]
		if !exists {
			return configImportError("REFERENCE_NOT_FOUND", "cost_rule_drafts[%d].route_target_ref %q does not exist", index, draft.RouteTargetRef)
		}
		if strings.TrimSpace(draft.UpstreamModel) == "" {
			return configImportError("SCHEMA_COST_UPSTREAM_MODEL", "cost_rule_drafts[%d].upstream_model is required", index)
		}
		if target.LineRef != draft.LineRef || target.CostVariantKey != draft.CostVariantKey {
			return configImportError("ROUTING_COST_VARIANT_MISMATCH", "cost_rule_drafts[%d] does not match route_target_ref %q", index, draft.RouteTargetRef)
		}
		if draft.UpstreamModel != "" && target.UpstreamModel != draft.UpstreamModel {
			return configImportError("ROUTING_COST_VARIANT_MISMATCH", "cost_rule_drafts[%d].upstream_model does not match route_target_ref %q", index, draft.RouteTargetRef)
		}
	}
	for index := range entities.ModelMappings {
		mapping := entities.ModelMappings[index]
		if err := requireConfigImportSourceReference("model_mappings", index, mapping.SourceRef, sourceIDs); err != nil {
			return err
		}
		if err := requireConfigImportReference("model_mappings", index, "line_ref", mapping.LineRef, businessIDs, "channel_lines"); err != nil {
			return err
		}
		if err := requireConfigImportReference("model_mappings", index, "sku_ref", mapping.SKURef, businessIDs, "model_skus"); err != nil {
			return err
		}
	}
	for index := range entities.RouteBlueprints {
		blueprint := entities.RouteBlueprints[index]
		if err := requireConfigImportSourceReference("route_blueprints", index, blueprint.SourceRef, sourceIDs); err != nil {
			return err
		}
		for _, mappingRef := range blueprint.ModelMappingRefs {
			if err := requireConfigImportReference("route_blueprints", index, "model_mapping_refs", mappingRef, businessIDs, "model_mappings"); err != nil {
				return err
			}
		}
		for targetIndex := range blueprint.Targets {
			target := blueprint.Targets[targetIndex]
			if err := requireConfigImportReference("route_blueprints", index, "targets.line_ref", target.LineRef, businessIDs, "channel_lines"); err != nil {
				return err
			}
			if err := requireConfigImportReference("route_blueprints", index, "targets.sku_ref", target.SKURef, businessIDs, "model_skus"); err != nil {
				return err
			}
			line := linesByRef[target.LineRef]
			if line.SupportsRealPerson != nil && (target.SupportsRealPerson == nil || *line.SupportsRealPerson != *target.SupportsRealPerson) {
				return configImportError("ROUTING_REAL_PERSON_MISMATCH", "route_blueprints[%d].targets[%d].supports_real_person conflicts with line_ref %q", index, targetIndex, target.LineRef)
			}
		}
	}
	for index := range entities.GroupRoutingRequirements {
		requirement := entities.GroupRoutingRequirements[index]
		if err := requireConfigImportSourceReference("group_routing_requirements", index, requirement.SourceRef, sourceIDs); err != nil {
			return err
		}
	}
	for index := range entities.UnresolvedVariants {
		variant := entities.UnresolvedVariants[index]
		if err := requireConfigImportSourceReference("unresolved_variants", index, variant.SourceRef, sourceIDs); err != nil {
			return err
		}
		if variant.LineRef != "" {
			if err := requireConfigImportReference("unresolved_variants", index, "line_ref", variant.LineRef, businessIDs, "channel_lines"); err != nil {
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
	all := make(map[string]string, len(entities.Channels)+len(entities.ChannelLines)+len(entities.ModelSKUs)+len(entities.SaleProposals)+len(entities.CostRuleDrafts)+len(entities.ModelMappings)+len(entities.RouteBlueprints)+len(entities.GroupRoutingRequirements)+len(entities.Sources)+len(entities.UnresolvedVariants))
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
	for _, requirement := range entities.GroupRoutingRequirements {
		all[requirement.BusinessID] = "group_routing_requirements"
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
	var canonicalEntities types.ConfigImportEntities
	if err := common.Unmarshal(entitiesJSON, &canonicalEntities); err != nil {
		return "", err
	}
	canonicalizeConfigImportEntities(&canonicalEntities)
	canonicalEntitiesJSON, err := common.Marshal(canonicalEntities)
	if err != nil {
		return "", err
	}
	var canonicalEntityMap map[string]any
	if err := common.Unmarshal(canonicalEntitiesJSON, &canonicalEntityMap); err != nil {
		return "", err
	}
	payload, err := common.Marshal(map[string]any{"entities": canonicalEntityMap})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return fmt.Sprintf("%x", digest), nil
}

func canonicalizeConfigImportEntities(entities *types.ConfigImportEntities) {
	sort.Slice(entities.Channels, func(left, right int) bool {
		return entities.Channels[left].BusinessID < entities.Channels[right].BusinessID
	})
	sort.Slice(entities.ChannelLines, func(left, right int) bool {
		return entities.ChannelLines[left].BusinessID < entities.ChannelLines[right].BusinessID
	})
	sort.Slice(entities.ModelSKUs, func(left, right int) bool {
		return entities.ModelSKUs[left].BusinessID < entities.ModelSKUs[right].BusinessID
	})
	sort.Slice(entities.SaleProposals, func(left, right int) bool {
		return entities.SaleProposals[left].BusinessID < entities.SaleProposals[right].BusinessID
	})
	sort.Slice(entities.CostRuleDrafts, func(left, right int) bool {
		return entities.CostRuleDrafts[left].BusinessID < entities.CostRuleDrafts[right].BusinessID
	})
	sort.Slice(entities.ModelMappings, func(left, right int) bool {
		return entities.ModelMappings[left].BusinessID < entities.ModelMappings[right].BusinessID
	})
	sort.Slice(entities.RouteBlueprints, func(left, right int) bool {
		return entities.RouteBlueprints[left].BusinessID < entities.RouteBlueprints[right].BusinessID
	})
	sort.Slice(entities.GroupRoutingRequirements, func(left, right int) bool {
		return entities.GroupRoutingRequirements[left].BusinessID < entities.GroupRoutingRequirements[right].BusinessID
	})
	sort.Slice(entities.Sources, func(left, right int) bool {
		return entities.Sources[left].BusinessID < entities.Sources[right].BusinessID
	})
	sort.Slice(entities.UnresolvedVariants, func(left, right int) bool {
		return entities.UnresolvedVariants[left].BusinessID < entities.UnresolvedVariants[right].BusinessID
	})

	for index := range entities.ModelSKUs {
		canonicalizeConfigImportStrings(entities.ModelSKUs[index].OutputResolutions)
		canonicalizeConfigImportInts(entities.ModelSKUs[index].DurationValues)
		canonicalizeConfigImportStrings(entities.ModelSKUs[index].AspectRatios)
		canonicalizeConfigImportStrings(entities.ModelSKUs[index].InputModes)
	}
	for index := range entities.RouteBlueprints {
		blueprint := &entities.RouteBlueprints[index]
		canonicalizeConfigImportStrings(blueprint.ModelMappingRefs)
		sort.Slice(blueprint.Targets, func(left, right int) bool {
			leftTarget := blueprint.Targets[left]
			rightTarget := blueprint.Targets[right]
			if leftTarget.RouteTargetRef != rightTarget.RouteTargetRef {
				return leftTarget.RouteTargetRef < rightTarget.RouteTargetRef
			}
			if leftTarget.LineRef != rightTarget.LineRef {
				return leftTarget.LineRef < rightTarget.LineRef
			}
			return leftTarget.SKURef < rightTarget.SKURef
		})
		for targetIndex := range blueprint.Targets {
			target := &blueprint.Targets[targetIndex]
			canonicalizeConfigImportStrings(target.OutputResolutions)
			canonicalizeConfigImportInts(target.DurationValues)
			canonicalizeConfigImportStrings(target.AspectRatios)
			canonicalizeConfigImportStrings(target.InputModes)
		}
	}
}

func canonicalizeConfigImportStrings(values []string) {
	sort.Strings(values)
}

func canonicalizeConfigImportInts(values []int) {
	sort.Ints(values)
}

func canonicalizeConfigImportGenericValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			canonicalizeConfigImportGenericValue(child)
		}
	case []any:
		for _, child := range typed {
			canonicalizeConfigImportGenericValue(child)
		}
		if len(typed) < 2 {
			return
		}
		if configImportAllStrings(typed) {
			sort.Slice(typed, func(left, right int) bool { return typed[left].(string) < typed[right].(string) })
			return
		}
		if configImportAllNumbers(typed) {
			sort.Slice(typed, func(left, right int) bool { return typed[left].(float64) < typed[right].(float64) })
			return
		}
		if configImportAllObjectsWithKey(typed, "business_id") {
			sort.Slice(typed, func(left, right int) bool {
				return typed[left].(map[string]any)["business_id"].(string) < typed[right].(map[string]any)["business_id"].(string)
			})
			return
		}
		if configImportAllObjectsWithKey(typed, "route_target_ref") {
			sort.Slice(typed, func(left, right int) bool {
				return typed[left].(map[string]any)["route_target_ref"].(string) < typed[right].(map[string]any)["route_target_ref"].(string)
			})
		}
	}
}

func configImportAllStrings(values []any) bool {
	for _, value := range values {
		if _, ok := value.(string); !ok {
			return false
		}
	}
	return true
}

func configImportAllNumbers(values []any) bool {
	for _, value := range values {
		if _, ok := value.(float64); !ok {
			return false
		}
	}
	return true
}

func configImportAllObjectsWithKey(values []any, key string) bool {
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
