package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
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
