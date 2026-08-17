package setting

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeCCSwitchAPIAddress(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		want      string
		wantError bool
	}{
		{name: "empty", value: "  ", want: ""},
		{name: "trim root", value: "  https://api.example.com///  ", want: "https://api.example.com"},
		{name: "keep path", value: "http://api.example.com/gateway/", want: "http://api.example.com/gateway"},
		{name: "relative", value: "/api", wantError: true},
		{name: "scheme", value: "ftp://api.example.com", wantError: true},
		{name: "userinfo", value: "https://user:password@api.example.com", wantError: true},
		{name: "query", value: "https://api.example.com?token=x", wantError: true},
		{name: "fragment", value: "https://api.example.com/#config", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NormalizeCCSwitchAPIAddress(test.value)
			if test.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestCCSwitchAPIAddressConcurrentAccess(t *testing.T) {
	original := GetCCSwitchAPIAddress()
	t.Cleanup(func() { SetCCSwitchAPIAddress(original) })

	const workers = 16
	const iterations = 500
	var waitGroup sync.WaitGroup
	waitGroup.Add(workers * 2)
	for worker := range workers {
		go func() {
			defer waitGroup.Done()
			for iteration := range iterations {
				SetCCSwitchAPIAddress(fmt.Sprintf("https://api-%d-%d.example.com", worker, iteration))
			}
		}()
		go func() {
			defer waitGroup.Done()
			for range iterations {
				_ = GetCCSwitchAPIAddress()
			}
		}()
	}
	waitGroup.Wait()
	SetCCSwitchAPIAddress("https://api.example.com/final")
	require.Equal(t, "https://api.example.com/final", GetCCSwitchAPIAddress())
}

func TestUpdateChatsKeepsPublishedValueOnInvalidJSON(t *testing.T) {
	original := Chats
	t.Cleanup(func() { Chats = original })
	require.NoError(t, UpdateChatsByJsonString(`[{"test":"https://example.com"}]`))
	require.Error(t, UpdateChatsByJsonString(`[{`))
	require.Equal(t, []map[string]string{{"test": "https://example.com"}}, Chats)
}
