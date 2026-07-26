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
)

type ConfigImportRouteMergeMode string

const (
	ConfigImportRouteMergeModeReplace ConfigImportRouteMergeMode = "replace"
	ConfigImportRouteMergeModeAppend  ConfigImportRouteMergeMode = "append"
	ConfigImportRouteMergeModeMerge   ConfigImportRouteMergeMode = "merge"
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
	SourceFile       string                   `json:"source_file"`
	SourceSHA256     string                   `json:"source_sha256"`
	PayloadSHA256    string                   `json:"payload_sha256"`
	GeneratedAt      string                   `json:"generated_at"`
	ConverterVersion string                   `json:"converter_version"`
	TemplateMatch    string                   `json:"template_match"`
	Counts           ConfigImportEntityCounts `json:"counts"`
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
	ChannelRef string  `json:"channel_ref"`
	LineKey    string  `json:"line_key,omitempty"`
	BaseURL    string  `json:"base_url,omitempty"`
	Weight     *string `json:"weight,omitempty"`
	Enabled    *bool   `json:"enabled,omitempty"`
}

type ConfigImportModelSKU struct {
	ConfigImportAuthoritativeEntity
	ChannelLineRef string   `json:"channel_line_ref"`
	UpstreamModel  string   `json:"upstream_model,omitempty"`
	Capabilities   []string `json:"capabilities,omitempty"`
	Enabled        *bool    `json:"enabled,omitempty"`
}

type ConfigImportSaleProposal struct {
	ConfigImportAuthoritativeEntity
	ModelSKURef  string  `json:"model_sku_ref"`
	Currency     string  `json:"currency,omitempty"`
	UnitPrice    *string `json:"unit_price,omitempty"`
	PricePerUnit *string `json:"price_per_unit,omitempty"`
	MarginRatio  *string `json:"margin_ratio,omitempty"`
	Enabled      *bool   `json:"enabled,omitempty"`
}

type ConfigImportCostRuleDraft struct {
	ConfigImportAuthoritativeEntity
	ChannelLineRef       string  `json:"channel_line_ref"`
	ModelSKURef          string  `json:"model_sku_ref"`
	CostMode             string  `json:"cost_mode,omitempty"`
	Currency             string  `json:"currency,omitempty"`
	UnitPrice            *string `json:"unit_price,omitempty"`
	PricePerSecond       *string `json:"price_per_second,omitempty"`
	InputPerMillion      *string `json:"input_per_million,omitempty"`
	OutputPerMillion     *string `json:"output_per_million,omitempty"`
	CompletionPerMillion *string `json:"completion_per_million,omitempty"`
	TotalPerMillion      *string `json:"total_per_million,omitempty"`
	BillingMultiplier    *string `json:"billing_multiplier,omitempty"`
	FeeRate              *string `json:"fee_rate,omitempty"`
	Enabled              *bool   `json:"enabled,omitempty"`
}

type ConfigImportModelMapping struct {
	ConfigImportAuthoritativeEntity
	ChannelRef  string `json:"channel_ref"`
	ModelSKURef string `json:"model_sku_ref"`
	PublicModel string `json:"public_model,omitempty"`
	Enabled     *bool  `json:"enabled,omitempty"`
}

type ConfigImportRouteTarget struct {
	ChannelLineRef string  `json:"channel_line_ref"`
	ModelSKURef    string  `json:"model_sku_ref"`
	Priority       *int    `json:"priority,omitempty"`
	Weight         *string `json:"weight,omitempty"`
	Enabled        *bool   `json:"enabled,omitempty"`
}

type ConfigImportRouteBlueprint struct {
	ConfigImportAuthoritativeEntity
	PublicModel string                     `json:"public_model,omitempty"`
	MergeMode   ConfigImportRouteMergeMode `json:"merge_mode,omitempty"`
	Targets     []ConfigImportRouteTarget  `json:"targets,omitempty"`
	Enabled     *bool                      `json:"enabled,omitempty"`
}

type ConfigImportSource struct {
	ConfigImportAuthoritativeEntity
	URL string `json:"url,omitempty"`
}

type ConfigImportUnresolvedVariant struct {
	ConfigImportAuthoritativeEntity
	ChannelRef  string `json:"channel_ref"`
	ModelSKURef string `json:"model_sku_ref,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Excluded    *bool  `json:"excluded,omitempty"`
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
