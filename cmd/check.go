package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/saltyorg/docs-automation/internal/config"
	"github.com/saltyorg/docs-automation/internal/docs"
	"github.com/saltyorg/docs-automation/internal/github"
	"github.com/spf13/cobra"
)

var (
	manageIssue bool
	issueLabel  string
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Run all coverage checks",
	Long: `Run all coverage checks and optionally manage a unified GitHub issue.

Checks for:
  - Missing documentation for roles
  - Missing managed section markers
  - Orphaned documentation

Use --manage-issue to automatically create, update, or close a GitHub issue
based on the check results. This requires the gh CLI to be installed and
authenticated.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load configuration
		cfg, err := config.Load(GetConfigPath())
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		return runChecks(cfg)
	},
}

func init() {
	checkCmd.Flags().BoolVar(&manageIssue, "manage-issue", false, "create/update/close GitHub issue based on results (requires gh CLI)")
	checkCmd.Flags().StringVar(&issueLabel, "issue-label", "docs-automation", "label to use for the managed GitHub issue")
	rootCmd.AddCommand(checkCmd)
}

// CheckResult holds the results of all coverage checks.
type CheckResult struct {
	MissingDocs            []string // Roles without documentation
	MissingSections        []string // Docs without managed variables sections
	MissingDetailsSections []string // Docs without managed details sections
	OrphanedDocs           []string // Docs without corresponding roles
	DisabledAutomation     []string // Docs with automation disabled
}

// roleHasDoc checks if a role has documentation, considering path overrides.
func roleHasDoc(cfg *config.Config, roleName, repoType string, docMap map[string]string) bool {
	// First check path overrides for this repo type
	if repoOverrides, ok := cfg.PathOverrides[repoType]; ok {
		if override, ok := repoOverrides[roleName]; ok {
			docPath := filepath.Join(cfg.Repositories.Docs, override)
			_, err := os.Stat(docPath)
			return err == nil
		}
	}
	// Fall back to standard doc map lookup
	_, exists := docMap[roleName]
	return exists
}

// runChecks performs all coverage checks.
func runChecks(cfg *config.Config) error {
	result := &CheckResult{}

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
		return fmt.Errorf("listing saltbox roles: %w", err)
	}

	sandboxRoles, err := listRoles(cfg.SandboxRolesPath())
	if err != nil {
		return fmt.Errorf("listing sandbox roles: %w", err)
	}

	// Filter blacklisted roles
	saltboxRoles = filterBlacklist(saltboxRoles, cfg.Blacklist.DocsCoverage.Saltbox)
	sandboxRoles = filterBlacklist(sandboxRoles, cfg.Blacklist.DocsCoverage.Sandbox)

	// Get all documentation files
	saltboxDocs, err := docs.ListDocFiles(cfg.SaltboxDocsPath())
	if err != nil {
		return fmt.Errorf("listing saltbox docs: %w", err)
	}

	sandboxDocs, err := docs.ListDocFiles(cfg.SandboxDocsPath())
	if err != nil {
		return fmt.Errorf("listing sandbox docs: %w", err)
	}

	// Create maps for quick lookup
	saltboxDocMap := make(map[string]string) // role name -> doc path
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
		if !roleHasDoc(cfg, role, "saltbox", saltboxDocMap) {
			result.MissingDocs = append(result.MissingDocs, role)
		}
	}

	for _, role := range sandboxRoles {
		if !roleHasDoc(cfg, role, "sandbox", sandboxDocMap) {
			result.MissingDocs = append(result.MissingDocs, "sandbox/"+role)
		}
	}

	// Build set of doc names that are targets of path overrides (only if the file exists)
	overrideTargets := make(map[string]bool)
	for _, repoOverrides := range cfg.PathOverrides {
		for _, overridePath := range repoOverrides {
			// Only consider it a valid override target if the file actually exists
			fullPath := filepath.Join(cfg.Repositories.Docs, overridePath)
			if _, err := os.Stat(fullPath); err == nil {
				// Extract the base name without extension to match against doc maps
				baseName := strings.TrimSuffix(filepath.Base(overridePath), ".md")
				overrideTargets[baseName] = true
			}
		}
	}

	// Check for orphaned documentation
	// Skip blacklisted roles - they are intentionally excluded from role scanning
	// Skip docs that are targets of path overrides (they belong to a different role)
	for name := range saltboxDocMap {
		if saltboxBlacklist[name] {
			continue
		}
		if overrideTargets[name] {
			continue
		}
		if !saltboxRoleSet[name] {
			result.OrphanedDocs = append(result.OrphanedDocs, name)
		}
	}

	for name := range sandboxDocMap {
		if sandboxBlacklist[name] {
			continue
		}
		if overrideTargets[name] {
			continue
		}
		if !sandboxRoleSet[name] {
			result.OrphanedDocs = append(result.OrphanedDocs, "sandbox/"+name)
		}
	}

	// Check for missing managed sections
	// Only check docs for roles that:
	// 1. Are not blacklisted
	// 2. Have a defaults/main.yml file (otherwise no variables to document)
	manager := docs.NewManager(docs.MarkerConfig{
		Variables: cfg.Markers.Variables,
		CLI:       cfg.Markers.CLI,
		Overview:  cfg.Markers.Overview,
	})

	// Check saltbox docs
	for _, docPath := range saltboxDocs {
		roleName := docs.ExtractRoleName(docPath)

		// Skip blacklisted roles
		if saltboxBlacklist[roleName] {
			continue
		}

		// Skip if role doesn't have defaults/main.yml (no variables to document)
		defaultsPath := filepath.Join(cfg.SaltboxRolesPath(), roleName, "defaults", "main.yml")
		if _, err := os.Stat(defaultsPath); os.IsNotExist(err) {
			continue
		}

		checkDocManagedSection(manager, docPath, cfg.Repositories.Docs, result)
	}

	// Check sandbox docs
	for _, docPath := range sandboxDocs {
		roleName := docs.ExtractRoleName(docPath)

		// Skip blacklisted roles
		if sandboxBlacklist[roleName] {
			continue
		}

		// Skip if role doesn't have defaults/main.yml (no variables to document)
		defaultsPath := filepath.Join(cfg.SandboxRolesPath(), roleName, "defaults", "main.yml")
		if _, err := os.Stat(defaultsPath); os.IsNotExist(err) {
			continue
		}

		checkDocManagedSection(manager, docPath, cfg.Repositories.Docs, result)
	}

	// Print results
	printCheckResults(result)

	// Output GitHub Actions variables if running in CI
	ghResult := &github.CheckResult{
		MissingDocs:            result.MissingDocs,
		MissingSections:        result.MissingSections,
		MissingDetailsSections: result.MissingDetailsSections,
		OrphanedDocs:           result.OrphanedDocs,
	}

	repo := github.GetRepository()
	workflowURL := github.GetWorkflowURL()
	issueManager := github.NewIssueManager(repo, workflowURL)
	issueManager.OutputGitHubActions(ghResult)

	// Manage GitHub issue if requested
	if manageIssue {
		if err := issueManager.ManageIssue(ghResult, issueLabel); err != nil {
			return fmt.Errorf("managing GitHub issue: %w", err)
		}
	}

	return nil
}

// checkDocManagedSection checks if a doc has the managed sections.
func checkDocManagedSection(manager *docs.Manager, docPath, docsRoot string, result *CheckResult) {
	doc, err := manager.LoadDocument(docPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load %s: %v\n", docPath, err)
		return
	}

	// Skip if automation is disabled
	if manager.IsAutomationDisabled(doc) {
		relPath, _ := filepath.Rel(docsRoot, docPath)
		result.DisabledAutomation = append(result.DisabledAutomation, relPath)
		return
	}

	// Get frontmatter config for per-section checks
	var fmConfig *docs.SaltboxAutomationConfig
	if doc.Frontmatter != nil {
		fmConfig = doc.Frontmatter.SaltboxAutomation
	}

	relPath, _ := filepath.Rel(docsRoot, docPath)

	// Check for managed inventory section (only if enabled in frontmatter)
	if fmConfig.IsInventorySectionEnabled() && !manager.HasVariablesSection(doc) {
		result.MissingSections = append(result.MissingSections, relPath)
	}

	// Check for managed overview section (only if enabled in frontmatter)
	if fmConfig.IsOverviewSectionEnabled() && !manager.HasOverviewSection(doc) {
		result.MissingDetailsSections = append(result.MissingDetailsSections, relPath)
	}
}

// printCheckResults prints the check results in a formatted way.
func printCheckResults(result *CheckResult) {
	fmt.Println("## 📝 Documentation Status")
	fmt.Println()

	if len(result.MissingDocs) > 0 {
		fmt.Printf("### Missing Documentation (%d)\n", len(result.MissingDocs))
		fmt.Println("Roles without corresponding documentation pages:")
		fmt.Println()
		for _, role := range result.MissingDocs {
			fmt.Printf("- [ ] `%s`\n", role)
		}
		fmt.Println()
	}

	if len(result.MissingSections) > 0 {
		fmt.Printf("### Missing Variables Sections (%d)\n", len(result.MissingSections))
		fmt.Println("Documentation pages without the managed variables section:")
		fmt.Println()
		for _, doc := range result.MissingSections {
			// Convert path to GitHub link format
			docName := strings.TrimSuffix(filepath.Base(doc), ".md")
			fmt.Printf("- [ ] [%s](%s)\n", docName, doc)
		}
		fmt.Println()
	}

	if len(result.MissingDetailsSections) > 0 {
		fmt.Printf("### Missing Details Sections (%d)\n", len(result.MissingDetailsSections))
		fmt.Println("Documentation pages without the managed details section:")
		fmt.Println()
		for _, doc := range result.MissingDetailsSections {
			// Convert path to GitHub link format
			docName := strings.TrimSuffix(filepath.Base(doc), ".md")
			fmt.Printf("- [ ] [%s](%s)\n", docName, doc)
		}
		fmt.Println()
	}

	if len(result.OrphanedDocs) > 0 {
		fmt.Printf("### Orphaned Documentation (%d)\n", len(result.OrphanedDocs))
		fmt.Println("Documentation pages without corresponding roles:")
		fmt.Println()
		for _, doc := range result.OrphanedDocs {
			fmt.Printf("- [ ] `%s`\n", doc)
		}
		fmt.Println()
	}

	if len(result.DisabledAutomation) > 0 && IsVerbose() {
		fmt.Printf("### Automation Disabled (%d)\n", len(result.DisabledAutomation))
		fmt.Println("Documentation pages with automation disabled (skipped):")
		fmt.Println()
		for _, doc := range result.DisabledAutomation {
			fmt.Printf("- `%s`\n", doc)
		}
		fmt.Println()
	}

	// Summary
	total := len(result.MissingDocs) + len(result.MissingSections) + len(result.MissingDetailsSections) + len(result.OrphanedDocs)
	if total == 0 {
		fmt.Println("✅ All checks passed!")
	} else {
		fmt.Printf("❌ Found %d issue(s)\n", total)
	}
}
