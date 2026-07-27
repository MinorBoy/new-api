package dto

import "github.com/QuantumNous/new-api/types"

// ConfigImportUploadRequest carries a parsed, credential-free import document.
// Upload handlers that receive a file should validate it with
// service.ParseConfigImportDocument before constructing this DTO.
type ConfigImportUploadRequest struct {
	Document types.ConfigImportDocument `json:"document"`
}

type ConfigImportBindingRequest struct {
	Bindings []ConfigImportBindingInput `json:"bindings"`
}

type ConfigImportBindingInput struct {
	LineRef              string                          `json:"line_ref"`
	Action               types.ConfigImportBindingAction `json:"action"`
	ChannelID            *int                            `json:"channel_id,omitempty"`
	CredentialsConfirmed bool                            `json:"credentials_confirmed"`
	Reason               string                          `json:"reason,omitempty"`
}

type ConfigImportResolutionRequest struct {
	BatchRef    string                        `json:"batch_ref"`
	Resolutions []ConfigImportResolutionInput `json:"resolutions"`
}

type ConfigImportResolutionInput struct {
	ItemBusinessID string                             `json:"item_business_id"`
	Action         types.ConfigImportResolutionAction `json:"action"`
	LineRef        string                             `json:"line_ref,omitempty"`
	CostVariantKey string                             `json:"cost_variant_key,omitempty"`
	RouteTargetRef string                             `json:"route_target_ref,omitempty"`
	Reason         string                             `json:"reason,omitempty"`
}

type ConfigImportStageRequest struct {
	BatchRef            string   `json:"batch_ref"`
	ExcludedBusinessIDs []string `json:"excluded_business_ids,omitempty"`
}

type ConfigImportValidateRequest struct {
	BatchRef string `json:"batch_ref"`
}

type ConfigImportPublishRequest struct {
	BatchRef string `json:"batch_ref"`
}

type ConfigImportListRequest struct {
	Status types.ConfigImportBatchStatus `json:"status,omitempty"`
	Cursor string                        `json:"cursor,omitempty"`
	Limit  *int                          `json:"limit,omitempty"`
}

type ConfigImportDetailRequest struct {
	BatchRef string `json:"batch_ref"`
}

type ConfigImportBatchResponse struct {
	BatchRef        string                         `json:"batch_ref"`
	Status          types.ConfigImportBatchStatus  `json:"status"`
	SchemaVersion   int                            `json:"schema_version"`
	TemplateVersion string                         `json:"template_version"`
	ItemCounts      types.ConfigImportEntityCounts `json:"item_counts"`
	IssueCount      int                            `json:"issue_count"`
}

type ConfigImportUploadResponse struct {
	Batch ConfigImportBatchResponse `json:"batch"`
}

type ConfigImportBindingResponse struct {
	Batch ConfigImportBatchResponse `json:"batch"`
}

type ConfigImportResolutionResponse struct {
	Batch ConfigImportBatchResponse `json:"batch"`
}

type ConfigImportRouteReviewRequest struct {
	Reviews []ConfigImportRouteReviewInput `json:"reviews"`
}

type ConfigImportRouteReviewInput struct {
	ItemBusinessID string                           `json:"item_business_id"`
	MergeMode      types.ConfigImportRouteMergeMode `json:"merge_mode"`
}

type ConfigImportStageResponse struct {
	Batch ConfigImportBatchResponse `json:"batch"`
}

type ConfigImportValidateResponse struct {
	Batch  ConfigImportBatchResponse       `json:"batch"`
	Issues []types.ConfigImportSourceIssue `json:"issues"`
}

type ConfigImportPublishResponse struct {
	Batch ConfigImportBatchResponse `json:"batch"`
}

type ConfigImportListResponse struct {
	Items      []ConfigImportBatchResponse `json:"items"`
	NextCursor string                      `json:"next_cursor,omitempty"`
}

type ConfigImportDetailResponse struct {
	Batch    ConfigImportBatchResponse       `json:"batch"`
	Document types.ConfigImportDocument      `json:"document"`
	Issues   []types.ConfigImportSourceIssue `json:"issues"`
}

// ConfigImportBatchSummary is the normalized, credential-free metadata that
// can be listed without loading every persisted entity.
type ConfigImportBatchSummary struct {
	ID              int64                          `json:"id"`
	SchemaVersion   int                            `json:"schema_version"`
	TemplateVersion string                         `json:"template_version"`
	SourceSHA256    string                         `json:"source_sha256"`
	PayloadSHA256   string                         `json:"payload_sha256"`
	Status          types.ConfigImportBatchStatus  `json:"status"`
	CreatedBy       int                            `json:"created_by"`
	ItemCounts      types.ConfigImportEntityCounts `json:"item_counts"`
	IssueCount      int                            `json:"issue_count"`
	AllowedActions  []string                       `json:"allowed_actions"`
	CreatedAt       int64                          `json:"created_at"`
	UpdatedAt       int64                          `json:"updated_at"`
}

// ConfigImportItemDetail exposes a normalized authoritative entity. It does
// not contain the uploaded document body or any credential material.
type ConfigImportItemDetail struct {
	ID               int64                       `json:"id"`
	EntityType       string                      `json:"entity_type"`
	BusinessID       string                      `json:"business_id"`
	EntityHash       string                      `json:"entity_hash"`
	CanonicalJSON    string                      `json:"canonical_json"`
	State            types.ConfigImportItemState `json:"state"`
	SourceRef        string                      `json:"source_ref"`
	SourceSheet      string                      `json:"source_sheet"`
	SourceRow        *int                        `json:"source_row,omitempty"`
	MaterializedType string                      `json:"materialized_type,omitempty"`
	MaterializedID   *int                        `json:"materialized_id,omitempty"`
	ConflictReason   string                      `json:"conflict_reason,omitempty"`
	ExclusionReason  string                      `json:"exclusion_reason,omitempty"`
}

type ConfigImportIssueDetail struct {
	ID               int64                           `json:"id"`
	Severity         types.ConfigImportIssueSeverity `json:"severity"`
	Code             string                          `json:"code"`
	EntityType       string                          `json:"entity_type,omitempty"`
	BusinessID       string                          `json:"business_id,omitempty"`
	Sheet            string                          `json:"sheet,omitempty"`
	Row              *int                            `json:"row,omitempty"`
	Field            string                          `json:"field,omitempty"`
	Message          string                          `json:"message"`
	Suggestion       string                          `json:"suggestion,omitempty"`
	ResolutionStatus string                          `json:"resolution_status"`
}

// ConfigImportBatchDetail recovers a resumable batch from normalized rows. It
// deliberately omits the raw upload and its derived preview.
type ConfigImportBatchDetail struct {
	ConfigImportBatchSummary
	Items    []ConfigImportItemDetail    `json:"items"`
	Bindings []ConfigImportBindingDetail `json:"bindings"`
	Issues   []ConfigImportIssueDetail   `json:"issues"`
}

type ConfigImportBindingDetail struct {
	LineRef              string `json:"line_ref"`
	Action               string `json:"action"`
	ChannelID            *int   `json:"channel_id,omitempty"`
	CredentialsConfirmed bool   `json:"credentials_confirmed"`
}

type ConfigImportBatchPage struct {
	Items    []ConfigImportBatchSummary `json:"items"`
	Total    int64                      `json:"total"`
	Page     int                        `json:"page"`
	PageSize int                        `json:"page_size"`
}
