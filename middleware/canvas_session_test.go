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
	assert.Equal(t, int(service.DirectLoginSessionTTL/time.Second), cookie.MaxAge)
	parsed, err := parseCanvasSessionTicket(cookie.Value, time.Now().Unix())
	require.NoError(t, err)
	assert.Equal(t, identity, parsed)
	_, err = parseCanvasSessionTicket(cookie.Value, time.Now().Add(8*time.Hour+time.Second).Unix())
	require.NoError(t, err)
	_, err = parseCanvasSessionTicket(cookie.Value, time.Now().Add(service.DirectLoginSessionTTL+time.Second).Unix())
	assert.Error(t, err)
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
	assert.Equal(t, int(service.DirectLoginSessionTTL/time.Second), cookie.MaxAge)
	parsed, err := parseExtensionSessionTicket(cookie.Value, time.Now().Unix())
	require.NoError(t, err)
	assert.Equal(t, identity, parsed)
	_, err = parseExtensionSessionTicket(cookie.Value, time.Now().Add(2*time.Hour+time.Second).Unix())
	require.NoError(t, err)
	_, err = parseExtensionSessionTicket(cookie.Value, time.Now().Add(service.DirectLoginSessionTTL+time.Second).Unix())
	assert.Error(t, err)
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

func TestUserSessionAuthFallsBackToLiveCanvasTicketWhenAuthorizationUnusable(t *testing.T) {
	setupDashboardAuthMiddlewareTest(t)
	user := createMiddlewarePATUser(t, "canvas-fallback-user", "canvas-pat-leftover")
	now := time.Now().Unix()
	session := &model.UserSession{
		SID: "canvas-fallback-live", UserID: user.Id, Version: 1, UserAuthVersion: user.AuthVersion,
		Status: model.UserSessionStatusActive, RefreshHash: strings.Repeat("c", 64), LoginMethod: "password",
		CreatedAt: now, LastActiveAt: now, ExpiresAt: now + int64(service.LoginSessionTTL/time.Second),
	}
	require.NoError(t, model.DB.Create(session).Error)
	identity := service.AuthIdentity{UserID: user.Id, SessionID: session.SID, UserAuthVersion: user.AuthVersion, SessionVersion: session.Version}
	ticket, err := signCanvasSessionTicket(identity, now+600)
	require.NoError(t, err)
	expiredJWT := issueExpiredDashboardAccessToken(t, identity)

	router := gin.New()
	router.Use(CanvasOriginGuard())
	router.GET("/canvas/v1/models", UserSessionAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": c.GetInt("id")})
	})

	for _, authorization := range []string{"Bearer canvas-pat-leftover", "Bearer " + expiredJWT} {
		request := httptest.NewRequest(http.MethodGet, "/canvas/v1/models", nil)
		request.Header.Set("Origin", DefaultCanvasOrigin)
		request.Header.Set("Authorization", authorization)
		request.AddCookie(&http.Cookie{Name: CanvasSessionCookieName, Value: ticket})
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusOK, recorder.Code, authorization)
		assert.JSONEq(t, `{"id":`+strconv.Itoa(user.Id)+`}`, recorder.Body.String())
	}

	expiredOnly := httptest.NewRequest(http.MethodGet, "/canvas/v1/models", nil)
	expiredOnly.Header.Set("Origin", DefaultCanvasOrigin)
	expiredOnly.Header.Set("Authorization", "Bearer "+expiredJWT)
	expiredOnlyRecorder := httptest.NewRecorder()
	router.ServeHTTP(expiredOnlyRecorder, expiredOnly)
	assert.Equal(t, http.StatusUnauthorized, expiredOnlyRecorder.Code)
}

func TestUserSessionAuthFallsBackWhenBearerSessionIsDeadAndPrefersLiveJWT(t *testing.T) {
	setupDashboardAuthMiddlewareTest(t)
	user := createMiddlewarePATUser(t, "canvas-jwt-fallback-user", "unused-pat")
	now := time.Now().Unix()
	oldSession := &model.UserSession{
		SID: "canvas-old-session", UserID: user.Id, Version: 1, UserAuthVersion: user.AuthVersion,
		Status: model.UserSessionStatusActive, RefreshHash: strings.Repeat("d", 64), LoginMethod: "password",
		CreatedAt: now, LastActiveAt: now, ExpiresAt: now + 3600,
	}
	liveSession := &model.UserSession{
		SID: "canvas-live-session", UserID: user.Id, Version: 1, UserAuthVersion: user.AuthVersion,
		Status: model.UserSessionStatusActive, RefreshHash: strings.Repeat("e", 64), LoginMethod: "password",
		CreatedAt: now, LastActiveAt: now, ExpiresAt: now + 3600,
	}
	jwtSession := &model.UserSession{
		SID: "canvas-jwt-session", UserID: user.Id, Version: 1, UserAuthVersion: user.AuthVersion,
		Status: model.UserSessionStatusActive, RefreshHash: strings.Repeat("f", 64), LoginMethod: "password",
		CreatedAt: now, LastActiveAt: now, ExpiresAt: now + 3600,
	}
	require.NoError(t, model.DB.Create(oldSession).Error)
	require.NoError(t, model.DB.Create(liveSession).Error)
	require.NoError(t, model.DB.Create(jwtSession).Error)
	oldIdentity := service.AuthIdentity{UserID: user.Id, SessionID: oldSession.SID, UserAuthVersion: user.AuthVersion, SessionVersion: oldSession.Version}
	liveIdentity := service.AuthIdentity{UserID: user.Id, SessionID: liveSession.SID, UserAuthVersion: user.AuthVersion, SessionVersion: liveSession.Version}
	jwtIdentity := service.AuthIdentity{UserID: user.Id, SessionID: jwtSession.SID, UserAuthVersion: user.AuthVersion, SessionVersion: jwtSession.Version}
	oldJWT, _, err := service.IssueAccessToken(oldIdentity)
	require.NoError(t, err)
	liveJWT, _, err := service.IssueAccessToken(jwtIdentity)
	require.NoError(t, err)
	liveTicket, err := signCanvasSessionTicket(liveIdentity, now+600)
	require.NoError(t, err)
	revoked, err := model.RevokeUserSession(user.Id, oldSession.SID, "logout")
	require.NoError(t, err)
	require.True(t, revoked)

	router := gin.New()
	router.Use(CanvasOriginGuard())
	router.GET("/canvas/v1/models", UserSessionAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": c.GetInt("id"), "sid": c.GetString("session_id")})
	})

	fallback := httptest.NewRequest(http.MethodGet, "/canvas/v1/models", nil)
	fallback.Header.Set("Origin", DefaultCanvasOrigin)
	fallback.Header.Set("Authorization", "Bearer "+oldJWT)
	fallback.AddCookie(&http.Cookie{Name: CanvasSessionCookieName, Value: liveTicket})
	fallbackRecorder := httptest.NewRecorder()
	router.ServeHTTP(fallbackRecorder, fallback)
	assert.Equal(t, http.StatusOK, fallbackRecorder.Code)
	assert.JSONEq(t, `{"id":`+strconv.Itoa(user.Id)+`,"sid":"canvas-live-session"}`, fallbackRecorder.Body.String())

	jwtOnly := httptest.NewRequest(http.MethodGet, "/canvas/v1/models", nil)
	jwtOnly.Header.Set("Origin", DefaultCanvasOrigin)
	jwtOnly.Header.Set("Authorization", "Bearer "+liveJWT)
	jwtOnlyRecorder := httptest.NewRecorder()
	router.ServeHTTP(jwtOnlyRecorder, jwtOnly)
	assert.Equal(t, http.StatusOK, jwtOnlyRecorder.Code)
	assert.JSONEq(t, `{"id":`+strconv.Itoa(user.Id)+`,"sid":"canvas-jwt-session"}`, jwtOnlyRecorder.Body.String())

	preferJWT := httptest.NewRequest(http.MethodGet, "/canvas/v1/models", nil)
	preferJWT.Header.Set("Origin", DefaultCanvasOrigin)
	preferJWT.Header.Set("Authorization", "Bearer "+liveJWT)
	preferJWT.AddCookie(&http.Cookie{Name: CanvasSessionCookieName, Value: liveTicket})
	preferRecorder := httptest.NewRecorder()
	router.ServeHTTP(preferRecorder, preferJWT)
	assert.Equal(t, http.StatusOK, preferRecorder.Code)
	assert.JSONEq(t, `{"id":`+strconv.Itoa(user.Id)+`,"sid":"canvas-jwt-session"}`, preferRecorder.Body.String())
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

	leftover := httptest.NewRequest(http.MethodGet, "/api/extensions/okx-alipay-rate/native/index/classic/entry", nil)
	leftover.Header.Set("Authorization", "Bearer extension-pat-must-not-work")
	leftover.AddCookie(&http.Cookie{Name: ExtensionSessionCookieName, Value: ticket})
	leftoverRecorder := httptest.NewRecorder()
	router.ServeHTTP(leftoverRecorder, leftover)
	assert.Equal(t, http.StatusOK, leftoverRecorder.Code)
	assert.JSONEq(t, `{"id":`+strconv.Itoa(user.Id)+`}`, leftoverRecorder.Body.String())

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
