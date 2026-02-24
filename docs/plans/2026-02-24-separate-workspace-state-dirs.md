# Separate workspace_dir and state_dir — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Separate `workspace_dir` (user-facing workspace directories) from `state_dir` (internal runtime state like sparsebundles, shadows, metadata).

**Architecture:** Add `StateDir` to the config struct. `ImageRuntimeRoot()` derives paths from `state_dir` instead of `workspace_dir`. Auto-migrate existing setups that store runtimes under workspace_dir.

**Tech Stack:** Go, cobra CLI, charmbracelet/huh (interactive prompts)

---

### Task 1: Add StateDir to Config struct and defaults

**Files:**
- Modify: `internal/config/config.go:31-44` (Config struct + DefaultConfig)
- Modify: `internal/config/config.go:109-132` (Save — persistedConfig struct)
- Test: `internal/config/config_test.go`

**Step 1: Write failing tests**

Add to `internal/config/config_test.go`:

```go
func TestDefaultConfig_StateDir(t *testing.T) {
	cfg := config.DefaultConfig("myapp")
	if cfg.StateDir != "~/.grove" {
		t.Errorf("DefaultConfig().StateDir = %q, want %q", cfg.StateDir, "~/.grove")
	}
}

func TestDefaultConfig_WorkspaceDir_NewDefault(t *testing.T) {
	cfg := config.DefaultConfig("myapp")
	want := "~/grove-workspaces/{project}"
	if cfg.WorkspaceDir != want {
		t.Errorf("DefaultConfig().WorkspaceDir = %q, want %q", cfg.WorkspaceDir, want)
	}
}

func TestSaveAndLoad_StateDir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".grove"), 0755)

	cfg := &config.Config{
		WorkspaceDir:  "/tmp/workspaces",
		StateDir:      "/custom/state",
		MaxWorkspaces: 5,
	}
	if err := config.Save(dir, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.StateDir != "/custom/state" {
		t.Errorf("expected /custom/state, got %q", loaded.StateDir)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/3776-i-think-we-need/grove && go test ./internal/config/ -run "TestDefaultConfig_StateDir|TestDefaultConfig_WorkspaceDir_NewDefault|TestSaveAndLoad_StateDir" -v`

Expected: FAIL — `StateDir` field doesn't exist, wrong default for WorkspaceDir

**Step 3: Implement changes**

In `internal/config/config.go`, update the Config struct (line 31-37):

```go
type Config struct {
	WarmupCommand string   `json:"warmup_command,omitempty"`
	WorkspaceDir  string   `json:"workspace_dir"`
	StateDir      string   `json:"state_dir"`
	MaxWorkspaces int      `json:"max_workspaces"`
	Exclude       []string `json:"exclude,omitempty"`
	CloneBackend  string   `json:"clone_backend,omitempty"`
}
```

Update `DefaultConfig()` (line 39-44):

```go
func DefaultConfig(projectName string) *Config {
	return &Config{
		WorkspaceDir:  "~/grove-workspaces/{project}",
		StateDir:      "~/.grove",
		MaxWorkspaces: 10,
	}
}
```

Update `Save()` — the `persistedConfig` struct (line 114-120) and marshaling (line 121-127):

```go
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
```

Update `Load()` — after unmarshaling, set StateDir default if empty (after line 62, near the MaxWorkspaces default):

```go
if cfg.StateDir == "" {
	cfg.StateDir = "~/.grove"
}
```

Update `LoadOrDefault()` — the default config already gets `StateDir` from `DefaultConfig()`, so no change needed there.

**Step 4: Update existing tests that check old default**

In `internal/config/config_test.go`, update `TestDefaultConfig_WorkspaceDir` (line 23-29):

```go
func TestDefaultConfig_WorkspaceDir(t *testing.T) {
	cfg := config.DefaultConfig("myapp")
	want := "~/grove-workspaces/{project}"
	if cfg.WorkspaceDir != want {
		t.Errorf("DefaultConfig().WorkspaceDir = %q, want %q", cfg.WorkspaceDir, want)
	}
}
```

Update `TestLoadOrDefault_NoConfig` (line 617-633) and `TestLoadOrDefault_NoGroveDir` (line 660-669) — change expected workspace dir from `"~/.grove/{project}"` to `"~/grove-workspaces/{project}"`.

Update `TestExpandWorkspaceDir_Tilde` (line 120-128) — change input and suffix check:

```go
func TestExpandWorkspaceDir_Tilde(t *testing.T) {
	result := config.ExpandWorkspaceDir("~/grove-workspaces/{project}", "myapp")
	if strings.HasPrefix(result, "~") {
		t.Errorf("tilde not expanded: %s", result)
	}
	if !strings.HasSuffix(result, "/grove-workspaces/myapp") {
		t.Errorf("unexpected expansion: %s", result)
	}
}
```

**Step 5: Run all config tests**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/3776-i-think-we-need/grove && go test ./internal/config/ -v`

Expected: ALL PASS

**Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add StateDir field, update workspace_dir default"
```

---

### Task 2: Add ExpandStateDir and update ImageRuntimeRoot to use StateDir

**Files:**
- Modify: `internal/config/config.go:155-163` (ExpandWorkspaceDir area — add ExpandStateDir)
- Modify: `internal/config/config.go:202-269` (ImageRuntimeRoot, EnsureImageRuntimeRoot, runtimeRootForID)
- Modify: `internal/config/config.go:292-319` (BuildImageSyncExcludes)
- Test: `internal/config/config_test.go`

**Step 1: Write failing tests**

Add to `internal/config/config_test.go`:

```go
func TestExpandStateDir(t *testing.T) {
	result := config.ExpandStateDir("~/.grove")
	if strings.HasPrefix(result, "~") {
		t.Errorf("tilde not expanded: %s", result)
	}
	if !strings.HasSuffix(result, "/.grove") {
		t.Errorf("unexpected expansion: %s", result)
	}
}

func TestExpandStateDir_Absolute(t *testing.T) {
	result := config.ExpandStateDir("/custom/state")
	if result != "/custom/state" {
		t.Errorf("expected /custom/state, got %q", result)
	}
}

func TestImageRuntimeRoot_UsesStateDir(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "My-Repo")
	if err := os.MkdirAll(filepath.Join(repo, ".grove"), 0755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".grove", ".runtime-id"), []byte("abc123\n"), 0644); err != nil {
		t.Fatalf("write runtime id: %v", err)
	}
	stateDir := filepath.Join(t.TempDir(), "state")
	cfg := &config.Config{
		WorkspaceDir: filepath.Join(t.TempDir(), "workspaces"),
		StateDir:     stateDir,
	}

	root, err := config.ImageRuntimeRoot(repo, cfg)
	if err != nil {
		t.Fatalf("ImageRuntimeRoot() error = %v", err)
	}
	want := filepath.Join(stateDir, "runtimes", "abc123")
	if root != want {
		t.Fatalf("expected runtime root %q, got %q", want, root)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/3776-i-think-we-need/grove && go test ./internal/config/ -run "TestExpandStateDir|TestImageRuntimeRoot_UsesStateDir" -v`

Expected: FAIL — `ExpandStateDir` doesn't exist; `ImageRuntimeRoot` still uses workspace_dir

**Step 3: Implement ExpandStateDir**

Add to `internal/config/config.go` after `ExpandWorkspaceDir` (after line 163):

```go
// ExpandStateDir expands ~ in the state directory path.
func ExpandStateDir(stateDir string) string {
	if strings.HasPrefix(stateDir, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, stateDir[2:])
		}
	}
	return stateDir
}
```

**Step 4: Update ImageRuntimeRoot and EnsureImageRuntimeRoot**

Change `ImageRuntimeRoot()` (line 204-217) to use `cfg.StateDir`:

```go
func ImageRuntimeRoot(repoRoot string, cfg *Config) (string, error) {
	stateDir := ExpandStateDir(cfg.StateDir)
	if !filepath.IsAbs(stateDir) {
		var err error
		stateDir, err = filepath.Abs(stateDir)
		if err != nil {
			return "", err
		}
	}
	runtimeID, err := LoadRuntimeID(repoRoot)
	switch {
	case err == nil:
		return runtimeRootForID(stateDir, runtimeID), nil
	case !errors.Is(err, os.ErrNotExist):
		return "", err
	}
	return legacyImageRuntimeRoot(repoRoot, cfg)
}
```

Change `EnsureImageRuntimeRoot()` (line 221-266) similarly:

```go
func EnsureImageRuntimeRoot(repoRoot string, cfg *Config) (string, error) {
	stateDir := ExpandStateDir(cfg.StateDir)
	if !filepath.IsAbs(stateDir) {
		var err error
		stateDir, err = filepath.Abs(stateDir)
		if err != nil {
			return "", err
		}
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
	runtimeRoot := runtimeRootForID(stateDir, runtimeID)
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
```

Update `runtimeRootForID()` (line 268-270) — the function signature stays the same, but the first arg is now `stateDir` instead of `workspaceDir`. Rename the parameter for clarity:

```go
func runtimeRootForID(stateDir, runtimeID string) string {
	return filepath.Join(stateDir, "runtimes", runtimeID)
}
```

Remove `expandedWorkspaceDirAbs()` (line 321-327) — it's no longer needed by ImageRuntimeRoot. But check if BuildImageSyncExcludes still needs it. It does — so keep it. Actually, `legacyImageRuntimeRoot` still uses it too. Keep it.

**Step 5: Update BuildImageSyncExcludes to also exclude state_dir**

Modify `BuildImageSyncExcludes()` (line 292-319) to also check `state_dir`:

```go
func BuildImageSyncExcludes(goldenRoot string, cfg *Config) ([]string, error) {
	excludes := append([]string(nil), cfg.Exclude...)

	absGoldenRoot, err := filepath.Abs(goldenRoot)
	if err != nil {
		return nil, err
	}

	// Exclude workspace_dir if inside repo
	workspaceDir, err := expandedWorkspaceDirAbs(goldenRoot, cfg.WorkspaceDir)
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

	// Exclude state_dir if inside repo
	stateDir := ExpandStateDir(cfg.StateDir)
	if !filepath.IsAbs(stateDir) {
		stateDir, err = filepath.Abs(stateDir)
		if err != nil {
			return nil, err
		}
	}
	rel, err = filepath.Rel(absGoldenRoot, stateDir)
	if err != nil {
		return nil, err
	}
	rel = filepath.Clean(rel)
	if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "." {
		excludes = append(excludes, filepath.ToSlash(rel)+"/")
	}

	return excludes, nil
}
```

**Step 6: Update existing tests that construct configs without StateDir**

Any test that creates a `config.Config{}` literal with `WorkspaceDir` and passes it to `ImageRuntimeRoot()`, `EnsureImageRuntimeRoot()`, or `BuildImageSyncExcludes()` needs a `StateDir` field too. Affected tests in `config_test.go`:

- `TestImageRuntimeRoot_UsesRuntimeIDFile` (line 202): add `StateDir: filepath.Join(t.TempDir(), "state")`
  - Update `want` to use the state dir path: `want := filepath.Join(stateDir, "runtimes", "abc123")`
- `TestImageRuntimeRoot_WithoutRuntimeIDFallsBackToLegacyPath` (line 231): add StateDir (can still use workspace-based legacy path)
- `TestEnsureImageRuntimeRoot_AssignsRuntimeIDAndMigratesLegacyDir` (line 249): add StateDir, update `wantRoot`
- `TestEnsureImageRuntimeRoot_DoesNotPersistExpandedWorkspaceDir` (line 303): add StateDir
- `TestEnsureBackendCompatible_CPDetectsRuntimeImageState` (line 535): add StateDir

**Step 7: Run all config tests**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/3776-i-think-we-need/grove && go test ./internal/config/ -v`

Expected: ALL PASS

**Step 8: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): ImageRuntimeRoot uses StateDir, add ExpandStateDir"
```

---

### Task 3: Add state_dir migration logic

**Files:**
- Modify: `internal/config/config.go` (add MigrateRuntimesToStateDir function)
- Test: `internal/config/config_test.go`

**Step 1: Write failing test**

Add to `internal/config/config_test.go`:

```go
func TestMigrateRuntimesToStateDir(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "myproject")
	os.MkdirAll(filepath.Join(repo, ".grove"), 0755)

	// Old workspace_dir with runtimes/ inside it
	oldWorkspaceDir := filepath.Join(t.TempDir(), "old-workspaces")
	runtimesDir := filepath.Join(oldWorkspaceDir, "runtimes", "abc123", "images")
	os.MkdirAll(runtimesDir, 0755)
	os.WriteFile(filepath.Join(runtimesDir, "state.json"), []byte("{}"), 0644)

	stateDir := filepath.Join(t.TempDir(), "state")
	cfg := &config.Config{
		WorkspaceDir:  oldWorkspaceDir,
		StateDir:      stateDir,
		MaxWorkspaces: 10,
	}

	migrated, err := config.MigrateRuntimesToStateDir(cfg)
	if err != nil {
		t.Fatalf("MigrateRuntimesToStateDir() error = %v", err)
	}
	if !migrated {
		t.Fatal("expected migration to occur")
	}

	// Runtimes should now be under state_dir
	newStateFile := filepath.Join(stateDir, "runtimes", "abc123", "images", "state.json")
	if _, err := os.Stat(newStateFile); err != nil {
		t.Fatalf("expected migrated state file at %s: %v", newStateFile, err)
	}

	// Old runtimes dir should be gone
	if _, err := os.Stat(filepath.Join(oldWorkspaceDir, "runtimes")); !os.IsNotExist(err) {
		t.Fatalf("expected old runtimes dir to be removed, err=%v", err)
	}
}

func TestMigrateRuntimesToStateDir_NoRuntimes(t *testing.T) {
	oldWorkspaceDir := filepath.Join(t.TempDir(), "workspaces")
	os.MkdirAll(oldWorkspaceDir, 0755)

	cfg := &config.Config{
		WorkspaceDir: oldWorkspaceDir,
		StateDir:     filepath.Join(t.TempDir(), "state"),
	}

	migrated, err := config.MigrateRuntimesToStateDir(cfg)
	if err != nil {
		t.Fatalf("MigrateRuntimesToStateDir() error = %v", err)
	}
	if migrated {
		t.Fatal("expected no migration when no runtimes/ exists")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/3776-i-think-we-need/grove && go test ./internal/config/ -run "TestMigrateRuntimesToStateDir" -v`

Expected: FAIL — function doesn't exist

**Step 3: Implement MigrateRuntimesToStateDir**

Add to `internal/config/config.go`:

```go
// MigrateRuntimesToStateDir moves runtimes/ from workspace_dir to state_dir
// if they exist under workspace_dir. Returns true if migration occurred.
func MigrateRuntimesToStateDir(cfg *Config) (bool, error) {
	stateDir := ExpandStateDir(cfg.StateDir)
	if !filepath.IsAbs(stateDir) {
		var err error
		stateDir, err = filepath.Abs(stateDir)
		if err != nil {
			return false, err
		}
	}

	// Check for runtimes/ under the (already-expanded) workspace_dir
	oldRuntimes := filepath.Join(cfg.WorkspaceDir, "runtimes")
	info, err := os.Stat(oldRuntimes)
	if err != nil || !info.IsDir() {
		return false, nil
	}

	newRuntimes := filepath.Join(stateDir, "runtimes")
	if oldRuntimes == newRuntimes {
		return false, nil
	}

	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return false, err
	}
	if err := os.Rename(oldRuntimes, newRuntimes); err != nil {
		return false, fmt.Errorf("migrating runtimes to state_dir: %w", err)
	}
	return true, nil
}
```

Note: This function expects `cfg.WorkspaceDir` to already be expanded (as it is in all command handlers). Callers expand before calling.

**Step 4: Run migration tests**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/3776-i-think-we-need/grove && go test ./internal/config/ -run "TestMigrateRuntimesToStateDir" -v`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add MigrateRuntimesToStateDir for auto-migration"
```

---

### Task 4: Update CLI commands — init

**Files:**
- Modify: `cmd/grove/init.go`

**Step 1: Add --state-dir flag**

In `cmd/grove/init.go`, update `init()` function (line 236-244):

```go
func init() {
	initCmd.Flags().String("warmup-command", "", "Command to run for warming up build caches")
	initCmd.Flags().String("workspace-dir", "", "Directory for workspaces (default: ~/grove-workspaces/{project})")
	initCmd.Flags().String("state-dir", "", "Directory for grove internal state (default: ~/.grove)")
	initCmd.Flags().String("backend", "", "Workspace backend: cp or image (experimental)")
	initCmd.Flags().Int("image-size-gb", 200, "Base sparsebundle size in GB when using --backend image")
	initCmd.Flags().Bool("progress", false, "Show progress output during image backend initialization")
	initCmd.Flags().Bool("defaults", false, "Skip interactive prompts and use all defaults")
	rootCmd.AddCommand(initCmd)
}
```

**Step 2: Parse the flag and apply overrides**

In the `RunE` function, after parsing `wsDirFlag` (line 57-58), add:

```go
stateDirFlag, _ := cmd.Flags().GetString("state-dir")
stateDirSet := cmd.Flags().Changed("state-dir")
```

After the `wsDirSet` override block (line 72-74), add:

```go
if stateDirSet {
	cfg.StateDir = stateDirFlag
}
```

**Step 3: Add interactive prompt for state dir**

After the workspace dir interactive prompt block (after line 118), add:

```go
if !stateDirSet {
	defaultStateDir := cfg.StateDir
	if defaultStateDir == "" {
		defaultStateDir = "~/.grove"
	}
	stateDir := defaultStateDir
	err := huh.NewInput().
		Title("State directory").
		Description("Internal state (runtime images, shadows)").
		Value(&stateDir).
		Run()
	if err != nil {
		return err
	}
	if stateDir != "" {
		cfg.StateDir = stateDir
	}
}
```

**Step 4: Update output at end of init**

At line 231, update to also display state_dir:

```go
fmt.Printf("Grove initialized at %s\n", absPath)
fmt.Printf("Workspace dir: %s\n", config.ExpandWorkspaceDir(cfg.WorkspaceDir, projectName))
fmt.Printf("State dir:     %s\n", config.ExpandStateDir(cfg.StateDir))
```

**Step 5: Update help text for workspace-dir flag**

Update line 238: change `"Directory for workspaces (default: ~/.grove/{project})"` to `"Directory for workspaces (default: ~/grove-workspaces/{project})"`.

**Step 6: Run build check**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/3776-i-think-we-need/grove && go build ./cmd/grove/`

Expected: Compiles without errors

**Step 7: Commit**

```bash
git add cmd/grove/init.go
git commit -m "feat(cli): add --state-dir flag to grove init"
```

---

### Task 5: Update CLI commands — create, list, status, destroy

**Files:**
- Modify: `cmd/grove/create.go`
- Modify: `cmd/grove/list.go`
- Modify: `cmd/grove/status.go`
- Modify: `cmd/grove/destroy.go`

**Step 1: Update create.go**

In `create.go`, after line 84 (`cfg.WorkspaceDir = config.ExpandWorkspaceDir(...)`), add:

```go
cfg.StateDir = config.ExpandStateDir(cfg.StateDir)
```

This ensures both are expanded before passing to backend functions.

Also add migration call after expanding dirs and before the backend operations (after the `EnsureBackendCompatible` block, around line 76):

```go
// Expand dirs
projectName := getProjectName(goldenRoot)
cfg.WorkspaceDir = config.ExpandWorkspaceDir(cfg.WorkspaceDir, projectName)
cfg.StateDir = config.ExpandStateDir(cfg.StateDir)

// Auto-migrate runtimes from workspace_dir to state_dir
if migrated, err := config.MigrateRuntimesToStateDir(cfg); err != nil {
	return fmt.Errorf("migrating runtime state: %w", err)
} else if migrated {
	fmt.Fprintf(os.Stderr, "Migrated runtime state to %s\n", cfg.StateDir)
}
```

Note: Remove the existing `projectName` and `cfg.WorkspaceDir` expansion lines (lines 83-84) and replace with the block above. Keep the variable declarations in the right order.

**Step 2: Update list.go**

In `list.go`, after line 35 (`cfg.WorkspaceDir = config.ExpandWorkspaceDir(...)`), add:

```go
cfg.StateDir = config.ExpandStateDir(cfg.StateDir)
```

**Step 3: Update status.go**

In `status.go`, after line 33 (`cfg.WorkspaceDir = config.ExpandWorkspaceDir(...)`), add:

```go
cfg.StateDir = config.ExpandStateDir(cfg.StateDir)
```

Update the output (around line 58) to also show state dir:

```go
fmt.Printf("Workspaces:  %d / %d (max)\n", len(workspaces), cfg.MaxWorkspaces)
fmt.Printf("Workspace dir: %s\n", cfg.WorkspaceDir)
fmt.Printf("State dir:     %s\n", cfg.StateDir)
```

**Step 4: Update destroy.go**

In `destroy.go`, after line 40 (`cfg.WorkspaceDir = config.ExpandWorkspaceDir(...)`), add:

```go
cfg.StateDir = config.ExpandStateDir(cfg.StateDir)
```

**Step 5: Run build check**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/3776-i-think-we-need/grove && go build ./cmd/grove/`

Expected: Compiles without errors

**Step 6: Commit**

```bash
git add cmd/grove/create.go cmd/grove/list.go cmd/grove/status.go cmd/grove/destroy.go
git commit -m "feat(cli): expand StateDir in all commands, add auto-migration to create"
```

---

### Task 6: Update backend/image.go and backend/destroy.go

**Files:**
- Modify: `internal/backend/image.go:85-111` (RefreshBase loads its own config — needs StateDir expansion)

**Step 1: Check RefreshBase**

In `internal/backend/image.go` line 85-111, `RefreshBase()` calls `config.Load()` and then `config.EnsureImageRuntimeRoot()`. Since `Load()` now defaults `StateDir` to `"~/.grove"`, and `EnsureImageRuntimeRoot` now reads from `cfg.StateDir`, this should work. But it doesn't expand `StateDir`. Add expansion.

After line 89 (`return fmt.Errorf("loading config: %w", err)`), before `EnsureImageRuntimeRoot`:

```go
cfg.StateDir = config.ExpandStateDir(cfg.StateDir)
```

**Step 2: Run all tests**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/3776-i-think-we-need/grove && go test ./... -v`

Expected: ALL PASS

**Step 3: Commit**

```bash
git add internal/backend/image.go
git commit -m "fix(backend): expand StateDir in RefreshBase"
```

---

### Task 7: Update update.go command

**Files:**
- Modify: `cmd/grove/update.go`

**Step 1: Add StateDir expansion**

In `update.go`, after loading config (line 38), add StateDir expansion. After `config.LoadOrDefault`:

```go
cfg.StateDir = config.ExpandStateDir(cfg.StateDir)
```

Note: `update.go` passes `goldenRoot` to `backendImpl.RefreshBase()` which loads config again internally. The image backend's `RefreshBase` now also expands StateDir (from Task 6), so the update command is covered.

**Step 2: Run build**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/3776-i-think-we-need/grove && go build ./cmd/grove/`

Expected: Compiles

**Step 3: Commit**

```bash
git add cmd/grove/update.go
git commit -m "feat(cli): expand StateDir in update command"
```

---

### Task 8: Run full test suite and fix any remaining failures

**Step 1: Run all tests**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/3776-i-think-we-need/grove && go test ./... -v`

Expected: ALL PASS

**Step 2: Fix any failures**

Look for tests that construct `Config{}` without `StateDir` and pass them to functions that now read `StateDir`. Common fixes: add `StateDir: "~/.grove"` or `StateDir: t.TempDir()` to test config literals.

**Step 3: Run tests again**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/3776-i-think-we-need/grove && go test ./... -v`

Expected: ALL PASS

**Step 4: Commit if any fixes needed**

```bash
git add -A
git commit -m "fix: update remaining tests for StateDir"
```
