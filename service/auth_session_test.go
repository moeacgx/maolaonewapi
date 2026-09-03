package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAuthSessionTestDB(t *testing.T) *model.User {
	t.Helper()
	previousDB, previousRedis := model.DB, common.RedisEnabled
	previousActiveLimit := common.UserSessionActiveLimit
	previousIssuanceLimit := common.UserSessionIssuanceLimit
	previousIssuanceWindow := common.UserSessionIssuanceWindowSeconds
	previousRevokedRetention := common.UserSessionRevokedRetentionDays
	previousAlertThreshold := common.UserSessionHourlyAlertThreshold
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.AuthFlow{}))
	model.DB = db
	common.RedisEnabled = false
	common.UserSessionActiveLimit = common.DefaultUserSessionActiveLimit
	common.UserSessionIssuanceLimit = common.DefaultUserSessionIssuanceLimit
	common.UserSessionIssuanceWindowSeconds = int64(common.DefaultUserSessionIssuanceWindowSeconds)
	common.UserSessionRevokedRetentionDays = common.DefaultUserSessionRevokedRetentionDays
	common.UserSessionHourlyAlertThreshold = common.DefaultUserSessionHourlyAlertThreshold
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
		common.UserSessionActiveLimit = previousActiveLimit
		common.UserSessionIssuanceLimit = previousIssuanceLimit
		common.UserSessionIssuanceWindowSeconds = previousIssuanceWindow
		common.UserSessionRevokedRetentionDays = previousRevokedRetention
		common.UserSessionHourlyAlertThreshold = previousAlertThreshold
		_ = sqlDB.Close()
	})
	user := &model.User{
		Username:    "session-user",
		Password:    "unused-password-hash",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AuthVersion: 1,
	}
	require.NoError(t, db.Create(user).Error)
	return user
}

func useIndependentAuthSessionRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client, *miniredis.Miniredis, *redis.Client) {
	t.Helper()
	previousRedisEnabled := common.RedisEnabled
	previousRDB := common.RDB
	previousSyncFrequency := common.SyncFrequency
	serverA := miniredis.RunT(t)
	serverB := miniredis.RunT(t)
	clientA := redis.NewClient(&redis.Options{Addr: serverA.Addr()})
	clientB := redis.NewClient(&redis.Options{Addr: serverB.Addr()})
	common.RedisEnabled = true
	common.SyncFrequency = 2
	common.RDB = clientA
	t.Cleanup(func() {
		_ = clientA.Close()
		_ = clientB.Close()
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRDB
		common.SyncFrequency = previousSyncFrequency
	})
	return serverA, clientA, serverB, clientB
}

func cachedLoginSessionKey(t *testing.T, server *miniredis.Miniredis) string {
	t.Helper()
	for _, key := range server.Keys() {
		if strings.HasPrefix(key, "auth:session:") {
			return key
		}
	}
	require.FailNow(t, "login session was not cached")
	return ""
}

type failLoginSessionDenyFenceHook struct {
	failed atomic.Bool
}

func (hook *failLoginSessionDenyFenceHook) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	if cmd.Name() != "eval" || len(cmd.Args()) < 2 || !strings.Contains(fmt.Sprint(cmd.Args()[1]), "'SID', ARGV[1]") {
		return ctx, nil
	}
	for _, arg := range cmd.Args() {
		if arg == model.UserSessionStatusRevoking && hook.failed.CompareAndSwap(false, true) {
			return ctx, errors.New("forced login session deny fence failure")
		}
	}
	return ctx, nil
}

func (*failLoginSessionDenyFenceHook) AfterProcess(context.Context, redis.Cmder) error {
	return nil
}

func (*failLoginSessionDenyFenceHook) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}

func (*failLoginSessionDenyFenceHook) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

func TestCreateLoginSessionAutoEvictsOldestAcrossAuthVersions(t *testing.T) {
	useTestSessionSecret(t)
	user := setupAuthSessionTestDB(t)
	common.UserSessionActiveLimit = 2
	common.UserSessionIssuanceLimit = 100
	now := time.Now().Unix()
	otherUser := &model.User{
		Username: "other-session-user", Password: "unused-password-hash", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
		AffCode: "other-session-aff-code",
	}
	require.NoError(t, model.DB.Create(otherUser).Error)
	rows := []model.UserSession{
		{
			SID: "active-limit-a", UserID: user.Id, Version: 1, UserAuthVersion: user.AuthVersion + 1,
			Status: model.UserSessionStatusActive, RefreshHash: "hash-stale-version", LoginMethod: "password",
			CreatedAt: now - 10, LastActiveAt: now - 10, ExpiresAt: now + 3600,
		},
		{
			SID: "active-limit-b", UserID: user.Id, Version: 1, UserAuthVersion: user.AuthVersion,
			Status: model.UserSessionStatusActive, RefreshHash: "hash-current-version", LoginMethod: "password",
			CreatedAt: now - 10, LastActiveAt: now - 10, ExpiresAt: now + 3600,
		},
		{
			SID: "active-limit-expired", UserID: user.Id, Version: 1, UserAuthVersion: user.AuthVersion,
			Status: model.UserSessionStatusActive, RefreshHash: "hash-expired", LoginMethod: "password",
			CreatedAt: now - 20, LastActiveAt: now - 20, ExpiresAt: now - 1,
		},
		{
			SID: "active-limit-other-user", UserID: otherUser.Id, Version: 1, UserAuthVersion: otherUser.AuthVersion,
			Status: model.UserSessionStatusActive, RefreshHash: "hash-other-user", LoginMethod: "password",
			CreatedAt: now - 30, LastActiveAt: now - 30, ExpiresAt: now + 3600,
		},
	}
	require.NoError(t, model.DB.Create(&rows).Error)

	bundle, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "test-agent")
	require.NoError(t, err)

	oldest, err := model.GetUserSessionBySID("active-limit-a")
	require.NoError(t, err)
	assert.Equal(t, model.UserSessionStatusRevoked, oldest.Status)
	assert.Equal(t, "login_session_auto_evicted", oldest.RevokedReason)
	currentVersion, err := model.GetUserSessionBySID("active-limit-b")
	require.NoError(t, err)
	assert.Equal(t, model.UserSessionStatusActive, currentVersion.Status)
	other, err := model.GetUserSessionBySID("active-limit-other-user")
	require.NoError(t, err)
	assert.Equal(t, model.UserSessionStatusActive, other.Status)
	created, err := model.GetUserSessionBySID(bundle.Session.SID)
	require.NoError(t, err)
	assert.Equal(t, model.UserSessionStatusActive, created.Status)
	activeCount, err := model.CountActiveUserSessions(user.Id, now)
	require.NoError(t, err)
	assert.Equal(t, int64(2), activeCount)
}

func TestCreateLoginSessionAtLimitOneRevokesPreviousSession(t *testing.T) {
	useTestSessionSecret(t)
	user := setupAuthSessionTestDB(t)
	common.UserSessionActiveLimit = 1
	common.UserSessionIssuanceLimit = 10

	first, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "first-device")
	require.NoError(t, err)
	second, err := CreateLoginSession(user.Id, "password", "127.0.0.2", "second-device")
	require.NoError(t, err)
	assert.NotEqual(t, first.Session.SID, second.Session.SID)

	storedFirst, err := model.GetUserSessionBySID(first.Session.SID)
	require.NoError(t, err)
	assert.Equal(t, model.UserSessionStatusRevoked, storedFirst.Status)
	assert.Equal(t, "login_session_auto_evicted", storedFirst.RevokedReason)
	storedSecond, err := model.GetUserSessionBySID(second.Session.SID)
	require.NoError(t, err)
	assert.Equal(t, model.UserSessionStatusActive, storedSecond.Status)
}

func TestCreateLoginSessionRestoresHistoricallyExceededActiveLimit(t *testing.T) {
	useTestSessionSecret(t)
	user := setupAuthSessionTestDB(t)
	common.UserSessionActiveLimit = 2
	common.UserSessionIssuanceLimit = 100
	now := time.Now().Unix()
	rows := make([]model.UserSession, 0, 4)
	for i := 0; i < 4; i++ {
		rows = append(rows, model.UserSession{
			SID: fmt.Sprintf("historical-over-limit-%d", i), UserID: user.Id, Version: 1, UserAuthVersion: user.AuthVersion,
			Status: model.UserSessionStatusActive, RefreshHash: fmt.Sprintf("historical-hash-%d", i), LoginMethod: "password",
			CreatedAt: now - int64(10-i), LastActiveAt: now - int64(10-i), ExpiresAt: now + 3600,
		})
	}
	require.NoError(t, model.DB.Create(&rows).Error)

	_, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "replacement-device")
	require.NoError(t, err)
	activeCount, err := model.CountActiveUserSessions(user.Id, now)
	require.NoError(t, err)
	assert.Equal(t, int64(2), activeCount)
	for _, sid := range []string{"historical-over-limit-0", "historical-over-limit-1", "historical-over-limit-2"} {
		evicted, getErr := model.GetUserSessionBySID(sid)
		require.NoError(t, getErr)
		assert.Equal(t, model.UserSessionStatusRevoked, evicted.Status)
		assert.Equal(t, "login_session_auto_evicted", evicted.RevokedReason)
	}
	remaining, err := model.GetUserSessionBySID("historical-over-limit-3")
	require.NoError(t, err)
	assert.Equal(t, model.UserSessionStatusActive, remaining.Status)
}

func TestCreateLoginSessionConcurrentAdmissionHonorsHardLimits(t *testing.T) {
	useTestSessionSecret(t)
	user := setupAuthSessionTestDB(t)
	common.UserSessionActiveLimit = 2
	common.UserSessionIssuanceLimit = 100

	const attempts = 12
	start := make(chan struct{})
	results := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "concurrent-agent")
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var succeeded int
	for err := range results {
		if err == nil {
			succeeded++
			continue
		}
		require.NoError(t, err)
	}
	assert.Equal(t, attempts, succeeded, "active-session eviction must allow every admission within the issuance limit")
	var activeCount, revokedCount int64
	require.NoError(t, model.DB.Model(&model.UserSession{}).
		Where("user_id = ? AND status = ? AND expires_at > ?", user.Id, model.UserSessionStatusActive, time.Now().Unix()).
		Count(&activeCount).Error)
	require.NoError(t, model.DB.Model(&model.UserSession{}).
		Where("user_id = ? AND status = ?", user.Id, model.UserSessionStatusRevoked).
		Count(&revokedCount).Error)
	assert.Equal(t, int64(2), activeCount, "concurrent admission must leave no more than the active-session limit")
	assert.Equal(t, int64(attempts-2), revokedCount)

	require.NoError(t, model.DB.Where("user_id = ?", user.Id).Delete(&model.UserSession{}).Error)
	common.UserSessionActiveLimit = attempts
	common.UserSessionIssuanceLimit = 2
	common.UserSessionIssuanceWindowSeconds = 60
	const issuanceAttempts = 8
	issuanceResults := make(chan error, issuanceAttempts)
	start = make(chan struct{})
	for i := 0; i < issuanceAttempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "issuance-agent")
			issuanceResults <- err
		}()
	}
	close(start)
	wg.Wait()
	close(issuanceResults)
	succeeded, rejected := 0, 0
	for err := range issuanceResults {
		if err == nil {
			succeeded++
			continue
		}
		if errors.Is(err, model.ErrUserSessionIssuanceLimit) {
			rejected++
			continue
		}
		require.NoError(t, err)
	}
	assert.Equal(t, 2, succeeded, "concurrent login admission must not exceed issuance-window limit")
	assert.Equal(t, issuanceAttempts-2, rejected)
}

func TestCreateLoginSessionEnforcesIssuanceLimitAcrossAllStatuses(t *testing.T) {
	useTestSessionSecret(t)
	user := setupAuthSessionTestDB(t)
	common.UserSessionActiveLimit = 10
	common.UserSessionIssuanceLimit = 3
	common.UserSessionIssuanceWindowSeconds = 60
	now := time.Now().Unix()
	rows := []model.UserSession{
		{
			SID: "issuance-limit-revoked", UserID: user.Id, Version: 1, UserAuthVersion: user.AuthVersion + 1,
			Status: model.UserSessionStatusRevoked, RefreshHash: "hash-revoked", LoginMethod: "password",
			CreatedAt: now - 2, LastActiveAt: now - 2, ExpiresAt: now + 3600, RevokedAt: now - 1,
		},
		{
			SID: "issuance-limit-expired", UserID: user.Id, Version: 1, UserAuthVersion: user.AuthVersion,
			Status: model.UserSessionStatusActive, RefreshHash: "hash-expired", LoginMethod: "password",
			CreatedAt: now - 1, LastActiveAt: now - 1, ExpiresAt: now - 1,
		},
		{
			SID: "issuance-outside-effective-window", UserID: user.Id, Version: 1, UserAuthVersion: user.AuthVersion,
			Status: model.UserSessionStatusRevoked, RefreshHash: "hash-outside", LoginMethod: "password",
			CreatedAt: now - 61, LastActiveAt: now - 61, ExpiresAt: now + 3600, RevokedAt: now - 60,
		},
	}
	require.NoError(t, model.DB.Create(&rows).Error)

	_, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "test-agent")
	require.NoError(t, err, "rows outside the effective issuance window must not consume the limit")

	_, err = CreateLoginSession(user.Id, "password", "127.0.0.1", "test-agent")
	assert.ErrorIs(t, err, model.ErrUserSessionIssuanceLimit)
	var count int64
	require.NoError(t, model.DB.Model(&model.UserSession{}).Count(&count).Error)
	assert.Equal(t, int64(4), count)
}

func TestCreateLoginSessionV243DefaultAllowsIssuancePastLegacyLimit(t *testing.T) {
	useTestSessionSecret(t)
	user := setupAuthSessionTestDB(t)
	common.UserSessionActiveLimit = 1
	common.UserSessionIssuanceLimit = 0
	common.UserSessionIssuanceWindowSeconds = 60
	now := time.Now().Unix()

	legacyRows := make([]model.UserSession, 0, 100)
	legacyRows = append(legacyRows, model.UserSession{
		SID: "legacy-active", UserID: user.Id, Version: 1, UserAuthVersion: user.AuthVersion,
		Status: model.UserSessionStatusActive, RefreshHash: "legacy-active-hash", LoginMethod: "password",
		CreatedAt: now - 10, LastActiveAt: now - 10, ExpiresAt: now + 3600,
	})
	for i := 1; i < 100; i++ {
		legacyRows = append(legacyRows, model.UserSession{
			SID: fmt.Sprintf("legacy-revoked-%03d", i), UserID: user.Id, Version: 1, UserAuthVersion: user.AuthVersion,
			Status: model.UserSessionStatusRevoked, RefreshHash: fmt.Sprintf("legacy-revoked-hash-%03d", i), LoginMethod: "password",
			CreatedAt: now - 10, LastActiveAt: now - 10, ExpiresAt: now + 3600,
			RevokedAt: now - 5, RevokedReason: "legacy-test",
		})
	}
	require.NoError(t, model.DB.Create(&legacyRows).Error)

	bundle, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "v243-compatible-device")
	require.NoError(t, err, "the compatibility default must allow the 101st issuance")
	require.NotNil(t, bundle)

	legacyActive, err := model.GetUserSessionBySID("legacy-active")
	require.NoError(t, err)
	assert.Equal(t, model.UserSessionStatusRevoked, legacyActive.Status)
	assert.Equal(t, "login_session_auto_evicted", legacyActive.RevokedReason)

	activeCount, err := model.CountActiveUserSessions(user.Id, now)
	require.NoError(t, err)
	assert.Equal(t, int64(1), activeCount, "active-session admission remains enforced independently")
	var totalCount int64
	require.NoError(t, model.DB.Model(&model.UserSession{}).Where("user_id = ?", user.Id).Count(&totalCount).Error)
	assert.Equal(t, int64(101), totalCount)
}

func TestCreateLoginSessionAllowsRepeatedIssuanceWhenLimitDisabled(t *testing.T) {
	useTestSessionSecret(t)
	user := setupAuthSessionTestDB(t)
	common.UserSessionActiveLimit = 1
	common.UserSessionIssuanceLimit = 0
	common.UserSessionIssuanceWindowSeconds = 60

	first, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "first-device")
	require.NoError(t, err)
	second, err := CreateLoginSession(user.Id, "password", "127.0.0.2", "second-device")
	require.NoError(t, err)

	storedFirst, err := model.GetUserSessionBySID(first.Session.SID)
	require.NoError(t, err)
	assert.Equal(t, model.UserSessionStatusRevoked, storedFirst.Status)
	assert.Equal(t, "login_session_auto_evicted", storedFirst.RevokedReason)
	assert.NotEqual(t, first.Session.SID, second.Session.SID)
}

func TestCreateLoginSessionIssuanceLimitDoesNotEvictActiveSession(t *testing.T) {
	useTestSessionSecret(t)
	user := setupAuthSessionTestDB(t)
	common.UserSessionActiveLimit = 1
	common.UserSessionIssuanceLimit = 1
	common.UserSessionIssuanceWindowSeconds = 60

	first, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "first-device")
	require.NoError(t, err)
	second, err := CreateLoginSession(user.Id, "password", "127.0.0.2", "second-device")
	require.Nil(t, second)
	assert.ErrorIs(t, err, model.ErrUserSessionIssuanceLimit)

	storedFirst, err := model.GetUserSessionBySID(first.Session.SID)
	require.NoError(t, err)
	assert.Equal(t, model.UserSessionStatusActive, storedFirst.Status)
	assert.Empty(t, storedFirst.RevokedReason)
	var sessionCount int64
	require.NoError(t, model.DB.Model(&model.UserSession{}).Where("user_id = ?", user.Id).Count(&sessionCount).Error)
	assert.Equal(t, int64(1), sessionCount)
}

func TestPasswordResetDoesNotClearSessionIssuanceHistory(t *testing.T) {
	useTestSessionSecret(t)
	user := setupAuthSessionTestDB(t)
	common.UserSessionActiveLimit = 50
	common.UserSessionIssuanceLimit = 1
	email := "session-reset@example.com"
	require.NoError(t, model.DB.Model(user).Update("email", email).Error)

	_, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "test-agent")
	require.NoError(t, err)
	require.NoError(t, model.ResetUserPasswordByEmail(email, "new-password"))

	_, err = CreateLoginSession(user.Id, "password", "127.0.0.1", "test-agent")
	assert.ErrorIs(t, err, model.ErrUserSessionIssuanceLimit)
}

func TestPasswordResetRecoversLoginAfterAutomaticSessionEviction(t *testing.T) {
	useTestSessionSecret(t)
	user := setupAuthSessionTestDB(t)
	common.UserSessionActiveLimit = 1
	common.UserSessionIssuanceLimit = 10
	email := "session-limit-recovery@example.com"
	require.NoError(t, model.DB.Model(user).Update("email", email).Error)

	first, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "first-device")
	require.NoError(t, err)
	second, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "new-device")
	require.NoError(t, err)
	stored, err := model.GetUserSessionBySID(first.Session.SID)
	require.NoError(t, err)
	assert.Equal(t, model.UserSessionStatusRevoked, stored.Status)
	assert.Equal(t, "login_session_auto_evicted", stored.RevokedReason)

	require.NoError(t, model.ResetUserPasswordByEmail(email, "recovery-password"))
	stored, err = model.GetUserSessionBySID(second.Session.SID)
	require.NoError(t, err)
	assert.Equal(t, model.UserSessionStatusRevoked, stored.Status)

	recovered, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "recovery-device")
	require.NoError(t, err)
	assert.NotEqual(t, first.Session.SID, recovered.Session.SID)
}

func TestCreateLoginSessionFailsClosedWhenAutoEvictionDenyFenceFails(t *testing.T) {
	useTestSessionSecret(t)
	user := setupAuthSessionTestDB(t)
	common.UserSessionActiveLimit = 1
	common.UserSessionIssuanceLimit = 10
	_, client, _, _ := useIndependentAuthSessionRedis(t)

	first, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "first-device")
	require.NoError(t, err)
	hook := &failLoginSessionDenyFenceHook{}
	client.AddHook(hook)

	second, err := CreateLoginSession(user.Id, "password", "127.0.0.2", "second-device")
	require.Nil(t, second)
	assert.ErrorContains(t, err, "forced login session deny fence failure")
	assert.True(t, hook.failed.Load())

	storedFirst, err := model.GetUserSessionBySID(first.Session.SID)
	require.NoError(t, err)
	assert.Equal(t, model.UserSessionStatusActive, storedFirst.Status)
	var totalCount, activeCount int64
	require.NoError(t, model.DB.Model(&model.UserSession{}).Where("user_id = ?", user.Id).Count(&totalCount).Error)
	require.NoError(t, model.DB.Model(&model.UserSession{}).
		Where("user_id = ? AND status = ? AND expires_at > ?", user.Id, model.UserSessionStatusActive, time.Now().Unix()).
		Count(&activeCount).Error)
	assert.Equal(t, int64(1), totalCount, "deny fence failure must abort creation of the replacement session")
	assert.Equal(t, int64(1), activeCount)
}

func TestCreateLoginSessionFailsClosedWhenLimitCountFails(t *testing.T) {
	useTestSessionSecret(t)
	user := setupAuthSessionTestDB(t)
	forcedErr := errors.New("forced session count failure")
	callbackName := "test:fail_user_session_limit_count"
	callbackRegistered := true
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "user_sessions" {
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() {
		if callbackRegistered {
			_ = model.DB.Callback().Query().Remove(callbackName)
		}
	})

	_, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "test-agent")
	assert.ErrorIs(t, err, forcedErr)
	require.NoError(t, model.DB.Callback().Query().Remove(callbackName))
	callbackRegistered = false
	var count int64
	require.NoError(t, model.DB.Model(&model.UserSession{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestCleanupAuthArtifactsAlertsBeforeDeletingHourlyIssuance(t *testing.T) {
	setupAuthSessionTestDB(t)
	common.UserSessionHourlyAlertThreshold = 2
	common.UserSessionIssuanceWindowSeconds = 1
	now := time.Now()
	boundaryRows := make([]model.UserSession, 0, 2)
	for i := 0; i < 2; i++ {
		boundaryRows = append(boundaryRows, model.UserSession{
			SID: "hourly-boundary-" + string(rune('a'+i)), UserID: 1, Version: 1, UserAuthVersion: 1,
			Status: model.UserSessionStatusActive, RefreshHash: "hash", LoginMethod: "password",
			CreatedAt: now.Add(-2 * time.Second).Unix(), LastActiveAt: now.Add(-time.Hour).Unix(), ExpiresAt: now.Add(-time.Minute).Unix(),
		})
	}
	require.NoError(t, model.DB.Create(&boundaryRows).Error)

	var logBuffer bytes.Buffer
	common.LogWriterMu.Lock()
	previousErrorWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logBuffer
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = previousErrorWriter
		common.LogWriterMu.Unlock()
	})

	cleanupAuthArtifacts()
	assert.Empty(t, logBuffer.String(), "the hourly alert uses a strict greater-than threshold")
	var count int64
	require.NoError(t, model.DB.Model(&model.UserSession{}).Count(&count).Error)
	assert.Zero(t, count)

	exceededRows := make([]model.UserSession, 0, 3)
	for i := 0; i < 3; i++ {
		exceededRows = append(exceededRows, model.UserSession{
			SID: "hourly-exceeded-" + string(rune('a'+i)), UserID: 1, Version: 1, UserAuthVersion: 1,
			Status: model.UserSessionStatusActive, RefreshHash: "hash", LoginMethod: "password",
			CreatedAt: now.Add(-2 * time.Second).Unix(), LastActiveAt: now.Add(-time.Hour).Unix(), ExpiresAt: now.Add(-time.Minute).Unix(),
		})
	}
	require.NoError(t, model.DB.Create(&exceededRows).Error)
	logBuffer.Reset()
	cleanupAuthArtifacts()
	assert.Contains(t, logBuffer.String(), "hourly user session issuance exceeded alert threshold")
	require.NoError(t, model.DB.Model(&model.UserSession{}).Count(&count).Error)
	assert.Zero(t, count, "alerting must happen before expired rows are deleted")
}

func TestCleanupAuthArtifactsRemovesOnlyExpiredRecords(t *testing.T) {
	setupAuthSessionTestDB(t)
	now := time.Now()
	oldExpiry := now.Add(-25 * time.Hour)
	require.NoError(t, model.DB.Create(&model.UserSession{
		SID: "expired-session", UserID: 1, Version: 1, UserAuthVersion: 1,
		Status: model.UserSessionStatusActive, RefreshHash: "hash", LoginMethod: "password",
		CreatedAt: oldExpiry.Unix(), LastActiveAt: oldExpiry.Unix(), ExpiresAt: oldExpiry.Unix(),
	}).Error)
	require.NoError(t, model.DB.Create(&model.AuthFlow{
		TokenHash: "expired-flow", Purpose: model.AuthFlowPurposeTwoFALogin,
		ExpiresAt: oldExpiry,
	}).Error)
	require.NoError(t, model.DB.Create(&model.AuthFlow{
		TokenHash: "recent-flow", Purpose: model.AuthFlowPurposeTwoFALogin,
		ExpiresAt: now.Add(time.Minute),
	}).Error)

	cleanupAuthArtifacts()

	var sessionCount int64
	require.NoError(t, model.DB.Model(&model.UserSession{}).Count(&sessionCount).Error)
	assert.Zero(t, sessionCount)
	var flows []model.AuthFlow
	require.NoError(t, model.DB.Find(&flows).Error)
	require.Len(t, flows, 1)
	assert.Equal(t, "recent-flow", flows[0].TokenHash)
}

func TestCleanupAuthArtifactsContinuesWithRevokedCleanupAfterExpiredBatchFailure(t *testing.T) {
	setupAuthSessionTestDB(t)
	now := time.Now()
	oldCreatedAt := now.Add(-8 * 24 * time.Hour).Unix()
	require.NoError(t, model.DB.Create(&[]model.UserSession{
		{
			SID: "failed-expired-cleanup", UserID: 1, Version: 1, UserAuthVersion: 1,
			Status: model.UserSessionStatusActive, RefreshHash: "hash-expired", LoginMethod: "password",
			CreatedAt: oldCreatedAt, LastActiveAt: oldCreatedAt, ExpiresAt: now.Add(-time.Minute).Unix(),
		},
		{
			SID: "independent-revoked-cleanup", UserID: 1, Version: 1, UserAuthVersion: 1,
			Status: model.UserSessionStatusRevoked, RefreshHash: "hash-revoked", LoginMethod: "password",
			CreatedAt: oldCreatedAt, LastActiveAt: oldCreatedAt, ExpiresAt: now.Add(time.Hour).Unix(),
			RevokedAt: now.Add(-8 * 24 * time.Hour).Unix(),
		},
	}).Error)

	forcedErr := errors.New("forced expired cleanup failure")
	callbackName := "test:fail_first_user_session_cleanup_batch"
	failedFirstDelete := false
	require.NoError(t, model.DB.Callback().Delete().Before("gorm:delete").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "user_sessions" && !failedFirstDelete {
			failedFirstDelete = true
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Delete().Remove(callbackName) })

	cleanupAuthArtifacts()

	var expired model.UserSession
	require.NoError(t, model.DB.First(&expired, "sid = ?", "failed-expired-cleanup").Error)
	var revoked model.UserSession
	assert.ErrorIs(t, model.DB.First(&revoked, "sid = ?", "independent-revoked-cleanup").Error, gorm.ErrRecordNotFound)
}

func TestLoginSessionCreateRefreshAndRevoke(t *testing.T) {
	useTestSessionSecret(t)
	user := setupAuthSessionTestDB(t)

	bundle, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "test-agent")
	require.NoError(t, err)
	assert.NotEmpty(t, bundle.RefreshToken)
	identity, err := ParseAccessToken(bundle.AccessToken)
	require.NoError(t, err)
	_, cachedUser, err := ValidateLoginSession(identity)
	require.NoError(t, err)
	assert.Equal(t, user.Id, cachedUser.Id)
	require.NoError(t, RevokeByRefreshToken(bundle.Session.SID+".wrong-refresh-secret", "", "logout"))
	_, _, err = ValidateLoginSession(identity)
	require.NoError(t, err, "a caller that only knows sid must not be able to revoke the session")

	refreshed, _, err := RefreshLoginSession(bundle.RefreshToken, bundle.Session.SID, "127.0.0.2", "test-agent-2")
	require.NoError(t, err)
	assert.NotEqual(t, bundle.RefreshToken, refreshed.RefreshToken)
	recovered, _, err := RefreshLoginSession(bundle.RefreshToken, bundle.Session.SID, "127.0.0.2", "test-agent-2")
	require.NoError(t, err)
	assert.Equal(t, refreshed.RefreshToken, recovered.RefreshToken, "a concurrent refresh must recover the winner's rotated token")

	_, _, err = RefreshLoginSession(refreshed.RefreshToken, "different-session", "127.0.0.2", "test-agent-2")
	assert.ErrorIs(t, err, ErrLoginSessionMismatch)

	require.NoError(t, RevokeByRefreshToken(refreshed.RefreshToken, refreshed.Session.SID, "logout"))
	_, _, err = ValidateLoginSession(identity)
	assert.True(t, errors.Is(err, ErrLoginSessionRevoked))
}

func TestIndependentRedisSessionRevokeConvergesAfterCacheTTL(t *testing.T) {
	useTestSessionSecret(t)
	user := setupAuthSessionTestDB(t)
	_, clientA, serverB, clientB := useIndependentAuthSessionRedis(t)

	common.RDB = clientA
	bundle, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "node-a")
	require.NoError(t, err)
	identity, err := ParseAccessToken(bundle.AccessToken)
	require.NoError(t, err)

	common.RDB = clientB
	_, _, err = ValidateLoginSession(identity)
	require.NoError(t, err)
	assert.NotEmpty(t, cachedLoginSessionKey(t, serverB), "node B must hold its own session cache entry")

	common.RDB = clientA
	require.NoError(t, RevokeByRefreshToken(bundle.RefreshToken, bundle.Session.SID, "logout"))

	serverB.FastForward(3 * time.Second)
	common.RDB = clientB
	_, _, err = ValidateLoginSession(identity)
	assert.ErrorIs(t, err, ErrLoginSessionRevoked)
}

func TestIndependentRedisAuthVersionAdvanceConvergesAfterCacheTTL(t *testing.T) {
	useTestSessionSecret(t)
	user := setupAuthSessionTestDB(t)
	_, clientA, serverB, clientB := useIndependentAuthSessionRedis(t)

	common.RDB = clientA
	bundle, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "node-a")
	require.NoError(t, err)
	oldIdentity, err := ParseAccessToken(bundle.AccessToken)
	require.NoError(t, err)

	common.RDB = clientB
	_, _, err = ValidateLoginSession(oldIdentity)
	require.NoError(t, err)
	cacheKey := cachedLoginSessionKey(t, serverB)
	version := serverB.HGet(cacheKey, "Version")
	assert.Equal(t, "1", version, "node B must hold the pre-rotation session version")

	common.RDB = clientA
	rotated, err := AdvanceCurrentSessionSecurity(oldIdentity, "security_update")
	require.NoError(t, err)
	newIdentity, err := ParseAccessToken(rotated.AccessToken)
	require.NoError(t, err)
	assert.Greater(t, newIdentity.SessionVersion, oldIdentity.SessionVersion)
	assert.Greater(t, newIdentity.UserAuthVersion, oldIdentity.UserAuthVersion)

	serverB.FastForward(3 * time.Second)
	common.RDB = clientB
	_, _, err = ValidateLoginSession(newIdentity)
	require.NoError(t, err)
	_, _, err = ValidateLoginSession(oldIdentity)
	assert.ErrorIs(t, err, ErrLoginSessionRevoked)
}

func TestUserAuthVersionInvalidatesExistingSession(t *testing.T) {
	useTestSessionSecret(t)
	user := setupAuthSessionTestDB(t)
	bundle, err := CreateLoginSession(user.Id, "password", "127.0.0.1", "test-agent")
	require.NoError(t, err)
	identity, err := ParseAccessToken(bundle.AccessToken)
	require.NoError(t, err)

	_, err = model.BumpUserAuthVersion(user.Id)
	require.NoError(t, err)
	_, _, err = ValidateLoginSession(identity)
	assert.ErrorIs(t, err, ErrLoginSessionRevoked)
	_, err = CreateLoginSessionAtAuthVersion(user.Id, identity.UserAuthVersion, "2fa", "127.0.0.1", "test-agent")
	assert.ErrorIs(t, err, ErrLoginSessionRevoked, "a pending 2FA flow must not survive an auth-version change")
}
