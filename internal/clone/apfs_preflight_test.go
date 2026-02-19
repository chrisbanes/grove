package clone

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

type fakeFileInfo struct {
	sys any
}

func (f fakeFileInfo) Name() string       { return "fake" }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() fs.FileMode  { return 0 }
func (f fakeFileInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (f fakeFileInfo) IsDir() bool        { return true }
func (f fakeFileInfo) Sys() any           { return f.sys }

func TestEnsureSameFilesystemForClone_AllowsSameDevice(t *testing.T) {
	src := "/src"
	dst := "/workspaces/new-clone"
	parent := filepath.Dir(dst)

	statFn := func(path string) (os.FileInfo, error) {
		switch path {
		case src:
			return fakeFileInfo{sys: &syscall.Stat_t{Dev: 7}}, nil
		case dst:
			return nil, &os.PathError{Op: "stat", Path: dst, Err: os.ErrNotExist}
		case parent:
			return fakeFileInfo{sys: &syscall.Stat_t{Dev: 7}}, nil
		default:
			t.Fatalf("unexpected stat path: %s", path)
			return nil, nil
		}
	}

	if err := ensureSameFilesystemForCloneWithStat(src, dst, statFn); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestEnsureSameFilesystemForClone_RejectsCrossDevice(t *testing.T) {
	src := "/src"
	dst := "/workspaces/new-clone"
	parent := filepath.Dir(dst)

	statFn := func(path string) (os.FileInfo, error) {
		switch path {
		case src:
			return fakeFileInfo{sys: &syscall.Stat_t{Dev: 7}}, nil
		case dst:
			return nil, &os.PathError{Op: "stat", Path: dst, Err: os.ErrNotExist}
		case parent:
			return fakeFileInfo{sys: &syscall.Stat_t{Dev: 8}}, nil
		default:
			t.Fatalf("unexpected stat path: %s", path)
			return nil, nil
		}
	}

	err := ensureSameFilesystemForCloneWithStat(src, dst, statFn)
	if err == nil {
		t.Fatal("expected error for cross-device clone")
	}
	if !strings.Contains(err.Error(), "same filesystem") {
		t.Fatalf("expected same-filesystem guidance, got: %v", err)
	}
}
