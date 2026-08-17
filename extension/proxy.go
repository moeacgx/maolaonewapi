package extension

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"log"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/service"
)

var sensitiveForwardHeaders = []string{
	"Authorization",
	"Cookie",
	"Proxy-Authorization",
	"X-Auth-Session",
	"X-Access-Token",
	"X-API-Key",
}

var representationResponseHeaders = []string{
	"Accept-Ranges",
	"Content-Encoding",
	"Content-Language",
	"Content-Length",
	"Content-Range",
	"Content-Type",
	"ETag",
	"Last-Modified",
	"Vary",
}

var blockedResponseHeaders = []string{
	"Access-Control-Allow-Credentials",
	"Access-Control-Allow-Headers",
	"Access-Control-Allow-Methods",
	"Access-Control-Allow-Origin",
	"Access-Control-Expose-Headers",
	"Access-Control-Max-Age",
	"Access-Control-Request-Headers",
	"Access-Control-Request-Method",
	"Authorization",
	"Authentication-Info",
	"Clear-Site-Data",
	"Content-Disposition",
	"Content-Location",
	"Location",
	"Proxy-Authenticate",
	"Proxy-Authentication-Info",
	"Refresh",
	"Set-Cookie",
	"Set-Cookie2",
	"WWW-Authenticate",
}

var (
	validateExtensionOutboundURL = service.ValidateSSRFProtectedFetchURL
	extensionProtectedHTTPClient = service.GetSSRFProtectedHTTPClient
	errStaticAssetNotFound       = errors.New("static extension asset not found")
	errStaticAssetUnsafe         = errors.New("static extension asset path is unsafe")
)

var hopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

const (
	extensionHTMLContentSecurityPolicy = "default-src 'self' data: blob:; base-uri 'none'; object-src 'none'; form-action 'self'; frame-ancestors 'self'; script-src 'self' 'unsafe-inline' blob:; style-src 'self' 'unsafe-inline'; connect-src 'self' https: wss:; img-src 'self' data: blob: https:; font-src 'self' data:; sandbox allow-forms allow-popups allow-popups-to-escape-sandbox allow-scripts"
	extensionIdentitySignatureHeader   = "X-NewAPI-Signature"
	extensionIdentityTimestampHeader   = "X-NewAPI-Timestamp"
	extensionIdentityPayloadVersion    = "new-api-extension-identity-v1"
)

var extensionIdentityHeaders = []string{
	"X-NewAPI-Module-ID",
	"X-NewAPI-User-ID",
	"X-NewAPI-Username",
	"X-NewAPI-User-Role",
	"X-NewAPI-User-Group",
	"X-NewAPI-Use-Access-Token",
	extensionIdentityTimestampHeader,
	extensionIdentitySignatureHeader,
}

type ProxyContext struct {
	UserID         string
	Username       string
	Role           string
	Group          string
	UseAccessToken string
}

func (m *Manager) ProxyHandler(moduleID string, proxyPath string, role int, ctx ProxyContext) (http.Handler, error) {
	module, ok := m.Get(strings.TrimSpace(moduleID))
	if !ok {
		return nil, errors.New("module not found")
	}
	if module.Error != "" {
		if module.Error == "module is incompatible with current host version" {
			return nil, errors.New("module is incompatible with current host version")
		}
		return nil, errors.New("module manifest is invalid")
	}
	if err := hostCompatibilityError(module.Manifest); err != nil {
		return nil, errors.New("module is incompatible with current host version")
	}
	if !module.Enabled {
		return nil, errors.New("module is disabled")
	}
	if !roleAllowed(role, module.Permissions.Roles) {
		return nil, errors.New("module permission denied")
	}
	if module.Runtime.Type == RuntimeTypeStatic {
		return staticHandler(module, proxyPath, ctx)
	}
	target, err := url.Parse(strings.TrimSpace(module.Runtime.BaseURL))
	if err != nil || target == nil || target.Host == "" || target.User != nil || target.RawQuery != "" || target.Fragment != "" {
		return nil, errors.New("runtime.base_url is invalid")
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, errors.New("runtime.base_url only supports http or https")
	}
	if err := validateExtensionOutboundURL(target.String()); err != nil {
		return nil, errors.New("runtime.base_url is blocked by outbound policy")
	}
	client := extensionProtectedHTTPClient()
	if client == nil || client.Transport == nil {
		return nil, errors.New("protected outbound transport is unavailable")
	}

	cleanPath := cleanProxyPath(proxyPath)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = client.Transport
	proxy.ErrorLog = log.New(io.Discard, "", 0)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		req.URL.Path = cleanPath
		req.URL.RawPath = ""
		originalDirector(req)
		req.Host = target.Host
		stripHopByHopHeaders(req.Header)
		for _, header := range sensitiveForwardHeaders {
			req.Header.Del(header)
		}
		setExtensionContextHeaders(req.Header, module.ID, module.Runtime.IdentitySecret, ctx, req.Method, extensionRequestPath(req), time.Now())
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		filterRepresentationResponseHeaders(resp.Header)
		resp.Header.Set("Cache-Control", "no-store")
		resp.Header.Set("Pragma", "no-cache")
		resp.Header.Set("Expires", "0")
		resp.Header.Set("X-Content-Type-Options", "nosniff")
		resp.Header.Set("Cross-Origin-Resource-Policy", "same-origin")
		if isHTMLContentType(resp.Header.Get("Content-Type")) {
			resp.Header.Set("Content-Security-Policy", extensionHTMLContentSecurityPolicy)
		}
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.Error(w, "extension upstream is unavailable", http.StatusBadGateway)
	}
	return proxy, nil
}

func staticHandler(module Module, proxyPath string, ctx ProxyContext) (http.Handler, error) {
	root, err := secureStaticRoot(module)
	if err != nil {
		return nil, err
	}
	cleanPath, err := validateStaticProxyPath(proxyPath)
	if err != nil {
		return nil, err
	}
	if _, err := secureStaticAssetPath(root, cleanPath); err != nil {
		if !errors.Is(err, errStaticAssetNotFound) {
			return nil, err
		}
		cleanPath = "/index.html"
		if _, err := secureStaticAssetPath(root, cleanPath); err != nil {
			return nil, errors.New("static module entry is unavailable")
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, private, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.Header().Set("X-NewAPI-Module-Version", module.Version)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		r.Header.Del("If-Modified-Since")
		r.Header.Del("If-None-Match")
		stripHopByHopHeaders(r.Header)
		for _, header := range sensitiveForwardHeaders {
			r.Header.Del(header)
		}
		setExtensionContextHeaders(r.Header, module.ID, module.Runtime.IdentitySecret, ctx, r.Method, extensionRequestPath(r), time.Now())

		targetPath, err := secureStaticAssetPath(root, cleanPath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		file, err := os.Open(targetPath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(targetPath)))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		w.Header().Set("Content-Type", contentType)
		if isHTMLContentType(contentType) {
			w.Header().Set("Content-Security-Policy", extensionHTMLContentSecurityPolicy)
		}
		http.ServeContent(w, r, filepath.Base(targetPath), info.ModTime(), file)
	}), nil
}

func isHTMLContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	return err == nil && strings.EqualFold(mediaType, "text/html")
}

func extensionRequestPath(request *http.Request) string {
	if request == nil || request.URL == nil {
		return "/"
	}
	requestPath := request.URL.EscapedPath()
	if requestPath == "" {
		return "/"
	}
	return requestPath
}

func setExtensionContextHeaders(header http.Header, moduleID, secret string, ctx ProxyContext, method, requestPath string, now time.Time) {
	for _, name := range extensionIdentityHeaders {
		header.Del(name)
	}
	header.Set("X-NewAPI-Module-ID", moduleID)
	if strings.TrimSpace(secret) == "" {
		return
	}
	timestamp := strconv.FormatInt(now.Unix(), 10)
	header.Set("X-NewAPI-User-ID", ctx.UserID)
	header.Set("X-NewAPI-Username", ctx.Username)
	header.Set("X-NewAPI-User-Role", ctx.Role)
	header.Set("X-NewAPI-User-Group", ctx.Group)
	header.Set("X-NewAPI-Use-Access-Token", ctx.UseAccessToken)
	header.Set(extensionIdentityTimestampHeader, timestamp)
	header.Set(extensionIdentitySignatureHeader, extensionIdentitySignature(secret, moduleID, ctx, method, requestPath, timestamp))
}

func writeExtensionIdentityField(mac hash.Hash, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = mac.Write(length[:])
	_, _ = io.WriteString(mac, value)
}

func extensionIdentitySignature(secret, moduleID string, ctx ProxyContext, method, requestPath, timestamp string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	for _, value := range []string{
		extensionIdentityPayloadVersion,
		moduleID,
		ctx.UserID,
		ctx.Username,
		ctx.Role,
		ctx.Group,
		ctx.UseAccessToken,
		method,
		requestPath,
		timestamp,
	} {
		writeExtensionIdentityField(mac, value)
	}
	return hex.EncodeToString(mac.Sum(nil))
}

func extensionSingleHeader(header http.Header, name string) (string, error) {
	values := header.Values(name)
	if len(values) != 1 {
		return "", errors.New("extension identity signature is invalid")
	}
	return values[0], nil
}

// VerifyExtensionContextHeaders authenticates identity headers emitted by an HTTP extension proxy.
func VerifyExtensionContextHeaders(header http.Header, secret, expectedModuleID, method, requestPath string, now time.Time, replayWindow time.Duration) (ProxyContext, error) {
	if strings.TrimSpace(secret) == "" || strings.TrimSpace(expectedModuleID) == "" || replayWindow <= 0 {
		return ProxyContext{}, errors.New("extension identity signature is invalid")
	}
	moduleID, err := extensionSingleHeader(header, "X-NewAPI-Module-ID")
	if err != nil || moduleID != expectedModuleID {
		return ProxyContext{}, errors.New("extension identity signature is invalid")
	}
	timestampText, err := extensionSingleHeader(header, extensionIdentityTimestampHeader)
	if err != nil {
		return ProxyContext{}, err
	}
	timestamp, err := strconv.ParseInt(timestampText, 10, 64)
	if err != nil || timestampText != strconv.FormatInt(timestamp, 10) {
		return ProxyContext{}, errors.New("extension identity signature is invalid")
	}
	signedAt := time.Unix(timestamp, 0)
	if signedAt.Before(now.Add(-replayWindow)) || signedAt.After(now.Add(replayWindow)) {
		return ProxyContext{}, errors.New("extension identity timestamp is outside the replay window")
	}
	ctx := ProxyContext{}
	if ctx.UserID, err = extensionSingleHeader(header, "X-NewAPI-User-ID"); err != nil {
		return ProxyContext{}, err
	}
	if ctx.Username, err = extensionSingleHeader(header, "X-NewAPI-Username"); err != nil {
		return ProxyContext{}, err
	}
	if ctx.Role, err = extensionSingleHeader(header, "X-NewAPI-User-Role"); err != nil {
		return ProxyContext{}, err
	}
	if ctx.Group, err = extensionSingleHeader(header, "X-NewAPI-User-Group"); err != nil {
		return ProxyContext{}, err
	}
	if ctx.UseAccessToken, err = extensionSingleHeader(header, "X-NewAPI-Use-Access-Token"); err != nil {
		return ProxyContext{}, err
	}
	signatureText, err := extensionSingleHeader(header, extensionIdentitySignatureHeader)
	if err != nil {
		return ProxyContext{}, err
	}
	signature, err := hex.DecodeString(signatureText)
	if err != nil {
		return ProxyContext{}, errors.New("extension identity signature is invalid")
	}
	expectedSignature := extensionIdentitySignature(secret, moduleID, ctx, method, requestPath, timestampText)
	expectedBytes, _ := hex.DecodeString(expectedSignature)
	if !hmac.Equal(signature, expectedBytes) {
		return ProxyContext{}, errors.New("extension identity signature is invalid")
	}
	return ctx, nil
}

func stripHopByHopHeaders(header http.Header) {
	for _, value := range header.Values("Connection") {
		for _, token := range strings.Split(value, ",") {
			header.Del(strings.TrimSpace(token))
		}
	}
	for _, name := range hopByHopHeaders {
		header.Del(name)
	}
}

func filterRepresentationResponseHeaders(header http.Header) {
	allowed := make(http.Header, len(representationResponseHeaders))
	for _, name := range representationResponseHeaders {
		if values := header.Values(name); len(values) != 0 {
			allowed[name] = append([]string(nil), values...)
		}
	}
	for name := range header {
		delete(header, name)
	}
	for name, values := range allowed {
		header[name] = values
	}
	for _, name := range blockedResponseHeaders {
		header.Del(name)
	}
}

func secureStaticRoot(module Module) (string, error) {
	if strings.TrimSpace(module.Path) == "" {
		return "", errors.New("static module directory is unavailable")
	}
	modulePath, err := filepath.Abs(module.Path)
	if err != nil {
		return "", errors.New("static module directory is unavailable")
	}
	staticDir := strings.TrimSpace(module.Runtime.StaticDir)
	if staticDir == "" {
		staticDir = DefaultStaticDir
	}
	staticDir = filepath.Clean(filepath.FromSlash(staticDir))
	if filepath.IsAbs(staticDir) || staticDir == "." || staticDir == ".." || strings.HasPrefix(staticDir, ".."+string(filepath.Separator)) || hasDotPathSegment(filepath.ToSlash(staticDir)) {
		return "", errors.New("static module directory is invalid")
	}
	root := filepath.Join(modulePath, staticDir)
	if err := ensurePathInside(modulePath, root); err != nil {
		return "", errors.New("static module directory escapes module path")
	}
	if err := rejectSymlinkComponents(modulePath, root); err != nil {
		if errors.Is(err, errStaticAssetUnsafe) {
			return "", errors.New("static module directory must not contain symbolic links")
		}
		return "", errors.New("static module directory is unavailable")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() {
		return "", errors.New("static module directory is unavailable")
	}
	resolvedModulePath, err := filepath.EvalSymlinks(modulePath)
	if err != nil {
		return "", errors.New("static module directory is unavailable")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", errors.New("static module directory is unavailable")
	}
	if err := ensurePathInside(resolvedModulePath, resolvedRoot); err != nil {
		return "", errors.New("static module directory escapes module path")
	}
	return resolvedRoot, nil
}

func validateStaticProxyPath(value string) (string, error) {
	if value == "" {
		return "/", nil
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	if hasDotPathSegment(value) {
		return "", errors.New("static module request path must not contain dot-prefixed segments")
	}
	if err := validatePagePath(value); err != nil {
		return "", errors.New("static module request path is invalid")
	}
	return value, nil
}

func secureStaticAssetPath(root, requestPath string) (string, error) {
	if hasDotPathSegment(requestPath) {
		return "", errStaticAssetUnsafe
	}
	if err := validatePagePath(requestPath); err != nil {
		return "", errStaticAssetUnsafe
	}
	if requestPath == "/" {
		requestPath = "/index.html"
	}
	target := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(requestPath, "/")))
	if err := ensurePathInside(root, target); err != nil {
		return "", errStaticAssetUnsafe
	}
	if err := rejectSymlinkComponents(root, target); err != nil {
		return "", err
	}
	targetInfo, err := os.Lstat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errStaticAssetNotFound
		}
		return "", errors.New("static extension asset is unavailable")
	}
	if !targetInfo.Mode().IsRegular() {
		return "", errStaticAssetUnsafe
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", errors.New("static extension asset is unavailable")
	}
	if err := ensurePathInside(root, resolvedTarget); err != nil {
		return "", errStaticAssetUnsafe
	}
	return resolvedTarget, nil
}

func rejectSymlinkComponents(base, target string) error {
	if err := ensurePathInside(base, target); err != nil {
		return errStaticAssetUnsafe
	}
	baseInfo, err := os.Lstat(base)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errStaticAssetNotFound
		}
		return errors.New("static extension asset is unavailable")
	}
	if baseInfo.Mode()&os.ModeSymlink != 0 {
		return errStaticAssetUnsafe
	}
	relative, err := filepath.Rel(base, target)
	if err != nil {
		return errStaticAssetUnsafe
	}
	current := base
	if relative == "." {
		return nil
	}
	for _, segment := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, segment)
		info, err := os.Lstat(current)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return errStaticAssetNotFound
			}
			return errors.New("static extension asset is unavailable")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errStaticAssetUnsafe
		}
	}
	return nil
}

func hasDotPathSegment(value string) bool {
	for _, segment := range strings.Split(value, "/") {
		if strings.HasPrefix(segment, ".") {
			return true
		}
	}
	return false
}

func cleanProxyPath(value string) string {
	if value == "" {
		return "/"
	}
	value = strings.ReplaceAll(value, "\\", "/")
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	cleaned := path.Clean(value)
	if cleaned == "." {
		return "/"
	}
	return cleaned
}
