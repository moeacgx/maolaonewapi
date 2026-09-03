package model

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

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

// InitDBColumns initializes dialect-specific quoted column names after database types are configured.
// Callers that install DB handles directly must invoke it before executing model queries.
func InitDBColumns() {
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
			db, err := gorm.Open(clickhouse.Open(normalizeClickHouseDSN(dsn)), newGormConfig(false))
			return db, common.DatabaseTypeClickHouse, err
		}
		if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
			// Use PostgreSQL
			common.SysLog("using PostgreSQL as database")
			db, err := gorm.Open(postgres.New(postgres.Config{
				DSN:                  dsn,
				PreferSimpleProtocol: true, // disables implicit prepared statement usage
			}), newGormConfig(true))
			return db, common.DatabaseTypePostgreSQL, err
		}
		if strings.HasPrefix(dsn, "local") {
			common.SysLog("SQL_DSN not set, using SQLite as database")
			db, err := gorm.Open(sqlite.Open(common.SQLitePath), newGormConfig(true))
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
		db, err := gorm.Open(mysql.Open(dsn), newGormConfig(true))
		return db, common.DatabaseTypeMySQL, err
	}
	// Use SQLite
	common.SysLog("SQL_DSN not set, using SQLite as database")
	db, err := gorm.Open(sqlite.Open(common.SQLitePath), newGormConfig(true))
	return db, common.DatabaseTypeSQLite, err
}

func InitDB() (err error) {
	db, dbType, err := chooseDB("SQL_DSN", false)
	if err == nil {
		common.SetMainDatabaseType(dbType)
		if os.Getenv("LOG_SQL_DSN") == "" {
			common.SetLogDatabaseType(dbType)
		}
		InitDBColumns()
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
		InitDBColumns()
		return
	}
	db, dbType, err := chooseDB("LOG_SQL_DSN", true)
	if err == nil {
		common.SetLogDatabaseType(dbType)
		InitDBColumns()
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
	// 钱包和返佣余额可能超过 32 位额度；在 AutoMigrate 读取旧结构前先升级历史整数列。
	if err := migrateWalletQuotaColumns(); err != nil {
		return err
	}
	if err := migrateAffiliateRecordSourceIndex(DB); err != nil {
		return err
	}
	// Migrate price_amount column from float/double to decimal for existing tables
	migrateSubscriptionPlanPriceAmount()
	// Migrate model_limits column from varchar to text for existing tables
	if err := migrateTokenModelLimitsToText(); err != nil {
		return err
	}
	if err := migrateSQLiteRequestArchiveDedupeKey(); err != nil {
		return err
	}
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		if err := ensureSQLiteLogIdempotencyKey(DB); err != nil {
			return err
		}
	}

	err := DB.AutoMigrate(
		&Channel{},
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
		&TopUpPaymentAttempt{},
		&InvoiceRecord{},
		&InvoiceOrderLink{},
		&PromoCode{},
		&PromoCodeUsage{},
		&PromoCodeReservation{},
		&BenefitActivity{},
		&BenefitActivityShare{},
		&BenefitUserVoucher{},
		&BenefitVoucherLedger{},

		&AffiliateRecord{},
		&AffiliateBalance{},
		&AffiliatePayoutAccount{},
		&AffiliateWithdrawal{},
		&AffiliateApplication{},
		&AffiliateFraudAlert{},
		&AffiliateRiskUser{},
		&AffiliateRiskEvent{},
		&AffiliateRiskDetachedInvitee{},
		&UserIPRecord{},
		&QuotaData{},
		&Task{},
		&GameWallet{},
		&GameWalletTransaction{},
		&GamePrediction{},
		&GamePredictionOption{},
		&GamePredictionBet{},
		&NotificationBot{},
		&NotificationTask{},
		&NotificationTarget{},
		&NotificationEvent{},
		&NotificationEventReceipt{},
		&NotificationDelivery{},
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
		&PromptAuditConfig{},
		&PromptAuditEndpoint{},
		&PromptAuditJob{},
		&PromptAuditEvent{},
		&PromptAuditQueueState{},
		&SystemTask{},
		&SystemTaskLock{},
		&CasbinRule{},
		&AuthzRole{},
		&RequestArchiveConfig{},
		&RequestArchiveTarget{},
		&RequestArchiveJob{},
		&RequestArchiveQueueState{},
		&ConversationArchiveConfig{},
		&ConversationArchive{},
		&Group{},
		&GroupAlias{},
		&AutoGroupMember{},
		&ChannelGroupBinding{},
		&TokenGroupBinding{},
	)
	if err != nil {
		return err
	}
	if err := EnsureConversationArchiveConfig(); err != nil {
		return err
	}
	if err := migrateBenefitActivityQuotaConfig(DB); err != nil {
		return err
	}
	if err := migratePromoCodeDeletionKey(DB); err != nil {
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
	if err := migrateGroupIdentity(); err != nil {
		return fmt.Errorf("failed to migrate group identity: %w", err)
	}
	if err := BackfillGroupBindings(); err != nil {
		return fmt.Errorf("failed to backfill group bindings: %w", err)
	}
	return nil
}

func migrateDBFast() error {
	if err := migrateWalletQuotaColumns(); err != nil {
		return err
	}
	if err := migrateAffiliateRecordSourceIndex(DB); err != nil {
		return err
	}
	if err := migrateSQLiteRequestArchiveDedupeKey(); err != nil {
		return err
	}

	var wg sync.WaitGroup

	migrations := []struct {
		model interface{}
		name  string
	}{
		{&Channel{}, "Channel"},
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
		{&TopUpPaymentAttempt{}, "TopUpPaymentAttempt"},
		{&InvoiceRecord{}, "InvoiceRecord"},
		{&InvoiceOrderLink{}, "InvoiceOrderLink"},
		{&PromoCode{}, "PromoCode"},
		{&PromoCodeUsage{}, "PromoCodeUsage"},
		{&PromoCodeReservation{}, "PromoCodeReservation"},
		{&BenefitActivity{}, "BenefitActivity"},
		{&BenefitActivityShare{}, "BenefitActivityShare"},
		{&BenefitUserVoucher{}, "BenefitUserVoucher"},
		{&BenefitVoucherLedger{}, "BenefitVoucherLedger"},

		{&AffiliateRecord{}, "AffiliateRecord"},
		{&AffiliateBalance{}, "AffiliateBalance"},
		{&AffiliatePayoutAccount{}, "AffiliatePayoutAccount"},
		{&AffiliateWithdrawal{}, "AffiliateWithdrawal"},
		{&AffiliateApplication{}, "AffiliateApplication"},
		{&AffiliateFraudAlert{}, "AffiliateFraudAlert"},
		{&AffiliateRiskUser{}, "AffiliateRiskUser"},
		{&AffiliateRiskEvent{}, "AffiliateRiskEvent"},
		{&AffiliateRiskDetachedInvitee{}, "AffiliateRiskDetachedInvitee"},
		{&UserIPRecord{}, "UserIPRecord"},
		{&QuotaData{}, "QuotaData"},
		{&Task{}, "Task"},
		{&GameWallet{}, "GameWallet"},
		{&GameWalletTransaction{}, "GameWalletTransaction"},
		{&GamePrediction{}, "GamePrediction"},
		{&GamePredictionOption{}, "GamePredictionOption"},
		{&GamePredictionBet{}, "GamePredictionBet"},
		{&NotificationBot{}, "NotificationBot"},
		{&NotificationTask{}, "NotificationTask"},
		{&NotificationTarget{}, "NotificationTarget"},
		{&NotificationEvent{}, "NotificationEvent"},
		{&NotificationEventReceipt{}, "NotificationEventReceipt"},
		{&NotificationDelivery{}, "NotificationDelivery"},
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
		{&PromptAuditConfig{}, "PromptAuditConfig"},
		{&PromptAuditEndpoint{}, "PromptAuditEndpoint"},
		{&PromptAuditJob{}, "PromptAuditJob"},
		{&PromptAuditEvent{}, "PromptAuditEvent"},
		{&PromptAuditQueueState{}, "PromptAuditQueueState"},
		{&SystemInstance{}, "SystemInstance"},
		{&SystemTask{}, "SystemTask"},
		{&SystemTaskLock{}, "SystemTaskLock"},
		{&RequestArchiveConfig{}, "RequestArchiveConfig"},
		{&RequestArchiveTarget{}, "RequestArchiveTarget"},
		{&RequestArchiveJob{}, "RequestArchiveJob"},
		{&RequestArchiveQueueState{}, "RequestArchiveQueueState"},
		{&ConversationArchiveConfig{}, "ConversationArchiveConfig"},
		{&ConversationArchive{}, "ConversationArchive"},
		{&Group{}, "Group"},
		{&GroupAlias{}, "GroupAlias"},
		{&AutoGroupMember{}, "AutoGroupMember"},
		{&ChannelGroupBinding{}, "ChannelGroupBinding"},
		{&TokenGroupBinding{}, "TokenGroupBinding"},
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
	if err := EnsureConversationArchiveConfig(); err != nil {
		return err
	}
	if err := migrateBenefitActivityQuotaConfig(DB); err != nil {
		return err
	}
	if err := migratePromoCodeDeletionKey(DB); err != nil {
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
	if err := migrateGroupIdentity(); err != nil {
		return fmt.Errorf("failed to migrate group identity: %w", err)
	}
	if err := BackfillGroupBindings(); err != nil {
		return fmt.Errorf("failed to backfill group bindings: %w", err)
	}
	common.SysLog("database migrated")
	return nil
}

func migrateLOGDB() error {
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return migrateClickHouseLogDB()
	}
	if common.UsingLogDatabase(common.DatabaseTypeSQLite) {
		if err := ensureSQLiteLogIdempotencyKey(LOG_DB); err != nil {
			return err
		}
	}
	return LOG_DB.AutoMigrate(&Log{})
}

func ensureSQLiteLogIdempotencyKey(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&Log{}) || db.Migrator().HasColumn(&Log{}, "idempotency_key") {
		return nil
	}
	return db.Exec("ALTER TABLE `logs` ADD COLUMN `idempotency_key` varchar(191)").Error
}

func migrateClickHouseLogDB() error {
	ttlDays := clickHouseLogTTLDays()
	if err := LOG_DB.Exec(clickHouseLogCreateTableSQL(ttlDays)).Error; err != nil {
		return err
	}
	if err := LOG_DB.Exec("ALTER TABLE logs ADD COLUMN IF NOT EXISTS idempotency_key Nullable(String) DEFAULT NULL").Error; err != nil {
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
	idempotency_key Nullable(String) DEFAULT NULL,
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

// walletQuotaColumn 标识钱包、充值、订阅、兑换码和返佣账务中的持久化额度列。
// 这些列的宽度应大于单次请求使用的 int32 额度。
type walletQuotaColumn struct {
	table  string
	column string
}

var walletQuotaColumns = []walletQuotaColumn{
	{table: "users", column: "quota"},
	{table: "users", column: "used_quota"},
	{table: "users", column: "aff_quota"},
	{table: "users", column: "aff_history"},
	{table: "top_ups", column: "affiliate_source_quota"},
	{table: "top_ups", column: "credited_quota"},
	{table: "affiliate_records", column: "source_quota"},
	{table: "affiliate_records", column: "reward_quota"},
	{table: "affiliate_records", column: "balance_after_quota"},
	{table: "affiliate_balances", column: "pending_quota"},
	{table: "affiliate_balances", column: "available_quota"},
	{table: "affiliate_balances", column: "frozen_quota"},
	{table: "affiliate_balances", column: "risk_frozen_quota"},
	{table: "affiliate_balances", column: "confiscated_quota"},
	{table: "affiliate_balances", column: "withdrawn_quota"},
	{table: "affiliate_balances", column: "transferred_quota"},
	{table: "affiliate_balances", column: "total_quota"},
	{table: "affiliate_withdrawals", column: "quota"},
	{table: "affiliate_fraud_alerts", column: "clawback_quota"},
	{table: "affiliate_risk_users", column: "cleared_quota"},
	{table: "subscription_orders", column: "affiliate_source_quota"},
	{table: "redemptions", column: "quota"},
}

// migrateWalletQuotaColumns 将历史钱包相关整数列升级为 BIGINT。SQLite 的
// INTEGER 本身就是有符号 64 位值，无需重建表；其他数据库需要显式转换，
// 因为 AutoMigrate 不一定会可靠地放宽已有的 int 列。迁移可重复执行，
// 对尚未创建的表或列直接跳过。
func migrateWalletQuotaColumns() error {
	if DB == nil || common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		return nil
	}

	for _, quotaColumn := range walletQuotaColumns {
		if !DB.Migrator().HasTable(quotaColumn.table) {
			continue
		}
		var err error
		switch {
		case common.UsingMainDatabase(common.DatabaseTypePostgreSQL):
			err = migratePostgresWalletQuotaColumn(quotaColumn)
		case common.UsingMainDatabase(common.DatabaseTypeMySQL):
			err = migrateMySQLWalletQuotaColumn(quotaColumn)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func migratePostgresWalletQuotaColumn(quotaColumn walletQuotaColumn) error {
	var dataType string
	result := DB.Raw(`SELECT data_type FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
		quotaColumn.table, quotaColumn.column).Scan(&dataType)
	if result.Error != nil {
		return fmt.Errorf("failed to inspect %s.%s quota type: %w", quotaColumn.table, quotaColumn.column, result.Error)
	}
	dataType = strings.TrimSpace(dataType)
	if result.RowsAffected == 0 || dataType == "" || strings.EqualFold(dataType, "bigint") {
		return nil
	}

	tableName := quoteMainIdentifier(quotaColumn.table)
	columnName := quoteMainIdentifier(quotaColumn.column)
	alterSQL := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE BIGINT USING %s::bigint", tableName, columnName, columnName)
	if err := DB.Exec(alterSQL).Error; err != nil {
		return fmt.Errorf("failed to migrate %s.%s to bigint: %w", quotaColumn.table, quotaColumn.column, err)
	}
	common.SysLog(fmt.Sprintf("Successfully migrated %s.%s to bigint", quotaColumn.table, quotaColumn.column))
	return nil
}

type mysqlWalletQuotaColumnMetadata struct {
	DataType      string         `gorm:"column:data_type"`
	ColumnType    string         `gorm:"column:column_type"`
	IsNullable    string         `gorm:"column:is_nullable"`
	ColumnDefault sql.NullString `gorm:"column:column_default"`
	ColumnComment sql.NullString `gorm:"column:column_comment"`
}

func migrateMySQLWalletQuotaColumn(quotaColumn walletQuotaColumn) error {
	var metadata mysqlWalletQuotaColumnMetadata
	result := DB.Raw(`SELECT DATA_TYPE AS data_type, COLUMN_TYPE AS column_type,
		IS_NULLABLE AS is_nullable, COLUMN_DEFAULT AS column_default,
		COLUMN_COMMENT AS column_comment
		FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
		quotaColumn.table, quotaColumn.column).Scan(&metadata)
	if result.Error != nil {
		return fmt.Errorf("failed to inspect %s.%s quota type: %w", quotaColumn.table, quotaColumn.column, result.Error)
	}
	metadata.DataType = strings.TrimSpace(metadata.DataType)
	isUnsigned := strings.Contains(strings.ToLower(metadata.ColumnType), "unsigned")
	if result.RowsAffected == 0 || metadata.DataType == "" ||
		(strings.EqualFold(metadata.DataType, "bigint") && !isUnsigned) {
		return nil
	}

	// Go 钱包契约是有符号 int64，不保留旧的 unsigned 定义，避免数据库存储
	// 应用无法表示的数值。
	typeClause := "BIGINT"
	if strings.EqualFold(metadata.IsNullable, "NO") {
		typeClause += " NOT NULL"
	} else {
		typeClause += " NULL"
	}
	if metadata.ColumnDefault.Valid {
		typeClause += " DEFAULT " + quoteMySQLDefault(metadata.ColumnDefault.String)
	}
	if metadata.ColumnComment.Valid && metadata.ColumnComment.String != "" {
		typeClause += " COMMENT " + quoteSQLString(metadata.ColumnComment.String)
	}

	alterSQL := fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s %s",
		quoteMainIdentifier(quotaColumn.table), quoteMainIdentifier(quotaColumn.column), typeClause)
	if err := DB.Exec(alterSQL).Error; err != nil {
		return fmt.Errorf("failed to migrate %s.%s to bigint: %w", quotaColumn.table, quotaColumn.column, err)
	}
	common.SysLog(fmt.Sprintf("Successfully migrated %s.%s to bigint", quotaColumn.table, quotaColumn.column))
	return nil
}

func quoteMainIdentifier(identifier string) string {
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
	}
	return "`" + strings.ReplaceAll(identifier, "`", "``") + "`"
}

func quoteSQLString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func quoteMySQLDefault(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.EqualFold(trimmed, "current_timestamp") || strings.HasPrefix(strings.ToUpper(trimmed), "CURRENT_TIMESTAMP(") {
		return trimmed
	}
	return quoteSQLString(value)
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
