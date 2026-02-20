package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/chrisbanes/grove/internal/backend"
	"github.com/chrisbanes/grove/internal/clone"
	"github.com/chrisbanes/grove/internal/config"
	gitpkg "github.com/chrisbanes/grove/internal/git"
	"github.com/chrisbanes/grove/internal/hooks"
	"github.com/chrisbanes/grove/internal/workspace"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new workspace from the golden copy",
	Long: `Creates a copy-on-write clone of the golden copy, including all build
caches and gitignored files. Builds in the workspace start warm.

Without --branch, the workspace stays on the golden copy's current branch.
With --branch, a new git branch is created and checked out in the workspace.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		progressEnabled, _ := cmd.Flags().GetBool("progress")
		var (
			progressMu sync.Mutex
			progress   *progressRenderer
			cloneState *progressState
		)
		updateProgress := func(percent int, phase string) {
			if progress == nil {
				return
			}
			progressMu.Lock()
			defer progressMu.Unlock()
			progress.Update(percent, phase)
		}
		if progressEnabled {
			progress = newProgressRenderer(os.Stderr, isTerminalFile(os.Stderr))
			defer progress.Done()
			cloneState = newProgressState(5, 95)
			updateProgress(0, "preflight")
		}

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		goldenRoot, err := config.FindGroveRoot(cwd)
		if err != nil {
			return err
		}

		// Don't allow creating workspaces from within a workspace
		if workspace.IsWorkspace(goldenRoot) {
			return fmt.Errorf("cannot create a workspace from inside another workspace.\nRun this from the golden copy instead")
		}

		cfg, err := config.Load(goldenRoot)
		if err != nil {
			return err
		}
		if err := config.EnsureBackendCompatible(goldenRoot, cfg); err != nil {
			return err
		}
		backendImpl, err := backend.ForName(cfg.CloneBackend)
		if err != nil {
			return err
		}

		// Expand workspace dir
		projectName := getProjectName(goldenRoot)
		cfg.WorkspaceDir = config.ExpandWorkspaceDir(cfg.WorkspaceDir, projectName)

		// Check for uncommitted changes
		force, _ := cmd.Flags().GetBool("force")
		dirty, err := gitpkg.IsDirty(goldenRoot)
		if err != nil {
			return fmt.Errorf("checking repo status: %w", err)
		}
		if dirty && !force {
			return fmt.Errorf(
				"golden copy has uncommitted changes.\n" +
					"These changes will be included in the workspace clone.\n" +
					"Use --force to proceed anyway")
		}

		branch, _ := cmd.Flags().GetString("branch")

		// If no branch specified, detect the golden copy's current branch for the ID
		branchForID := branch
		if branchForID == "" {
			if detected, err := gitpkg.CurrentBranch(goldenRoot); err == nil {
				branchForID = detected
			}
		}

		// Get current commit
		commit, _ := gitpkg.CurrentCommit(goldenRoot)

		updateProgress(5, "clone")

		opts := backend.CreateOptions{
			Branch:       branch,
			BranchForID:  branchForID,
			GoldenCommit: commit,
		}
		if progressEnabled {
			opts.OnClone = func(event clone.ProgressEvent) {
				if event.Phase != "clone" {
					return
				}
				progressMu.Lock()
				defer progressMu.Unlock()
				cloneState.updateClone(event.Copied, event.Total)
				progress.Update(cloneState.percent, "clone")
			}
		}

		info, err := backendImpl.CreateWorkspace(goldenRoot, cfg, opts)
		if err != nil {
			updateProgress(100, "failed")
			return err
		}
		updateProgress(95, "post-clone hook")

		// Run post-clone hook
		if err := hooks.Run(info.Path, "post-clone"); err != nil {
			if cleanupErr := backendImpl.DestroyWorkspace(goldenRoot, cfg, info.ID); cleanupErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: cleanup failed for %s: %v\n", info.ID, cleanupErr)
			}
			updateProgress(100, "failed")
			return fmt.Errorf("post-clone hook failed: %w\nWorkspace cleaned up", err)
		}

		// Checkout branch if specified
		if branch != "" {
			updateProgress(99, "branch checkout")
			if err := gitpkg.Checkout(info.Path, branch, true); err != nil {
				// Don't clean up — clone succeeded, branch is secondary
				fmt.Fprintf(os.Stderr, "Warning: branch checkout failed: %v\n", err)
			}
			info.Branch = branch
		}
		updateProgress(100, "done")

		// Output result
		jsonOut, _ := cmd.Flags().GetBool("json")
		if jsonOut {
			data, _ := json.MarshalIndent(info, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("Workspace created: %s\n", info.ID)
			fmt.Printf("Path: %s\n", info.Path)
			if branch != "" {
				fmt.Printf("Branch: %s\n", branch)
			}
		}

		return nil
	},
}

func getProjectName(repoRoot string) string {
	return filepath.Base(repoRoot)
}

func init() {
	createCmd.Flags().String("branch", "", "Create and checkout a new git branch in the workspace (default: golden copy's current branch)")
	createCmd.Flags().Bool("force", false, "Proceed even if golden copy has uncommitted changes")
	createCmd.Flags().Bool("json", false, "Output workspace info as JSON")
	createCmd.Flags().Bool("progress", false, "Show progress output for long-running create operations")
	rootCmd.AddCommand(createCmd)
}
