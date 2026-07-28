package service

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func TestCaptureConfigImportBaselineLoadsOptionRowsOnMySQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		NamingStrategy: schema.NamingStrategy{
			TablePrefix: fmt.Sprintf("cib_%x_", time.Now().UnixNano()),
		},
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	t.Cleanup(func() {
		require.NoError(t, db.Migrator().DropTable(
			&model.ChannelModelCostRule{},
			&model.Option{},
			&model.ConfigImportBinding{},
			&model.ConfigImportItem{},
			&model.ConfigImportBatch{},
		))
	})
	require.NoError(t, db.AutoMigrate(
		&model.ConfigImportBatch{},
		&model.ConfigImportItem{},
		&model.ConfigImportBinding{},
		&model.Option{},
		&model.ChannelModelCostRule{},
	))

	batch := model.ConfigImportBatch{
		SchemaVersion:   1,
		TemplateVersion: "1",
		SourceSHA256:    "source",
		PayloadSHA256:   "payload",
		Status:          "staged",
		CreatedBy:       1,
	}
	require.NoError(t, db.Create(&batch).Error)
	staged := struct {
		StagedProposal struct {
			OptionPatches map[string]map[string]any `json:"option_patches"`
		} `json:"staged_proposal"`
	}{}
	staged.StagedProposal.OptionPatches = map[string]map[string]any{
		"billing_setting.billing_expr": {"canonical-video": "expr"},
	}
	encoded, err := common.Marshal(staged)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.ConfigImportItem{
		BatchID:       batch.ID,
		EntityType:    "sale_proposals",
		BusinessID:    "sale-a",
		CanonicalJSON: string(encoded),
		State:         "new",
	}).Error)
	require.NoError(t, db.Create(&model.Option{
		Key:   "billing_setting.billing_expr",
		Value: `{"canonical-video":"existing"}`,
	}).Error)

	baseline, err := CaptureConfigImportBaseline(db, batch.ID)
	require.NoError(t, err)
	assert.Contains(t, baseline.Options, "billing_setting.billing_expr|canonical-video")
}

func TestPublishConfigImportSaleOptionsLoadsOptionRowsOnMySQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		NamingStrategy: schema.NamingStrategy{
			TablePrefix: fmt.Sprintf("cip_%x_", time.Now().UnixNano()),
		},
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(&model.Option{})) })
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	require.NoError(t, db.Create(&model.Option{
		Key:   "ModelPrice",
		Value: `{"existing-model":1}`,
	}).Error)

	document := map[string]any{
		"staged_proposal": map[string]any{
			"option_patches": map[string]any{
				"ModelPrice": map[string]any{"canonical-video": 1.2},
			},
		},
	}
	encoded, err := common.Marshal(document)
	require.NoError(t, err)
	refresh := ConfigImportRefreshKeys{}
	tx := db.Begin()
	require.NoError(t, tx.Error)
	require.NoError(t, publishConfigImportSaleOptions(tx, []model.ConfigImportItem{{
		EntityType:    "sale_proposals",
		BusinessID:    "sale-a",
		CanonicalJSON: string(encoded),
		State:         "new",
	}}, &refresh))
	require.NoError(t, tx.Commit().Error)

	var option model.Option
	require.NoError(t, db.Where(clause.Eq{
		Column: clause.Column{Name: "key"},
		Value:  "ModelPrice",
	}).First(&option).Error)
	assert.JSONEq(t, `{"canonical-video":1.2,"existing-model":1}`, option.Value)
	assert.Equal(t, []string{"ModelPrice"}, refresh.OptionKeys)
}

func TestRefreshPublishedConfigLoadsOptionRowsOnMySQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		NamingStrategy: schema.NamingStrategy{
			TablePrefix: fmt.Sprintf("cir_%x_", time.Now().UnixNano()),
		},
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(&model.Option{})) })
	require.NoError(t, db.AutoMigrate(&model.Option{}))

	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	common.OptionMapRWMutex.Lock()
	previousOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	const key = "config_import_refresh_mysql_test"
	const value = "refreshed"
	require.NoError(t, db.Create(&model.Option{Key: key, Value: value}).Error)

	require.NoError(t, RefreshPublishedConfig(ConfigImportRefreshKeys{
		OptionKeys: []string{key},
	}))
	common.OptionMapRWMutex.RLock()
	refreshed := common.OptionMap[key]
	common.OptionMapRWMutex.RUnlock()
	assert.Equal(t, value, refreshed)
}
