# Exclude Globs Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Allow users to define exclude globs in `.grove/config.json` so that matching files and directories are never cloned when creating workspaces.

**Architecture:** Add an `Exclude []string` field to Config. Implement glob matching (basename for simple patterns, relative path for patterns containing `/`). Replace the single `cp -c -R` with a selective clone that walks the tree, plans which subtrees to clone, and executes multiple targeted `cp -c -R` calls while skipping excluded entries. Progress reporting reuses the plan walk's entry count.

**Tech Stack:** Go stdlib only (`filepath.Match`, `filepath.WalkDir`, `os/exec`)

**Design doc:** `docs/plans/2026-02-19-exclude-globs-design.md`

---

### Task 1: Add `Exclude` field to Config and validate patterns

**Files:**
- Modify: `internal/config/config.go:18-22` (Config struct)
- Modify: `internal/config/config.go:31-45` (Load function)
- Test: `internal/config/config_test.go`

**Step 1: Write the failing tests**

Add to `internal/config/config_test.go`:

```go
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
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -v -run "TestSaveAndLoad_Exclude|TestLoad_InvalidExcludePattern|TestLoad_EmptyExclude"`
Expected: `TestSaveAndLoad_Exclude` fails (Exclude field doesn't exist), others may pass vacuously.

**Step 3: Add the Exclude field and validation**

In `internal/config/config.go`, add the field to the struct:

```go
type Config struct {
	WarmupCommand string   `json:"warmup_command,omitempty"`
	WorkspaceDir  string   `json:"workspace_dir"`
	MaxWorkspaces int      `json:"max_workspaces"`
	Exclude       []string `json:"exclude,omitempty"`
}
```

In the `Load` function, add validation after unmarshaling (before the `MaxWorkspaces` default):

```go
for _, pattern := range cfg.Exclude {
	if _, err := filepath.Match(pattern, ""); err != nil {
		return nil, fmt.Errorf("invalid exclude pattern %q: %w", pattern, err)
	}
}
```

This requires adding `"path/filepath"` to the imports.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: All tests pass.

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add Exclude field with pattern validation"
```

---

### Task 2: Implement glob matching function

**Files:**
- Create: `internal/clone/exclude.go`
- Create: `internal/clone/exclude_test.go`

**Step 1: Write the failing tests**

Create `internal/clone/exclude_test.go`:

```go
package clone

import "testing"

func TestMatchExclude(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		relPath string
		want    bool
	}{
		// Basename matching (no / in pattern)
		{"basename wildcard matches file", "*.lock", "yarn.lock", true},
		{"basename wildcard matches nested file", "*.lock", "packages/foo/yarn.lock", true},
		{"basename wildcard no match", "*.lock", "yarn.txt", false},
		{"basename exact matches dir", "__pycache__", "__pycache__", true},
		{"basename exact matches nested dir", "__pycache__", "src/lib/__pycache__", true},
		{"basename exact no match", "__pycache__", "pycache", false},

		// Path matching (/ in pattern)
		{"path pattern matches exact", ".gradle/configuration-cache", ".gradle/configuration-cache", true},
		{"path pattern no match at wrong depth", ".gradle/configuration-cache", "sub/.gradle/configuration-cache", false},
		{"path pattern no match partial", ".gradle/configuration-cache", ".gradle/caches", false},

		// Edge cases
		{"empty relPath", "*.lock", "", false},
		{"root file basename match", "*.lock", "package.lock", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchExclude(tt.pattern, tt.relPath)
			if got != tt.want {
				t.Errorf("matchExclude(%q, %q) = %v, want %v", tt.pattern, tt.relPath, got, tt.want)
			}
		})
	}
}

func TestIsExcluded(t *testing.T) {
	excludes := []string{"*.lock", "__pycache__", ".gradle/configuration-cache"}

	tests := []struct {
		name    string
		relPath string
		want    bool
	}{
		{"matches wildcard", "packages/yarn.lock", true},
		{"matches basename", "src/__pycache__", true},
		{"matches path", ".gradle/configuration-cache", true},
		{"no match", "src/main.go", false},
		{"grove dir never excluded", ".grove", false},
		{"grove subpath never excluded", ".grove/config.json", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isExcluded(tt.relPath, excludes)
			if got != tt.want {
				t.Errorf("isExcluded(%q, ...) = %v, want %v", tt.relPath, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/clone/ -v -run "TestMatchExclude|TestIsExcluded"`
Expected: FAIL — functions don't exist yet.

**Step 3: Implement the matching functions**

Create `internal/clone/exclude.go`:

```go
package clone

import (
	"path/filepath"
	"strings"
)

// matchExclude checks whether relPath matches a single exclude pattern.
// If the pattern contains no /, it matches against the basename.
// If the pattern contains /, it matches against the full relative path.
func matchExclude(pattern, relPath string) bool {
	if relPath == "" {
		return false
	}
	if strings.Contains(pattern, "/") {
		matched, _ := filepath.Match(pattern, relPath)
		return matched
	}
	matched, _ := filepath.Match(pattern, filepath.Base(relPath))
	return matched
}

// isExcluded checks whether relPath matches any of the exclude patterns.
// The .grove directory is never excluded.
func isExcluded(relPath string, excludes []string) bool {
	if relPath == ".grove" || strings.HasPrefix(relPath, ".grove/") || strings.HasPrefix(relPath, ".grove\\") {
		return false
	}
	for _, pattern := range excludes {
		if matchExclude(pattern, relPath) {
			return true
		}
	}
	return false
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/clone/ -v -run "TestMatchExclude|TestIsExcluded"`
Expected: All pass.

**Step 5: Commit**

```bash
git add internal/clone/exclude.go internal/clone/exclude_test.go
git commit -m "feat(clone): add exclude glob matching functions"
```

---

### Task 3: Implement the plan walk (Phase 1)

**Files:**
- Modify: `internal/clone/exclude.go`
- Modify: `internal/clone/exclude_test.go`

**Step 1: Write the failing tests**

Add to `internal/clone/exclude_test.go`:

```go
func TestBuildClonePlan_NoExcludes(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "sub"), 0755)
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("b"), 0644)

	plan, err := buildClonePlan(src, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.totalEntries < 3 {
		t.Errorf("expected at least 3 entries (root dir, sub dir, 2 files), got %d", plan.totalEntries)
	}
	if len(plan.dirsWithExcludes) != 0 {
		t.Errorf("expected no dirs with excludes, got %v", plan.dirsWithExcludes)
	}
}

func TestBuildClonePlan_WithExcludes(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "pkg", "foo"), 0755)
	os.WriteFile(filepath.Join(src, "pkg", "foo", "yarn.lock"), []byte("lock"), 0644)
	os.WriteFile(filepath.Join(src, "pkg", "foo", "main.go"), []byte("go"), 0644)
	os.WriteFile(filepath.Join(src, "root.lock"), []byte("lock"), 0644)
	os.WriteFile(filepath.Join(src, "keep.txt"), []byte("keep"), 0644)

	plan, err := buildClonePlan(src, []string{"*.lock"})
	if err != nil {
		t.Fatal(err)
	}

	// root.lock and pkg/foo/yarn.lock should be excluded from count
	// Included entries: src(root dir), pkg(dir), pkg/foo(dir), pkg/foo/main.go, keep.txt = 5
	if plan.totalEntries != 5 {
		t.Errorf("expected 5 non-excluded entries, got %d", plan.totalEntries)
	}

	// "." and "pkg" and "pkg/foo" should be marked as containing excludes
	if !plan.dirsWithExcludes["."] {
		t.Error("expected root dir to be marked as containing excludes")
	}
	if !plan.dirsWithExcludes["pkg/foo"] {
		t.Error("expected pkg/foo to be marked as containing excludes")
	}
}

func TestBuildClonePlan_ExcludedDirSkipsDescendants(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "__pycache__", "deep"), 0755)
	os.WriteFile(filepath.Join(src, "__pycache__", "deep", "file.pyc"), []byte("pyc"), 0644)
	os.WriteFile(filepath.Join(src, "keep.txt"), []byte("keep"), 0644)

	plan, err := buildClonePlan(src, []string{"__pycache__"})
	if err != nil {
		t.Fatal(err)
	}

	// Only root dir + keep.txt should be counted
	if plan.totalEntries != 2 {
		t.Errorf("expected 2 entries, got %d", plan.totalEntries)
	}
}

func TestBuildClonePlan_PathPatternExclude(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, ".gradle", "configuration-cache"), 0755)
	os.MkdirAll(filepath.Join(src, ".gradle", "caches"), 0755)
	os.WriteFile(filepath.Join(src, ".gradle", "configuration-cache", "data.bin"), []byte("data"), 0644)
	os.WriteFile(filepath.Join(src, ".gradle", "caches", "deps.jar"), []byte("jar"), 0644)

	plan, err := buildClonePlan(src, []string{".gradle/configuration-cache"})
	if err != nil {
		t.Fatal(err)
	}

	// root, .gradle, .gradle/caches, .gradle/caches/deps.jar = 4
	if plan.totalEntries != 4 {
		t.Errorf("expected 4 entries, got %d", plan.totalEntries)
	}
}
```

Update imports at the top of the test file to include `"os"` and `"path/filepath"`.

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/clone/ -v -run "TestBuildClonePlan"`
Expected: FAIL — `buildClonePlan` doesn't exist.

**Step 3: Implement `buildClonePlan`**

Add to `internal/clone/exclude.go`:

```go
import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// clonePlan holds the results of walking the source tree with exclude patterns.
type clonePlan struct {
	// totalEntries is the count of non-excluded entries (for progress reporting).
	totalEntries int
	// dirsWithExcludes maps relative directory paths that contain excluded
	// descendants. The key "." represents the source root.
	dirsWithExcludes map[string]bool
}

// buildClonePlan walks src and computes which entries are excluded.
// It returns the total non-excluded entry count and which directories
// contain excluded descendants (and therefore cannot be cloned as a single subtree).
func buildClonePlan(src string, excludes []string) (*clonePlan, error) {
	plan := &clonePlan{
		dirsWithExcludes: make(map[string]bool),
	}
	if len(excludes) == 0 {
		count, err := countAllEntries(src)
		if err != nil {
			return nil, err
		}
		plan.totalEntries = count
		return plan, nil
	}

	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			plan.totalEntries++
			return nil
		}
		if isExcluded(rel, excludes) {
			// Mark all ancestor directories as containing excludes
			markAncestors(rel, plan.dirsWithExcludes)
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		plan.totalEntries++
		return nil
	})
	return plan, err
}

// countAllEntries counts all filesystem entries under root (used when no excludes).
func countAllEntries(root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(_ string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
}

// markAncestors marks all ancestor directories of rel (up to ".") in the map.
func markAncestors(rel string, dirs map[string]bool) {
	for {
		parent := filepath.Dir(rel)
		if parent == "." || parent == rel {
			dirs["."] = true
			return
		}
		dirs[parent] = true
		rel = parent
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/clone/ -v -run "TestBuildClonePlan"`
Expected: All pass.

**Step 5: Commit**

```bash
git add internal/clone/exclude.go internal/clone/exclude_test.go
git commit -m "feat(clone): implement clone plan walk with exclude support"
```

---

### Task 4: Implement `SelectiveClone` (Phase 2 — no progress)

**Files:**
- Modify: `internal/clone/exclude.go`
- Modify: `internal/clone/exclude_test.go`

**Step 1: Write the failing tests**

Add to `internal/clone/exclude_test.go`. These tests use the real APFS cloner and need the macOS guard:

```go
func TestSelectiveClone_NoExcludes(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "sub"), 0755)
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("b"), 0644)

	dst := filepath.Join(t.TempDir(), "clone")
	c, _ := NewCloner(src)

	if err := SelectiveClone(c, src, dst, nil); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dst, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "a" {
		t.Errorf("expected 'a', got %q", string(data))
	}
	data, err = os.ReadFile(filepath.Join(dst, "sub", "b.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "b" {
		t.Errorf("expected 'b', got %q", string(data))
	}
}

func TestSelectiveClone_ExcludesTopLevel(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "keep"), 0755)
	os.MkdirAll(filepath.Join(src, "__pycache__"), 0755)
	os.WriteFile(filepath.Join(src, "keep", "file.txt"), []byte("keep"), 0644)
	os.WriteFile(filepath.Join(src, "__pycache__", "module.pyc"), []byte("pyc"), 0644)

	dst := filepath.Join(t.TempDir(), "clone")
	c, _ := NewCloner(src)

	if err := SelectiveClone(c, src, dst, []string{"__pycache__"}); err != nil {
		t.Fatal(err)
	}

	// keep/file.txt should exist
	if _, err := os.Stat(filepath.Join(dst, "keep", "file.txt")); err != nil {
		t.Error("keep/file.txt should exist")
	}
	// __pycache__ should NOT exist
	if _, err := os.Stat(filepath.Join(dst, "__pycache__")); !os.IsNotExist(err) {
		t.Error("__pycache__ should not exist in clone")
	}
}

func TestSelectiveClone_ExcludesNestedFile(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "pkg", "foo"), 0755)
	os.WriteFile(filepath.Join(src, "pkg", "foo", "main.go"), []byte("go"), 0644)
	os.WriteFile(filepath.Join(src, "pkg", "foo", "yarn.lock"), []byte("lock"), 0644)
	os.WriteFile(filepath.Join(src, "root.txt"), []byte("root"), 0644)

	dst := filepath.Join(t.TempDir(), "clone")
	c, _ := NewCloner(src)

	if err := SelectiveClone(c, src, dst, []string{"*.lock"}); err != nil {
		t.Fatal(err)
	}

	// main.go and root.txt should exist
	if _, err := os.Stat(filepath.Join(dst, "pkg", "foo", "main.go")); err != nil {
		t.Error("pkg/foo/main.go should exist")
	}
	if _, err := os.Stat(filepath.Join(dst, "root.txt")); err != nil {
		t.Error("root.txt should exist")
	}
	// yarn.lock should NOT exist
	if _, err := os.Stat(filepath.Join(dst, "pkg", "foo", "yarn.lock")); !os.IsNotExist(err) {
		t.Error("pkg/foo/yarn.lock should not exist in clone")
	}
}

func TestSelectiveClone_PathPatternExclude(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, ".gradle", "configuration-cache"), 0755)
	os.MkdirAll(filepath.Join(src, ".gradle", "caches"), 0755)
	os.WriteFile(filepath.Join(src, ".gradle", "configuration-cache", "data.bin"), []byte("data"), 0644)
	os.WriteFile(filepath.Join(src, ".gradle", "caches", "deps.jar"), []byte("jar"), 0644)

	dst := filepath.Join(t.TempDir(), "clone")
	c, _ := NewCloner(src)

	if err := SelectiveClone(c, src, dst, []string{".gradle/configuration-cache"}); err != nil {
		t.Fatal(err)
	}

	// caches/deps.jar should exist
	if _, err := os.Stat(filepath.Join(dst, ".gradle", "caches", "deps.jar")); err != nil {
		t.Error(".gradle/caches/deps.jar should exist")
	}
	// configuration-cache should NOT exist
	if _, err := os.Stat(filepath.Join(dst, ".gradle", "configuration-cache")); !os.IsNotExist(err) {
		t.Error(".gradle/configuration-cache should not exist in clone")
	}
}

func TestSelectiveClone_GroveDirNeverExcluded(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, ".grove"), 0755)
	os.WriteFile(filepath.Join(src, ".grove", "config.json"), []byte("{}"), 0644)

	dst := filepath.Join(t.TempDir(), "clone")
	c, _ := NewCloner(src)

	// Even if someone tries to exclude .grove, it should still be cloned
	if err := SelectiveClone(c, src, dst, []string{".grove"}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dst, ".grove", "config.json")); err != nil {
		t.Error(".grove/config.json should exist despite exclude pattern")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/clone/ -v -run "TestSelectiveClone"`
Expected: FAIL — `SelectiveClone` doesn't exist.

**Step 3: Implement `SelectiveClone`**

Add to `internal/clone/exclude.go`:

```go
// SelectiveClone clones src to dst, excluding paths matching the given globs.
// If excludes is empty, falls back to a single full clone.
func SelectiveClone(cloner Cloner, src, dst string, excludes []string) error {
	if len(excludes) == 0 {
		return cloner.Clone(src, dst)
	}

	plan, err := buildClonePlan(src, excludes)
	if err != nil {
		return fmt.Errorf("planning clone: %w", err)
	}

	return executeClonePlan(cloner, src, dst, ".", excludes, plan)
}

// executeClonePlan recursively clones children of srcDir into dstDir,
// skipping excluded entries and recursing into directories that contain excludes.
func executeClonePlan(cloner Cloner, srcRoot, dstRoot, rel string, excludes []string, plan *clonePlan) error {
	srcDir := filepath.Join(srcRoot, rel)
	dstDir := filepath.Join(dstRoot, rel)

	if rel == "." {
		srcDir = srcRoot
		dstDir = dstRoot
	}

	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		childRel := filepath.Join(rel, entry.Name())
		if rel == "." {
			childRel = entry.Name()
		}

		if isExcluded(childRel, excludes) {
			continue
		}

		childSrc := filepath.Join(srcDir, entry.Name())
		childDst := filepath.Join(dstDir, entry.Name())

		if entry.IsDir() && plan.dirsWithExcludes[childRel] {
			// This directory contains excluded descendants — recurse
			if err := executeClonePlan(cloner, srcRoot, dstRoot, childRel, excludes, plan); err != nil {
				return err
			}
			continue
		}

		// Fast path: clone the entire entry with a single cp -c -R
		if err := cloner.Clone(childSrc, childDst); err != nil {
			return err
		}
	}
	return nil
}
```

Add `"fmt"` to imports if not already present.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/clone/ -v -run "TestSelectiveClone"`
Expected: All pass.

**Step 5: Commit**

```bash
git add internal/clone/exclude.go internal/clone/exclude_test.go
git commit -m "feat(clone): implement SelectiveClone with subtree-level granularity"
```

---

### Task 5: Implement `SelectiveCloneWithProgress`

**Files:**
- Modify: `internal/clone/exclude.go`
- Modify: `internal/clone/exclude_test.go`

**Step 1: Write the failing test**

Add to `internal/clone/exclude_test.go`:

```go
func TestSelectiveCloneWithProgress_ReportsCorrectTotal(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "keep"), 0755)
	os.MkdirAll(filepath.Join(src, "__pycache__"), 0755)
	os.WriteFile(filepath.Join(src, "keep", "file.txt"), []byte("keep"), 0644)
	os.WriteFile(filepath.Join(src, "__pycache__", "module.pyc"), []byte("pyc"), 0644)
	os.WriteFile(filepath.Join(src, "root.txt"), []byte("root"), 0644)

	dst := filepath.Join(t.TempDir(), "clone")
	c, _ := NewCloner(src)

	var scanTotal int
	var lastCopied int
	onProgress := func(e ProgressEvent) {
		if e.Phase == "scan" {
			scanTotal = e.Total
		}
		if e.Phase == "clone" {
			lastCopied = e.Copied
		}
	}

	if err := SelectiveCloneWithProgress(c, src, dst, []string{"__pycache__"}, onProgress); err != nil {
		t.Fatal(err)
	}

	// Non-excluded: root dir, keep dir, keep/file.txt, root.txt = 4
	if scanTotal != 4 {
		t.Errorf("expected scan total 4, got %d", scanTotal)
	}
	if lastCopied < 1 {
		t.Error("expected at least one progress event during clone")
	}
}

func TestSelectiveCloneWithProgress_NoExcludesFallback(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0644)

	dst := filepath.Join(t.TempDir(), "clone")
	c, _ := NewCloner(src)

	var gotScan bool
	onProgress := func(e ProgressEvent) {
		if e.Phase == "scan" {
			gotScan = true
		}
	}

	if err := SelectiveCloneWithProgress(c, src, dst, nil, onProgress); err != nil {
		t.Fatal(err)
	}
	if !gotScan {
		t.Error("expected scan phase event even with no excludes")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/clone/ -v -run "TestSelectiveCloneWithProgress"`
Expected: FAIL — function doesn't exist.

**Step 3: Implement `SelectiveCloneWithProgress`**

Add to `internal/clone/exclude.go`:

```go
// SelectiveCloneWithProgress clones src to dst with excludes and progress reporting.
// If excludes is empty, falls back to the cloner's CloneWithProgress if available.
func SelectiveCloneWithProgress(cloner Cloner, src, dst string, excludes []string, onProgress ProgressFunc) error {
	if len(excludes) == 0 {
		if pc, ok := cloner.(ProgressCloner); ok && onProgress != nil {
			return pc.CloneWithProgress(src, dst, onProgress)
		}
		return cloner.Clone(src, dst)
	}

	plan, err := buildClonePlan(src, excludes)
	if err != nil {
		return fmt.Errorf("planning clone: %w", err)
	}

	if onProgress != nil {
		onProgress(ProgressEvent{Total: plan.totalEntries, Phase: "scan"})
	}

	copied := 0
	countingCloner := &progressTrackingCloner{
		inner:      cloner,
		copied:     &copied,
		total:      plan.totalEntries,
		onProgress: onProgress,
	}

	return executeClonePlan(countingCloner, src, dst, ".", excludes, plan)
}

// progressTrackingCloner wraps a Cloner and counts entries via cp -c -R -v output.
type progressTrackingCloner struct {
	inner      Cloner
	copied     *int
	total      int
	onProgress ProgressFunc
}

func (p *progressTrackingCloner) Clone(src, dst string) error {
	if pc, ok := p.inner.(ProgressCloner); ok && p.onProgress != nil {
		return pc.CloneWithProgress(src, dst, func(e ProgressEvent) {
			if e.Phase != "clone" {
				return
			}
			*p.copied += e.Copied
			p.onProgress(ProgressEvent{
				Copied: *p.copied,
				Total:  p.total,
				Phase:  "clone",
			})
		})
	}
	return p.inner.Clone(src, dst)
}
```

Note: The `progressTrackingCloner` wraps each individual `cp -c -R` call and accumulates the copied count across all calls, reporting against the plan's total.

The progress tracking wrapper above has a subtle issue: `CloneWithProgress` reports cumulative `Copied` per call, but we need to accumulate across calls. Adjust so we track the delta:

```go
func (p *progressTrackingCloner) Clone(src, dst string) error {
	if pc, ok := p.inner.(ProgressCloner); ok && p.onProgress != nil {
		prevCopied := 0
		return pc.CloneWithProgress(src, dst, func(e ProgressEvent) {
			if e.Phase != "clone" {
				return
			}
			delta := e.Copied - prevCopied
			prevCopied = e.Copied
			*p.copied += delta
			p.onProgress(ProgressEvent{
				Copied: *p.copied,
				Total:  p.total,
				Phase:  "clone",
			})
		})
	}
	return p.inner.Clone(src, dst)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/clone/ -v -run "TestSelectiveCloneWithProgress"`
Expected: All pass.

**Step 5: Commit**

```bash
git add internal/clone/exclude.go internal/clone/exclude_test.go
git commit -m "feat(clone): implement SelectiveCloneWithProgress"
```

---

### Task 6: Wire SelectiveClone into workspace.Create

**Files:**
- Modify: `internal/workspace/workspace.go:58-89` (cloneWorkspace function and Create)
- Modify: `internal/workspace/workspace_test.go`

**Step 1: Write the failing test**

Add to `internal/workspace/workspace_test.go`:

```go
func TestCreate_WithExcludes(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".grove", "hooks"), 0755)
	os.WriteFile(filepath.Join(dir, "src.txt"), []byte("source"), 0644)
	os.MkdirAll(filepath.Join(dir, "__pycache__"), 0755)
	os.WriteFile(filepath.Join(dir, "__pycache__", "module.pyc"), []byte("pyc"), 0644)

	wsDir := filepath.Join(t.TempDir(), "workspaces")
	cfg := &config.Config{
		WorkspaceDir:  wsDir,
		MaxWorkspaces: 3,
		Exclude:       []string{"__pycache__"},
	}
	config.Save(dir, cfg)
	c, _ := clone.NewCloner(dir)

	info, err := workspace.Create(dir, cfg, c, workspace.CreateOpts{})
	if err != nil {
		t.Fatal(err)
	}

	// src.txt should be cloned
	if _, err := os.Stat(filepath.Join(info.Path, "src.txt")); err != nil {
		t.Error("src.txt should exist in workspace")
	}
	// __pycache__ should NOT be cloned
	if _, err := os.Stat(filepath.Join(info.Path, "__pycache__")); !os.IsNotExist(err) {
		t.Error("__pycache__ should not exist in workspace")
	}
	// Workspace marker should still exist
	if !workspace.IsWorkspace(info.Path) {
		t.Error("workspace marker should exist")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/workspace/ -v -run "TestCreate_WithExcludes"`
Expected: FAIL — `__pycache__` still exists because `Create` doesn't use excludes yet.

**Step 3: Wire up SelectiveClone in workspace.Create**

Modify `internal/workspace/workspace.go`:

Change the `cloneWorkspace` function to accept excludes:

```go
func cloneWorkspace(cloner clone.Cloner, src, dst string, excludes []string, onClone clone.ProgressFunc) error {
	if onClone != nil {
		return clone.SelectiveCloneWithProgress(cloner, src, dst, excludes, onClone)
	}
	return clone.SelectiveClone(cloner, src, dst, excludes)
}
```

Update the call in `Create`:

```go
if err := cloneWorkspace(cloner, goldenRoot, wsPath, cfg.Exclude, opts.OnClone); err != nil {
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/workspace/ -v`
Expected: All pass (both new and existing tests).

**Step 5: Run all tests to check for regressions**

Run: `go test ./...`
Expected: All pass.

**Step 6: Commit**

```bash
git add internal/workspace/workspace.go internal/workspace/workspace_test.go
git commit -m "feat(workspace): wire SelectiveClone into Create for exclude support"
```

---

### Task 7: E2E test

**Files:**
- Modify: `test/e2e_test.go`

**Step 1: Write the failing e2e test**

Add to `test/e2e_test.go`:

```go
func TestCreateWithExcludes(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	binary := buildGrove(t)
	repo := setupTestRepo(t)

	// Add files that should be excluded
	os.MkdirAll(filepath.Join(repo, "__pycache__"), 0755)
	os.WriteFile(filepath.Join(repo, "__pycache__", "module.pyc"), []byte("pyc"), 0644)
	os.WriteFile(filepath.Join(repo, "yarn.lock"), []byte("lockfile"), 0644)

	// Init with exclude patterns
	grove(t, binary, repo, "init")

	// Manually update config to add excludes
	cfgPath := filepath.Join(repo, ".grove", "config.json")
	cfgData, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	json.Unmarshal(cfgData, &cfg)
	cfg["exclude"] = []string{"__pycache__", "*.lock"}
	updated, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(cfgPath, updated, 0644)

	// Create workspace
	out := grove(t, binary, repo, "create", "--json")
	var info workspace.Info
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		t.Fatalf("invalid JSON: %s\n%s", err, out)
	}
	defer grove(t, binary, repo, "destroy", info.ID)

	// Verify included files
	if _, err := os.Stat(filepath.Join(info.Path, "main.go")); err != nil {
		t.Error("main.go should exist in workspace")
	}
	if _, err := os.Stat(filepath.Join(info.Path, "build", "output.bin")); err != nil {
		t.Error("build/output.bin should exist in workspace")
	}

	// Verify excluded files
	if _, err := os.Stat(filepath.Join(info.Path, "__pycache__")); !os.IsNotExist(err) {
		t.Error("__pycache__ should not exist in workspace")
	}
	if _, err := os.Stat(filepath.Join(info.Path, "yarn.lock")); !os.IsNotExist(err) {
		t.Error("yarn.lock should not exist in workspace")
	}

	// Verify .grove still exists (never excluded)
	if _, err := os.Stat(filepath.Join(info.Path, ".grove", "config.json")); err != nil {
		t.Error(".grove/config.json should exist in workspace")
	}
}
```

**Step 2: Run the e2e test to verify it fails**

Run: `go test ./test/ -v -run "TestCreateWithExcludes" -timeout 60s`
Expected: FAIL — excluded files still present.

**Step 3: This should already pass**

After Task 6, the full pipeline is wired. If this test fails, debug and fix.

**Step 4: Run the full e2e suite to check for regressions**

Run: `go test ./test/ -v -timeout 120s`
Expected: All pass.

**Step 5: Commit**

```bash
git add test/e2e_test.go
git commit -m "test: add e2e test for exclude globs in workspace creation"
```

---

### Task 8: Run full test suite and clean up

**Step 1: Run all tests**

Run: `go test ./... -timeout 120s`
Expected: All pass.

**Step 2: Run linter if available**

Run: `go vet ./...`
Expected: Clean.

**Step 3: Final commit if any cleanup needed**

Only if there are changes to commit from cleanup.
