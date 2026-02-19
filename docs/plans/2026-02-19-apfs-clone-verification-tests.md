# APFS Clone Verification Tests Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add macOS-focused tests that verify Grove is performing APFS clone-based copies, not just producing equivalent file contents.

**Architecture:** Extend `internal/clone` tests with filesystem metadata assertions (`stat`) and a physical disk usage comparison (`df`) that differentiates APFS CoW clones from regular copies. Keep tests Darwin-only and run them on the same mount to avoid cross-volume noise.

**Tech Stack:** Go test framework, `os/exec`, macOS `/usr/bin/stat`, `df`, existing `clone.NewCloner` and APFS `cp -c -R` implementation.

---

## Approach Options

1. `stat`-only metadata checks
- What: Compare inode/size/blocks from source vs clone and ensure clone mutations do not change source metadata/content.
- Pros: Simple, fast, easy to debug.
- Cons: Does not fully prove APFS block sharing; regular copies can satisfy many of the same checks.

2. Disk usage delta check (`df`) only
- What: Measure free space before/after `cloner.Clone` for a large file.
- Pros: Stronger signal of CoW sharing.
- Cons: Can be noisy under background disk activity.

3. Hybrid (Recommended)
- What: Keep `stat` checks for structure/integrity and add an in-test baseline comparison between CoW clone and regular copy disk deltas.
- Pros: Strong proof with lower flake risk because assertions are relative (`clone delta << regular copy delta`).
- Cons: Slightly slower test.

Recommended: **Approach 3**.

---

### Task 1: Add macOS stat helper + structural clone assertions

**Files:**
- Create: `internal/clone/apfs_clone_semantics_test.go`

**Step 1: Write failing test for metadata/structure parity**

Add test skeleton:
- `TestClone_StatMetadataParity`
- Guard: `runtime.GOOS != "darwin"` => `t.Skip(...)`
- Create source tree with nested files.
- Clone using `clone.NewCloner(src)` + `c.Clone(src, dst)`.
- Assert source and clone files have:
  - same size
  - distinct inode values
  - readable content equality for selected files

**Step 2: Run test to verify failure (or incomplete implementation)**

Run:
```bash
go test ./internal/clone -run TestClone_StatMetadataParity -count=1 -v
```

Expected: FAIL until helpers/parsing are implemented.

**Step 3: Implement stat helper used by the test**

In `internal/clone/apfs_clone_semantics_test.go` add helper(s):
- `readStat(path string) (inode int64, size int64, blocks int64, err error)`
- Execute `/usr/bin/stat -f "%i %z %b" <path>` and parse output.

**Step 4: Re-run test to verify pass**

Run:
```bash
go test ./internal/clone -run TestClone_StatMetadataParity -count=1 -v
```

Expected: PASS on macOS/APFS.

**Step 5: Commit**

```bash
git add internal/clone/apfs_clone_semantics_test.go
git commit -m "test: add APFS stat-based clone metadata verification"
```

---

### Task 2: Add CoW proof via relative disk-usage delta

**Files:**
- Modify: `internal/clone/apfs_clone_semantics_test.go`

**Step 1: Write failing test for clone-vs-copy physical usage**

Add test:
- `TestClone_DiskUsageDeltaMuchLowerThanRegularCopy`
- Create a sufficiently large file (e.g., 128-256 MB) in source.
- Capture free KB via `df -k` on the source mount (`before`).
- Perform CoW clone into `dstClone`; capture free KB (`afterClone`).
- Perform regular copy into `dstCopy` using `cp -R`; capture free KB (`afterCopy`).
- Compute deltas:
  - `cloneDelta = before - afterClone`
  - `copyDelta = afterClone - afterCopy`
- Assert:
  - `copyDelta` is materially positive
  - `cloneDelta` is significantly smaller than `copyDelta` (ratio assertion, e.g. `cloneDelta*5 < copyDelta`)

**Step 2: Run targeted test and confirm initial failure**

Run:
```bash
go test ./internal/clone -run TestClone_DiskUsageDeltaMuchLowerThanRegularCopy -count=1 -v
```

Expected: FAIL before helper logic/threshold tuning.

**Step 3: Implement robust helper(s) and stability guardrails**

Add helper(s):
- `freeKB(path string) (int64, error)` parsing `df -k` output.
- Optional: skip if `copyDelta` is too small to be meaningful (environment noise).
- Keep all temp dirs under a shared parent to ensure same filesystem/mount.

**Step 4: Re-run targeted + package tests**

Run:
```bash
go test ./internal/clone -run 'TestClone_(StatMetadataParity|DiskUsageDeltaMuchLowerThanRegularCopy)' -count=1 -v
go test ./internal/clone/... -count=1 -v
```

Expected: PASS on macOS/APFS.

**Step 5: Commit**

```bash
git add internal/clone/apfs_clone_semantics_test.go
git commit -m "test: verify APFS CoW clone behavior with disk usage baseline"
```

---

### Task 3: Add CLI-level regression coverage (optional but recommended)

**Files:**
- Modify: `test/e2e_test.go`

**Step 1: Add failing e2e test skeleton**

Add `TestCreate_CloneMetadataSignal`:
- `grove init` + `grove create --json`
- Compare `stat` on a representative tracked file in golden vs workspace.
- Assert equal size + distinct inode.

**Step 2: Run targeted e2e test**

Run:
```bash
go test ./test -run TestCreate_CloneMetadataSignal -count=1 -v
```

Expected: FAIL before helper wiring.

**Step 3: Implement small reusable e2e stat helper**

In `test/e2e_test.go` add helper for `/usr/bin/stat -f "%i %z %b"` and assertions.

**Step 4: Re-run targeted e2e tests**

Run:
```bash
go test ./test -run 'TestCreate_CloneMetadataSignal|TestE2E_CreateListDestroy' -count=1 -v
```

Expected: PASS on macOS/APFS.

**Step 5: Commit**

```bash
git add test/e2e_test.go
git commit -m "test: add e2e stat metadata check for workspace clones"
```

---

### Task 4: Final verification sweep

**Files:**
- Verify: `internal/clone/apfs_clone_semantics_test.go`
- Verify: `test/e2e_test.go` (if Task 3 implemented)

**Step 1: Run complete test suite**

Run:
```bash
go test ./... -count=1
```

Expected: Full suite passes; APFS-specific tests auto-skip on non-macOS.

**Step 2: Update docs only if behavior/guarantees wording changes**

If needed, update:
- `README.md` testing section with brief mention of CoW verification strategy.

**Step 3: Commit final docs/test polish (if any)**

```bash
git add README.md
git commit -m "docs: note APFS clone verification test strategy"
```
