package workspace_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/chrisbanes/grove/internal/clone"
	"github.com/chrisbanes/grove/internal/config"
	"github.com/chrisbanes/grove/internal/workspace"
)

func setupGolden(t *testing.T) (string, *config.Config) {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".grove", "hooks"), 0755)
	os.WriteFile(filepath.Join(dir, "src.txt"), []byte("source"), 0644)

	wsDir := filepath.Join(t.TempDir(), "workspaces")
	cfg := &config.Config{
		WorkspaceDir:  wsDir,
		MaxWorkspaces: 3,
	}
	config.Save(dir, cfg)
	return dir, cfg
}

func TestCreate(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	golden, cfg := setupGolden(t)
	c, _ := clone.NewCloner(golden)

	info, err := workspace.Create(golden, cfg, c, workspace.CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}

	if info.ID == "" {
		t.Error("expected non-empty ID")
	}
	if info.Path == "" {
		t.Error("expected non-empty Path")
	}

	// Verify source file was cloned
	data, err := os.ReadFile(filepath.Join(info.Path, "src.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "source" {
		t.Errorf("expected 'source', got %q", string(data))
	}

	// Verify workspace marker exists
	if !workspace.IsWorkspace(info.Path) {
		t.Error("expected workspace marker to exist")
	}
}

func TestCreate_MaxWorkspaces(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	golden, cfg := setupGolden(t)
	cfg.MaxWorkspaces = 2
	c, _ := clone.NewCloner(golden)

	workspace.Create(golden, cfg, c, workspace.CreateOpts{})
	workspace.Create(golden, cfg, c, workspace.CreateOpts{})
	_, err := workspace.Create(golden, cfg, c, workspace.CreateOpts{})
	if err == nil {
		t.Error("expected error when exceeding max workspaces")
	}
}

func TestList(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	golden, cfg := setupGolden(t)
	c, _ := clone.NewCloner(golden)

	workspace.Create(golden, cfg, c, workspace.CreateOpts{})
	workspace.Create(golden, cfg, c, workspace.CreateOpts{})

	list, err := workspace.List(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 workspaces, got %d", len(list))
	}
}

func TestDestroy(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	golden, cfg := setupGolden(t)
	c, _ := clone.NewCloner(golden)

	info, _ := workspace.Create(golden, cfg, c, workspace.CreateOpts{})
	err := workspace.Destroy(cfg, info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(info.Path); !os.IsNotExist(err) {
		t.Error("workspace directory should be deleted")
	}

	list, _ := workspace.List(cfg)
	if len(list) != 0 {
		t.Errorf("expected 0 workspaces after destroy, got %d", len(list))
	}
}

func TestIsWorkspace(t *testing.T) {
	dir := t.TempDir()
	if workspace.IsWorkspace(dir) {
		t.Error("expected false for non-workspace")
	}

	os.MkdirAll(filepath.Join(dir, ".grove"), 0755)
	os.WriteFile(filepath.Join(dir, ".grove", "workspace.json"), []byte("{}"), 0644)
	if !workspace.IsWorkspace(dir) {
		t.Error("expected true for workspace with marker")
	}
}
