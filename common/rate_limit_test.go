package common

import (
	"testing"
	"time"
)

func TestInMemoryRateLimiterAllowDoesNotConsumeCapacity(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)
	if !limiter.Allow("new", 1, 60) {
		t.Fatal("unused key should be allowed")
	}
	if !limiter.Allow("new", 1, 60) {
		t.Fatal("Allow must not consume capacity")
	}
	if !limiter.Request("new", 1, 60) {
		t.Fatal("first recorded request should be allowed")
	}
	if limiter.Allow("new", 1, 60) {
		t.Fatal("full window should not be allowed")
	}

	limiter.mutex.Lock()
	old := time.Now().Add(-2 * time.Minute).Unix()
	limiter.store["new"] = &[]int64{old}
	limiter.mutex.Unlock()
	if !limiter.Allow("new", 1, 60) {
		t.Fatal("expired window should be allowed")
	}
}

func TestInMemoryRateLimiterRequestBatchDoesNotPartiallyConsume(t *testing.T) {
	var limiter InMemoryRateLimiter
	limiter.Init(0)
	const duration = int64(60)
	laterRequest := RateLimitBatchRequest{Key: "batch-later", MaxRequestNum: 1, Duration: duration}
	if !limiter.Request(laterRequest.Key, laterRequest.MaxRequestNum, laterRequest.Duration) {
		t.Fatal("failed to fill later batch counter")
	}

	rejectedIndex, allowed := limiter.RequestBatch([]RateLimitBatchRequest{
		{Key: "batch-earlier", MaxRequestNum: 1, Duration: duration},
		laterRequest,
	})
	if allowed {
		t.Fatal("batch should reject when a later counter is full")
	}
	if rejectedIndex != 1 {
		t.Fatalf("rejected index = %d, want 1", rejectedIndex)
	}
	if !limiter.Allow("batch-earlier", 1, duration) {
		t.Fatal("rejected batch must not consume an earlier counter")
	}
}
