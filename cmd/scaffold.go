package cmd

import (
	"fmt"

	"github.com/saltyorg/docs-automation/automation"
	"github.com/spf13/cobra"
)

func newScaffoldCommand(rootOpts *rootOptions, deps dependencies) *cobra.Command {
	opts := automation.ScaffoldOptions{}
	cmd := &cobra.Command{
		Use:   "scaffold <role>",
		Short: "Generate new app documentation from template",
		Long: `Generate new app documentation from template.

Creates a starter documentation file at the appropriate path
for the specified role.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := deps.loadConfig(rootOpts.configPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			runner := automation.NewRunner(cmd.OutOrStdout(), cmd.ErrOrStderr(), rootOpts.verbose)
			return runner.Scaffold(cmd.Context(), cfg, args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.TemplatePath, "template", "", "path to scaffold template (default: from config)")
	cmd.Flags().StringVar(&opts.OutputPath, "output", "", "output path override")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "overwrite existing file if present")
	return cmd
}
