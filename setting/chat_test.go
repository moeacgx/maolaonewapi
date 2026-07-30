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
		expected  string
		shouldErr bool
	}{
		{name: "空值", value: "  ", expected: ""},
		{name: "清理空白和尾斜杠", value: "  https://api.example.com///  ", expected: "https://api.example.com"},
		{name: "保留根路径", value: "http://api.example.com/gateway/", expected: "http://api.example.com/gateway"},
		{name: "拒绝相对地址", value: "/api", shouldErr: true},
		{name: "拒绝其他协议", value: "ftp://api.example.com", shouldErr: true},
		{name: "拒绝用户信息", value: "https://user:password@api.example.com", shouldErr: true},
		{name: "拒绝查询参数", value: "https://api.example.com?token=x", shouldErr: true},
		{name: "拒绝锚点", value: "https://api.example.com/#config", shouldErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := NormalizeCCSwitchAPIAddress(test.value)
			if test.shouldErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.expected, actual)
		})
	}
}

func TestCCSwitchAPIAddressConcurrentAccess(t *testing.T) {
	original := GetCCSwitchAPIAddress()
	t.Cleanup(func() {
		SetCCSwitchAPIAddress(original)
	})

	const workers = 16
	const iterations = 500
	var waitGroup sync.WaitGroup
	waitGroup.Add(workers * 2)
	for worker := 0; worker < workers; worker++ {
		worker := worker
		go func() {
			defer waitGroup.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				SetCCSwitchAPIAddress(fmt.Sprintf("https://api-%d-%d.example.com", worker, iteration))
			}
		}()
		go func() {
			defer waitGroup.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				_ = GetCCSwitchAPIAddress()
			}
		}()
	}
	waitGroup.Wait()

	const expected = "https://api.example.com/final"
	SetCCSwitchAPIAddress(expected)
	require.Equal(t, expected, GetCCSwitchAPIAddress())
}
