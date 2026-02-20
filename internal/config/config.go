package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	GroveDirName  = ".grove"
	ConfigFile    = "config.json"
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
