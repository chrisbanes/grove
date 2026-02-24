package backend

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/chrisbanes/grove/internal/image"
)

func TestLoadOrInitImageState_UsesExistingState(t *testing.T) {
	origLoad := imageLoadState
	origInit := imageInitBase
	t.Cleanup(func() {
		imageLoadState = origLoad
		imageInitBase = origInit
	})

	want := &image.State{Backend: "image", BasePath: "/tmp/base.sparsebundle", BaseGeneration: 3}
	imageLoadState = func(string) (*image.State, error) {
		return want, nil
	}
	imageInitBase = func(string, string, image.Runner, int, []string, func(int, string)) (*image.State, error) {
		t.Fatal("imageInitBase should not be called when state exists")
		return nil, nil
	}

	got, initialized, err := loadOrInitImageState("/tmp/runtime", "/tmp/repo", nil, nil)
	if err != nil {
		t.Fatalf("loadOrInitImageState() error = %v", err)
	}
	if initialized {
		t.Fatal("expected initialized=false when state exists")
	}
	if got != want {
		t.Fatalf("expected existing state pointer to be reused")
	}
}

func TestLoadOrInitImageState_InitializesWhenMissing(t *testing.T) {
	origLoad := imageLoadState
	origInit := imageInitBase
	t.Cleanup(func() {
		imageLoadState = origLoad
		imageInitBase = origInit
	})

	imageLoadState = func(string) (*image.State, error) {
		return nil, os.ErrNotExist
	}

	seenProgress := false
	imageInitBase = func(runtimeRoot, goldenRoot string, runner image.Runner, sizeGB int, excludes []string, onProgress func(int, string)) (*image.State, error) {
		if runtimeRoot != "/tmp/runtime" {
			t.Fatalf("unexpected runtime root: %s", runtimeRoot)
		}
		if goldenRoot != "/tmp/repo" {
			t.Fatalf("unexpected golden root: %s", goldenRoot)
		}
		if runner != nil {
			t.Fatal("expected nil runner")
		}
		if sizeGB != createInitBaseSizeGB {
			t.Fatalf("expected size %d, got %d", createInitBaseSizeGB, sizeGB)
		}
		if len(excludes) != 1 || excludes[0] != "node_modules" {
			t.Fatalf("unexpected excludes: %v", excludes)
		}
		if onProgress == nil {
			t.Fatal("expected progress callback to be forwarded")
		}
		onProgress(10, "creating")
		return &image.State{Backend: "image", BasePath: "/tmp/base.sparsebundle", BaseGeneration: 1}, nil
	}

	onProgress := func(int, string) { seenProgress = true }
	got, initialized, err := loadOrInitImageState("/tmp/runtime", "/tmp/repo", []string{"node_modules"}, onProgress)
	if err != nil {
		t.Fatalf("loadOrInitImageState() error = %v", err)
	}
	if !initialized {
		t.Fatal("expected initialized=true when state is missing")
	}
	if got == nil || got.BaseGeneration != 1 {
		t.Fatalf("unexpected state: %#v", got)
	}
	if !seenProgress {
		t.Fatal("expected progress callback to be invoked")
	}
}

func TestLoadOrInitImageState_PropagatesLoadError(t *testing.T) {
	origLoad := imageLoadState
	origInit := imageInitBase
	t.Cleanup(func() {
		imageLoadState = origLoad
		imageInitBase = origInit
	})

	imageLoadState = func(string) (*image.State, error) {
		return nil, image.ErrInitIncomplete
	}
	imageInitBase = func(string, string, image.Runner, int, []string, func(int, string)) (*image.State, error) {
		t.Fatal("imageInitBase should not be called on non-ENOENT load errors")
		return nil, nil
	}

	_, _, err := loadOrInitImageState("/tmp/runtime", "/tmp/repo", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "loading image backend state") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadOrInitImageState_PropagatesInitError(t *testing.T) {
	origLoad := imageLoadState
	origInit := imageInitBase
	t.Cleanup(func() {
		imageLoadState = origLoad
		imageInitBase = origInit
	})

	imageLoadState = func(string) (*image.State, error) {
		return nil, os.ErrNotExist
	}
	imageInitBase = func(string, string, image.Runner, int, []string, func(int, string)) (*image.State, error) {
		return nil, errors.New("hdiutil failed")
	}

	_, _, err := loadOrInitImageState("/tmp/runtime", "/tmp/repo", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "initializing image backend") {
		t.Fatalf("unexpected error: %v", err)
	}
}
