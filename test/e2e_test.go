package test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/AmpInc/grove/internal/workspace"
)

func buildGrove(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "grove")
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/grove")
	// Build from the repo root
	cmd.Dir = filepath.Join(repoRoot(t))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build grove: %s\n%s", err, out)
	}
	return binary
}

func repoRoot(t *testing.T) string {
	t.Helper()
	// Walk up from test/ to find go.mod
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}
}

func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("build/\n.grove/\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
	os.MkdirAll(filepath.Join(dir, "build"), 0755)
	os.WriteFile(filepath.Join(dir, "build", "output.bin"), []byte("compiled"), 0644)
	run(t, dir, "git", "add", "main.go", ".gitignore")
	run(t, dir, "git", "commit", "-m", "init")
	return dir
}

func run(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %s\n%s", name, args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func grove(t *testing.T, binary, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("grove %v failed: %s\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestFullLifecycle(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}

	binary := buildGrove(t)
	repo := setupTestRepo(t)

	// grove init
	out := grove(t, binary, repo, "init")
	if !strings.Contains(out, "Grove initialized") {
		t.Fatalf("unexpected init output: %s", out)
	}

	// Verify .grove/config.json exists
	if _, err := os.Stat(filepath.Join(repo, ".grove", "config.json")); err != nil {
		t.Fatal("config.json not created")
	}

	// grove create --json --branch test-feature
	out = grove(t, binary, repo, "create", "--json", "--branch", "test-feature")
	var info workspace.Info
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		t.Fatalf("invalid JSON output: %s\n%s", err, out)
	}
	if info.ID == "" || info.Path == "" {
		t.Fatal("missing ID or Path in output")
	}

	// Verify source files in workspace
	data, err := os.ReadFile(filepath.Join(info.Path, "main.go"))
	if err != nil {
		t.Fatal("main.go not in workspace")
	}
	if string(data) != "package main\n" {
		t.Error("main.go content mismatch")
	}

	// Verify gitignored build artifacts are cloned
	data, err = os.ReadFile(filepath.Join(info.Path, "build", "output.bin"))
	if err != nil {
		t.Fatal("build/output.bin not in workspace — gitignored files not cloned")
	}
	if string(data) != "compiled" {
		t.Error("build artifact content mismatch")
	}

	// Verify CoW isolation — modify workspace, check golden unchanged
	os.WriteFile(filepath.Join(info.Path, "main.go"), []byte("modified\n"), 0644)
	origData, _ := os.ReadFile(filepath.Join(repo, "main.go"))
	if string(origData) != "package main\n" {
		t.Error("golden copy was modified — CoW isolation broken")
	}

	// grove list
	out = grove(t, binary, repo, "list")
	if !strings.Contains(out, info.ID) {
		t.Errorf("list output doesn't contain workspace ID: %s", out)
	}

	// grove status
	out = grove(t, binary, repo, "status")
	if !strings.Contains(out, "1 / ") {
		t.Errorf("status doesn't show 1 workspace: %s", out)
	}

	// grove destroy
	grove(t, binary, repo, "destroy", info.ID)
	if _, err := os.Stat(info.Path); !os.IsNotExist(err) {
		t.Error("workspace not cleaned up after destroy")
	}

	// grove list should be empty
	out = grove(t, binary, repo, "list")
	if strings.Contains(out, info.ID) {
		t.Error("destroyed workspace still in list")
	}
}

func TestPostCloneHook(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}

	binary := buildGrove(t)
	repo := setupTestRepo(t)

	grove(t, binary, repo, "init")

	// Create a post-clone hook that creates a marker file
	hookPath := filepath.Join(repo, ".grove", "hooks", "post-clone")
	os.WriteFile(hookPath, []byte("#!/bin/bash\ntouch hook-executed\n"), 0755)

	out := grove(t, binary, repo, "create", "--json")
	var info workspace.Info
	json.Unmarshal([]byte(out), &info)

	if _, err := os.Stat(filepath.Join(info.Path, "hook-executed")); err != nil {
		t.Error("post-clone hook did not run")
	}

	// Cleanup
	grove(t, binary, repo, "destroy", "--all")
}

func TestDirtyGoldenCopy(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}

	binary := buildGrove(t)
	repo := setupTestRepo(t)

	grove(t, binary, repo, "init") // init on a clean repo

	// Make golden copy dirty
	os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("wip"), 0644)

	// create should fail without --force
	cmd := exec.Command(binary, "create")
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for dirty golden copy")
	}
	if !strings.Contains(string(out), "uncommitted changes") {
		t.Errorf("expected uncommitted changes error, got: %s", out)
	}

	// create should succeed with --force
	grove(t, binary, repo, "create", "--force")

	// Cleanup
	grove(t, binary, repo, "destroy", "--all")
}
