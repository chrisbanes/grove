package config_test

import (
	"os"
	"path/filepath"
	"strings"
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

func TestLoad_RuntimeIDNoLongerSupported(t *testing.T) {
	dir := t.TempDir()
	groveDir := filepath.Join(dir, ".grove")
	os.MkdirAll(groveDir, 0755)

	os.WriteFile(
		filepath.Join(groveDir, "config.json"),
		[]byte(`{"workspace_dir": "/tmp/test", "runtime_id": "abc123"}`),
		0644,
	)

	_, err := config.Load(dir)
	if err == nil {
		t.Fatal("expected error for legacy runtime_id")
	}
	if !strings.Contains(err.Error(), "runtime_id") {
		t.Fatalf("expected runtime_id guidance, got: %v", err)
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

func TestBuildImageSyncExcludes_AddsWorkspaceDirWhenInsideRepo(t *testing.T) {
	repo := t.TempDir()
	cfg := &config.Config{
		WorkspaceDir: filepath.Join(repo, "workspaces"),
		Exclude:      []string{"node_modules"},
	}

	excludes, err := config.BuildImageSyncExcludes(repo, cfg)
	if err != nil {
		t.Fatalf("BuildImageSyncExcludes() error = %v", err)
	}

	if len(excludes) != 2 {
		t.Fatalf("expected 2 excludes, got %d: %v", len(excludes), excludes)
	}
	if excludes[0] != "node_modules" {
		t.Fatalf("expected first exclude node_modules, got %q", excludes[0])
	}
	if excludes[1] != "workspaces/" {
		t.Fatalf("expected workspace exclude workspaces/, got %q", excludes[1])
	}
}

func TestBuildImageSyncExcludes_NoWorkspaceExcludeWhenOutsideRepo(t *testing.T) {
	repo := t.TempDir()
	cfg := &config.Config{
		WorkspaceDir: filepath.Join(t.TempDir(), "workspaces"),
		Exclude:      []string{"node_modules"},
	}

	excludes, err := config.BuildImageSyncExcludes(repo, cfg)
	if err != nil {
		t.Fatalf("BuildImageSyncExcludes() error = %v", err)
	}
	if len(excludes) != 1 || excludes[0] != "node_modules" {
		t.Fatalf("expected user excludes only, got %v", excludes)
	}
}

func TestBuildImageSyncExcludes_ExpandsTemplateBeforeComparison(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "myproj")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	cfg := &config.Config{
		WorkspaceDir: "workspaces/{project}",
	}

	excludes, err := config.BuildImageSyncExcludes(repo, cfg)
	if err != nil {
		t.Fatalf("BuildImageSyncExcludes() error = %v", err)
	}
	if len(excludes) != 1 || excludes[0] != "workspaces/myproj/" {
		t.Fatalf("expected expanded workspace exclude, got %v", excludes)
	}
}

func TestBuildImageSyncExcludes_ErrorsWhenWorkspaceDirIsRepoRoot(t *testing.T) {
	repo := t.TempDir()
	cfg := &config.Config{
		WorkspaceDir: ".",
	}

	_, err := config.BuildImageSyncExcludes(repo, cfg)
	if err == nil {
		t.Fatal("expected error when workspace dir resolves to repo root")
	}
	if !strings.Contains(err.Error(), "repository root") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestImageRuntimeRoot_UsesRuntimeIDFile(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "My-Repo")
	if err := os.MkdirAll(filepath.Join(repo, ".grove"), 0755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".grove", ".runtime-id"), []byte("abc123\n"), 0644); err != nil {
		t.Fatalf("write runtime id: %v", err)
	}
	workspaceDir := filepath.Join(t.TempDir(), "workspaces", "{project}")
	cfg := &config.Config{WorkspaceDir: workspaceDir}

	rootA, err := config.ImageRuntimeRoot(repo, cfg)
	if err != nil {
		t.Fatalf("ImageRuntimeRoot() error = %v", err)
	}
	rootB, err := config.ImageRuntimeRoot(repo, cfg)
	if err != nil {
		t.Fatalf("ImageRuntimeRoot() second call error = %v", err)
	}

	if rootA != rootB {
		t.Fatalf("expected stable runtime root, got %q vs %q", rootA, rootB)
	}
	want := filepath.Join(filepath.Dir(workspaceDir), "My-Repo", "runtimes", "abc123")
	if rootA != want {
		t.Fatalf("expected runtime root %q, got %q", want, rootA)
	}
}

func TestImageRuntimeRoot_WithoutRuntimeIDFallsBackToLegacyPath(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "My-Repo")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	workspaceDir := filepath.Join(t.TempDir(), "workspaces", "{project}")
	cfg := &config.Config{WorkspaceDir: workspaceDir}

	root, err := config.ImageRuntimeRoot(repo, cfg)
	if err != nil {
		t.Fatalf("ImageRuntimeRoot() error = %v", err)
	}
	prefix := filepath.Join(filepath.Dir(workspaceDir), "My-Repo", ".grove-runtime") + string(filepath.Separator)
	if !strings.HasPrefix(root, prefix) {
		t.Fatalf("expected legacy runtime root under %q, got %q", prefix, root)
	}
}

func TestEnsureImageRuntimeRoot_AssignsRuntimeIDAndMigratesLegacyDir(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "My-Repo")
	if err := os.MkdirAll(filepath.Join(repo, ".grove"), 0755); err != nil {
		t.Fatalf("mkdir .grove: %v", err)
	}
	workspaceDir := filepath.Join(t.TempDir(), "workspaces", "{project}")
	cfg := &config.Config{
		WorkspaceDir:  workspaceDir,
		CloneBackend:  "image",
		MaxWorkspaces: 10,
	}
	if err := config.Save(repo, cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	legacyRoot, err := config.ImageRuntimeRoot(repo, cfg)
	if err != nil {
		t.Fatalf("ImageRuntimeRoot() legacy error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(legacyRoot, "images"), 0755); err != nil {
		t.Fatalf("mkdir legacy images: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "images", "state.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}

	runtimeRoot, err := config.EnsureImageRuntimeRoot(repo, cfg)
	if err != nil {
		t.Fatalf("EnsureImageRuntimeRoot() error = %v", err)
	}
	runtimeID, err := config.LoadRuntimeID(repo)
	if err != nil {
		t.Fatalf("LoadRuntimeID() error = %v", err)
	}
	wantRoot := filepath.Join(filepath.Dir(workspaceDir), "My-Repo", "runtimes", runtimeID)
	if runtimeRoot != wantRoot {
		t.Fatalf("expected runtime root %q, got %q", wantRoot, runtimeRoot)
	}
	if _, err := os.Stat(filepath.Join(runtimeRoot, "images", "state.json")); err != nil {
		t.Fatalf("expected migrated state at new runtime root: %v", err)
	}
	if _, err := os.Stat(legacyRoot); !os.IsNotExist(err) {
		t.Fatalf("expected legacy root moved away, got err=%v", err)
	}

	configRaw, err := os.ReadFile(filepath.Join(repo, ".grove", "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(configRaw), `"runtime_id"`) {
		t.Fatalf("expected runtime_id to be moved out of config.json, got:\n%s", string(configRaw))
	}
}

func TestEnsureImageRuntimeRoot_DoesNotPersistExpandedWorkspaceDir(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "My-Repo")
	if err := os.MkdirAll(filepath.Join(repo, ".grove"), 0755); err != nil {
		t.Fatalf("mkdir .grove: %v", err)
	}
	templateDir := filepath.Join(t.TempDir(), "workspaces", "{project}")
	savedCfg := &config.Config{
		WorkspaceDir:  templateDir,
		CloneBackend:  "image",
		MaxWorkspaces: 10,
	}
	if err := config.Save(repo, savedCfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loadedCfg, err := config.Load(repo)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	loadedCfg.WorkspaceDir = config.ExpandWorkspaceDir(templateDir, "My-Repo")

	if _, err := config.EnsureImageRuntimeRoot(repo, loadedCfg); err != nil {
		t.Fatalf("EnsureImageRuntimeRoot() error = %v", err)
	}

	reloadedCfg, err := config.Load(repo)
	if err != nil {
		t.Fatalf("Load() after ensure error = %v", err)
	}
	if reloadedCfg.WorkspaceDir != templateDir {
		t.Fatalf("expected workspace_dir to remain template %q, got %q", templateDir, reloadedCfg.WorkspaceDir)
	}
	if _, err := config.LoadRuntimeID(repo); err != nil {
		t.Fatalf("expected runtime_id file to be persisted: %v", err)
	}
	configRaw, err := os.ReadFile(filepath.Join(repo, ".grove", "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(configRaw), `"runtime_id"`) {
		t.Fatalf("expected runtime_id to be absent from config.json, got:\n%s", string(configRaw))
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

func TestEnsureBackendCompatible_SeedsCPState(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".grove"), 0755)
	os.WriteFile(
		filepath.Join(dir, ".grove", "config.json"),
		[]byte(`{"workspace_dir": "/tmp/test", "clone_backend": "cp"}`),
		0644,
	)
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := config.EnsureBackendCompatible(dir, cfg); err != nil {
		t.Fatalf("EnsureBackendCompatible() error = %v", err)
	}

	backend, err := config.LoadBackendState(dir)
	if err != nil {
		t.Fatalf("LoadBackendState() error = %v", err)
	}
	if backend != "cp" {
		t.Fatalf("expected backend state cp, got %q", backend)
	}
}

func TestEnsureBackendCompatible_ImageWithoutStateAllowsLazyBootstrap(t *testing.T) {
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

	if err := config.EnsureBackendCompatible(dir, cfg); err != nil {
		t.Fatalf("EnsureBackendCompatible() error = %v", err)
	}

	if _, err := config.LoadBackendState(dir); !os.IsNotExist(err) {
		t.Fatalf("expected backend state to remain unset for lazy bootstrap, got err=%v", err)
	}
}

func TestEnsureBackendCompatible_MismatchErrors(t *testing.T) {
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

	if err := config.SaveBackendState(dir, "cp"); err != nil {
		t.Fatalf("SaveBackendState() error = %v", err)
	}
	err = config.EnsureBackendCompatible(dir, cfg)
	if err == nil {
		t.Fatal("expected mismatch error")
	}
	if !strings.Contains(err.Error(), "grove migrate --to image") {
		t.Fatalf("expected migrate guidance in error, got: %v", err)
	}
}

func TestEnsureBackendCompatible_CPDetectsRuntimeImageState(t *testing.T) {
	dir := t.TempDir()
	workspaceDir := filepath.Join(t.TempDir(), "workspaces")
	os.MkdirAll(filepath.Join(dir, ".grove"), 0755)
	os.WriteFile(
		filepath.Join(dir, ".grove", "config.json"),
		[]byte(`{"workspace_dir": "`+workspaceDir+`", "clone_backend": "cp"}`),
		0644,
	)
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	runtimeRoot, err := config.ImageRuntimeRoot(dir, cfg)
	if err != nil {
		t.Fatalf("ImageRuntimeRoot() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(runtimeRoot, "images"), 0755); err != nil {
		t.Fatalf("mkdir runtime images: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runtimeRoot, "images", "state.json"), []byte(`{}`), 0644); err != nil {
		t.Fatalf("write runtime state: %v", err)
	}

	err = config.EnsureBackendCompatible(dir, cfg)
	if err == nil {
		t.Fatal("expected mismatch error")
	}
	if !strings.Contains(err.Error(), "grove migrate --to cp") {
		t.Fatalf("expected migrate guidance in error, got: %v", err)
	}
}

func TestEnsureGroveGitignore_CreatesDefaultFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".grove"), 0755); err != nil {
		t.Fatalf("mkdir .grove: %v", err)
	}

	if err := config.EnsureGroveGitignore(dir); err != nil {
		t.Fatalf("EnsureGroveGitignore() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".grove", ".gitignore"))
	if err != nil {
		t.Fatalf("read .grove/.gitignore: %v", err)
	}
	content := string(data)
	for _, pattern := range []string{
		"images/base.sparsebundle/",
		"images/state.json",
		"images/init-in-progress",
		"shadows/",
		"workspaces/",
		"mnt/",
		"workspace.json",
		".runtime-id",
	} {
		if !strings.Contains(content, pattern) {
			t.Fatalf("expected pattern %q in .grove/.gitignore, got:\n%s", pattern, content)
		}
	}
}

func TestEnsureGroveGitignore_DoesNotOverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".grove"), 0755); err != nil {
		t.Fatalf("mkdir .grove: %v", err)
	}

	path := filepath.Join(dir, ".grove", ".gitignore")
	if err := os.WriteFile(path, []byte("custom\n"), 0644); err != nil {
		t.Fatalf("seed .gitignore: %v", err)
	}

	if err := config.EnsureGroveGitignore(dir); err != nil {
		t.Fatalf("EnsureGroveGitignore() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read .grove/.gitignore: %v", err)
	}
	if string(data) != "custom\n" {
		t.Fatalf("expected existing .gitignore to remain unchanged, got:\n%s", string(data))
	}
}
