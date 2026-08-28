package model

import (
	"errors"
	"sync"
)

// ErrChannelConcurrencyLimitReached means eligible channels exist but all are
// currently occupied. It is distinct from a missing model or group.
var ErrChannelConcurrencyLimitReached = errors.New("channel concurrency limit reached")

var channelConcurrency = struct {
	sync.Mutex
	active map[int]int
}{
	active: make(map[int]int),
}

func IsChannelConcurrencyAvailable(channel *Channel) bool {
	if channel == nil || channel.GetConcurrencyLimit() <= 0 {
		return true
	}

	channelConcurrency.Lock()
	defer channelConcurrency.Unlock()
	return channelConcurrency.active[channel.Id] < channel.GetConcurrencyLimit()
}

func TryAcquireChannelConcurrency(channel *Channel) bool {
	if channel == nil || channel.GetConcurrencyLimit() <= 0 {
		return true
	}

	channelConcurrency.Lock()
	defer channelConcurrency.Unlock()
	current := channelConcurrency.active[channel.Id]
	if current >= channel.GetConcurrencyLimit() {
		return false
	}
	channelConcurrency.active[channel.Id] = current + 1
	return true
}

func ReleaseChannelConcurrency(channelID int) {
	if channelID <= 0 {
		return
	}

	channelConcurrency.Lock()
	defer channelConcurrency.Unlock()
	current := channelConcurrency.active[channelID]
	if current <= 1 {
		delete(channelConcurrency.active, channelID)
		return
	}
	channelConcurrency.active[channelID] = current - 1
}

func resetChannelConcurrencyForTest() {
	channelConcurrency.Lock()
	defer channelConcurrency.Unlock()
	channelConcurrency.active = make(map[int]int)
}
