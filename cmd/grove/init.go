package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/chrisbanes/grove/internal/config"
	gitpkg "github.com/chrisbanes/grove/internal/git"
	"github.com/chrisbanes/grove/internal/image"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Initialize a golden copy from an existing repo",
	Long: `Registers a git repository as a grove-managed golden copy.
Creates a .grove/ directory with config and optional hooks.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		progressEnabled, _ := cmd.Flags().GetBool("progress")
		var progress *progressRenderer
		if progressEnabled {
			progress = newProgressRenderer(os.Stderr, isTerminalFile(os.Stderr), "init")
			defer progress.Done()
		}

		path := "."
		if len(args) > 0 {
			path = args[0]
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			return err
		}

		// Verify it's a git repo
		if !gitpkg.IsRepo(absPath) {
			return fmt.Errorf("%s is not a git repository", absPath)
		}

		// Check for existing .grove
		groveDir := filepath.Join(absPath, config.GroveDirName)
		if _, err := os.Stat(groveDir); err == nil {
			return fmt.Errorf("grove already initialized at %s", absPath)
		}

		// Check for uncommitted changes
		force, _ := cmd.Flags().GetBool("force")
		dirty, err := gitpkg.IsDirty(absPath)
		if err != nil {
			return fmt.Errorf("checking repo status: %w", err)
		}
		if dirty && !force {
			return fmt.Errorf(
				"golden copy has uncommitted changes.\n" +
					"These changes will be included in workspace clones.\n" +
					"Use --force to proceed anyway")
		}

		// Create .grove directory structure
		if err := os.MkdirAll(filepath.Join(groveDir, config.HooksDir), 0755); err != nil {
			return err
		}

		// Write default config
		projectName := filepath.Base(absPath)
		cfg := config.DefaultConfig(projectName)

		warmupCmd, _ := cmd.Flags().GetString("warmup-command")
		if warmupCmd != "" {
			cfg.WarmupCommand = warmupCmd
		}
		wsDir, _ := cmd.Flags().GetString("workspace-dir")
		if wsDir != "" {
			cfg.WorkspaceDir = wsDir
		}
		backend, _ := cmd.Flags().GetString("backend")
		switch backend {
		case "cp", "image":
		default:
			return fmt.Errorf("invalid --backend %q: expected cp or image", backend)
		}
		cfg.CloneBackend = backend

		if err := config.Save(absPath, cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}

		// Run warmup command if configured
		if cfg.WarmupCommand != "" {
			fmt.Printf("Running warmup: %s\n", cfg.WarmupCommand)
			warmup := exec.Command("sh", "-c", cfg.WarmupCommand)
			warmup.Dir = absPath
			warmup.Stdout = os.Stdout
			warmup.Stderr = os.Stderr
			if err := warmup.Run(); err != nil {
				return fmt.Errorf("warmup command failed: %w", err)
			}
		}

		if cfg.CloneBackend == "image" {
			imageSizeGB, _ := cmd.Flags().GetInt("image-size-gb")
			if progress == nil {
				fmt.Println("Initializing image backend...")
			}
			var onProgress func(int, string)
			if progress != nil {
				onProgress = func(pct int, phase string) {
					progress.Update(pct, phase)
				}
			}
			if _, err := image.InitBase(absPath, nil, imageSizeGB, cfg.Exclude, onProgress); err != nil {
				return fmt.Errorf("initializing image backend: %w", err)
			}
		}
		if err := config.SaveBackendState(absPath, cfg.CloneBackend); err != nil {
			return fmt.Errorf("saving backend state: %w", err)
		}

		fmt.Printf("Grove initialized at %s\n", absPath)
		fmt.Printf("Workspace dir: %s\n", config.ExpandWorkspaceDir(cfg.WorkspaceDir, projectName))
		return nil
	},
}

func init() {
	initCmd.Flags().String("warmup-command", "", "Command to run for warming up build caches")
	initCmd.Flags().String("workspace-dir", "", "Directory for workspaces (default: /tmp/grove/{project})")
	initCmd.Flags().String("backend", "cp", "Workspace backend: cp or image (experimental)")
	initCmd.Flags().Int("image-size-gb", 200, "Base sparsebundle size in GB when using --backend image")
	initCmd.Flags().Bool("force", false, "Proceed even if golden copy has uncommitted changes")
	initCmd.Flags().Bool("progress", false, "Show progress output during image backend initialization")
	rootCmd.AddCommand(initCmd)
}
