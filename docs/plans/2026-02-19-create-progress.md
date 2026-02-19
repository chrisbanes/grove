# Create Progress Flag Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add an opt-in `grove create --progress` mode that shows determinate clone progress while preserving existing output contracts, especially `--json`.

**Architecture:** Add a progress-capable clone path for APFS that emits clone events while running `cp -c -R -v`. Keep rendering in the CLI layer (`cmd/grove`) so internal packages remain UI-agnostic. Wire `create` to choose progress-aware cloning only when `--progress` is set and keep `stdout` machine-safe by sending progress to `stderr`.

**Tech Stack:** Go 1.25, Cobra CLI, existing `internal/clone` and `test/e2e_test.go` harness.

---

### Task 1: Add Clone Progress Parsing and Mapping Unit Tests

**Files:**
- Create: `internal/clone/progress_test.go`
- Modify: `internal/clone/apfs.go`

**Step 1: Write the failing tests**

Add tests covering:
- parsing a verbose cp line (`src/file -> dst/file`) as one progress increment,
- ignoring malformed/noise lines,
- mapping copied/total to bounded range (`5-95`),
- clamping when copied exceeds total.

```go
func TestMapClonePercent(t *testing.T) {
	got := mapClonePercent(50, 100, 5, 95)
	if got != 50 {
		t.Fatalf("expected 50, got %d", got)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/clone -run 'TestMapClonePercent|TestParseVerboseLine' -v`
Expected: FAIL with undefined functions/types.

**Step 3: Write minimal implementation**

In `internal/clone/apfs.go`, add small helper functions used by tests:
- `parseCPVerboseLine(line string) bool`
- `mapClonePercent(copied, total, min, max int) int`

**Step 4: Run test to verify it passes**

Run: `go test ./internal/clone -run 'TestMapClonePercent|TestParseVerboseLine' -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/clone/progress_test.go internal/clone/apfs.go
git commit -m "test: cover clone progress parsing and percent mapping"
```

### Task 2: Add Progress-Capable Clone Interface and APFS Implementation

**Files:**
- Modify: `internal/clone/clone.go`
- Modify: `internal/clone/apfs.go`

**Step 1: Write the failing tests**

Extend `internal/clone/clone_test.go` with a test that type-asserts the cloner to progress capability on macOS/APFS.

```go
func TestNewCloner_ImplementsProgressCloner(t *testing.T) {
	c, _ := clone.NewCloner(t.TempDir())
	if _, ok := c.(clone.ProgressCloner); !ok {
		t.Fatal("expected ProgressCloner")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/clone -run TestNewCloner_ImplementsProgressCloner -v`
Expected: FAIL (missing `ProgressCloner` type).

**Step 3: Write minimal implementation**

In `internal/clone/clone.go`, add:

```go
type ProgressEvent struct {
	Copied int
	Total  int
	Phase  string
}

type ProgressFunc func(ProgressEvent)

type ProgressCloner interface {
	CloneWithProgress(src, dst string, onProgress ProgressFunc) error
}
```

In `internal/clone/apfs.go`, implement `CloneWithProgress`:
- count source entries,
- emit `"scan"` event with total,
- run `cp -c -R -v`,
- read stdout/stderr stream, parse lines, emit `"clone"` events,
- return existing error style on failure.

Keep existing `Clone` method unchanged.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/clone -v`
Expected: PASS on macOS/APFS.

**Step 5: Commit**

```bash
git add internal/clone/clone.go internal/clone/apfs.go internal/clone/clone_test.go
git commit -m "feat: add progress-capable clone interface for APFS"
```

### Task 3: Add Create Command Progress Renderer and Flag Wiring

**Files:**
- Create: `cmd/grove/progress.go`
- Modify: `cmd/grove/create.go`
- Modify: `cmd/grove/main.go`

**Step 1: Write the failing tests**

Create renderer-focused tests in `cmd/grove/progress_test.go`:
- percent never decreases,
- percent bounded [0,100],
- stage conversion (`scan/clone/hook/checkout`) to labels,
- non-TTY formatting uses line-based output.

```go
func TestProgressState_NonDecreasing(t *testing.T) {
	s := newProgressState(5, 95)
	s.updateClone(10, 100)
	first := s.percent
	s.updateClone(5, 100)
	if s.percent < first {
		t.Fatal("percent regressed")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/grove -run TestProgressState_NonDecreasing -v`
Expected: FAIL (missing renderer/state).

**Step 3: Write minimal implementation**

In `cmd/grove/progress.go`:
- implement lightweight renderer writing to `io.Writer` (`stderr`),
- implement band mapping (`0-5`, `5-95`, `95-99`, `99-100`),
- implement TTY and non-TTY formatting.

In `cmd/grove/create.go`:
- add `--progress` flag,
- when enabled:
  - run preflight progress updates,
  - use `ProgressCloner` if available via type assertion,
  - otherwise fallback to stage-only progress updates,
  - update hook/checkout/done progress stages.

Ensure:
- progress writes only to `os.Stderr`,
- final non-JSON and JSON outputs remain on `stdout`.

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/grove -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/grove/progress.go cmd/grove/progress_test.go cmd/grove/create.go
git commit -m "feat: add create --progress renderer and wiring"
```

### Task 4: Lock Output Contract with E2E Tests

**Files:**
- Modify: `test/e2e_test.go`

**Step 1: Write the failing tests**

Add e2e cases:
- `create --progress --json`:
  - parse JSON from `stdout`,
  - assert progress text appears in `stderr`.
- `create --json` (without progress):
  - assert `stderr` has no progress noise.

Use a helper that captures stdout and stderr separately (new helper if needed in this file).

**Step 2: Run test to verify it fails**

Run: `go test ./test -run 'TestCreateProgressJsonContract|TestCreateJsonNoProgressNoise' -v`
Expected: FAIL before helper/wiring updates.

**Step 3: Write minimal implementation**

Update test helpers in `test/e2e_test.go`:
- keep current `grove(...)` helper for existing tests,
- add `groveOutErr(...) (stdout string, stderr string)` for new assertions.

Then complete the new tests.

**Step 4: Run test to verify it passes**

Run: `go test ./test -run 'TestCreateProgressJsonContract|TestCreateJsonNoProgressNoise' -v`
Expected: PASS on macOS/APFS.

**Step 5: Commit**

```bash
git add test/e2e_test.go
git commit -m "test: verify create progress output contract for json and stderr"
```

### Task 5: Document the New Flag

**Files:**
- Modify: `README.md`
- Modify: `docs/DESIGN.md`

**Step 1: Write the failing doc checks (manual)**

Create a checklist:
- `create` command table includes `--progress`,
- example shows optional progress behavior,
- `--json` contract mentions progress on `stderr`.

**Step 2: Run validation**

Run: `rg -n \"--progress|stderr|create\" README.md docs/DESIGN.md`
Expected: missing entries before doc updates.

**Step 3: Write minimal documentation**

Update command docs and examples in:
- `README.md` (`grove create` section),
- `docs/DESIGN.md` (`grove create` behavior and output contract).

**Step 4: Run validation**

Run: `rg -n \"--progress|stderr|create\" README.md docs/DESIGN.md`
Expected: new entries present and coherent.

**Step 5: Commit**

```bash
git add README.md docs/DESIGN.md
git commit -m "docs: document create progress flag and output behavior"
```

### Task 6: Full Verification and Final Commit

**Files:**
- Modify: `cmd/grove/create.go` (only if final polish needed)
- Modify: `internal/clone/apfs.go` (only if final polish needed)
- Modify: `test/e2e_test.go` (only if final polish needed)

**Step 1: Run targeted tests**

Run: `go test ./internal/clone ./cmd/grove ./test -v`
Expected: PASS on macOS/APFS.

**Step 2: Run full suite**

Run: `go test ./...`
Expected: PASS.

**Step 3: Fix minimal regressions (if any)**

Apply smallest possible fixes, rerun only affected tests, then rerun `go test ./...`.

**Step 4: Final commit (squash/finalize as desired by branch policy)**

```bash
git add -A
git commit -m "feat: add opt-in create progress with json-safe output"
```

**Step 5: Capture proof in PR body/checklist**

Include:
- commands run,
- pass/fail results,
- note that `--json` output remains parseable from `stdout`.
