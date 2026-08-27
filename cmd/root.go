package cmd

import (
	"context"

	"github.com/saltyorg/docs-automation/buildinfo"
	"github.com/saltyorg/docs-automation/config"
	"github.com/spf13/cobra"
)

type rootOptions struct {
	configPath string
	verbose    bool
}

type dependencies struct {
	loadConfig func(string) (*config.Config, error)
}

func defaultDependencies() dependencies {
	return dependencies{loadConfig: config.Load}
}

// NewRootCmd builds a fresh sb-docs command tree.
func NewRootCmd() *cobra.Command {
	return newRootCmd(defaultDependencies())
}

func newRootCmd(deps dependencies) *cobra.Command {
	opts := &rootOptions{}
	rootCmd := &cobra.Command{
		Use:   "sb-docs",
		Short: "Saltbox documentation automation tool",
		Long: `sb-docs automates documentation management for Saltbox and Sandbox Ansible roles.

It performs the following core functions:
  - Documentation coverage checking
  - Inventory section generation from role defaults
  - CLI help documentation updates
  - Overview table generation from frontmatter
  - New app documentation scaffolding`,
		Version:       buildinfo.VersionString(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	rootCmd.SetVersionTemplate("{{.Version}}\n")
	rootCmd.PersistentFlags().StringVar(&opts.configPath, "config", "config.yml", "full config or path-only overlay file")
	rootCmd.PersistentFlags().BoolVarP(&opts.verbose, "verbose", "v", false, "enable verbose output")
	rootCmd.AddCommand(
		newCLICommand(opts, deps),
		newGenerateCommand(opts, deps),
		newIndexCommand(opts),
		newScaffoldCommand(opts, deps),
		newUpdateCommand(opts, deps),
		newValidateCommand(opts, deps),
		newVersionCommand(),
	)
	return rootCmd
}

// Execute runs a fresh command tree with the supplied context.
func Execute(ctx context.Context) error {
	return NewRootCmd().ExecuteContext(ctx)
}
