package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/saltyorg/docs-automation/internal/config"
	"github.com/saltyorg/docs-automation/internal/details"
	"github.com/saltyorg/docs-automation/internal/docs"
	"github.com/saltyorg/docs-automation/internal/github"
	"github.com/saltyorg/docs-automation/internal/parser"
	"github.com/saltyorg/docs-automation/internal/template"
	"github.com/spf13/cobra"
)

var (
	updateNoCLI       bool
	updateRunCheck    bool
	updateManageIssue bool
	updateIssueLabel  string
)

// skipError represents a non-fatal skip condition (not an actual error).
type skipError struct {
	reason string
}

func (e *skipError) Error() string {
	return e.reason
}

var updateCmd = &cobra.Command{
	Use:   "update [role]",
	Short: "Update documentation files in place",
	Long: `Update documentation files in place.

Without a role argument, updates all roles + CLI help.
With a role argument, updates only that role (no CLI by default).`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load configuration
		cfg, err := config.Load(GetConfigPath())
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		role := ""
		if len(args) > 0 {
			role = args[0]
		}

		if role != "" {
			// Update single role
			return updateRole(cfg, role)
		}

		// Update all roles
		return updateAllRoles(cfg)
	},
}

func init() {
	updateCmd.Flags().BoolVar(&updateNoCLI, "no-cli", false, "exclude CLI help generation")
	updateCmd.Flags().BoolVar(&updateRunCheck, "check", false, "run coverage checks after updating")
	updateCmd.Flags().BoolVar(&updateManageIssue, "manage-issue", false, "create/update/close GitHub issue based on check results (requires --check and gh CLI)")
	updateCmd.Flags().StringVar(&updateIssueLabel, "issue-label", "docs-automation", "label to use for the managed GitHub issue")
	rootCmd.AddCommand(updateCmd)
}

// updateRole updates documentation for a single role.
func updateRole(cfg *config.Config, roleName string) error {
	// Try to find the role in saltbox first, then sandbox
	defaultsPath := filepath.Join(cfg.SaltboxRolesPath(), roleName, "defaults", "main.yml")
	repoType := "saltbox"

	if _, err := os.Stat(defaultsPath); os.IsNotExist(err) {
		// Try sandbox
		defaultsPath = filepath.Join(cfg.SandboxRolesPath(), roleName, "defaults", "main.yml")
		repoType = "sandbox"

		if _, err := os.Stat(defaultsPath); os.IsNotExist(err) {
			return fmt.Errorf("role %q not found in saltbox or sandbox", roleName)
		}
	}

	return updateRoleWithType(cfg, roleName, repoType)
}

// updateAllRoles updates documentation for all roles.
func updateAllRoles(cfg *config.Config) error {
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

	if IsVerbose() {
		fmt.Fprintf(os.Stderr, "Found %d saltbox roles and %d sandbox roles\n",
			len(saltboxRoles), len(sandboxRoles))
	}

	summary := github.NewUpdateSummary()

	// Update each role
	for _, role := range saltboxRoles {
		if IsVerbose() {
			fmt.Fprintf(os.Stderr, "Updating: %s (saltbox)\n", role)
		}
		result := updateRoleWithResult(cfg, role, "saltbox")
		summary.AddRole(result)

		switch result.Status {
		case github.StatusSkipped:
			fmt.Printf("Skipping %s: %s\n", role, result.SkipReason)
		case github.StatusError:
			fmt.Fprintf(os.Stderr, "Error: failed to update %s: %s\n", role, result.Error)
		}
	}

	for _, role := range sandboxRoles {
		if IsVerbose() {
			fmt.Fprintf(os.Stderr, "Updating: %s (sandbox)\n", role)
		}
		result := updateRoleWithResult(cfg, role, "sandbox")
		summary.AddRole(result)

		switch result.Status {
		case github.StatusSkipped:
			fmt.Printf("Skipping %s: %s\n", role, result.SkipReason)
		case github.StatusError:
			fmt.Fprintf(os.Stderr, "Error: failed to update %s: %s\n", role, result.Error)
		}
	}

	fmt.Printf("Updated %d roles, %d unchanged, %d skipped, %d errors\n", summary.Updated, summary.Unchanged, summary.Skipped, summary.Errors)

	// Update CLI help unless --no-cli was specified
	if !updateNoCLI {
		changed, err := updateCLIHelp(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update CLI help: %v\n", err)
		} else if changed {
			summary.CLIUpdated = true
		}
	}

	// Run coverage checks if requested
	if updateRunCheck {
		checkResult, err := runCoverageChecks(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to run coverage checks: %v\n", err)
		} else {
			summary.SetCheckResult(checkResult)

			// Print check results
			printCoverageCheckResults(checkResult)

			// Manage GitHub issue if requested
			if updateManageIssue {
				repo := github.GetRepository()
				workflowURL := github.GetWorkflowURL()
				issueManager := github.NewIssueManager(repo, workflowURL)

				if err := issueManager.ManageIssue(checkResult, updateIssueLabel); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to manage GitHub issue: %v\n", err)
				}
			}
		}
	}

	// Write GitHub Actions summary
	if err := summary.WriteGitHubSummary(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write GitHub summary: %v\n", err)
	}

	return nil
}

// updateRoleWithType updates documentation for a role with known repo type.
func updateRoleWithType(cfg *config.Config, roleName, repoType string) error {
	result := updateRoleWithResult(cfg, roleName, repoType)
	if result.Status == github.StatusError {
		return fmt.Errorf("%s", result.Error)
	}
	if result.Status == github.StatusSkipped {
		return &skipError{reason: result.SkipReason}
	}
	return nil
}

// updateRoleWithResult updates documentation for a role and returns a detailed result.
func updateRoleWithResult(cfg *config.Config, roleName, repoType string) github.RoleResult {
	result := github.RoleResult{
		Name:     roleName,
		RepoType: repoType,
		Status:   github.StatusUpdated,
		Sections: []string{},
	}

	var rolesPath string
	if repoType == "saltbox" {
		rolesPath = cfg.SaltboxRolesPath()
	} else {
		rolesPath = cfg.SandboxRolesPath()
	}

	defaultsPath := filepath.Join(rolesPath, roleName, "defaults", "main.yml")

	// Check if defaults file exists
	if _, err := os.Stat(defaultsPath); os.IsNotExist(err) {
		result.Status = github.StatusSkipped
		result.SkipReason = "no defaults/main.yml"
		return result
	}

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
		result.Status = github.StatusSkipped
		result.SkipReason = "no documentable variables"
		return result
	}

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
	manager := docs.NewManager(docs.MarkerConfig{
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
	var fmConfig *docs.SaltboxAutomationConfig
	if doc.Frontmatter != nil {
		fmConfig = doc.Frontmatter.SaltboxAutomation
	}

	// Update inventory section if enabled
	if fmConfig.IsInventorySectionEnabled() && manager.HasVariablesSection(doc) {
		// Build template data
		data := template.BuildRoleData(roleInfo, cfg, fmConfig)

		// Create template engine and render
		engine := template.New()
		if err := engine.LoadRoleTemplate(cfg.RoleVariablesTemplatePath()); err != nil {
			result.Status = github.StatusError
			result.Error = fmt.Sprintf("loading template: %v", err)
			return result
		}

		output, err := engine.Render("role", data)
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

	// Update overview section if enabled and the document has the section
	if fmConfig.IsOverviewSectionEnabled() && manager.HasOverviewSection(doc) {
		tableGen := details.NewTableGenerator(cfg.OverviewTemplatePath())
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
		result.SkipReason = "no enabled sections to update"
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

	if IsVerbose() {
		fmt.Fprintf(os.Stderr, "  Updated %s\n", docPath)
	}

	return result
}

// runCoverageChecks performs coverage checks and returns the results.
func runCoverageChecks(cfg *config.Config) (*github.CheckResult, error) {
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
	saltboxDocs, err := docs.ListDocFiles(cfg.SaltboxDocsPath())
	if err != nil {
		return nil, fmt.Errorf("listing saltbox docs: %w", err)
	}

	sandboxDocs, err := docs.ListDocFiles(cfg.SandboxDocsPath())
	if err != nil {
		return nil, fmt.Errorf("listing sandbox docs: %w", err)
	}

	// Create maps for quick lookup
	saltboxDocMap := make(map[string]string)
	for _, path := range saltboxDocs {
		name := docs.ExtractRoleName(path)
		saltboxDocMap[name] = path
	}

	sandboxDocMap := make(map[string]string)
	for _, path := range sandboxDocs {
		name := docs.ExtractRoleName(path)
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
	manager := docs.NewManager(docs.MarkerConfig{
		Variables: cfg.Markers.Variables,
		CLI:       cfg.Markers.CLI,
		Overview:  cfg.Markers.Overview,
	})

	// Check saltbox docs
	for _, docPath := range saltboxDocs {
		roleName := docs.ExtractRoleName(docPath)
		if saltboxBlacklist[roleName] {
			continue
		}
		defaultsPath := filepath.Join(cfg.SaltboxRolesPath(), roleName, "defaults", "main.yml")
		if _, err := os.Stat(defaultsPath); os.IsNotExist(err) {
			continue
		}
		checkDocManagedSections(manager, docPath, cfg.Repositories.Docs, result)
	}

	// Check sandbox docs
	for _, docPath := range sandboxDocs {
		roleName := docs.ExtractRoleName(docPath)
		if sandboxBlacklist[roleName] {
			continue
		}
		defaultsPath := filepath.Join(cfg.SandboxRolesPath(), roleName, "defaults", "main.yml")
		if _, err := os.Stat(defaultsPath); os.IsNotExist(err) {
			continue
		}
		checkDocManagedSections(manager, docPath, cfg.Repositories.Docs, result)
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
func checkDocManagedSections(manager *docs.Manager, docPath, docsRoot string, result *github.CheckResult) {
	doc, err := manager.LoadDocument(docPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load %s: %v\n", docPath, err)
		return
	}

	if manager.IsAutomationDisabled(doc) {
		return
	}

	var fmConfig *docs.SaltboxAutomationConfig
	if doc.Frontmatter != nil {
		fmConfig = doc.Frontmatter.SaltboxAutomation
	}

	relPath, _ := filepath.Rel(docsRoot, docPath)

	if fmConfig.IsInventorySectionEnabled() && !manager.HasVariablesSection(doc) {
		result.MissingSections = append(result.MissingSections, relPath)
	}

	if fmConfig.IsOverviewSectionEnabled() && !manager.HasOverviewSection(doc) {
		result.MissingDetailsSections = append(result.MissingDetailsSections, relPath)
	}
}

// printCoverageCheckResults prints the coverage check results.
func printCoverageCheckResults(result *github.CheckResult) {
	fmt.Println()
	fmt.Println("## Coverage Check Results")
	fmt.Println()

	if len(result.MissingDocs) > 0 {
		fmt.Printf("Missing Documentation: %d roles\n", len(result.MissingDocs))
	}

	if len(result.MissingSections) > 0 {
		fmt.Printf("Missing Variables Sections: %d docs\n", len(result.MissingSections))
	}

	if len(result.MissingDetailsSections) > 0 {
		fmt.Printf("Missing Overview Sections: %d docs\n", len(result.MissingDetailsSections))
	}

	if len(result.OrphanedDocs) > 0 {
		fmt.Printf("Orphaned Documentation: %d docs\n", len(result.OrphanedDocs))
	}

	total := result.TotalIssues()
	if total == 0 {
		fmt.Println("✅ All coverage checks passed!")
	} else {
		fmt.Printf("❌ Found %d issue(s)\n", total)
	}
}
