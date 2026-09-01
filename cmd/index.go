package cmd

import (
	"github.com/saltyorg/docs-automation/automation"
	"github.com/spf13/cobra"
)

func newIndexCommand(rootOpts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "index",
		Short: "Generate index pages from frontmatter categories",
		Long: `Generate index.md files for documentation sections based on frontmatter.

This command reads the 'categories' field from each documentation file's
frontmatter and generates categorized index pages.

Frontmatter format:
  saltbox_automation:
    project_description:
      name: "App Name"
      categories:
        - "Content Delivery Apps > Media Server"
        - "Admin Apps > Container Operation"

The generated index will organize apps by their category hierarchies.

NOTE: This command is not yet implemented.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			runner := automation.NewRunner(cmd.OutOrStdout(), cmd.ErrOrStderr(), rootOpts.verbose)
			return runner.Index()
		},
	}
}
