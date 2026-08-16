package router

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-contrib/static"
)

func TestIsRealStaticWebAssetRequest(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatalf("create assets directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "app.js"), []byte("app"), 0o600); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("index"), 0o600); err != nil {
		t.Fatalf("write index: %v", err)
	}
	frontendFS := static.LocalFile(root, false)

	tests := []struct {
		name   string
		method string
		path   string
		want   bool
	}{
		{name: "existing GET asset", method: http.MethodGet, path: "/assets/app.js", want: true},
		{name: "existing HEAD asset", method: http.MethodHead, path: "/assets/app.js", want: true},
		{name: "missing asset", method: http.MethodGet, path: "/assets/missing.js"},
		{name: "asset write", method: http.MethodPost, path: "/assets/app.js"},
		{name: "html", method: http.MethodGet, path: "/index.html"},
		{name: "spa route", method: http.MethodGet, path: "/dashboard"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			if got := isRealStaticWebAssetRequest(request, frontendFS); got != test.want {
				t.Fatalf("isRealStaticWebAssetRequest() = %v, want %v", got, test.want)
			}
		})
	}
}
