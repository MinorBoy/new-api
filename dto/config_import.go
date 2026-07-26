package dto

import "github.com/QuantumNous/new-api/types"

// ConfigImportUploadRequest carries a parsed, credential-free import document.
// Upload handlers that receive a file should validate it with
// service.ParseConfigImportDocument before constructing this DTO.
type ConfigImportUploadRequest struct {
	Document types.ConfigImportDocument `json:"document"`
}

type ConfigImportBindingRequest struct {
	BatchRef string                     `json:"batch_ref"`
	Bindings []ConfigImportBindingInput `json:"bindings"`
}

type ConfigImportBindingInput struct {
	ItemBusinessID   string                          `json:"item_business_id"`
	Action           types.ConfigImportBindingAction `json:"action"`
	TargetBusinessID string                          `json:"target_business_id,omitempty"`
}

type ConfigImportResolutionRequest struct {
	BatchRef    string                        `json:"batch_ref"`
	Resolutions []ConfigImportResolutionInput `json:"resolutions"`
}

type ConfigImportResolutionInput struct {
	ItemBusinessID string                             `json:"item_business_id"`
	Action         types.ConfigImportResolutionAction `json:"action"`
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
