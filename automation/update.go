package automation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/document"
	"github.com/saltyorg/docs-automation/github"
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
func (r *Runner) Update(ctx context.Context, cfg *config.Config, role string, opts UpdateOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if role != "" {
		return r.updateRole(ctx, cfg, role)
	}
	return r.updateAllRoles(ctx, cfg, opts)
}

// updateRole updates documentation for a single role.
func (r *Runner) updateRole(ctx context.Context, cfg *config.Config, roleName string) error {
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

	return r.updateRoleWithType(ctx, cfg, roleName, repoType)
}

// updateAllRoles updates documentation for all roles.
func (r *Runner) updateAllRoles(ctx context.Context, cfg *config.Config, opts UpdateOptions) error {
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
		result := r.updateRoleWithResult(ctx, cfg, role, "saltbox")
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
		result := r.updateRoleWithResult(ctx, cfg, role, "sandbox")
		summary.AddRole(result)

		switch result.Status {
		case github.StatusSkipped:
			r.printf("Skipping %s: %s\n", role, result.SkipReason)
		case github.StatusError:
			r.errorf("Error: failed to update %s: %s\n", role, result.Error)
		}
	}

	r.printf("Updated %d roles, %d unchanged, %d skipped, %d errors\n", summary.Updated, summary.Unchanged, summary.Skipped, summary.Errors)

	// Update CLI help unless --no-cli was specified
	if !opts.NoCLI {
		changed, err := r.UpdateCLIHelp(ctx, cfg, "")
		if err != nil {
			r.errorf("Warning: failed to update CLI help: %v\n", err)
		} else if changed {
			summary.CLIUpdated = true
		}
	}

	// Run coverage checks if requested
	if opts.RunCheck {
		checkResult, err := r.runCoverageChecks(ctx, cfg)
		if err != nil {
			r.errorf("Warning: failed to run coverage checks: %v\n", err)
		} else {
			summary.SetCheckResult(checkResult)

			// Print check results
			r.printCoverageCheckResults(checkResult)

			// Manage GitHub issue if requested
			if opts.ManageIssue {
				repo := github.GetRepository()
				workflowURL := github.GetWorkflowURL()
				issueManager := github.NewIssueManager(repo, workflowURL)

				if err := issueManager.ManageIssue(checkResult, opts.IssueLabel); err != nil {
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
func (r *Runner) updateRoleWithType(ctx context.Context, cfg *config.Config, roleName, repoType string) error {
	result := r.updateRoleWithResult(ctx, cfg, roleName, repoType)
	if result.Status == github.StatusError {
		return fmt.Errorf("%s", result.Error)
	}
	if result.Status == github.StatusSkipped {
		return &skipError{reason: result.SkipReason}
	}
	return nil
}

// updateRoleWithResult updates documentation for a role and returns a detailed result.
func (r *Runner) updateRoleWithResult(ctx context.Context, cfg *config.Config, roleName, repoType string) github.RoleResult {
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

	// Update inventory section if enabled
	if fmConfig.IsInventorySectionEnabled() && manager.HasVariablesSection(doc) {
		if _, err := os.Stat(defaultsPath); os.IsNotExist(err) {
			inventorySkipReason = "no defaults/main.yml"
		} else {
			// Parse the role
			p := parser.New(roleName, repoType)
			roleInfo, err := p.ParseFile(defaultsPath)
			if err != nil {
				result.Status = github.StatusError
				result.Error = fmt.Sprintf("parsing: %v", err)
				return result
			}

			// Skip if no variables (use filtered count for this check)
			filteredVars := parser.FilterVariables(roleInfo.AllVariables, roleName)
			if len(filteredVars) == 0 {
				inventorySkipReason = "no documentable variables"
			} else {
				// Build template data
				data := render.BuildRoleData(roleInfo, cfg, fmConfig)

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
	if err := manager.SaveDocument(doc); err != nil {
		result.Status = github.StatusError
		result.Error = fmt.Sprintf("saving document: %v", err)
		return result
	}

	r.verbosef("  Updated %s\n", docPath)

	return result
}

// runCoverageChecks performs coverage checks and returns the results.
func (r *Runner) runCoverageChecks(ctx context.Context, cfg *config.Config) (*github.CheckResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := &github.CheckResult{}

	// Create blacklist sets for quick lookup
	saltboxBlacklist := make(map[string]bool)
	for _, r := range cfg.Blacklist.DocsCoverage.Saltbox {
		saltboxBlacklist[r] = true
	}
	sandboxBlacklist := make(map[string]bool)
	for _, r := range cfg.Blacklist.DocsCoverage.Sandbox {
		sandboxBlacklist[r] = true
	}

	// Get all roles
	saltboxRoles, err := listRoles(cfg.SaltboxRolesPath())
	if err != nil {
		return nil, fmt.Errorf("listing saltbox roles: %w", err)
	}

	sandboxRoles, err := listRoles(cfg.SandboxRolesPath())
	if err != nil {
		return nil, fmt.Errorf("listing sandbox roles: %w", err)
	}

	// Filter blacklisted roles
	saltboxRoles = filterBlacklist(saltboxRoles, cfg.Blacklist.DocsCoverage.Saltbox)
	sandboxRoles = filterBlacklist(sandboxRoles, cfg.Blacklist.DocsCoverage.Sandbox)

	// Get all documentation files
	saltboxDocs, err := document.ListDocFiles(cfg.SaltboxDocsPath())
	if err != nil {
		return nil, fmt.Errorf("listing saltbox docs: %w", err)
	}

	sandboxDocs, err := document.ListDocFiles(cfg.SandboxDocsPath())
	if err != nil {
		return nil, fmt.Errorf("listing sandbox docs: %w", err)
	}

	// Create maps for quick lookup
	saltboxDocMap := make(map[string]string)
	for _, path := range saltboxDocs {
		name := document.ExtractRoleName(path)
		saltboxDocMap[name] = path
	}

	sandboxDocMap := make(map[string]string)
	for _, path := range sandboxDocs {
		name := document.ExtractRoleName(path)
		sandboxDocMap[name] = path
	}

	saltboxRoleSet := make(map[string]bool)
	for _, role := range saltboxRoles {
		saltboxRoleSet[role] = true
	}

	sandboxRoleSet := make(map[string]bool)
	for _, role := range sandboxRoles {
		sandboxRoleSet[role] = true
	}

	// Check for missing documentation
	for _, role := range saltboxRoles {
		if !roleHasDocCheck(cfg, role, "saltbox", saltboxDocMap) {
			result.MissingDocs = append(result.MissingDocs, role)
		}
	}

	for _, role := range sandboxRoles {
		if !roleHasDocCheck(cfg, role, "sandbox", sandboxDocMap) {
			result.MissingDocs = append(result.MissingDocs, "sandbox/"+role)
		}
	}

	// Build set of doc names that are targets of path overrides
	overrideTargets := make(map[string]bool)
	for _, repoOverrides := range cfg.PathOverrides {
		for _, overridePath := range repoOverrides {
			fullPath := filepath.Join(cfg.Repositories.Docs, overridePath)
			if _, err := os.Stat(fullPath); err == nil {
				baseName := strings.TrimSuffix(filepath.Base(overridePath), ".md")
				overrideTargets[baseName] = true
			}
		}
	}

	// Check for orphaned documentation
	for name := range saltboxDocMap {
		if saltboxBlacklist[name] || overrideTargets[name] {
			continue
		}
		if !saltboxRoleSet[name] {
			result.OrphanedDocs = append(result.OrphanedDocs, name)
		}
	}

	for name := range sandboxDocMap {
		if sandboxBlacklist[name] || overrideTargets[name] {
			continue
		}
		if !sandboxRoleSet[name] {
			result.OrphanedDocs = append(result.OrphanedDocs, "sandbox/"+name)
		}
	}

	// Check for missing managed sections
	manager := document.NewManager(document.MarkerConfig{
		Variables: cfg.Markers.Variables,
		CLI:       cfg.Markers.CLI,
		Overview:  cfg.Markers.Overview,
	})
	checkedDocs := make(map[string]bool)

	// Check saltbox docs
	for _, docPath := range saltboxDocs {
		checkedDocs[docPath] = true
		roleName := document.ExtractRoleName(docPath)
		if saltboxBlacklist[roleName] {
			continue
		}
		defaultsPath := filepath.Join(cfg.SaltboxRolesPath(), roleName, "defaults", "main.yml")
		hasDefaults := true
		if _, err := os.Stat(defaultsPath); err != nil {
			if !os.IsNotExist(err) {
				r.errorf("Warning: failed to stat %s: %v\n", defaultsPath, err)
			}
			hasDefaults = false
		}
		r.checkDocManagedSections(manager, docPath, cfg.Repositories.Docs, result, hasDefaults)
	}

	// Check sandbox docs
	for _, docPath := range sandboxDocs {
		checkedDocs[docPath] = true
		roleName := document.ExtractRoleName(docPath)
		if sandboxBlacklist[roleName] {
			continue
		}
		defaultsPath := filepath.Join(cfg.SandboxRolesPath(), roleName, "defaults", "main.yml")
		hasDefaults := true
		if _, err := os.Stat(defaultsPath); err != nil {
			if !os.IsNotExist(err) {
				r.errorf("Warning: failed to stat %s: %v\n", defaultsPath, err)
			}
			hasDefaults = false
		}
		r.checkDocManagedSections(manager, docPath, cfg.Repositories.Docs, result, hasDefaults)
	}

	return result, nil
}

// roleHasDocCheck checks if a role has documentation.
func roleHasDocCheck(cfg *config.Config, roleName, repoType string, docMap map[string]string) bool {
	if repoOverrides, ok := cfg.PathOverrides[repoType]; ok {
		if override, ok := repoOverrides[roleName]; ok {
			docPath := filepath.Join(cfg.Repositories.Docs, override)
			_, err := os.Stat(docPath)
			return err == nil
		}
	}
	_, exists := docMap[roleName]
	return exists
}

// checkDocManagedSections checks if a doc has the managed sections.
func (r *Runner) checkDocManagedSections(manager *document.Manager, docPath, docsRoot string, result *github.CheckResult, hasDefaults bool) {
	doc, err := manager.LoadDocument(docPath)
	if err != nil {
		r.errorf("Warning: failed to load %s: %v\n", docPath, err)
		return
	}

	if manager.IsAutomationDisabled(doc) {
		return
	}

	var fmConfig *document.SaltboxAutomationConfig
	if doc.Frontmatter != nil {
		fmConfig = doc.Frontmatter.SaltboxAutomation
	}

	relPath, _ := filepath.Rel(docsRoot, docPath)

	if hasDefaults && fmConfig.IsInventorySectionEnabled() && !manager.HasVariablesSection(doc) {
		result.MissingSections = append(result.MissingSections, relPath)
	}

	if fmConfig.IsOverviewSectionEnabled() && !manager.HasOverviewSection(doc) {
		result.MissingOverviewSections = append(result.MissingOverviewSections, relPath)
	}
}

// printCoverageCheckResults prints the coverage check results.
func (r *Runner) printCoverageCheckResults(result *github.CheckResult) {
	r.printf("\n## Coverage Check Results\n\n")

	if len(result.MissingDocs) > 0 {
		r.printf("Missing Documentation: %d roles\n", len(result.MissingDocs))
	}

	if len(result.MissingSections) > 0 {
		r.printf("Missing Variables Sections: %d docs\n", len(result.MissingSections))
	}

	if len(result.MissingOverviewSections) > 0 {
		r.printf("Missing Overview Sections: %d docs\n", len(result.MissingOverviewSections))
	}

	if len(result.OrphanedDocs) > 0 {
		r.printf("Orphaned Documentation: %d docs\n", len(result.OrphanedDocs))
	}

	total := result.TotalIssues()
	if total == 0 {
		r.printf("✅ All coverage checks passed!\n")
	} else {
		r.printf("❌ Found %d issue(s)\n", total)
	}
}
