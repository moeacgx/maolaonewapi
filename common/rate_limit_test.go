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
