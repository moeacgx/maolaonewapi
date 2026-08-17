package extension

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

const (
	MaxInstallArchiveBytes         int64 = 100 << 20
	maxInstallArchiveExpandedBytes int64 = 200 << 20
	maxInstallArchiveEntries             = 4096
)

type stateFile struct {
	Modules map[string]moduleState `json:"modules"`
}

type moduleState struct {
	Enabled bool `json:"enabled"`
}

type Manager struct {
	rootDir string

	operationMu sync.Mutex
	mu          sync.RWMutex
	modules     map[string]Module
	state       stateFile
}

var DefaultManager = NewManager(resolveRootDir())

func NewManager(rootDir string) *Manager {
	if strings.TrimSpace(rootDir) == "" {
		rootDir = DefaultRootDir
	}
	return &Manager{
		rootDir: filepath.Clean(rootDir),
		modules: map[string]Module{},
		state: stateFile{
			Modules: map[string]moduleState{},
		},
	}
}

func Init() error {
	manager := NewManager(resolveRootDir())
	if err := installBuiltinModules(manager.rootDir); err != nil {
		return err
	}
	if err := manager.Scan(); err != nil {
		return err
	}
	DefaultManager = manager
	return nil
}

func resolveRootDir() string {
	if value := strings.TrimSpace(os.Getenv("EXTENSIONS_ROOT")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("MODULES_ROOT")); value != "" {
		return value
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		legacyDir := filepath.Join("/data", DefaultRootDir)
		if hasInstalledModules(legacyDir) {
			return legacyDir
		}
		return "/data/modules"
	}
	return DefaultRootDir
}

func hasInstalledModules(rootDir string) bool {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			continue
		}
		if regularFileExists(filepath.Join(rootDir, entry.Name(), "manifest.json")) {
			return true
		}
	}
	return false
}

func (m *Manager) RootDir() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.rootDir
}

func (m *Manager) Scan() error {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	return m.scanLocked()
}

func (m *Manager) scanLocked() error {
	root, err := canonicalDirectory(m.rootDir, true)
	if err != nil {
		return errors.New("extension root is unavailable")
	}
	state, err := m.loadStateFrom(root)
	if err != nil {
		return errors.New("extension state is unavailable")
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return errors.New("extension root is unavailable")
	}
	nextModules := make(map[string]Module)
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			continue
		}
		directoryID := strings.TrimSpace(entry.Name())
		if _, err := safeModuleTargetDir(root, directoryID); err != nil {
			continue
		}
		moduleDir := filepath.Join(root, entry.Name())
		if err := rejectSymlinkComponents(root, moduleDir); err != nil {
			nextModules[directoryID] = invalidModule(directoryID, moduleDir)
			continue
		}
		manifestPath := filepath.Join(moduleDir, "manifest.json")
		manifestInfo, statErr := os.Lstat(manifestPath)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				continue
			}
			nextModules[directoryID] = invalidModule(directoryID, moduleDir)
			continue
		}
		if !manifestInfo.Mode().IsRegular() {
			nextModules[directoryID] = invalidModule(directoryID, moduleDir)
			continue
		}
		manifestBytes, readErr := os.ReadFile(manifestPath)
		if readErr != nil {
			nextModules[directoryID] = invalidModule(directoryID, moduleDir)
			continue
		}
		var manifest Manifest
		if err := common.Unmarshal(manifestBytes, &manifest); err != nil {
			nextModules[directoryID] = invalidModule(directoryID, moduleDir)
			continue
		}
		manifest.ID = strings.TrimSpace(manifest.ID)
		manifest.Name = strings.TrimSpace(manifest.Name)
		manifest.Version = strings.TrimSpace(manifest.Version)
		if manifest.ID != directoryID {
			nextModules[directoryID] = invalidModule(directoryID, moduleDir)
			continue
		}
		if _, duplicate := nextModules[manifest.ID]; duplicate {
			nextModules[directoryID] = invalidModule(directoryID, moduleDir)
			continue
		}
		if err := manifest.Validate(); err != nil {
			nextModules[directoryID] = invalidModule(directoryID, moduleDir)
			continue
		}
		if err := hostCompatibilityError(manifest); err != nil {
			nextModules[directoryID] = Module{
				Manifest: manifest,
				Error:    "module is incompatible with current host version",
			}
			continue
		}
		assetRevision, err := nativeAssetRevision(moduleDir, manifest)
		if err != nil {
			nextModules[directoryID] = invalidModule(directoryID, moduleDir)
			continue
		}
		enabled := false
		if saved, ok := state.Modules[manifest.ID]; ok {
			enabled = saved.Enabled
		}
		nextModules[manifest.ID] = Module{
			Manifest:      manifest,
			Enabled:       enabled,
			AssetRevision: assetRevision,
			Path:          moduleDir,
		}
	}

	m.mu.Lock()
	m.rootDir = root
	m.modules = nextModules
	m.state = state
	m.mu.Unlock()
	return nil
}

func invalidModule(id, _ string) Module {
	return Module{
		Manifest: Manifest{ID: id, Name: id},
		Error:    "module manifest is invalid",
	}
}

func (m *Manager) List(role int, includeDisabled bool) []PublicModule {
	if !common.IsValidateRole(role) {
		return nil
	}
	if includeDisabled && role != common.RoleRootUser {
		includeDisabled = false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]PublicModule, 0, len(m.modules))
	for _, module := range m.modules {
		if module.Error == "" {
			if err := hostCompatibilityError(module.Manifest); err != nil {
				module.Enabled = false
				module.Error = "module is incompatible with current host version"
			}
		}
		if !includeDisabled && (!module.Enabled || module.Error != "") {
			continue
		}
		if !includeDisabled && !roleAllowed(role, module.Permissions.Roles) {
			continue
		}
		result = append(result, module.Public(includeDisabled))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (m *Manager) Get(id string) (Module, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	module, ok := m.modules[strings.TrimSpace(id)]
	return module, ok
}

func (m *Manager) SetEnabled(id string, enabled bool) (Module, error) {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()

	id = strings.TrimSpace(id)
	m.mu.Lock()
	defer m.mu.Unlock()
	module, ok := m.modules[id]
	if !ok {
		return Module{}, errors.New("module not found")
	}
	if module.Error != "" {
		if module.Error == "module is incompatible with current host version" {
			return Module{}, errors.New("module is incompatible with current host version")
		}
		return Module{}, errors.New("module manifest is invalid")
	}
	if enabled {
		if err := hostCompatibilityError(module.Manifest); err != nil {
			return Module{}, errors.New("module is incompatible with current host version")
		}
	}
	previousModule := module
	previousState, hadState := m.state.Modules[id]
	module.Enabled = enabled
	m.modules[id] = module
	if m.state.Modules == nil {
		m.state.Modules = map[string]moduleState{}
	}
	m.state.Modules[id] = moduleState{Enabled: enabled}
	if err := m.saveStateLocked(); err != nil {
		m.modules[id] = previousModule
		if hadState {
			m.state.Modules[id] = previousState
		} else {
			delete(m.state.Modules, id)
		}
		return Module{}, errors.New("extension state could not be saved")
	}
	return module, nil
}

func (m *Manager) Uninstall(id string) error {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()

	id = strings.TrimSpace(id)
	m.mu.Lock()
	defer m.mu.Unlock()
	module, ok := m.modules[id]
	if !ok {
		return errors.New("module not found")
	}
	targetDir, err := safeModuleTargetDir(m.rootDir, id)
	if err != nil {
		return errors.New("module id is invalid")
	}
	if module.Path != "" && !sameCleanPath(module.Path, targetDir) {
		return errors.New("module path does not match module id")
	}
	quarantine := targetDir + ".uninstall"
	_ = os.RemoveAll(quarantine)
	if err := os.Rename(targetDir, quarantine); err != nil {
		return errors.New("module could not be removed")
	}
	previousState, hadState := m.state.Modules[id]
	delete(m.modules, id)
	delete(m.state.Modules, id)
	if err := m.saveStateLocked(); err != nil {
		m.modules[id] = module
		if hadState {
			m.state.Modules[id] = previousState
		}
		_ = os.Rename(quarantine, targetDir)
		return errors.New("extension state could not be saved")
	}
	_ = os.RemoveAll(quarantine)
	return nil
}

func (m *Manager) InstallArchive(readerAt io.ReaderAt, size int64) (Module, error) {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()

	if readerAt == nil {
		return Module{}, errors.New("module archive is required")
	}
	if size <= 0 {
		return Module{}, errors.New("module archive is empty")
	}
	if size > MaxInstallArchiveBytes {
		return Module{}, fmt.Errorf("module archive exceeds %d MiB", MaxInstallArchiveBytes>>20)
	}
	archive, err := zip.NewReader(readerAt, size)
	if err != nil {
		return Module{}, errors.New("invalid module archive")
	}
	if err := validateArchiveMetadata(archive.File); err != nil {
		return Module{}, err
	}

	root, err := canonicalDirectory(m.rootDir, true)
	if err != nil {
		return Module{}, errors.New("extension root is unavailable")
	}
	tmpDir, err := os.MkdirTemp(root, ".install-*")
	if err != nil {
		return Module{}, errors.New("module staging directory is unavailable")
	}
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.RemoveAll(tmpDir)
		}
	}()
	if err := extractArchive(archive, tmpDir); err != nil {
		return Module{}, err
	}
	sourceDir, err := findArchiveManifestDir(tmpDir)
	if err != nil {
		return Module{}, err
	}
	manifest, err := readArchiveManifest(sourceDir)
	if err != nil {
		return Module{}, errors.New("module manifest is invalid")
	}
	if err := manifest.Validate(); err != nil {
		return Module{}, errors.New("module manifest is invalid")
	}
	if err := hostCompatibilityError(manifest); err != nil {
		return Module{}, errors.New("module is incompatible with current host version")
	}
	if _, err := nativeAssetRevision(sourceDir, manifest); err != nil {
		return Module{}, errors.New("native module resources are invalid")
	}
	targetDir, err := safeModuleTargetDir(root, manifest.ID)
	if err != nil {
		return Module{}, errors.New("module id is invalid")
	}

	backupDir := targetDir + ".install-backup"
	_ = os.RemoveAll(backupDir)
	hadTarget := false
	if _, err := os.Lstat(targetDir); err == nil {
		hadTarget = true
		if err := os.Rename(targetDir, backupDir); err != nil {
			return Module{}, errors.New("existing module could not be replaced")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Module{}, errors.New("module target is unavailable")
	}
	rollback := func() {
		_ = os.RemoveAll(targetDir)
		if hadTarget {
			_ = os.Rename(backupDir, targetDir)
		}
	}
	if err := os.Rename(sourceDir, targetDir); err != nil {
		rollback()
		return Module{}, errors.New("module could not be installed")
	}
	if sameCleanPath(sourceDir, tmpDir) {
		cleanupTemp = false
	}
	if err := m.scanLocked(); err != nil {
		rollback()
		_ = m.scanLocked()
		return Module{}, errors.New("installed module could not be loaded")
	}
	module, ok := m.Get(manifest.ID)
	if !ok || module.Error != "" {
		rollback()
		_ = m.scanLocked()
		return Module{}, errors.New("installed module manifest is invalid")
	}
	if hadTarget {
		_ = os.RemoveAll(backupDir)
	}
	return module, nil
}

func (m *Manager) loadStateFrom(root string) (stateFile, error) {
	state := stateFile{Modules: map[string]moduleState{}}
	statePath := filepath.Join(root, "state.json")
	info, err := os.Lstat(statePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return state, nil
		}
		return stateFile{}, err
	}
	if !info.Mode().IsRegular() {
		return stateFile{}, errors.New("extension state must be a regular file")
	}
	data, err := os.ReadFile(statePath)
	if err != nil {
		return stateFile{}, err
	}
	if err := common.Unmarshal(data, &state); err != nil {
		return stateFile{}, err
	}
	if state.Modules == nil {
		state.Modules = map[string]moduleState{}
	}
	return state, nil
}

func (m *Manager) saveStateLocked() error {
	if err := os.MkdirAll(m.rootDir, 0o755); err != nil {
		return err
	}
	data, err := common.Marshal(m.state)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(m.rootDir, ".state-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, m.statePath()); err != nil {
		return err
	}
	if directory, err := os.Open(m.rootDir); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}

func validateArchiveMetadata(files []*zip.File) error {
	if len(files) == 0 || len(files) > maxInstallArchiveEntries {
		return fmt.Errorf("module archive supports at most %d entries", maxInstallArchiveEntries)
	}
	var declaredExpanded uint64
	for _, file := range files {
		if file == nil || file.UncompressedSize64 > uint64(maxInstallArchiveExpandedBytes) || declaredExpanded > uint64(maxInstallArchiveExpandedBytes)-file.UncompressedSize64 {
			return fmt.Errorf("module archive extracted content exceeds %d MiB", maxInstallArchiveExpandedBytes>>20)
		}
		declaredExpanded += file.UncompressedSize64
	}
	return nil
}

func (m *Manager) statePath() string { return filepath.Join(m.rootDir, "state.json") }

func extractArchive(archive *zip.Reader, targetDir string) error {
	targetAbs, err := filepath.Abs(targetDir)
	if err != nil {
		return errors.New("module staging directory is unavailable")
	}
	seen := make(map[string]struct{}, len(archive.File))
	var extracted int64
	for _, file := range archive.File {
		relativePath, err := cleanArchivePath(file.Name)
		if err != nil {
			return err
		}
		collisionKey := filepath.ToSlash(relativePath)
		if runtime.GOOS == "windows" {
			collisionKey = strings.ToLower(collisionKey)
		}
		if _, duplicate := seen[collisionKey]; duplicate {
			return fmt.Errorf("archive contains duplicate path: %s", file.Name)
		}
		seen[collisionKey] = struct{}{}
		targetPath := filepath.Join(targetAbs, relativePath)
		if err := ensurePathInside(targetAbs, targetPath); err != nil {
			return errors.New("archive contains unsafe path")
		}
		info := file.FileInfo()
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("archive contains unsupported file: %s", file.Name)
		}
		if info.IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return errors.New("archive directory could not be created")
			}
			if err := rejectSymlinkComponents(targetAbs, targetPath); err != nil {
				return errors.New("archive contains symbolic link path")
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return errors.New("archive directory could not be created")
		}
		if err := rejectSymlinkComponents(targetAbs, filepath.Dir(targetPath)); err != nil {
			return errors.New("archive contains symbolic link path")
		}
		source, err := file.Open()
		if err != nil {
			return errors.New("archive entry could not be opened")
		}
		target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			_ = source.Close()
			return errors.New("archive entry could not be created")
		}
		remaining := maxInstallArchiveExpandedBytes - extracted + 1
		written, copyErr := io.Copy(target, io.LimitReader(source, remaining))
		sourceErr := source.Close()
		targetErr := target.Close()
		extracted += written
		if copyErr != nil || sourceErr != nil || targetErr != nil {
			return errors.New("archive entry could not be extracted")
		}
		if extracted > maxInstallArchiveExpandedBytes {
			return fmt.Errorf("module archive extracted content exceeds %d MiB", maxInstallArchiveExpandedBytes>>20)
		}
	}
	return nil
}

func cleanArchivePath(name string) (string, error) {
	normalized := strings.ReplaceAll(name, "\\", "/")
	if normalized == "" || normalized != strings.TrimSpace(normalized) || strings.HasPrefix(normalized, "/") || strings.Contains(normalized, ":") || strings.ContainsRune(normalized, '\x00') {
		return "", errors.New("archive contains unsafe path")
	}
	for _, character := range normalized {
		if character < 0x20 || character == 0x7f {
			return "", errors.New("archive contains unsafe path")
		}
	}
	cleaned := filepath.Clean(filepath.FromSlash(normalized))
	if filepath.IsAbs(cleaned) || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return "", errors.New("archive contains unsafe path")
	}
	if filepath.ToSlash(cleaned) != strings.TrimSuffix(normalized, "/") {
		return "", errors.New("archive contains non-canonical path")
	}
	return cleaned, nil
}

func findArchiveManifestDir(root string) (string, error) {
	candidates := make([]string, 0, 2)
	if regularFileExists(filepath.Join(root, "manifest.json")) {
		candidates = append(candidates, root)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", errors.New("module archive is unavailable")
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			continue
		}
		moduleDir := filepath.Join(root, entry.Name())
		if regularFileExists(filepath.Join(moduleDir, "manifest.json")) {
			candidates = append(candidates, moduleDir)
		}
	}
	switch len(candidates) {
	case 0:
		return "", errors.New("module archive must contain manifest.json")
	case 1:
		return candidates[0], nil
	default:
		return "", errors.New("module archive contains multiple manifest.json files")
	}
}

func readArchiveManifest(moduleDir string) (Manifest, error) {
	manifestPath := filepath.Join(moduleDir, "manifest.json")
	info, err := os.Lstat(manifestPath)
	if err != nil || !info.Mode().IsRegular() {
		return Manifest{}, errors.New("module manifest is unavailable")
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := common.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, err
	}
	manifest.ID = strings.TrimSpace(manifest.ID)
	manifest.Name = strings.TrimSpace(manifest.Name)
	manifest.Version = strings.TrimSpace(manifest.Version)
	return manifest, nil
}

func safeModuleTargetDir(rootDir, id string) (string, error) {
	id = strings.TrimSpace(id)
	if !notificationModuleIDPattern.MatchString(id) || id == "." || id == ".." || strings.ContainsAny(id, `/\\`) || filepath.Clean(id) != id {
		return "", errors.New("module id is not a safe directory name")
	}
	rootAbs, err := filepath.Abs(rootDir)
	if err != nil {
		return "", err
	}
	targetDir := filepath.Join(rootAbs, id)
	if err := ensurePathInside(rootAbs, targetDir); err != nil {
		return "", err
	}
	return targetDir, nil
}

func canonicalDirectory(path string, create bool) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if create {
		if err := os.MkdirAll(absolute, 0o755); err != nil {
			return "", err
		}
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.IsDir() {
		return "", errors.New("directory is unavailable")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func regularFileExists(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func ensurePathInside(root, target string) error {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return errors.New("path escapes root")
	}
	return nil
}

func sameCleanPath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return filepath.Clean(left) == filepath.Clean(right)
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(leftAbs), filepath.Clean(rightAbs))
	}
	return filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}

func roleAllowed(role int, allowed []string) bool {
	if !common.IsValidateRole(role) {
		return false
	}
	if len(allowed) == 0 {
		return role >= common.RoleCommonUser
	}
	for _, value := range allowed {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "guest":
			return true
		case "user", "common":
			if role >= common.RoleCommonUser {
				return true
			}
		case "admin":
			if role >= common.RoleAdminUser {
				return true
			}
		case "root", "super_admin":
			if role >= common.RoleRootUser {
				return true
			}
		}
	}
	return false
}
