# Omit Default Config Values Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** When saving config.json, omit fields that match their default values — except `workspace_dir` which is always written.

**Architecture:** Modify `Save()` in `internal/config/config.go` to compare each field against `DefaultConfig()` defaults before marshaling. Fields matching defaults get zeroed so `omitempty` handles them. `workspace_dir` is always included.

**Tech Stack:** Go, encoding/json

---

### Task 1: Write failing tests for default-value omission

**Files:**
- Modify: `internal/config/config_test.go`

**Step 1: Write the failing tests**

Add three tests:

```go
func TestSave_OmitsDefaultStateDir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".grove"), 0755)

	cfg := &config.Config{
		WorkspaceDir:  "~/grove-workspaces/{project}",
		StateDir:      "~/.grove",
		MaxWorkspaces: 10,
	}
	if err := config.Save(dir, cfg); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".grove", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, `"state_dir"`) {
		t.Fatalf("expected state_dir to be omitted when default, got:\n%s", content)
	}
	if strings.Contains(content, `"max_workspaces"`) {
		t.Fatalf("expected max_workspaces to be omitted when default, got:\n%s", content)
	}
	if !strings.Contains(content, `"workspace_dir"`) {
		t.Fatalf("expected workspace_dir to always be present, got:\n%s", content)
	}
}

func TestSave_IncludesNonDefaultValues(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".grove"), 0755)

	cfg := &config.Config{
		WorkspaceDir:  "~/grove-workspaces/{project}",
		StateDir:      "/custom/state",
		MaxWorkspaces: 5,
	}
	if err := config.Save(dir, cfg); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".grove", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, `"state_dir"`) {
		t.Fatalf("expected non-default state_dir to be present, got:\n%s", content)
	}
	if !strings.Contains(content, `"max_workspaces"`) {
		t.Fatalf("expected non-default max_workspaces to be present, got:\n%s", content)
	}
}

func TestSave_RoundTripsWithDefaults(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".grove"), 0755)

	cfg := &config.Config{
		WorkspaceDir:  "~/grove-workspaces/{project}",
		StateDir:      "~/.grove",
		MaxWorkspaces: 10,
		CloneBackend:  "cp",
	}
	if err := config.Save(dir, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.StateDir != "~/.grove" {
		t.Errorf("expected state_dir ~.grove after round-trip, got %q", loaded.StateDir)
	}
	if loaded.MaxWorkspaces != 10 {
		t.Errorf("expected max_workspaces 10 after round-trip, got %d", loaded.MaxWorkspaces)
	}
	if loaded.CloneBackend != "cp" {
		t.Errorf("expected clone_backend cp after round-trip, got %q", loaded.CloneBackend)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/ccad-when-the-user-is/grove && go test ./internal/config/ -run "TestSave_Omits|TestSave_Includes|TestSave_RoundTrips" -v`
Expected: `TestSave_OmitsDefaultStateDir` FAILS (state_dir and max_workspaces are currently always written)

**Step 3: Commit**

```bash
git add internal/config/config_test.go
git commit -m "test: add tests for omitting default config values"
```

---

### Task 2: Modify Save() to omit default values

**Files:**
- Modify: `internal/config/config.go:114-139` (the `Save` function)

**Step 1: Update the Save function**

Replace the current `Save` function with logic that compares against defaults before marshaling:

```go
func Save(repoRoot string, cfg *Config) error {
	groveDir := filepath.Join(repoRoot, GroveDirName)
	if err := os.MkdirAll(groveDir, 0755); err != nil {
		return err
	}
	defaults := DefaultConfig("")
	type persistedConfig struct {
		WarmupCommand string   `json:"warmup_command,omitempty"`
		WorkspaceDir  string   `json:"workspace_dir"`
		StateDir      string   `json:"state_dir,omitempty"`
		MaxWorkspaces int      `json:"max_workspaces,omitempty"`
		Exclude       []string `json:"exclude,omitempty"`
		CloneBackend  string   `json:"clone_backend,omitempty"`
	}
	pc := persistedConfig{
		WarmupCommand: cfg.WarmupCommand,
		WorkspaceDir:  cfg.WorkspaceDir,
		Exclude:       cfg.Exclude,
	}
	// Only persist non-default values
	if cfg.StateDir != defaults.StateDir {
		pc.StateDir = cfg.StateDir
	}
	if cfg.MaxWorkspaces != defaults.MaxWorkspaces {
		pc.MaxWorkspaces = cfg.MaxWorkspaces
	}
	if cfg.CloneBackend != "" && cfg.CloneBackend != "cp" {
		pc.CloneBackend = cfg.CloneBackend
	}
	data, err := json.MarshalIndent(&pc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(groveDir, ConfigFile), data, 0644)
}
```

Key changes:
- `max_workspaces` gets `omitempty` on the persisted struct (zero value = omit)
- `StateDir`, `MaxWorkspaces`, and `CloneBackend` are only set on `persistedConfig` when they differ from defaults
- `workspace_dir` is always written (no conditional)
- `clone_backend` "cp" is the default, so only "image" gets written

**Step 2: Run all config tests**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/ccad-when-the-user-is/grove && go test ./internal/config/ -v`
Expected: ALL PASS

**Step 3: Commit**

```bash
git add internal/config/config.go
git commit -m "feat: omit default values from config.json

Only persist config values that differ from defaults.
workspace_dir is always written. state_dir, max_workspaces,
and clone_backend are omitted when they match defaults."
```

---

### Task 3: Run full test suite

**Step 1: Run all tests**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/ccad-when-the-user-is/grove && go test ./... -v`
Expected: ALL PASS

No other files need changes — `Load()` already fills in defaults for missing fields (lines 70-80 of config.go).
