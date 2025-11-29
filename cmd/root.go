package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
	verbose bool
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "sb-docs",
	Short: "Saltbox documentation automation tool",
	Long: `sb-docs automates documentation management for Saltbox and Sandbox Ansible roles.

It performs the following core functions:
  - Documentation coverage checking
  - Inventory section generation from role defaults
  - CLI help documentation updates
  - Overview table generation from frontmatter
  - New app documentation scaffolding`,
	SilenceUsage: true, // Don't print usage on errors unrelated to flags
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "config.yml", "config file path")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
}

// GetConfigPath returns the configured config file path.
func GetConfigPath() string {
	return cfgFile
}

// IsVerbose returns whether verbose mode is enabled.
func IsVerbose() bool {
	return verbose
}
