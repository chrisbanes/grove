package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chrisbanes/grove/internal/config"
	"github.com/chrisbanes/grove/internal/image"
	"github.com/chrisbanes/grove/internal/workspace"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate --to <cp|image>",
	Short: "Migrate workspace backend safely",
	Long:  `Migrates an initialized golden copy between cp and image backends.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		progressEnabled, _ := cmd.Flags().GetBool("progress")
		var progress *progressRenderer
		if progressEnabled {
			progress = newProgressRenderer(os.Stderr, isTerminalFile(os.Stderr), "migrate")
			defer progress.Done()
		}

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		goldenRoot, err := config.FindGroveRoot(cwd)
		if err != nil {
			return err
		}
		if workspace.IsWorkspace(goldenRoot) {
			return fmt.Errorf("cannot migrate backend from inside a workspace.\nRun this from the golden copy instead")
		}

		cfg, err := config.Load(goldenRoot)
		if err != nil {
			return err
		}

		to, _ := cmd.Flags().GetString("to")
		switch to {
		case "cp", "image":
		default:
			return fmt.Errorf("invalid --to %q: expected cp or image", to)
		}

		currentBackend, err := detectInitializedBackend(goldenRoot)
		if err != nil {
			return err
		}

		if currentBackend == to {
			if cfg.CloneBackend != to {
				cfg.CloneBackend = to
				if err := config.Save(goldenRoot, cfg); err != nil {
					return fmt.Errorf("saving config: %w", err)
				}
			}
			if err := config.SaveBackendState(goldenRoot, to); err != nil {
				return err
			}
			fmt.Printf("Backend already set to %s\n", to)
			return nil
		}

		switch to {
		case "image":
			if _, err := image.LoadState(goldenRoot); err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("loading image backend state: %w", err)
				}
				sizeGB, _ := cmd.Flags().GetInt("image-size-gb")
				if progress == nil {
					fmt.Println("Initializing image backend...")
				}
				var onProgress func(int, string)
				if progress != nil {
					onProgress = func(pct int, phase string) {
						progress.Update(pct, phase)
					}
				}
				if _, err := image.InitBase(goldenRoot, nil, sizeGB, nil, onProgress); err != nil {
					return fmt.Errorf("initializing image backend: %w", err)
				}
			}
		case "cp":
			metas, err := image.ListWorkspaceMeta(goldenRoot)
			if err != nil {
				return err
			}
			if len(metas) > 0 {
				return fmt.Errorf("cannot migrate to cp with active image workspaces (%d). Destroy them first", len(metas))
			}
		}

		cfg.CloneBackend = to
		if err := config.Save(goldenRoot, cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		if err := config.SaveBackendState(goldenRoot, to); err != nil {
			return fmt.Errorf("saving backend state: %w", err)
		}

		fmt.Printf("Migrated backend to %s\n", to)
		return nil
	},
}

func init() {
	migrateCmd.Flags().String("to", "", "Target backend: cp or image")
	migrateCmd.Flags().Int("image-size-gb", 200, "Base sparsebundle size in GB when migrating to image")
	migrateCmd.Flags().Bool("progress", false, "Show progress output during image backend initialization")
	_ = migrateCmd.MarkFlagRequired("to")
	rootCmd.AddCommand(migrateCmd)
}

func detectInitializedBackend(repoRoot string) (string, error) {
	backend, err := config.LoadBackendState(repoRoot)
	if err == nil {
		return backend, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	legacyImageStatePath := filepath.Join(repoRoot, config.GroveDirName, "images", "state.json")
	if _, err := os.Stat(legacyImageStatePath); err == nil {
		return "image", nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	return "cp", nil
}
