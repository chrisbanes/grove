# Workspace Exclude Globs Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add config-only `exclude_globs` so `grove create` can skip selected Git-ignored paths for faster workspace creation, while failing if excludes touch non-ignored files.

**Architecture:** Keep the current clone path unchanged when excludes are empty. When excludes are configured, validate globs against the golden copy and Git ignore rules, warn on unmatched globs, then run a filtered CoW clone path that skips validated excluded subtrees. Keep validation/reporting in the CLI layer and clone mechanics in `internal/clone`/`internal/workspace`.

**Tech Stack:** Go 1.25, Cobra CLI (`cmd/grove`), internal packages (`config`, `git`, `workspace`, `clone`), Go test + existing e2e harness.

---

### Task 1: Add `exclude_globs` Config Support

**Skill:** @test-driven-development

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Step 1: Write the failing test**

Add config tests that assert:
- `exclude_globs` survives `Save` + `Load`,
- missing `exclude_globs` loads as empty slice behavior.

```go
func TestSaveAndLoad_ExcludeGlobs(t *testing.T) {
	cfg := &config.Config{
		WorkspaceDir:  "/tmp/grove/test",
		MaxWorkspaces: 5,
		ExcludeGlobs:  []string{"node_modules/**", ".gradle/caches/**"},
	}
	// save + load, then assert ExcludeGlobs length/content
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config -run ExcludeGlobs -v`  
Expected: FAIL because `Config` does not define `ExcludeGlobs`.

**Step 3: Write minimal implementation**

Add field to `Config`:

```go
type Config struct {
	WarmupCommand string   `json:"warmup_command,omitempty"`
	WorkspaceDir  string   `json:"workspace_dir"`
	MaxWorkspaces int      `json:"max_workspaces"`
	ExcludeGlobs  []string `json:"exclude_globs,omitempty"`
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config -run ExcludeGlobs -v`  
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add exclude_globs to grove config"
```

### Task 2: Add Git Ignore Query Helper

**Skill:** @test-driven-development

**Files:**
- Modify: `internal/git/git.go`
- Modify: `internal/git/git_test.go`

**Step 1: Write the failing test**

Add tests with a repo containing tracked and ignored files:
- ignored file returns `true`,
- tracked file returns `false`.

```go
func TestIsIgnored(t *testing.T) {
	repo := setupRepo(t)
	_ = os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("build/\n"), 0644)
	_ = os.WriteFile(filepath.Join(repo, "build/out.bin"), []byte("x"), 0644)
	ignored, err := git.IsIgnored(repo, "build/out.bin")
	if err != nil || !ignored {
		t.Fatalf("expected ignored=true, err=%v", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/git -run IsIgnored -v`  
Expected: FAIL with undefined `git.IsIgnored`.

**Step 3: Write minimal implementation**

In `internal/git/git.go`, add:

```go
func IsIgnored(path, rel string) (bool, error) {
	cmd := exec.Command("git", "-C", path, "check-ignore", "--quiet", "--", rel)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("git check-ignore: %w", err)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/git -run IsIgnored -v`  
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/git/git.go internal/git/git_test.go
git commit -m "feat: add git ignored-path helper"
```

### Task 3: Implement Exclude Glob Validation Rules

**Skill:** @test-driven-development

**Files:**
- Create: `internal/workspace/exclude.go`
- Create: `internal/workspace/exclude_test.go`

**Step 1: Write the failing tests**

Cover:
- valid ignored matches pass,
- non-ignored match fails,
- protected roots (`.git`, `.grove`) fail,
- absolute/`..` globs fail,
- unmatched globs are returned as warnings, not errors.

```go
func TestValidateExcludeGlobs_UnmatchedWarning(t *testing.T) {
	res, err := workspace.ValidateExcludeGlobs(repo, []string{"cache/**"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected warning for unmatched glob")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/workspace -run ValidateExcludeGlobs -v`  
Expected: FAIL because validator does not exist.

**Step 3: Write minimal implementation**

Create validator API:

```go
type ExcludeValidation struct {
	ExcludedRelPaths map[string]struct{}
	Warnings         []string
}

func ValidateExcludeGlobs(goldenRoot string, globs []string) (*ExcludeValidation, error)
```

Implementation details:
- clean and normalize glob inputs,
- reject unsafe globs,
- walk repo tree to match paths,
- for each match call `git.IsIgnored(goldenRoot, rel)`,
- build excluded path set for clone step.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/workspace -run ValidateExcludeGlobs -v`  
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/workspace/exclude.go internal/workspace/exclude_test.go
git commit -m "feat: validate exclude globs against git ignore rules"
```

### Task 4: Add Filtered CoW Clone Capability

**Skill:** @test-driven-development

**Files:**
- Modify: `internal/clone/clone.go`
- Modify: `internal/clone/apfs.go`
- Create: `internal/clone/filtered_clone_test.go`

**Step 1: Write the failing test**

Add APFS test that excludes a subtree and asserts:
- excluded files are absent in destination,
- included files still exist.

```go
func TestCloneFiltered_ExcludesPaths(t *testing.T) {
	// src has keep.txt and node_modules/pkg/index.js
	// run filtered clone excluding node_modules/**
	// assert keep.txt exists, excluded file does not
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/clone -run CloneFiltered -v`  
Expected: FAIL with missing filtered clone API.

**Step 3: Write minimal implementation**

In `internal/clone/clone.go`, add:

```go
type PathExcludeFunc func(rel string, isDir bool) bool

type FilteredCloner interface {
	CloneFiltered(src, dst string, exclude PathExcludeFunc, onProgress ProgressFunc) error
}
```

In `internal/clone/apfs.go`, implement `CloneFiltered`:
- create destination root,
- walk source with `filepath.WalkDir`,
- skip excluded directories with `filepath.SkipDir`,
- CoW copy files/dirs/symlinks with `cp -c` commands,
- emit `"clone"` progress events.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/clone -run CloneFiltered -v`  
Expected: PASS on macOS/APFS.

**Step 5: Commit**

```bash
git add internal/clone/clone.go internal/clone/apfs.go internal/clone/filtered_clone_test.go
git commit -m "feat: add filtered APFS clone path"
```

### Task 5: Wire Validation + Warnings + Filtered Clone into `grove create`

**Skill:** @test-driven-development

**Files:**
- Modify: `internal/workspace/workspace.go`
- Modify: `cmd/grove/create.go`
- Modify: `cmd/grove/progress_test.go`

**Step 1: Write the failing tests**

Add tests for:
- warning emission for unmatched globs,
- progress phases include `validate-excludes` and `filtered-clone`,
- create path uses filtered clone when excludes are configured.

```go
func TestProgressRenderer_FilteredClonePhases(t *testing.T) {
	// render updates for validate-excludes and filtered-clone
	// assert output includes both phase labels
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/grove -run 'Filtered|Exclude' -v`  
Expected: FAIL before wiring exists.

**Step 3: Write minimal implementation**

In `cmd/grove/create.go`:
- run `workspace.ValidateExcludeGlobs(goldenRoot, cfg.ExcludeGlobs)` before clone,
- print validation warnings to `stderr`,
- set progress phases (`validate-excludes`, `filtered-clone`),
- pass validated excluded paths into workspace create options.

In `internal/workspace/workspace.go`:
- extend `CreateOpts` with excluded paths + optional matcher,
- in clone step:
  - if no excludes: existing clone path unchanged,
  - if excludes: require `clone.FilteredCloner` and call `CloneFiltered`.

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/grove ./internal/workspace -run 'Filtered|Exclude|Create' -v`  
Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/grove/create.go cmd/grove/progress_test.go internal/workspace/workspace.go
git commit -m "feat: wire exclude validation and filtered clone into create"
```

### Task 6: Add E2E Coverage for Exclude Behavior

**Skill:** @test-driven-development

**Files:**
- Modify: `test/e2e_test.go`

**Step 1: Write the failing tests**

Add e2e cases:
- create succeeds and excluded ignored files are missing in workspace,
- create fails when glob matches tracked file,
- unmatched glob logs warning on `stderr` and still succeeds.

```go
func TestCreate_ExcludeGlobWarnsWhenUnmatched(t *testing.T) {
	stdout, stderr := groveOutErr(t, binary, repo, "create", "--json")
	if !strings.Contains(stderr, "Warning: exclude glob matched nothing") {
		t.Fatalf("expected warning, stderr=%s", stderr)
	}
	_ = stdout // still valid JSON
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./test -run 'ExcludeGlob|FilteredClone' -v`  
Expected: FAIL before feature wiring is complete.

**Step 3: Write minimal implementation support in fixtures**

In `setupTestRepo`, create:
- tracked file: `src/tracked.txt`,
- ignored trees in `.gitignore` and files under `build/`, `node_modules/`.

Then complete new tests using `groveOutErr` and `groveExpectErr`.

**Step 4: Run test to verify it passes**

Run: `go test ./test -run 'ExcludeGlob|FilteredClone' -v`  
Expected: PASS on macOS/APFS.

**Step 5: Commit**

```bash
git add test/e2e_test.go
git commit -m "test: cover create exclude-glob validation and warnings"
```

### Task 7: Documentation and Final Verification

**Skill:** @verification-before-completion

**Files:**
- Modify: `README.md`
- Modify: `docs/DESIGN.md`

**Step 1: Write doc updates**

Document:
- new config field `exclude_globs`,
- ignored-only subset rule,
- unmatched-glob warning behavior,
- filtered clone speed intent.

**Step 2: Run docs validation**

Run: `rg -n "exclude_globs|ignored|warning|filtered-clone" README.md docs/DESIGN.md`  
Expected: new entries found.

**Step 3: Run targeted verification**

Run: `go test ./internal/config ./internal/git ./internal/workspace ./internal/clone ./cmd/grove ./test -v`  
Expected: PASS (APFS-gated tests may skip outside macOS).

**Step 4: Run full suite**

Run: `go test ./...`  
Expected: PASS.

**Step 5: Final commit**

```bash
git add README.md docs/DESIGN.md
git commit -m "docs: document exclude globs and ignored-only behavior"
```

