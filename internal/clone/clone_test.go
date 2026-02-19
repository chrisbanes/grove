package clone_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/chrisbanes/grove/internal/clone"
)

func TestNewCloner_ReturnsCloner(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	dir := t.TempDir()
	c, err := clone.NewCloner(dir)
	if err != nil {
		t.Fatalf("expected cloner on macOS/APFS, got error: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil cloner")
	}
}

func TestNewCloner_ImplementsProgressCloner(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	dir := t.TempDir()
	c, err := clone.NewCloner(dir)
	if err != nil {
		t.Fatalf("expected cloner on macOS/APFS, got error: %v", err)
	}
	if _, ok := c.(clone.ProgressCloner); !ok {
		t.Fatal("expected cloner to implement ProgressCloner")
	}
}

func TestClone_CopiesAllFiles(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}

	src := t.TempDir()
	// Create a directory structure with files
	os.MkdirAll(filepath.Join(src, "sub", "deep"), 0755)
	os.WriteFile(filepath.Join(src, "root.txt"), []byte("root"), 0644)
	os.WriteFile(filepath.Join(src, "sub", "mid.txt"), []byte("mid"), 0644)
	os.WriteFile(filepath.Join(src, "sub", "deep", "leaf.txt"), []byte("leaf"), 0644)

	dst := filepath.Join(t.TempDir(), "clone")

	c, err := clone.NewCloner(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Clone(src, dst); err != nil {
		t.Fatal(err)
	}

	// Verify all files exist in clone
	for _, rel := range []string{"root.txt", "sub/mid.txt", "sub/deep/leaf.txt"} {
		data, err := os.ReadFile(filepath.Join(dst, rel))
		if err != nil {
			t.Errorf("missing file %s: %v", rel, err)
			continue
		}
		expected := filepath.Base(rel)
		expected = expected[:len(expected)-len(filepath.Ext(expected))]
		if string(data) != expected {
			t.Errorf("file %s: expected %q, got %q", rel, expected, string(data))
		}
	}
}

func TestClone_CopyOnWrite(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}

	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "file.txt"), []byte("original"), 0644)

	dst := filepath.Join(t.TempDir(), "clone")

	c, _ := clone.NewCloner(src)
	c.Clone(src, dst)

	// Modify the clone
	os.WriteFile(filepath.Join(dst, "file.txt"), []byte("modified"), 0644)

	// Source should be unchanged
	data, _ := os.ReadFile(filepath.Join(src, "file.txt"))
	if string(data) != "original" {
		t.Error("source was modified — CoW isolation broken")
	}
}

func TestClone_HiddenFiles(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}

	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, ".hidden"), 0755)
	os.WriteFile(filepath.Join(src, ".hidden", "secret.txt"), []byte("secret"), 0644)
	os.WriteFile(filepath.Join(src, ".dotfile"), []byte("dot"), 0644)

	dst := filepath.Join(t.TempDir(), "clone")

	c, _ := clone.NewCloner(src)
	c.Clone(src, dst)

	if _, err := os.Stat(filepath.Join(dst, ".hidden", "secret.txt")); err != nil {
		t.Error("hidden directory not cloned")
	}
	if _, err := os.Stat(filepath.Join(dst, ".dotfile")); err != nil {
		t.Error("dotfile not cloned")
	}
}
