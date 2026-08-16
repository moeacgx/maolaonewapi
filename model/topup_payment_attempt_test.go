package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTopUpPaymentAttemptTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	originalDB := DB
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	require.NoError(t, db.AutoMigrate(&User{}, &TopUp{}, &TopUpPaymentAttempt{}))
	t.Cleanup(func() { DB = originalDB })
	return db
}

func TestTopUpPaymentAttemptsAreAppendOnlyAcrossRetries(t *testing.T) {
	db := setupTopUpPaymentAttemptTestDB(t)
	topUp := TopUp{UserId: 9, Amount: 10, Money: 72, TradeNo: "retry-order", PaymentMethod: PaymentMethodOkpay, PaymentProvider: PaymentProviderOkpay, CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending}
	require.NoError(t, db.Create(&topUp).Error)

	oldAttempt, err := CreateTopUpPaymentAttempt(topUp.TradeNo, PaymentProviderOkpay, PaymentMethodOkpay, "10.00000000", "USDT")
	require.NoError(t, err)
	require.NoError(t, MarkTopUpPaymentAttemptLaunched(oldAttempt.Id, "provider-old"))
	newAttempt, err := CreateTopUpPaymentAttempt(topUp.TradeNo, PaymentProviderOkpay, PaymentMethodOkpay, "9.50000000", "USDT")
	require.NoError(t, err)
	require.NoError(t, MarkTopUpPaymentAttemptLaunched(newAttempt.Id, "provider-new"))

	var attempts []TopUpPaymentAttempt
	require.NoError(t, db.Order("id").Find(&attempts).Error)
	require.Len(t, attempts, 2)
	require.Equal(t, "provider-old", attempts[0].ProviderOrderId)
	require.Equal(t, "10.00000000", attempts[0].ProviderAmount)
	require.Equal(t, "provider-new", attempts[1].ProviderOrderId)
	require.Equal(t, "9.50000000", attempts[1].ProviderAmount)

	resolvedOld, err := ResolveTopUpPaymentAttempt(PaymentProviderOkpay, topUp.TradeNo, "provider-old")
	require.NoError(t, err)
	require.Equal(t, oldAttempt.Id, resolvedOld.Id)
}

func TestUnboundLaunchFailureCanBeClaimedBySignedProviderCallback(t *testing.T) {
	db := setupTopUpPaymentAttemptTestDB(t)
	topUp := TopUp{UserId: 10, Amount: 10, Money: 72, TradeNo: "late-provider-order", PaymentMethod: PaymentMethodOkpay, PaymentProvider: PaymentProviderOkpay, CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending}
	require.NoError(t, db.Create(&topUp).Error)
	attempt, err := CreateTopUpPaymentAttempt(topUp.TradeNo, PaymentProviderOkpay, PaymentMethodOkpay, "10.00000000", "USDT")
	require.NoError(t, err)
	require.NoError(t, MarkTopUpPaymentAttemptLaunchFailed(attempt.Id, "transport timeout"))

	resolved, err := ResolveTopUpPaymentAttempt(PaymentProviderOkpay, topUp.TradeNo, "provider-late")
	require.NoError(t, err)
	require.Equal(t, attempt.Id, resolved.Id)
	require.NoError(t, ValidateTopUpPaymentAttemptSnapshot(resolved, PaymentProviderOkpay, "provider-late", "10.00000000", "USDT", decimal.Zero))
	require.NoError(t, BindTopUpPaymentAttemptProviderOrder(resolved.Id, "provider-late"))

	bound, err := ResolveTopUpPaymentAttempt(PaymentProviderOkpay, topUp.TradeNo, "provider-late")
	require.NoError(t, err)
	require.Equal(t, attempt.Id, bound.Id)
	require.Equal(t, "provider-late", bound.ProviderOrderId)
	require.ErrorIs(t, BindTopUpPaymentAttemptProviderOrder(bound.Id, "provider-other"), ErrTopUpPaymentAttemptMismatch)
}

func TestTopUpPaymentAttemptSnapshotRejectsProviderOrderAmountAndCurrencyMismatch(t *testing.T) {
	attempt := &TopUpPaymentAttempt{Id: 1, PaymentProvider: PaymentProviderOkpay, ProviderOrderId: "provider-1", ProviderAmount: "6.00000000", ProviderCurrency: "USDT"}

	require.NoError(t, ValidateTopUpPaymentAttemptSnapshot(attempt, PaymentProviderOkpay, "provider-1", "6.00000000", "usdt", decimal.Zero))
	require.ErrorIs(t, ValidateTopUpPaymentAttemptSnapshot(attempt, PaymentProviderBepusdt, "provider-1", "6.00000000", "USDT", decimal.Zero), ErrTopUpPaymentAttemptMismatch)
	require.ErrorIs(t, ValidateTopUpPaymentAttemptSnapshot(attempt, PaymentProviderOkpay, "provider-2", "6.00000000", "USDT", decimal.Zero), ErrTopUpPaymentAttemptMismatch)
	require.ErrorIs(t, ValidateTopUpPaymentAttemptSnapshot(attempt, PaymentProviderOkpay, "provider-1", "6.00000001", "USDT", decimal.Zero), ErrTopUpPaymentAttemptMismatch)
	require.ErrorIs(t, ValidateTopUpPaymentAttemptSnapshot(attempt, PaymentProviderOkpay, "provider-1", "6.00000000", "TRX", decimal.Zero), ErrTopUpPaymentAttemptMismatch)
}

func TestBepusdtAttemptAmountToleranceIsBounded(t *testing.T) {
	attempt := &TopUpPaymentAttempt{Id: 2, PaymentProvider: PaymentProviderBepusdt, ProviderOrderId: "trade-1", ProviderAmount: "72.00", ProviderCurrency: "CNY"}

	require.NoError(t, ValidateTopUpPaymentAttemptSnapshot(attempt, PaymentProviderBepusdt, "trade-1", "72.01", "CNY", decimal.NewFromFloat(0.01)))
	require.ErrorIs(t, ValidateTopUpPaymentAttemptSnapshot(attempt, PaymentProviderBepusdt, "trade-1", "72.02", "CNY", decimal.NewFromFloat(0.01)), ErrTopUpPaymentAttemptMismatch)
}

func TestCompleteTopUpPaymentAttemptRetainsAtomicMaxQuotaGuard(t *testing.T) {
	db := setupTopUpPaymentAttemptTestDB(t)
	user := User{Id: 19, Username: "attempt-limit-user", Quota: common.MaxQuota - 5, Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, db.Create(&user).Error)
	topUp := TopUp{UserId: user.Id, Amount: 1, Money: 7.2, CreditedQuota: 10, TradeNo: "limit-order", PaymentMethod: PaymentMethodOkpay, PaymentProvider: PaymentProviderOkpay, CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending}
	require.NoError(t, db.Create(&topUp).Error)
	attempt, err := CreateTopUpPaymentAttempt(topUp.TradeNo, PaymentProviderOkpay, PaymentMethodOkpay, "1.00000000", "USDT")
	require.NoError(t, err)
	require.NoError(t, MarkTopUpPaymentAttemptLaunched(attempt.Id, "provider-limit"))

	_, err = CompleteTopUpPaymentAttempt(attempt.Id, topUp.TradeNo, PaymentProviderOkpay, PaymentMethodOkpay, "127.0.0.1")

	require.ErrorIs(t, err, ErrTopUpQuotaLimitExceeded)
	require.NoError(t, db.First(&user, user.Id).Error)
	require.Equal(t, common.MaxQuota-5, user.Quota)
	require.NoError(t, db.First(&topUp, topUp.Id).Error)
	require.Equal(t, common.TopUpStatusPending, topUp.Status)
	require.NoError(t, db.First(&attempt, attempt.Id).Error)
	require.Equal(t, TopUpPaymentAttemptLaunched, attempt.Status)
}
