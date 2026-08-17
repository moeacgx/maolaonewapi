package extension

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
)

var hostVersionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

type hostVersion struct {
	core       [3]uint64
	prerelease []string
}

const (
	RuntimeTypeHTTP    = "http"
	RuntimeTypeStatic  = "static"
	DefaultRootDir     = "data/modules"
	DefaultStaticDir   = "public"
	RenderTypeNative   = "native"
	NativeSDKV1        = "v1"
	CapabilityUINative = "ui.native"
)

type Manifest struct {
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	Version       string                   `json:"version"`
	Description   string                   `json:"description,omitempty"`
	Author        string                   `json:"author,omitempty"`
	Host          HostCompat               `json:"host,omitempty"`
	Runtime       Runtime                  `json:"runtime"`
	UI            UIContribution           `json:"ui,omitempty"`
	Permissions   PermissionConfig         `json:"permissions,omitempty"`
	Notifications NotificationContribution `json:"notifications,omitempty"`
}

type HostCompat struct {
	Min string `json:"min,omitempty"`
	Max string `json:"max,omitempty"`
}

type Runtime struct {
	Type           string `json:"type"`
	BaseURL        string `json:"base_url,omitempty"`
	HealthPath     string `json:"health_path,omitempty"`
	StaticDir      string `json:"static_dir,omitempty"`
	IdentitySecret string `json:"identity_secret,omitempty"`
}

type UIContribution struct {
	Nav   []NavItem `json:"nav,omitempty"`
	Pages []Page    `json:"pages,omitempty"`
}

type NavItem struct {
	Title   string `json:"title"`
	Page    string `json:"page"`
	Icon    string `json:"icon,omitempty"`
	Section string `json:"section,omitempty"`
	Order   int    `json:"order,omitempty"`
}

type Page struct {
	Key    string      `json:"key"`
	Title  string      `json:"title,omitempty"`
	Path   string      `json:"path"`
	Embed  bool        `json:"embed"`
	Render *PageRender `json:"render,omitempty"`
}

type PageRender struct {
	Type    string              `json:"type"`
	SDK     string              `json:"sdk"`
	Targets NativeRenderTargets `json:"targets"`
}

type NativeRenderTargets struct {
	Default NativeRenderTarget `json:"default"`
	Classic NativeRenderTarget `json:"classic"`
}

type NativeRenderTarget struct {
	Entry  string   `json:"entry"`
	Styles []string `json:"styles,omitempty"`
}

type PermissionConfig struct {
	Roles        []string `json:"roles,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type Module struct {
	Manifest
	Enabled       bool   `json:"enabled"`
	AssetRevision string `json:"asset_revision,omitempty"`
	Path          string `json:"path,omitempty"`
	Error         string `json:"error,omitempty"`
}

type PublicModule struct {
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	Version       string                   `json:"version"`
	Description   string                   `json:"description,omitempty"`
	Author        string                   `json:"author,omitempty"`
	Host          HostCompat               `json:"host,omitempty"`
	Runtime       PublicRuntime            `json:"runtime,omitempty"`
	UI            UIContribution           `json:"ui,omitempty"`
	Permissions   PermissionConfig         `json:"permissions,omitempty"`
	Notifications NotificationContribution `json:"notifications,omitempty"`
	Enabled       bool                     `json:"enabled"`
	AssetRevision string                   `json:"asset_revision,omitempty"`
	Error         string                   `json:"error,omitempty"`
}

type PublicRuntime struct {
	Type       string `json:"type"`
	HealthPath string `json:"health_path,omitempty"`
	StaticDir  string `json:"static_dir,omitempty"`
}

func parseHostVersion(raw string) (hostVersion, error) {
	raw = strings.TrimSpace(raw)
	matches := hostVersionPattern.FindStringSubmatch(raw)
	if len(matches) == 0 {
		return hostVersion{}, errors.New("host version must use vMAJOR.MINOR.PATCH[-prerelease] syntax")
	}
	var version hostVersion
	for index := range version.core {
		value, err := strconv.ParseUint(matches[index+1], 10, 64)
		if err != nil {
			return hostVersion{}, errors.New("host version component is out of range")
		}
		version.core[index] = value
	}
	if matches[4] != "" {
		version.prerelease = strings.Split(matches[4], ".")
		for _, part := range version.prerelease {
			if _, err := strconv.ParseUint(part, 10, 64); err == nil && len(part) > 1 && part[0] == '0' {
				return hostVersion{}, errors.New("host version prerelease numbers must not have leading zeroes")
			}
		}
	}
	return version, nil
}

func compareHostVersions(left, right hostVersion) int {
	for index := range left.core {
		if left.core[index] < right.core[index] {
			return -1
		}
		if left.core[index] > right.core[index] {
			return 1
		}
	}
	if len(left.prerelease) == 0 && len(right.prerelease) != 0 {
		return 1
	}
	if len(left.prerelease) != 0 && len(right.prerelease) == 0 {
		return -1
	}
	for index := 0; index < len(left.prerelease) && index < len(right.prerelease); index++ {
		leftPart, rightPart := left.prerelease[index], right.prerelease[index]
		leftNumber, leftErr := strconv.ParseUint(leftPart, 10, 64)
		rightNumber, rightErr := strconv.ParseUint(rightPart, 10, 64)
		switch {
		case leftErr == nil && rightErr == nil:
			if leftNumber < rightNumber {
				return -1
			}
			if leftNumber > rightNumber {
				return 1
			}
		case leftErr == nil && rightErr != nil:
			return -1
		case leftErr != nil && rightErr == nil:
			return 1
		case leftPart < rightPart:
			return -1
		case leftPart > rightPart:
			return 1
		}
	}
	if len(left.prerelease) < len(right.prerelease) {
		return -1
	}
	if len(left.prerelease) > len(right.prerelease) {
		return 1
	}
	return 0
}

func hostCompatibilityError(manifest Manifest) error {
	minText := strings.TrimSpace(manifest.Host.Min)
	maxText := strings.TrimSpace(manifest.Host.Max)
	if minText == "" && maxText == "" {
		return nil
	}
	current, err := parseHostVersion(common.Version)
	if err != nil {
		return errors.New("module host compatibility cannot be determined")
	}
	if minText != "" {
		minimum, parseErr := parseHostVersion(minText)
		if parseErr != nil || compareHostVersions(current, minimum) < 0 {
			return errors.New("module is incompatible with current host version")
		}
	}
	if maxText != "" {
		maximum, parseErr := parseHostVersion(maxText)
		if parseErr != nil || compareHostVersions(current, maximum) > 0 {
			return errors.New("module is incompatible with current host version")
		}
	}
	return nil
}

func (m *Manifest) Validate() error {
	m.ID = strings.TrimSpace(m.ID)
	m.Name = strings.TrimSpace(m.Name)
	m.Version = strings.TrimSpace(m.Version)
	m.Description = strings.TrimSpace(m.Description)
	m.Author = strings.TrimSpace(m.Author)
	m.Host.Min = strings.TrimSpace(m.Host.Min)
	m.Host.Max = strings.TrimSpace(m.Host.Max)
	if m.Host.Min != "" {
		if _, err := parseHostVersion(m.Host.Min); err != nil {
			return fmt.Errorf("host.min is invalid: %w", err)
		}
	}
	if m.Host.Max != "" {
		if _, err := parseHostVersion(m.Host.Max); err != nil {
			return fmt.Errorf("host.max is invalid: %w", err)
		}
	}
	if m.Host.Min != "" && m.Host.Max != "" {
		minimum, _ := parseHostVersion(m.Host.Min)
		maximum, _ := parseHostVersion(m.Host.Max)
		if compareHostVersions(minimum, maximum) > 0 {
			return errors.New("host.min must not exceed host.max")
		}
	}
	if !notificationModuleIDPattern.MatchString(m.ID) {
		return errors.New("module id must use lowercase letters, numbers, - or _")
	}
	if m.Name == "" {
		return errors.New("module name is required")
	}
	if m.Version == "" {
		return errors.New("module version is required")
	}
	m.Runtime.Type = strings.TrimSpace(m.Runtime.Type)
	m.Runtime.BaseURL = strings.TrimSpace(m.Runtime.BaseURL)
	m.Runtime.IdentitySecret = strings.TrimSpace(m.Runtime.IdentitySecret)
	if m.Runtime.Type == "" {
		m.Runtime.Type = RuntimeTypeHTTP
	}

	switch m.Runtime.Type {
	case RuntimeTypeHTTP:
		parsed, err := url.Parse(strings.TrimSpace(m.Runtime.BaseURL))
		if err != nil || parsed == nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return errors.New("runtime.base_url is invalid")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return errors.New("runtime.base_url only supports http or https")
		}
	case RuntimeTypeStatic:
		staticDir := strings.TrimSpace(m.Runtime.StaticDir)
		if staticDir == "" {
			staticDir = DefaultStaticDir
		}
		staticDir = filepath.Clean(filepath.FromSlash(staticDir))
		if filepath.IsAbs(staticDir) || staticDir == "." || staticDir == ".." || strings.HasPrefix(staticDir, ".."+string(filepath.Separator)) || hasDotPathSegment(filepath.ToSlash(staticDir)) {
			return errors.New("runtime.static_dir is invalid")
		}
		m.Runtime.StaticDir = filepath.ToSlash(staticDir)
	default:
		return errors.New("only http and static runtimes are supported")
	}
	if m.Runtime.HealthPath != "" {
		m.Runtime.HealthPath = strings.TrimSpace(m.Runtime.HealthPath)
		if err := validatePagePath(m.Runtime.HealthPath); err != nil {
			return fmt.Errorf("runtime.health_path is invalid: %w", err)
		}
	}
	if err := m.validateRoles(); err != nil {
		return err
	}
	pageKeys := make(map[string]struct{}, len(m.UI.Pages))
	for _, page := range m.UI.Pages {
		if !isValidPageKey(page.Key) {
			return errors.New("ui.pages.key must contain only letters, numbers, - or _")
		}
		if _, exists := pageKeys[page.Key]; exists {
			return errors.New("ui.pages.key must be unique")
		}
		pageKeys[page.Key] = struct{}{}
		if err := validatePagePath(page.Path); err != nil {
			return fmt.Errorf("ui.pages[%s].path is invalid: %w", page.Key, err)
		}
		if m.Runtime.Type == RuntimeTypeStatic && hasDotPathSegment(page.Path) {
			return fmt.Errorf("ui.pages[%s].path is invalid: static paths must not contain dot-prefixed segments", page.Key)
		}
		if err := m.validatePageRender(page); err != nil {
			return err
		}
	}
	for _, nav := range m.UI.Nav {
		if strings.TrimSpace(nav.Title) == "" {
			return errors.New("ui.nav.title is required")
		}
		if strings.TrimSpace(nav.Page) == "" {
			return errors.New("ui.nav.page is required")
		}
		if _, exists := pageKeys[nav.Page]; !exists {
			return errors.New("ui.nav.page must reference ui.pages.key")
		}
	}
	if err := m.validateContributions(); err != nil {
		return err
	}
	return nil
}

func isValidPageKey(value string) bool {
	if value == "" || value != strings.TrimSpace(value) {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}

func validatePagePath(value string) error {
	if value == "" || value != strings.TrimSpace(value) || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return errors.New("path must be an absolute module path")
	}
	if strings.ContainsAny(value, `\?#%`) {
		return errors.New("path must not contain backslashes, percent encoding, query or fragment components")
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return errors.New("path must not contain control characters")
		}
	}
	if path.Clean(value) != value {
		return errors.New("path must be canonical and must not contain dot segments, duplicate separators or a trailing separator")
	}
	if value == "/" {
		return nil
	}
	for _, segment := range strings.Split(strings.TrimPrefix(value, "/"), "/") {
		if segment == "" || segment == "." || segment == ".." {
			return errors.New("path must not contain empty or dot segments")
		}
	}
	return nil
}

func (m *Manifest) validatePageRender(page Page) error {
	if page.Render == nil {
		return nil
	}
	render := page.Render
	if render.Type != RenderTypeNative {
		return fmt.Errorf("ui.pages[%s].render.type only supports native", page.Key)
	}
	if m.Runtime.Type != RuntimeTypeStatic {
		return fmt.Errorf("ui.pages[%s].render native requires static runtime", page.Key)
	}
	if render.SDK != NativeSDKV1 {
		return fmt.Errorf("ui.pages[%s].render.sdk must be v1", page.Key)
	}
	if !hasPermissionCapability(m.Permissions.Capabilities, CapabilityUINative) {
		return fmt.Errorf("ui.pages[%s].render native requires ui.native capability", page.Key)
	}
	if err := validateNativeRenderTarget(page.Key, "default", render.Targets.Default); err != nil {
		return err
	}
	return validateNativeRenderTarget(page.Key, "classic", render.Targets.Classic)
}

func validateNativeRenderTarget(pageKey, targetName string, target NativeRenderTarget) error {
	if err := validateNativeAssetPath(target.Entry, ".js", ".mjs"); err != nil {
		return fmt.Errorf("ui.pages[%s].render.targets.%s.entry is invalid: %w", pageKey, targetName, err)
	}
	for index, style := range target.Styles {
		if err := validateNativeAssetPath(style, ".css"); err != nil {
			return fmt.Errorf("ui.pages[%s].render.targets.%s.styles[%d] is invalid: %w", pageKey, targetName, index, err)
		}
	}
	return nil
}

func validateNativeAssetPath(value string, extensions ...string) error {
	if value == "" || value != strings.TrimSpace(value) || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return errors.New("path must be an absolute module path")
	}
	if strings.ContainsAny(value, "\\:?#\x00") || path.Clean(value) != value {
		return errors.New("path must not contain traversal, query or fragment components")
	}
	for _, segment := range strings.Split(strings.TrimPrefix(value, "/"), "/") {
		if segment == "" || segment == "." || segment == ".." {
			return errors.New("path must not contain traversal components")
		}
	}
	extension := path.Ext(value)
	for _, allowed := range extensions {
		if extension == allowed {
			return nil
		}
	}
	return fmt.Errorf("path extension must be %s", strings.Join(extensions, " or "))
}

func (m *Manifest) validateRoles() error {
	seen := make(map[string]struct{}, len(m.Permissions.Roles))
	for index, role := range m.Permissions.Roles {
		role = strings.ToLower(strings.TrimSpace(role))
		switch role {
		case "guest", "user", "common", "admin", "root", "super_admin":
		default:
			return fmt.Errorf("unsupported permission role: %s", role)
		}
		if _, exists := seen[role]; exists {
			return fmt.Errorf("duplicate permission role: %s", role)
		}
		seen[role] = struct{}{}
		m.Permissions.Roles[index] = role
	}
	return nil
}

func (m *Manifest) validateContributions() error {
	seen := make(map[string]struct{}, len(m.Permissions.Capabilities))
	notificationCapabilities := make([]string, 0, 1)
	for index, capability := range m.Permissions.Capabilities {
		capability = strings.TrimSpace(capability)
		if capability == "" {
			return errors.New("permissions.capabilities cannot contain an empty value")
		}
		if _, exists := seen[capability]; exists {
			return fmt.Errorf("duplicate permission capability: %s", capability)
		}
		seen[capability] = struct{}{}
		m.Permissions.Capabilities[index] = capability
		switch capability {
		case CapabilityUINative:
		case CapabilityNotificationEventsPublish:
			notificationCapabilities = append(notificationCapabilities, capability)
		default:
			return fmt.Errorf("unsupported permission capability: %s", capability)
		}
	}

	capabilities := m.Permissions.Capabilities
	m.Permissions.Capabilities = notificationCapabilities
	err := m.validateNotificationContribution()
	m.Permissions.Capabilities = capabilities
	return err
}

func hasPermissionCapability(capabilities []string, expected string) bool {
	for _, capability := range capabilities {
		if strings.TrimSpace(capability) == expected {
			return true
		}
	}
	return false
}

func (m Module) Public(includeAdminFields bool) PublicModule {
	result := PublicModule{
		ID:          m.ID,
		Name:        m.Name,
		Version:     m.Version,
		Description: m.Description,
		Author:      m.Author,
		Host:        m.Host,
		Runtime: PublicRuntime{
			Type:       m.Runtime.Type,
			HealthPath: m.Runtime.HealthPath,
			StaticDir:  m.Runtime.StaticDir,
		},
		UI:            m.UI,
		Permissions:   m.Permissions,
		Notifications: m.Notifications,
		Enabled:       m.Enabled,
		AssetRevision: m.AssetRevision,
	}
	if includeAdminFields {
		result.Error = m.Error
	}
	return result
}
