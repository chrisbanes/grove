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
	if err == nil {
		return nil
	}

	// If the current branch has no upstream configured, retry by explicitly
	// pulling the current branch from a sensible remote and then restore tracking.
	if strings.Contains(string(out), "There is no tracking information for the current branch.") {
		branch, branchErr := CurrentBranch(path)
		if branchErr != nil || branch == "" {
			return fmt.Errorf("git pull: %w\n%s", err, out)
		}

		remote, remoteErr := pullRemote(path, branch)
		if remoteErr != nil {
			return fmt.Errorf("git pull: %w\n%s", err, out)
		}

		fallback := exec.Command("git", "-C", path, "pull", remote, branch)
		fallbackOut, fallbackErr := fallback.CombinedOutput()
		if fallbackErr != nil {
			return fmt.Errorf("git pull: %w\n%s", fallbackErr, fallbackOut)
		}

		setUpstream := exec.Command("git", "-C", path, "branch", "--set-upstream-to="+remote+"/"+branch, branch)
		setUpstreamOut, setUpstreamErr := setUpstream.CombinedOutput()
		if setUpstreamErr != nil {
			return fmt.Errorf("git branch --set-upstream-to: %w\n%s", setUpstreamErr, setUpstreamOut)
		}
		return nil
	}

	return fmt.Errorf("git pull: %w\n%s", err, out)
}

func pullRemote(path, branch string) (string, error) {
	branchRemote := exec.Command("git", "-C", path, "config", "--get", "branch."+branch+".remote")
	branchRemoteOut, branchRemoteErr := branchRemote.Output()
	if branchRemoteErr == nil {
		remote := strings.TrimSpace(string(branchRemoteOut))
		if remote != "" {
			return remote, nil
		}
	}

	remotesCmd := exec.Command("git", "-C", path, "remote")
	remotesOut, remotesErr := remotesCmd.Output()
	if remotesErr != nil {
		return "", remotesErr
	}

	remotes := strings.Fields(string(remotesOut))
	if len(remotes) == 0 {
		return "", fmt.Errorf("no git remotes configured")
	}

	for _, remote := range remotes {
		if remote == "origin" {
			return remote, nil
		}
	}

	if len(remotes) == 1 {
		return remotes[0], nil
	}

	return "", fmt.Errorf("multiple remotes configured and no tracking remote for branch %s", branch)
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
