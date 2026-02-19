package clone_test

import (
	"bytes"
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

func TestClone_DiskUsageDeltaMuchLowerThanRegularCopy(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}

	root := t.TempDir()
	src := filepath.Join(root, "src")
	dstClone := filepath.Join(root, "dst-clone")
	dstCopy := filepath.Join(root, "dst-copy")

	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a real 128 MiB file so df deltas are meaningful.
	bigFile := filepath.Join(src, "big.bin")
	f, err := os.Create(bigFile)
	if err != nil {
		t.Fatal(err)
	}
	chunk := bytes.Repeat([]byte{'z'}, 1024*1024)
	for i := 0; i < 128; i++ {
		if _, err := f.Write(chunk); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	before, err := freeKB(root)
	if err != nil {
		t.Fatal(err)
	}

	c, err := clone.NewCloner(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Clone(src, dstClone); err != nil {
		t.Fatal(err)
	}
	afterClone, err := freeKB(root)
	if err != nil {
		t.Fatal(err)
	}

	if out, err := exec.Command("cp", "-R", src, dstCopy).CombinedOutput(); err != nil {
		t.Fatalf("regular copy failed: %v (%s)", err, string(out))
	}
	afterCopy, err := freeKB(root)
	if err != nil {
		t.Fatal(err)
	}

	cloneDelta := before - afterClone
	copyDelta := afterClone - afterCopy

	if copyDelta <= 1024 {
		t.Skipf("disk delta too small/noisy for reliable assertion: clone=%dKB copy=%dKB", cloneDelta, copyDelta)
	}
	if cloneDelta*5 >= copyDelta {
		t.Fatalf("expected CoW clone delta to be much smaller than regular copy delta: clone=%dKB copy=%dKB", cloneDelta, copyDelta)
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

func freeKB(path string) (int64, error) {
	out, err := exec.Command("df", "-k", path).Output()
	if err != nil {
		return 0, fmt.Errorf("df failed for %s: %w", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("unexpected df output for %s: %q", path, strings.TrimSpace(string(out)))
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 4 {
		return 0, fmt.Errorf("unexpected df row for %s: %q", path, lines[len(lines)-1])
	}
	free, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse free KB for %s: %w", path, err)
	}
	return free, nil
}
