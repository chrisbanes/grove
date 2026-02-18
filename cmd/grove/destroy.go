package main

import (
	"fmt"
	"os"

	"github.com/chrisbanes/grove/internal/config"
	gitpkg "github.com/chrisbanes/grove/internal/git"
	"github.com/chrisbanes/grove/internal/workspace"
	"github.com/spf13/cobra"
)

var destroyCmd = &cobra.Command{
	Use:   "destroy <id|path>",
	Short: "Remove a workspace",
	Long:  `Removes a workspace directory. Optionally pushes the branch first.`,
	Args:  cobra.MaximumNArgs(1),
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

		all, _ := cmd.Flags().GetBool("all")
		push, _ := cmd.Flags().GetBool("push")

		if all {
			list, err := workspace.List(cfg)
			if err != nil {
				return err
			}
			if len(list) == 0 {
				fmt.Println("No workspaces to destroy.")
				return nil
			}
			for _, ws := range list {
				if push && ws.Branch != "" {
					if err := gitpkg.Push(ws.Path, ws.Branch); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: failed to push %s (%s): %v\n", ws.ID, ws.Branch, err)
						continue
					}
				}
				if err := workspace.Destroy(cfg, ws.ID); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to destroy %s: %v\n", ws.ID, err)
					continue
				}
				fmt.Printf("Destroyed: %s\n", ws.ID)
			}
			return nil
		}

		if len(args) == 0 {
			return fmt.Errorf("provide a workspace ID or path, or use --all")
		}

		idOrPath := args[0]
		if push {
			info, err := workspace.Get(cfg, idOrPath)
			if err == nil && info.Branch != "" {
				if err := gitpkg.Push(info.Path, info.Branch); err != nil {
					return fmt.Errorf("push failed for %s (%s): %w", info.ID, info.Branch, err)
				}
			}
		}

		if err := workspace.Destroy(cfg, idOrPath); err != nil {
			return err
		}
		fmt.Printf("Destroyed: %s\n", idOrPath)
		return nil
	},
}

func init() {
	destroyCmd.Flags().Bool("all", false, "Destroy all workspaces")
	destroyCmd.Flags().Bool("push", false, "Push branch before destroying")
	rootCmd.AddCommand(destroyCmd)
}
