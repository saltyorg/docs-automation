package cmd

import (
	"fmt"

	"github.com/saltyorg/docs-automation/automation"
	"github.com/spf13/cobra"
)

func newUpdateCommand(rootOpts *rootOptions, deps dependencies) *cobra.Command {
	opts := automation.UpdateOptions{IssueLabel: "docs-automation"}
	cmd := &cobra.Command{
		Use:   "update [role]",
		Short: "Update documentation files in place",
		Long: `Update documentation files in place.

Without a role argument, updates all roles + CLI help.
With a role argument, updates only that role (no CLI by default).`,
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
			return runner.Update(cmd.Context(), cfg, role, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.NoCLI, "no-cli", false, "exclude CLI help generation")
	cmd.Flags().BoolVar(&opts.RunCheck, "check", false, "run coverage checks after updating")
	cmd.Flags().BoolVar(&opts.ManageIssue, "manage-issue", false, "create/update/close GitHub issue based on check results (requires --check and gh CLI)")
	cmd.Flags().StringVar(&opts.IssueLabel, "issue-label", "docs-automation", "label to use for the managed GitHub issue")
	return cmd
}
