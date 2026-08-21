package automation

import (
	"context"
	"fmt"
	"os"

	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/document"
)

// ValidateFrontmatter validates frontmatter in all documentation files.
func (r *Runner) ValidateFrontmatter(ctx context.Context, cfg *config.Config) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Get all documentation files
	saltboxDocs, err := document.ListDocFiles(cfg.SaltboxDocsPath())
	if err != nil {
		return fmt.Errorf("listing saltbox docs: %w", err)
	}

	sandboxDocs, err := document.ListDocFiles(cfg.SandboxDocsPath())
	if err != nil {
		return fmt.Errorf("listing sandbox docs: %w", err)
	}

	allDocs := make([]string, 0, len(saltboxDocs)+len(sandboxDocs))
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
	valid := 0
	invalid := 0
	noFrontmatter := 0

	for _, docPath := range allDocs {
		if err := ctx.Err(); err != nil {
			return err
		}
		content, err := os.ReadFile(docPath)
		if err != nil {
			r.errorf("Warning: could not read %s: %v\n", docPath, err)
			continue
		}

		fm, _, err := document.ParseFrontmatter(string(content))
		if err != nil {
			r.printf("❌ %s: %v\n", docPath, err)
			invalid++
			continue
		}

		if fm == nil {
			noFrontmatter++
			if r.verbose {
				r.printf("⚠️  %s: no frontmatter\n", docPath)
			}
			continue
		}

		// Validate saltbox_automation section if present
		if fm.SaltboxAutomation != nil {
			if err := validateSaltboxAutomation(fm.SaltboxAutomation); err != nil {
				r.printf("❌ %s: %v\n", docPath, err)
				invalid++
				continue
			}
		}

		valid++
		if r.verbose {
			r.printf("✅ %s\n", docPath)
		}
	}

	r.printf("\nValidation complete: %d valid, %d invalid, %d without frontmatter\n",
		valid, invalid, noFrontmatter)

	if invalid > 0 {
		return fmt.Errorf("found %d invalid files", invalid)
	}

	return nil
}

// validateSaltboxAutomation validates the saltbox_automation frontmatter section.
func validateSaltboxAutomation(sa *document.SaltboxAutomationConfig) error {
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
