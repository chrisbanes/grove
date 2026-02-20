package image

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStateRoundTrip(t *testing.T) {
	repoRoot := t.TempDir()

	input := &State{
		Backend:        "image",
		BasePath:       ".grove/images/base.sparsebundle",
		BaseGeneration: 3,
		LastSyncCommit: "abc1234",
	}

	if err := SaveState(repoRoot, input); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	got, err := LoadState(repoRoot)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if got.Backend != input.Backend {
		t.Fatalf("Backend: want %q got %q", input.Backend, got.Backend)
	}
	if got.BasePath != input.BasePath {
		t.Fatalf("BasePath: want %q got %q", input.BasePath, got.BasePath)
	}
	if got.BaseGeneration != input.BaseGeneration {
		t.Fatalf("BaseGeneration: want %d got %d", input.BaseGeneration, got.BaseGeneration)
	}
	if got.LastSyncCommit != input.LastSyncCommit {
		t.Fatalf("LastSyncCommit: want %q got %q", input.LastSyncCommit, got.LastSyncCommit)
	}
}

func TestWorkspaceMetaLifecycle(t *testing.T) {
	repoRoot := t.TempDir()

	now := time.Now().UTC().Round(time.Second)
	metaA := &WorkspaceMeta{
		ID:             "main-a1b2",
		Mountpoint:     "/tmp/grove/project/main-a1b2",
		Device:         "/dev/disk7s1",
		ShadowPath:     "/tmp/.grove/shadows/main-a1b2.shadow",
		BaseGeneration: 1,
		CreatedAt:      now,
	}
	metaB := &WorkspaceMeta{
		ID:             "main-c3d4",
		Mountpoint:     "/tmp/grove/project/main-c3d4",
		Device:         "/dev/disk8s1",
		ShadowPath:     "/tmp/.grove/shadows/main-c3d4.shadow",
		BaseGeneration: 1,
		CreatedAt:      now,
	}

	if err := SaveWorkspaceMeta(repoRoot, metaA); err != nil {
		t.Fatalf("SaveWorkspaceMeta(metaA) error = %v", err)
	}
	if err := SaveWorkspaceMeta(repoRoot, metaB); err != nil {
		t.Fatalf("SaveWorkspaceMeta(metaB) error = %v", err)
	}

	gotA, err := LoadWorkspaceMeta(repoRoot, metaA.ID)
	if err != nil {
		t.Fatalf("LoadWorkspaceMeta(metaA) error = %v", err)
	}
	if gotA.Device != metaA.Device {
		t.Fatalf("Device: want %q got %q", metaA.Device, gotA.Device)
	}

	all, err := ListWorkspaceMeta(repoRoot)
	if err != nil {
		t.Fatalf("ListWorkspaceMeta() error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 metadata entries, got %d", len(all))
	}

	if err := DeleteWorkspaceMeta(repoRoot, metaA.ID); err != nil {
		t.Fatalf("DeleteWorkspaceMeta(metaA) error = %v", err)
	}
	all, err = ListWorkspaceMeta(repoRoot)
	if err != nil {
		t.Fatalf("ListWorkspaceMeta() after delete error = %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 metadata entry after delete, got %d", len(all))
	}
	if all[0].ID != metaB.ID {
		t.Fatalf("expected remaining ID %q, got %q", metaB.ID, all[0].ID)
	}
}

func TestListWorkspaceMeta_NoDirectory(t *testing.T) {
	repoRoot := t.TempDir()

	got, err := ListWorkspaceMeta(repoRoot)
	if err != nil {
		t.Fatalf("ListWorkspaceMeta() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty list, got %d entries", len(got))
	}
}

func TestStatePathLocation(t *testing.T) {
	repoRoot := t.TempDir()
	input := &State{Backend: "image", BasePath: ".grove/images/base.sparsebundle", BaseGeneration: 1}
	if err := SaveState(repoRoot, input); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(repoRoot, ".grove", "images", "state.json")); err != nil {
		t.Fatalf("expected state file in .grove/images/state.json: %v", err)
	}
}

