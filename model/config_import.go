package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type ConfigImportBaselineJSON string

func (ConfigImportBaselineJSON) GormDataType() string {
	return "config_import_baseline_json"
}

func (ConfigImportBaselineJSON) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	if db.Dialector.Name() == "mysql" {
		return "longtext"
	}
	return "text"
}

type ConfigImportSummaryJSON string

func (ConfigImportSummaryJSON) GormDataType() string {
	return "config_import_summary_json"
}

func (ConfigImportSummaryJSON) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	if db.Dialector.Name() == "mysql" {
		return "longtext"
	}
	return "text"
}

// ConfigImportBatch is the resumable record for one credential-free
// configuration import document.
type ConfigImportBatch struct {
	ID                int64                    `json:"id" gorm:"primaryKey"`
	SchemaVersion     int                      `json:"schema_version"`
	TemplateVersion   string                   `json:"template_version" gorm:"type:varchar(32)"`
	SourceSHA256      string                   `json:"source_sha256" gorm:"type:varchar(64);index"`
	PayloadSHA256     string                   `json:"payload_sha256" gorm:"type:varchar(64);index"`
	DeduplicationKey  *string                  `json:"-" gorm:"type:varchar(128);uniqueIndex"`
	CopiedFromBatchID *int64                   `json:"copied_from_batch_id,omitempty" gorm:"index"`
	Status            string                   `json:"status" gorm:"type:varchar(32);index"`
	CreatedBy         int                      `json:"created_by" gorm:"index"`
	SummaryJSON       ConfigImportSummaryJSON  `json:"summary_json"`
	BaselineJSON      ConfigImportBaselineJSON `json:"baseline_json"`
	FailureCode       string                   `json:"failure_code" gorm:"type:varchar(64)"`
	FailureMessage    string                   `json:"failure_message" gorm:"type:text"`
	ValidatedAt       *int64                   `json:"validated_at,omitempty"`
	PublishedAt       *int64                   `json:"published_at,omitempty"`
	ActivatedAt       *int64                   `json:"activated_at,omitempty"`
	FailedAt          *int64                   `json:"failed_at,omitempty"`
	CreatedAt         int64                    `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt         int64                    `json:"updated_at" gorm:"autoUpdateTime"`
}

func (ConfigImportBatch) TableName() string {
	return "config_import_batches"
}

func ConfigImportUploadDeduplicationKey(payloadSHA256 string) string {
	return "upload:" + payloadSHA256
}

// ConfigImportItem persists a normalized authoritative entity and its
// materialization state. CanonicalJSON is kept as text for all supported
// database engines.
type ConfigImportItem struct {
	ID               int64  `json:"id" gorm:"primaryKey"`
	BatchID          int64  `json:"batch_id" gorm:"not null;uniqueIndex:idx_config_import_item_business,priority:1;index"`
	EntityType       string `json:"entity_type" gorm:"type:varchar(64);not null;uniqueIndex:idx_config_import_item_business,priority:2"`
	BusinessID       string `json:"business_id" gorm:"type:varchar(191);not null;uniqueIndex:idx_config_import_item_business,priority:3"`
	EntityHash       string `json:"entity_hash" gorm:"type:varchar(64);index"`
	CanonicalJSON    string `json:"canonical_json" gorm:"type:text"`
	State            string `json:"state" gorm:"type:varchar(32);index"`
	SourceRef        string `json:"source_ref" gorm:"type:varchar(512)"`
	SourceSheet      string `json:"source_sheet" gorm:"type:varchar(255)"`
	SourceRow        *int   `json:"source_row,omitempty"`
	MaterializedType string `json:"materialized_type" gorm:"type:varchar(64)"`
	MaterializedID   *int   `json:"materialized_id,omitempty"`
	ConflictReason   string `json:"conflict_reason" gorm:"type:text"`
	ExclusionReason  string `json:"exclusion_reason" gorm:"type:text"`
	CreatedAt        int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt        int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (ConfigImportItem) TableName() string {
	return "config_import_items"
}

// ConfigImportBinding records a line-to-channel decision. Credentials are
// deliberately absent; only their confirmation metadata is retained.
type ConfigImportBinding struct {
	ID                     int64  `json:"id" gorm:"primaryKey"`
	BatchID                int64  `json:"batch_id" gorm:"not null;uniqueIndex:idx_config_import_binding_line,priority:1;index"`
	LineRef                string `json:"line_ref" gorm:"type:varchar(191);not null;uniqueIndex:idx_config_import_binding_line,priority:2"`
	Action                 string `json:"action" gorm:"type:varchar(32)"`
	ChannelID              *int   `json:"channel_id,omitempty" gorm:"index"`
	CredentialsConfirmedBy int    `json:"credentials_confirmed_by"`
	CredentialsConfirmedAt *int64 `json:"credentials_confirmed_at,omitempty"`
	SkipStateJSON          string `json:"-" gorm:"type:text"`
	CreatedAt              int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt              int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (ConfigImportBinding) TableName() string {
	return "config_import_bindings"
}

// ConfigImportIssue is the persisted validation result for an import batch.
type ConfigImportIssue struct {
	ID               int64  `json:"id" gorm:"primaryKey"`
	BatchID          int64  `json:"batch_id" gorm:"not null;index"`
	Severity         string `json:"severity" gorm:"type:varchar(16);index"`
	Code             string `json:"code" gorm:"type:varchar(64);index"`
	EntityType       string `json:"entity_type" gorm:"type:varchar(64)"`
	BusinessID       string `json:"business_id" gorm:"type:varchar(191)"`
	Sheet            string `json:"sheet" gorm:"type:varchar(255)"`
	Row              *int   `json:"row,omitempty"`
	Field            string `json:"field" gorm:"type:varchar(191)"`
	Message          string `json:"message" gorm:"type:text"`
	Suggestion       string `json:"suggestion" gorm:"type:text"`
	ResolutionStatus string `json:"resolution_status" gorm:"type:varchar(32);index"`
	CreatedAt        int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt        int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (ConfigImportIssue) TableName() string {
	return "config_import_issues"
}

// ConfigImportResolution retains an administrator's normalized conflict
// decision without depending on database-native JSON features.
type ConfigImportResolution struct {
	ID             int64  `json:"id" gorm:"primaryKey"`
	BatchID        int64  `json:"batch_id" gorm:"not null;index"`
	ItemBusinessID string `json:"item_business_id" gorm:"type:varchar(191);index"`
	Action         string `json:"action" gorm:"type:varchar(32)"`
	DecisionJSON   string `json:"decision_json" gorm:"type:text"`
	CreatedBy      int    `json:"created_by" gorm:"index"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (ConfigImportResolution) TableName() string {
	return "config_import_resolutions"
}

// ConfigImportPublishAudit is an append-only record of a publish attempt and
// the configuration hashes it transitioned between.
type ConfigImportPublishAudit struct {
	ID           int64  `json:"id" gorm:"primaryKey"`
	BatchID      int64  `json:"batch_id" gorm:"not null;index"`
	AdminID      int    `json:"admin_id" gorm:"index"`
	BeforeSHA256 string `json:"before_sha256" gorm:"type:varchar(64)"`
	AfterSHA256  string `json:"after_sha256" gorm:"type:varchar(64)"`
	Outcome      string `json:"outcome" gorm:"type:varchar(64)"`
	CreatedAt    int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (ConfigImportPublishAudit) TableName() string {
	return "config_import_publish_audits"
}

type ConfigImportActivationAudit struct {
	ID                 int64                   `json:"id" gorm:"primaryKey"`
	BatchID            int64                   `json:"batch_id" gorm:"not null;index"`
	AdminID            int                     `json:"admin_id" gorm:"index"`
	Outcome            string                  `json:"outcome" gorm:"type:varchar(64);index"`
	ChannelCount       int                     `json:"channel_count"`
	PolicyCount        int                     `json:"policy_count"`
	TargetCount        int                     `json:"target_count"`
	RetiredTargetCount int                     `json:"retired_target_count"`
	BeforeSHA256       string                  `json:"before_sha256" gorm:"type:varchar(64);not null"`
	AfterSHA256        string                  `json:"after_sha256" gorm:"type:varchar(64);not null"`
	FailureCode        string                  `json:"failure_code" gorm:"type:varchar(64)"`
	FailureMessage     string                  `json:"failure_message" gorm:"type:text"`
	SummaryJSON        ConfigImportSummaryJSON `json:"summary_json"`
	CreatedAt          int64                   `json:"created_at" gorm:"autoCreateTime"`
}

func (ConfigImportActivationAudit) TableName() string {
	return "config_import_activation_audits"
}

type ConfigImportRouteOwnershipChange struct {
	ID                     int64  `json:"id" gorm:"primaryKey"`
	OperationID            string `json:"operation_id" gorm:"type:varchar(64);not null;uniqueIndex:idx_route_ownership_change,priority:1;index"`
	RouteTargetID          int    `json:"route_target_id" gorm:"not null;uniqueIndex:idx_route_ownership_change,priority:2;index"`
	PreviousManagedBy      string `json:"previous_managed_by" gorm:"type:varchar(32);not null"`
	PreviousSourceBatchID  *int64 `json:"previous_source_batch_id,omitempty"`
	AssignedBatchID        int64  `json:"assigned_batch_id" gorm:"not null;index"`
	AppliedTargetUpdatedAt int64  `json:"applied_target_updated_at"`
	AppliedTargetSHA256    string `json:"applied_target_sha256" gorm:"type:varchar(64);not null"`
	AppliedBy              int    `json:"applied_by" gorm:"index"`
	RevertedBy             int    `json:"reverted_by" gorm:"index"`
	RevertedAt             *int64 `json:"reverted_at,omitempty"`
	CreatedAt              int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (ConfigImportRouteOwnershipChange) TableName() string {
	return "config_import_route_ownership_changes"
}

// UpdateConfigImportBatchStatus atomically advances a batch only when its
// current status equals from. Callers provide the surrounding transaction so
// related batch updates can commit with the state transition.
func UpdateConfigImportBatchStatus(
	tx *gorm.DB,
	id int64,
	from, to types.ConfigImportBatchStatus,
) (bool, error) {
	if tx == nil {
		return false, fmt.Errorf("config import batch transaction is required")
	}

	var batch ConfigImportBatch
	if err := lockForUpdate(tx).Select("id", "status").Where("id = ?", id).First(&batch).Error; err != nil {
		return false, err
	}
	if batch.Status != string(from) {
		return false, nil
	}

	result := tx.Model(&ConfigImportBatch{}).
		Where("id = ? AND status = ?", id, string(from)).
		Updates(map[string]any{
			"status":     string(to),
			"updated_at": common.GetTimestamp(),
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}
