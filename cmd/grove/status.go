package main

import (
	"fmt"
	"os"

	"github.com/AmpInc/grove/internal/config"
	gitpkg "github.com/AmpInc/grove/internal/git"
	"github.com/AmpInc/grove/internal/workspace"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show golden copy info and workspace summary",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		goldenRoot, err := config.FindGroveRoot(cwd)
		if err != nil {
			return err
		}

		cfg, err := config.Load(goldenRoot)
		if err != nil {
			return err
		}

		projectName := getProjectName(goldenRoot)
		cfg.WorkspaceDir = config.ExpandWorkspaceDir(cfg.WorkspaceDir, projectName)

		branch, _ := gitpkg.CurrentBranch(goldenRoot)
		commit, _ := gitpkg.CurrentCommit(goldenRoot)
		dirty, _ := gitpkg.IsDirty(goldenRoot)

		statusStr := "clean"
		if dirty {
			statusStr = "dirty (uncommitted changes)"
		}

		isWs := workspace.IsWorkspace(goldenRoot)
		if isWs {
			fmt.Println("You are inside a grove workspace.")
			fmt.Println()
		}

		fmt.Printf("Golden copy: %s\n", goldenRoot)
		fmt.Printf("Branch:      %s\n", branch)
		fmt.Printf("Commit:      %s\n", commit)
		fmt.Printf("Status:      %s\n", statusStr)
		fmt.Println()

		workspaces, _ := workspace.List(cfg)
		fmt.Printf("Workspaces:  %d / %d (max)\n", len(workspaces), cfg.MaxWorkspaces)
		fmt.Printf("Directory:   %s\n", cfg.WorkspaceDir)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
