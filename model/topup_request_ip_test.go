package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTopUpRequestIPTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := DB
	originalLogDB := LOG_DB
	originalRedis := common.RedisEnabled
	originalMainDatabase := common.MainDatabaseType()
	originalLogDatabase := common.LogDatabaseType()

	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}, &TopUp{}, &TopUpPaymentAttempt{}))

	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		DB = originalDB
		LOG_DB = originalLogDB
		common.RedisEnabled = originalRedis
		common.SetDatabaseTypes(originalMainDatabase, originalLogDatabase)
	})
	return db
}

func topupLogAdminInfo(t *testing.T, log Log) map[string]interface{} {
	t.Helper()
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	return adminInfo
}

func TestRecordTopupOrderLogUsesRequestIP(t *testing.T) {
	db := setupTopUpRequestIPTestDB(t)
	require.NoError(t, db.Create(&User{Id: 1101, Username: "topup-ip-user"}).Error)
	topUp := &TopUp{
		UserId:          1101,
		TradeNo:         "TOPUP-IP-ORDER",
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		RequestIP:       "203.0.113.10",
	}

	RecordTopupOrderLog(topUp, "充值成功", PaymentProviderEpay, "198.51.100.20")

	var log Log
	require.NoError(t, db.Where("user_id = ? AND type = ?", topUp.UserId, LogTypeTopup).First(&log).Error)
	assert.Equal(t, "203.0.113.10", log.Ip)

	adminInfo := topupLogAdminInfo(t, log)
	assert.Equal(t, "203.0.113.10", adminInfo["caller_ip"])
	assert.Equal(t, "203.0.113.10", adminInfo["request_ip"])
	assert.Equal(t, "198.51.100.20", adminInfo["callback_ip"])
	assert.Equal(t, topUp.TradeNo, adminInfo["trade_no"])
}

func TestRecordTopupLogDoesNotUseCallbackIP(t *testing.T) {
	db := setupTopUpRequestIPTestDB(t)
	require.NoError(t, db.Create(&User{Id: 1102, Username: "legacy-topup-ip-user"}).Error)

	RecordTopupLog(1102, "充值成功", "", "alipay", PaymentProviderEpay, "198.51.100.21")

	var log Log
	require.NoError(t, db.Where("user_id = ? AND type = ?", 1102, LogTypeTopup).First(&log).Error)
	assert.Empty(t, log.Ip)

	adminInfo := topupLogAdminInfo(t, log)
	assert.Empty(t, adminInfo["caller_ip"])
	assert.Empty(t, adminInfo["request_ip"])
	assert.Equal(t, "198.51.100.21", adminInfo["callback_ip"])
}

func TestCompleteTopUpPaymentAttemptLogsOrderRequestIP(t *testing.T) {
	db := setupTopUpRequestIPTestDB(t)
	user := User{Id: 1103, Username: "attempt-topup-ip-user", Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, db.Create(&user).Error)
	topUp := TopUp{
		UserId:          user.Id,
		Amount:          1,
		Money:           7.2,
		CreditedQuota:   10,
		TradeNo:         "ATTEMPT-IP-ORDER",
		PaymentMethod:   PaymentMethodOkpay,
		PaymentProvider: PaymentProviderOkpay,
		RequestIP:       "203.0.113.30",
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(&topUp).Error)
	attempt, err := CreateTopUpPaymentAttempt(topUp.TradeNo, PaymentProviderOkpay, PaymentMethodOkpay, "1.00000000", "USDT")
	require.NoError(t, err)
	require.NoError(t, MarkTopUpPaymentAttemptLaunched(attempt.Id, "provider-ip-order"))

	alreadyDone, err := CompleteTopUpPaymentAttempt(attempt.Id, topUp.TradeNo, PaymentProviderOkpay, PaymentMethodOkpay, "198.51.100.30")

	require.NoError(t, err)
	assert.False(t, alreadyDone)
	var log Log
	require.NoError(t, db.Where("user_id = ? AND type = ?", user.Id, LogTypeTopup).First(&log).Error)
	assert.Equal(t, "203.0.113.30", log.Ip)

	adminInfo := topupLogAdminInfo(t, log)
	assert.Equal(t, "203.0.113.30", adminInfo["request_ip"])
	assert.Equal(t, "198.51.100.30", adminInfo["callback_ip"])
}

func TestCompleteTopUpPaymentAttemptDoesNotBackfillRequestIPFromCallback(t *testing.T) {
	db := setupTopUpRequestIPTestDB(t)
	user := User{Id: 1104, Username: "empty-request-ip-user", Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, db.Create(&user).Error)
	topUp := TopUp{
		UserId:          user.Id,
		Amount:          1,
		Money:           7.2,
		CreditedQuota:   10,
		TradeNo:         "EMPTY-REQUEST-IP-ORDER",
		PaymentMethod:   PaymentMethodOkpay,
		PaymentProvider: PaymentProviderOkpay,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(&topUp).Error)
	attempt, err := CreateTopUpPaymentAttempt(topUp.TradeNo, PaymentProviderOkpay, PaymentMethodOkpay, "1.00000000", "USDT")
	require.NoError(t, err)
	require.NoError(t, MarkTopUpPaymentAttemptLaunched(attempt.Id, "provider-empty-request-ip"))

	alreadyDone, err := CompleteTopUpPaymentAttempt(attempt.Id, topUp.TradeNo, PaymentProviderOkpay, PaymentMethodOkpay, "198.51.100.31")

	require.NoError(t, err)
	assert.False(t, alreadyDone)
	var saved TopUp
	require.NoError(t, db.Where("trade_no = ?", topUp.TradeNo).First(&saved).Error)
	assert.Empty(t, saved.RequestIP)
	var log Log
	require.NoError(t, db.Where("user_id = ? AND type = ?", user.Id, LogTypeTopup).First(&log).Error)
	assert.Empty(t, log.Ip)

	adminInfo := topupLogAdminInfo(t, log)
	assert.Empty(t, adminInfo["request_ip"])
	assert.Equal(t, "198.51.100.31", adminInfo["callback_ip"])
}

func TestManualCompleteTopUpLogsOrderRequestIPAndAdminIPSeparately(t *testing.T) {
	db := setupTopUpRequestIPTestDB(t)
	user := User{Id: 1105, Username: "manual-topup-ip-user", Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, db.Create(&user).Error)
	topUp := TopUp{
		UserId:          user.Id,
		Amount:          1,
		Money:           7.2,
		CreditedQuota:   10,
		TradeNo:         "MANUAL-IP-ORDER",
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		RequestIP:       "203.0.113.40",
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(&topUp).Error)

	require.NoError(t, ManualCompleteTopUp(topUp.TradeNo, "198.51.100.40"))

	var log Log
	require.NoError(t, db.Where("user_id = ? AND type = ?", user.Id, LogTypeTopup).First(&log).Error)
	assert.Equal(t, "203.0.113.40", log.Ip)

	adminInfo := topupLogAdminInfo(t, log)
	assert.Equal(t, "203.0.113.40", adminInfo["request_ip"])
	assert.Equal(t, "198.51.100.40", adminInfo["callback_ip"])
	assert.Equal(t, topUp.TradeNo, adminInfo["trade_no"])
}
