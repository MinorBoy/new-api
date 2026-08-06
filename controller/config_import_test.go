package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestConfigImportActivateReturnsBatchDetailAndIsIdempotent(t *testing.T) {
	prepareConfigImportControllerTest(t)
	batch := createPublishedConfigImportControllerBatch(t)

	firstRecorder, firstContext := configImportControllerContext(http.MethodPost, batch.ID)
	ActivateConfigImportBatch(firstContext)

	require.Equal(t, http.StatusOK, firstRecorder.Code)
	var firstResponse struct {
		Success bool                        `json:"success"`
		Data    dto.ConfigImportBatchDetail `json:"data"`
	}
	require.NoError(t, common.Unmarshal(firstRecorder.Body.Bytes(), &firstResponse))
	assert.True(t, firstResponse.Success)
	assert.Equal(t, batch.ID, firstResponse.Data.ID)
	assert.Equal(t, types.ConfigImportBatchStatusPublished, firstResponse.Data.Status)
	require.NotNil(t, firstResponse.Data.ActivatedAt)
	assert.Equal(t, []string{"copy_for_binding"}, firstResponse.Data.AllowedActions)
	assert.Nil(t, firstResponse.Data.ActivationPreview)

	secondRecorder, secondContext := configImportControllerContext(http.MethodPost, batch.ID)
	ActivateConfigImportBatch(secondContext)

	require.Equal(t, http.StatusOK, secondRecorder.Code)
	var auditCount int64
	require.NoError(t, model.DB.Model(&model.ConfigImportActivationAudit{}).
		Where("batch_id = ? AND outcome = ?", batch.ID, "activated").Count(&auditCount).Error)
	assert.Equal(t, int64(1), auditCount)
}

func TestConfigImportCopyForBindingReturnsFreshBatchDetail(t *testing.T) {
	prepareConfigImportControllerTest(t)
	source := createPublishedConfigImportControllerBatch(t)
	require.NoError(t, model.DB.Create(&model.ConfigImportItem{
		BatchID: source.ID, EntityType: "sources", BusinessID: "source-a", EntityHash: "hash-a",
		CanonicalJSON: `{"business_id":"source-a"}`, State: string(types.ConfigImportItemStateChanged), SourceRef: "source-a",
	}).Error)

	recorder, context := configImportControllerContext(http.MethodPost, source.ID)
	context.Set("id", 99)

	CopyConfigImportBatchForBinding(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool                        `json:"success"`
		Data    dto.ConfigImportBatchDetail `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, types.ConfigImportBatchStatusBinding, response.Data.Status)
	require.NotNil(t, response.Data.CopiedFromBatchID)
	assert.Equal(t, source.ID, *response.Data.CopiedFromBatchID)
	assert.Equal(t, 99, response.Data.CreatedBy)
	assert.Empty(t, response.Data.Bindings)
	require.Len(t, response.Data.Items, 1)
	assert.Equal(t, types.ConfigImportItemStateNew, response.Data.Items[0].State)
}

func TestConfigImportCopyForBindingRejectsUnpublishedBatch(t *testing.T) {
	prepareConfigImportControllerTest(t)
	source := createPublishedConfigImportControllerBatch(t)
	require.NoError(t, model.DB.Model(&model.ConfigImportBatch{}).Where("id = ?", source.ID).
		Update("status", string(types.ConfigImportBatchStatusBinding)).Error)

	recorder, context := configImportControllerContext(http.MethodPost, source.ID)
	context.Set("id", 99)

	CopyConfigImportBatchForBinding(context)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	var response struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Equal(t, "COPY_FOR_BINDING_SOURCE_STATUS", response.Code)
}

func TestConfigImportActivateReturnsStructuredBlockers(t *testing.T) {
	prepareConfigImportControllerTest(t)
	batch := createPublishedConfigImportControllerBatch(t)
	require.NoError(t, model.DB.Create(&model.ConfigImportIssue{
		BatchID: batch.ID, Severity: string(types.ConfigImportIssueSeverityWarning),
		Code: "REVIEW_REQUIRED", Message: "review required", ResolutionStatus: "open",
	}).Error)

	recorder, context := configImportControllerContext(http.MethodPost, batch.ID)
	ActivateConfigImportBatch(context)

	require.Equal(t, http.StatusConflict, recorder.Code)
	var response map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "ACTIVATION_BLOCKED", response["code"])
	data, ok := response["data"].(map[string]any)
	require.True(t, ok)
	require.Len(t, data, 1)
	blockers, ok := data["blockers"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, blockers)
	firstBlocker, ok := blockers[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "ACTIVATION_OPEN_ISSUES", firstBlocker["code"])
}

func TestConfigImportActivateRejectsInvalidBatchID(t *testing.T) {
	recorder, context := configImportControllerContext(http.MethodPost, 0)
	context.Params = gin.Params{{Key: "id", Value: "invalid"}}

	ActivateConfigImportBatch(context)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	var response map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "SCHEMA_BATCH_ID", response["code"])
	assert.Equal(t, "invalid config import batch id", response["message"])
}

func TestConfigImportActivationCacheFailureIsRecoverableFromBatchDetail(t *testing.T) {
	prepareConfigImportControllerTest(t)
	batch := createPublishedConfigImportControllerBatch(t)
	activatedAt := int64(123)
	require.NoError(t, model.DB.Model(&model.ConfigImportBatch{}).Where("id = ?", batch.ID).
		Update("activated_at", activatedAt).Error)
	require.NoError(t, model.DB.Create(&model.ConfigImportIssue{
		BatchID: batch.ID, Severity: string(types.ConfigImportIssueSeverityWarning),
		Code: "ACTIVATION_CACHE_REFRESH_PENDING", Message: "cache refresh pending", ResolutionStatus: "open",
	}).Error)

	errorRecorder, errorContext := configImportControllerContext(http.MethodPost, batch.ID)
	writeConfigImportError(errorContext, &service.ConfigImportSchemaError{
		Code: "ACTIVATION_CACHE_REFRESH_PENDING", Message: "activation committed but cache refresh is pending",
		Data: map[string]any{"batch_id": batch.ID, "activated": true},
	})

	require.Equal(t, http.StatusServiceUnavailable, errorRecorder.Code)
	var errorResponse map[string]any
	require.NoError(t, common.Unmarshal(errorRecorder.Body.Bytes(), &errorResponse))
	assert.Equal(t, "ACTIVATION_CACHE_REFRESH_PENDING", errorResponse["code"])
	assert.Equal(t, map[string]any{"batch_id": float64(batch.ID), "activated": true}, errorResponse["data"])

	getRecorder, getContext := configImportControllerContext(http.MethodGet, batch.ID)
	GetConfigImportBatch(getContext)

	require.Equal(t, http.StatusOK, getRecorder.Code)
	var getResponse struct {
		Success bool                        `json:"success"`
		Data    dto.ConfigImportBatchDetail `json:"data"`
	}
	require.NoError(t, common.Unmarshal(getRecorder.Body.Bytes(), &getResponse))
	require.NotNil(t, getResponse.Data.ActivatedAt)
	assert.Equal(t, activatedAt, *getResponse.Data.ActivatedAt)
	assert.Equal(t, []string{"refresh_cache", "copy_for_binding"}, getResponse.Data.AllowedActions)
}

func prepareConfigImportControllerTest(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	previousDB := model.DB
	previousMainType := common.MainDatabaseType()
	previousLogType := common.LogDatabaseType()
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, previousLogType)
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		require.NoError(t, sqlDB.Close())
	})

	require.NoError(t, db.AutoMigrate(
		&model.ConfigImportBatch{}, &model.ConfigImportItem{}, &model.ConfigImportBinding{},
		&model.ConfigImportIssue{}, &model.ConfigImportResolution{}, &model.ConfigImportPublishAudit{},
		&model.ConfigImportActivationAudit{}, &model.Channel{}, &model.Ability{},
		&model.ChannelModelCostRule{},
	))
}

func createPublishedConfigImportControllerBatch(t *testing.T) model.ConfigImportBatch {
	t.Helper()
	summary, err := common.Marshal(map[string]any{"item_counts": types.ConfigImportEntityCounts{}})
	require.NoError(t, err)
	batch := model.ConfigImportBatch{
		SchemaVersion: 1, TemplateVersion: "1", SourceSHA256: strings.Repeat("a", 64),
		PayloadSHA256: strings.Repeat("b", 64), Status: string(types.ConfigImportBatchStatusPublished),
		CreatedBy: 42, SummaryJSON: model.ConfigImportSummaryJSON(summary), BaselineJSON: "{}",
	}
	require.NoError(t, model.DB.Create(&batch).Error)
	baseline, err := service.CaptureConfigImportBaseline(model.DB, batch.ID)
	require.NoError(t, err)
	encoded, err := common.Marshal(baseline)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.ConfigImportBatch{}).Where("id = ?", batch.ID).
		Update("baseline_json", string(encoded)).Error)
	return batch
}

func configImportControllerContext(method string, batchID int64) (*httptest.ResponseRecorder, *gin.Context) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(method, "/api/config-import/batches/"+strconv.FormatInt(batchID, 10), nil)
	context.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(batchID, 10)}}
	context.Set("id", 42)
	return recorder, context
}
