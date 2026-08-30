package model

import (
	"sync"
	"time"
)

var ErrGroupUserConcurrencyLimitReached = &groupUserConcurrencyLimitError{}

type groupUserConcurrencyLimitError struct{}

func (*groupUserConcurrencyLimitError) Error() string { return "group user concurrency limit reached" }

type groupUserConcurrencyKey struct {
	UserID  int
	GroupID int
}

var groupUserConcurrency = struct {
	sync.Mutex
	active map[groupUserConcurrencyKey]int
}{active: make(map[groupUserConcurrencyKey]int)}

// GroupUserConcurrencyLease owns one user/group request slot.
type GroupUserConcurrencyLease struct {
	key      groupUserConcurrencyKey
	mu       sync.Mutex
	released bool
}

func TryAcquireGroupUserConcurrency(userID, groupID, limit int) (*GroupUserConcurrencyLease, bool) {
	if userID <= 0 || groupID <= 0 || limit <= 0 {
		return nil, true
	}
	key := groupUserConcurrencyKey{UserID: userID, GroupID: groupID}
	groupUserConcurrency.Lock()
	defer groupUserConcurrency.Unlock()
	if groupUserConcurrency.active[key] >= limit {
		return nil, false
	}
	groupUserConcurrency.active[key]++
	return &GroupUserConcurrencyLease{key: key}, true
}

func (lease *GroupUserConcurrencyLease) Release() {
	if lease == nil {
		return
	}
	lease.mu.Lock()
	if lease.released {
		lease.mu.Unlock()
		return
	}
	lease.released = true
	lease.mu.Unlock()

	groupUserConcurrency.Lock()
	defer groupUserConcurrency.Unlock()
	if count := groupUserConcurrency.active[lease.key]; count <= 1 {
		delete(groupUserConcurrency.active, lease.key)
	} else {
		groupUserConcurrency.active[lease.key] = count - 1
	}
}

func HasActiveBenefitActivityForGroup(groupID int) bool {
	if DB == nil || groupID <= 0 {
		return false
	}
	now := time.Now().Unix()
	var count int64
	if err := DB.Model(&BenefitActivity{}).
		Where("group_id = ? AND status IN ? AND starts_at <= ? AND ends_at > ?", groupID, []string{BenefitActivityStatusPublished, BenefitActivityStatusPaused}, now, now).
		Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}
