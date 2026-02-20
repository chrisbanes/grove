package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chrisbanes/grove/internal/config"
)

func TestDefaultConfig(t *testing.T) {
	cfg := config.DefaultConfig("myapp")
	if cfg.MaxWorkspaces != 10 {
		t.Errorf("expected max_workspaces 10, got %d", cfg.MaxWorkspaces)
	}
	if cfg.WorkspaceDir == "" {
		t.Error("expected non-empty workspace_dir")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	groveDir := filepath.Join(dir, ".grove")
	os.MkdirAll(groveDir, 0755)

	cfg := &config.Config{
		WarmupCommand: "make build",
		WorkspaceDir:  "/tmp/grove/test",
		MaxWorkspaces: 5,
	}
	err := config.Save(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.WarmupCommand != "make build" {
		t.Errorf("expected 'make build', got %q", loaded.WarmupCommand)
	}
	if loaded.MaxWorkspaces != 5 {
		t.Errorf("expected 5, got %d", loaded.MaxWorkspaces)
	}
}

func TestLoad_Defaults(t *testing.T) {
	dir := t.TempDir()
	groveDir := filepath.Join(dir, ".grove")
	os.MkdirAll(groveDir, 0755)

	os.WriteFile(
		filepath.Join(groveDir, "config.json"),
		[]byte(`{"workspace_dir": "/tmp/test"}`),
		0644,
	)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxWorkspaces != 10 {
		t.Errorf("expected default max_workspaces 10, got %d", cfg.MaxWorkspaces)
	}
	if cfg.CloneBackend != "cp" {
		t.Errorf("expected default clone_backend cp, got %q", cfg.CloneBackend)
	}
}

func TestLoad_NotInitialized(t *testing.T) {
	dir := t.TempDir()
	_, err := config.Load(dir)
	if err == nil {
		t.Error("expected error for non-initialized repo")
	}
}

func TestExpandWorkspaceDir(t *testing.T) {
	cfg := config.DefaultConfig("myapp")
	expanded := config.ExpandWorkspaceDir(cfg.WorkspaceDir, "myapp")
	if expanded == cfg.WorkspaceDir {
		t.Error("expected {project} to be expanded")
	}
	if filepath.Base(expanded) != "myapp" {
		t.Errorf("expected 'myapp' in path, got %q", expanded)
	}
}

func TestFindRepoRoot(t *testing.T) {
	dir := t.TempDir()
	groveDir := filepath.Join(dir, ".grove")
	os.MkdirAll(groveDir, 0755)
	subDir := filepath.Join(dir, "sub", "deep")
	os.MkdirAll(subDir, 0755)

	root, err := config.FindGroveRoot(subDir)
	if err != nil {
		t.Fatal(err)
	}
	if root != dir {
		t.Errorf("expected %s, got %s", dir, root)
	}
}

func TestFindRepoRoot_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := config.FindGroveRoot(dir)
	if err == nil {
		t.Error("expected error when no .grove found")
	}
}

func TestSaveAndLoad_Exclude(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".grove"), 0755)

	cfg := &config.Config{
		WorkspaceDir:  "/tmp/grove/test",
		MaxWorkspaces: 5,
		Exclude:       []string{"*.lock", "__pycache__", ".gradle/configuration-cache"},
	}
	if err := config.Save(dir, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Exclude) != 3 {
		t.Fatalf("expected 3 exclude patterns, got %d", len(loaded.Exclude))
	}
	if loaded.Exclude[0] != "*.lock" {
		t.Errorf("expected '*.lock', got %q", loaded.Exclude[0])
	}
}

func TestLoad_InvalidExcludePattern(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".grove"), 0755)
	os.WriteFile(
		filepath.Join(dir, ".grove", "config.json"),
		[]byte(`{"workspace_dir": "/tmp/test", "exclude": ["[invalid"]}`),
		0644,
	)

	_, err := config.Load(dir)
	if err == nil {
		t.Error("expected error for invalid exclude pattern")
	}
}

func TestLoad_EmptyExclude(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".grove"), 0755)
	os.WriteFile(
		filepath.Join(dir, ".grove", "config.json"),
		[]byte(`{"workspace_dir": "/tmp/test"}`),
		0644,
	)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Exclude != nil {
		t.Errorf("expected nil exclude, got %v", cfg.Exclude)
	}
}

func TestLoad_CloneBackendImage(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".grove"), 0755)
	os.WriteFile(
		filepath.Join(dir, ".grove", "config.json"),
		[]byte(`{"workspace_dir": "/tmp/test", "clone_backend": "image"}`),
		0644,
	)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CloneBackend != "image" {
		t.Errorf("expected clone_backend image, got %q", cfg.CloneBackend)
	}
}

func TestLoad_InvalidCloneBackend(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".grove"), 0755)
	os.WriteFile(
		filepath.Join(dir, ".grove", "config.json"),
		[]byte(`{"workspace_dir": "/tmp/test", "clone_backend": "bad"}`),
		0644,
	)

	_, err := config.Load(dir)
	if err == nil {
		t.Error("expected error for invalid clone backend")
	}
}
