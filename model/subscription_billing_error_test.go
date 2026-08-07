package model

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPreConsumeUserSubscriptionReturnsNoActiveSubscriptionSentinel(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&SubscriptionPreConsumeRecord{}))
	userID := 19401
	require.NoError(t, DB.Where("user_id = ?", userID).Delete(&UserSubscription{}).Error)

	_, err := PreConsumeUserSubscription("no-active-subscription", userID, "test-model", 0, 10)
	require.ErrorIs(t, err, ErrNoActiveSubscription)
}

func TestPreConsumeUserSubscriptionPreservesQueryFailure(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&SubscriptionPreConsumeRecord{}))
	injectedErr := errors.New("forced active subscription query failure")
	callbackName := "test:subscription_active_query_failure"
	require.NoError(t, DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "user_subscriptions" {
			tx.AddError(injectedErr)
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Query().Remove(callbackName) })

	_, err := PreConsumeUserSubscription("subscription-query-failure", 19402, "test-model", 0, 10)
	require.ErrorIs(t, err, injectedErr)
	require.NotErrorIs(t, err, ErrNoActiveSubscription)
	require.NotErrorIs(t, err, ErrSubscriptionQuotaInsufficient)
}

func TestSubscriptionQuotaInsufficientUsesSentinel(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&SubscriptionPreConsumeRecord{}))
	const (
		userID = 19403
		planID = 19403
	)
	plan := SubscriptionPlan{
		Id: planID, Title: "quota sentinel plan", Enabled: true,
		DurationUnit: SubscriptionDurationMonth, DurationValue: 1,
		TotalAmount: 100, QuotaResetPeriod: SubscriptionResetNever,
	}
	require.NoError(t, DB.Create(&plan).Error)
	t.Cleanup(func() {
		InvalidateSubscriptionPlanCache(planID)
		_ = DB.Delete(&plan).Error
	})
	sub := UserSubscription{
		UserId: userID, PlanId: planID, AmountTotal: 100, AmountUsed: 90,
		Status: "active", StartTime: time.Now().Add(-time.Hour).Unix(),
		EndTime: time.Now().Add(time.Hour).Unix(),
	}
	require.NoError(t, DB.Create(&sub).Error)
	t.Cleanup(func() { _ = DB.Delete(&sub).Error })

	_, err := PreConsumeUserSubscription("subscription-quota-insufficient", userID, "test-model", 0, 20)
	require.ErrorIs(t, err, ErrSubscriptionQuotaInsufficient)

	err = PostConsumeUserSubscriptionDelta(sub.Id, 20)
	require.ErrorIs(t, err, ErrSubscriptionQuotaInsufficient)

	var persisted UserSubscription
	require.NoError(t, DB.First(&persisted, sub.Id).Error)
	require.EqualValues(t, 90, persisted.AmountUsed)
}

func TestSubscriptionErrorTextAloneIsNotBusinessSentinel(t *testing.T) {
	for _, message := range []string{"no active subscription", "subscription quota insufficient"} {
		err := fmt.Errorf("database returned text: %s", message)
		require.False(t, errors.Is(err, ErrNoActiveSubscription))
		require.False(t, errors.Is(err, ErrSubscriptionQuotaInsufficient))
	}
}
