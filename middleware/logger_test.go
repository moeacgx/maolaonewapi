package middleware

import (
	"reflect"
	"testing"
)

func TestLogSkipPathsDefault(t *testing.T) {
	t.Setenv("GIN_LOG_SKIP_PATHS", "")
	if got, want := logSkipPaths(), []string{"/api/status"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("默认跳过路径不匹配: got %v, want %v", got, want)
	}
}

func TestLogSkipPathsConfigured(t *testing.T) {
	t.Setenv("GIN_LOG_SKIP_PATHS", " /api/status, /healthz, ,/metrics ")
	want := []string{"/api/status", "/healthz", "/metrics"}
	if got := logSkipPaths(); !reflect.DeepEqual(got, want) {
		t.Fatalf("配置的跳过路径不匹配: got %v, want %v", got, want)
	}
}

func TestLogSkipPathsDisabled(t *testing.T) {
	t.Setenv("GIN_LOG_SKIP_PATHS", "none")
	if got := logSkipPaths(); len(got) != 0 {
		t.Fatalf("禁用跳过路径时应返回空列表: %v", got)
	}
}
