package middleware

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withCanvasSidebarOption(t *testing.T, value string) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	previous, existed := common.OptionMap["SidebarModulesAdmin"]
	common.OptionMap["SidebarModulesAdmin"] = value
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		if existed {
			common.OptionMap["SidebarModulesAdmin"] = previous
		} else {
			delete(common.OptionMap, "SidebarModulesAdmin")
		}
		common.OptionMapRWMutex.Unlock()
	})
}

func canvasGuardResponse(method, origin string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CanvasOriginGuard())
	router.Any("/canvas/v1/models", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := httptest.NewRequest(method, "/canvas/v1/models", nil)
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	if method == http.MethodOptions {
		request.Header.Set("Access-Control-Request-Method", http.MethodGet)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestCanvasOriginGuardUsesExactDefaultAndConfiguredOrigin(t *testing.T) {
	withCanvasSidebarOption(t, "")
	allowed := canvasGuardResponse(http.MethodOptions, DefaultCanvasOrigin)
	require.Equal(t, http.StatusNoContent, allowed.Code)
	assert.Equal(t, DefaultCanvasOrigin, allowed.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", allowed.Header().Get("Access-Control-Allow-Credentials"))

	for _, origin := range []string{"", "https://evil.example", "https://sub.canvas.maolaoapi.com"} {
		denied := canvasGuardResponse(http.MethodGet, origin)
		assert.Equal(t, http.StatusForbidden, denied.Code, origin)
	}

	withCanvasSidebarOption(t, `{"chat":{"canvasOrigin":"canvas.example.com"}}`)
	custom := canvasGuardResponse(http.MethodGet, "https://canvas.example.com")
	assert.Equal(t, http.StatusNoContent, custom.Code)
	assert.Equal(t, "https://canvas.example.com", custom.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, http.StatusForbidden, canvasGuardResponse(http.MethodGet, DefaultCanvasOrigin).Code)
}

func TestIssueCanvasSessionCookieMatchesDefaultLaunchContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	identity := service.AuthIdentity{UserID: 7, SessionID: "canvas-launch-session", UserAuthVersion: 2, SessionVersion: 3}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(authIdentityContextKey, identity)
		c.Set("id", identity.UserID)
		c.Set("session_id", identity.SessionID)
		c.Set("auth_version", identity.UserAuthVersion)
		c.Set("session_version", identity.SessionVersion)
		c.Next()
	})
	router.GET("/api/user/self/groups", IssueCanvasSessionCookie(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodGet, "https://panel.example.com/api/user/self/groups", nil)
	request.TLS = &tls.ConnectionState{}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
	cookies := recorder.Result().Cookies()
	require.Len(t, cookies, 1)
	cookie := cookies[0]
	assert.Equal(t, CanvasSessionCookieName, cookie.Name)
	assert.Equal(t, "/canvas", cookie.Path)
	assert.True(t, cookie.HttpOnly)
	assert.True(t, cookie.Secure)
	assert.Equal(t, http.SameSiteNoneMode, cookie.SameSite)
	parsed, err := parseCanvasSessionTicket(cookie.Value, time.Now().Unix())
	require.NoError(t, err)
	assert.Equal(t, identity, parsed)
}

func TestIssueExtensionSessionCookieMatchesExtensionResourceContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	identity := service.AuthIdentity{UserID: 9, SessionID: "extension-launch-session", UserAuthVersion: 2, SessionVersion: 4}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(authIdentityContextKey, identity)
		c.Set("id", identity.UserID)
		c.Set("session_id", identity.SessionID)
		c.Set("auth_version", identity.UserAuthVersion)
		c.Set("session_version", identity.SessionVersion)
		c.Next()
	})
	router.GET("/api/extensions/", IssueExtensionSessionCookie(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodGet, "https://panel.example.com/api/extensions/", nil)
	request.TLS = &tls.ConnectionState{}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
	cookies := recorder.Result().Cookies()
	require.Len(t, cookies, 1)
	cookie := cookies[0]
	assert.Equal(t, ExtensionSessionCookieName, cookie.Name)
	assert.Equal(t, "/api/extensions", cookie.Path)
	assert.True(t, cookie.HttpOnly)
	assert.True(t, cookie.Secure)
	assert.Equal(t, http.SameSiteStrictMode, cookie.SameSite)
	parsed, err := parseExtensionSessionTicket(cookie.Value, time.Now().Unix())
	require.NoError(t, err)
	assert.Equal(t, identity, parsed)
}

func TestUserSessionAuthAcceptsLiveCanvasTicketAndRejectsPAT(t *testing.T) {
	setupDashboardAuthMiddlewareTest(t)
	user := createMiddlewarePATUser(t, "canvas-session-user", "canvas-pat-must-not-work")
	now := time.Now().Unix()
	session := &model.UserSession{
		SID: "canvas-session-live", UserID: user.Id, Version: 1, UserAuthVersion: user.AuthVersion,
		Status: model.UserSessionStatusActive, RefreshHash: strings.Repeat("a", 64), LoginMethod: "password",
		CreatedAt: now, LastActiveAt: now, ExpiresAt: now + 3600,
	}
	require.NoError(t, model.DB.Create(session).Error)
	identity := service.AuthIdentity{UserID: user.Id, SessionID: session.SID, UserAuthVersion: user.AuthVersion, SessionVersion: session.Version}
	ticket, err := signCanvasSessionTicket(identity, now+600)
	require.NoError(t, err)

	router := gin.New()
	router.Use(CanvasOriginGuard())
	router.GET("/canvas/v1/models", UserSessionAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": c.GetInt("id")})
	})
	request := httptest.NewRequest(http.MethodGet, "/canvas/v1/models", nil)
	request.Header.Set("Origin", DefaultCanvasOrigin)
	request.AddCookie(&http.Cookie{Name: CanvasSessionCookieName, Value: ticket})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusOK, recorder.Code)

	patRequest := httptest.NewRequest(http.MethodGet, "/canvas/v1/models", nil)
	patRequest.Header.Set("Origin", DefaultCanvasOrigin)
	patRequest.Header.Set("Authorization", "Bearer canvas-pat-must-not-work")
	patRecorder := httptest.NewRecorder()
	router.ServeHTTP(patRecorder, patRequest)
	assert.Equal(t, http.StatusUnauthorized, patRecorder.Code)
}

func TestUserSessionAuthAcceptsLiveExtensionTicket(t *testing.T) {
	setupDashboardAuthMiddlewareTest(t)
	user := createMiddlewarePATUser(t, "extension-session-user", "extension-pat-must-not-work")
	now := time.Now().Unix()
	session := &model.UserSession{
		SID: "extension-session-live", UserID: user.Id, Version: 1, UserAuthVersion: user.AuthVersion,
		Status: model.UserSessionStatusActive, RefreshHash: strings.Repeat("b", 64), LoginMethod: "password",
		CreatedAt: now, LastActiveAt: now, ExpiresAt: now + 3600,
	}
	require.NoError(t, model.DB.Create(session).Error)
	identity := service.AuthIdentity{UserID: user.Id, SessionID: session.SID, UserAuthVersion: user.AuthVersion, SessionVersion: session.Version}
	ticket, err := signExtensionSessionTicket(identity, now+600)
	require.NoError(t, err)

	router := gin.New()
	router.GET("/api/extensions/okx-alipay-rate/native/index/classic/entry", UserSessionAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": c.GetInt("id")})
	})
	router.GET("/canvas/v1/models", UserSessionAuth(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/api/extensions/okx-alipay-rate/native/index/classic/entry", nil)
	request.AddCookie(&http.Cookie{Name: ExtensionSessionCookieName, Value: ticket})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"id":`+strconv.Itoa(user.Id)+`}`, recorder.Body.String())

	canvasTicket, err := signCanvasSessionTicket(identity, now+600)
	require.NoError(t, err)
	crossExtensionRequest := httptest.NewRequest(http.MethodGet, "/api/extensions/okx-alipay-rate/native/index/classic/entry", nil)
	crossExtensionRequest.AddCookie(&http.Cookie{Name: CanvasSessionCookieName, Value: canvasTicket})
	crossExtensionRecorder := httptest.NewRecorder()
	router.ServeHTTP(crossExtensionRecorder, crossExtensionRequest)
	assert.Equal(t, http.StatusUnauthorized, crossExtensionRecorder.Code)

	crossCanvasRequest := httptest.NewRequest(http.MethodGet, "/canvas/v1/models", nil)
	crossCanvasRequest.AddCookie(&http.Cookie{Name: ExtensionSessionCookieName, Value: ticket})
	crossCanvasRecorder := httptest.NewRecorder()
	router.ServeHTTP(crossCanvasRecorder, crossCanvasRequest)
	assert.Equal(t, http.StatusUnauthorized, crossCanvasRecorder.Code)
}
