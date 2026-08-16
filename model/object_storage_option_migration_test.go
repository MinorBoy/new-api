package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	object_storage "github.com/QuantumNous/new-api/setting/object_storage"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateLegacyObjectStorageTransferDefaultsPersistsEnabledDefault(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))

	previousDB := DB
	previousOptions := common.OptionMap
	configValue := config.GlobalConfig.Get(object_storage.ConfigName)
	require.NotNil(t, configValue)
	previousConfig, err := config.ConfigToMap(configValue)
	require.NoError(t, err)
	DB = db
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		DB = previousDB
		common.OptionMap = previousOptions
		require.NoError(t, config.UpdateConfigFromMap(configValue, previousConfig))
		object_storage.UpdateAndSync()
	})

	legacyOptions := map[string]string{
		"enabled":                      "true",
		"transfer_domain_whitelist":    `[]`,
		"no_transfer_domain_blacklist": `["official.example.com"]`,
	}
	for key, value := range legacyOptions {
		require.NoError(t, db.Create(&Option{Key: object_storage.ConfigName + "." + key, Value: value}).Error)
	}
	require.NoError(t, config.UpdateConfigFromMap(configValue, legacyOptions))
	object_storage.UpdateAndSync()

	migrateLegacyObjectStorageTransferDefaults(legacyOptions)

	var migrated Option
	require.NoError(t, db.First(&migrated, "key = ?", object_storage.ConfigName+".rules_default_transfer").Error)
	assert.Equal(t, "true", migrated.Value)
	assert.True(t, object_storage.Runtime().RulesDefaultTransfer)
	assert.Equal(t, "true", common.OptionMap[object_storage.ConfigName+".rules_default_transfer"])
}
