package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const (
	DefaultCanvasOrigin        = "https://canvas.maolaoapi.com"
	CanvasSessionCookieName    = "new_api_canvas"
	ExtensionSessionCookieName = "new_api_extension"
	canvasSessionTTL           = 8 * time.Hour
	extensionSessionTTL        = 2 * time.Hour
)

type canvasSessionTicket struct {
	UserID          int    `json:"uid"`
	SessionID       string `json:"sid"`
	UserAuthVersion int64  `json:"uv"`
	SessionVersion  int64  `json:"sv"`
	ExpiresAt       int64  `json:"exp"`
}

type sessionTicketPurpose string

const (
	sessionTicketPurposeCanvas    sessionTicketPurpose = "new-api/canvas-session/v1"
	sessionTicketPurposeExtension sessionTicketPurpose = "new-api/extension-session/v1"
)

func CanvasConfiguredOrigin() string {
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap["SidebarModulesAdmin"]
	common.OptionMapRWMutex.RUnlock()

	var modules struct {
		Chat struct {
			CanvasOrigin string `json:"canvasOrigin"`
		} `json:"chat"`
	}
	candidate := ""
	if strings.TrimSpace(raw) != "" && common.UnmarshalJsonStr(raw, &modules) == nil {
		candidate = strings.TrimSpace(modules.Chat.CanvasOrigin)
	}
	if candidate == "" {
		candidate = DefaultCanvasOrigin
	} else if !strings.Contains(candidate, "://") {
		candidate = "https://" + candidate
	}
	parsed, err := url.Parse(candidate)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return DefaultCanvasOrigin
	}
	origin, err := common.NormalizeOrigin(parsed.Scheme + "://" + parsed.Host)
	if err != nil {
		return DefaultCanvasOrigin
	}
	return origin
}

// CanvasRequestOriginTrusted applies the same exact-origin check as
// CanvasOriginGuard so downstream trust establishment cannot rely only on
// router ordering.
func CanvasRequestOriginTrusted(request *http.Request) bool {
	if request == nil {
		return false
	}
	origin, ok := requestBrowserOrigin(request)
	return ok && subtle.ConstantTimeCompare([]byte(origin), []byte(CanvasConfiguredOrigin())) == 1
}

// CanvasOriginGuard is both the exact-origin credentialed CORS policy and the
// CSRF boundary for /canvas. Install it on the engine before generic CORS so it
// can answer preflights and restore the exact origin after downstream handlers.
func CanvasOriginGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isCanvasRequestPath(c.Request.URL.Path) {
			c.Next()
			return
		}

		allowed := CanvasConfiguredOrigin()
		if !CanvasRequestOriginTrusted(c.Request) {

			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"code":    "CANVAS_ORIGIN_FORBIDDEN",
				"message": "request origin is not allowed",
			})
			return
		}

		writeCanvasCORSHeaders(c, allowed)
		if c.Request.Method == http.MethodOptions {
			method := strings.ToUpper(strings.TrimSpace(c.GetHeader("Access-Control-Request-Method")))
			if method != http.MethodGet && method != http.MethodPost {
				c.AbortWithStatus(http.StatusMethodNotAllowed)
				return
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
		writeCanvasCORSHeaders(c, allowed)
	}
}

func isCanvasRequestPath(path string) bool {
	return path == "/canvas" || strings.HasPrefix(path, "/canvas/")
}

func writeCanvasCORSHeaders(c *gin.Context, origin string) {
	c.Header("Access-Control-Allow-Origin", origin)
	c.Header("Access-Control-Allow-Credentials", "true")
	c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-API-Key")
	c.Header("Vary", "Origin")
}

// IssueCanvasSessionCookie bridges the authenticated Default launch sequence to
// the external Canvas without putting an API key or bearer token in its URL.
// Mount it on GET /api/user/self/groups after UserAuth and before the handler.
func IssueCanvasSessionCookie() gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := GetSessionAuthIdentity(c)
		if !ok {
			c.Next()
			return
		}
		ticket, err := signCanvasSessionTicket(identity, time.Now().Add(canvasSessionTTL).Unix())
		if err != nil {
			writeDashboardAuthError(c, service.ErrAuthTokenInvalid)
			return
		}

		secure := common.SessionCookieSecure || c.Request.TLS != nil
		sameSite := http.SameSiteLaxMode
		if secure {
			sameSite = http.SameSiteNoneMode
		}
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     CanvasSessionCookieName,
			Value:    ticket,
			Path:     "/canvas",
			MaxAge:   int(canvasSessionTTL / time.Second),
			Expires:  time.Now().Add(canvasSessionTTL),
			HttpOnly: true,
			Secure:   secure,
			SameSite: sameSite,
		})
		c.Next()
	}
}

// IssueExtensionSessionCookie bridges authenticated dashboard API calls to
// browser-loaded extension resources. The cookie is HttpOnly and path-scoped to
// /api/extensions, so sandboxed extension code cannot read the bearer token.
func IssueExtensionSessionCookie() gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := GetSessionAuthIdentity(c)
		if !ok {
			c.Next()
			return
		}
		expiresAt := time.Now().Add(extensionSessionTTL)
		ticket, err := signExtensionSessionTicket(identity, expiresAt.Unix())
		if err != nil {
			writeDashboardAuthError(c, service.ErrAuthTokenInvalid)
			return
		}

		secure := common.SessionCookieSecure || c.Request.TLS != nil
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     ExtensionSessionCookieName,
			Value:    ticket,
			Path:     "/api/extensions",
			MaxAge:   int(extensionSessionTTL / time.Second),
			Expires:  expiresAt,
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteStrictMode,
		})
		c.Next()
	}
}

// UserSessionAuth accepts only a live dashboard session: either its internal
// bearer access token or a scoped HttpOnly browser ticket. Dashboard PATs and
// relay API keys are deliberately rejected.
func UserSessionAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		var identity service.AuthIdentity
		if raw, present := authorizationToken(c.GetHeader("Authorization")); present {
			parsed, internal, err := service.ParseDashboardAccessToken(raw)
			if err != nil || !internal {
				writeDashboardAuthError(c, service.ErrAuthTokenInvalid)
				return
			}
			identity = parsed
		} else {
			parsed, err := parseScopedBrowserSessionCookie(c, time.Now().Unix())
			if err != nil {
				writeDashboardAuthError(c, service.ErrAuthTokenInvalid)
				return
			}
			identity = parsed
		}

		_, user, err := service.ValidateLoginSession(identity)
		if err != nil {
			writeDashboardAuthError(c, err)
			return
		}
		if user.Status != common.UserStatusEnabled || user.Role < common.RoleCommonUser || !validUserInfo(user.Username, user.Role) {
			writeDashboardAuthError(c, service.ErrLoginSessionRevoked)
			return
		}
		setDashboardAuthContext(c, user, identity, false)
		c.Next()
	}
}

func signCanvasSessionTicket(identity service.AuthIdentity, expiresAt int64) (string, error) {
	return signBrowserSessionTicket(identity, expiresAt, sessionTicketPurposeCanvas)
}

func signExtensionSessionTicket(identity service.AuthIdentity, expiresAt int64) (string, error) {
	return signBrowserSessionTicket(identity, expiresAt, sessionTicketPurposeExtension)
}

func signBrowserSessionTicket(identity service.AuthIdentity, expiresAt int64, purpose sessionTicketPurpose) (string, error) {
	if identity.UserID <= 0 || identity.SessionID == "" || identity.UserAuthVersion <= 0 || identity.SessionVersion <= 0 || expiresAt <= time.Now().Unix() {
		return "", errors.New("invalid browser session identity")
	}
	payload, err := common.Marshal(canvasSessionTicket{
		UserID:          identity.UserID,
		SessionID:       identity.SessionID,
		UserAuthVersion: identity.UserAuthVersion,
		SessionVersion:  identity.SessionVersion,
		ExpiresAt:       expiresAt,
	})
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signature := scopedSessionTicketMAC(encoded, purpose)
	return encoded + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func parseCanvasSessionTicket(raw string, nowUnix int64) (service.AuthIdentity, error) {
	return parseBrowserSessionTicket(raw, nowUnix, sessionTicketPurposeCanvas)
}

func parseExtensionSessionTicket(raw string, nowUnix int64) (service.AuthIdentity, error) {
	return parseBrowserSessionTicket(raw, nowUnix, sessionTicketPurposeExtension)
}

func parseScopedBrowserSessionCookie(c *gin.Context, nowUnix int64) (service.AuthIdentity, error) {
	requestPath := c.Request.URL.Path
	if strings.HasPrefix(requestPath, "/canvas") {
		return parseNamedBrowserSessionCookie(c, CanvasSessionCookieName, nowUnix, parseCanvasSessionTicket)
	}
	if strings.HasPrefix(requestPath, "/api/extensions") {
		return parseNamedBrowserSessionCookie(c, ExtensionSessionCookieName, nowUnix, parseExtensionSessionTicket)
	}
	return service.AuthIdentity{}, errors.New("browser session ticket is unsupported for this path")
}

func parseNamedBrowserSessionCookie(
	c *gin.Context,
	name string,
	nowUnix int64,
	parseTicket func(string, int64) (service.AuthIdentity, error),
) (service.AuthIdentity, error) {
	raw, err := c.Cookie(name)
	if err != nil || strings.TrimSpace(raw) == "" {
		return service.AuthIdentity{}, errors.New("browser session ticket is missing")
	}
	identity, err := parseTicket(raw, nowUnix)
	if err != nil {
		return service.AuthIdentity{}, errors.New("invalid browser session ticket")
	}
	return identity, nil
}

func parseBrowserSessionTicket(raw string, nowUnix int64, purpose sessionTicketPurpose) (service.AuthIdentity, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return service.AuthIdentity{}, errors.New("invalid browser session ticket")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, scopedSessionTicketMAC(parts[0], purpose)) {
		return service.AuthIdentity{}, errors.New("invalid browser session signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return service.AuthIdentity{}, errors.New("invalid browser session payload")
	}
	var ticket canvasSessionTicket
	if err := common.Unmarshal(payload, &ticket); err != nil || ticket.ExpiresAt <= nowUnix {
		return service.AuthIdentity{}, errors.New("expired browser session ticket")
	}
	identity := service.AuthIdentity{
		UserID:          ticket.UserID,
		SessionID:       ticket.SessionID,
		UserAuthVersion: ticket.UserAuthVersion,
		SessionVersion:  ticket.SessionVersion,
	}
	if identity.UserID <= 0 || identity.SessionID == "" || identity.UserAuthVersion <= 0 || identity.SessionVersion <= 0 {
		return service.AuthIdentity{}, errors.New("invalid browser session identity")
	}
	return identity, nil
}

func canvasSessionMAC(encodedPayload string) []byte {
	return scopedSessionTicketMAC(encodedPayload, sessionTicketPurposeCanvas)
}

func scopedSessionTicketMAC(encodedPayload string, purpose sessionTicketPurpose) []byte {
	keyMAC := hmac.New(sha256.New, []byte(common.SessionSecret))
	_, _ = keyMAC.Write([]byte(purpose))
	mac := hmac.New(sha256.New, keyMAC.Sum(nil))
	_, _ = mac.Write([]byte(encodedPayload))
	return mac.Sum(nil)
}
