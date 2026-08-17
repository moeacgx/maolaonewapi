package constant

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPath2RelayMode(t *testing.T) {
	tests := []struct {
		path string
		want int
	}{
		{path: "/v1/alpha/search", want: RelayModeAlphaSearch},
		{path: "/v1/alpha/search?foo=1", want: RelayModeAlphaSearch},
		{path: "/canvas/v1/chat/completions", want: RelayModeChatCompletions},
		{path: "/canvas/v1/images/generations", want: RelayModeImagesGenerations},
		{path: "/canvas/v1/images/edits", want: RelayModeImagesEdits},
		{path: "/canvas/v1/audio/speech", want: RelayModeAudioSpeech},
		{path: "/not-canvas/v1/images/generations", want: RelayModeUnknown},
		{path: "/canvas/v1evil/images/generations", want: RelayModeUnknown},
		{path: "/canvas-v1/images/generations", want: RelayModeUnknown},
		{path: "/v1/images/generations", want: RelayModeImagesGenerations},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, Path2RelayMode(tt.path))
		})
	}
}
