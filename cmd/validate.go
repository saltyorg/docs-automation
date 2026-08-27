package cmd

import (
	"fmt"

	"github.com/saltyorg/docs-automation/automation"
	"github.com/spf13/cobra"
)

func newValidateCommand(rootOpts *rootOptions, deps dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration and frontmatter",
		Long:  "Validate configuration files and documentation frontmatter.",
	}
	cmd.AddCommand(
		newValidateConfigCommand(rootOpts, deps),
		newValidateFrontmatterCommand(rootOpts, deps),
	)
	return cmd
}

func newValidateConfigCommand(rootOpts *rootOptions, deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Validate configuration",
		Long:  "Validate a full configuration file or path-only local overlay.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := deps.loadConfig(rootOpts.configPath); err != nil {
				return err
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "✅ Config is valid")
			return err
		},
	}
}

func newValidateFrontmatterCommand(rootOpts *rootOptions, deps dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "frontmatter",
		Short: "Validate frontmatter in doc files",
		Long:  "Validate frontmatter configuration in documentation files.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := deps.loadConfig(rootOpts.configPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			runner := automation.NewRunner(cmd.OutOrStdout(), cmd.ErrOrStderr(), rootOpts.verbose)
			return runner.ValidateFrontmatter(cmd.Context(), cfg)
		},
	}
}
