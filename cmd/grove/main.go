package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "grove",
	Short: "Manage CoW-cloned workspaces with warm build caches",
	Long: `Grove creates copy-on-write clones of a "golden copy" repository,
preserving gitignored build state so every workspace starts with warm caches.`,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the grove version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("grove %s\n", version)
	},
}

func main() {
	rootCmd.AddCommand(versionCmd)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
