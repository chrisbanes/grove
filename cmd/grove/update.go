package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/chrisbanes/grove/internal/backend"
	"github.com/chrisbanes/grove/internal/config"
	gitpkg "github.com/chrisbanes/grove/internal/git"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Pull latest and rebuild the golden copy",
	Long:  `Convenience command to refresh the golden copy: git pull + warmup command.`,
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
		if err := config.EnsureBackendCompatible(goldenRoot, cfg); err != nil {
			return err
		}
		backendImpl, err := backend.ForName(cfg.CloneBackend)
		if err != nil {
			return err
		}

		fmt.Println("Pulling latest...")
		if err := gitpkg.Pull(goldenRoot); err != nil {
			return fmt.Errorf("git pull failed: %w", err)
		}

		if cfg.WarmupCommand != "" {
			fmt.Printf("Running warmup: %s\n", cfg.WarmupCommand)
			warmup := exec.Command("sh", "-c", cfg.WarmupCommand)
			warmup.Dir = goldenRoot
			warmup.Stdout = os.Stdout
			warmup.Stderr = os.Stderr
			if err := warmup.Run(); err != nil {
				return fmt.Errorf("warmup command failed: %w", err)
			}
		}

		commit, _ := gitpkg.CurrentCommit(goldenRoot)
		if backendImpl.Name() == "image" {
			fmt.Println("Refreshing image backend...")
		}
		if err := backendImpl.RefreshBase(goldenRoot, commit, nil); err != nil {
			return err
		}
		fmt.Printf("Golden copy updated to %s\n", commit)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
