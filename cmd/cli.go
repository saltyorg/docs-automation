package cmd

import (
	"fmt"

	"github.com/saltyorg/docs-automation/automation"
	"github.com/spf13/cobra"
)

func newCLICommand(rootOpts *rootOptions, deps dependencies) *cobra.Command {
	var binaryPath string
	cmd := &cobra.Command{
		Use:   "cli",
		Short: "Update CLI help documentation",
		Long: `Update CLI help documentation from sb-go binary output.

Executes the sb binary with -h flag and updates the managed
CLI section in the documentation file.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := deps.loadConfig(rootOpts.configPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			runner := automation.NewRunner(cmd.OutOrStdout(), cmd.ErrOrStderr(), rootOpts.verbose)
			_, err = runner.UpdateCLIHelp(cmd.Context(), cfg, binaryPath)
			return err
		},
	}
	cmd.Flags().StringVar(&binaryPath, "binary", "", "path to sb binary (default: from config)")
	return cmd
}
