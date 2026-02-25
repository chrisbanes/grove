# Configuration-Less Create Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make `grove create` auto-initialize minimal Grove config in unconfigured repos so first-run create works without a prior `grove config` command.

**Architecture:** Keep a single config model (`internal/config.Config`) and add a minimal-init helper in `internal/config` that persists config only when `config.json` is missing. Update `cmd/grove/create.go` to call that helper, then continue existing create flow unchanged. Emit a one-time informational note when auto-init occurs.

**Tech Stack:** Go, Cobra CLI, encoding/json, existing unit + e2e test suites

---

### Task 1: Add failing config-layer tests for minimal auto-init persistence

**Files:**
- Modify: `internal/config/config_test.go`
- Modify: `internal/config/config.go` (later in Task 2)

**Step 1: Write a failing test for missing-config auto-init write**

Add a new test that starts with a git repo root containing no `.grove/config.json`, calls a new helper (proposed name `LoadOrInitMinimal`), and verifies:
- it returns defaults (`workspace_dir` uses `~/grove-workspaces/{project}`, `max_workspaces`, backend `cp`)
- `.grove/config.json` now exists
- persisted JSON includes `workspace_dir`
- persisted JSON does not include default-only fields (`max_workspaces`, `state_dir`, `clone_backend`)

```go
func TestLoadOrInitMinimal_WritesConfigWhenMissing(t *testing.T) {
    dir := t.TempDir()

    cfg, initialized, err := config.LoadOrInitMinimal(dir)
    if err != nil {
        t.Fatal(err)
    }
    if !initialized {
        t.Fatal("expected initialized=true when config is missing")
    }
    if cfg.WorkspaceDir == "" {
        t.Fatal("expected default workspace_dir")
    }

    raw, err := os.ReadFile(filepath.Join(dir, ".grove", "config.json"))
    if err != nil {
        t.Fatal(err)
    }
    content := string(raw)
    if !strings.Contains(content, `"workspace_dir"`) {
        t.Fatalf("expected workspace_dir in persisted config, got:\n%s", content)
    }
    if strings.Contains(content, `"max_workspaces"`) {
        t.Fatalf("expected max_workspaces omitted, got:\n%s", content)
    }
    if strings.Contains(content, `"state_dir"`) {
        t.Fatalf("expected state_dir omitted, got:\n%s", content)
    }
    if strings.Contains(content, `"clone_backend"`) {
        t.Fatalf("expected clone_backend omitted, got:\n%s", content)
    }
}
```

**Step 2: Write a failing test to ensure existing config is not overwritten**

Add a test that seeds `.grove/config.json` with non-default values, calls `LoadOrInitMinimal`, and verifies `initialized=false` and seeded values remain.

```go
func TestLoadOrInitMinimal_DoesNotOverwriteExistingConfig(t *testing.T) {
    dir := t.TempDir()
    if err := os.MkdirAll(filepath.Join(dir, ".grove"), 0755); err != nil {
        t.Fatal(err)
    }
    seeded := []byte(`{"workspace_dir":"/tmp/custom","max_workspaces":3,"clone_backend":"image"}`)
    if err := os.WriteFile(filepath.Join(dir, ".grove", "config.json"), seeded, 0644); err != nil {
        t.Fatal(err)
    }

    cfg, initialized, err := config.LoadOrInitMinimal(dir)
    if err != nil {
        t.Fatal(err)
    }
    if initialized {
        t.Fatal("expected initialized=false when config already exists")
    }
    if cfg.WorkspaceDir != "/tmp/custom" {
        t.Fatalf("expected existing workspace_dir preserved, got %q", cfg.WorkspaceDir)
    }
    if cfg.MaxWorkspaces != 3 {
        t.Fatalf("expected existing max_workspaces preserved, got %d", cfg.MaxWorkspaces)
    }
    if cfg.CloneBackend != "image" {
        t.Fatalf("expected existing clone_backend preserved, got %q", cfg.CloneBackend)
    }
}
```

**Step 3: Run targeted tests and confirm failure**

Run: `go test ./internal/config -run "LoadOrInitMinimal" -v`
Expected: compile/test failure because `LoadOrInitMinimal` does not exist yet.

**Step 4: Commit tests**

```bash
git add internal/config/config_test.go
git commit -m "test: add coverage for config auto-init persistence"
```

---

### Task 2: Implement config minimal auto-init helper

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Step 1: Implement `LoadOrInitMinimal`**

Add a new helper to `internal/config/config.go`:

```go
// LoadOrInitMinimal loads config if present. When missing, it persists
// minimal default config and returns initialized=true.
func LoadOrInitMinimal(repoRoot string) (*Config, bool, error) {
    path := filepath.Join(repoRoot, GroveDirName, ConfigFile)
    if _, err := os.Stat(path); err == nil {
        cfg, err := Load(repoRoot)
        return cfg, false, err
    } else if !errors.Is(err, os.ErrNotExist) {
        return nil, false, err
    }

    projectName := filepath.Base(repoRoot)
    cfg := DefaultConfig(projectName)
    cfg.CloneBackend = "cp"

    if err := EnsureMinimalGroveDir(repoRoot); err != nil {
        return nil, false, err
    }
    if err := Save(repoRoot, cfg); err != nil {
        return nil, false, err
    }
    return cfg, true, nil
}
```

Notes for implementation:
- Reuse existing `Save()` behavior so persisted config stays minimal.
- Keep `LoadOrDefault` unchanged for backward compatibility with existing call sites.

**Step 2: Run config package tests**

Run: `go test ./internal/config -v`
Expected: PASS.

**Step 3: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add minimal config auto-init helper"
```

---

### Task 3: Wire `grove create` to auto-init and emit first-run note

**Files:**
- Modify: `cmd/grove/create.go`
- Modify: `test/e2e_test.go`

**Step 1: Add failing e2e expectations for first-run create behavior**

Update `TestCreateWithoutConfig` in `test/e2e_test.go`:
- it should now expect `.grove/config.json` to exist after first `create`
- it should verify config content includes `workspace_dir`
- it should verify defaults are omitted (`max_workspaces`, `state_dir`, `clone_backend`)

Replace old assertion:

```go
if _, err := os.Stat(filepath.Join(repo, ".grove", "config.json")); err == nil {
    t.Error("config.json should not exist without explicit init")
}
```

with new assertions that require config.json to exist and be minimal.

**Step 2: Run e2e target and verify failure**

Run: `go test ./test -run TestCreateWithoutConfig -v`
Expected: FAIL due to current behavior not writing config.json.

**Step 3: Update create command to use new helper**

In `cmd/grove/create.go`:
- replace `config.LoadOrDefault(goldenRoot)` with `config.LoadOrInitMinimal(goldenRoot)`
- capture `initialized bool`
- keep existing `.grove` ensure call (safe/idempotent)
- after successful create output, print one-time note when initialized

Example integration sketch:

```go
cfg, initialized, err := config.LoadOrInitMinimal(goldenRoot)
if err != nil {
    return err
}
...
if jsonOut {
    ...
} else {
    ...
}
if initialized {
    fmt.Fprintln(os.Stderr, "Initialized Grove config at .grove/config.json using defaults. Run `grove config` to customize.")
}
```

**Step 4: Re-run e2e target**

Run: `go test ./test -run TestCreateWithoutConfig -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/grove/create.go test/e2e_test.go
git commit -m "feat: auto-initialize config on first create"
```

---

### Task 4: Add targeted command-level regression test for malformed config safety

**Files:**
- Modify: `test/e2e_test.go`

**Step 1: Add failing test for malformed existing config**

Add a new e2e test:
- seed repo with `.grove/config.json` containing invalid JSON
- run `grove create`
- assert failure message contains `invalid config`
- assert file remains untouched

```go
func TestCreateWithMalformedConfigFails(t *testing.T) {
    if runtime.GOOS != "darwin" {
        t.Skip("APFS tests only run on macOS")
    }
    binary := buildGrove(t)
    repo := setupTestRepo(t)

    if err := os.MkdirAll(filepath.Join(repo, ".grove"), 0755); err != nil {
        t.Fatal(err)
    }
    badPath := filepath.Join(repo, ".grove", "config.json")
    if err := os.WriteFile(badPath, []byte("{"), 0644); err != nil {
        t.Fatal(err)
    }

    errOut := groveExpectErr(t, binary, repo, "create")
    if !strings.Contains(errOut, "invalid config") {
        t.Fatalf("expected invalid config error, got: %s", errOut)
    }
}
```

**Step 2: Run target test**

Run: `go test ./test -run TestCreateWithMalformedConfigFails -v`
Expected: PASS.

**Step 3: Commit**

```bash
git add test/e2e_test.go
git commit -m "test: preserve malformed-config failure behavior in create"
```

---

### Task 5: Update user docs for first-run behavior

**Files:**
- Modify: `README.md`

**Step 1: Update Quick Start and create command docs**

Adjust docs to reflect:
- `grove create` can be first command in a git repo
- first create writes minimal `.grove/config.json`
- `grove config` remains customization path

Suggested edits:
- In Basic Workflow, add a config-less variant snippet before explicit `grove config`
- In `grove create` section, add a short note about automatic first-run initialization

**Step 2: Validate docs references are consistent**

Run: `go test ./... -run TestCreateWithoutConfig -v`
Expected: PASS for the selected regression signal.

**Step 3: Commit docs**

```bash
git add README.md
git commit -m "docs: document first-run auto-init in create"
```

---

### Task 6: Full verification pass

**Files:**
- Verify only (no new files expected)

**Step 1: Run focused suites first**

Run: `go test ./internal/config ./cmd/grove ./test -v`
Expected: PASS.

**Step 2: Run full repository tests**

Run: `go test ./... -v`
Expected: PASS.

**Step 3: Final commit check**

Run: `git status`
Expected: clean working tree.
