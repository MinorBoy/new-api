package types

// ConfigImportBatchStatus describes the lifecycle of a channel configuration
// import batch. These values are persisted as contract values, so do not rename
// or repurpose them.
type ConfigImportBatchStatus string

const (
	ConfigImportBatchStatusValidating    ConfigImportBatchStatus = "validating"
	ConfigImportBatchStatusBlocked       ConfigImportBatchStatus = "blocked"
	ConfigImportBatchStatusBinding       ConfigImportBatchStatus = "binding"
	ConfigImportBatchStatusStaged        ConfigImportBatchStatus = "staged"
	ConfigImportBatchStatusReady         ConfigImportBatchStatus = "ready"
	ConfigImportBatchStatusPublishing    ConfigImportBatchStatus = "publishing"
	ConfigImportBatchStatusPublished     ConfigImportBatchStatus = "published"
	ConfigImportBatchStatusPublishFailed ConfigImportBatchStatus = "publish_failed"
)

type ConfigImportItemState string

const (
	ConfigImportItemStateNew       ConfigImportItemState = "new"
	ConfigImportItemStateUnchanged ConfigImportItemState = "unchanged"
	ConfigImportItemStateChanged   ConfigImportItemState = "changed"
	ConfigImportItemStateConflict  ConfigImportItemState = "conflict"
	ConfigImportItemStateExcluded  ConfigImportItemState = "excluded"
)

type ConfigImportIssueSeverity string

const (
	ConfigImportIssueSeverityInfo    ConfigImportIssueSeverity = "info"
	ConfigImportIssueSeverityWarning ConfigImportIssueSeverity = "warning"
	ConfigImportIssueSeverityError   ConfigImportIssueSeverity = "error"
)

type ConfigImportBindingAction string

const (
	ConfigImportBindingActionCreate ConfigImportBindingAction = "create"
	ConfigImportBindingActionBind   ConfigImportBindingAction = "bind"
	ConfigImportBindingActionSkip   ConfigImportBindingAction = "skip"
)

type ConfigImportResolutionAction string

const (
	ConfigImportResolutionActionUseImport    ConfigImportResolutionAction = "use_import"
	ConfigImportResolutionActionKeepExisting ConfigImportResolutionAction = "keep_existing"
	ConfigImportResolutionActionExclude      ConfigImportResolutionAction = "exclude"
	ConfigImportResolutionActionSplitLine    ConfigImportResolutionAction = "split_line"
	ConfigImportResolutionActionBindVariant  ConfigImportResolutionAction = "bind_variant"
)

type ConfigImportRouteMergeMode string

const (
	ConfigImportRouteMergeModeReplace ConfigImportRouteMergeMode = "replace"
	ConfigImportRouteMergeModeMerge   ConfigImportRouteMergeMode = "merge"
	ConfigImportRouteMergeModeSkip    ConfigImportRouteMergeMode = "skip"
)

// ConfigImportDocument is the versioned, credential-free configuration import
// artifact. It only carries stable business identifiers; database identifiers
// are intentionally absent from this contract.
type ConfigImportDocument struct {
	Kind            string                    `json:"kind"`
	SchemaVersion   int                       `json:"schema_version"`
	TemplateVersion string                    `json:"template_version"`
	Manifest        ConfigImportManifest      `json:"manifest"`
	Entities        ConfigImportEntities      `json:"entities"`
	DerivedPreview  map[string]any            `json:"derived_preview"`
	Issues          []ConfigImportSourceIssue `json:"issues"`
}

type ConfigImportManifest struct {
	SourceFileName   string                      `json:"source_file_name"`
	SourceSHA256     string                      `json:"source_sha256"`
	PayloadSHA256    string                      `json:"payload_sha256"`
	GeneratedAt      string                      `json:"generated_at"`
	ConverterVersion string                      `json:"converter_version"`
	TemplateMatch    string                      `json:"template_match"`
	Counts           *ConfigImportManifestCounts `json:"counts"`
}

// ConfigImportManifestCounts uses pointers so an absent count cannot be
// confused with an explicitly supplied zero.
type ConfigImportManifestCounts struct {
	Channels           *int `json:"channels"`
	ChannelLines       *int `json:"channel_lines"`
	ModelSKUs          *int `json:"model_skus"`
	SaleProposals      *int `json:"sale_proposals"`
	CostRuleDrafts     *int `json:"cost_rule_drafts"`
	ModelMappings      *int `json:"model_mappings"`
	RouteBlueprints    *int `json:"route_blueprints"`
	Sources            *int `json:"sources"`
	UnresolvedVariants *int `json:"unresolved_variants"`
}

type ConfigImportEntityCounts struct {
	Channels           int `json:"channels"`
	ChannelLines       int `json:"channel_lines"`
	ModelSKUs          int `json:"model_skus"`
	SaleProposals      int `json:"sale_proposals"`
	CostRuleDrafts     int `json:"cost_rule_drafts"`
	ModelMappings      int `json:"model_mappings"`
	RouteBlueprints    int `json:"route_blueprints"`
	Sources            int `json:"sources"`
	UnresolvedVariants int `json:"unresolved_variants"`
}

type ConfigImportChannelModelSnapshotDiff struct {
	ChannelID      int      `json:"channel_id"`
	ChannelName    string   `json:"channel_name"`
	LineRefs       []string `json:"line_refs"`
	AddedModels    []string `json:"added_models"`
	RetainedModels []string `json:"retained_models"`
	RemovedModels  []string `json:"removed_models"`
}

// ConfigImportAuthoritativeEntity is embedded by every entity supplied by the
// converter. Its source location makes each imported fact traceable to a
// workbook row without relying on a mutable database identifier.
type ConfigImportAuthoritativeEntity struct {
	BusinessID    string `json:"business_id"`
	EntityHash    string `json:"entity_hash"`
	SourceRef     string `json:"source_ref"`
	Sheet         string `json:"sheet,omitempty"`
	Row           *int   `json:"row,omitempty"`
	RawBusinessID string `json:"raw_business_id,omitempty"`
	AuditNote     string `json:"audit_note,omitempty"`
}

type ConfigImportEntities struct {
	Channels           []ConfigImportChannel           `json:"channels"`
	ChannelLines       []ConfigImportChannelLine       `json:"channel_lines"`
	ModelSKUs          []ConfigImportModelSKU          `json:"model_skus"`
	SaleProposals      []ConfigImportSaleProposal      `json:"sale_proposals"`
	CostRuleDrafts     []ConfigImportCostRuleDraft     `json:"cost_rule_drafts"`
	ModelMappings      []ConfigImportModelMapping      `json:"model_mappings"`
	RouteBlueprints    []ConfigImportRouteBlueprint    `json:"route_blueprints"`
	Sources            []ConfigImportSource            `json:"sources"`
	UnresolvedVariants []ConfigImportUnresolvedVariant `json:"unresolved_variants"`
}

type ConfigImportChannel struct {
	ConfigImportAuthoritativeEntity
	DisplayName    string `json:"display_name,omitempty"`
	Provider       string `json:"provider,omitempty"`
	ChannelType    *int   `json:"channel_type,omitempty"`
	Enabled        *bool  `json:"enabled,omitempty"`
	RoutingEnabled *bool  `json:"routing_enabled,omitempty"`
}

type ConfigImportChannelLine struct {
	ConfigImportAuthoritativeEntity
	LineRef            string `json:"line_ref"`
	ChannelRef         string `json:"channel_ref"`
	DisplayName        string `json:"display_name,omitempty"`
	ProviderTypeHint   string `json:"provider_type_hint,omitempty"`
	Region             string `json:"region,omitempty"`
	Protocol           string `json:"protocol,omitempty"`
	SupportsRealPerson *bool  `json:"supports_real_person,omitempty"`
	StatusProposal     string `json:"status_proposal"`
	Note               string `json:"note,omitempty"`
}

type ConfigImportModelSKU struct {
	ConfigImportAuthoritativeEntity
	// LineRef is retained only to read legacy import documents. Structured v2
	// SKU records describe global model specifications; mappings and targets
	// own the channel-line and upstream-model relationship.
	LineRef            string                       `json:"line_ref,omitempty"`
	UpstreamModel      string                       `json:"upstream_model,omitempty"`
	OutputResolutions  []string                     `json:"output_resolutions,omitempty"`
	DurationValues     []int                        `json:"duration_values,omitempty"`
	DurationMin        *int                         `json:"duration_min,omitempty"`
	DurationMax        *int                         `json:"duration_max,omitempty"`
	AspectRatios       []string                     `json:"aspect_ratios,omitempty"`
	InputModes         []string                     `json:"input_modes,omitempty"`
	ReferenceMinimums  *ConfigImportReferenceBounds `json:"reference_minimums,omitempty"`
	ReferenceLimits    *ConfigImportReferenceBounds `json:"reference_limits,omitempty"`
	SupportsRealPerson *bool                        `json:"supports_real_person,omitempty"`
}

type ConfigImportSaleProposal struct {
	ConfigImportAuthoritativeEntity
	ModelSKURef          string                 `json:"model_sku_ref"`
	Currency             string                 `json:"currency,omitempty"`
	UnitPrice            *string                `json:"unit_price,omitempty"`
	PricePerUnit         *string                `json:"price_per_unit,omitempty"`
	MarginRatio          *string                `json:"margin_ratio,omitempty"`
	Enabled              *bool                  `json:"enabled,omitempty"`
	BillingMode          string                 `json:"billing_mode,omitempty"`
	BillingExpr          string                 `json:"billing_expr,omitempty"`
	TokenMode            string                 `json:"token_mode,omitempty"`
	DurationPrice        *DurationPriceProposal `json:"duration_price,omitempty"`
	SelectedGroups       []string               `json:"selected_groups,omitempty"`
	GroupPrices          map[string]string      `json:"group_prices,omitempty"`
	InputPerMillion      *string                `json:"input_per_million,omitempty"`
	OutputPerMillion     *string                `json:"output_per_million,omitempty"`
	CompletionPerMillion *string                `json:"completion_per_million,omitempty"`
	TotalPerMillion      *string                `json:"total_per_million,omitempty"`
}

// DurationPriceProposal is kept decimal-safe at the contract boundary. The
// runtime billing setting uses float64 for compatibility, while staging
// validates the bounds before converting to that representation.
type DurationPriceProposal struct {
	Price                  string `json:"price"`
	Unit                   string `json:"unit"`
	RoundingStepSeconds    int    `json:"rounding_step_seconds"`
	MinimumDurationSeconds int    `json:"minimum_duration_seconds"`
}

type ConfigImportCostRuleDraft struct {
	ConfigImportAuthoritativeEntity
	LineRef                           string  `json:"line_ref"`
	UpstreamModel                     string  `json:"upstream_model,omitempty"`
	CostVariantKey                    string  `json:"cost_variant_key"`
	RouteTargetRef                    string  `json:"route_target_ref"`
	Scenario                          string  `json:"scenario,omitempty"`
	CostMode                          string  `json:"cost_mode,omitempty"`
	Currency                          string  `json:"currency,omitempty"`
	UnitPrice                         *string `json:"unit_price,omitempty"`
	PricePerSecond                    *string `json:"price_per_second,omitempty"`
	InputPerMillion                   *string `json:"input_per_million,omitempty"`
	OutputPerMillion                  *string `json:"output_per_million,omitempty"`
	CompletionPerMillion              *string `json:"completion_per_million,omitempty"`
	TotalPerMillion                   *string `json:"total_per_million,omitempty"`
	BillingMultiplier                 *string `json:"billing_multiplier,omitempty"`
	PurchaseDiscountRatio             *string `json:"purchase_discount_ratio,omitempty"`
	RechargeExchangeRatio             *string `json:"recharge_exchange_ratio,omitempty"`
	FeeRate                           *string `json:"fee_rate,omitempty"`
	CurrencyToUSDRate                 *string `json:"currency_to_usd_rate,omitempty"`
	NormalizedUSDUnitPrice            *string `json:"normalized_usd_unit_price,omitempty"`
	NormalizedUSDPricePerSecond       *string `json:"normalized_usd_price_per_second,omitempty"`
	NormalizedUSDInputPerMillion      *string `json:"normalized_usd_input_per_million,omitempty"`
	NormalizedUSDOutputPerMillion     *string `json:"normalized_usd_output_per_million,omitempty"`
	NormalizedUSDCompletionPerMillion *string `json:"normalized_usd_completion_per_million,omitempty"`
	NormalizedUSDTotalPerMillion      *string `json:"normalized_usd_total_per_million,omitempty"`
	ZeroCostReason                    string  `json:"zero_cost_reason,omitempty"`
	ChargeEvent                       string  `json:"charge_event,omitempty"`
	MeterSource                       string  `json:"meter_source,omitempty"`
	TokenMode                         string  `json:"token_mode,omitempty"`
}

type ConfigImportModelMapping struct {
	ConfigImportAuthoritativeEntity
	CanonicalModel string `json:"canonical_model"`
	ClientModel    string `json:"client_model"`
	LineRef        string `json:"line_ref"`
	UpstreamModel  string `json:"upstream_model"`
	SKURef         string `json:"sku_ref"`
}

type ConfigImportReferenceBounds struct {
	Images *int `json:"images,omitempty"`
	Videos *int `json:"videos,omitempty"`
	Audios *int `json:"audios,omitempty"`
}

type ConfigImportRouteTarget struct {
	RouteTargetRef     string                       `json:"route_target_ref"`
	LineRef            string                       `json:"line_ref"`
	UpstreamModel      string                       `json:"upstream_model"`
	SKURef             string                       `json:"sku_ref"`
	CostVariantKey     string                       `json:"cost_variant_key"`
	OutputResolutions  []string                     `json:"output_resolutions,omitempty"`
	DurationValues     []int                        `json:"duration_values,omitempty"`
	DurationMin        *int                         `json:"duration_min,omitempty"`
	DurationMax        *int                         `json:"duration_max,omitempty"`
	AspectRatios       []string                     `json:"aspect_ratios,omitempty"`
	InputModes         []string                     `json:"input_modes,omitempty"`
	ReferenceMinimums  *ConfigImportReferenceBounds `json:"reference_minimums,omitempty"`
	ReferenceLimits    *ConfigImportReferenceBounds `json:"reference_limits,omitempty"`
	SupportsRealPerson *bool                        `json:"supports_real_person,omitempty"`
	Priority           *int                         `json:"priority,omitempty"`
	Enabled            *bool                        `json:"enabled"`
}

type ConfigImportRouteBlueprint struct {
	ConfigImportAuthoritativeEntity
	CanonicalModel   string                     `json:"canonical_model"`
	ClientModel      string                     `json:"client_model"`
	ModelMappingRefs []string                   `json:"model_mapping_refs,omitempty"`
	MergeMode        ConfigImportRouteMergeMode `json:"merge_mode,omitempty"`
	Targets          []ConfigImportRouteTarget  `json:"targets"`
}

type ConfigImportSource struct {
	ConfigImportAuthoritativeEntity
	URL string `json:"url,omitempty"`
}

type ConfigImportUnresolvedVariant struct {
	ConfigImportAuthoritativeEntity
	LineRef         string   `json:"line_ref"`
	UpstreamModel   string   `json:"upstream_model,omitempty"`
	CostVariantKey  string   `json:"cost_variant_key,omitempty"`
	CostRuleRefs    []string `json:"cost_rule_refs,omitempty"`
	RouteTargetRefs []string `json:"route_target_refs,omitempty"`
	Reason          string   `json:"reason,omitempty"`
	Excluded        *bool    `json:"excluded,omitempty"`
}

type ConfigImportSourceIssue struct {
	Code      string                    `json:"code"`
	Severity  ConfigImportIssueSeverity `json:"severity"`
	Message   string                    `json:"message"`
	EntityRef string                    `json:"entity_ref,omitempty"`
	Sheet     string                    `json:"sheet,omitempty"`
	Row       *int                      `json:"row,omitempty"`
	Note      string                    `json:"note,omitempty"`
}
