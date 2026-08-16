package model

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func withSubscriptionRefundSQLiteDB(t *testing.T) {
	t.Helper()
	originalDB := DB
	t.Cleanup(func() {
		DB = originalDB
	})

	dsn := filepath.Join(t.TempDir(), "subscription-refund.db") + "?_busy_timeout=100"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(2)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	DB = db
	require.NoError(t, DB.AutoMigrate(&UserSubscription{}, &SubscriptionPreConsumeRecord{}, &SubscriptionOrder{}))
}

func TestRefundSubscriptionPreConsumeUsesSingleTransaction(t *testing.T) {
	withSubscriptionRefundSQLiteDB(t)

	sub := UserSubscription{
		UserId:      1,
		PlanId:      1,
		AmountTotal: 1000,
		AmountUsed:  500,
		Status:      "active",
	}
	require.NoError(t, DB.Create(&sub).Error)
	require.NoError(t, DB.Create(&SubscriptionPreConsumeRecord{
		RequestId:          "refund-single-tx",
		UserId:             1,
		UserSubscriptionId: sub.Id,
		PreConsumed:        200,
		Status:             "consumed",
	}).Error)

	require.NoError(t, RefundSubscriptionPreConsume("refund-single-tx"))

	var gotSub UserSubscription
	require.NoError(t, DB.First(&gotSub, sub.Id).Error)
	require.EqualValues(t, 300, gotSub.AmountUsed)

	var gotRecord SubscriptionPreConsumeRecord
	require.NoError(t, DB.Where("request_id = ?", "refund-single-tx").First(&gotRecord).Error)
	require.Equal(t, "refunded", gotRecord.Status)
}

func TestUpdateSubscriptionOrderProviderSnapshotIsImmutable(t *testing.T) {
	withSubscriptionRefundSQLiteDB(t)
	order := &SubscriptionOrder{TradeNo: "snapshot-bind", PaymentProvider: PaymentProviderOkpay, Status: "pending"}
	require.NoError(t, DB.Create(order).Error)
	require.NoError(t, UpdateSubscriptionOrderProviderSnapshot(order.TradeNo, PaymentProviderOkpay, "provider-1", "10.00000000", "USDT"))
	require.Error(t, UpdateSubscriptionOrderProviderSnapshot(order.TradeNo, PaymentProviderOkpay, "provider-2", "11.00000000", "USDT"))
	var got SubscriptionOrder
	require.NoError(t, DB.First(&got, order.Id).Error)
	require.Equal(t, "provider-1", got.ProviderOrderId)
	require.Equal(t, "10.00000000", got.ProviderAmount)
	require.Equal(t, "USDT", got.ProviderCurrency)
}
