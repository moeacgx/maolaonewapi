package model

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/utils/tests"
)

func newLifecyclePromoCode(code string) *PromoCode {
	return &PromoCode{
		Name:           "lifecycle promo",
		Code:           code,
		Status:         common.RedemptionCodeStatusEnabled,
		DiscountType:   PromoCodeDiscountTypePercent,
		DiscountValue:  10,
		AppliesToTopup: true,
		MaxRedeemCount: 10,
	}
}

func TestPromoCodeDeleteAllowsCodeReuseAndKeepsHistory(t *testing.T) {
	truncateTables(t)

	original := newLifecyclePromoCode("REUSE_ME")
	require.NoError(t, original.Insert())
	require.NoError(t, DeletePromoCodeById(original.Id))

	recreated := newLifecyclePromoCode("reuse_me")
	require.NoError(t, recreated.Insert())
	assert.NotEqual(t, original.Id, recreated.Id)
	assert.Equal(t, "REUSE_ME", recreated.Code)

	var archived PromoCode
	require.NoError(t, DB.Unscoped().First(&archived, original.Id).Error)
	assert.True(t, archived.DeletedAt.Valid)
	assert.Equal(t, "REUSE_ME", archived.Code)
	assert.Equal(t, original.Id, archived.DeletedId)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return recordPromoCodeUsageTx(
			tx, original.Id, 901, PromoCodeTargetTopUp, "deleted-promo-order",
			100, 10, 90, false,
		)
	}))

	var usage PromoCodeUsage
	require.NoError(t, DB.Where("promo_code_id = ?", original.Id).First(&usage).Error)
	assert.Equal(t, "deleted-promo-order", usage.OrderNo)
	require.NoError(t, DB.Unscoped().First(&archived, original.Id).Error)
	assert.Equal(t, 1, archived.RedeemedCount)
	assert.True(t, archived.DeletedAt.Valid)
	var active PromoCode
	require.NoError(t, DB.First(&active, recreated.Id).Error)
	assert.Zero(t, active.RedeemedCount)

	require.NoError(t, DeletePromoCodeById(recreated.Id))
	third := newLifecyclePromoCode("REUSE_ME")
	require.NoError(t, third.Insert())
	assert.NotEqual(t, recreated.Id, third.Id)
	var secondArchived PromoCode
	require.NoError(t, DB.Unscoped().First(&secondArchived, recreated.Id).Error)
	assert.Equal(t, "REUSE_ME", secondArchived.Code)
	assert.Equal(t, recreated.Id, secondArchived.DeletedId)
}

func TestDeletePromoCodesByIDsArchivesSelectedCodes(t *testing.T) {
	truncateTables(t)
	first := newLifecyclePromoCode("BATCH_DELETE_FIRST")
	second := newLifecyclePromoCode("BATCH_DELETE_SECOND")
	require.NoError(t, first.Insert())
	require.NoError(t, second.Insert())

	deleted, err := DeletePromoCodesByIDs([]int{first.Id, second.Id, 999999})
	require.NoError(t, err)
	assert.EqualValues(t, 2, deleted)

	var archived []PromoCode
	require.NoError(t, DB.Unscoped().Where("id IN ?", []int{first.Id, second.Id}).Find(&archived).Error)
	require.Len(t, archived, 2)
	for _, promo := range archived {
		assert.True(t, promo.DeletedAt.Valid)
		assert.Equal(t, promo.Id, promo.DeletedId)
	}
}

func TestPromoCodeInsertRepairsLegacySoftDeletedCollision(t *testing.T) {
	truncateTables(t)

	legacy := newLifecyclePromoCode("LEGACY_REUSE")
	require.NoError(t, legacy.Insert())
	// 模拟旧版本只写 deleted_at、没有释放唯一 code 的记录。
	require.NoError(t, DB.Delete(&PromoCode{}, legacy.Id).Error)

	recreated := newLifecyclePromoCode("legacy_reuse")
	require.NoError(t, recreated.Insert())
	assert.Equal(t, "LEGACY_REUSE", recreated.Code)

	var archived PromoCode
	require.NoError(t, DB.Unscoped().First(&archived, legacy.Id).Error)
	assert.True(t, archived.DeletedAt.Valid)
	assert.Equal(t, "LEGACY_REUSE", archived.Code)
	assert.Equal(t, legacy.Id, archived.DeletedId)

	var activeCount int64
	require.NoError(t, DB.Model(&PromoCode{}).Where("code = ?", "LEGACY_REUSE").Count(&activeCount).Error)
	assert.EqualValues(t, 1, activeCount)
}

func TestPromoCodeInsertRejectsActiveDuplicate(t *testing.T) {
	truncateTables(t)

	require.NoError(t, newLifecyclePromoCode("ACTIVE_DUPLICATE").Insert())
	err := newLifecyclePromoCode("active_duplicate").Insert()
	require.EqualError(t, err, "优惠码已存在")

	var activeCount int64
	require.NoError(t, DB.Model(&PromoCode{}).Where("code = ?", "ACTIVE_DUPLICATE").Count(&activeCount).Error)
	assert.EqualValues(t, 1, activeCount)
}

func TestPromoCodeUpdateCanReuseLegacySoftDeletedCode(t *testing.T) {
	truncateTables(t)

	legacy := newLifecyclePromoCode("LEGACY_UPDATE")
	require.NoError(t, legacy.Insert())
	require.NoError(t, DB.Delete(&PromoCode{}, legacy.Id).Error)

	current := newLifecyclePromoCode("CURRENT_CODE")
	require.NoError(t, current.Insert())
	current.Code = "legacy_update"
	require.NoError(t, current.Update())
	assert.Equal(t, "LEGACY_UPDATE", current.Code)

	var archived PromoCode
	require.NoError(t, DB.Unscoped().First(&archived, legacy.Id).Error)
	assert.Equal(t, "LEGACY_UPDATE", archived.Code)
	assert.Equal(t, legacy.Id, archived.DeletedId)

	var saved PromoCode
	require.NoError(t, DB.First(&saved, current.Id).Error)
	assert.Equal(t, "LEGACY_UPDATE", saved.Code)
}

type legacyPromoCodeMigration struct {
	Id        int
	Code      string         `gorm:"type:varchar(64);uniqueIndex"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (legacyPromoCodeMigration) TableName() string {
	return "promo_codes"
}

func TestMigratePromoCodeDeletionKeyPreservesCodeAndReleasesUniqueValue(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&legacyPromoCodeMigration{}))

	legacy := &legacyPromoCodeMigration{Code: "MIGRATION_REUSE"}
	require.NoError(t, db.Create(legacy).Error)
	require.NoError(t, db.Delete(legacy).Error)

	require.NoError(t, db.AutoMigrate(&PromoCode{}))
	require.NoError(t, migratePromoCodeDeletionKey(db))
	require.NoError(t, migratePromoCodeDeletionKey(db))
	assert.True(t, db.Migrator().HasIndex(&PromoCode{}, promoCodeUniqueIndex))
	assert.False(t, db.Migrator().HasIndex(&PromoCode{}, promoCodeLegacyCodeIndex))

	var migrated PromoCode
	require.NoError(t, db.Unscoped().First(&migrated, legacy.Id).Error)
	assert.Equal(t, "MIGRATION_REUSE", migrated.Code)
	assert.Equal(t, legacy.Id, migrated.DeletedId)

	active := newLifecyclePromoCode("MIGRATION_REUSE")
	require.NoError(t, db.Create(active).Error)
	require.Error(t, db.Create(newLifecyclePromoCode("MIGRATION_REUSE")).Error)
}

func TestMigratePromoCodeDeletionKeyDropsPostgreSQLLegacyConstraint(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("NEW_API_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("NEW_API_TEST_POSTGRES_DSN is not configured")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	var databaseName string
	require.NoError(t, db.Raw("SELECT current_database()").Scan(&databaseName).Error)
	require.True(t, strings.HasPrefix(databaseName, "newapi_test_"), "refusing destructive migration test against database %q", databaseName)

	require.NoError(t, db.Migrator().DropTable(&PromoCode{}))
	t.Cleanup(func() {
		_ = db.Migrator().DropTable(&PromoCode{})
	})
	require.NoError(t, db.Exec(`
		CREATE TABLE promo_codes (
			id BIGSERIAL PRIMARY KEY,
			code VARCHAR(64),
			deleted_at TIMESTAMPTZ,
			CONSTRAINT idx_promo_codes_code UNIQUE (code)
		)
	`).Error)
	require.NoError(t, db.Exec("INSERT INTO promo_codes (code, deleted_at) VALUES (?, NOW())", "PG_MIGRATION_REUSE").Error)
	require.NoError(t, db.AutoMigrate(&PromoCode{}))
	require.True(t, db.Migrator().HasConstraint(&PromoCode{}, promoCodeLegacyCodeIndex))

	require.NoError(t, migratePromoCodeDeletionKey(db))
	require.NoError(t, migratePromoCodeDeletionKey(db))
	assert.False(t, db.Migrator().HasConstraint(&PromoCode{}, promoCodeLegacyCodeIndex))
	assert.False(t, db.Migrator().HasIndex(&PromoCode{}, promoCodeLegacyCodeIndex))
	assert.True(t, db.Migrator().HasIndex(&PromoCode{}, promoCodeUniqueIndex))

	var migrated PromoCode
	require.NoError(t, db.Unscoped().Where("code = ?", "PG_MIGRATION_REUSE").First(&migrated).Error)
	assert.Equal(t, migrated.Id, migrated.DeletedId)
	require.NoError(t, db.Create(newLifecyclePromoCode("PG_MIGRATION_REUSE")).Error)
	require.Error(t, db.Create(newLifecyclePromoCode("PG_MIGRATION_REUSE")).Error)
}

func newCapacityPromoCode(code string, maxRedeemCount int, discountValue int64) *PromoCode {
	promo := newLifecyclePromoCode(code)
	promo.MaxRedeemCount = maxRedeemCount
	promo.DiscountValue = discountValue
	return promo
}

func promoTopUpFromDiscount(discount *PromoCodeDiscountResult, tradeNo string, userId int) *TopUp {
	topUp := &TopUp{
		UserId:          userId,
		Amount:          100,
		Money:           100,
		CreditedQuota:   1000,
		TradeNo:         tradeNo,
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	ApplyPromoCodeResultToTopUp(topUp, discount)
	return topUp
}

func TestPromoCodeCapacitySequentialExhaustion(t *testing.T) {
	truncateTables(t)
	promo := newCapacityPromoCode("SEQUENTIAL_CAPACITY", 1, 50)
	require.NoError(t, promo.Insert())

	firstDiscount, err := CalculatePromoCodeDiscount(promo.Code, PromoCodeTargetTopUp, 0, 100)
	require.NoError(t, err)
	secondDiscount, err := CalculatePromoCodeDiscount(promo.Code, PromoCodeTargetTopUp, 0, 100)
	require.NoError(t, err)

	require.NoError(t, promoTopUpFromDiscount(firstDiscount, "promo-sequential-1", 901).Insert())
	err = promoTopUpFromDiscount(secondDiscount, "promo-sequential-2", 902).Insert()
	require.ErrorContains(t, err, "使用次数上限")

	var stored PromoCode
	require.NoError(t, DB.First(&stored, promo.Id).Error)
	assert.Equal(t, 1, stored.ReservedCount)
	assert.Zero(t, stored.RedeemedCount)
}

func TestPromoCodeCapacityConcurrentLastSlot(t *testing.T) {
	truncateTables(t)
	promo := newCapacityPromoCode("CONCURRENT_CAPACITY", 1, 50)
	require.NoError(t, promo.Insert())

	discount, err := CalculatePromoCodeDiscount(promo.Code, PromoCodeTargetTopUp, 0, 100)
	require.NoError(t, err)
	orders := []*TopUp{
		promoTopUpFromDiscount(discount, "promo-concurrent-1", 903),
		promoTopUpFromDiscount(discount, "promo-concurrent-2", 904),
	}

	start := make(chan struct{})
	results := make(chan error, len(orders))
	var wg sync.WaitGroup
	for _, order := range orders {
		wg.Add(1)
		go func(topUp *TopUp) {
			defer wg.Done()
			<-start
			results <- topUp.Insert()
		}(order)
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	failures := 0
	for insertErr := range results {
		if insertErr == nil {
			successes++
		} else {
			failures++
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, failures)

	var stored PromoCode
	require.NoError(t, DB.First(&stored, promo.Id).Error)
	assert.Equal(t, 1, stored.ReservedCount)
	var orderCount int64
	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no IN ?", []string{"promo-concurrent-1", "promo-concurrent-2"}).Count(&orderCount).Error)
	assert.EqualValues(t, 1, orderCount)
}

func TestPromoCodeDuplicatePaidCallbackSettlesOnce(t *testing.T) {
	truncateTables(t)
	user := &User{Id: 905, Username: "promo-callback-user", Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, DB.Create(user).Error)
	promo := newCapacityPromoCode("DUPLICATE_CALLBACK", 1, 50)
	require.NoError(t, promo.Insert())
	discount, err := CalculatePromoCodeDiscount(promo.Code, PromoCodeTargetTopUp, 0, 100)
	require.NoError(t, err)
	topUp := promoTopUpFromDiscount(discount, "promo-duplicate-callback", user.Id)
	require.NoError(t, topUp.Insert())

	alreadyDone, err := RechargeEpay(topUp.TradeNo, "alipay")
	require.NoError(t, err)
	assert.False(t, alreadyDone)
	alreadyDone, err = RechargeEpay(topUp.TradeNo, "alipay")
	require.NoError(t, err)
	assert.True(t, alreadyDone)

	var stored PromoCode
	require.NoError(t, DB.First(&stored, promo.Id).Error)
	assert.Equal(t, 1, stored.RedeemedCount)
	assert.Zero(t, stored.ReservedCount)
	var usageCount int64
	require.NoError(t, DB.Model(&PromoCodeUsage{}).Where("order_no = ?", topUp.TradeNo).Count(&usageCount).Error)
	assert.EqualValues(t, 1, usageCount)
	var storedUser User
	require.NoError(t, DB.First(&storedUser, user.Id).Error)
	assert.Equal(t, topUp.CreditedQuota, storedUser.Quota)
}

func TestPromoCodeFailedAndExpiredOrdersReleaseOnce(t *testing.T) {
	truncateTables(t)
	t.Run("failed topup", func(t *testing.T) {
		promo := newCapacityPromoCode("FAILED_RELEASE", 1, 25)
		require.NoError(t, promo.Insert())
		discount, err := CalculatePromoCodeDiscount(promo.Code, PromoCodeTargetTopUp, 0, 100)
		require.NoError(t, err)
		topUp := promoTopUpFromDiscount(discount, "promo-failed-release", 906)
		require.NoError(t, topUp.Insert())
		require.NoError(t, UpdatePendingTopUpStatus(topUp.TradeNo, PaymentProviderEpay, common.TopUpStatusFailed))
		require.ErrorIs(t, UpdatePendingTopUpStatus(topUp.TradeNo, PaymentProviderEpay, common.TopUpStatusFailed), ErrTopUpStatusInvalid)

		var stored PromoCode
		require.NoError(t, DB.First(&stored, promo.Id).Error)
		assert.Zero(t, stored.ReservedCount)
		replacement := promoTopUpFromDiscount(discount, "promo-failed-replacement", 907)
		require.NoError(t, replacement.Insert())
	})

	t.Run("expired subscription", func(t *testing.T) {
		promo := newCapacityPromoCode("EXPIRED_RELEASE", 1, 25)
		promo.AppliesToTopup = false
		promo.AppliesToAllSubscription = true
		require.NoError(t, promo.Insert())
		discount, err := CalculatePromoCodeDiscount(promo.Code, PromoCodeTargetSubscription, 77, 100)
		require.NoError(t, err)
		order := &SubscriptionOrder{UserId: 908, PlanId: 77, Money: 100, TradeNo: "promo-expired-release", PaymentProvider: PaymentProviderStripe, PaymentMethod: PaymentMethodStripe, Status: common.TopUpStatusPending}
		ApplyPromoCodeResultToSubscriptionOrder(order, discount)
		require.NoError(t, order.Insert())
		require.NoError(t, ExpireSubscriptionOrder(order.TradeNo, PaymentProviderStripe))
		require.NoError(t, ExpireSubscriptionOrder(order.TradeNo, PaymentProviderStripe))

		var stored PromoCode
		require.NoError(t, DB.First(&stored, promo.Id).Error)
		assert.Zero(t, stored.ReservedCount)
		var reservation PromoCodeReservation
		require.NoError(t, promoReservationQuery(DB, promo.Id, PromoCodeTargetSubscription, order.TradeNo).First(&reservation).Error)
		assert.Equal(t, promoReservationStatusReleased, reservation.Status)
	})
}

func TestPromoCodeZeroPriceOrderReservesAndSettles(t *testing.T) {
	truncateTables(t)
	user := &User{Id: 909, Username: "promo-free-user", Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, DB.Create(user).Error)
	promo := newCapacityPromoCode("ZERO_PRICE", 1, 100)
	require.NoError(t, promo.Insert())
	discount, err := CalculatePromoCodeDiscount(promo.Code, PromoCodeTargetTopUp, 0, 100)
	require.NoError(t, err)
	topUp := promoTopUpFromDiscount(discount, "promo-zero-price", user.Id)
	require.NoError(t, topUp.Insert())

	_, _, completedNow, err := CompleteFreeTopUp(topUp.TradeNo, PaymentProviderEpay)
	require.NoError(t, err)
	assert.True(t, completedNow)
	_, _, completedNow, err = CompleteFreeTopUp(topUp.TradeNo, PaymentProviderEpay)
	require.NoError(t, err)
	assert.False(t, completedNow)
	var topupLogCount int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("type = ?", LogTypeTopup).Count(&topupLogCount).Error)
	assert.Equal(t, int64(1), topupLogCount)
	audit := readTopUpAuditInfo(t, topUp.TradeNo)
	assert.Equal(t, float64(0), audit["balance_before"])
	assert.Equal(t, float64(topUp.CreditedQuota), audit["credited_quota"])
	assert.Equal(t, float64(topUp.CreditedQuota), audit["balance_after"])
	assert.Equal(t, topUp.TradeNo, audit["trade_no"])

	var stored PromoCode
	require.NoError(t, DB.First(&stored, promo.Id).Error)
	assert.Equal(t, 1, stored.RedeemedCount)
	assert.Zero(t, stored.ReservedCount)
	var reservation PromoCodeReservation
	require.NoError(t, promoReservationQuery(DB, promo.Id, PromoCodeTargetTopUp, topUp.TradeNo).First(&reservation).Error)
	assert.Equal(t, promoReservationStatusSettled, reservation.Status)
}

func TestPromoCodeGenericUpdateCannotForgeLifecycleCounts(t *testing.T) {
	truncateTables(t)
	promo := newCapacityPromoCode("FORGED_COUNT", 10, 10)
	require.NoError(t, promo.Insert())
	discount, err := CalculatePromoCodeDiscount(promo.Code, PromoCodeTargetTopUp, 0, 100)
	require.NoError(t, err)
	require.NoError(t, promoTopUpFromDiscount(discount, "promo-forged-count", 910).Insert())

	forged := *promo
	forged.Name = "updated without forging"
	forged.RedeemedCount = 99
	forged.ReservedCount = 99
	require.NoError(t, forged.Update())

	var stored PromoCode
	require.NoError(t, DB.First(&stored, promo.Id).Error)
	assert.Equal(t, "updated without forging", stored.Name)
	assert.Zero(t, stored.RedeemedCount)
	assert.Equal(t, 1, stored.ReservedCount)
}

func TestPromoCodeCallbackEligibleFailedLaunchRetainsAndSettles(t *testing.T) {
	truncateTables(t)
	userA := &User{Id: 920, Username: "promo-retained-topup-a", Status: common.UserStatusEnabled, Group: "default", AffCode: "promo-retained-topup-aff-a"}
	require.NoError(t, DB.Create(userA).Error)
	promo := newCapacityPromoCode("RETAINED_TOPUP_CAPACITY", 1, 50)
	require.NoError(t, promo.Insert())
	discountA, err := CalculatePromoCodeDiscount(promo.Code, PromoCodeTargetTopUp, 0, 100)
	require.NoError(t, err)
	discountB, err := CalculatePromoCodeDiscount(promo.Code, PromoCodeTargetTopUp, 0, 100)
	require.NoError(t, err)

	topUpA := promoTopUpFromDiscount(discountA, "promo-retained-topup-a", userA.Id)
	topUpA.PaymentMethod = PaymentMethodOkpay
	topUpA.PaymentProvider = PaymentProviderOkpay
	require.NoError(t, topUpA.Insert())
	attemptA, err := CreateTopUpPaymentAttempt(topUpA.TradeNo, PaymentProviderOkpay, PaymentMethodOkpay, "50.00", "USD")
	require.NoError(t, err)
	require.NoError(t, MarkTopUpPaymentAttemptLaunchFailed(attemptA.Id, "ambiguous launch failure"))
	require.NoError(t, UpdatePendingTopUpStatus(topUpA.TradeNo, PaymentProviderOkpay, common.TopUpStatusFailed))
	require.NoError(t, DB.Model(&PromoCodeReservation{}).
		Where("promo_code_id = ? AND order_no = ?", promo.Id, topUpA.TradeNo).
		Update("updated_time", common.GetTimestamp()-promoReservationTTLSeconds-1).Error)

	var reservation PromoCodeReservation
	require.NoError(t, promoReservationQuery(DB, promo.Id, PromoCodeTargetTopUp, topUpA.TradeNo).First(&reservation).Error)
	assert.Equal(t, promoReservationStatusReserved, reservation.Status)
	var reservedPromo PromoCode
	require.NoError(t, DB.First(&reservedPromo, promo.Id).Error)
	assert.Equal(t, 1, reservedPromo.ReservedCount)

	topUpB := promoTopUpFromDiscount(discountB, "promo-retained-topup-b", 921)
	topUpB.PaymentMethod = PaymentMethodOkpay
	topUpB.PaymentProvider = PaymentProviderOkpay
	require.ErrorContains(t, topUpB.Insert(), "使用次数上限")

	alreadyDone, err := CompleteTopUpPaymentAttempt(attemptA.Id, topUpA.TradeNo, PaymentProviderOkpay, PaymentMethodOkpay)
	require.NoError(t, err)
	assert.False(t, alreadyDone)
	var storedA TopUp
	require.NoError(t, DB.Where("trade_no = ?", topUpA.TradeNo).First(&storedA).Error)
	assert.Equal(t, common.TopUpStatusSuccess, storedA.Status)
	var storedUserA User
	require.NoError(t, DB.First(&storedUserA, userA.Id).Error)
	assert.Equal(t, topUpA.CreditedQuota, storedUserA.Quota)
	var storedPromo PromoCode
	require.NoError(t, DB.First(&storedPromo, promo.Id).Error)
	assert.Equal(t, 1, storedPromo.RedeemedCount)
	assert.Zero(t, storedPromo.ReservedCount)
}

func TestPromoCodeSnapshottedExpiredSubscriptionRetainsAndSettles(t *testing.T) {
	truncateTables(t)
	userA := &User{Id: 922, Username: "promo-retained-sub-a", Status: common.UserStatusEnabled, Group: "default", AffCode: "promo-retained-sub-aff-a"}
	require.NoError(t, DB.Create(userA).Error)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 920)
	promo := newCapacityPromoCode("RETAINED_SUB_CAPACITY", 1, 50)
	promo.AppliesToTopup = false
	promo.AppliesToAllSubscription = true
	require.NoError(t, promo.Insert())
	discountA, err := CalculatePromoCodeDiscount(promo.Code, PromoCodeTargetSubscription, plan.Id, 100)
	require.NoError(t, err)
	discountB, err := CalculatePromoCodeDiscount(promo.Code, PromoCodeTargetSubscription, plan.Id, 100)
	require.NoError(t, err)

	orderA := &SubscriptionOrder{
		UserId: userA.Id, PlanId: plan.Id, Money: 100, TradeNo: "promo-retained-sub-a",
		PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending,
		ProviderOrderId: "promo-provider-sub-a", ProviderAmount: "50.00", ProviderCurrency: SubscriptionCurrencyUSD,
	}
	ApplyPromoCodeResultToSubscriptionOrder(orderA, discountA)
	require.NoError(t, orderA.Insert())
	require.NoError(t, ExpireSubscriptionOrder(orderA.TradeNo, PaymentProviderStripe))
	require.NoError(t, DB.Model(&PromoCodeReservation{}).
		Where("promo_code_id = ? AND order_no = ?", promo.Id, orderA.TradeNo).
		Update("updated_time", common.GetTimestamp()-promoReservationTTLSeconds-1).Error)

	var reservation PromoCodeReservation
	require.NoError(t, promoReservationQuery(DB, promo.Id, PromoCodeTargetSubscription, orderA.TradeNo).First(&reservation).Error)
	assert.Equal(t, promoReservationStatusReserved, reservation.Status)
	orderB := &SubscriptionOrder{UserId: 923, PlanId: plan.Id, Money: 100, TradeNo: "promo-retained-sub-b", PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending}
	ApplyPromoCodeResultToSubscriptionOrder(orderB, discountB)
	require.ErrorContains(t, orderB.Insert(), "使用次数上限")

	require.NoError(t, CompleteSubscriptionOrder(orderA.TradeNo, "", PaymentProviderStripe, PaymentMethodStripe))
	var storedA SubscriptionOrder
	require.NoError(t, DB.Where("trade_no = ?", orderA.TradeNo).First(&storedA).Error)
	assert.Equal(t, common.TopUpStatusSuccess, storedA.Status)
	var usageCount int64
	require.NoError(t, DB.Model(&PromoCodeUsage{}).Where("promo_code_id = ?", promo.Id).Count(&usageCount).Error)
	assert.EqualValues(t, 1, usageCount)
	var storedPromo PromoCode
	require.NoError(t, DB.First(&storedPromo, promo.Id).Error)
	assert.Equal(t, 1, storedPromo.RedeemedCount)
	assert.Zero(t, storedPromo.ReservedCount)
}

func TestPromoCodeStalePendingReservationIsReclaimed(t *testing.T) {
	truncateTables(t)
	promo := newCapacityPromoCode("STALE_RESERVATION", 1, 50)
	require.NoError(t, promo.Insert())
	discountA, err := CalculatePromoCodeDiscount(promo.Code, PromoCodeTargetTopUp, 0, 100)
	require.NoError(t, err)
	topUpA := promoTopUpFromDiscount(discountA, "promo-stale-a", 924)
	require.NoError(t, topUpA.Insert())

	staleTime := common.GetTimestamp() - promoReservationTTLSeconds - 1
	require.NoError(t, DB.Model(&PromoCodeReservation{}).
		Where("promo_code_id = ? AND order_no = ?", promo.Id, topUpA.TradeNo).
		Update("updated_time", staleTime).Error)
	discountB, err := CalculatePromoCodeDiscount(promo.Code, PromoCodeTargetTopUp, 0, 100)
	require.NoError(t, err)
	topUpB := promoTopUpFromDiscount(discountB, "promo-stale-b", 925)
	require.NoError(t, topUpB.Insert())

	var reservationA PromoCodeReservation
	require.NoError(t, promoReservationQuery(DB, promo.Id, PromoCodeTargetTopUp, topUpA.TradeNo).First(&reservationA).Error)
	assert.Equal(t, promoReservationStatusReleased, reservationA.Status)
	var reservationB PromoCodeReservation
	require.NoError(t, promoReservationQuery(DB, promo.Id, PromoCodeTargetTopUp, topUpB.TradeNo).First(&reservationB).Error)
	assert.Equal(t, promoReservationStatusReserved, reservationB.Status)
	var storedPromo PromoCode
	require.NoError(t, DB.First(&storedPromo, promo.Id).Error)
	assert.Equal(t, 1, storedPromo.ReservedCount)
	assert.Zero(t, storedPromo.RedeemedCount)
}

func TestPromoCodeAgedCallbackReservationIsReclaimedAndRejected(t *testing.T) {
	truncateTables(t)
	t.Run("topup attempt", func(t *testing.T) {
		user := &User{Id: 926, Username: "promo-aged-topup", Status: common.UserStatusEnabled, Group: "default", AffCode: "promo-aged-topup-aff"}
		require.NoError(t, DB.Create(user).Error)
		promo := newCapacityPromoCode("AGED_TOPUP_CALLBACK", 1, 50)
		require.NoError(t, promo.Insert())
		discount, err := CalculatePromoCodeDiscount(promo.Code, PromoCodeTargetTopUp, 0, 100)
		require.NoError(t, err)
		topUp := promoTopUpFromDiscount(discount, "promo-aged-topup", user.Id)
		topUp.PaymentMethod = PaymentMethodOkpay
		topUp.PaymentProvider = PaymentProviderOkpay
		require.NoError(t, topUp.Insert())
		attempt, err := CreateTopUpPaymentAttempt(topUp.TradeNo, PaymentProviderOkpay, PaymentMethodOkpay, "50.00", "USD")
		require.NoError(t, err)
		require.NoError(t, MarkTopUpPaymentAttemptLaunchFailed(attempt.Id, "ambiguous launch failure"))
		require.NoError(t, UpdatePendingTopUpStatus(topUp.TradeNo, PaymentProviderOkpay, common.TopUpStatusFailed))

		agedTime := topUpQueryCutoff() - 1
		require.NoError(t, DB.Model(&TopUp{}).Where("id = ?", topUp.Id).Update("create_time", agedTime).Error)
		require.NoError(t, DB.Model(&TopUpPaymentAttempt{}).Where("id = ?", attempt.Id).Updates(map[string]interface{}{"create_time": agedTime, "update_time": agedTime}).Error)
		require.NoError(t, DB.Model(&PromoCodeReservation{}).Where("promo_code_id = ? AND order_no = ?", promo.Id, topUp.TradeNo).Update("updated_time", agedTime).Error)

		_, err = CalculatePromoCodeDiscount(promo.Code, PromoCodeTargetTopUp, 0, 100)
		require.NoError(t, err)
		var reservation PromoCodeReservation
		require.NoError(t, promoReservationQuery(DB, promo.Id, PromoCodeTargetTopUp, topUp.TradeNo).First(&reservation).Error)
		assert.Equal(t, promoReservationStatusReleased, reservation.Status)
		_, err = CompleteTopUpPaymentAttempt(attempt.Id, topUp.TradeNo, PaymentProviderOkpay, PaymentMethodOkpay)
		require.ErrorIs(t, err, ErrTopUpPaymentAttemptNotFound)
		var storedPromo PromoCode
		require.NoError(t, DB.First(&storedPromo, promo.Id).Error)
		assert.Zero(t, storedPromo.ReservedCount)
		assert.Zero(t, storedPromo.RedeemedCount)
	})

	t.Run("subscription snapshot", func(t *testing.T) {
		user := &User{Id: 927, Username: "promo-aged-sub", Status: common.UserStatusEnabled, Group: "default", AffCode: "promo-aged-sub-aff"}
		require.NoError(t, DB.Create(user).Error)
		plan := insertSubscriptionPlanForPaymentGuardTest(t, 921)
		promo := newCapacityPromoCode("AGED_SUB_CALLBACK", 1, 50)
		promo.AppliesToTopup = false
		promo.AppliesToAllSubscription = true
		require.NoError(t, promo.Insert())
		discount, err := CalculatePromoCodeDiscount(promo.Code, PromoCodeTargetSubscription, plan.Id, 100)
		require.NoError(t, err)
		order := &SubscriptionOrder{
			UserId: user.Id, PlanId: plan.Id, Money: 100, TradeNo: "promo-aged-sub",
			PaymentMethod: PaymentMethodStripe, PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusPending,
			ProviderOrderId: "promo-aged-provider-sub", ProviderAmount: "50.00", ProviderCurrency: SubscriptionCurrencyUSD,
		}
		ApplyPromoCodeResultToSubscriptionOrder(order, discount)
		require.NoError(t, order.Insert())
		require.NoError(t, ExpireSubscriptionOrder(order.TradeNo, PaymentProviderStripe))

		agedTime := topUpQueryCutoff() - 1
		require.NoError(t, DB.Model(&SubscriptionOrder{}).Where("id = ?", order.Id).Update("create_time", agedTime).Error)
		require.NoError(t, DB.Model(&PromoCodeReservation{}).Where("promo_code_id = ? AND order_no = ?", promo.Id, order.TradeNo).Update("updated_time", agedTime).Error)

		_, err = CalculatePromoCodeDiscount(promo.Code, PromoCodeTargetSubscription, plan.Id, 100)
		require.NoError(t, err)
		var reservation PromoCodeReservation
		require.NoError(t, promoReservationQuery(DB, promo.Id, PromoCodeTargetSubscription, order.TradeNo).First(&reservation).Error)
		assert.Equal(t, promoReservationStatusReleased, reservation.Status)
		err = CompleteSubscriptionOrder(order.TradeNo, "", PaymentProviderStripe, PaymentMethodStripe)
		require.ErrorIs(t, err, ErrSubscriptionOrderNotFound)
		var storedPromo PromoCode
		require.NoError(t, DB.First(&storedPromo, promo.Id).Error)
		assert.Zero(t, storedPromo.ReservedCount)
		assert.Zero(t, storedPromo.RedeemedCount)
	})
}

func TestPromoCodeStatusAfterRedemptionBoundaries(t *testing.T) {
	tests := []struct {
		name           string
		currentStatus  int
		maxRedeemCount int
		redeemedCount  int
		increment      int
		want           int
	}{
		{name: "unlimited remains enabled", currentStatus: common.RedemptionCodeStatusEnabled, redeemedCount: 100, increment: 1, want: common.RedemptionCodeStatusEnabled},
		{name: "below limit remains enabled", currentStatus: common.RedemptionCodeStatusEnabled, maxRedeemCount: 2, redeemedCount: 0, increment: 1, want: common.RedemptionCodeStatusEnabled},
		{name: "reaching limit becomes used", currentStatus: common.RedemptionCodeStatusEnabled, maxRedeemCount: 2, redeemedCount: 1, increment: 1, want: common.RedemptionCodeStatusUsed},
		{name: "already over limit becomes used", currentStatus: common.RedemptionCodeStatusEnabled, maxRedeemCount: 2, redeemedCount: 3, increment: 1, want: common.RedemptionCodeStatusUsed},
		{name: "zero increment preserves status", currentStatus: common.RedemptionCodeStatusEnabled, maxRedeemCount: 2, redeemedCount: 2, increment: 0, want: common.RedemptionCodeStatusEnabled},
		{name: "non-capacity status is preserved below limit", currentStatus: common.RedemptionCodeStatusDisabled, maxRedeemCount: 2, redeemedCount: 0, increment: 1, want: common.RedemptionCodeStatusDisabled},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, promoCodeStatusAfterRedemption(test.currentStatus, test.maxRedeemCount, test.redeemedCount, test.increment))
		})
	}
}

func TestPromoCodeSettlementStatusUsesLockReadLiteral(t *testing.T) {
	promo := &PromoCode{
		Status:         common.RedemptionCodeStatusEnabled,
		MaxRedeemCount: 2,
		RedeemedCount:  1,
	}
	updates := promoCodeSettlementUpdates(promo, 123, true)
	assert.Equal(t, common.RedemptionCodeStatusUsed, updates["status"])

	dummyDB, err := gorm.Open(tests.DummyDialector{}, &gorm.Config{DryRun: true})
	require.NoError(t, err)
	statement := dummyDB.Unscoped().Model(&PromoCode{}).Where("id = ?", 99).Updates(updates).Statement
	sql := strings.ToLower(statement.SQL.String())
	assert.Contains(t, sql, "status")
	assert.NotContains(t, sql, "max_redeem_count")
	assert.NotContains(t, sql, "redeemed_count + 1 >=")
	assert.Contains(t, statement.Vars, common.RedemptionCodeStatusUsed)
}

func TestPromoCodeMaxTwoFirstSettlementEnabledSecondUsed(t *testing.T) {
	truncateTables(t)
	promo := newCapacityPromoCode("STATUS_TWO_SETTLEMENTS", 2, 50)
	require.NoError(t, promo.Insert())

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return recordPromoCodeUsageTx(tx, promo.Id, 940, PromoCodeTargetTopUp, "status-two-first", 100, 50, 50, true)
	}))
	var afterFirst PromoCode
	require.NoError(t, DB.First(&afterFirst, promo.Id).Error)
	assert.Equal(t, 1, afterFirst.RedeemedCount)
	assert.Equal(t, common.RedemptionCodeStatusEnabled, afterFirst.Status)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return recordPromoCodeUsageTx(tx, promo.Id, 941, PromoCodeTargetTopUp, "status-two-second", 100, 50, 50, true)
	}))
	var afterSecond PromoCode
	require.NoError(t, DB.First(&afterSecond, promo.Id).Error)
	assert.Equal(t, 2, afterSecond.RedeemedCount)
	assert.Equal(t, common.RedemptionCodeStatusUsed, afterSecond.Status)
}

func TestPromoCodePaidHistoricalTopUpSettlesOverCapacity(t *testing.T) {
	tests := []struct {
		name                string
		withReleasedReserve bool
	}{
		{name: "missing reservation"},
		{name: "released reservation", withReleasedReserve: true},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			truncateTables(t)
			userId := 950 + index
			user := &User{Id: userId, Username: "historical-paid-" + test.name, Status: common.UserStatusEnabled, Group: "default"}
			require.NoError(t, DB.Create(user).Error)

			promoCodeSecret := "SECRET_PROMO_" + strings.ToUpper(strings.ReplaceAll(test.name, " ", "_"))
			promo := newCapacityPromoCode(promoCodeSecret, 1, 50)
			require.NoError(t, promo.Insert())
			require.NoError(t, DB.Model(&PromoCode{}).Where("id = ?", promo.Id).Updates(map[string]interface{}{
				"redeemed_count": 1,
				"status":         common.RedemptionCodeStatusUsed,
			}).Error)

			tradeNoSecret := "secret-order-payload-" + strings.ReplaceAll(test.name, " ", "-")
			providerSecret := "secret-provider-payload-" + strings.ReplaceAll(test.name, " ", "-")
			topUp := &TopUp{
				UserId:          user.Id,
				Amount:          100,
				Money:           50,
				OriginalMoney:   987654.321,
				DiscountMoney:   987604.321,
				ActualMoney:     50,
				CreditedQuota:   1000,
				PromoCodeId:     promo.Id,
				PromoCode:       promoCodeSecret,
				TradeNo:         tradeNoSecret,
				PaymentMethod:   "secret-payment-method",
				PaymentProvider: PaymentProviderEpay,
				RequestIP:       "secret-request-payload",
				ProviderOrderId: providerSecret,
				CreateTime:      common.GetTimestamp(),
				Status:          common.TopUpStatusPending,
			}
			require.NoError(t, DB.Create(topUp).Error)
			if test.withReleasedReserve {
				now := common.GetTimestamp()
				require.NoError(t, DB.Create(&PromoCodeReservation{
					PromoCodeId: promo.Id,
					OrderType:   PromoCodeTargetTopUp,
					OrderNo:     topUp.TradeNo,
					Status:      promoReservationStatusReleased,
					CreatedTime: now,
					UpdatedTime: now,
				}).Error)
			}

			var audit bytes.Buffer
			common.LogWriterMu.Lock()
			previousErrorWriter := gin.DefaultErrorWriter
			gin.DefaultErrorWriter = &audit
			common.LogWriterMu.Unlock()
			t.Cleanup(func() {
				common.LogWriterMu.Lock()
				gin.DefaultErrorWriter = previousErrorWriter
				common.LogWriterMu.Unlock()
			})

			require.NoError(t, ManualCompleteTopUp(topUp.TradeNo))
			require.NoError(t, ManualCompleteTopUp(topUp.TradeNo))

			var storedPromo PromoCode
			require.NoError(t, DB.First(&storedPromo, promo.Id).Error)
			assert.Equal(t, 2, storedPromo.RedeemedCount)
			assert.Zero(t, storedPromo.ReservedCount)
			assert.Equal(t, common.RedemptionCodeStatusUsed, storedPromo.Status)
			var storedUser User
			require.NoError(t, DB.First(&storedUser, user.Id).Error)
			assert.Equal(t, topUp.CreditedQuota, storedUser.Quota)
			var usageCount int64
			require.NoError(t, DB.Model(&PromoCodeUsage{}).Where("promo_code_id = ? AND order_no = ?", promo.Id, topUp.TradeNo).Count(&usageCount).Error)
			assert.EqualValues(t, 1, usageCount)
			var reservations []PromoCodeReservation
			require.NoError(t, promoReservationQuery(DB, promo.Id, PromoCodeTargetTopUp, topUp.TradeNo).Find(&reservations).Error)
			require.Len(t, reservations, 1)
			assert.Equal(t, promoReservationStatusSettled, reservations[0].Status)

			auditText := audit.String()
			expectedContext := fmt.Sprintf("promo_id=%d user_id=%d order_type=topup", promo.Id, user.Id)
			assert.Contains(t, auditText, expectedContext)
			assert.Equal(t, 1, strings.Count(auditText, "paid promo settlement exceeded capacity"))
			assert.NotContains(t, auditText, promoCodeSecret)
			assert.NotContains(t, auditText, tradeNoSecret)
			assert.NotContains(t, auditText, providerSecret)
			assert.NotContains(t, auditText, "secret-request-payload")
			assert.NotContains(t, auditText, "secret-payment-method")
			assert.NotContains(t, auditText, "987654.321")
		})
	}
}

func TestPromoCodeStrictFreeSettlementRejectsOverCapacity(t *testing.T) {
	truncateTables(t)
	promo := newCapacityPromoCode("STRICT_FREE_OVER_CAP", 1, 100)
	require.NoError(t, promo.Insert())
	require.NoError(t, DB.Model(&PromoCode{}).Where("id = ?", promo.Id).Updates(map[string]interface{}{
		"redeemed_count": 1,
		"status":         common.RedemptionCodeStatusEnabled,
	}).Error)
	topUp := &TopUp{
		UserId:        960,
		PromoCodeId:   promo.Id,
		TradeNo:       "strict-free-over-cap",
		OriginalMoney: 100,
		DiscountMoney: 100,
		ActualMoney:   0,
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		return recordTopUpPromoUsageTx(tx, topUp, true)
	})
	require.ErrorContains(t, err, "使用次数上限")
	var storedPromo PromoCode
	require.NoError(t, DB.First(&storedPromo, promo.Id).Error)
	assert.Equal(t, 1, storedPromo.RedeemedCount)
	var usageCount int64
	require.NoError(t, DB.Model(&PromoCodeUsage{}).Where("promo_code_id = ?", promo.Id).Count(&usageCount).Error)
	assert.Zero(t, usageCount)
}
