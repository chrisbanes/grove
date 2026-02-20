package image

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestInitBase_CreatesStateAndRunsCommands(t *testing.T) {
	repoRoot := t.TempDir()

	r := &fakeRunner{
		outputs: [][]byte{
			nil,
			[]byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>system-entities</key>
  <array>
    <dict><key>dev-entry</key><string>/dev/disk9</string></dict>
    <dict><key>dev-entry</key><string>/dev/disk9s1</string><key>mount-point</key><string>` + filepath.Join(repoRoot, ".grove", "mnt", "base") + `</string></dict>
  </array>
</dict>
</plist>`),
		},
	}

	st, err := InitBase(repoRoot, r, 20, nil)
	if err != nil {
		t.Fatalf("InitBase() error = %v", err)
	}
	if st.Backend != "image" {
		t.Fatalf("expected backend image, got %q", st.Backend)
	}
	if st.BaseGeneration != 1 {
		t.Fatalf("expected base generation 1, got %d", st.BaseGeneration)
	}
	if st.BasePath != filepath.Join(repoRoot, ".grove", "images", "base.sparsebundle") {
		t.Fatalf("unexpected base path %q", st.BasePath)
	}

	loaded, err := LoadState(repoRoot)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if loaded.BasePath != st.BasePath {
		t.Fatalf("state not persisted correctly: %q vs %q", loaded.BasePath, st.BasePath)
	}

	if len(r.calls) != 4 {
		t.Fatalf("expected 4 command calls, got %d", len(r.calls))
	}
	if r.calls[0].name != "hdiutil" || r.calls[1].name != "hdiutil" || r.calls[2].name != "rsync" || r.calls[3].name != "hdiutil" {
		t.Fatalf("unexpected command sequence: %+v", r.calls)
	}
}

func TestInitBase_CallsOnProgress(t *testing.T) {
	repoRoot := t.TempDir()

	r := &fakeRunner{
		outputs: [][]byte{
			nil, // CreateSparseBundle
			[]byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>system-entities</key>
  <array>
    <dict><key>dev-entry</key><string>/dev/disk9</string></dict>
    <dict><key>dev-entry</key><string>/dev/disk9s1</string><key>mount-point</key><string>` + filepath.Join(repoRoot, ".grove", "mnt", "base") + `</string></dict>
  </array>
</dict>
</plist>`), // Attach
		},
		streamLines: []string{
			"  4,585,881,600  50%  109.38MB/s    0:01:02",
			"  7,643,136,000 100%  109.38MB/s    0:01:02 (xfr#1, to-chk=0/100)",
		},
	}

	var phases []string
	var percents []int
	onProgress := func(pct int, phase string) {
		phases = append(phases, phase)
		percents = append(percents, pct)
	}

	st, err := InitBase(repoRoot, r, 20, onProgress)
	if err != nil {
		t.Fatalf("InitBase() error = %v", err)
	}
	if st.Backend != "image" {
		t.Fatalf("expected backend image, got %q", st.Backend)
	}

	// Should have progress callbacks: creating base image, syncing (2 rsync updates), done
	if len(phases) < 3 {
		t.Fatalf("expected at least 3 progress callbacks, got %d: phases=%v percents=%v", len(phases), phases, percents)
	}
	if phases[0] != "creating base image" {
		t.Fatalf("expected first phase 'creating base image', got %q", phases[0])
	}
	if phases[len(phases)-1] != "done" {
		t.Fatalf("expected last phase 'done', got %q", phases[len(phases)-1])
	}
	if percents[len(percents)-1] != 100 {
		t.Fatalf("expected final percent 100, got %d", percents[len(percents)-1])
	}

	// Verify SyncBaseWithProgress was used (Stream called) instead of SyncBase (CombinedOutput)
	if len(r.streamCalls) != 1 {
		t.Fatalf("expected 1 stream call for rsync, got %d", len(r.streamCalls))
	}
	if r.streamCalls[0].name != "rsync" {
		t.Fatalf("expected stream call to rsync, got %q", r.streamCalls[0].name)
	}
}

func TestRefreshBase_RefusesWhenWorkspacesExist(t *testing.T) {
	repoRoot := t.TempDir()
	if err := SaveState(repoRoot, &State{
		Backend:        "image",
		BasePath:       filepath.Join(repoRoot, ".grove", "images", "base.sparsebundle"),
		BaseGeneration: 1,
	}); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	if err := SaveWorkspaceMeta(repoRoot, &WorkspaceMeta{
		ID:         "main-a1b2",
		Mountpoint: "/tmp/grove/main-a1b2",
		Device:     "/dev/disk7s1",
		ShadowPath: "/tmp/main-a1b2.shadow",
	}); err != nil {
		t.Fatalf("SaveWorkspaceMeta() error = %v", err)
	}

	r := &fakeRunner{}
	_, err := RefreshBase(repoRoot, repoRoot, r, "abc1234", nil)
	if err == nil {
		t.Fatal("expected refresh to fail with active workspaces")
	}
	if !strings.Contains(err.Error(), "active image workspaces") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.calls) != 0 {
		t.Fatalf("expected no command calls, got %d", len(r.calls))
	}
}

func TestRefreshBase_UpdatesGenerationAndCommit(t *testing.T) {
	repoRoot := t.TempDir()
	basePath := filepath.Join(repoRoot, ".grove", "images", "base.sparsebundle")
	if err := SaveState(repoRoot, &State{
		Backend:        "image",
		BasePath:       basePath,
		BaseGeneration: 2,
	}); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	r := &fakeRunner{
		outputs: [][]byte{
			[]byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>system-entities</key>
  <array>
    <dict><key>dev-entry</key><string>/dev/disk9</string></dict>
    <dict><key>dev-entry</key><string>/dev/disk9s1</string><key>mount-point</key><string>` + filepath.Join(repoRoot, ".grove", "mnt", "base") + `</string></dict>
  </array>
</dict>
</plist>`),
		},
	}

	updated, err := RefreshBase(repoRoot, repoRoot, r, "abc1234", nil)
	if err != nil {
		t.Fatalf("RefreshBase() error = %v", err)
	}
	if updated.BaseGeneration != 3 {
		t.Fatalf("expected generation 3, got %d", updated.BaseGeneration)
	}
	if updated.LastSyncCommit != "abc1234" {
		t.Fatalf("expected last sync commit abc1234, got %q", updated.LastSyncCommit)
	}

	persisted, err := LoadState(repoRoot)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if persisted.BaseGeneration != 3 {
		t.Fatalf("expected persisted generation 3, got %d", persisted.BaseGeneration)
	}
}

