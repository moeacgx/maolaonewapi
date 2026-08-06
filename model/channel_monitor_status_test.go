package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestHandlerMultiKeyUpdateRestoresOnlyProbedKey(t *testing.T) {
	channel := &Channel{
		Id:     991004,
		Status: common.ChannelStatusAutoDisabled,
		Key:    "auto-disabled-key-0\nauto-disabled-key-1",
		ChannelInfo: ChannelInfo{
			IsMultiKey: true,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusAutoDisabled,
				1: common.ChannelStatusAutoDisabled,
			},
		},
	}

	handlerMultiKeyUpdate(channel, "auto-disabled-key-0", common.ChannelStatusEnabled, "")

	if channel.Status != common.ChannelStatusEnabled {
		t.Fatalf("expected channel to become enabled after restoring one key, got status %d", channel.Status)
	}
	if _, exists := channel.ChannelInfo.MultiKeyStatusList[0]; exists {
		t.Fatal("expected the probed key to be removed from the disabled status list")
	}
	if status := channel.ChannelInfo.MultiKeyStatusList[1]; status != common.ChannelStatusAutoDisabled {
		t.Fatalf("expected the unprobed key to remain auto-disabled, got status %d", status)
	}
}

func TestHandlerMultiKeyUpdateAtIndexRestoresExactDuplicateKey(t *testing.T) {
	channel := &Channel{
		Id:     991005,
		Status: common.ChannelStatusAutoDisabled,
		Key:    "duplicate-key\nduplicate-key",
		ChannelInfo: ChannelInfo{
			IsMultiKey: true,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusAutoDisabled,
				1: common.ChannelStatusAutoDisabled,
			},
		},
	}

	updated := handlerMultiKeyUpdateAtIndex(channel, 1, "duplicate-key", common.ChannelStatusEnabled, "")

	if !updated {
		t.Fatal("expected the probed key index to be restored")
	}
	if status := channel.ChannelInfo.MultiKeyStatusList[0]; status != common.ChannelStatusAutoDisabled {
		t.Fatalf("expected duplicate key at index 0 to remain auto-disabled, got status %d", status)
	}
	if _, exists := channel.ChannelInfo.MultiKeyStatusList[1]; exists {
		t.Fatal("expected the actually probed duplicate key at index 1 to be restored")
	}
}

func TestHandlerMultiKeyUpdateAtIndexRejectsChangedConfiguration(t *testing.T) {
	channel := &Channel{
		Id:     991006,
		Status: common.ChannelStatusAutoDisabled,
		Key:    "replacement-key\nprobed-key",
		ChannelInfo: ChannelInfo{
			IsMultiKey: true,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusAutoDisabled,
				1: common.ChannelStatusAutoDisabled,
			},
		},
	}

	updated := handlerMultiKeyUpdateAtIndex(channel, 0, "probed-key", common.ChannelStatusEnabled, "")

	if updated {
		t.Fatal("expected stale probe index to be rejected after key configuration changed")
	}
	if channel.Status != common.ChannelStatusAutoDisabled {
		t.Fatalf("expected channel status to remain auto-disabled, got %d", channel.Status)
	}
}
