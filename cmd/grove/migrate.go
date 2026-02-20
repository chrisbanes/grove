package main

import (
	"errors"
	"fmt"
	"os"

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

		if cfg.CloneBackend == to {
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
				fmt.Println("Initializing image backend...")
				if _, err := image.InitBase(goldenRoot, nil, sizeGB); err != nil {
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
	_ = migrateCmd.MarkFlagRequired("to")
	rootCmd.AddCommand(migrateCmd)
}

