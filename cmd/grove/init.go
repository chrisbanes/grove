package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/chrisbanes/grove/internal/config"
	gitpkg "github.com/chrisbanes/grove/internal/git"
	"github.com/chrisbanes/grove/internal/image"
	"github.com/chrisbanes/grove/internal/termio"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Configure grove for a repository",
	Long: `Sets up grove configuration for a git repository.
Runs an interactive wizard when called without flags.
Can be re-run to update configuration.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		progressEnabled := resolveProgress(cmd)
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

		if !gitpkg.IsRepo(absPath) {
			return fmt.Errorf("%s is not a git repository", absPath)
		}

		projectName := filepath.Base(absPath)

		// Load existing config or start with defaults
		cfg, _ := config.LoadOrDefault(absPath)

		useDefaults, _ := cmd.Flags().GetBool("defaults")
		interactive := isTerminalFile(os.Stdin) && !useDefaults

		// Apply flag overrides
		backendFlag, _ := cmd.Flags().GetString("backend")
		backendSet := cmd.Flags().Changed("backend")
		wsDirFlag, _ := cmd.Flags().GetString("workspace-dir")
		wsDirSet := cmd.Flags().Changed("workspace-dir")
		warmupFlag, _ := cmd.Flags().GetString("warmup-command")
		warmupSet := cmd.Flags().Changed("warmup-command")
		imageSizeFlag, _ := cmd.Flags().GetInt("image-size-gb")
		imageSizeSet := cmd.Flags().Changed("image-size-gb")

		if backendSet {
			switch backendFlag {
			case "cp", "image":
				cfg.CloneBackend = backendFlag
			default:
				return fmt.Errorf("invalid --backend %q: expected cp or image", backendFlag)
			}
		}
		if wsDirSet {
			cfg.WorkspaceDir = wsDirFlag
		}
		if warmupSet {
			cfg.WarmupCommand = warmupFlag
		}

		// Interactive prompts for unset options
		if interactive {
			fmt.Printf("Initializing grove for %s...\n\n", projectName)

			if !backendSet {
				backendChoice := cfg.CloneBackend
				if backendChoice == "" {
					backendChoice = "cp"
				}
				err := huh.NewSelect[string]().
					Title("Which clone backend?").
					Options(
						huh.NewOption("cp - fast APFS copy-on-write clones (default)", "cp"),
						huh.NewOption("image - sparsebundle-based clones (experimental)", "image"),
					).
					Value(&backendChoice).
					Run()
				if err != nil {
					return err
				}
				cfg.CloneBackend = backendChoice
			}

			if !wsDirSet {
				defaultDir := cfg.WorkspaceDir
				if defaultDir == "" {
					defaultDir = "~/.grove/{project}"
				}
				wsDir := defaultDir
				err := huh.NewInput().
					Title("Workspace directory").
					Value(&wsDir).
					Run()
				if err != nil {
					return err
				}
				if wsDir != "" {
					cfg.WorkspaceDir = wsDir
				}
			}

			if cfg.CloneBackend == "image" && !imageSizeSet {
				sizeStr := "200"
				err := huh.NewInput().
					Title("Base image size in GB").
					Value(&sizeStr).
					Run()
				if err != nil {
					return err
				}
				if sizeStr != "" {
					fmt.Sscanf(sizeStr, "%d", &imageSizeFlag)
				}
			}

			if !warmupSet {
				warmup := cfg.WarmupCommand
				err := huh.NewInput().
					Title("Warmup command (optional)").
					Description("e.g. npm install && npm run build").
					Value(&warmup).
					Run()
				if err != nil {
					return err
				}
				cfg.WarmupCommand = warmup
			}

			// Excludes prompt
			excludeStr := strings.Join(cfg.Exclude, ", ")
			err := huh.NewInput().
				Title("Exclude patterns (comma-separated, optional)").
				Description("e.g. *.lock, __pycache__").
				Value(&excludeStr).
				Run()
			if err != nil {
				return err
			}
			if excludeStr != "" {
				parts := strings.Split(excludeStr, ",")
				cfg.Exclude = nil
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if p != "" {
						cfg.Exclude = append(cfg.Exclude, p)
					}
				}
			} else {
				cfg.Exclude = nil
			}
		}

		// Ensure backend is set
		if cfg.CloneBackend == "" {
			cfg.CloneBackend = "cp"
		}

		// Create full .grove directory structure (including hooks/)
		groveDir := filepath.Join(absPath, config.GroveDirName)
		if err := os.MkdirAll(filepath.Join(groveDir, config.HooksDir), 0755); err != nil {
			return err
		}
		if err := config.EnsureGroveGitignore(absPath); err != nil {
			return fmt.Errorf("writing .grove/.gitignore: %w", err)
		}

		if err := config.Save(absPath, cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}

		// Run warmup if configured
		if cfg.WarmupCommand != "" {
			fmt.Printf("Running warmup: %s\n", cfg.WarmupCommand)
			warmup := exec.Command("sh", "-c", cfg.WarmupCommand)
			warmup.Dir = absPath
			if err := termio.RunInteractive(warmup); err != nil {
				return fmt.Errorf("warmup command failed: %w", err)
			}
		}

		// Initialize image backend if needed
		if cfg.CloneBackend == "image" {
			sizeGB := imageSizeFlag
			if sizeGB == 0 {
				sizeGB = 200
			}
			if progress == nil {
				fmt.Println("Initializing image backend...")
			}
			runtimeRoot, err := config.EnsureImageRuntimeRoot(absPath, cfg)
			if err != nil {
				return fmt.Errorf("resolving image runtime root: %w", err)
			}
			excludes, err := config.BuildImageSyncExcludes(absPath, cfg)
			if err != nil {
				return fmt.Errorf("computing image sync excludes: %w", err)
			}
			var onProgress func(int, string)
			if progress != nil {
				onProgress = func(pct int, phase string) {
					progress.Update(pct, phase)
				}
			}
			if _, err := image.InitBase(runtimeRoot, absPath, nil, sizeGB, excludes, onProgress); err != nil {
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
	initCmd.Flags().String("workspace-dir", "", "Directory for workspaces (default: ~/.grove/{project})")
	initCmd.Flags().String("backend", "", "Workspace backend: cp or image (experimental)")
	initCmd.Flags().Int("image-size-gb", 200, "Base sparsebundle size in GB when using --backend image")
	initCmd.Flags().Bool("progress", false, "Show progress output (default: auto-detect TTY)")
	initCmd.Flags().Bool("defaults", false, "Skip interactive prompts and use all defaults")
	rootCmd.AddCommand(initCmd)
}
