package extension

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestHTTPProxyStripsSensitiveHeadersAndOmitsUnsignedIdentity(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Empty(t, request.Header.Get("Cookie"))
		require.Empty(t, request.Header.Get("Authorization"))
		require.Empty(t, request.Header.Get("Proxy-Authorization"))
		require.Equal(t, "orders", request.Header.Get("X-NewAPI-Module-ID"))
		for _, header := range []string{"X-NewAPI-User-ID", "X-NewAPI-Username", "X-NewAPI-User-Role", "X-NewAPI-User-Group", "X-NewAPI-Use-Access-Token", extensionIdentityTimestampHeader, extensionIdentitySignatureHeader} {
			require.Empty(t, request.Header.Values(header), header)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()
	originalValidate := validateExtensionOutboundURL
	originalClient := extensionProtectedHTTPClient
	validateExtensionOutboundURL = func(string) error { return nil }
	extensionProtectedHTTPClient = func() *http.Client { return backend.Client() }
	t.Cleanup(func() {
		validateExtensionOutboundURL = originalValidate
		extensionProtectedHTTPClient = originalClient
	})

	manager := NewManager(t.TempDir())
	writeManifest(t, manager.RootDir(), "orders", Manifest{
		ID:      "orders",
		Name:    "Orders",
		Version: "1.0.0",
		Runtime: Runtime{Type: RuntimeTypeHTTP, BaseURL: backend.URL},
		Permissions: PermissionConfig{
			Roles: []string{"root"},
		},
	})
	require.NoError(t, manager.Scan())
	_, err := manager.SetEnabled("orders", true)
	require.NoError(t, err)

	handler, err := manager.ProxyHandler("orders", "/health", common.RoleRootUser, ProxyContext{
		UserID: "7", Username: "root", Role: "100", Group: "admin", UseAccessToken: "true",
	})
	require.NoError(t, err)

	request := httptest.NewRequest(http.MethodGet, "/api/extensions/orders/proxy/health", nil)
	request.Header.Set("Cookie", "session=secret")
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("Proxy-Authorization", "Basic secret")
	request.Header.Set("X-NewAPI-User-ID", "forged")
	request.Header.Set("X-NewAPI-User-Role", "forged-root")
	request.Header.Set(extensionIdentityTimestampHeader, "1")
	request.Header.Set(extensionIdentitySignatureHeader, "forged")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestManifestRejectsUnsafePagePaths(t *testing.T) {
	validPaths := []string{"/", "/ui", "/assets/app.min.js", "/.well-known/status", "/运维/概览"}
	for _, pagePath := range validPaths {
		t.Run("valid_"+pagePath, func(t *testing.T) {
			manifest := Manifest{
				ID:      "page-path",
				Name:    "Page Path",
				Version: "1.0.0",
				Runtime: Runtime{Type: RuntimeTypeHTTP, BaseURL: "http://127.0.0.1:39001"},
				UI:      UIContribution{Pages: []Page{{Key: "index", Path: pagePath}}},
			}
			require.NoError(t, manifest.Validate())
		})
	}

	invalidPaths := []string{
		"ui",
		"//example.com/ui",
		"/ui//index.html",
		"/ui/../admin",
		"/ui/./index.html",
		`/ui\..\admin`,
		"/ui?mode=admin",
		"/ui#admin",
		"/%2e%2e/status",
		"/ui/",
		" /ui",
		"/ui\nadmin",
	}
	for _, pagePath := range invalidPaths {
		t.Run("invalid_"+pagePath, func(t *testing.T) {
			manifest := Manifest{
				ID:      "page-path",
				Name:    "Page Path",
				Version: "1.0.0",
				Runtime: Runtime{Type: RuntimeTypeHTTP, BaseURL: "http://127.0.0.1:39001"},
				UI:      UIContribution{Pages: []Page{{Key: "index", Path: pagePath}}},
			}
			require.ErrorContains(t, manifest.Validate(), "ui.pages[index].path is invalid")
		})
	}
}

func TestManifestRejectsDotPathsForStaticRuntime(t *testing.T) {
	manifest := Manifest{
		ID:      "static-dot-path",
		Name:    "Static Dot Path",
		Version: "1.0.0",
		Runtime: Runtime{Type: RuntimeTypeStatic, StaticDir: "public"},
		UI:      UIContribution{Pages: []Page{{Key: "index", Path: "/.well-known/status"}}},
	}
	require.ErrorContains(t, manifest.Validate(), "static paths must not contain dot-prefixed segments")

	manifest.UI.Pages[0].Path = "/index.html"
	manifest.Runtime.StaticDir = ".public"
	require.ErrorContains(t, manifest.Validate(), "runtime.static_dir is invalid")
}

func TestStaticProxyRejectsDotFilesAndSymbolicLinks(t *testing.T) {
	rootDir := t.TempDir()
	moduleDir := writeManifest(t, rootDir, "static-security", Manifest{
		ID:      "static-security",
		Name:    "Static Security",
		Version: "1.0.0",
		Runtime: Runtime{Type: RuntimeTypeStatic, StaticDir: "public"},
		UI:      UIContribution{Pages: []Page{{Key: "index", Path: "/"}}},
		Permissions: PermissionConfig{
			Roles: []string{"root"},
		},
	})
	publicDir := filepath.Join(moduleDir, "public")
	require.NoError(t, os.MkdirAll(publicDir, 0755))
	require.NoError(t, os.Mkdir(filepath.Join(publicDir, "assets"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(publicDir, "index.html"), []byte("static index"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(publicDir, ".env"), []byte("SECRET=value"), 0644))

	manager := NewManager(rootDir)
	require.NoError(t, manager.Scan())
	_, err := manager.SetEnabled("static-security", true)
	require.NoError(t, err)

	_, err = manager.ProxyHandler("static-security", "/.env", common.RoleRootUser, ProxyContext{})
	require.Error(t, err)
	_, err = manager.ProxyHandler("static-security", "/assets", common.RoleRootUser, ProxyContext{})
	require.Error(t, err)

	t.Run("file symbolic link escapes static root", func(t *testing.T) {
		outsideFile := filepath.Join(t.TempDir(), "secret.txt")
		require.NoError(t, os.WriteFile(outsideFile, []byte("outside secret"), 0644))
		linkPath := filepath.Join(publicDir, "linked-secret.txt")
		if err := os.Symlink(outsideFile, linkPath); err != nil {
			t.Skipf("current environment cannot create file symlinks: %v", err)
		}
		_, err := manager.ProxyHandler("static-security", "/linked-secret.txt", common.RoleRootUser, ProxyContext{})
		require.Error(t, err)
	})
}

func TestStaticProxyRejectsSymbolicStaticRoot(t *testing.T) {
	rootDir := t.TempDir()
	moduleDir := writeManifest(t, rootDir, "static-root-link", Manifest{
		ID:      "static-root-link",
		Name:    "Static Root Link",
		Version: "1.0.0",
		Runtime: Runtime{Type: RuntimeTypeStatic, StaticDir: "public"},
		UI:      UIContribution{Pages: []Page{{Key: "index", Path: "/"}}},
		Permissions: PermissionConfig{
			Roles: []string{"root"},
		},
	})
	outsideDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outsideDir, "index.html"), []byte("outside index"), 0644))
	if err := os.Symlink(outsideDir, filepath.Join(moduleDir, "public")); err != nil {
		t.Skipf("current environment cannot create directory symlinks: %v", err)
	}

	manager := NewManager(rootDir)
	require.NoError(t, manager.Scan())
	_, err := manager.SetEnabled("static-root-link", true)
	require.NoError(t, err)
	_, err = manager.ProxyHandler("static-root-link", "/", common.RoleRootUser, ProxyContext{})
	require.Error(t, err)
}

func TestHTTPProxyRejectsBlockedTarget(t *testing.T) {
	originalValidate := validateExtensionOutboundURL
	originalClient := extensionProtectedHTTPClient
	validateExtensionOutboundURL = func(string) error { return errors.New("private address") }
	extensionProtectedHTTPClient = func() *http.Client {
		t.Fatal("blocked target requested an outbound client")
		return nil
	}
	t.Cleanup(func() {
		validateExtensionOutboundURL = originalValidate
		extensionProtectedHTTPClient = originalClient
	})

	manager := NewManager(t.TempDir())
	writeManifest(t, manager.RootDir(), "blocked", Manifest{
		ID: "blocked", Name: "Blocked", Version: "1.0.0",
		Runtime: Runtime{Type: RuntimeTypeHTTP, BaseURL: "http://127.0.0.1:8080"},
	})
	require.NoError(t, manager.Scan())
	_, err := manager.SetEnabled("blocked", true)
	require.NoError(t, err)
	_, err = manager.ProxyHandler("blocked", "/", common.RoleRootUser, ProxyContext{})
	require.ErrorContains(t, err, "blocked")
}

func TestHTTPProxyIsolatesRedirectCookiesCORSAndAuthHeaders(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Empty(t, request.Header.Get("X-Hop-Secret"))
		writer.Header().Set("Set-Cookie", "module_session=secret")
		writer.Header().Set("Location", "https://evil.example/escape")
		writer.Header().Set("Access-Control-Allow-Origin", "*")
		writer.Header().Set("Access-Control-Allow-Credentials", "true")
		writer.Header().Set("WWW-Authenticate", `Bearer realm="module"`)
		writer.Header().Set("Connection", "X-Response-Hop")
		writer.Header().Set("X-Response-Hop", "secret")
		writer.WriteHeader(http.StatusFound)
	}))
	defer backend.Close()

	originalValidate := validateExtensionOutboundURL
	originalClient := extensionProtectedHTTPClient
	validateExtensionOutboundURL = func(string) error { return nil }
	extensionProtectedHTTPClient = func() *http.Client { return backend.Client() }
	t.Cleanup(func() {
		validateExtensionOutboundURL = originalValidate
		extensionProtectedHTTPClient = originalClient
	})

	manager := NewManager(t.TempDir())
	writeManifest(t, manager.RootDir(), "isolation", Manifest{
		ID: "isolation", Name: "Isolation", Version: "1.0.0",
		Runtime:     Runtime{Type: RuntimeTypeHTTP, BaseURL: backend.URL},
		Permissions: PermissionConfig{Roles: []string{"root"}},
	})
	require.NoError(t, manager.Scan())
	_, err := manager.SetEnabled("isolation", true)
	require.NoError(t, err)
	handler, err := manager.ProxyHandler("isolation", "/redirect", common.RoleRootUser, ProxyContext{})
	require.NoError(t, err)

	request := httptest.NewRequest(http.MethodGet, "/api/extensions/isolation/proxy/redirect", nil)
	request.Header.Set("Connection", "X-Hop-Secret")
	request.Header.Set("X-Hop-Secret", "secret")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusFound, recorder.Code)
	for _, header := range []string{"Set-Cookie", "Location", "Access-Control-Allow-Origin", "Access-Control-Allow-Credentials", "WWW-Authenticate", "X-Response-Hop"} {
		require.Empty(t, recorder.Header().Values(header), header)
	}
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
}
func TestStaticHTMLResponseUsesSandboxWithoutSameOrigin(t *testing.T) {
	rootDir := t.TempDir()
	moduleDir := writeManifest(t, rootDir, "static-html", Manifest{
		ID: "static-html", Name: "Static HTML", Version: "1.0.0",
		Runtime:     Runtime{Type: RuntimeTypeStatic, StaticDir: "public"},
		UI:          UIContribution{Pages: []Page{{Key: "index", Path: "/", Embed: false}}},
		Permissions: PermissionConfig{Roles: []string{"root"}},
	})
	publicDir := filepath.Join(moduleDir, "public")
	require.NoError(t, os.MkdirAll(publicDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(publicDir, "index.html"), []byte("<script>window.ok=true</script>"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(publicDir, "app.js"), []byte("window.ok=true"), 0644))

	manager := NewManager(rootDir)
	require.NoError(t, manager.Scan())
	_, err := manager.SetEnabled("static-html", true)
	require.NoError(t, err)

	htmlHandler, err := manager.ProxyHandler("static-html", "/", common.RoleRootUser, ProxyContext{})
	require.NoError(t, err)
	htmlRecorder := httptest.NewRecorder()
	htmlHandler.ServeHTTP(htmlRecorder, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, htmlRecorder.Code)
	csp := htmlRecorder.Header().Get("Content-Security-Policy")
	require.Equal(t, extensionHTMLContentSecurityPolicy, csp)
	require.Contains(t, csp, "sandbox allow-forms allow-popups allow-popups-to-escape-sandbox allow-scripts")
	require.NotContains(t, csp, "allow-same-origin")

	scriptHandler, err := manager.ProxyHandler("static-html", "/app.js", common.RoleRootUser, ProxyContext{})
	require.NoError(t, err)
	scriptRecorder := httptest.NewRecorder()
	scriptHandler.ServeHTTP(scriptRecorder, httptest.NewRequest(http.MethodGet, "/app.js", nil))
	require.Equal(t, http.StatusOK, scriptRecorder.Code)
	require.Empty(t, scriptRecorder.Header().Get("Content-Security-Policy"))
}

func TestHTTPProxyHTMLResponseReplacesUnsafeCSP(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.Header().Set("Content-Security-Policy", "default-src *; sandbox allow-same-origin")
		_, _ = writer.Write([]byte("<html></html>"))
	}))
	defer backend.Close()
	originalValidate := validateExtensionOutboundURL
	originalClient := extensionProtectedHTTPClient
	validateExtensionOutboundURL = func(string) error { return nil }
	extensionProtectedHTTPClient = func() *http.Client { return backend.Client() }
	t.Cleanup(func() {
		validateExtensionOutboundURL = originalValidate
		extensionProtectedHTTPClient = originalClient
	})

	manager := NewManager(t.TempDir())
	writeManifest(t, manager.RootDir(), "http-html", Manifest{
		ID: "http-html", Name: "HTTP HTML", Version: "1.0.0",
		Runtime:     Runtime{Type: RuntimeTypeHTTP, BaseURL: backend.URL},
		Permissions: PermissionConfig{Roles: []string{"root"}},
	})
	require.NoError(t, manager.Scan())
	_, err := manager.SetEnabled("http-html", true)
	require.NoError(t, err)
	handler, err := manager.ProxyHandler("http-html", "/", common.RoleRootUser, ProxyContext{})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, extensionHTMLContentSecurityPolicy, recorder.Header().Get("Content-Security-Policy"))
	require.NotContains(t, recorder.Header().Get("Content-Security-Policy"), "allow-same-origin")
}

func TestHTTPProxyAllowsRepresentationHeadersOnly(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		writer.Header().Set("ETag", `"safe"`)
		writer.Header().Set("Last-Modified", "Mon, 01 Jan 2024 00:00:00 GMT")
		writer.Header().Set("Vary", "Accept-Encoding")
		writer.Header().Set("Clear-Site-Data", `"*"`)
		writer.Header().Set("Refresh", "0;url=https://evil.example")
		writer.Header().Set("Location", "https://evil.example")
		writer.Header().Set("Set-Cookie", "secret=1")
		writer.Header().Set("WWW-Authenticate", `Bearer realm="module"`)
		writer.Header().Set("Access-Control-Allow-Origin", "*")
		writer.Header().Set("Content-Security-Policy", "default-src *")
		_, _ = writer.Write([]byte("safe"))
	}))
	defer backend.Close()
	originalValidate := validateExtensionOutboundURL
	originalClient := extensionProtectedHTTPClient
	validateExtensionOutboundURL = func(string) error { return nil }
	extensionProtectedHTTPClient = func() *http.Client { return backend.Client() }
	t.Cleanup(func() {
		validateExtensionOutboundURL = originalValidate
		extensionProtectedHTTPClient = originalClient
	})

	manager := NewManager(t.TempDir())
	writeManifest(t, manager.RootDir(), "headers", Manifest{
		ID: "headers", Name: "Headers", Version: "1.0.0",
		Runtime: Runtime{BaseURL: backend.URL}, Permissions: PermissionConfig{Roles: []string{"root"}},
	})
	require.NoError(t, manager.Scan())
	_, err := manager.SetEnabled("headers", true)
	require.NoError(t, err)
	handler, err := manager.ProxyHandler("headers", "/", common.RoleRootUser, ProxyContext{})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	for _, header := range []string{"ETag", "Last-Modified", "Vary"} {
		require.NotEmpty(t, recorder.Header().Get(header), header)
	}
	for _, header := range []string{"Clear-Site-Data", "Refresh", "Location", "Set-Cookie", "WWW-Authenticate", "Access-Control-Allow-Origin", "Content-Security-Policy"} {
		require.Empty(t, recorder.Header().Get(header), header)
	}
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "same-origin", recorder.Header().Get("Cross-Origin-Resource-Policy"))
}

func TestHTTPProxySignsIdentityClaimsWhenSecretConfigured(t *testing.T) {
	const secret = "module-shared-secret"
	expected := ProxyContext{UserID: "7", Username: "root", Role: "100", Group: "admin", UseAccessToken: "true"}
	backend := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		claims, err := VerifyExtensionContextHeaders(
			request.Header,
			secret,
			"signed-orders",
			request.Method,
			extensionRequestPath(request),
			time.Now(),
			time.Minute,
		)
		require.NoError(t, err)
		require.Equal(t, expected, claims)
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()
	originalValidate := validateExtensionOutboundURL
	originalClient := extensionProtectedHTTPClient
	validateExtensionOutboundURL = func(string) error { return nil }
	extensionProtectedHTTPClient = func() *http.Client { return backend.Client() }
	t.Cleanup(func() {
		validateExtensionOutboundURL = originalValidate
		extensionProtectedHTTPClient = originalClient
	})

	manager := NewManager(t.TempDir())
	writeManifest(t, manager.RootDir(), "signed-orders", Manifest{
		ID: "signed-orders", Name: "Signed Orders", Version: "1.0.0",
		Runtime:     Runtime{Type: RuntimeTypeHTTP, BaseURL: backend.URL, IdentitySecret: secret},
		Permissions: PermissionConfig{Roles: []string{"root"}},
	})
	require.NoError(t, manager.Scan())
	module, exists := manager.Get("signed-orders")
	require.True(t, exists)
	publicJSON, err := common.Marshal(module.Public(true))
	require.NoError(t, err)
	require.NotContains(t, string(publicJSON), secret)
	require.NotContains(t, string(publicJSON), "identity_secret")
	_, err = manager.SetEnabled("signed-orders", true)
	require.NoError(t, err)
	handler, err := manager.ProxyHandler("signed-orders", "/health", common.RoleRootUser, expected)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/proxy/health", nil))
	require.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestVerifyExtensionContextHeadersRejectsTamperAndReplay(t *testing.T) {
	const secret = "module-shared-secret"
	now := time.Unix(1_700_000_000, 0)
	ctx := ProxyContext{UserID: "7", Username: "root", Role: "100", Group: "admin", UseAccessToken: "true"}
	header := make(http.Header)
	setExtensionContextHeaders(header, "orders", secret, ctx, http.MethodGet, "/health", now)
	verified, err := VerifyExtensionContextHeaders(header, secret, "orders", http.MethodGet, "/health", now, time.Minute)
	require.NoError(t, err)
	require.Equal(t, ctx, verified)

	header.Set("X-NewAPI-User-Role", "forged-root")
	_, err = VerifyExtensionContextHeaders(header, secret, "orders", http.MethodGet, "/health", now, time.Minute)
	require.ErrorContains(t, err, "signature is invalid")

	setExtensionContextHeaders(header, "orders", secret, ctx, http.MethodGet, "/health", now.Add(-2*time.Minute))
	_, err = VerifyExtensionContextHeaders(header, secret, "orders", http.MethodGet, "/health", now, time.Minute)
	require.ErrorContains(t, err, "replay window")

	setExtensionContextHeaders(header, "orders", secret, ctx, http.MethodGet, "/health", now)
	_, err = VerifyExtensionContextHeaders(header, secret, "orders", http.MethodPost, "/health", now, time.Minute)
	require.ErrorContains(t, err, "signature is invalid")
	_, err = VerifyExtensionContextHeaders(header, secret, "orders", http.MethodGet, "/other", now, time.Minute)
	require.ErrorContains(t, err, "signature is invalid")
}
