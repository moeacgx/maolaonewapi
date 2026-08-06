package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
)

func TestGetMonitorProbeKeyUsesEnabledKeyWhenAvailable(t *testing.T) {
	channel := &Channel{
		Id:  991001,
		Key: "enabled-key\nauto-disabled-key",
		ChannelInfo: ChannelInfo{
			IsMultiKey: true,
			MultiKeyStatusList: map[int]int{
				1: common.ChannelStatusAutoDisabled,
			},
			MultiKeyMode: constant.MultiKeyModeRandom,
		},
	}

	key, index, err := channel.GetMonitorProbeKey()
	if err != nil {
		t.Fatalf("expected enabled key to be selected, got %v", err)
	}
	if key != "enabled-key" || index != 0 {
		t.Fatalf("expected enabled key at index 0, got key=%q index=%d", key, index)
	}
}

func TestGetMonitorProbeKeyFallsBackToAutoDisabledKey(t *testing.T) {
	channel := &Channel{
		Id:  991002,
		Key: "auto-disabled-key-0\nauto-disabled-key-1",
		ChannelInfo: ChannelInfo{
			IsMultiKey: true,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusAutoDisabled,
				1: common.ChannelStatusAutoDisabled,
			},
			MultiKeyMode: constant.MultiKeyModeRandom,
		},
	}

	key, index, err := channel.GetMonitorProbeKey()
	if err != nil {
		t.Fatalf("expected an auto-disabled key to be selected, got %v", err)
	}
	if index < 0 || index > 1 {
		t.Fatalf("unexpected key index %d", index)
	}
	if key != channel.GetKeys()[index] {
		t.Fatalf("selected key %q does not match index %d", key, index)
	}
}

func TestGetMonitorProbeKeyDoesNotProbeManualKeys(t *testing.T) {
	channel := &Channel{
		Id:  991003,
		Key: "manual-key-0\nmanual-key-1",
		ChannelInfo: ChannelInfo{
			IsMultiKey: true,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusManuallyDisabled,
				1: common.ChannelStatusManuallyDisabled,
			},
			MultiKeyMode: constant.MultiKeyModeRandom,
		},
	}

	_, _, err := channel.GetMonitorProbeKey()
	if err == nil {
		t.Fatal("expected manual-disabled keys to remain unavailable for monitoring")
	}
}

func TestGetMonitorProbeKeySkipsManualKeyWhenAutoDisabledKeyExists(t *testing.T) {
	channel := &Channel{
		Id:  991007,
		Key: "auto-disabled-key\nmanual-key",
		ChannelInfo: ChannelInfo{
			IsMultiKey: true,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusAutoDisabled,
				1: common.ChannelStatusManuallyDisabled,
			},
			MultiKeyMode: constant.MultiKeyModeRandom,
		},
	}

	key, index, err := channel.GetMonitorProbeKey()
	if err != nil {
		t.Fatalf("expected the auto-disabled key to be selected, got %v", err)
	}
	if key != "auto-disabled-key" || index != 0 {
		t.Fatalf("expected only the auto-disabled key to be probed, got key=%q index=%d", key, index)
	}
}
