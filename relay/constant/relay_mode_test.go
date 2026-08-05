package constant

import "testing"

func TestPath2RelayModePlaygroundImageGeneration(t *testing.T) {
	if got := Path2RelayMode("/pg/images/generations"); got != RelayModeImagesGenerations {
		t.Fatalf("Path2RelayMode(/pg/images/generations) = %d, want %d", got, RelayModeImagesGenerations)
	}
}
