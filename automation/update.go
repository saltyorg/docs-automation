package automation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/document"
	"github.com/saltyorg/docs-automation/github"
	"github.com/saltyorg/docs-automation/health"
	"github.com/saltyorg/docs-automation/parser"
	"github.com/saltyorg/docs-automation/render"
)

// UpdateOptions controls optional update and coverage behavior.
type UpdateOptions struct {
	NoCLI       bool
	RunCheck    bool
	ManageIssue bool
	IssueLabel  string
}

// skipError represents a non-fatal skip condition (not an actual error).
type skipError struct {
	reason string
}

func (e *skipError) Error() string {
	return e.reason
}

// Update updates one role or all configured roles in place.
func (r *Runner) Update(ctx context.Context, cfg *config.Config, role string, opts UpdateOptions) (err error) {
	defer func() { err = r.result(err) }()
	if err := ctx.Err(); err != nil {
		return err
	}
	sources, err := loadSourceCatalog(cfg)
	if err != nil {
		return err
	}
	if role != "" {
		return r.updateRole(ctx, cfg, sources, role)
	}
	return r.updateAllRoles(ctx, cfg, sources, opts)
}

// updateRole updates documentation for a single role.
func (r *Runner) updateRole(ctx context.Context, cfg *config.Config, sources render.SourceCatalog, roleName string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Try to find the role directory in saltbox first, then sandbox.
	repoType := "saltbox"
	rolePath := filepath.Join(cfg.SaltboxRolesPath(), roleName)

	if info, err := os.Stat(rolePath); err != nil || !info.IsDir() {
		// Try sandbox
		rolePath = filepath.Join(cfg.SandboxRolesPath(), roleName)
		repoType = "sandbox"

		if info, err := os.Stat(rolePath); err != nil || !info.IsDir() {
			return fmt.Errorf("role %q not found in saltbox or sandbox", roleName)
		}
	}

	return r.updateRoleWithType(ctx, cfg, sources, roleName, repoType)
}

// updateAllRoles updates documentation for all roles.
func (r *Runner) updateAllRoles(ctx context.Context, cfg *config.Config, sources render.SourceCatalog, opts UpdateOptions) error {
	// Get all saltbox roles
	saltboxRoles, err := listRoles(cfg.SaltboxRolesPath())
	if err != nil {
		return fmt.Errorf("listing saltbox roles: %w", err)
	}

	// Get all sandbox roles
	sandboxRoles, err := listRoles(cfg.SandboxRolesPath())
	if err != nil {
		return fmt.Errorf("listing sandbox roles: %w", err)
	}

	// Filter out blacklisted roles
	saltboxRoles = filterBlacklist(saltboxRoles, cfg.Blacklist.DocsCoverage.Saltbox)
	sandboxRoles = filterBlacklist(sandboxRoles, cfg.Blacklist.DocsCoverage.Sandbox)

	r.verbosef("Found %d saltbox roles and %d sandbox roles\n", len(saltboxRoles), len(sandboxRoles))

	summary := github.NewUpdateSummary()

	// Update each role
	for _, role := range saltboxRoles {
		if err := ctx.Err(); err != nil {
			return err
		}
		r.verbosef("Updating: %s (saltbox)\n", role)
		result := r.updateRoleWithResult(ctx, cfg, sources, role, "saltbox")
		summary.AddRole(result)

		switch result.Status {
		case github.StatusSkipped:
			r.printf("Skipping %s: %s\n", role, result.SkipReason)
		case github.StatusError:
			r.errorf("Error: failed to update %s: %s\n", role, result.Error)
		}
	}

	for _, role := range sandboxRoles {
		if err := ctx.Err(); err != nil {
			return err
		}
		r.verbosef("Updating: %s (sandbox)\n", role)
		result := r.updateRoleWithResult(ctx, cfg, sources, role, "sandbox")
		summary.AddRole(result)

		switch result.Status {
		case github.StatusSkipped:
			r.printf("Skipping %s: %s\n", role, result.SkipReason)
		case github.StatusError:
			r.errorf("Error: failed to update %s: %s\n", role, result.Error)
		}
	}

	r.printf("Updated %d roles, %d unchanged, %d skipped, %d errors\n", summary.Updated, summary.Unchanged, summary.Skipped, summary.Errors)

	// Update CLI help unless --no-cli was specified.
	var cliErr error
	if !opts.NoCLI {
		changed, updateErr := r.UpdateCLIHelp(ctx, cfg, "")
		if updateErr != nil {
			cliErr = updateErr
			r.errorf("Warning: failed to update CLI help: %v\n", updateErr)
		} else if changed {
			summary.CLIUpdated = true
		}
	}

	// Build the canonical health report if requested.
	if opts.RunCheck {
		report, err := r.buildHealthReport(ctx, cfg, summary, !opts.NoCLI, cliErr)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			r.errorf("Warning: failed to build documentation health report: %v\n", err)
		} else {
			summary.SetHealthReport(&report)
			r.printHealthReport(report)

			if opts.ManageIssue {
				repo := github.GetRepository()
				issueManager := github.NewIssueManager(repo, r.out, r.errOut)

				if err := issueManager.ManageIssue(ctx, report, opts.IssueLabel); err != nil {
					r.errorf("Warning: failed to manage GitHub issue: %v\n", err)
				}
			}
		}
	}

	// Write GitHub Actions summary
	if err := summary.WriteGitHubSummary(); err != nil {
		r.errorf("Warning: failed to write GitHub summary: %v\n", err)
	}

	return nil
}

// updateRoleWithType updates documentation for a role with known repo type.
func (r *Runner) updateRoleWithType(ctx context.Context, cfg *config.Config, sources render.SourceCatalog, roleName, repoType string) error {
	result := r.updateRoleWithResult(ctx, cfg, sources, roleName, repoType)
	if result.Status == github.StatusError {
		return fmt.Errorf("%s", result.Error)
	}
	if result.Status == github.StatusSkipped {
		return &skipError{reason: result.SkipReason}
	}
	return nil
}

// updateRoleWithResult updates documentation for a role and returns a detailed result.
func (r *Runner) updateRoleWithResult(ctx context.Context, cfg *config.Config, sources render.SourceCatalog, roleName, repoType string) github.RoleResult {
	result := github.RoleResult{
		Name:     roleName,
		RepoType: repoType,
		Status:   github.StatusUpdated,
		Sections: []string{},
	}
	if err := ctx.Err(); err != nil {
		result.Status = github.StatusError
		result.Error = err.Error()
		return result
	}

	var rolesPath string
	if repoType == "saltbox" {
		rolesPath = cfg.SaltboxRolesPath()
	} else {
		rolesPath = cfg.SandboxRolesPath()
	}

	defaultsPath := filepath.Join(rolesPath, roleName, "defaults", "main.yml")

	// Get documentation path
	docPath := getDocPath(cfg, roleName, repoType)
	if docPath == "" {
		result.Status = github.StatusError
		result.Error = "could not determine doc path"
		return result
	}

	// Check if doc file exists
	if _, err := os.Stat(docPath); os.IsNotExist(err) {
		result.Status = github.StatusSkipped
		result.SkipReason = "doc file does not exist"
		return result
	}

	// Create docs manager
	manager := document.NewManager(document.MarkerConfig{
		Variables: cfg.Markers.Variables,
		CLI:       cfg.Markers.CLI,
		Overview:  cfg.Markers.Overview,
	})

	// Load existing document
	doc, err := manager.LoadDocument(docPath)
	if err != nil {
		result.Status = github.StatusError
		result.Error = fmt.Sprintf("loading document: %v", err)
		return result
	}

	// Store original content to detect actual changes
	originalContent := doc.Content

	// Check if automation is disabled
	if manager.IsAutomationDisabled(doc) {
		result.Status = github.StatusSkipped
		result.SkipReason = "automation disabled in frontmatter"
		return result
	}

	// Get frontmatter config
	var fmConfig *document.SaltboxAutomationConfig
	if doc.Frontmatter != nil {
		fmConfig = doc.Frontmatter.SaltboxAutomation
	}

	inventorySkipReason := ""
	var roleInfo *parser.RoleInfo
	needsInventoryRole := fmConfig.IsInventorySectionEnabled() && manager.HasVariablesSection(doc)
	needsMetadataRole := cfg.DockerMetadata.Enabled() && fmConfig.IsOverviewSectionEnabled()
	if needsInventoryRole || needsMetadataRole {
		if _, err := os.Stat(defaultsPath); os.IsNotExist(err) {
			if needsInventoryRole {
				inventorySkipReason = "no defaults/main.yml"
			}
		} else {
			roleInfo, err = r.parseRole(roleName, repoType, defaultsPath)
			if err != nil {
				result.Status = github.StatusError
				result.Error = fmt.Sprintf("parsing: %v", err)
				return result
			}
		}
	}

	if needsMetadataRole && roleInfo != nil {
		changed, err := manager.ApplyFrontmatterChanges(doc, dockerMetadataChanges(doc, roleInfo, cfg.DockerMetadata))
		if err != nil {
			result.Status = github.StatusError
			result.Error = fmt.Sprintf("repairing docker metadata: %v", err)
			return result
		}
		if changed {
			result.Sections = append(result.Sections, "frontmatter")
			if doc.Frontmatter != nil {
				fmConfig = doc.Frontmatter.SaltboxAutomation
			}
		}
	}

	// Update inventory section if enabled
	if needsInventoryRole && roleInfo != nil {
		// Skip if no variables (use filtered count for this check)
		filteredVars := parser.FilterVariables(roleInfo.AllVariables, roleName)
		if len(filteredVars) == 0 {
			inventorySkipReason = "no documentable variables"
		} else {
			// Build template data
			data := render.BuildRoleData(roleInfo, cfg, fmConfig, sources)

			// Create template engine and render
			engine := render.New()
			if err := engine.LoadFile("inventory", cfg.InventoryTemplatePath()); err != nil {
				result.Status = github.StatusError
				result.Error = fmt.Sprintf("loading template: %v", err)
				return result
			}

			output, err := engine.Render("inventory", data)
			if err != nil {
				result.Status = github.StatusError
				result.Error = fmt.Sprintf("rendering: %v", err)
				return result
			}

			// Update the managed section
			if err := manager.UpdateVariablesSection(doc, output); err != nil {
				result.Status = github.StatusError
				result.Error = fmt.Sprintf("updating section: %v", err)
				return result
			}
			result.Sections = append(result.Sections, "variables")
		}
	}

	// Update overview section if enabled and the document has the section
	if fmConfig.IsOverviewSectionEnabled() && manager.HasOverviewSection(doc) {
		tableGen := render.NewTableGenerator(cfg.OverviewTemplatePath())
		if err := tableGen.LoadTemplate(); err != nil {
			result.Status = github.StatusError
			result.Error = fmt.Sprintf("loading overview template: %v", err)
			return result
		}
		tableContent, err := tableGen.GenerateFromDocument(doc)
		if err != nil {
			result.Status = github.StatusError
			result.Error = fmt.Sprintf("generating overview table: %v", err)
			return result
		}
		if tableContent != "" {
			if err := manager.UpdateOverviewSection(doc, tableContent); err != nil {
				result.Status = github.StatusError
				result.Error = fmt.Sprintf("updating overview section: %v", err)
				return result
			}
			result.Sections = append(result.Sections, "overview")
		}
	}

	// Skip if nothing was updated
	if len(result.Sections) == 0 {
		result.Status = github.StatusSkipped
		if inventorySkipReason != "" {
			result.SkipReason = inventorySkipReason
		} else {
			result.SkipReason = "no enabled sections to update"
		}
		return result
	}

	// Check if content actually changed
	if doc.Content == originalContent {
		result.Status = github.StatusUnchanged
		return result
	}

	// Save the document
	if err := r.saveDocument(manager, doc); err != nil {
		result.Status = github.StatusError
		result.Error = fmt.Sprintf("saving document: %v", err)
		return result
	}

	r.verbosef("  Updated %s\n", docPath)

	return result
}

// printHealthReport prints compact nonzero health finding counts.
func (r *Runner) printHealthReport(report health.Report) {
	report = report.Canonical()
	r.printf("\n## Documentation Health\n\n")
	for _, result := range report.Results {
		if !result.Enabled || len(result.Findings) == 0 {
			continue
		}
		if result.Kind.Severity() == health.Notice {
			r.printf("Notice: %s: %d\n", healthResultLabel(result.Kind), len(result.Findings))
			continue
		}
		r.printf("Error: %s: %d\n", healthResultLabel(result.Kind), len(result.Findings))
	}

	errors := report.TotalSeverity(health.Error)
	notices := report.TotalSeverity(health.Notice)
	if errors == 0 && notices == 0 {
		r.printf("✅ All enabled documentation health checks passed!\n")
		return
	}
	r.printf("Found %d error(s), %d notice(s)\n", errors, notices)
}

func healthResultLabel(kind health.Kind) string {
	switch kind {
	case health.RoleAutomationError:
		return "Role Automation Errors"
	case health.CLIHelpAutomationError:
		return "CLI Help Automation Errors"
	case health.MissingDocumentation:
		return "Missing Documentation"
	case health.InvalidFrontmatter:
		return "Invalid Frontmatter"
	case health.MissingVariablesSection:
		return "Missing Variables Sections"
	case health.MissingOverviewSection:
		return "Missing Overview Sections"
	case health.OrphanedDocumentation:
		return "Orphaned Documentation"
	case health.EditorialAttention:
		return "Editorial Attention"
	default:
		return string(kind)
	}
}
