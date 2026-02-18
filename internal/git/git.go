package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// IsRepo returns true if path is inside a git repository.
func IsRepo(path string) bool {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--git-dir")
	return cmd.Run() == nil
}

// IsDirty returns true if the repo at path has uncommitted changes.
func IsDirty(path string) (bool, error) {
	cmd := exec.Command("git", "-C", path, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// CurrentBranch returns the current branch name for the repo at path.
func CurrentBranch(path string) (string, error) {
	cmd := exec.Command("git", "-C", path, "branch", "--show-current")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// CurrentCommit returns the short SHA of HEAD for the repo at path.
func CurrentCommit(path string) (string, error) {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Pull runs git pull in the repo at path.
func Pull(path string) error {
	cmd := exec.Command("git", "-C", path, "pull")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git pull: %w\n%s", err, out)
	}
	return nil
}

// Push pushes a branch to origin.
func Push(path, branch string) error {
	cmd := exec.Command("git", "-C", path, "push", "-u", "origin", branch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git push: %w\n%s", err, out)
	}
	return nil
}

// Checkout checks out a branch. If create is true, creates a new branch.
func Checkout(path, branch string, create bool) error {
	args := []string{"-C", path, "checkout"}
	if create {
		args = append(args, "-b")
	}
	args = append(args, branch)
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git checkout: %w\n%s", err, out)
	}
	return nil
}
