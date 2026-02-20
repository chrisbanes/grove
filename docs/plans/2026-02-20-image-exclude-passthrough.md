# Image Backend Exclude Passthrough

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Wire user-configured exclude patterns through the image backend's rsync calls so `InitBase` and `RefreshBase` skip excluded files.

**Architecture:** Add `excludes []string` parameter to `SyncBase`, `SyncBaseWithProgress`, `InitBase`, `RefreshBase`, and the `Backend.RefreshBase` interface method. Each user-defined exclude pattern becomes an `--exclude` flag in the rsync invocation, appended after the hardcoded `.grove/*` excludes.

**Tech Stack:** Go, rsync

---

### Task 1: Add excludes to SyncBase and SyncBaseWithProgress

**Files:**
- Modify: `internal/image/commands.go:153-199`
- Test: `internal/image/commands_test.go`

**Step 1: Write failing tests for excludes in SyncBase**

Add to `commands_test.go`:

```go
func TestSyncBase_WithExcludes(t *testing.T) {
	r := &fakeRunner{}
	if err := SyncBase(r, "/src", "/dst", []string{"node_modules", "*.lock"}); err != nil {
		t.Fatalf("SyncBase() error = %v", err)
	}

	if len(r.calls) != 1 {
		t.Fatalf("expected 1 command call, got %d", len(r.calls))
	}
	call := r.calls[0]
	if call.name != "rsync" {
		t.Fatalf("expected rsync, got %q", call.name)
	}
	want := []string{
		"-a",
		"--delete",
		"--exclude", ".grove/images/",
		"--exclude", ".grove/workspaces/",
		"--exclude", ".grove/shadows/",
		"--exclude", ".grove/mnt/",
		"--exclude", "node_modules",
		"--exclude", "*.lock",
		"/src/",
		"/dst/",
	}
	if strings.Join(call.args, " ") != strings.Join(want, " ") {
		t.Fatalf("unexpected args\nwant: %v\ngot:  %v", want, call.args)
	}
}

func TestSyncBaseWithProgress_WithExcludes(t *testing.T) {
	r := &fakeRunner{
		streamLines: []string{
			"  7,643,136,000 100%  109.38MB/s    0:01:02 (xfr#1, to-chk=0/100)",
		},
	}

	if err := SyncBaseWithProgress(r, "/src", "/dst", []string{"__pycache__"}, nil); err != nil {
		t.Fatalf("SyncBaseWithProgress() error = %v", err)
	}

	if len(r.streamCalls) != 1 {
		t.Fatalf("expected 1 stream call, got %d", len(r.streamCalls))
	}
	argsStr := strings.Join(r.streamCalls[0].args, " ")
	if !strings.Contains(argsStr, "--exclude __pycache__") {
		t.Fatalf("expected user exclude in args, got %v", r.streamCalls[0].args)
	}
}

func TestSyncBase_NilExcludes(t *testing.T) {
	r := &fakeRunner{}
	if err := SyncBase(r, "/src", "/dst", nil); err != nil {
		t.Fatalf("SyncBase() error = %v", err)
	}
	call := r.calls[0]
	want := []string{
		"-a",
		"--delete",
		"--exclude", ".grove/images/",
		"--exclude", ".grove/workspaces/",
		"--exclude", ".grove/shadows/",
		"--exclude", ".grove/mnt/",
		"/src/",
		"/dst/",
	}
	if strings.Join(call.args, " ") != strings.Join(want, " ") {
		t.Fatalf("unexpected args\nwant: %v\ngot:  %v", want, call.args)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/e6ad-can-you-verify-t/grove && go test ./internal/image/ -run "TestSyncBase_WithExcludes|TestSyncBaseWithProgress_WithExcludes|TestSyncBase_NilExcludes" -v`
Expected: FAIL — too many arguments

**Step 3: Update SyncBase and SyncBaseWithProgress signatures**

In `internal/image/commands.go`, change both functions to accept `excludes []string`. Build the args slice dynamically:

```go
func SyncBaseWithProgress(r Runner, src, dst string, excludes []string, onPercent func(int)) error {
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
	}
	for _, pattern := range excludes {
		args = append(args, "--exclude", pattern)
	}
	args = append(args, src, dst)
	return r.Stream("rsync", args, func(line string) {
		if onPercent == nil {
			return
		}
		if pct, ok := parseRsyncPercent(line); ok {
			onPercent(pct)
		}
	})
}

func SyncBase(r Runner, src, dst string, excludes []string) error {
	if r == nil {
		r = execRunner{}
	}
	src = ensureTrailingSlash(src)
	dst = ensureTrailingSlash(dst)
	args := []string{
		"-a",
		"--delete",
		"--exclude", ".grove/images/",
		"--exclude", ".grove/workspaces/",
		"--exclude", ".grove/shadows/",
		"--exclude", ".grove/mnt/",
	}
	for _, pattern := range excludes {
		args = append(args, "--exclude", pattern)
	}
	args = append(args, src, dst)
	return run(r, "rsync", args...)
}
```

**Step 4: Fix existing callers and tests that pass old signatures**

Update all call sites that now need the extra `excludes` parameter:
- `internal/image/backend.go`: `InitBase` and `RefreshBase` — pass `nil` for now (fixed in Task 2)
- `internal/image/commands_test.go`: existing `TestSyncBase_UsesExpectedCommand` and `TestSyncBaseWithProgress_*` tests — add `nil` excludes arg

**Step 5: Run all image package tests**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/e6ad-can-you-verify-t/grove && go test ./internal/image/ -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/image/commands.go internal/image/commands_test.go internal/image/backend.go
git commit -m "feat(image): add excludes parameter to SyncBase and SyncBaseWithProgress"
```

---

### Task 2: Thread excludes through InitBase and RefreshBase

**Files:**
- Modify: `internal/image/backend.go:9,65`
- Test: `internal/image/backend_test.go`

**Step 1: Write failing test for InitBase with excludes**

Add to `backend_test.go`:

```go
func TestInitBase_PassesExcludesToRsync(t *testing.T) {
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
	}

	excludes := []string{"node_modules", "*.lock"}
	_, err := InitBase(repoRoot, r, 20, excludes, nil)
	if err != nil {
		t.Fatalf("InitBase() error = %v", err)
	}

	// rsync is call index 2 (after hdiutil create, hdiutil attach)
	if len(r.calls) < 3 {
		t.Fatalf("expected at least 3 calls, got %d", len(r.calls))
	}
	rsyncCall := r.calls[2]
	if rsyncCall.name != "rsync" {
		t.Fatalf("expected call[2] to be rsync, got %q", rsyncCall.name)
	}
	argsStr := strings.Join(rsyncCall.args, " ")
	if !strings.Contains(argsStr, "--exclude node_modules") {
		t.Errorf("expected --exclude node_modules in rsync args: %s", argsStr)
	}
	if !strings.Contains(argsStr, "--exclude *.lock") {
		t.Errorf("expected --exclude *.lock in rsync args: %s", argsStr)
	}
}

func TestRefreshBase_PassesExcludesToRsync(t *testing.T) {
	repoRoot := t.TempDir()
	basePath := filepath.Join(repoRoot, ".grove", "images", "base.sparsebundle")
	if err := SaveState(repoRoot, &State{
		Backend:        "image",
		BasePath:       basePath,
		BaseGeneration: 1,
	}); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	r := &fakeRunner{
		outputs: [][]byte{
			[]byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>system-entities</key>
  <array>
    <dict><key>dev-entry</key><string>/dev/disk9</string></dict>
    <dict><key>dev-entry</key><string>/dev/disk9s1</string><key>mount-point</key><string>` + filepath.Join(repoRoot, ".grove", "mnt", "base") + `</string></dict>
  </array>
</dict>
</plist>`),
		},
	}

	excludes := []string{"__pycache__"}
	_, err := RefreshBase(repoRoot, repoRoot, r, "abc1234", excludes, nil)
	if err != nil {
		t.Fatalf("RefreshBase() error = %v", err)
	}

	if len(r.calls) < 2 {
		t.Fatalf("expected at least 2 calls, got %d", len(r.calls))
	}
	rsyncCall := r.calls[1]
	if rsyncCall.name != "rsync" {
		t.Fatalf("expected call[1] to be rsync, got %q", rsyncCall.name)
	}
	argsStr := strings.Join(rsyncCall.args, " ")
	if !strings.Contains(argsStr, "--exclude __pycache__") {
		t.Errorf("expected --exclude __pycache__ in rsync args: %s", argsStr)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/e6ad-can-you-verify-t/grove && go test ./internal/image/ -run "TestInitBase_PassesExcludesToRsync|TestRefreshBase_PassesExcludesToRsync" -v`
Expected: FAIL — too many arguments

**Step 3: Add excludes parameter to InitBase and RefreshBase**

In `internal/image/backend.go`:

```go
func InitBase(repoRoot string, runner Runner, baseSizeGB int, excludes []string, onProgress func(int, string)) (_ *State, err error) {
```

Pass `excludes` to `SyncBaseWithProgress` and `SyncBase`:

```go
	if onProgress != nil {
		onProgress(5, "syncing golden copy")
		err = SyncBaseWithProgress(runner, repoRoot, vol.MountPoint, excludes, func(pct int) {
			onProgress(mapPercent(pct, 100, 5, 95), "syncing golden copy")
		})
	} else {
		err = SyncBase(runner, repoRoot, vol.MountPoint, excludes)
	}
```

Same for `RefreshBase`:

```go
func RefreshBase(repoRoot, goldenRoot string, runner Runner, commit string, excludes []string, onProgress func(int, string)) (_ *State, err error) {
```

Pass `excludes` to sync calls similarly.

**Step 4: Fix existing tests that use old signatures**

In `backend_test.go`, update all existing `InitBase(...)` and `RefreshBase(...)` calls to include `nil` for excludes:
- `TestInitBase_CreatesStateAndRunsCommands`: `InitBase(repoRoot, r, 20, nil, nil)`
- `TestInitBase_CallsOnProgress`: `InitBase(repoRoot, r, 20, nil, onProgress)`
- `TestRefreshBase_CallsOnProgress`: `RefreshBase(repoRoot, repoRoot, r, "abc1234", nil, onProgress)`
- `TestRefreshBase_RefusesWhenWorkspacesExist`: `RefreshBase(repoRoot, repoRoot, r, "abc1234", nil, nil)`
- `TestRefreshBase_UpdatesGenerationAndCommit`: `RefreshBase(repoRoot, repoRoot, r, "abc1234", nil, nil)`

**Step 5: Run all image package tests**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/e6ad-can-you-verify-t/grove && go test ./internal/image/ -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/image/backend.go internal/image/backend_test.go
git commit -m "feat(image): thread excludes through InitBase and RefreshBase"
```

---

### Task 3: Update Backend interface and callers

**Files:**
- Modify: `internal/backend/backend.go:24`
- Modify: `internal/backend/image.go:68-73`
- Modify: `internal/backend/cp.go:33-35`
- Modify: `cmd/grove/init.go:114`
- Modify: `cmd/grove/update.go:74`

**Step 1: Update the Backend interface**

In `internal/backend/backend.go`, change:

```go
RefreshBase(goldenRoot, commit string, excludes []string, onProgress func(int, string)) error
```

**Step 2: Update cpBackend.RefreshBase**

In `internal/backend/cp.go`:

```go
func (cpBackend) RefreshBase(_ string, _ string, _ []string, _ func(int, string)) error {
	return nil
}
```

**Step 3: Update imageBackend.RefreshBase**

In `internal/backend/image.go`:

```go
func (imageBackend) RefreshBase(goldenRoot, commit string, excludes []string, onProgress func(int, string)) error {
	if _, err := image.RefreshBase(goldenRoot, goldenRoot, nil, commit, excludes, onProgress); err != nil {
		return fmt.Errorf("image backend refresh failed: %w", err)
	}
	return nil
}
```

**Step 4: Update cmd/grove/init.go**

At line 114, pass `cfg.Exclude`:

```go
if _, err := image.InitBase(absPath, nil, imageSizeGB, cfg.Exclude, onProgress); err != nil {
```

**Step 5: Update cmd/grove/update.go**

At line 74, pass `cfg.Exclude`:

```go
if err := backendImpl.RefreshBase(goldenRoot, commit, cfg.Exclude, onProgress); err != nil {
```

**Step 6: Run full test suite**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/e6ad-can-you-verify-t/grove && go test ./... -v -count=1`
Expected: PASS (skip e2e tests that need macOS if not on darwin)

**Step 7: Commit**

```bash
git add internal/backend/backend.go internal/backend/image.go internal/backend/cp.go cmd/grove/init.go cmd/grove/update.go
git commit -m "feat(image): wire config excludes through Backend interface to rsync"
```
