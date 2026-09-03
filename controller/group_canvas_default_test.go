package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanvasDefaultGroupForUsableGroups(t *testing.T) {
	usableGroups := map[string]map[string]interface{}{"vip": {}}
	assert.Equal(t, "vip", canvasDefaultGroupForUsableGroups(usableGroups, " vip "))
	assert.Empty(t, canvasDefaultGroupForUsableGroups(usableGroups, "default"))

	legacyGroups := map[string]map[string]interface{}{
		"legacy-vip": {"code": "vip"},
	}
	assert.Equal(t, "legacy-vip", canvasDefaultGroupForUsableGroups(legacyGroups, "vip"))
}
