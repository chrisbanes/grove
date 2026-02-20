package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	GroveDirName  = ".grove"
	ConfigFile    = "config.json"
	BackendFile   = "backend.json"
	WorkspaceFile = "workspace.json"
	HooksDir      = "hooks"
)

type Config struct {
	WarmupCommand string   `json:"warmup_command,omitempty"`
	WorkspaceDir  string   `json:"workspace_dir"`
	MaxWorkspaces int      `json:"max_workspaces"`
	Exclude       []string `json:"exclude,omitempty"`
	CloneBackend  string   `json:"clone_backend,omitempty"`
}

func DefaultConfig(projectName string) *Config {
	return &Config{
		WorkspaceDir:  "/tmp/grove/{project}",
		MaxWorkspaces: 10,
	}
}

func Load(repoRoot string) (*Config, error) {
	path := filepath.Join(repoRoot, GroveDirName, ConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("grove not initialized: %w", err)
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
	backend, err := normalizeCloneBackend(cfg.CloneBackend)
	if err != nil {
		return nil, err
	}
	cfg.CloneBackend = backend
	return &cfg, nil
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
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(groveDir, ConfigFile), data, 0644)
}

func ExpandWorkspaceDir(tmpl, projectName string) string {
	return strings.ReplaceAll(tmpl, "{project}", projectName)
}

func FindGroveRoot(startPath string) (string, error) {
	absPath, err := filepath.Abs(startPath)
	if err != nil {
		return "", err
	}
	dir := absPath
	for {
		candidate := filepath.Join(dir, GroveDirName)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("grove not initialized: no .grove/ directory found above %s", startPath)
		}
		dir = parent
	}
}

type backendState struct {
	Backend string `json:"backend"`
}

// EnsureBackendCompatible ensures the configured backend matches the initialized backend.
// If no backend state exists yet, this bootstraps it for cp repos and image repos that already
// have image state. It returns actionable errors for backend mismatches or uninitialized image mode.
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

	imageStatePath := filepath.Join(repoRoot, GroveDirName, "images", "state.json")
	_, imageStateErr := os.Stat(imageStatePath)
	hasImageState := imageStateErr == nil

	switch cfg.CloneBackend {
	case "cp":
		if hasImageState {
			return fmt.Errorf("configured clone_backend is %q but initialized backend appears to be %q.\nRun `grove migrate --to %s`", "cp", "image", "cp")
		}
		return SaveBackendState(repoRoot, "cp")
	case "image":
		if !hasImageState {
			return fmt.Errorf("image backend is not initialized.\nRun `grove migrate --to image`")
		}
		return SaveBackendState(repoRoot, "image")
	default:
		return fmt.Errorf("invalid clone_backend %q: expected cp or image", cfg.CloneBackend)
	}
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
