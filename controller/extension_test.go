package controller

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/extension"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestListExtensionsReturnsAuthenticatedPublicManifestSchema(t *testing.T) {
	rootDir := t.TempDir()
	moduleDir := filepath.Join(rootDir, "public-module")
	require.NoError(t, os.MkdirAll(moduleDir, 0o755))
	manifest := extension.Manifest{
		ID: "public-module", Name: "Public Module", Version: "1.2.3", Description: "module description", Author: "author",
		Runtime: extension.Runtime{Type: extension.RuntimeTypeHTTP, BaseURL: "https://upstream.example", HealthPath: "/health"},
		UI: extension.UIContribution{
			Nav:   []extension.NavItem{{Title: "Public Module", Page: "index", Icon: "Box", Section: "admin", Order: 5}},
			Pages: []extension.Page{{Key: "index", Title: "Public Module", Path: "/", Embed: true}},
		},
		Permissions: extension.PermissionConfig{Roles: []string{"user"}},
	}
	manifestData, err := common.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "manifest.json"), manifestData, 0o644))
	manager := extension.NewManager(rootDir)
	require.NoError(t, manager.Scan())
	_, err = manager.SetEnabled(manifest.ID, true)
	require.NoError(t, err)
	originalManager := extension.DefaultManager
	extension.DefaultManager = manager
	t.Cleanup(func() { extension.DefaultManager = originalManager })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/extensions/", nil)
	ctx.Set("id", 7)
	ctx.Set("role", common.RoleCommonUser)
	ListExtensions(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Root    string                   `json:"root"`
			Modules []extension.PublicModule `json:"modules"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Empty(t, response.Data.Root)
	require.Len(t, response.Data.Modules, 1)
	module := response.Data.Modules[0]
	require.Equal(t, "public-module", module.ID)
	require.Equal(t, "1.2.3", module.Version)
	require.True(t, module.Enabled)
	require.Equal(t, extension.RuntimeTypeHTTP, module.Runtime.Type)
	require.Equal(t, "/health", module.Runtime.HealthPath)
	require.NotContains(t, recorder.Body.String(), "upstream.example")
	require.NotContains(t, recorder.Body.String(), moduleDir)
	require.Empty(t, module.Error)
}

func TestListExtensionsAllQueryOnlyAppliesToRoot(t *testing.T) {
	rootDir := t.TempDir()
	moduleDir := filepath.Join(rootDir, "disabled-module")
	require.NoError(t, os.MkdirAll(moduleDir, 0o755))
	manifest := extension.Manifest{ID: "disabled-module", Name: "Disabled", Version: "1.0.0", Runtime: extension.Runtime{Type: extension.RuntimeTypeHTTP, BaseURL: "https://example.com"}}
	manifestData, err := common.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "manifest.json"), manifestData, 0o644))
	manager := extension.NewManager(rootDir)
	require.NoError(t, manager.Scan())
	originalManager := extension.DefaultManager
	extension.DefaultManager = manager
	t.Cleanup(func() { extension.DefaultManager = originalManager })

	call := func(role int) int {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/extensions/?all=true", nil)
		ctx.Set("id", 1)
		ctx.Set("role", role)
		ListExtensions(ctx)
		var response struct {
			Data struct {
				Modules []extension.PublicModule `json:"modules"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		return len(response.Data.Modules)
	}
	require.Zero(t, call(common.RoleCommonUser))
	require.Equal(t, 1, call(common.RoleRootUser))
}

func TestExtensionAdminControllersDenyNonRootAtHelperBoundary(t *testing.T) {
	handlers := []struct {
		name   string
		method string
		path   string
		call   func(*gin.Context)
	}{
		{name: "refresh", method: http.MethodPost, path: "/api/extension-admin/refresh", call: RefreshExtensions},
		{name: "upload", method: http.MethodPost, path: "/api/extension-admin/upload", call: UploadExtension},
		{name: "enable", method: http.MethodPut, path: "/api/extension-admin/module/enabled", call: SetExtensionEnabled},
		{name: "uninstall", method: http.MethodDelete, path: "/api/extension-admin/module", call: UninstallExtension},
		{name: "rate config", method: http.MethodGet, path: "/api/extension-admin/okx-alipay-rate/config", call: GetOkxAlipayRateConfig},
		{name: "rate preview", method: http.MethodGet, path: "/api/extension-admin/okx-alipay-rate/quote", call: PreviewOkxAlipayRate},
	}
	for _, test := range handlers {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(test.method, test.path, nil)
			ctx.Set("id", 8)
			ctx.Set("role", common.RoleAdminUser)
			test.call(ctx)
			require.Equal(t, http.StatusForbidden, recorder.Code)
		})
	}
}

func TestGetExtensionHostContextRequiresAuthenticatedUser(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/extensions/host/me", nil)
	ctx.Set("id", 42)
	ctx.Set("username", "alice")
	ctx.Set("role", common.RoleCommonUser)
	ctx.Set("group", "default")
	ctx.Set("use_access_token", true)
	GetExtensionHostContext(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"user_id":42`)
	require.Contains(t, recorder.Body.String(), `"username":"alice"`)

	invalid := httptest.NewRecorder()
	invalidContext, _ := gin.CreateTestContext(invalid)
	invalidContext.Request = httptest.NewRequest(http.MethodGet, "/api/extensions/host/me", nil)
	invalidContext.Set("id", 42)
	invalidContext.Set("role", common.RoleRootUser+1)
	GetExtensionHostContext(invalidContext)
	require.Equal(t, http.StatusForbidden, invalid.Code)
}
