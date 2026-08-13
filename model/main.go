package model

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/types"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/clickhouse"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var commonGroupCol string
var commonKeyCol string
var commonTrueVal string
var commonFalseVal string

var logKeyCol string
var logGroupCol string

func init() {
	initCol()
}

func initCol() {
	// init common column names
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		commonGroupCol = `"group"`
		commonKeyCol = `"key"`
		commonTrueVal = "true"
		commonFalseVal = "false"
	} else {
		commonGroupCol = "`group`"
		commonKeyCol = "`key`"
		commonTrueVal = "1"
		commonFalseVal = "0"
	}
	switch common.LogDatabaseType() {
	case common.DatabaseTypePostgreSQL:
		logGroupCol = `"group"`
		logKeyCol = `"key"`
	default:
		logGroupCol = "`group`"
		logKeyCol = "`key`"
	}
}

var DB *gorm.DB

var LOG_DB *gorm.DB

func createRootAccountIfNeed() error {
	var user User
	//if user.Status != common.UserStatusEnabled {
	if err := DB.First(&user).Error; err != nil {
		common.SysLog("no user exists, create a root user for you: username is root, password is 123456")
		hashedPassword, err := common.Password2Hash("123456")
		if err != nil {
			return err
		}
		rootUser := User{
			Username:    "root",
			Password:    hashedPassword,
			Role:        common.RoleRootUser,
			Status:      common.UserStatusEnabled,
			DisplayName: "Root User",
			AccessToken: nil,
			Quota:       100000000,
		}
		DB.Create(&rootUser)
	}
	return nil
}

func CheckSetup() {
	setup := GetSetup()
	if setup == nil {
		// No setup record exists, check if we have a root user
		if RootUserExists() {
			common.SysLog("system is not initialized, but root user exists")
			// Create setup record
			newSetup := Setup{
				Version:       common.Version,
				InitializedAt: time.Now().Unix(),
			}
			err := DB.Create(&newSetup).Error
			if err != nil {
				common.SysLog("failed to create setup record: " + err.Error())
			}
			constant.Setup = true
		} else {
			common.SysLog("system is not initialized and no root user exists")
			constant.Setup = false
		}
	} else {
		// Setup record exists, system is initialized
		common.SysLog("system is already initialized at: " + time.Unix(setup.InitializedAt, 0).String())
		constant.Setup = true
	}
}

func isClickHouseDSN(dsn string) bool {
	return strings.HasPrefix(dsn, "clickhouse://") ||
		strings.HasPrefix(dsn, "tcp://") ||
		strings.HasPrefix(dsn, "http://") ||
		strings.HasPrefix(dsn, "https://")
}

func normalizeClickHouseDSN(dsn string) string {
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Scheme != "https" {
		return dsn
	}
	query := parsed.Query()
	if _, ok := query["secure"]; !ok {
		query.Set("secure", "true")
		parsed.RawQuery = query.Encode()
	}
	return parsed.String()
}

func chooseDB(envName string, isLog bool) (*gorm.DB, common.DatabaseType, error) {
	dsn := os.Getenv(envName)
	if dsn != "" {
		if isClickHouseDSN(dsn) {
			if !isLog {
				return nil, "", fmt.Errorf("%s does not support ClickHouse; use SQLite, MySQL, or PostgreSQL for the primary database and LOG_SQL_DSN for ClickHouse logs", envName)
			}
			common.SysLog("using ClickHouse as log database")
			db, err := gorm.Open(clickhouse.Open(normalizeClickHouseDSN(dsn)), &gorm.Config{
				PrepareStmt: false,
			})
			return db, common.DatabaseTypeClickHouse, err
		}
		if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
			// Use PostgreSQL
			common.SysLog("using PostgreSQL as database")
			db, err := gorm.Open(postgres.New(postgres.Config{
				DSN:                  dsn,
				PreferSimpleProtocol: true, // disables implicit prepared statement usage
			}), &gorm.Config{
				PrepareStmt: true, // precompile SQL
			})
			return db, common.DatabaseTypePostgreSQL, err
		}
		if strings.HasPrefix(dsn, "local") {
			common.SysLog("SQL_DSN not set, using SQLite as database")
			db, err := gorm.Open(sqlite.Open(common.SQLitePath), &gorm.Config{
				PrepareStmt: true, // precompile SQL
			})
			return db, common.DatabaseTypeSQLite, err
		}
		// Use MySQL
		common.SysLog("using MySQL as database")
		// check parseTime
		if !strings.Contains(dsn, "parseTime") {
			if strings.Contains(dsn, "?") {
				dsn += "&parseTime=true"
			} else {
				dsn += "?parseTime=true"
			}
		}
		db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
			PrepareStmt: true, // precompile SQL
		})
		return db, common.DatabaseTypeMySQL, err
	}
	// Use SQLite
	common.SysLog("SQL_DSN not set, using SQLite as database")
	db, err := gorm.Open(sqlite.Open(common.SQLitePath), &gorm.Config{
		PrepareStmt: true, // precompile SQL
	})
	return db, common.DatabaseTypeSQLite, err
}

func InitDB() (err error) {
	db, dbType, err := chooseDB("SQL_DSN", false)
	if err == nil {
		common.SetMainDatabaseType(dbType)
		if os.Getenv("LOG_SQL_DSN") == "" {
			common.SetLogDatabaseType(dbType)
		}
		initCol()
		if common.DebugEnabled {
			db = db.Debug()
		}
		DB = db
		// MySQL charset/collation startup check: ensure Chinese-capable charset
		if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
			if err := checkMySQLChineseSupport(DB); err != nil {
				panic(err)
			}
		}
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		sqlDB.SetMaxIdleConns(common.GetEnvOrDefault("SQL_MAX_IDLE_CONNS", 100))
		sqlDB.SetMaxOpenConns(common.GetEnvOrDefault("SQL_MAX_OPEN_CONNS", 1000))
		sqlDB.SetConnMaxLifetime(time.Second * time.Duration(common.GetEnvOrDefault("SQL_MAX_LIFETIME", 60)))

		if !common.IsMasterNode {
			return nil
		}
		if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
			//_, _ = sqlDB.Exec("ALTER TABLE channels MODIFY model_mapping TEXT;") // TODO: delete this line when most users have upgraded
		}
		common.SysLog("database migration started")
		err = migrateDB()
		return err
	} else {
		common.FatalLog(err)
	}
	return err
}

func InitLogDB() (err error) {
	if os.Getenv("LOG_SQL_DSN") == "" {
		LOG_DB = DB
		common.SetLogDatabaseType(common.MainDatabaseType())
		initCol()
		return
	}
	db, dbType, err := chooseDB("LOG_SQL_DSN", true)
	if err == nil {
		common.SetLogDatabaseType(dbType)
		initCol()
		if common.DebugEnabled {
			db = db.Debug()
		}
		LOG_DB = db
		// If log DB is MySQL, also ensure Chinese-capable charset
		if common.UsingLogDatabase(common.DatabaseTypeMySQL) {
			if err := checkMySQLChineseSupport(LOG_DB); err != nil {
				panic(err)
			}
		}
		sqlDB, err := LOG_DB.DB()
		if err != nil {
			return err
		}
		sqlDB.SetMaxIdleConns(common.GetEnvOrDefault("SQL_MAX_IDLE_CONNS", 100))
		sqlDB.SetMaxOpenConns(common.GetEnvOrDefault("SQL_MAX_OPEN_CONNS", 1000))
		sqlDB.SetConnMaxLifetime(time.Second * time.Duration(common.GetEnvOrDefault("SQL_MAX_LIFETIME", 60)))

		if !common.IsMasterNode {
			return nil
		}
		common.SysLog("database migration started")
		err = migrateLOGDB()
		return err
	} else {
		common.FatalLog(err)
	}
	return err
}

func migrateDB() error {
	if err := migrateConfigImportBindingChannelIndex(); err != nil {
		return err
	}
	if err := migrateConfigImportBatchIdentity(); err != nil {
		return err
	}
	// Extend cost rule and route target business keys with cost_variant_key
	// before AutoMigrate runs, so the new column is present, backfilled, and
	// the old three-column unique index is replaced by the four-column one.
	if err := migrateCostVariantKeys(); err != nil {
		return err
	}
	if err := migrateRouteTargetOwnershipColumns(); err != nil {
		return err
	}
	// Migrate price_amount column from float/double to decimal for existing tables
	migrateSubscriptionPlanPriceAmount()
	// Migrate model_limits column from varchar to text for existing tables
	if err := migrateTokenModelLimitsToText(); err != nil {
		return err
	}

	err := DB.AutoMigrate(
		&Channel{},
		&RoutingPolicy{},
		&RouteTarget{},
		&Token{},
		&User{},
		&UserSession{},
		&AuthFlow{},
		&ExternalIdentityClaim{},
		&PasskeyCredential{},
		&Option{},
		&Redemption{},
		&Ability{},
		&Log{},
		&Midjourney{},
		&TopUp{},
		&QuotaData{},
		&Task{},
		&Model{},
		&Vendor{},
		&PrefillGroup{},
		&Setup{},
		&TwoFA{},
		&TwoFABackupCode{},
		&Checkin{},
		&SubscriptionOrder{},
		&UserSubscription{},
		&SubscriptionPreConsumeRecord{},
		&CustomOAuthProvider{},
		&UserOAuthBinding{},
		&PerfMetric{},
		&SystemInstance{},
		&SystemTask{},
		&SystemTaskLock{},
		&CasbinRule{},
		&AuthzRole{},
		&ChannelModelCostRule{},
		&CostAccountingRequest{},
		&CostAccountingAttempt{},
		&CostAccountingAudit{},
		&ConfigImportBatch{},
		&ConfigImportItem{},
		&ConfigImportBinding{},
		&ConfigImportIssue{},
		&ConfigImportResolution{},
		&ConfigImportPublishAudit{},
		&ConfigImportActivationAudit{},
		&ConfigImportRouteOwnershipChange{},
		&Asset{},
		&AssetProviderBinding{},
	)
	if err != nil {
		return err
	}
	if err := migrateChannelTypeIDs(); err != nil {
		return err
	}
	if err := InitializeUserAuthVersions(); err != nil {
		return err
	}
	if err := InitializeExternalIdentityClaims(); err != nil {
		return err
	}
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		if err := ensureSubscriptionPlanTableSQLite(); err != nil {
			return err
		}
	} else {
		if err := DB.AutoMigrate(&SubscriptionPlan{}); err != nil {
			return err
		}
	}
	return nil
}

func migrateDBFast() error {
	if err := migrateConfigImportBindingChannelIndex(); err != nil {
		return err
	}
	if err := migrateConfigImportBatchIdentity(); err != nil {
		return err
	}

	// Extend cost rule and route target business keys with cost_variant_key
	// before AutoMigrate runs. The fast path still needs this because it
	// skips the linear ordering and would otherwise create the columns with
	// the wrong index shape.
	if err := migrateCostVariantKeys(); err != nil {
		return err
	}
	if err := migrateRouteTargetOwnershipColumns(); err != nil {
		return err
	}

	var wg sync.WaitGroup

	migrations := []struct {
		model interface{}
		name  string
	}{
		{&Channel{}, "Channel"},
		{&RoutingPolicy{}, "RoutingPolicy"},
		{&RouteTarget{}, "RouteTarget"},
		{&Token{}, "Token"},
		{&User{}, "User"},
		{&UserSession{}, "UserSession"},
		{&AuthFlow{}, "AuthFlow"},
		{&ExternalIdentityClaim{}, "ExternalIdentityClaim"},
		{&PasskeyCredential{}, "PasskeyCredential"},
		{&Option{}, "Option"},
		{&Redemption{}, "Redemption"},
		{&Ability{}, "Ability"},
		{&Log{}, "Log"},
		{&Midjourney{}, "Midjourney"},
		{&TopUp{}, "TopUp"},
		{&QuotaData{}, "QuotaData"},
		{&Task{}, "Task"},
		{&Model{}, "Model"},
		{&Vendor{}, "Vendor"},
		{&PrefillGroup{}, "PrefillGroup"},
		{&Setup{}, "Setup"},
		{&TwoFA{}, "TwoFA"},
		{&TwoFABackupCode{}, "TwoFABackupCode"},
		{&Checkin{}, "Checkin"},
		{&SubscriptionOrder{}, "SubscriptionOrder"},
		{&UserSubscription{}, "UserSubscription"},
		{&SubscriptionPreConsumeRecord{}, "SubscriptionPreConsumeRecord"},
		{&CustomOAuthProvider{}, "CustomOAuthProvider"},
		{&UserOAuthBinding{}, "UserOAuthBinding"},
		{&PerfMetric{}, "PerfMetric"},
		{&SystemInstance{}, "SystemInstance"},
		{&SystemTask{}, "SystemTask"},
		{&SystemTaskLock{}, "SystemTaskLock"},
		{&ChannelModelCostRule{}, "ChannelModelCostRule"},
		{&CostAccountingRequest{}, "CostAccountingRequest"},
		{&CostAccountingAttempt{}, "CostAccountingAttempt"},
		{&CostAccountingAudit{}, "CostAccountingAudit"},
		{&ConfigImportBatch{}, "ConfigImportBatch"},
		{&ConfigImportItem{}, "ConfigImportItem"},
		{&ConfigImportBinding{}, "ConfigImportBinding"},
		{&ConfigImportIssue{}, "ConfigImportIssue"},
		{&ConfigImportResolution{}, "ConfigImportResolution"},
		{&ConfigImportPublishAudit{}, "ConfigImportPublishAudit"},
		{&ConfigImportActivationAudit{}, "ConfigImportActivationAudit"},
		{&ConfigImportRouteOwnershipChange{}, "ConfigImportRouteOwnershipChange"},
		{&Asset{}, "Asset"},
		{&AssetProviderBinding{}, "AssetProviderBinding"},
	}
	// 动态计算migration数量，确保errChan缓冲区足够大
	errChan := make(chan error, len(migrations))

	for _, m := range migrations {
		wg.Add(1)
		go func(model interface{}, name string) {
			defer wg.Done()
			if err := DB.AutoMigrate(model); err != nil {
				errChan <- fmt.Errorf("failed to migrate %s: %v", name, err)
			}
		}(m.model, m.name)
	}

	// Wait for all migrations to complete
	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		if err != nil {
			return err
		}
	}
	if err := migrateChannelTypeIDs(); err != nil {
		return err
	}
	if err := InitializeUserAuthVersions(); err != nil {
		return err
	}
	if err := InitializeExternalIdentityClaims(); err != nil {
		return err
	}
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		if err := ensureSubscriptionPlanTableSQLite(); err != nil {
			return err
		}
	} else {
		if err := DB.AutoMigrate(&SubscriptionPlan{}); err != nil {
			return err
		}
	}
	common.SysLog("database migrated")
	return nil
}

const secondaryChannelTypeMigrationMarker = "migration.secondary_channel_type_ids_20260727"
const ysrChannelTypeMigrationMarker = "migration.ysr_channel_type_ids_20260727"

func migrateChannelTypeIDs() error {
	if err := migrateSecondaryChannelTypeIDs(); err != nil {
		return err
	}
	return migrateYSRChannelTypeIDs()
}

// migrateSecondaryChannelTypeIDs moves the secondary branch channel types out
// of the range that origin/main now reserves for Sub2API and New API. Channel
// rows and persisted task platforms must move together so polling continues to
// select the original provider after the merge.
func migrateSecondaryChannelTypeIDs() error {
	if DB == nil {
		return nil
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		var marker Option
		err := tx.Where(&Option{Key: secondaryChannelTypeMigrationMarker}).First(&marker).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		for _, migration := range []struct{ from, to int }{
			{66, 68},
			{65, 67},
			{64, 66},
			{63, 65},
			{62, 64},
			{61, 63},
			{60, 62},
			{59, 61},
		} {
			if err := tx.Model(&Channel{}).Where("type = ?", migration.from).Update("type", migration.to).Error; err != nil {
				return err
			}
			if err := tx.Model(&Task{}).
				Where("platform = ?", strconv.Itoa(migration.from)).
				Update("platform", strconv.Itoa(migration.to)).Error; err != nil {
				return err
			}
		}

		return tx.Create(&Option{Key: secondaryChannelTypeMigrationMarker, Value: "complete"}).Error
	})
}

// migrateYSRChannelTypeIDs moves YSR-specific channel types into the reserved
// range. Channel rows and persisted task platforms must move together so
// polling continues to select the original provider after renumbering.
func migrateYSRChannelTypeIDs() error {
	if DB == nil {
		return nil
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		var marker Option
		err := tx.Where(&Option{Key: ysrChannelTypeMigrationMarker}).First(&marker).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		for _, migration := range []struct{ from, to int }{
			{61, constant.ChannelTypeDimensio},
			{62, constant.ChannelTypeNewAPIVideo},
			{63, constant.ChannelTypeClmmMall},
			{64, constant.ChannelTypeLucen},
			{65, constant.ChannelTypeMegaByAI},
			{66, constant.ChannelTypeCangyuan},
			{67, constant.ChannelTypePaipu},
			{68, constant.ChannelTypeSecure},
		} {
			if err := tx.Model(&Channel{}).Where("type = ?", migration.from).Update("type", migration.to).Error; err != nil {
				return err
			}
			if err := tx.Model(&Task{}).
				Where("platform = ?", strconv.Itoa(migration.from)).
				Update("platform", strconv.Itoa(migration.to)).Error; err != nil {
				return err
			}
		}

		return tx.Create(&Option{Key: ysrChannelTypeMigrationMarker, Value: "complete"}).Error
	})
}

func migrateLOGDB() error {
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return migrateClickHouseLogDB()
	}
	return LOG_DB.AutoMigrate(&Log{})
}

func migrateClickHouseLogDB() error {
	ttlDays := clickHouseLogTTLDays()
	if err := LOG_DB.Exec(clickHouseLogCreateTableSQL(ttlDays)).Error; err != nil {
		return err
	}
	return syncClickHouseLogTTL(ttlDays)
}

func clickHouseLogTTLDays() int {
	ttlDays := common.GetEnvOrDefault("LOG_SQL_CLICKHOUSE_TTL_DAYS", 0)
	if ttlDays < 0 {
		return 0
	}
	return ttlDays
}

func clickHouseLogTTLExpression(ttlDays int) string {
	if ttlDays <= 0 {
		return ""
	}
	return fmt.Sprintf("toDateTime(created_at) + INTERVAL %d DAY DELETE", ttlDays)
}

func clickHouseLogTTLClause(ttlDays int) string {
	expression := clickHouseLogTTLExpression(ttlDays)
	if expression == "" {
		return ""
	}
	return "\nTTL " + expression
}

func clickHouseLogCreateTableSQL(ttlDays int) string {
	return fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS logs (
	id Int64 DEFAULT 0,
	user_id Int32 DEFAULT 0,
	created_at Int64 DEFAULT 0,
	type Int32 DEFAULT 0,
	content String DEFAULT '',
	username String DEFAULT '',
	token_name String DEFAULT '',
	model_name String DEFAULT '',
	quota Int32 DEFAULT 0,
	prompt_tokens Int32 DEFAULT 0,
	completion_tokens Int32 DEFAULT 0,
	use_time Int32 DEFAULT 0,
	is_stream UInt8 DEFAULT 0,
	channel_id Int32 DEFAULT 0,
	token_id Int32 DEFAULT 0,
	`+"`group`"+` String DEFAULT '',
	ip String DEFAULT '',
	request_id String DEFAULT '',
	upstream_request_id String DEFAULT '',
	other String DEFAULT ''
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(toDateTime(created_at))
ORDER BY (created_at, request_id)%s`, clickHouseLogTTLClause(ttlDays))
}

func syncClickHouseLogTTL(ttlDays int) error {
	expression := clickHouseLogTTLExpression(ttlDays)
	if expression != "" {
		return LOG_DB.Exec("ALTER TABLE logs MODIFY TTL " + expression).Error
	}

	hasTTL, err := clickHouseLogTableHasTTL()
	if err != nil {
		return err
	}
	if !hasTTL {
		return nil
	}
	return LOG_DB.Exec("ALTER TABLE logs REMOVE TTL").Error
}

func clickHouseLogTableHasTTL() (bool, error) {
	var createTableSQL string
	if err := LOG_DB.Raw("SHOW CREATE TABLE logs").Scan(&createTableSQL).Error; err != nil {
		return false, err
	}
	return clickHouseCreateTableHasTTL(createTableSQL), nil
}

func clickHouseCreateTableHasTTL(createTableSQL string) bool {
	upperSQL := strings.ToUpper(createTableSQL)
	return strings.Contains(upperSQL, "\nTTL ") || strings.Contains(upperSQL, " TTL ")
}

type sqliteColumnDef struct {
	Name string
	DDL  string
}

func ensureSubscriptionPlanTableSQLite() error {
	if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		return nil
	}
	tableName := "subscription_plans"
	if !DB.Migrator().HasTable(tableName) {
		createSQL := `CREATE TABLE ` + "`" + tableName + "`" + ` (
` + "`id`" + ` integer,
` + "`title`" + ` varchar(128) NOT NULL,
` + "`subtitle`" + ` varchar(255) DEFAULT '',
` + "`price_amount`" + ` decimal(10,6) NOT NULL,
` + "`currency`" + ` varchar(8) NOT NULL DEFAULT 'USD',
` + "`duration_unit`" + ` varchar(16) NOT NULL DEFAULT 'month',
` + "`duration_value`" + ` integer NOT NULL DEFAULT 1,
` + "`custom_seconds`" + ` bigint NOT NULL DEFAULT 0,
` + "`enabled`" + ` numeric DEFAULT 1,
` + "`sort_order`" + ` integer DEFAULT 0,
` + "`allow_balance_pay`" + ` numeric DEFAULT 1,
` + "`allow_wallet_overflow`" + ` numeric DEFAULT 1,
` + "`stripe_price_id`" + ` varchar(128) DEFAULT '',
` + "`creem_product_id`" + ` varchar(128) DEFAULT '',
` + "`waffo_pancake_product_id`" + ` varchar(128) DEFAULT '',
` + "`max_purchase_per_user`" + ` integer DEFAULT 0,
` + "`upgrade_group`" + ` varchar(64) DEFAULT '',
` + "`downgrade_group`" + ` varchar(64) DEFAULT '',
` + "`total_amount`" + ` bigint NOT NULL DEFAULT 0,
` + "`quota_reset_period`" + ` varchar(16) DEFAULT 'never',
` + "`quota_reset_custom_seconds`" + ` bigint DEFAULT 0,
` + "`created_at`" + ` bigint,
` + "`updated_at`" + ` bigint,
PRIMARY KEY (` + "`id`" + `)
)`
		return DB.Exec(createSQL).Error
	}
	var cols []struct {
		Name string `gorm:"column:name"`
	}
	if err := DB.Raw("PRAGMA table_info(`" + tableName + "`)").Scan(&cols).Error; err != nil {
		return err
	}
	existing := make(map[string]struct{}, len(cols))
	for _, c := range cols {
		existing[c.Name] = struct{}{}
	}
	required := []sqliteColumnDef{
		{Name: "title", DDL: "`title` varchar(128) NOT NULL"},
		{Name: "subtitle", DDL: "`subtitle` varchar(255) DEFAULT ''"},
		{Name: "price_amount", DDL: "`price_amount` decimal(10,6) NOT NULL"},
		{Name: "currency", DDL: "`currency` varchar(8) NOT NULL DEFAULT 'USD'"},
		{Name: "duration_unit", DDL: "`duration_unit` varchar(16) NOT NULL DEFAULT 'month'"},
		{Name: "duration_value", DDL: "`duration_value` integer NOT NULL DEFAULT 1"},
		{Name: "custom_seconds", DDL: "`custom_seconds` bigint NOT NULL DEFAULT 0"},
		{Name: "enabled", DDL: "`enabled` numeric DEFAULT 1"},
		{Name: "sort_order", DDL: "`sort_order` integer DEFAULT 0"},
		{Name: "allow_balance_pay", DDL: "`allow_balance_pay` numeric DEFAULT 1"},
		{Name: "allow_wallet_overflow", DDL: "`allow_wallet_overflow` numeric DEFAULT 1"},
		{Name: "stripe_price_id", DDL: "`stripe_price_id` varchar(128) DEFAULT ''"},
		{Name: "creem_product_id", DDL: "`creem_product_id` varchar(128) DEFAULT ''"},
		{Name: "waffo_pancake_product_id", DDL: "`waffo_pancake_product_id` varchar(128) DEFAULT ''"},
		{Name: "max_purchase_per_user", DDL: "`max_purchase_per_user` integer DEFAULT 0"},
		{Name: "upgrade_group", DDL: "`upgrade_group` varchar(64) DEFAULT ''"},
		{Name: "downgrade_group", DDL: "`downgrade_group` varchar(64) DEFAULT ''"},
		{Name: "total_amount", DDL: "`total_amount` bigint NOT NULL DEFAULT 0"},
		{Name: "quota_reset_period", DDL: "`quota_reset_period` varchar(16) DEFAULT 'never'"},
		{Name: "quota_reset_custom_seconds", DDL: "`quota_reset_custom_seconds` bigint DEFAULT 0"},
		{Name: "created_at", DDL: "`created_at` bigint"},
		{Name: "updated_at", DDL: "`updated_at` bigint"},
	}
	for _, col := range required {
		if _, ok := existing[col.Name]; ok {
			continue
		}
		if err := DB.Exec("ALTER TABLE `" + tableName + "` ADD COLUMN " + col.DDL).Error; err != nil {
			return err
		}
	}
	return nil
}

// migrateConfigImportBindingChannelIndex removes the legacy one-channel-per-line
// constraint. Snapshot imports may bind multiple source lines to one channel;
// the publish transaction unions those lines before replacing the model list.
func migrateConfigImportBindingChannelIndex() error {
	if DB == nil || !DB.Migrator().HasTable(&ConfigImportBinding{}) {
		return nil
	}
	if !DB.Migrator().HasIndex(&ConfigImportBinding{}, "idx_config_import_binding_channel") {
		return nil
	}
	if err := DB.Migrator().DropIndex(&ConfigImportBinding{}, "idx_config_import_binding_channel"); err != nil {
		return fmt.Errorf("drop legacy idx_config_import_binding_channel: %w", err)
	}
	return nil
}

// migrateConfigImportBatchIdentity preserves payload hashes while replacing
// the legacy payload unique index with an explicit batch-instance key.
func migrateConfigImportBatchIdentity() error {
	if DB == nil || !DB.Migrator().HasTable(&ConfigImportBatch{}) {
		return nil
	}
	columns := []struct {
		Name string
		DDL  string
	}{
		{Name: "deduplication_key", DDL: "VARCHAR(128) NULL"},
		{Name: "copied_from_batch_id", DDL: "BIGINT NULL"},
	}
	for _, column := range columns {
		if DB.Migrator().HasColumn(&ConfigImportBatch{}, column.Name) {
			continue
		}
		statement := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s",
			quoteIdentifier("config_import_batches"), quoteIdentifier(column.Name), column.DDL)
		if err := DB.Exec(statement).Error; err != nil {
			return fmt.Errorf("add config_import_batches.%s: %w", column.Name, err)
		}
	}
	var batches []ConfigImportBatch
	if err := DB.Where("deduplication_key IS NULL OR deduplication_key = ''").Order("id ASC").Find(&batches).Error; err != nil {
		return err
	}
	for _, batch := range batches {
		key := ConfigImportUploadDeduplicationKey(batch.PayloadSHA256)
		if err := DB.Model(&ConfigImportBatch{}).Where("id = ?", batch.ID).Update("deduplication_key", key).Error; err != nil {
			return fmt.Errorf("backfill config import batch %d identity: %w", batch.ID, err)
		}
	}
	for _, indexName := range []string{
		"idx_config_import_batches_payload_sha256",
		"uni_config_import_batches_payload_sha256",
	} {
		if !DB.Migrator().HasIndex(&ConfigImportBatch{}, indexName) {
			continue
		}
		if err := DB.Migrator().DropIndex(&ConfigImportBatch{}, indexName); err != nil {
			return fmt.Errorf("drop legacy config import batch payload index %s: %w", indexName, err)
		}
	}
	return nil
}

func migrateRouteTargetOwnershipColumns() error {
	if DB == nil || !DB.Migrator().HasTable(&RouteTarget{}) {
		return nil
	}
	columns := []struct {
		Name string
		DDL  string
	}{
		{Name: "managed_by", DDL: "VARCHAR(32) NOT NULL DEFAULT 'manual'"},
		{Name: "source_batch_id", DDL: "BIGINT NULL"},
		{Name: "retired_at", DDL: "BIGINT NULL"},
	}
	for _, column := range columns {
		if DB.Migrator().HasColumn(&RouteTarget{}, column.Name) {
			continue
		}
		statement := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s",
			quoteIdentifier("route_targets"), quoteIdentifier(column.Name), column.DDL)
		if err := DB.Exec(statement).Error; err != nil {
			return fmt.Errorf("add route_targets.%s: %w", column.Name, err)
		}
	}
	if err := DB.Model(&RouteTarget{}).
		Where("managed_by IS NULL OR managed_by = ?", "").
		Update("managed_by", string(types.RouteTargetManagedByManual)).Error; err != nil {
		return err
	}
	if !common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		return nil
	}

	var managedByColumn gorm.ColumnType
	columnTypes, err := DB.Migrator().ColumnTypes(&RouteTarget{})
	if err != nil {
		return fmt.Errorf("inspect route_targets ownership columns: %w", err)
	}
	for _, columnType := range columnTypes {
		if columnType.Name() == "managed_by" {
			managedByColumn = columnType
			break
		}
	}
	if managedByColumn == nil {
		return fmt.Errorf("route_targets.managed_by was not created")
	}
	nullable, nullableKnown := managedByColumn.Nullable()
	_, hasDefault := managedByColumn.DefaultValue()
	if nullableKnown && !nullable && !hasDefault {
		return nil
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`CREATE TABLE "route_targets__ownership_migration" ("id" integer,"policy_id" integer NOT NULL,"channel_id" integer NOT NULL,"name" varchar(128) NOT NULL,"upstream_model" varchar(255) NOT NULL,"cost_variant_key" varchar(64) NOT NULL,"target_priority" integer NOT NULL,"minimum_expected_margin_bps" integer,"constraints" text NOT NULL,"enabled" numeric NOT NULL,"managed_by" varchar(32) NOT NULL,"source_batch_id" integer,"retired_at" integer,"created_at" integer,"updated_at" integer,PRIMARY KEY ("id"))`).Error; err != nil {
			return fmt.Errorf("create normalized route_targets table: %w", err)
		}
		if err := tx.Exec(`INSERT INTO route_targets__ownership_migration (
			id, policy_id, channel_id, name, upstream_model, cost_variant_key,
			target_priority, minimum_expected_margin_bps, constraints, enabled,
			managed_by, source_batch_id, retired_at, created_at, updated_at
		) SELECT
			id, policy_id, channel_id, name, upstream_model, cost_variant_key,
			target_priority, minimum_expected_margin_bps, constraints, enabled,
			managed_by, source_batch_id, retired_at, created_at, updated_at
		FROM route_targets`).Error; err != nil {
			return fmt.Errorf("copy normalized route_targets rows: %w", err)
		}
		if err := tx.Exec("DROP TABLE route_targets").Error; err != nil {
			return fmt.Errorf("drop legacy route_targets table: %w", err)
		}
		if err := tx.Exec("ALTER TABLE route_targets__ownership_migration RENAME TO route_targets").Error; err != nil {
			return fmt.Errorf("rename normalized route_targets table: %w", err)
		}
		return nil
	})
}

// migrateCostVariantKeys extends the channel_model_cost_rules and route_targets
// business keys with a cost_variant_key column. It is idempotent and must run
// before AutoMigrate so the four-column unique index replaces the legacy
// three-column one without leaving a duplicate index behind.
//
// Existing rows are backfilled to types.DefaultCostVariantKey ("default") so
// legacy queries keep resolving a single active rule per business key. We do
// not use a GORM default tag because MySQL and PostgreSQL normalize boolean
// and string defaults differently, which can make AutoMigrate re-issue ALTER
// statements on every restart.
func migrateCostVariantKeys() error {
	if DB == nil {
		return nil
	}
	if err := migrateCostVariantKeyColumn(&ChannelModelCostRule{}, "channel_model_cost_rules", "cost_variant_key"); err != nil {
		return err
	}
	if err := migrateCostVariantKeyColumn(&RouteTarget{}, "route_targets", "cost_variant_key"); err != nil {
		return err
	}
	if err := migrateCostVariantKeyColumn(&CostAccountingAttempt{}, "cost_accounting_attempts", "cost_variant_key"); err != nil {
		return err
	}
	return nil
}

// migrateCostVariantKeyColumn adds the variant column to one table, backfills
// blank values to the default variant, and drops the legacy three-column
// cost-rule unique index when present so AutoMigrate recreates it with the new
// four-column shape.
func migrateCostVariantKeyColumn(model interface{}, tableName, columnName string) error {
	if !DB.Migrator().HasTable(model) {
		return nil
	}
	quotedColumn := quoteIdentifier(columnName)
	quotedTable := quoteIdentifier(tableName)
	if !DB.Migrator().HasColumn(model, columnName) {
		columnDDL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s VARCHAR(64) NOT NULL DEFAULT 'default'", quotedTable, quotedColumn)
		if err := DB.Exec(columnDDL).Error; err != nil {
			return fmt.Errorf("add %s.%s: %w", tableName, columnName, err)
		}
	}

	// Backfill any blank/NULL values to the default variant. NULL cannot occur
	// after the ADD COLUMN above (NOT NULL DEFAULT), but legacy rows copied by
	// external tooling or partial migrations may still be blank.
	updateSQL := fmt.Sprintf("UPDATE %s SET %s = ? WHERE %s IS NULL OR %s = ''",
		quotedTable, quotedColumn, quotedColumn, quotedColumn)
	if err := DB.Exec(updateSQL, string(types.DefaultCostVariantKey)).Error; err != nil {
		return fmt.Errorf("backfill %s.%s: %w", tableName, columnName, err)
	}

	// Drop the legacy three-column unique index on channel_model_cost_rules so
	// AutoMigrate recreates idx_cost_rule_version with the new four-column
	// shape. SQLite and PostgreSQL store indexes by name; MySQL does too, so a
	// shared DropIfExists on the index name is sufficient across all three.
	if tableName == "channel_model_cost_rules" {
		legacy := &ChannelModelCostRule{}
		if DB.Migrator().HasIndex(legacy, "idx_cost_rule_version") {
			if err := DB.Migrator().DropIndex(legacy, "idx_cost_rule_version"); err != nil {
				return fmt.Errorf("drop legacy idx_cost_rule_version: %w", err)
			}
		}
	}
	return nil
}

// quoteIdentifier quotes a single SQL identifier for the active dialect:
// PostgreSQL uses double quotes while MySQL and SQLite use backticks.
func quoteIdentifier(identifier string) string {
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		return `"` + identifier + `"`
	}
	return "`" + identifier + "`"
}

// migrateTokenModelLimitsToText migrates model_limits column from varchar(1024) to text
// This is safe to run multiple times - it checks the column type first
func migrateTokenModelLimitsToText() error {
	// SQLite uses type affinity, so TEXT and VARCHAR are effectively the same — no migration needed
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		return nil
	}

	tableName := "tokens"
	columnName := "model_limits"

	if !DB.Migrator().HasTable(tableName) {
		return nil
	}

	if !DB.Migrator().HasColumn(&Token{}, columnName) {
		return nil
	}

	var alterSQL string
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		var dataType string
		if err := DB.Raw(`SELECT data_type FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
			tableName, columnName).Scan(&dataType).Error; err != nil {
			common.SysLog(fmt.Sprintf("Warning: failed to query metadata for %s.%s: %v", tableName, columnName, err))
		} else if dataType == "text" {
			return nil
		}
		alterSQL = fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN %s TYPE text`, tableName, columnName)
	} else if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
		var columnType string
		if err := DB.Raw(`SELECT COLUMN_TYPE FROM information_schema.columns
				WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
			tableName, columnName).Scan(&columnType).Error; err != nil {
			common.SysLog(fmt.Sprintf("Warning: failed to query metadata for %s.%s: %v", tableName, columnName, err))
		} else if strings.ToLower(columnType) == "text" {
			return nil
		}
		alterSQL = fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s text", tableName, columnName)
	} else {
		return nil
	}

	if alterSQL != "" {
		if err := DB.Exec(alterSQL).Error; err != nil {
			return fmt.Errorf("failed to migrate %s.%s to text: %w", tableName, columnName, err)
		}
		common.SysLog(fmt.Sprintf("Successfully migrated %s.%s to text", tableName, columnName))
	}
	return nil
}

// migrateSubscriptionPlanPriceAmount migrates price_amount column from float/double to decimal(10,6)
// This is safe to run multiple times - it checks the column type first
func migrateSubscriptionPlanPriceAmount() {
	// SQLite doesn't support ALTER COLUMN, and its type affinity handles this automatically
	// Skip early to avoid GORM parsing the existing table DDL which may cause issues
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		return
	}

	tableName := "subscription_plans"
	columnName := "price_amount"

	// Check if table exists first
	if !DB.Migrator().HasTable(tableName) {
		return
	}

	// Check if column exists
	if !DB.Migrator().HasColumn(&SubscriptionPlan{}, columnName) {
		return
	}

	var alterSQL string
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		// PostgreSQL: Check if already decimal/numeric
		var dataType string
		if err := DB.Raw(`SELECT data_type FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
			tableName, columnName).Scan(&dataType).Error; err != nil {
			common.SysLog(fmt.Sprintf("Warning: failed to query metadata for %s.%s: %v", tableName, columnName, err))
		} else if dataType == "numeric" {
			return // Already decimal/numeric
		}
		alterSQL = fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN %s TYPE decimal(10,6) USING %s::decimal(10,6)`,
			tableName, columnName, columnName)
	} else if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
		// MySQL: Check if already decimal
		var columnType string
		if err := DB.Raw(`SELECT COLUMN_TYPE FROM information_schema.columns
				WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
			tableName, columnName).Scan(&columnType).Error; err != nil {
			common.SysLog(fmt.Sprintf("Warning: failed to query metadata for %s.%s: %v", tableName, columnName, err))
		} else if strings.HasPrefix(strings.ToLower(columnType), "decimal") {
			return // Already decimal
		}
		alterSQL = fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s decimal(10,6) NOT NULL DEFAULT 0",
			tableName, columnName)
	} else {
		return
	}

	if alterSQL != "" {
		if err := DB.Exec(alterSQL).Error; err != nil {
			common.SysLog(fmt.Sprintf("Warning: failed to migrate %s.%s to decimal: %v", tableName, columnName, err))
		} else {
			common.SysLog(fmt.Sprintf("Successfully migrated %s.%s to decimal(10,6)", tableName, columnName))
		}
	}
}

func closeDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	err = sqlDB.Close()
	return err
}

func CloseDB() error {
	if LOG_DB != DB {
		err := closeDB(LOG_DB)
		if err != nil {
			return err
		}
	}
	return closeDB(DB)
}

// checkMySQLChineseSupport ensures the MySQL connection and current schema
// default charset/collation can store Chinese characters. It allows common
// Chinese-capable charsets (utf8mb4, utf8, gbk, big5, gb18030) and panics otherwise.
func checkMySQLChineseSupport(db *gorm.DB) error {
	// 仅检测：当前库默认字符集/排序规则 + 各表的排序规则（隐含字符集）

	// Read current schema defaults
	var schemaCharset, schemaCollation string
	err := db.Raw("SELECT DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = DATABASE()").Row().Scan(&schemaCharset, &schemaCollation)
	if err != nil {
		return fmt.Errorf("读取当前库默认字符集/排序规则失败 / Failed to read schema default charset/collation: %v", err)
	}

	toLower := func(s string) string { return strings.ToLower(s) }
	// Allowed charsets that can store Chinese text
	allowedCharsets := map[string]string{
		"utf8mb4": "utf8mb4_",
		"utf8":    "utf8_",
		"gbk":     "gbk_",
		"big5":    "big5_",
		"gb18030": "gb18030_",
	}
	isChineseCapable := func(cs, cl string) bool {
		csLower := toLower(cs)
		clLower := toLower(cl)
		if prefix, ok := allowedCharsets[csLower]; ok {
			if clLower == "" {
				return true
			}
			return strings.HasPrefix(clLower, prefix)
		}
		// 如果仅提供了排序规则，尝试按排序规则前缀判断
		for _, prefix := range allowedCharsets {
			if strings.HasPrefix(clLower, prefix) {
				return true
			}
		}
		return false
	}

	// 1) 当前库默认值必须支持中文
	if !isChineseCapable(schemaCharset, schemaCollation) {
		return fmt.Errorf("当前库默认字符集/排序规则不支持中文：schema(%s/%s)。请将库设置为 utf8mb4/utf8/gbk/big5/gb18030 / Schema default charset/collation is not Chinese-capable: schema(%s/%s). Please set to utf8mb4/utf8/gbk/big5/gb18030",
			schemaCharset, schemaCollation, schemaCharset, schemaCollation)
	}

	// 2) 所有物理表的排序规则（隐含字符集）必须支持中文
	type tableInfo struct {
		Name      string
		Collation *string
	}
	var tables []tableInfo
	if err := db.Raw("SELECT TABLE_NAME, TABLE_COLLATION FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE'").Scan(&tables).Error; err != nil {
		return fmt.Errorf("读取表排序规则失败 / Failed to read table collations: %v", err)
	}

	var badTables []string
	for _, t := range tables {
		// NULL 或空表示继承库默认设置，已在上面校验库默认，视为通过
		if t.Collation == nil || *t.Collation == "" {
			continue
		}
		cl := *t.Collation
		// 仅凭排序规则判断是否中文可用
		ok := false
		lower := strings.ToLower(cl)
		for _, prefix := range allowedCharsets {
			if strings.HasPrefix(lower, prefix) {
				ok = true
				break
			}
		}
		if !ok {
			badTables = append(badTables, fmt.Sprintf("%s(%s)", t.Name, cl))
		}
	}

	if len(badTables) > 0 {
		// 限制输出数量以避免日志过长
		maxShow := 20
		shown := badTables
		if len(shown) > maxShow {
			shown = shown[:maxShow]
		}
		return fmt.Errorf(
			"存在不支持中文的表，请修复其排序规则/字符集。示例（最多展示 %d 项）：%v / Found tables not Chinese-capable. Please fix their collation/charset. Examples (showing up to %d): %v",
			maxShow, shown, maxShow, shown,
		)
	}
	return nil
}

var (
	lastPingTime time.Time
	pingMutex    sync.Mutex
)

func PingDB() error {
	pingMutex.Lock()
	defer pingMutex.Unlock()

	if time.Since(lastPingTime) < time.Second*10 {
		return nil
	}

	sqlDB, err := DB.DB()
	if err != nil {
		log.Printf("Error getting sql.DB from GORM: %v", err)
		return err
	}

	err = sqlDB.Ping()
	if err != nil {
		log.Printf("Error pinging DB: %v", err)
		return err
	}

	lastPingTime = time.Now()
	common.SysLog("Database pinged successfully")
	return nil
}
