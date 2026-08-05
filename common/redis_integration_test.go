package common_test

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func TestRedisGenerationAndQuotaFallbackIntegration(t *testing.T) {
	redisURL := os.Getenv("NEWAPI_REDIS_INTEGRATION_URL")
	if redisURL == "" {
		t.Skip("NEWAPI_REDIS_INTEGRATION_URL is not set")
	}
	options, err := redis.ParseURL(redisURL)
	require.NoError(t, err)
	client := redis.NewClient(options)
	t.Cleanup(func() { _ = client.Close() })

	previousClient := common.RDB
	common.RDB = client
	t.Cleanup(func() { common.RDB = previousClient })

	prefix := fmt.Sprintf("newapi:redis-integration:%d", time.Now().UnixNano())
	keys := make([]string, 0, 16)
	key := func(suffix string) string {
		value := prefix + ":" + suffix
		keys = append(keys, value)
		return value
	}
	t.Cleanup(func() {
		if len(keys) > 0 {
			_ = client.Del(client.Context(), keys...).Err()
		}
	})
	deleteGenerationKey := key("delete-generation")
	deleteDataKey := key("delete-data")
	require.NoError(t, client.HSet(client.Context(), deleteDataKey, "Quota", "10").Err())
	require.NoError(t, common.RedisBumpGenerationAndDeleteKeys(deleteGenerationKey, []string{deleteDataKey}))
	require.Equal(t, "1", client.Get(client.Context(), deleteGenerationKey).Val())
	require.Equal(t, time.Duration(-1), client.PTTL(client.Context(), deleteGenerationKey).Val())
	require.Zero(t, client.Exists(client.Context(), deleteDataKey).Val())

	keepGenerationKey := key("keep-generation")
	keepDataKey := key("keep-data")
	require.NoError(t, client.HSet(
		client.Context(), keepDataKey,
		"Id", "42", "Quota", "90", "Role", "1",
	).Err())
	require.NoError(t, common.RedisBumpGenerationAndKeepHashFields(
		keepGenerationKey, keepDataKey, "Id", "Quota",
	))
	require.Equal(t, time.Duration(-1), client.PTTL(client.Context(), keepGenerationKey).Val())
	require.Equal(t, map[string]string{"Id": "42", "Quota": "90"}, client.HGetAll(client.Context(), keepDataKey).Val())

	conditionalGenerationKey := key("conditional-generation")
	conditionalDataKey := key("conditional-data")
	require.NoError(t, client.HSet(client.Context(), conditionalDataKey, "Quota", "10").Err())
	deleted, err := common.RedisBumpGenerationAndDeleteIfCurrent(
		conditionalGenerationKey, conditionalDataKey, 0,
	)
	require.NoError(t, err)
	require.True(t, deleted)
	require.Equal(t, time.Duration(-1), client.PTTL(client.Context(), conditionalGenerationKey).Val())

	fallbackGenerationKey := key("fallback-generation")
	fallbackDataKey := key("fallback-data")
	fallbackLockKey := key("fallback-lock")
	require.NoError(t, client.HSet(
		client.Context(), fallbackDataKey,
		"Id", "999", "Quota", "100", "Role", "1",
	).Err())
	state, err := common.RedisHIncrByOrAcquireFallback(
		fallbackDataKey,
		fallbackGenerationKey,
		fallbackLockKey,
		"Id",
		"42",
		"Quota",
		-5,
		time.Minute,
		"owner-a",
		10*time.Second,
	)
	require.NoError(t, err)
	require.Equal(t, common.RedisHashIncrementFallbackAcquired, state)
	require.Equal(t, time.Duration(-1), client.PTTL(client.Context(), fallbackGenerationKey).Val())
	require.Zero(t, client.Exists(client.Context(), fallbackDataKey).Val())

	renewed, err := common.RedisRenewHashFallback(fallbackLockKey, "owner-b", 10*time.Second)
	require.NoError(t, err)
	require.False(t, renewed)
	renewed, err = common.RedisRenewHashFallback(fallbackLockKey, "owner-a", 10*time.Second)
	require.NoError(t, err)
	require.True(t, renewed)
	require.Greater(t, client.PTTL(client.Context(), fallbackLockKey).Val(), 9*time.Second)

	finished, err := common.RedisFinishHashFallback(
		fallbackDataKey, fallbackGenerationKey, fallbackLockKey, "owner-b",
	)
	require.NoError(t, err)
	require.False(t, finished)
	finished, err = common.RedisFinishHashFallback(
		fallbackDataKey, fallbackGenerationKey, fallbackLockKey, "owner-a",
	)
	require.NoError(t, err)
	require.True(t, finished)
	require.Equal(t, time.Duration(-1), client.PTTL(client.Context(), fallbackGenerationKey).Val())
	require.Zero(t, client.Exists(client.Context(), fallbackLockKey).Val())
	protected, err := common.RedisEnsureHashFallback(
		fallbackDataKey,
		fallbackGenerationKey,
		fallbackLockKey,
		"owner-a",
		10*time.Second,
	)
	require.NoError(t, err)
	require.True(t, protected)
	protected, err = common.RedisEnsureHashFallback(
		fallbackDataKey,
		fallbackGenerationKey,
		fallbackLockKey,
		"owner-b",
		10*time.Second,
	)
	require.NoError(t, err)
	require.False(t, protected)
	finished, err = common.RedisFinishHashFallback(
		fallbackDataKey, fallbackGenerationKey, fallbackLockKey, "owner-a",
	)
	require.NoError(t, err)
	require.True(t, finished)

	corruptDataKey := key("corrupt-data")
	require.NoError(t, client.HSet(client.Context(), corruptDataKey, "Quota", "broken").Err())
	var snapshot struct{ Quota int }
	err = common.RedisHGetObj(corruptDataKey, &snapshot)
	require.ErrorIs(t, err, common.ErrRedisHashCorrupt)
	var fieldErr *common.RedisHashFieldDecodeError
	require.True(t, errors.As(err, &fieldErr))
	require.Equal(t, "Quota", fieldErr.Field)

	validDataKey := key("valid-data")
	validGenerationKey := key("valid-generation")
	validLockKey := key("valid-lock")
	require.NoError(t, client.HSet(
		client.Context(), validDataKey,
		"Id", "42", "Quota", "100", "Role", "1",
	).Err())
	state, err = common.RedisHIncrByOrAcquireFallback(
		validDataKey,
		validGenerationKey,
		validLockKey,
		"Id",
		"42",
		"Quota",
		-5,
		time.Minute,
		"owner-valid",
		10*time.Second,
	)
	require.NoError(t, err)
	require.Equal(t, common.RedisHashIncrementUpdated, state)
	require.Equal(t, "95", client.HGet(client.Context(), validDataKey, "Quota").Val())

	guardGenerationKey := key("guard-generation")
	guardDataKey := key("guard-data")
	guardLockKey := key("guard-lock")
	require.NoError(t, client.Set(client.Context(), guardGenerationKey, "3", 0).Err())
	require.NoError(t, client.HSet(
		client.Context(), guardDataKey,
		"Id", "999", "Quota", "80", "Role", "0",
	).Err())
	type guardedSnapshot struct {
		Id    int
		Quota int
		Role  int
	}
	written, err := common.RedisHSetObjIfGenerationWithPreserveGuardAndBlockKey(
		guardGenerationKey,
		guardDataKey,
		3,
		&guardedSnapshot{Id: 42, Quota: 100, Role: 1},
		time.Minute,
		"Id",
		guardLockKey,
		"Quota",
	)
	require.NoError(t, err)
	require.True(t, written)
	require.Equal(t, "42", client.HGet(client.Context(), guardDataKey, "Id").Val())
	require.Equal(t, "100", client.HGet(client.Context(), guardDataKey, "Quota").Val())
}
