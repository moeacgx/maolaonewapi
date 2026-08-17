package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func useSubscriptionMaintenanceRedis(t *testing.T) {
	t.Helper()
	server := miniredis.RunT(t)
	previousRedisEnabled := common.RedisEnabled
	previousRDB := common.RDB
	previousSyncFrequency := common.SyncFrequency
	common.RedisEnabled = true
	common.SyncFrequency = 2
	common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRDB
		common.SyncFrequency = previousSyncFrequency
	})
}

func TestSubscriptionMaintenanceExpiresDueSubscriptionAndRefreshesCache(t *testing.T) {
	truncate(t)
	useSubscriptionMaintenanceRedis(t)

	userId := 7301
	now := model.GetDBTimestamp()
	user := model.User{
		Id:          userId,
		Username:    "subscription-maintenance-user",
		Group:       "vip",
		Status:      common.UserStatusEnabled,
		AuthVersion: 1,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	subscription := model.UserSubscription{
		UserId:        userId,
		PlanId:        1,
		AmountTotal:   100,
		StartTime:     now - 200,
		EndTime:       now - 100,
		Status:        "active",
		UpgradeGroup:  "vip",
		PrevUserGroup: "default",
	}
	require.NoError(t, model.DB.Create(&subscription).Error)

	cachedBefore, err := model.GetUserCache(userId)
	require.NoError(t, err)
	require.Equal(t, "vip", cachedBefore.Group)

	runSubscriptionQuotaResetOnce()

	var authoritative model.User
	require.NoError(t, model.DB.Select("id", "group").First(&authoritative, userId).Error)
	require.Equal(t, "default", authoritative.Group)
	var expired model.UserSubscription
	require.NoError(t, model.DB.First(&expired, subscription.Id).Error)
	require.Equal(t, "expired", expired.Status)
	cachedAfter, err := model.GetUserCache(userId)
	require.NoError(t, err)
	require.Equal(t, "default", cachedAfter.Group)
}
