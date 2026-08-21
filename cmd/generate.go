package cmd

import (
	"fmt"

	"github.com/saltyorg/docs-automation/automation"
	"github.com/spf13/cobra"
)

func newGenerateCommand(rootOpts *rootOptions, deps dependencies) *cobra.Command {
	var includeCLI bool
	cmd := &cobra.Command{
		Use:   "generate [role]",
		Short: "Generate documentation content to stdout",
		Long: `Generate documentation content to stdout.

Without a role argument, generates all roles + CLI help.
With a role argument, generates only that role (no CLI by default).`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := deps.loadConfig(rootOpts.configPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			role := ""
			if len(args) == 1 {
				role = args[0]
			}
			runner := automation.NewRunner(cmd.OutOrStdout(), cmd.ErrOrStderr(), rootOpts.verbose)
			return runner.Generate(cmd.Context(), cfg, role, automation.GenerateOptions{IncludeCLI: includeCLI})
		},
	}
	cmd.Flags().BoolVar(&includeCLI, "cli", false, "include CLI help generation")
	return cmd
}
