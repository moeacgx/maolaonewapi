package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPreConsumeUserSubscriptionPrioritizesNearestReset(t *testing.T) {
	truncateTables(t)

	userId := 7301
	now := GetDBTimestamp()
	plan := SubscriptionPlan{
		Title:            "rolling plan",
		DurationUnit:     SubscriptionDurationMonth,
		DurationValue:    1,
		TotalAmount:      1000,
		QuotaResetPeriod: SubscriptionResetDaily,
	}
	require.NoError(t, DB.Create(&plan).Error)

	laterReset := UserSubscription{
		UserId:        userId,
		PlanId:        plan.Id,
		AmountTotal:   1000,
		AmountUsed:    0,
		StartTime:     now - 7200,
		EndTime:       now + 30*86400,
		Status:        "active",
		LastResetTime: now - 7200,
		NextResetTime: now + 7200,
	}
	require.NoError(t, DB.Create(&laterReset).Error)
	soonerReset := UserSubscription{
		UserId:        userId,
		PlanId:        plan.Id,
		AmountTotal:   1000,
		AmountUsed:    0,
		StartTime:     now - 3600,
		EndTime:       now + 30*86400,
		Status:        "active",
		LastResetTime: now - 3600,
		NextResetTime: now + 3600,
	}
	require.NoError(t, DB.Create(&soonerReset).Error)

	result, err := PreConsumeUserSubscription("nearest-reset", userId, "gpt-test", 0, 100)
	require.NoError(t, err)
	require.Equal(t, soonerReset.Id, result.UserSubscriptionId)

	var gotSooner UserSubscription
	require.NoError(t, DB.First(&gotSooner, soonerReset.Id).Error)
	require.EqualValues(t, 100, gotSooner.AmountUsed)

	var gotLater UserSubscription
	require.NoError(t, DB.First(&gotLater, laterReset.Id).Error)
	require.EqualValues(t, 0, gotLater.AmountUsed)
}
