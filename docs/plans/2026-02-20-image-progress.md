# Image Backend Progress Reporting — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show real-time rsync progress during image backend initialization, migration, and update.

**Architecture:** Add a `Stream` method to the `Runner` interface for line-by-line command output. Thread an optional `onProgress` callback through `InitBase` and `RefreshBase`. Parse `rsync --info=progress2` output to drive the existing `progressRenderer`. Add `--progress` to `init`, `migrate`, and `update` commands.

**Tech Stack:** Go, `bufio.Scanner` for stream parsing, `ttyprogress` for TTY bars, `regexp` for rsync output parsing.

---

### Task 1: Add `Stream` to `Runner` interface and `execRunner`

**Files:**
- Modify: `internal/image/commands.go:10-13` (Runner interface)
- Modify: `internal/image/commands.go:15-19` (execRunner)

**Step 1: Write the failing test**

Add to `internal/image/commands_test.go`:

```go
func TestExecRunner_StreamCallsOnLine(t *testing.T) {
	r := execRunner{}
	var lines []string
	err := r.Stream("echo", []string{"hello"}, func(line string) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if len(lines) == 0 {
		t.Fatal("expected at least one line from echo")
	}
	if lines[0] != "hello" {
		t.Fatalf("expected 'hello', got %q", lines[0])
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/f94f-for-the-new-imag/grove && go test ./internal/image/ -run TestExecRunner_StreamCallsOnLine -v`
Expected: FAIL — `execRunner` does not implement `Stream`.

**Step 3: Add `Stream` to `Runner` interface and implement on `execRunner`**

In `internal/image/commands.go`, update the `Runner` interface:

```go
type Runner interface {
	CombinedOutput(name string, args ...string) ([]byte, error)
	Stream(name string, args []string, onLine func(string)) error
}
```

Add the `execRunner` implementation:

```go
func (execRunner) Stream(name string, args []string, onLine func(string)) error {
	cmd := exec.Command(name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Split(scanCRLF)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			onLine(line)
		}
	}
	return cmd.Wait()
}
```

Add the custom split function that splits on both `\r` and `\n` (needed for rsync `--info=progress2` which uses `\r` to overwrite lines):

```go
func scanCRLF(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}
```

Add `"bufio"` to the imports.

**Step 4: Update `fakeRunner` in test file**

Add `Stream` to `fakeRunner` in `internal/image/commands_test.go`:

```go
type streamCall struct {
	name string
	args []string
}

type fakeRunner struct {
	calls       []runnerCall
	outputs     [][]byte
	errs        []error
	streamCalls []streamCall
	streamLines []string
	streamErr   error
}

func (f *fakeRunner) Stream(name string, args []string, onLine func(string)) error {
	f.streamCalls = append(f.streamCalls, streamCall{name: name, args: append([]string(nil), args...)})
	for _, line := range f.streamLines {
		onLine(line)
	}
	return f.streamErr
}
```

**Step 5: Run test to verify it passes**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/f94f-for-the-new-imag/grove && go test ./internal/image/ -run TestExecRunner_StreamCallsOnLine -v`
Expected: PASS

**Step 6: Run all existing tests to verify nothing broke**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/f94f-for-the-new-imag/grove && go test ./...`
Expected: All PASS

**Step 7: Commit**

```bash
git add internal/image/commands.go internal/image/commands_test.go
git commit -m "feat(image): add Stream method to Runner interface for line-by-line output"
```

---

### Task 2: Add `SyncBaseWithProgress` function

**Files:**
- Modify: `internal/image/commands.go` (add new function)
- Modify: `internal/image/commands_test.go` (add tests)

**Step 1: Write the failing test for rsync percent parsing**

Add to `internal/image/commands_test.go`:

```go
func TestParseRsyncPercent(t *testing.T) {
	tests := []struct {
		line string
		want int
		ok   bool
	}{
		{"    458,588,160   6%  109.38MB/s    0:01:02", 6, true},
		{"  1,234,567,890  99%   50.00MB/s    0:00:01", 99, true},
		{"              0   0%    0.00kB/s    0:00:00", 0, true},
		{"  1,234,567,890 100%   50.00MB/s    0:00:01 (xfr#1, to-chk=0/100)", 100, true},
		{"sending incremental file list", -1, false},
		{"", -1, false},
	}
	for _, tt := range tests {
		got, ok := parseRsyncPercent(tt.line)
		if ok != tt.ok {
			t.Errorf("parseRsyncPercent(%q) ok = %v, want %v", tt.line, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Errorf("parseRsyncPercent(%q) = %d, want %d", tt.line, got, tt.want)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/f94f-for-the-new-imag/grove && go test ./internal/image/ -run TestParseRsyncPercent -v`
Expected: FAIL — `parseRsyncPercent` undefined.

**Step 3: Implement `parseRsyncPercent`**

Add to `internal/image/commands.go`:

```go
var rsyncPercentPattern = regexp.MustCompile(`\s+(\d+)%\s`)

func parseRsyncPercent(line string) (int, bool) {
	m := rsyncPercentPattern.FindStringSubmatch(line)
	if m == nil {
		return -1, false
	}
	var pct int
	fmt.Sscanf(m[1], "%d", &pct)
	return pct, true
}
```

Add `"regexp"` to imports (already present via `dictPattern`).

**Step 4: Run test to verify it passes**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/f94f-for-the-new-imag/grove && go test ./internal/image/ -run TestParseRsyncPercent -v`
Expected: PASS

**Step 5: Write the failing test for `SyncBaseWithProgress`**

Add to `internal/image/commands_test.go`:

```go
func TestSyncBaseWithProgress_CallsRsyncWithProgressFlags(t *testing.T) {
	r := &fakeRunner{
		streamLines: []string{
			"sending incremental file list",
			"    458,588,160   6%  109.38MB/s    0:01:02",
			"  4,585,881,600  60%  109.38MB/s    0:01:02",
			"  7,643,136,000 100%  109.38MB/s    0:01:02 (xfr#1, to-chk=0/100)",
		},
	}

	var percents []int
	onProgress := func(pct int) {
		percents = append(percents, pct)
	}

	if err := SyncBaseWithProgress(r, "/src", "/dst", onProgress); err != nil {
		t.Fatalf("SyncBaseWithProgress() error = %v", err)
	}

	if len(r.streamCalls) != 1 {
		t.Fatalf("expected 1 stream call, got %d", len(r.streamCalls))
	}
	call := r.streamCalls[0]
	if call.name != "rsync" {
		t.Fatalf("expected rsync, got %q", call.name)
	}
	argsStr := strings.Join(call.args, " ")
	if !strings.Contains(argsStr, "--info=progress2") {
		t.Fatalf("expected --info=progress2 in args, got %v", call.args)
	}
	if !strings.Contains(argsStr, "--no-inc-recursive") {
		t.Fatalf("expected --no-inc-recursive in args, got %v", call.args)
	}

	if len(percents) != 3 {
		t.Fatalf("expected 3 progress callbacks, got %d: %v", len(percents), percents)
	}
	if percents[0] != 6 || percents[1] != 60 || percents[2] != 100 {
		t.Fatalf("unexpected percents: %v", percents)
	}
}

func TestSyncBaseWithProgress_NilRunnerUsesDefault(t *testing.T) {
	// Just verifying it doesn't panic when runner is nil — will fail
	// with a real rsync error since paths don't exist, which is fine.
	_ = SyncBaseWithProgress(nil, "/nonexistent/src", "/nonexistent/dst", nil)
}
```

**Step 6: Run test to verify it fails**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/f94f-for-the-new-imag/grove && go test ./internal/image/ -run TestSyncBaseWithProgress -v`
Expected: FAIL — `SyncBaseWithProgress` undefined.

**Step 7: Implement `SyncBaseWithProgress`**

Add to `internal/image/commands.go`:

```go
func SyncBaseWithProgress(r Runner, src, dst string, onPercent func(int)) error {
	if r == nil {
		r = execRunner{}
	}
	src = ensureTrailingSlash(src)
	dst = ensureTrailingSlash(dst)
	args := []string{
		"-a",
		"--delete",
		"--info=progress2",
		"--no-inc-recursive",
		"--exclude", ".grove/images/",
		"--exclude", ".grove/workspaces/",
		"--exclude", ".grove/shadows/",
		"--exclude", ".grove/mnt/",
		src,
		dst,
	}
	return r.Stream("rsync", args, func(line string) {
		if onPercent == nil {
			return
		}
		if pct, ok := parseRsyncPercent(line); ok {
			onPercent(pct)
		}
	})
}
```

**Step 8: Run tests to verify they pass**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/f94f-for-the-new-imag/grove && go test ./internal/image/ -run TestSyncBaseWithProgress -v`
Expected: PASS

**Step 9: Run all tests**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/f94f-for-the-new-imag/grove && go test ./...`
Expected: All PASS

**Step 10: Commit**

```bash
git add internal/image/commands.go internal/image/commands_test.go
git commit -m "feat(image): add SyncBaseWithProgress with rsync percent parsing"
```

---

### Task 3: Thread `onProgress` through `InitBase` and `RefreshBase`

**Files:**
- Modify: `internal/image/backend.go:9` (InitBase signature)
- Modify: `internal/image/backend.go:51` (RefreshBase signature)
- Modify: `internal/image/backend_test.go` (update calls)
- Modify: `internal/backend/image.go` (update callers)
- Modify: `internal/backend/backend.go:24` (Backend interface)
- Modify: `internal/backend/cp.go:33` (cpBackend.RefreshBase)
- Modify: `cmd/grove/init.go:98-101` (init caller)
- Modify: `cmd/grove/migrate.go:72` (migrate caller)
- Modify: `cmd/grove/update.go:58-63` (update caller)

**Step 1: Update `InitBase` signature and implementation**

In `internal/image/backend.go`, change:

```go
func InitBase(repoRoot string, runner Runner, baseSizeGB int, onProgress func(int, string)) (_ *State, err error) {
```

Before `CreateSparseBundle`, call the callback:

```go
	if onProgress != nil {
		onProgress(0, "creating base image")
	}
```

Replace the `SyncBase` call with a conditional:

```go
	if onProgress != nil {
		onProgress(5, "syncing golden copy")
		err = SyncBaseWithProgress(runner, repoRoot, vol.MountPoint, func(pct int) {
			onProgress(mapPercent(pct, 100, 5, 95), "syncing golden copy")
		})
	} else {
		err = SyncBase(runner, repoRoot, vol.MountPoint)
	}
	if err != nil {
		return nil, err
	}
```

Add `mapPercent` helper (reuse from `cmd/grove/progress.go` pattern — but simpler, in the image package):

```go
func mapPercent(value, total, min, max int) int {
	if total <= 0 {
		return min
	}
	if value > total {
		value = total
	}
	return min + (value*(max-min))/total
}
```

After saving state, call progress done:

```go
	if onProgress != nil {
		onProgress(100, "done")
	}
```

**Step 2: Update `RefreshBase` signature and implementation**

In `internal/image/backend.go`, change:

```go
func RefreshBase(repoRoot, goldenRoot string, runner Runner, commit string, onProgress func(int, string)) (_ *State, err error) {
```

Before `SyncBase`, call the callback:

```go
	if onProgress != nil {
		onProgress(5, "syncing golden copy")
	}
```

Replace the `SyncBase` call with a conditional (same pattern as above):

```go
	if onProgress != nil {
		err = SyncBaseWithProgress(runner, goldenRoot, vol.MountPoint, func(pct int) {
			onProgress(mapPercent(pct, 100, 5, 95), "syncing golden copy")
		})
	} else {
		err = SyncBase(runner, goldenRoot, vol.MountPoint)
	}
	if err != nil {
		return nil, err
	}
```

After saving state:

```go
	if onProgress != nil {
		onProgress(100, "done")
	}
```

**Step 3: Update tests**

In `internal/image/backend_test.go`, update `InitBase` calls to pass `nil`:

```go
// Change:
st, err := InitBase(repoRoot, r, 20)
// To:
st, err := InitBase(repoRoot, r, 20, nil)
```

Update `RefreshBase` calls:

```go
// Change:
_, err := RefreshBase(repoRoot, repoRoot, r, "abc1234")
// To:
_, err := RefreshBase(repoRoot, repoRoot, r, "abc1234", nil)
```

Both occurrences of `RefreshBase` in that file.

**Step 4: Update `Backend` interface**

In `internal/backend/backend.go`, change:

```go
RefreshBase(goldenRoot, commit string, onProgress func(int, string)) error
```

**Step 5: Update `imageBackend.RefreshBase`**

In `internal/backend/image.go`, change:

```go
func (imageBackend) RefreshBase(goldenRoot, commit string, onProgress func(int, string)) error {
	if _, err := image.RefreshBase(goldenRoot, goldenRoot, nil, commit, onProgress); err != nil {
		return fmt.Errorf("image backend refresh failed: %w", err)
	}
	return nil
}
```

**Step 6: Update `cpBackend.RefreshBase`**

In `internal/backend/cp.go`, change:

```go
func (cpBackend) RefreshBase(_ string, _ string, _ func(int, string)) error {
	return nil
}
```

**Step 7: Update CLI callers to pass `nil` for now**

In `cmd/grove/init.go:99`, change:

```go
// Change:
if _, err := image.InitBase(absPath, nil, imageSizeGB); err != nil {
// To:
if _, err := image.InitBase(absPath, nil, imageSizeGB, nil); err != nil {
```

In `cmd/grove/migrate.go:72`, change:

```go
// Change:
if _, err := image.InitBase(goldenRoot, nil, sizeGB); err != nil {
// To:
if _, err := image.InitBase(goldenRoot, nil, sizeGB, nil); err != nil {
```

In `cmd/grove/update.go:61`, change:

```go
// Change:
if err := backendImpl.RefreshBase(goldenRoot, commit); err != nil {
// To:
if err := backendImpl.RefreshBase(goldenRoot, commit, nil); err != nil {
```

**Step 8: Run all tests**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/f94f-for-the-new-imag/grove && go test ./...`
Expected: All PASS

**Step 9: Commit**

```bash
git add internal/image/backend.go internal/image/backend_test.go internal/backend/backend.go internal/backend/image.go internal/backend/cp.go cmd/grove/init.go cmd/grove/migrate.go cmd/grove/update.go
git commit -m "refactor: thread onProgress callback through InitBase, RefreshBase, and Backend interface"
```

---

### Task 4: Make progress label configurable

**Files:**
- Modify: `cmd/grove/progress.go:62-79` (newFancyProgress)

**Step 1: Write the failing test**

Add to `cmd/grove/progress_test.go`:

```go
func TestNewFancyProgress_UsesCustomLabel(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "progress-*.txt")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer f.Close()

	r := newProgressRenderer(f, true, "init")
	if r.ttyBar == nil {
		t.Fatal("expected ttyprogress bar")
	}
	r.Update(15, "syncing")
	r.Done()
}
```

**Step 2: Run test to verify it fails**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/f94f-for-the-new-imag/grove && go test ./cmd/grove/ -run TestNewFancyProgress_UsesCustomLabel -v`
Expected: FAIL — `newProgressRenderer` doesn't accept a label parameter.

**Step 3: Add label parameter**

In `cmd/grove/progress.go`, change `newProgressRenderer`:

```go
func newProgressRenderer(w io.Writer, tty bool, label string) *progressRenderer {
```

And pass it through:

```go
		ctx, bar, err := newFancyProgress(w, label)
```

Change `newFancyProgress`:

```go
func newFancyProgress(w io.Writer, label string) (ttyprogress.Context, ttyprogress.Bar, error) {
	ctx := ttyprogress.For(w)
	bar, err := ttyprogress.NewBar().
		SetPredefined(10).
		SetTotal(100).
		SetWidth(ttyprogress.ReserveTerminalSize(45)).
		PrependElapsed().
		PrependMessage(label).
		AppendCompleted().
		AppendMessage("phase:").
		AppendVariable(progressPhaseVariable).
		Add(ctx)
```

**Step 4: Update the existing caller in `create.go`**

In `cmd/grove/create.go:43`, change:

```go
// Change:
progress = newProgressRenderer(os.Stderr, isTerminalFile(os.Stderr))
// To:
progress = newProgressRenderer(os.Stderr, isTerminalFile(os.Stderr), "create")
```

**Step 5: Fix existing tests**

In `cmd/grove/progress_test.go`, update all `newProgressRenderer` calls:

```go
// Change:
r := newProgressRenderer(&buf, false)
// To:
r := newProgressRenderer(&buf, false, "create")

// Change:
r := newProgressRenderer(&buf, true)
// To:
r := newProgressRenderer(&buf, true, "create")

// Change:
r := newProgressRenderer(f, true)
// To:
r := newProgressRenderer(f, true, "create")
```

**Step 6: Run all tests**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/f94f-for-the-new-imag/grove && go test ./...`
Expected: All PASS

**Step 7: Commit**

```bash
git add cmd/grove/progress.go cmd/grove/progress_test.go cmd/grove/create.go
git commit -m "refactor: make progress bar label configurable"
```

---

### Task 5: Add `--progress` to `grove init`

**Files:**
- Modify: `cmd/grove/init.go`

**Step 1: Add the flag and progress wiring**

In `cmd/grove/init.go`, add to `init()`:

```go
initCmd.Flags().Bool("progress", false, "Show progress output during image backend initialization")
```

In the `RunE` function, add progress setup at the top (after the `func(cmd *cobra.Command, args []string) error {` line):

```go
		progressEnabled, _ := cmd.Flags().GetBool("progress")
		var progress *progressRenderer
		if progressEnabled {
			progress = newProgressRenderer(os.Stderr, isTerminalFile(os.Stderr), "init")
			defer progress.Done()
		}
```

Replace the `if cfg.CloneBackend == "image"` block (lines 96-102):

```go
		if cfg.CloneBackend == "image" {
			imageSizeGB, _ := cmd.Flags().GetInt("image-size-gb")
			if progress == nil {
				fmt.Println("Initializing image backend...")
			}
			var onProgress func(int, string)
			if progress != nil {
				onProgress = func(pct int, phase string) {
					progress.Update(pct, phase)
				}
			}
			if _, err := image.InitBase(absPath, nil, imageSizeGB, onProgress); err != nil {
				return fmt.Errorf("initializing image backend: %w", err)
			}
		}
```

**Step 2: Run all tests**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/f94f-for-the-new-imag/grove && go test ./...`
Expected: All PASS

**Step 3: Commit**

```bash
git add cmd/grove/init.go
git commit -m "feat: add --progress flag to grove init for image backend"
```

---

### Task 6: Add `--progress` to `grove migrate`

**Files:**
- Modify: `cmd/grove/migrate.go`

**Step 1: Add the flag and progress wiring**

In `cmd/grove/migrate.go`, add to `init()`:

```go
migrateCmd.Flags().Bool("progress", false, "Show progress output during image backend initialization")
```

In the `RunE` function, add progress setup at the top (after the `func(cmd *cobra.Command, args []string) error {` line):

```go
		progressEnabled, _ := cmd.Flags().GetBool("progress")
		var progress *progressRenderer
		if progressEnabled {
			progress = newProgressRenderer(os.Stderr, isTerminalFile(os.Stderr), "migrate")
			defer progress.Done()
		}
```

Replace the image init section in the `case "image":` block (lines 66-75):

```go
		case "image":
			if _, err := image.LoadState(goldenRoot); err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("loading image backend state: %w", err)
				}
				sizeGB, _ := cmd.Flags().GetInt("image-size-gb")
				if progress == nil {
					fmt.Println("Initializing image backend...")
				}
				var onProgress func(int, string)
				if progress != nil {
					onProgress = func(pct int, phase string) {
						progress.Update(pct, phase)
					}
				}
				if _, err := image.InitBase(goldenRoot, nil, sizeGB, onProgress); err != nil {
					return fmt.Errorf("initializing image backend: %w", err)
				}
			}
```

**Step 2: Run all tests**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/f94f-for-the-new-imag/grove && go test ./...`
Expected: All PASS

**Step 3: Commit**

```bash
git add cmd/grove/migrate.go
git commit -m "feat: add --progress flag to grove migrate for image backend"
```

---

### Task 7: Add `--progress` to `grove update`

**Files:**
- Modify: `cmd/grove/update.go`

**Step 1: Add the flag and progress wiring**

In `cmd/grove/update.go`, add to `init()`:

```go
updateCmd.Flags().Bool("progress", false, "Show progress output during image backend refresh")
```

In the `RunE` function, add progress setup at the top (after the `func(cmd *cobra.Command, args []string) error {` line):

```go
		progressEnabled, _ := cmd.Flags().GetBool("progress")
		var progress *progressRenderer
		if progressEnabled {
			progress = newProgressRenderer(os.Stderr, isTerminalFile(os.Stderr), "update")
			defer progress.Done()
		}
```

Replace the refresh block (lines 57-63):

```go
		commit, _ := gitpkg.CurrentCommit(goldenRoot)
		if backendImpl.Name() == "image" && progress == nil {
			fmt.Println("Refreshing image backend...")
		}
		var onProgress func(int, string)
		if progress != nil {
			onProgress = func(pct int, phase string) {
				progress.Update(pct, phase)
			}
		}
		if err := backendImpl.RefreshBase(goldenRoot, commit, onProgress); err != nil {
			return err
		}
```

**Step 2: Run all tests**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/f94f-for-the-new-imag/grove && go test ./...`
Expected: All PASS

**Step 3: Commit**

```bash
git add cmd/grove/update.go
git commit -m "feat: add --progress flag to grove update for image backend refresh"
```

---

### Task 8: Add integration test for `InitBase` with progress callback

**Files:**
- Modify: `internal/image/backend_test.go`

**Step 1: Write the test**

Add to `internal/image/backend_test.go`:

```go
func TestInitBase_CallsOnProgress(t *testing.T) {
	repoRoot := t.TempDir()

	r := &fakeRunner{
		outputs: [][]byte{
			nil, // CreateSparseBundle
			[]byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>system-entities</key>
  <array>
    <dict><key>dev-entry</key><string>/dev/disk9</string></dict>
    <dict><key>dev-entry</key><string>/dev/disk9s1</string><key>mount-point</key><string>` + filepath.Join(repoRoot, ".grove", "mnt", "base") + `</string></dict>
  </array>
</dict>
</plist>`), // Attach
		},
		streamLines: []string{
			"  4,585,881,600  50%  109.38MB/s    0:01:02",
			"  7,643,136,000 100%  109.38MB/s    0:01:02 (xfr#1, to-chk=0/100)",
		},
	}

	var phases []string
	var percents []int
	onProgress := func(pct int, phase string) {
		phases = append(phases, phase)
		percents = append(percents, pct)
	}

	st, err := InitBase(repoRoot, r, 20, onProgress)
	if err != nil {
		t.Fatalf("InitBase() error = %v", err)
	}
	if st.Backend != "image" {
		t.Fatalf("expected backend image, got %q", st.Backend)
	}

	// Should have progress callbacks: creating base image, syncing (2 rsync updates), done
	if len(phases) < 3 {
		t.Fatalf("expected at least 3 progress callbacks, got %d: phases=%v percents=%v", len(phases), phases, percents)
	}
	if phases[0] != "creating base image" {
		t.Fatalf("expected first phase 'creating base image', got %q", phases[0])
	}
	if phases[len(phases)-1] != "done" {
		t.Fatalf("expected last phase 'done', got %q", phases[len(phases)-1])
	}
	if percents[len(percents)-1] != 100 {
		t.Fatalf("expected final percent 100, got %d", percents[len(percents)-1])
	}

	// Verify SyncBaseWithProgress was used (Stream called) instead of SyncBase (CombinedOutput)
	if len(r.streamCalls) != 1 {
		t.Fatalf("expected 1 stream call for rsync, got %d", len(r.streamCalls))
	}
	if r.streamCalls[0].name != "rsync" {
		t.Fatalf("expected stream call to rsync, got %q", r.streamCalls[0].name)
	}
}
```

**Step 2: Run the test**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/f94f-for-the-new-imag/grove && go test ./internal/image/ -run TestInitBase_CallsOnProgress -v`
Expected: PASS (all infra was added in prior tasks)

**Step 3: Run all tests**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/f94f-for-the-new-imag/grove && go test ./...`
Expected: All PASS

**Step 4: Commit**

```bash
git add internal/image/backend_test.go
git commit -m "test: add integration test for InitBase progress callback"
```
