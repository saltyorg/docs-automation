package cmd

import (
	"fmt"

	"github.com/saltyorg/docs-automation/internal/runtime"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  "Print the version, git commit, and build time of sb-docs.",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(runtime.VersionString())
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
