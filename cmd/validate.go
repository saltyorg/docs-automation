package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/saltyorg/docs-automation/internal/config"
	"github.com/saltyorg/docs-automation/internal/docs"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration and frontmatter",
	Long:  "Validate configuration files and documentation frontmatter.",
}

var validateConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Validate config.yml",
	Long:  "Validate the configuration file for required fields and correct format.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load() now calls Validate() automatically
		_, err := config.Load(GetConfigPath())
		if err != nil {
			return err
		}

		fmt.Println("✅ Config is valid")
		return nil
	},
}

var validateFrontmatterCmd = &cobra.Command{
	Use:   "frontmatter",
	Short: "Validate frontmatter in doc files",
	Long:  "Validate frontmatter configuration in documentation files.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(GetConfigPath())
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		return validateFrontmatter(cfg)
	},
}

func init() {
	validateCmd.AddCommand(validateConfigCmd)
	validateCmd.AddCommand(validateFrontmatterCmd)
	rootCmd.AddCommand(validateCmd)
}

// validateFrontmatter validates frontmatter in all documentation files.
func validateFrontmatter(cfg *config.Config) error {
	// Get all documentation files
	saltboxDocs, err := docs.ListDocFiles(cfg.SaltboxDocsPath())
	if err != nil {
		return fmt.Errorf("listing saltbox docs: %w", err)
	}

	sandboxDocs, err := docs.ListDocFiles(cfg.SandboxDocsPath())
	if err != nil {
		return fmt.Errorf("listing sandbox docs: %w", err)
	}

	allDocs := make([]string, 0, len(saltboxDocs)+len(sandboxDocs)+len(cfg.FrontmatterDocs))
	seen := make(map[string]bool)

	for _, docPath := range saltboxDocs {
		if seen[docPath] {
			continue
		}
		seen[docPath] = true
		allDocs = append(allDocs, docPath)
	}

	for _, docPath := range sandboxDocs {
		if seen[docPath] {
			continue
		}
		seen[docPath] = true
		allDocs = append(allDocs, docPath)
	}

	for _, relPath := range cfg.FrontmatterDocs {
		trimmed := strings.TrimSpace(relPath)
		if trimmed == "" {
			continue
		}
		docPath := filepath.Join(cfg.Repositories.Docs, trimmed)
		if seen[docPath] {
			continue
		}
		seen[docPath] = true
		allDocs = append(allDocs, docPath)
	}
	valid := 0
	invalid := 0
	noFrontmatter := 0

	for _, docPath := range allDocs {
		content, err := os.ReadFile(docPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not read %s: %v\n", docPath, err)
			continue
		}

		fm, _, err := docs.ParseFrontmatter(string(content))
		if err != nil {
			fmt.Printf("❌ %s: %v\n", docPath, err)
			invalid++
			continue
		}

		if fm == nil {
			noFrontmatter++
			if IsVerbose() {
				fmt.Printf("⚠️  %s: no frontmatter\n", docPath)
			}
			continue
		}

		// Validate saltbox_automation section if present
		if fm.SaltboxAutomation != nil {
			if err := validateSaltboxAutomation(fm.SaltboxAutomation); err != nil {
				fmt.Printf("❌ %s: %v\n", docPath, err)
				invalid++
				continue
			}
		}

		valid++
		if IsVerbose() {
			fmt.Printf("✅ %s\n", docPath)
		}
	}

	fmt.Printf("\nValidation complete: %d valid, %d invalid, %d without frontmatter\n",
		valid, invalid, noFrontmatter)

	if invalid > 0 {
		return fmt.Errorf("found %d invalid files", invalid)
	}

	return nil
}

// validateSaltboxAutomation validates the saltbox_automation frontmatter section.
func validateSaltboxAutomation(sa *docs.SaltboxAutomationConfig) error {
	// Validate app_links if present
	for i, link := range sa.AppLinks {
		if link.Name == "" {
			return fmt.Errorf("app_links[%d]: name is required", i)
		}
		if link.URL == "" {
			return fmt.Errorf("app_links[%d]: url is required", i)
		}
	}

	// Validate project_description if present
	if sa.ProjectDescription != nil {
		if sa.ProjectDescription.Name == "" && sa.ProjectDescription.Summary != "" {
			return fmt.Errorf("project_description: name is required when summary is set")
		}
	}

	return nil
}
