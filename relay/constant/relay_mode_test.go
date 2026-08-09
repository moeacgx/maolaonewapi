package constant

import "testing"

func TestPath2RelayModeAlphaSearch(t *testing.T) {
	if got := Path2RelayMode("/v1/alpha/search"); got != RelayModeAlphaSearch {
		t.Fatalf("Path2RelayMode(/v1/alpha/search) = %d, want %d", got, RelayModeAlphaSearch)
	}
}

func TestPath2RelayModeResponsesRemainDistinct(t *testing.T) {
	if got := Path2RelayMode("/v1/responses"); got != RelayModeResponses {
		t.Fatalf("Path2RelayMode(/v1/responses) = %d, want %d", got, RelayModeResponses)
	}
	if got := Path2RelayMode("/v1/responses/compact"); got != RelayModeResponsesCompact {
		t.Fatalf("Path2RelayMode(/v1/responses/compact) = %d, want %d", got, RelayModeResponsesCompact)
	}
}
