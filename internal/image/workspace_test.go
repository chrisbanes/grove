package image

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateWorkspace_SavesMetadata(t *testing.T) {
	repoRoot := t.TempDir()
	wsPath := filepath.Join(t.TempDir(), "workspaces", "main-a1b2")
	st := &State{
		Backend:        "image",
		BasePath:       filepath.Join(repoRoot, ".grove", "images", "base.sparsebundle"),
		BaseGeneration: 2,
	}

	r := &fakeRunner{
		outputs: [][]byte{
			[]byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>system-entities</key>
  <array>
    <dict><key>dev-entry</key><string>/dev/disk11</string></dict>
    <dict><key>dev-entry</key><string>/dev/disk11s1</string><key>mount-point</key><string>` + wsPath + `</string></dict>
  </array>
</dict>
</plist>`),
		},
	}

	meta, err := CreateWorkspace(repoRoot, repoRoot, wsPath, "main-a1b2", st, r)
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if meta.ID != "main-a1b2" {
		t.Fatalf("expected ID main-a1b2, got %q", meta.ID)
	}
	if meta.Device != "/dev/disk11s1" {
		t.Fatalf("expected device /dev/disk11s1, got %q", meta.Device)
	}
	if meta.BaseGeneration != 2 {
		t.Fatalf("expected base generation 2, got %d", meta.BaseGeneration)
	}

	loaded, err := LoadWorkspaceMeta(repoRoot, "main-a1b2")
	if err != nil {
		t.Fatalf("LoadWorkspaceMeta() error = %v", err)
	}
	if loaded.Device != "/dev/disk11s1" {
		t.Fatalf("expected persisted device /dev/disk11s1, got %q", loaded.Device)
	}
}

func TestCreateWorkspace_CleansUpOnMetadataFailure(t *testing.T) {
	repoRoot := t.TempDir()
	wsPath := filepath.Join(t.TempDir(), "workspaces", "main-a1b2")
	st := &State{
		Backend:        "image",
		BasePath:       filepath.Join(repoRoot, ".grove", "images", "base.sparsebundle"),
		BaseGeneration: 1,
	}

	// Force metadata write failure by making .grove/workspaces a file.
	if err := os.MkdirAll(filepath.Join(repoRoot, ".grove"), 0755); err != nil {
		t.Fatalf("mkdir .grove: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".grove", "workspaces"), []byte("not-a-dir"), 0644); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}

	r := &fakeRunner{
		outputs: [][]byte{
			[]byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>system-entities</key>
  <array>
    <dict><key>dev-entry</key><string>/dev/disk12</string></dict>
    <dict><key>dev-entry</key><string>/dev/disk12s1</string><key>mount-point</key><string>` + wsPath + `</string></dict>
  </array>
</dict>
</plist>`),
		},
	}

	_, err := CreateWorkspace(repoRoot, repoRoot, wsPath, "main-a1b2", st, r)
	if err == nil {
		t.Fatal("expected create failure")
	}
	if len(r.calls) < 2 {
		t.Fatalf("expected attach + detach cleanup calls, got %d", len(r.calls))
	}
	last := r.calls[len(r.calls)-1]
	if last.name != "hdiutil" || len(last.args) < 2 || last.args[0] != "detach" {
		t.Fatalf("expected cleanup detach call, got %+v", last)
	}
}

func TestDestroyWorkspace_DetachesAndRemovesMetadata(t *testing.T) {
	repoRoot := t.TempDir()
	mountpoint := filepath.Join(t.TempDir(), "workspaces", "main-a1b2")
	shadowPath := filepath.Join(repoRoot, ".grove", "shadows", "main-a1b2.shadow")
	if err := os.MkdirAll(filepath.Dir(shadowPath), 0755); err != nil {
		t.Fatalf("mkdir shadow dir: %v", err)
	}
	if err := os.WriteFile(shadowPath, []byte("shadow"), 0644); err != nil {
		t.Fatalf("write shadow: %v", err)
	}
	if err := os.MkdirAll(mountpoint, 0755); err != nil {
		t.Fatalf("mkdir mountpoint: %v", err)
	}
	if err := SaveWorkspaceMeta(repoRoot, &WorkspaceMeta{
		ID:         "main-a1b2",
		Mountpoint: mountpoint,
		Device:     "/dev/disk13s1",
		ShadowPath: shadowPath,
	}); err != nil {
		t.Fatalf("SaveWorkspaceMeta() error = %v", err)
	}

	r := &fakeRunner{}
	if err := DestroyWorkspace(repoRoot, "main-a1b2", r); err != nil {
		t.Fatalf("DestroyWorkspace() error = %v", err)
	}

	if len(r.calls) != 1 {
		t.Fatalf("expected one detach call, got %d", len(r.calls))
	}
	if r.calls[0].name != "hdiutil" || r.calls[0].args[0] != "detach" {
		t.Fatalf("expected hdiutil detach call, got %+v", r.calls[0])
	}
	if _, err := os.Stat(shadowPath); !os.IsNotExist(err) {
		t.Fatalf("expected shadow file removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".grove", "workspaces", "main-a1b2.json")); !os.IsNotExist(err) {
		t.Fatalf("expected metadata removed, stat err = %v", err)
	}
	if _, err := os.Stat(mountpoint); !os.IsNotExist(err) {
		t.Fatalf("expected mountpoint removed, stat err = %v", err)
	}
}

