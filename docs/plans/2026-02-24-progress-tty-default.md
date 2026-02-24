# Default `--progress` to true for interactive shells

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Auto-enable progress output when stderr is a TTY, so interactive users see progress by default without passing `--progress`.

**Architecture:** Add a `resolveProgress(cmd)` helper in `progress.go` that checks whether the user explicitly set `--progress`; if not, falls back to `isTerminalFile(os.Stderr)`. Each command calls this helper instead of `cmd.Flags().GetBool("progress")`.

**Tech Stack:** Go, Cobra (`cmd.Flags().Changed`)

---

### Task 1: Write failing test for `resolveProgress`

**Files:**
- Modify: `cmd/grove/progress_test.go`

**Step 1: Write the failing tests**

Add tests that exercise `resolveProgress` with a Cobra command whose `--progress` flag is/isn't explicitly set:

```go
func TestResolveProgress_DefaultsTrueWhenStderrIsTTY(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("progress", false, "")
	// Flag not changed — should fall back to isTerminalFile(os.Stderr).
	// In test, stderr is typically not a TTY, so expect false.
	got := resolveProgress(cmd)
	if got {
		t.Fatal("expected false when stderr is not a TTY and flag not set")
	}
}

func TestResolveProgress_RespectsExplicitTrue(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("progress", false, "")
	_ = cmd.Flags().Set("progress", "true")
	got := resolveProgress(cmd)
	if !got {
		t.Fatal("expected true when --progress explicitly set")
	}
}

func TestResolveProgress_RespectsExplicitFalse(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("progress", false, "")
	_ = cmd.Flags().Set("progress", "false")
	got := resolveProgress(cmd)
	if got {
		t.Fatal("expected false when --progress=false explicitly set")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./cmd/grove/ -run TestResolveProgress -v`
Expected: FAIL — `resolveProgress` undefined.

**Step 3: Commit**

```bash
git add cmd/grove/progress_test.go
git commit -m "test: add failing tests for resolveProgress helper"
```

---

### Task 2: Implement `resolveProgress`

**Files:**
- Modify: `cmd/grove/progress.go`

**Step 1: Add the helper function**

Add at the end of `progress.go`, before `isTerminalFile`:

```go
// resolveProgress returns whether progress output should be enabled.
// If the user explicitly set --progress or --progress=false, that value wins.
// Otherwise, progress is enabled when stderr is a TTY.
func resolveProgress(cmd *cobra.Command) bool {
	if cmd.Flags().Changed("progress") {
		v, _ := cmd.Flags().GetBool("progress")
		return v
	}
	return isTerminalFile(os.Stderr)
}
```

**Step 2: Run tests to verify they pass**

Run: `go test ./cmd/grove/ -run TestResolveProgress -v`
Expected: PASS (all three tests).

**Step 3: Commit**

```bash
git add cmd/grove/progress.go
git commit -m "feat: add resolveProgress helper for TTY-aware default"
```

---

### Task 3: Wire up `resolveProgress` in all four commands

**Files:**
- Modify: `cmd/grove/create.go:28`
- Modify: `cmd/grove/init.go:26`
- Modify: `cmd/grove/update.go:20`
- Modify: `cmd/grove/migrate.go:20`

**Step 1: Replace `GetBool` calls with `resolveProgress`**

In each file, change:
```go
progressEnabled, _ := cmd.Flags().GetBool("progress")
```
to:
```go
progressEnabled := resolveProgress(cmd)
```

**Step 2: Update flag help text**

In each file's `init()` function, change the flag description to indicate the auto-detect behavior:

- `create.go:183`: `"Show progress output for long-running create operations"` → `"Show progress output (default: auto-detect TTY)"`
- `init.go:241`: `"Show progress output during image backend initialization"` → `"Show progress output (default: auto-detect TTY)"`
- `update.go:93`: `"Show progress output during image backend refresh"` → `"Show progress output (default: auto-detect TTY)"`
- `migrate.go:135`: `"Show progress output during image backend initialization"` → `"Show progress output (default: auto-detect TTY)"`

**Step 3: Run full test suite**

Run: `go test ./cmd/grove/ -v`
Expected: PASS — all existing tests plus the new `resolveProgress` tests.

**Step 4: Run build**

Run: `go build ./cmd/grove/`
Expected: Clean build, no errors.

**Step 5: Commit**

```bash
git add cmd/grove/create.go cmd/grove/init.go cmd/grove/update.go cmd/grove/migrate.go
git commit -m "feat: default --progress to true for interactive shells

Detect stderr TTY to auto-enable progress output. Users in interactive
terminals now see progress by default. CI and piped output stays quiet.
Explicit --progress or --progress=false always wins."
```
