package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
caches and gitignored files. Builds in the workspace start warm.`,
	RunE: func(cmd *cobra.Command, args []string) error {
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

		// Get CoW cloner
		cloner, err := clone.NewCloner(goldenRoot)
		if err != nil {
			return err
		}

		// Get current commit
		commit, _ := gitpkg.CurrentCommit(goldenRoot)

		branch, _ := cmd.Flags().GetString("branch")
		opts := workspace.CreateOpts{
			Branch:       branch,
			GoldenCommit: commit,
		}

		info, err := workspace.Create(goldenRoot, cfg, cloner, opts)
		if err != nil {
			return err
		}

		// Run post-clone hook
		if err := hooks.Run(info.Path, "post-clone"); err != nil {
			// Clean up on hook failure
			os.RemoveAll(info.Path)
			return fmt.Errorf("post-clone hook failed: %w\nWorkspace cleaned up", err)
		}

		// Checkout branch if specified
		if branch != "" {
			if err := gitpkg.Checkout(info.Path, branch, true); err != nil {
				// Don't clean up — clone succeeded, branch is secondary
				fmt.Fprintf(os.Stderr, "Warning: branch checkout failed: %v\n", err)
			}
			info.Branch = branch
		}

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
	createCmd.Flags().String("branch", "", "Branch to create/checkout in the workspace")
	createCmd.Flags().Bool("force", false, "Proceed even if golden copy has uncommitted changes")
	createCmd.Flags().Bool("json", false, "Output workspace info as JSON")
	rootCmd.AddCommand(createCmd)
}
