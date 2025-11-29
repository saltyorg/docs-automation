package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var indexCmd = &cobra.Command{
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
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Index generation is not yet implemented.")
		fmt.Println("")
		fmt.Println("This command will eventually:")
		fmt.Println("  1. Scan all app documentation files")
		fmt.Println("  2. Read categories from saltbox_automation.project_description.categories")
		fmt.Println("  3. Generate categorized index.md files")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(indexCmd)
}
