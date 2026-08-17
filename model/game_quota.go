package model

// AdjustUserQuotaCache synchronizes a committed game exchange with the
// existing Redis-backed user quota cache. A cache miss is intentionally a
// no-op; the next read hydrates from the committed database value.
func AdjustUserQuotaCache(userID int, delta int64) error {
	return cacheIncrUserQuota(userID, delta)
}
