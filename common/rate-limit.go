package common

import (
	"sync"
	"time"
)

type InMemoryRateLimiter struct {
	store              map[string]*[]int64
	mutex              sync.Mutex
	expirationDuration time.Duration
}

func (l *InMemoryRateLimiter) Init(expirationDuration time.Duration) {
	if l.store == nil {
		l.mutex.Lock()
		if l.store == nil {
			l.store = make(map[string]*[]int64)
			l.expirationDuration = expirationDuration
			if expirationDuration > 0 {
				go l.clearExpiredItems()
			}
		}
		l.mutex.Unlock()
	}
}

func (l *InMemoryRateLimiter) clearExpiredItems() {
	for {
		time.Sleep(l.expirationDuration)
		l.mutex.Lock()
		now := time.Now().Unix()
		for key := range l.store {
			queue := l.store[key]
			size := len(*queue)
			if size == 0 || now-(*queue)[size-1] > int64(l.expirationDuration.Seconds()) {
				delete(l.store, key)
			}
		}
		l.mutex.Unlock()
	}
}

// Request parameter duration's unit is seconds
func (l *InMemoryRateLimiter) Request(key string, maxRequestNum int, duration int64) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	// [old <-- new]
	queue, ok := l.store[key]
	now := time.Now().Unix()
	if ok {
		if len(*queue) < maxRequestNum {
			*queue = append(*queue, now)
			return true
		} else {
			if now-(*queue)[0] >= duration {
				*queue = (*queue)[1:]
				*queue = append(*queue, now)
				return true
			} else {
				return false
			}
		}
	} else {
		s := make([]int64, 0, maxRequestNum)
		l.store[key] = &s
		*(l.store[key]) = append(*(l.store[key]), now)
	}
	return true
}

// Allow reports whether key has room in the current window without recording an event.
// It is used for counters that must only be incremented after a protected operation succeeds.
func (l *InMemoryRateLimiter) Allow(key string, maxRequestNum int, duration int64) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	queue, ok := l.store[key]
	if !ok || len(*queue) < maxRequestNum {
		return true
	}
	return time.Now().Unix()-(*queue)[0] >= duration
}

// RateLimitBatchRequest describes one counter admission in RequestBatch.
// A MaxRequestNum of zero means the counter is unlimited and is skipped.
type RateLimitBatchRequest struct {
	Key           string
	MaxRequestNum int
	Duration      int64
}

// RequestBatch validates and records all requested counters atomically. If any
// request is rejected, the returned index identifies it and no counter is
// changed.
func (l *InMemoryRateLimiter) RequestBatch(requests []RateLimitBatchRequest) (int, bool) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	now := time.Now().Unix()
	// Keep tentative queues separate from the store until every request passes.
	tentative := make(map[string][]int64, len(requests))
	for index, request := range requests {
		if request.MaxRequestNum == 0 {
			continue
		}

		queue, exists := tentative[request.Key]
		if !exists {
			if stored, ok := l.store[request.Key]; ok {
				queue = append([]int64(nil), (*stored)...)
			}
		}

		if len(queue) < request.MaxRequestNum {
			queue = append(queue, now)
			tentative[request.Key] = queue
			continue
		}
		if now-queue[0] >= request.Duration {
			queue = append(queue[1:], now)
			tentative[request.Key] = queue
			continue
		}
		return index, false
	}

	for key, queue := range tentative {
		stored := queue
		l.store[key] = &stored
	}
	return -1, true
}
