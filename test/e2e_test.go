package test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/AmpInc/grove/internal/config"
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

func groveExpectErr(t *testing.T, binary, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("grove %v succeeded but expected failure.\nOutput: %s", args, out)
	}
	return string(out)
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
	if err := os.WriteFile(hookPath, []byte("#!/bin/bash\ntouch hook-executed\n"), 0755); err != nil {
		t.Fatalf("failed to write hook: %v", err)
	}

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

func TestInitEdgeCases(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	binary := buildGrove(t)

	t.Run("non-git-directory", func(t *testing.T) {
		dir := t.TempDir()
		out := groveExpectErr(t, binary, dir, "init")
		if !strings.Contains(out, "is not a git repository") {
			t.Errorf("expected 'is not a git repository', got: %s", out)
		}
	})

	t.Run("already-initialized", func(t *testing.T) {
		repo := setupTestRepo(t)
		grove(t, binary, repo, "init")
		out := groveExpectErr(t, binary, repo, "init")
		if !strings.Contains(out, "grove already initialized") {
			t.Errorf("expected 'grove already initialized', got: %s", out)
		}
	})

	t.Run("warmup-command", func(t *testing.T) {
		repo := setupTestRepo(t)
		out := grove(t, binary, repo, "init", "--warmup-command", "touch warmup-marker")
		if !strings.Contains(out, "Running warmup") {
			t.Errorf("expected warmup output, got: %s", out)
		}
		if _, err := os.Stat(filepath.Join(repo, "warmup-marker")); err != nil {
			t.Error("warmup command did not create marker file")
		}
	})

	t.Run("explicit-path-argument", func(t *testing.T) {
		repo := setupTestRepo(t)
		otherDir := t.TempDir()
		grove(t, binary, otherDir, "init", repo)
		if _, err := os.Stat(filepath.Join(repo, ".grove", "config.json")); err != nil {
			t.Error("init with explicit path did not create .grove/config.json at target")
		}
		if _, err := os.Stat(filepath.Join(otherDir, ".grove")); err == nil {
			t.Error(".grove was incorrectly created in the working directory")
		}
	})
}

func TestCreateEdgeCases(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	binary := buildGrove(t)

	t.Run("from-inside-workspace", func(t *testing.T) {
		repo := setupTestRepo(t)
		grove(t, binary, repo, "init")
		out := grove(t, binary, repo, "create", "--json")
		var info workspace.Info
		if err := json.Unmarshal([]byte(out), &info); err != nil {
			t.Fatalf("invalid JSON from create: %s\n%s", err, out)
		}

		errOut := groveExpectErr(t, binary, info.Path, "create")
		if !strings.Contains(errOut, "cannot create a workspace from inside another workspace") {
			t.Errorf("expected workspace-inside-workspace error, got: %s", errOut)
		}
		grove(t, binary, repo, "destroy", "--all")
	})

	t.Run("max-workspaces-reached", func(t *testing.T) {
		repo := setupTestRepo(t)
		grove(t, binary, repo, "init")

		// Patch config to allow only 1 workspace
		cfgPath := filepath.Join(repo, ".grove", "config.json")
		cfgData, _ := os.ReadFile(cfgPath)
		var cfg config.Config
		json.Unmarshal(cfgData, &cfg)
		cfg.MaxWorkspaces = 1
		patched, _ := json.MarshalIndent(&cfg, "", "  ")
		os.WriteFile(cfgPath, patched, 0644)

		grove(t, binary, repo, "create")
		errOut := groveExpectErr(t, binary, repo, "create")
		if !strings.Contains(errOut, "max workspaces") {
			t.Errorf("expected max workspaces error, got: %s", errOut)
		}
		grove(t, binary, repo, "destroy", "--all")
	})

	t.Run("failed-post-clone-hook-cleanup", func(t *testing.T) {
		repo := setupTestRepo(t)
		grove(t, binary, repo, "init")

		hookPath := filepath.Join(repo, ".grove", "hooks", "post-clone")
		if err := os.WriteFile(hookPath, []byte("#!/bin/bash\nexit 1\n"), 0755); err != nil {
			t.Fatalf("failed to write hook: %v", err)
		}

		errOut := groveExpectErr(t, binary, repo, "create")
		if !strings.Contains(errOut, "post-clone hook failed") {
			t.Errorf("expected hook failure message, got: %s", errOut)
		}

		// Verify no workspaces remain (directory was cleaned up)
		listOut := grove(t, binary, repo, "list")
		if !strings.Contains(listOut, "No active workspaces") {
			t.Errorf("expected no workspaces after failed hook, got: %s", listOut)
		}
	})

	t.Run("no-branch-stays-on-HEAD", func(t *testing.T) {
		repo := setupTestRepo(t)
		grove(t, binary, repo, "init")

		goldenCommit := run(t, repo, "git", "rev-parse", "--short", "HEAD")

		out := grove(t, binary, repo, "create", "--json")
		var info workspace.Info
		if err := json.Unmarshal([]byte(out), &info); err != nil {
			t.Fatalf("invalid JSON from create: %s\n%s", err, out)
		}

		wsCommit := run(t, info.Path, "git", "rev-parse", "--short", "HEAD")
		if wsCommit != goldenCommit {
			t.Errorf("workspace HEAD %s != golden HEAD %s", wsCommit, goldenCommit)
		}
		grove(t, binary, repo, "destroy", "--all")
	})
}

func TestDestroyEdgeCases(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	binary := buildGrove(t)

	t.Run("by-absolute-path", func(t *testing.T) {
		repo := setupTestRepo(t)
		grove(t, binary, repo, "init")
		out := grove(t, binary, repo, "create", "--json")
		var info workspace.Info
		json.Unmarshal([]byte(out), &info)

		grove(t, binary, repo, "destroy", info.Path)
		if _, err := os.Stat(info.Path); !os.IsNotExist(err) {
			t.Error("workspace not cleaned up after destroy by path")
		}
	})

	t.Run("all-with-no-workspaces", func(t *testing.T) {
		repo := setupTestRepo(t)
		grove(t, binary, repo, "init")
		out := grove(t, binary, repo, "destroy", "--all")
		if !strings.Contains(out, "No workspaces to destroy") {
			t.Errorf("expected 'No workspaces to destroy', got: %s", out)
		}
	})

	t.Run("nonexistent-id", func(t *testing.T) {
		repo := setupTestRepo(t)
		grove(t, binary, repo, "init")
		out := groveExpectErr(t, binary, repo, "destroy", "nonexistent-id-abc123")
		if !strings.Contains(out, "workspace not found") {
			t.Errorf("expected 'workspace not found', got: %s", out)
		}
	})

	t.Run("no-args-no-all", func(t *testing.T) {
		repo := setupTestRepo(t)
		grove(t, binary, repo, "init")
		out := groveExpectErr(t, binary, repo, "destroy")
		if !strings.Contains(out, "provide a workspace ID or path") {
			t.Errorf("expected usage error, got: %s", out)
		}
	})
}

func TestMultiWorkspaceIsolation(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	binary := buildGrove(t)
	repo := setupTestRepo(t)
	grove(t, binary, repo, "init")

	// Create two workspaces
	out1 := grove(t, binary, repo, "create", "--json", "--branch", "ws1-branch")
	var ws1 workspace.Info
	if err := json.Unmarshal([]byte(out1), &ws1); err != nil {
		t.Fatalf("invalid JSON from create (ws1): %s\n%s", err, out1)
	}

	out2 := grove(t, binary, repo, "create", "--json", "--branch", "ws2-branch")
	var ws2 workspace.Info
	if err := json.Unmarshal([]byte(out2), &ws2); err != nil {
		t.Fatalf("invalid JSON from create (ws2): %s\n%s", err, out2)
	}

	// Modify workspace 1
	os.WriteFile(filepath.Join(ws1.Path, "ws1-only.txt"), []byte("ws1"), 0644)

	// Verify ws2 does not have ws1's file
	if _, err := os.Stat(filepath.Join(ws2.Path, "ws1-only.txt")); err == nil {
		t.Error("ws2 has ws1's file — isolation broken between workspaces")
	}
	// Verify golden does not have ws1's file
	if _, err := os.Stat(filepath.Join(repo, "ws1-only.txt")); err == nil {
		t.Error("golden has ws1's file — CoW isolation broken")
	}
	// Verify ws2 has original golden content
	data, err := os.ReadFile(filepath.Join(ws2.Path, "main.go"))
	if err != nil || string(data) != "package main\n" {
		t.Error("ws2 main.go content doesn't match golden")
	}

	// Destroy ws1, verify ws2 still works
	grove(t, binary, repo, "destroy", ws1.ID)
	if _, err := os.Stat(ws1.Path); !os.IsNotExist(err) {
		t.Error("ws1 not cleaned up after destroy")
	}
	data, err = os.ReadFile(filepath.Join(ws2.Path, "main.go"))
	if err != nil || string(data) != "package main\n" {
		t.Error("ws2 broken after destroying ws1")
	}

	// List should show exactly 1 workspace
	listOut := grove(t, binary, repo, "list", "--json")
	var remaining []workspace.Info
	json.Unmarshal([]byte(listOut), &remaining)
	if len(remaining) != 1 {
		t.Errorf("expected 1 remaining workspace, got %d", len(remaining))
	}
	if len(remaining) == 1 && remaining[0].ID != ws2.ID {
		t.Errorf("remaining workspace should be ws2 (%s), got %s", ws2.ID, remaining[0].ID)
	}

	grove(t, binary, repo, "destroy", "--all")
}

func TestListOutput(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	binary := buildGrove(t)

	t.Run("no-workspaces", func(t *testing.T) {
		repo := setupTestRepo(t)
		grove(t, binary, repo, "init")
		out := grove(t, binary, repo, "list")
		if !strings.Contains(out, "No active workspaces") {
			t.Errorf("expected 'No active workspaces', got: %s", out)
		}
	})

	t.Run("json-multiple-workspaces", func(t *testing.T) {
		repo := setupTestRepo(t)
		grove(t, binary, repo, "init")

		out1 := grove(t, binary, repo, "create", "--json")
		var info1 workspace.Info
		if err := json.Unmarshal([]byte(out1), &info1); err != nil {
			t.Fatalf("invalid JSON from create: %s\n%s", err, out1)
		}

		out2 := grove(t, binary, repo, "create", "--json", "--branch", "feature-x")
		var info2 workspace.Info
		if err := json.Unmarshal([]byte(out2), &info2); err != nil {
			t.Fatalf("invalid JSON from create: %s\n%s", err, out2)
		}

		listOut := grove(t, binary, repo, "list", "--json")
		var list []workspace.Info
		if err := json.Unmarshal([]byte(listOut), &list); err != nil {
			t.Fatalf("list --json output is not valid JSON array: %s\n%s", err, listOut)
		}
		if len(list) != 2 {
			t.Fatalf("expected 2 workspaces in JSON list, got %d", len(list))
		}

		ids := map[string]bool{}
		for _, ws := range list {
			ids[ws.ID] = true
			if ws.Path == "" {
				t.Error("workspace in list missing Path")
			}
			if ws.GoldenCopy == "" {
				t.Error("workspace in list missing GoldenCopy")
			}
			if ws.CreatedAt.IsZero() {
				t.Error("workspace in list missing CreatedAt")
			}
		}
		if !ids[info1.ID] {
			t.Errorf("workspace %s not found in list", info1.ID)
		}
		if !ids[info2.ID] {
			t.Errorf("workspace %s not found in list", info2.ID)
		}

		grove(t, binary, repo, "destroy", "--all")
	})
}

func TestStatusOutput(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	binary := buildGrove(t)

	t.Run("from-inside-workspace", func(t *testing.T) {
		repo := setupTestRepo(t)
		grove(t, binary, repo, "init")
		out := grove(t, binary, repo, "create", "--json")
		var info workspace.Info
		if err := json.Unmarshal([]byte(out), &info); err != nil {
			t.Fatalf("invalid JSON from create: %s\n%s", err, out)
		}

		// grove status from inside the workspace finds the workspace's .grove/config.json
		// (CoW clone includes it) and detects workspace.json → prints the warning.
		statusOut := grove(t, binary, info.Path, "status")
		if !strings.Contains(statusOut, "You are inside a grove workspace") {
			t.Errorf("expected workspace warning, got: %s", statusOut)
		}
		grove(t, binary, repo, "destroy", "--all")
	})

	t.Run("dirty-golden-copy", func(t *testing.T) {
		repo := setupTestRepo(t)
		grove(t, binary, repo, "init")
		os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("wip"), 0644)

		statusOut := grove(t, binary, repo, "status")
		if !strings.Contains(statusOut, "dirty") {
			t.Errorf("expected 'dirty' in status, got: %s", statusOut)
		}
	})
}

func TestUpdateWithLocalRemote(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	binary := buildGrove(t)

	// Create a bare repo to act as remote
	bareRepo := filepath.Join(t.TempDir(), "remote.git")
	run(t, "/", "git", "init", "--bare", bareRepo)

	// Clone it to create the golden copy
	goldenDir := t.TempDir()
	run(t, "/", "git", "clone", bareRepo, goldenDir)
	run(t, goldenDir, "git", "config", "user.email", "test@test.com")
	run(t, goldenDir, "git", "config", "user.name", "Test")

	// Create initial content and push
	os.WriteFile(filepath.Join(goldenDir, "main.go"), []byte("package main\n"), 0644)
	os.WriteFile(filepath.Join(goldenDir, ".gitignore"), []byte(".grove/\n"), 0644)
	run(t, goldenDir, "git", "add", ".")
	run(t, goldenDir, "git", "commit", "-m", "initial")
	// Detect the default branch name
	branch := run(t, goldenDir, "git", "branch", "--show-current")
	run(t, goldenDir, "git", "push", "-u", "origin", branch)

	// Initialize grove with a warmup command
	grove(t, binary, goldenDir, "init", "--warmup-command", "touch warmup-ran")

	// Make a new commit via a separate clone
	pusher := t.TempDir()
	run(t, "/", "git", "clone", bareRepo, pusher)
	run(t, pusher, "git", "config", "user.email", "test@test.com")
	run(t, pusher, "git", "config", "user.name", "Test")
	os.WriteFile(filepath.Join(pusher, "new-file.txt"), []byte("from remote"), 0644)
	run(t, pusher, "git", "add", ".")
	run(t, pusher, "git", "commit", "-m", "add new file")
	run(t, pusher, "git", "push")

	// Remove warmup marker from init
	os.Remove(filepath.Join(goldenDir, "warmup-ran"))

	// Run grove update
	updateOut := grove(t, binary, goldenDir, "update")
	if !strings.Contains(updateOut, "Running warmup") {
		t.Errorf("expected warmup to run during update, got: %s", updateOut)
	}
	if !strings.Contains(updateOut, "Golden copy updated") {
		t.Errorf("expected update success message, got: %s", updateOut)
	}

	// Verify new file was pulled
	if _, err := os.Stat(filepath.Join(goldenDir, "new-file.txt")); err != nil {
		t.Error("new-file.txt not present after update — git pull did not work")
	}

	// Verify warmup ran
	if _, err := os.Stat(filepath.Join(goldenDir, "warmup-ran")); err != nil {
		t.Error("warmup command did not run during update")
	}
}

func TestListJsonEmpty(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	binary := buildGrove(t)
	repo := setupTestRepo(t)
	grove(t, binary, repo, "init")

	out := strings.TrimSpace(grove(t, binary, repo, "list", "--json"))
	// json.MarshalIndent on a nil slice returns "null". This is the current known
	// behavior. The test guards against the human-readable "No active workspaces."
	// string leaking into --json output, which would break machine consumers.
	if out == "No active workspaces." || !json.Valid([]byte(out)) {
		t.Errorf("list --json with no workspaces must be valid JSON, got: %s", out)
	}
}
