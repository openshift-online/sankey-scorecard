package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  "Print the version, build date, and commit hash of the sankey-scorecard binary.",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("sankey-scorecard %s (commit: %s, built: %s)\n", Version, Commit, BuildDate)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
