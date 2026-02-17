package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/AmpInc/grove/internal/config"
	"github.com/AmpInc/grove/internal/workspace"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List active workspaces",
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

		workspaces, err := workspace.List(cfg)
		if err != nil {
			return err
		}

		jsonOut, _ := cmd.Flags().GetBool("json")
		if jsonOut {
			data, _ := json.MarshalIndent(workspaces, "", "  ")
			fmt.Println(string(data))
			return nil
		}

		if len(workspaces) == 0 {
			fmt.Println("No active workspaces.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tBRANCH\tCREATED\tPATH")
		for _, ws := range workspaces {
			age := formatAge(ws.CreatedAt)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", ws.ID, ws.Branch, age, ws.Path)
		}
		w.Flush()
		return nil
	},
}

func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func init() {
	listCmd.Flags().Bool("json", false, "Output workspace list as JSON")
	rootCmd.AddCommand(listCmd)
}
