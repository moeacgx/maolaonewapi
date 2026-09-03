package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTopUpAuditServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}))
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLog)
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestWaffoPancakeServiceResolutionPreservesTopUpAuditFlow(t *testing.T) {
	db := setupTopUpAuditServiceTestDB(t)
	user := &model.User{Id: 931, Username: "service-audit-user", Status: common.UserStatusEnabled, Quota: 2}
	require.NoError(t, db.Create(user).Error)
	topUp := &model.TopUp{
		UserId: user.Id, Amount: 2, Money: 2, CreditedQuota: 3, PaidAmountCNY: 2,
		TradeNo: "service-audit-order", PaymentMethod: model.PaymentMethodWaffoPancake,
		PaymentProvider: model.PaymentProviderWaffoPancake, CreateTime: common.GetTimestamp(),
		Status: common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())

	event := &WaffoPancakeWebhookEvent{Data: WaffoPancakeWebhookData{
		OrderMerchantExternalID:       topUp.TradeNo,
		MerchantProvidedBuyerIdentity: WaffoPancakeBuyerIdentityFromUserID(user.Id),
	}}
	tradeNo, err := ResolveWaffoPancakeTradeNo(event)
	require.NoError(t, err)
	require.NoError(t, model.RechargeWaffoPancake(tradeNo))

	var log model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeTopup).First(&log).Error)
	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(2), adminInfo["balance_before"])
	assert.Equal(t, float64(3), adminInfo["credited_quota"])
	assert.Equal(t, float64(5), adminInfo["balance_after"])
	assert.Equal(t, topUp.TradeNo, adminInfo["trade_no"])
}
