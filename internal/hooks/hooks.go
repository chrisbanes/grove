package hooks

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Run executes the named hook from the .grove/hooks/ directory within
// repoRoot. The hook runs with its working directory set to repoRoot.
// If the hook doesn't exist, Run returns nil (hooks are optional).
// If the hook exists but is not executable, Run returns an error.
func Run(repoRoot, hookName string) error {
	hookPath := filepath.Join(repoRoot, ".grove", "hooks", hookName)

	info, err := os.Stat(hookPath)
	if os.IsNotExist(err) {
		return nil // hooks are optional
	}
	if err != nil {
		return fmt.Errorf("checking hook %s: %w", hookName, err)
	}

	// Check executable bit
	if info.Mode()&0111 == 0 {
		return fmt.Errorf("hook %s exists but is not executable: chmod +x %s", hookName, hookPath)
	}

	cmd := exec.Command(hookPath)
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hook %s failed: %w", hookName, err)
	}
	return nil
}
