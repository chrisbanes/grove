package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	GroveDirName  = ".grove"
	ConfigFile    = "config.json"
	BackendFile   = "backend.json"
	WorkspaceFile = "workspace.json"
	HooksDir      = "hooks"
	runtimeIDFile = ".runtime-id"
)

const groveGitignoreContents = `# Grove local metadata (safe to ignore)
workspace.json
backend.json
.runtime-id
`

type Config struct {
	WarmupCommand string   `json:"warmup_command,omitempty"`
	WorkspaceDir  string   `json:"workspace_dir"`
	StateDir      string   `json:"state_dir"`
	MaxWorkspaces int      `json:"max_workspaces"`
	Exclude       []string `json:"exclude,omitempty"`
	CloneBackend  string   `json:"clone_backend,omitempty"`
}

func DefaultConfig(projectName string) *Config {
	return &Config{
		WorkspaceDir:  "~/grove-workspaces/{project}",
		StateDir:      "~/.grove",
		MaxWorkspaces: 10,
	}
}

func Load(repoRoot string) (*Config, error) {
	path := filepath.Join(repoRoot, GroveDirName, ConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("grove not initialized: %w", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	if _, hasLegacyRuntimeID := raw["runtime_id"]; hasLegacyRuntimeID {
		return nil, fmt.Errorf("runtime_id in %s is no longer supported; remove it and rerun", path)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	for _, pattern := range cfg.Exclude {
		if _, err := filepath.Match(pattern, ""); err != nil {
			return nil, fmt.Errorf("invalid exclude pattern %q: %w", pattern, err)
		}
	}
	if cfg.MaxWorkspaces == 0 {
		cfg.MaxWorkspaces = 10
	}
	if cfg.StateDir == "" {
		cfg.StateDir = "~/.grove"
	}
	backend, err := normalizeCloneBackend(cfg.CloneBackend)
	if err != nil {
		return nil, err
	}
	cfg.CloneBackend = backend
	return &cfg, nil
}

// LoadOrDefault loads config from .grove/config.json if it exists,
// otherwise returns default config. This enables config-free mode
// where grove works without explicit initialization.
func LoadOrDefault(repoRoot string) (*Config, error) {
	path := filepath.Join(repoRoot, GroveDirName, ConfigFile)
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		projectName := filepath.Base(repoRoot)
		cfg := DefaultConfig(projectName)
		cfg.CloneBackend = "cp"
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	return Load(repoRoot)
}

func normalizeCloneBackend(value string) (string, error) {
	if value == "" {
		return "cp", nil
	}
	switch value {
	case "cp", "image":
		return value, nil
	default:
		return "", fmt.Errorf("invalid clone_backend %q: expected cp or image", value)
	}
}

func Save(repoRoot string, cfg *Config) error {
	groveDir := filepath.Join(repoRoot, GroveDirName)
	if err := os.MkdirAll(groveDir, 0755); err != nil {
		return err
	}
	type persistedConfig struct {
		WarmupCommand string   `json:"warmup_command,omitempty"`
		WorkspaceDir  string   `json:"workspace_dir"`
		StateDir      string   `json:"state_dir,omitempty"`
		MaxWorkspaces int      `json:"max_workspaces"`
		Exclude       []string `json:"exclude,omitempty"`
		CloneBackend  string   `json:"clone_backend,omitempty"`
	}
	data, err := json.MarshalIndent(&persistedConfig{
		WarmupCommand: cfg.WarmupCommand,
		WorkspaceDir:  cfg.WorkspaceDir,
		StateDir:      cfg.StateDir,
		MaxWorkspaces: cfg.MaxWorkspaces,
		Exclude:       cfg.Exclude,
		CloneBackend:  cfg.CloneBackend,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(groveDir, ConfigFile), data, 0644)
}

func EnsureGroveGitignore(repoRoot string) error {
	path := filepath.Join(repoRoot, GroveDirName, ".gitignore")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.WriteFile(path, []byte(groveGitignoreContents), 0644)
}

// EnsureMinimalGroveDir creates a .grove/ directory with just a .gitignore.
// This is the lazy-init path: no config.json, no hooks dir. Used by commands
// like create that need to write runtime files (.runtime-id, workspace.json).
func EnsureMinimalGroveDir(repoRoot string) error {
	groveDir := filepath.Join(repoRoot, GroveDirName)
	if err := os.MkdirAll(groveDir, 0755); err != nil {
		return err
	}
	return EnsureGroveGitignore(repoRoot)
}

func ExpandWorkspaceDir(tmpl, projectName string) string {
	expanded := strings.ReplaceAll(tmpl, "{project}", projectName)
	if strings.HasPrefix(expanded, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			expanded = filepath.Join(home, expanded[2:])
		}
	}
	return expanded
}

var nonAlnumPattern = regexp.MustCompile(`[^a-z0-9]+`)
var runtimeIDPattern = regexp.MustCompile(`^[a-z0-9]+$`)

func GenerateRuntimeID() (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func LoadRuntimeID(repoRoot string) (string, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, GroveDirName, runtimeIDFile))
	if err != nil {
		return "", err
	}
	runtimeID := strings.TrimSpace(string(data))
	if runtimeID == "" {
		return "", fmt.Errorf("invalid runtime id in %s: file is empty", filepath.Join(repoRoot, GroveDirName, runtimeIDFile))
	}
	if !runtimeIDPattern.MatchString(runtimeID) {
		return "", fmt.Errorf("invalid runtime id %q in %s: expected lowercase letters and numbers", runtimeID, filepath.Join(repoRoot, GroveDirName, runtimeIDFile))
	}
	return runtimeID, nil
}

func saveRuntimeID(repoRoot, runtimeID string) error {
	if !runtimeIDPattern.MatchString(runtimeID) {
		return fmt.Errorf("invalid runtime id %q: expected lowercase letters and numbers", runtimeID)
	}
	groveDir := filepath.Join(repoRoot, GroveDirName)
	if err := os.MkdirAll(groveDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(groveDir, runtimeIDFile), []byte(runtimeID+"\n"), 0644)
}

// ImageRuntimeRoot returns the directory that stores image backend runtime data
// (base image, shadows, mount metadata) for this repository.
func ImageRuntimeRoot(repoRoot string, cfg *Config) (string, error) {
	workspaceDir, err := expandedWorkspaceDirAbs(repoRoot, cfg.WorkspaceDir)
	if err != nil {
		return "", err
	}
	runtimeID, err := LoadRuntimeID(repoRoot)
	switch {
	case err == nil:
		return runtimeRootForID(workspaceDir, runtimeID), nil
	case !errors.Is(err, os.ErrNotExist):
		return "", err
	}
	return legacyImageRuntimeRoot(repoRoot, cfg)
}

// EnsureImageRuntimeRoot ensures the runtime ID file is present and migrates any
// legacy runtime path into the runtime-id-based path.
func EnsureImageRuntimeRoot(repoRoot string, cfg *Config) (string, error) {
	workspaceDir, err := expandedWorkspaceDirAbs(repoRoot, cfg.WorkspaceDir)
	if err != nil {
		return "", err
	}
	runtimeID, err := LoadRuntimeID(repoRoot)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		runtimeID, err = GenerateRuntimeID()
		if err != nil {
			return "", err
		}
		if err := saveRuntimeID(repoRoot, runtimeID); err != nil {
			return "", err
		}
	}
	runtimeRoot := runtimeRootForID(workspaceDir, runtimeID)
	legacyRoot, err := legacyImageRuntimeRoot(repoRoot, cfg)
	if err != nil {
		return "", err
	}
	if runtimeRoot == legacyRoot {
		return runtimeRoot, nil
	}
	legacyInfo, err := os.Stat(legacyRoot)
	switch {
	case err == nil && legacyInfo.IsDir():
		if _, statErr := os.Stat(runtimeRoot); errors.Is(statErr, os.ErrNotExist) {
			if err := os.MkdirAll(filepath.Dir(runtimeRoot), 0755); err != nil {
				return "", err
			}
			if err := os.Rename(legacyRoot, runtimeRoot); err != nil {
				return "", err
			}
		} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
		}
	case errors.Is(err, os.ErrNotExist):
		// No legacy runtime to migrate.
	case err != nil:
		return "", err
	}
	return runtimeRoot, nil
}

func runtimeRootForID(workspaceDir, runtimeID string) string {
	return filepath.Join(workspaceDir, "runtimes", runtimeID)
}

func legacyImageRuntimeRoot(repoRoot string, cfg *Config) (string, error) {
	workspaceDir, err := expandedWorkspaceDirAbs(repoRoot, cfg.WorkspaceDir)
	if err != nil {
		return "", err
	}
	absRepoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", err
	}
	repoName := strings.ToLower(filepath.Base(absRepoRoot))
	repoName = nonAlnumPattern.ReplaceAllString(repoName, "-")
	repoName = strings.Trim(repoName, "-")
	if repoName == "" {
		repoName = "repo"
	}
	sum := sha256.Sum256([]byte(absRepoRoot))
	token := hex.EncodeToString(sum[:6])
	return filepath.Join(workspaceDir, ".grove-runtime", repoName+"-"+token), nil
}

// BuildImageSyncExcludes returns the excludes used for image backend base sync.
// It includes user excludes plus the workspace directory when that directory
// lives inside the golden copy (to avoid recursive workspace ingestion).
func BuildImageSyncExcludes(goldenRoot string, cfg *Config) ([]string, error) {
	excludes := append([]string(nil), cfg.Exclude...)

	workspaceDir, err := expandedWorkspaceDirAbs(goldenRoot, cfg.WorkspaceDir)
	if err != nil {
		return nil, err
	}
	absGoldenRoot, err := filepath.Abs(goldenRoot)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(absGoldenRoot, workspaceDir)
	if err != nil {
		return nil, err
	}

	rel = filepath.Clean(rel)
	if rel == "." {
		return nil, fmt.Errorf("workspace_dir resolves to the repository root; choose a subdirectory or external path")
	}
	if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		excludes = append(excludes, filepath.ToSlash(rel)+"/")
	}
	return excludes, nil
}

func expandedWorkspaceDirAbs(goldenRoot, workspaceDir string) (string, error) {
	expanded := ExpandWorkspaceDir(workspaceDir, filepath.Base(goldenRoot))
	if !filepath.IsAbs(expanded) {
		expanded = filepath.Join(goldenRoot, expanded)
	}
	return filepath.Abs(expanded)
}

func FindGroveRoot(startPath string) (string, error) {
	absPath, err := filepath.Abs(startPath)
	if err != nil {
		return "", err
	}

	// First pass: look for .grove/ directory (existing behavior)
	dir := absPath
	for {
		candidate := filepath.Join(dir, GroveDirName)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Second pass: fall back to git root
	dir = absPath
	for {
		gitDir := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("not a git repository (or any parent): %s", startPath)
}

type backendState struct {
	Backend string `json:"backend"`
}

// EnsureBackendCompatible ensures the configured backend matches the initialized backend.
// If no backend state exists yet, this bootstraps it for cp repos and image repos that already
// have image state. Image mode without state is allowed for lazy bootstrap at create/update time.
func EnsureBackendCompatible(repoRoot string, cfg *Config) error {
	backend, err := LoadBackendState(repoRoot)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		if backend != cfg.CloneBackend {
			return fmt.Errorf("configured clone_backend is %q but initialized backend is %q.\nRun `grove migrate --to %s`", cfg.CloneBackend, backend, cfg.CloneBackend)
		}
		return nil
	}

	runtimeRoot, err := ImageRuntimeRoot(repoRoot, cfg)
	if err != nil {
		return err
	}
	hasImageState := imageStateExists(runtimeRoot)
	legacyRoot, err := legacyImageRuntimeRoot(repoRoot, cfg)
	if err != nil {
		return err
	}
	if legacyRoot != runtimeRoot {
		hasImageState = hasImageState || imageStateExists(legacyRoot)
	}

	switch cfg.CloneBackend {
	case "cp":
		if hasImageState {
			return fmt.Errorf("configured clone_backend is %q but initialized backend appears to be %q.\nRun `grove migrate --to %s`", "cp", "image", "cp")
		}
		return SaveBackendState(repoRoot, "cp")
	case "image":
		if !hasImageState {
			// Allow lazy image backend bootstrap. `grove create` and `grove update`
			// will initialize the base image when state is missing.
			return nil
		}
		return SaveBackendState(repoRoot, "image")
	default:
		return fmt.Errorf("invalid clone_backend %q: expected cp or image", cfg.CloneBackend)
	}
}

func imageStateExists(runtimeRoot string) bool {
	_, err := os.Stat(filepath.Join(runtimeRoot, "images", "state.json"))
	return err == nil
}

func LoadBackendState(repoRoot string) (string, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, GroveDirName, BackendFile))
	if err != nil {
		return "", err
	}
	var st backendState
	if err := json.Unmarshal(data, &st); err != nil {
		return "", err
	}
	backend, err := normalizeCloneBackend(st.Backend)
	if err != nil {
		return "", err
	}
	return backend, nil
}

func SaveBackendState(repoRoot, backend string) error {
	normalized, err := normalizeCloneBackend(backend)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, GroveDirName), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(&backendState{Backend: normalized}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(repoRoot, GroveDirName, BackendFile), data, 0644)
}
