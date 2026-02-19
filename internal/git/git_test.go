package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chrisbanes/grove/internal/git"
)

func setupRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
	f := filepath.Join(dir, "README.md")
	os.WriteFile(f, []byte("# test"), 0644)
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "init")
	return dir
}

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %s\n%s", name, args, err, out)
	}
}

func runOutput(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %s\n%s", name, args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestIsRepo(t *testing.T) {
	repo := setupRepo(t)
	if !git.IsRepo(repo) {
		t.Error("expected true for git repo")
	}
	if git.IsRepo(t.TempDir()) {
		t.Error("expected false for non-repo dir")
	}
}

func TestIsDirty_Clean(t *testing.T) {
	repo := setupRepo(t)
	dirty, err := git.IsDirty(repo)
	if err != nil {
		t.Fatal(err)
	}
	if dirty {
		t.Error("expected clean repo")
	}
}

func TestIsDirty_Dirty(t *testing.T) {
	repo := setupRepo(t)
	os.WriteFile(filepath.Join(repo, "new.txt"), []byte("change"), 0644)
	dirty, err := git.IsDirty(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Error("expected dirty repo")
	}
}

func TestCurrentBranch(t *testing.T) {
	repo := setupRepo(t)
	branch, err := git.CurrentBranch(repo)
	if err != nil {
		t.Fatal(err)
	}
	if branch == "" {
		t.Error("expected non-empty branch")
	}
}

func TestCurrentCommit(t *testing.T) {
	repo := setupRepo(t)
	commit, err := git.CurrentCommit(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(commit) < 7 {
		t.Errorf("expected short hash, got %q", commit)
	}
}

func TestCheckout_NewBranch(t *testing.T) {
	repo := setupRepo(t)
	err := git.Checkout(repo, "feature/test", true)
	if err != nil {
		t.Fatal(err)
	}
	branch, _ := git.CurrentBranch(repo)
	if branch != "feature/test" {
		t.Errorf("expected feature/test, got %s", branch)
	}
}

func TestCheckout_ExistingBranch(t *testing.T) {
	repo := setupRepo(t)
	// Create a branch, then switch back to original
	git.Checkout(repo, "feature/test", true)
	run(t, repo, "git", "checkout", "-")

	// Now checkout the existing branch without create flag
	err := git.Checkout(repo, "feature/test", false)
	if err != nil {
		t.Fatal(err)
	}
	branch, _ := git.CurrentBranch(repo)
	if branch != "feature/test" {
		t.Errorf("expected feature/test, got %s", branch)
	}
}

func TestCheckout_ErrorIncludesGitOutput(t *testing.T) {
	repo := setupRepo(t)
	err := git.Checkout(repo, "definitely-missing-branch", false)
	if err == nil {
		t.Fatal("expected checkout to fail for missing branch")
	}
	msg := err.Error()
	if !strings.Contains(msg, "git checkout") {
		t.Errorf("expected wrapped command name, got: %s", msg)
	}
	if !strings.Contains(msg, "pathspec") {
		t.Errorf("expected git stderr in error, got: %s", msg)
	}
}

func TestPush_ErrorIncludesGitOutput(t *testing.T) {
	repo := setupRepo(t)
	branch, err := git.CurrentBranch(repo)
	if err != nil {
		t.Fatal(err)
	}
	err = git.Push(repo, branch)
	if err == nil {
		t.Fatal("expected push to fail without an origin remote")
	}
	msg := err.Error()
	if !strings.Contains(msg, "git push") {
		t.Errorf("expected wrapped command name, got: %s", msg)
	}
	if !strings.Contains(msg, "fatal") {
		t.Errorf("expected git stderr in error, got: %s", msg)
	}
}

func TestPull_WithoutUpstream_UsesCurrentBranchAndSetsTracking(t *testing.T) {
	remoteRoot := t.TempDir()
	bareRepo := filepath.Join(remoteRoot, "remote.git")
	run(t, remoteRoot, "git", "init", "--bare", bareRepo)

	seedRepo := t.TempDir()
	run(t, remoteRoot, "git", "clone", bareRepo, seedRepo)
	run(t, seedRepo, "git", "config", "user.email", "test@test.com")
	run(t, seedRepo, "git", "config", "user.name", "Test")
	os.WriteFile(filepath.Join(seedRepo, "README.md"), []byte("# seed"), 0644)
	run(t, seedRepo, "git", "add", ".")
	run(t, seedRepo, "git", "commit", "-m", "seed")
	branch := runOutput(t, seedRepo, "git", "branch", "--show-current")
	run(t, seedRepo, "git", "push", "-u", "origin", branch)

	localRepo := t.TempDir()
	run(t, remoteRoot, "git", "clone", bareRepo, localRepo)
	run(t, localRepo, "git", "config", "user.email", "test@test.com")
	run(t, localRepo, "git", "config", "user.name", "Test")
	run(t, localRepo, "git", "branch", "--unset-upstream")

	pusherRepo := t.TempDir()
	run(t, remoteRoot, "git", "clone", bareRepo, pusherRepo)
	run(t, pusherRepo, "git", "config", "user.email", "test@test.com")
	run(t, pusherRepo, "git", "config", "user.name", "Test")
	os.WriteFile(filepath.Join(pusherRepo, "new-file.txt"), []byte("from remote"), 0644)
	run(t, pusherRepo, "git", "add", ".")
	run(t, pusherRepo, "git", "commit", "-m", "new file")
	run(t, pusherRepo, "git", "push", "origin", branch)

	if err := git.Pull(localRepo); err != nil {
		t.Fatalf("expected pull to succeed without upstream tracking, got: %v", err)
	}

	if _, err := os.Stat(filepath.Join(localRepo, "new-file.txt")); err != nil {
		t.Fatalf("expected pulled file in local repo, got: %v", err)
	}

	upstream := runOutput(t, localRepo, "git", "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if upstream != "origin/"+branch {
		t.Fatalf("expected upstream to be origin/%s, got %s", branch, upstream)
	}
}
