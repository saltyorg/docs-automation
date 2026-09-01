package automation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/document"
)

// ValidateFrontmatter validates frontmatter in all documentation files.
func (r *Runner) ValidateFrontmatter(ctx context.Context, cfg *config.Config) (err error) {
	defer func() { err = r.result(err) }()
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
	excluded := 0

	for _, docPath := range allDocs {
		if err := ctx.Err(); err != nil {
			return err
		}
		relPath, err := filepath.Rel(cfg.Repositories.Docs, docPath)
		if err != nil {
			return fmt.Errorf("getting docs-relative path for %s: %w", docPath, err)
		}
		relPath = filepath.ToSlash(relPath)
		if cfg.Checks.Frontmatter.Excludes(relPath) {
			excluded++
			if r.verbose {
				r.printf("⏭️  %s: excluded by config\n", docPath)
			}
			continue
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

		if fm.SaltboxAutomation != nil &&
			(!fm.SaltboxAutomation.IsFrontmatterCheckEnabled() || !fm.SaltboxAutomation.IsOverviewSectionEnabled()) {
			excluded++
			if r.verbose {
				r.printf("⏭️  %s: excluded by frontmatter\n", docPath)
			}
			continue
		}

		diagnostics := document.ValidateAutomationFrontmatter(fm)
		if len(diagnostics) > 0 {
			invalid++
			for _, diagnostic := range diagnostics {
				r.printf("❌ %s: %s (%s)\n", docPath, diagnostic.Message, diagnostic.Code)
			}
			continue
		}

		valid++
		if r.verbose {
			r.printf("✅ %s\n", docPath)
		}
	}

	r.printf("\nValidation complete: %d valid, %d invalid, %d without frontmatter, %d excluded\n",
		valid, invalid, noFrontmatter, excluded)

	if invalid > 0 {
		return fmt.Errorf("found %d invalid files", invalid)
	}

	return nil
}
