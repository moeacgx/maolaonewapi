package model

import (
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func insertSubscriptionExpireTestUser(t *testing.T, id int, group string) {
	t.Helper()
	require.NoError(t, DB.Create(&User{
		Id:          id,
		Username:    "subscription_expire_user",
		Group:       group,
		Status:      common.UserStatusEnabled,
		AuthVersion: 1,
	}).Error)
}

func getSubscriptionExpireTestUserGroup(t *testing.T, id int) string {
	t.Helper()
	var group string
	require.NoError(t, DB.Model(&User{}).Where("id = ?", id).Select(commonGroupCol).Find(&group).Error)
	return group
}

func TestExpireDueSubscriptionsFallsBackThroughRenewedUpgradeGroup(t *testing.T) {
	truncateTables(t)

	userId := 7001
	now := GetDBTimestamp()
	insertSubscriptionExpireTestUser(t, userId, "vip")
	require.NoError(t, DB.Create(&UserSubscription{
		UserId:        userId,
		PlanId:        1,
		AmountTotal:   100,
		StartTime:     now - 300,
		EndTime:       now - 200,
		Status:        "active",
		UpgradeGroup:  "vip",
		PrevUserGroup: "default",
	}).Error)
	require.NoError(t, DB.Create(&UserSubscription{
		UserId:       userId,
		PlanId:       2,
		AmountTotal:  100,
		StartTime:    now - 200,
		EndTime:      now - 100,
		Status:       "active",
		UpgradeGroup: "vip",
	}).Error)

	count, err := ExpireDueSubscriptions(20)
	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.Equal(t, "default", getSubscriptionExpireTestUserGroup(t, userId))
}

func TestGetUserCacheHitDoesNotQueryDueSubscriptions(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)

	userId := 7002
	now := GetDBTimestamp()
	insertSubscriptionExpireTestUser(t, userId, "vip")
	require.NoError(t, DB.Create(&UserSubscription{
		UserId:        userId,
		PlanId:        1,
		AmountTotal:   100,
		StartTime:     now - 200,
		EndTime:       now - 100,
		Status:        "active",
		UpgradeGroup:  "vip",
		PrevUserGroup: "default",
	}).Error)
	var user User
	require.NoError(t, DB.First(&user, userId).Error)
	require.NoError(t, populateUserCache(user))

	queryStarted := make(chan struct{})
	releaseQuery := make(chan struct{})
	var queryOnce sync.Once
	const callbackName = "test:block_auth_subscription_expiry_query"
	require.NoError(t, DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Table != "user_subscriptions" {
			return
		}
		queryOnce.Do(func() { close(queryStarted) })
		<-releaseQuery
	}))
	t.Cleanup(func() {
		_ = DB.Callback().Query().Remove(callbackName)
	})

	type cacheResult struct {
		user *UserBase
		err  error
	}
	result := make(chan cacheResult, 1)
	go func() {
		cached, err := GetUserCache(userId)
		result <- cacheResult{user: cached, err: err}
	}()

	select {
	case got := <-result:
		close(releaseQuery)
		require.NoError(t, got.err)
		require.Equal(t, "vip", got.user.Group)
	case <-queryStarted:
		close(releaseQuery)
		got := <-result
		require.NoError(t, got.err)
		t.Fatal("valid cache hit queried subscription expiry rows")
	case <-time.After(time.Second):
		close(releaseQuery)
		t.Fatal("valid cache hit did not return promptly")
	}
}

func TestScheduledExpiryRefreshesUserGroupCacheAfterCommit(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)

	userId := 7004
	now := GetDBTimestamp()
	insertSubscriptionExpireTestUser(t, userId, "vip")
	require.NoError(t, DB.Create(&UserSubscription{
		UserId:        userId,
		PlanId:        1,
		AmountTotal:   100,
		StartTime:     now - 200,
		EndTime:       now - 100,
		Status:        "active",
		UpgradeGroup:  "vip",
		PrevUserGroup: "default",
	}).Error)
	var user User
	require.NoError(t, DB.First(&user, userId).Error)
	require.NoError(t, populateUserCache(user))

	var cacheGroupDuringUpdate string
	var cacheReadErr error
	groupUpdateObserved := false
	callbackRegistered := true
	const callbackName = "test:observe_subscription_expiry_group_update"
	require.NoError(t, DB.Callback().Update().After("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if groupUpdateObserved || tx.Statement == nil || tx.Statement.Table != "users" {
			return
		}
		groupUpdateObserved = true
		cached, err := cacheGetUserBase(userId)
		if err != nil {
			cacheReadErr = err
			return
		}
		cacheGroupDuringUpdate = cached.Group
	}))
	t.Cleanup(func() {
		if callbackRegistered {
			_ = DB.Callback().Update().Remove(callbackName)
		}
	})

	count, err := ExpireDueSubscriptions(20)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.NoError(t, DB.Callback().Update().Remove(callbackName))
	callbackRegistered = false

	require.True(t, groupUpdateObserved)
	require.NoError(t, cacheReadErr)
	require.Equal(t, "vip", cacheGroupDuringUpdate, "cache must remain unchanged until the expiry transaction commits")
	require.Equal(t, "default", getSubscriptionExpireTestUserGroup(t, userId))

	var sub UserSubscription
	require.NoError(t, DB.Where("user_id = ?", userId).First(&sub).Error)
	require.Equal(t, "expired", sub.Status)
	cached, err := cacheGetUserBase(userId)
	require.NoError(t, err)
	require.Equal(t, "default", cached.Group)
}

func TestAdminDeleteActiveSubscriptionDowngradesGroup(t *testing.T) {
	truncateTables(t)

	userId := 7003
	now := GetDBTimestamp()
	insertSubscriptionExpireTestUser(t, userId, "vip")
	sub := UserSubscription{
		UserId:        userId,
		PlanId:        1,
		AmountTotal:   100,
		StartTime:     now - 100,
		EndTime:       now + 3600,
		Status:        "active",
		UpgradeGroup:  "vip",
		PrevUserGroup: "default",
	}
	require.NoError(t, DB.Create(&sub).Error)

	message, err := AdminDeleteUserSubscription(sub.Id)

	require.NoError(t, err)
	require.Contains(t, message, "default")
	require.Equal(t, "default", getSubscriptionExpireTestUserGroup(t, userId))
	var count int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", sub.Id).Count(&count).Error)
	require.Zero(t, count)
}
