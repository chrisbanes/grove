package clone_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/chrisbanes/grove/internal/clone"
)

func TestClone_StatMetadataParity(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}

	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")

	if err := os.MkdirAll(filepath.Join(src, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "nested", "b.txt"), []byte("beta"), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := clone.NewCloner(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Clone(src, dst); err != nil {
		t.Fatal(err)
	}

	for _, rel := range []string{"a.txt", "nested/b.txt"} {
		srcPath := filepath.Join(src, rel)
		dstPath := filepath.Join(dst, rel)

		srcData, err := os.ReadFile(srcPath)
		if err != nil {
			t.Fatalf("read source %s: %v", rel, err)
		}
		dstData, err := os.ReadFile(dstPath)
		if err != nil {
			t.Fatalf("read clone %s: %v", rel, err)
		}
		if string(srcData) != string(dstData) {
			t.Errorf("%s content mismatch: src=%q dst=%q", rel, string(srcData), string(dstData))
		}

		srcInode, srcSize, _, err := readStat(srcPath)
		if err != nil {
			t.Fatalf("stat source %s: %v", rel, err)
		}
		dstInode, dstSize, _, err := readStat(dstPath)
		if err != nil {
			t.Fatalf("stat clone %s: %v", rel, err)
		}

		if srcSize != dstSize {
			t.Errorf("%s size mismatch: src=%d dst=%d", rel, srcSize, dstSize)
		}
		if srcInode == dstInode {
			t.Errorf("%s inode should differ between src and clone", rel)
		}
	}
}

func readStat(path string) (inode int64, size int64, blocks int64, err error) {
	out, err := exec.Command("/usr/bin/stat", "-f", "%i %z %b", path).Output()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("stat failed for %s: %w", path, err)
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) != 3 {
		return 0, 0, 0, fmt.Errorf("unexpected stat output for %s: %q", path, strings.TrimSpace(string(out)))
	}
	inode, err = strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse inode for %s: %w", path, err)
	}
	size, err = strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse size for %s: %w", path, err)
	}
	blocks, err = strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse blocks for %s: %w", path, err)
	}
	return inode, size, blocks, nil
}
