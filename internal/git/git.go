package git

import (
	"os/exec"
	"strings"
)

func IsRepo(path string) bool {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--git-dir")
	return cmd.Run() == nil
}

func IsDirty(path string) (bool, error) {
	cmd := exec.Command("git", "-C", path, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func CurrentBranch(path string) (string, error) {
	cmd := exec.Command("git", "-C", path, "branch", "--show-current")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func CurrentCommit(path string) (string, error) {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func Pull(path string) error {
	cmd := exec.Command("git", "-C", path, "pull")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// Push pushes a branch to origin.
func Push(path, branch string) error {
	cmd := exec.Command("git", "-C", path, "push", "-u", "origin", branch)
	return cmd.Run()
}

func Checkout(path, branch string, create bool) error {
	args := []string{"-C", path, "checkout"}
	if create {
		args = append(args, "-b")
	}
	args = append(args, branch)
	cmd := exec.Command("git", args...)
	return cmd.Run()
}

func RepoRoot(path string) (string, error) {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
