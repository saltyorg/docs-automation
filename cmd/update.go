package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/saltyorg/docs-automation/internal/config"
	"github.com/saltyorg/docs-automation/internal/details"
	"github.com/saltyorg/docs-automation/internal/docs"
	"github.com/saltyorg/docs-automation/internal/parser"
	"github.com/saltyorg/docs-automation/internal/template"
	"github.com/spf13/cobra"
)

var (
	updateNoCLI bool
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

	updated := 0
	skipped := 0
	errors := 0

	// Update each role
	for _, role := range saltboxRoles {
		if IsVerbose() {
			fmt.Fprintf(os.Stderr, "Updating: %s (saltbox)\n", role)
		}
		if err := updateRoleWithType(cfg, role, "saltbox"); err != nil {
			if _, ok := err.(*skipError); ok {
				fmt.Printf("Skipping %s: %v\n", role, err)
				skipped++
			} else {
				fmt.Fprintf(os.Stderr, "Error: failed to update %s: %v\n", role, err)
				errors++
			}
		} else {
			updated++
		}
	}

	for _, role := range sandboxRoles {
		if IsVerbose() {
			fmt.Fprintf(os.Stderr, "Updating: %s (sandbox)\n", role)
		}
		if err := updateRoleWithType(cfg, role, "sandbox"); err != nil {
			if _, ok := err.(*skipError); ok {
				fmt.Printf("Skipping %s: %v\n", role, err)
				skipped++
			} else {
				fmt.Fprintf(os.Stderr, "Error: failed to update %s: %v\n", role, err)
				errors++
			}
		} else {
			updated++
		}
	}

	fmt.Printf("Updated %d roles, %d skipped, %d errors\n", updated, skipped, errors)

	// Update CLI help unless --no-cli was specified
	if !updateNoCLI {
		if err := updateCLIHelp(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update CLI help: %v\n", err)
		}
	}

	return nil
}

// updateRoleWithType updates documentation for a role with known repo type.
func updateRoleWithType(cfg *config.Config, roleName, repoType string) error {
	var rolesPath string
	if repoType == "saltbox" {
		rolesPath = cfg.SaltboxRolesPath()
	} else {
		rolesPath = cfg.SandboxRolesPath()
	}

	defaultsPath := filepath.Join(rolesPath, roleName, "defaults", "main.yml")

	// Check if defaults file exists
	if _, err := os.Stat(defaultsPath); os.IsNotExist(err) {
		return &skipError{reason: "no defaults/main.yml"}
	}

	// Parse the role
	p := parser.New(roleName, repoType)
	roleInfo, err := p.ParseFile(defaultsPath)
	if err != nil {
		return fmt.Errorf("parsing: %w", err)
	}

	// Note: Variable filtering is now done in BuildRoleData to ensure
	// sections are also filtered consistently

	// Skip if no variables (use filtered count for this check)
	filteredVars := parser.FilterVariables(roleInfo.AllVariables, roleName)
	if len(filteredVars) == 0 {
		return &skipError{reason: "no documentable variables"}
	}

	// Get documentation path
	docPath := getDocPath(cfg, roleName, repoType)
	if docPath == "" {
		return fmt.Errorf("could not determine doc path")
	}

	// Check if doc file exists
	if _, err := os.Stat(docPath); os.IsNotExist(err) {
		return &skipError{reason: "doc file does not exist"}
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
		return fmt.Errorf("loading document: %w", err)
	}

	// Check if automation is disabled
	if manager.IsAutomationDisabled(doc) {
		return &skipError{reason: "automation disabled in frontmatter"}
	}

	// Get frontmatter config
	var fmConfig *docs.SaltboxAutomationConfig
	if doc.Frontmatter != nil {
		fmConfig = doc.Frontmatter.SaltboxAutomation
	}

	updatedAny := false

	// Update inventory section if enabled
	if fmConfig.IsInventorySectionEnabled() && manager.HasVariablesSection(doc) {
		// Build template data
		data := template.BuildRoleData(roleInfo, cfg, fmConfig)

		// Create template engine and render
		engine := template.New()
		if err := engine.LoadRoleTemplate(cfg.RoleVariablesTemplatePath()); err != nil {
			return fmt.Errorf("loading template: %w", err)
		}

		output, err := engine.Render("role", data)
		if err != nil {
			return fmt.Errorf("rendering: %w", err)
		}

		// Update the managed section
		if err := manager.UpdateVariablesSection(doc, output); err != nil {
			return fmt.Errorf("updating section: %w", err)
		}
		updatedAny = true
	}

	// Update overview section if enabled and the document has the section
	if fmConfig.IsOverviewSectionEnabled() && manager.HasOverviewSection(doc) {
		tableGen := details.NewTableGenerator(cfg.OverviewTemplatePath())
		if err := tableGen.LoadTemplate(); err != nil {
			return fmt.Errorf("loading overview template: %w", err)
		}
		tableContent, err := tableGen.GenerateFromDocument(doc)
		if err != nil {
			return fmt.Errorf("generating overview table: %w", err)
		}
		if tableContent != "" {
			if err := manager.UpdateOverviewSection(doc, tableContent); err != nil {
				return fmt.Errorf("updating overview section: %w", err)
			}
			updatedAny = true
		}
	}

	// Skip if nothing was updated
	if !updatedAny {
		return &skipError{reason: "no enabled sections to update"}
	}

	// Save the document
	if err := manager.SaveDocument(doc); err != nil {
		return fmt.Errorf("saving document: %w", err)
	}

	if IsVerbose() {
		fmt.Fprintf(os.Stderr, "  Updated %s\n", docPath)
	}

	return nil
}
