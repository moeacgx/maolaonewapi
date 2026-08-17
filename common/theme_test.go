package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFrontendThemeRuntimeDefaultAndInvalidSet(t *testing.T) {
	previousTheme := GetTheme()
	t.Cleanup(func() { SetTheme(previousTheme) })

	assert.Equal(t, "default", GetTheme())

	SetTheme("classic")
	assert.Equal(t, "classic", GetTheme())

	SetTheme("legacy")
	assert.Equal(t, "classic", GetTheme())
}
