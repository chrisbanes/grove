package clone

import (
	"os"
	"path/filepath"
	"testing"
)

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
