package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/saltyorg/docs-automation/internal/cli"
	"github.com/saltyorg/docs-automation/internal/config"
	"github.com/saltyorg/docs-automation/internal/docs"
	"github.com/saltyorg/docs-automation/internal/parser"
	"github.com/saltyorg/docs-automation/internal/template"
	"github.com/spf13/cobra"
)

var generateCmd = &cobra.Command{
	Use:   "generate [role]",
	Short: "Generate documentation content to stdout",
	Long: `Generate documentation content to stdout.

Without a role argument, generates all roles + CLI help.
With a role argument, generates only that role (no CLI by default).`,
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
			// Generate single role
			return generateRole(cfg, role)
		}

		// Generate all roles
		return generateAllRoles(cfg)
	},
}

var (
	generateCLI bool
)

func init() {
	generateCmd.Flags().BoolVar(&generateCLI, "cli", false, "include CLI help generation")
	rootCmd.AddCommand(generateCmd)
}

// generateRole generates documentation for a single role.
func generateRole(cfg *config.Config, roleName string) error {
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

	// Parse the role
	p := parser.New(roleName, repoType)
	roleInfo, err := p.ParseFile(defaultsPath)
	if err != nil {
		return fmt.Errorf("parsing role %q: %w", roleName, err)
	}

	// Note: Variable filtering is now done in BuildRoleData to ensure
	// sections are also filtered consistently

	// Try to load frontmatter from existing doc
	var fmConfig *docs.SaltboxAutomationConfig
	docPath := getDocPath(cfg, roleName, repoType)
	if docPath != "" {
		if content, err := os.ReadFile(docPath); err == nil {
			if fm, _, err := docs.ParseFrontmatter(string(content)); err == nil && fm != nil {
				fmConfig = fm.SaltboxAutomation
			}
		}
	}

	// Build template data
	data := template.BuildRoleData(roleInfo, cfg, fmConfig)

	// Create template engine and render
	engine := template.New()
	if err := engine.LoadRoleTemplate(cfg.RoleVariablesTemplatePath()); err != nil {
		return fmt.Errorf("loading template: %w", err)
	}

	output, err := engine.Render("role", data)
	if err != nil {
		return fmt.Errorf("rendering template: %w", err)
	}

	fmt.Print(output)
	return nil
}

// generateAllRoles generates documentation for all roles.
func generateAllRoles(cfg *config.Config) error {
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

	// Generate each role
	for _, role := range saltboxRoles {
		if IsVerbose() {
			fmt.Fprintf(os.Stderr, "Generating: %s (saltbox)\n", role)
		}
		if err := generateRoleWithType(cfg, role, "saltbox"); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to generate %s: %v\n", role, err)
		}
	}

	for _, role := range sandboxRoles {
		if IsVerbose() {
			fmt.Fprintf(os.Stderr, "Generating: %s (sandbox)\n", role)
		}
		if err := generateRoleWithType(cfg, role, "sandbox"); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to generate %s: %v\n", role, err)
		}
	}

	// Generate CLI help if --cli was specified
	if generateCLI {
		if err := generateCLIHelp(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to generate CLI help: %v\n", err)
		}
	}

	return nil
}

// generateRoleWithType generates documentation for a role with known repo type.
func generateRoleWithType(cfg *config.Config, roleName, repoType string) error {
	var rolesPath string
	if repoType == "saltbox" {
		rolesPath = cfg.SaltboxRolesPath()
	} else {
		rolesPath = cfg.SandboxRolesPath()
	}

	defaultsPath := filepath.Join(rolesPath, roleName, "defaults", "main.yml")

	// Check if defaults file exists
	if _, err := os.Stat(defaultsPath); os.IsNotExist(err) {
		return fmt.Errorf("no defaults/main.yml found")
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
		if IsVerbose() {
			fmt.Fprintf(os.Stderr, "  Skipping %s: no documentable variables\n", roleName)
		}
		return nil
	}

	// Try to load frontmatter from existing doc
	var fmConfig *docs.SaltboxAutomationConfig
	docPath := getDocPath(cfg, roleName, repoType)
	if docPath != "" {
		if content, err := os.ReadFile(docPath); err == nil {
			if fm, _, err := docs.ParseFrontmatter(string(content)); err == nil && fm != nil {
				fmConfig = fm.SaltboxAutomation
			}
		}
	}

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

	// Print with role header for clarity
	fmt.Printf("\n=== %s (%s) ===\n", roleName, repoType)
	fmt.Print(output)

	return nil
}

// listRoles returns all role names in a roles directory.
func listRoles(rolesPath string) ([]string, error) {
	entries, err := os.ReadDir(rolesPath)
	if err != nil {
		return nil, err
	}

	var roles []string
	for _, entry := range entries {
		if entry.IsDir() {
			roles = append(roles, entry.Name())
		}
	}
	return roles, nil
}

// filterBlacklist removes blacklisted roles from a list.
func filterBlacklist(roles, blacklist []string) []string {
	blacklistMap := make(map[string]bool)
	for _, r := range blacklist {
		blacklistMap[r] = true
	}

	var filtered []string
	for _, r := range roles {
		if !blacklistMap[r] {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// getDocPath returns the documentation file path for a role.
func getDocPath(cfg *config.Config, roleName, repoType string) string {
	// Check for path override for this repo type
	if repoOverrides, ok := cfg.PathOverrides[repoType]; ok {
		if override, ok := repoOverrides[roleName]; ok {
			return filepath.Join(cfg.Repositories.Docs, override)
		}
	}

	var docPath string
	if repoType == "saltbox" {
		docPath = filepath.Join(cfg.SaltboxDocsPath(), roleName+".md")
	} else {
		docPath = filepath.Join(cfg.SandboxDocsPath(), roleName+".md")
	}

	return docPath
}

// generateCLIHelp generates CLI help content to stdout.
func generateCLIHelp(cfg *config.Config) error {
	binaryPath := cfg.CLIHelp.BinaryPath
	if binaryPath == "" {
		return fmt.Errorf("no binary path configured")
	}

	templatePath := cfg.CLIHelpTemplatePath()

	generator := cli.NewHelpGenerator(binaryPath, templatePath)
	if !generator.BinaryExists() {
		return fmt.Errorf("binary not found at %s", binaryPath)
	}

	if err := generator.LoadTemplate(); err != nil {
		return fmt.Errorf("loading template: %w", err)
	}

	helpContent, err := generator.Generate()
	if err != nil {
		return fmt.Errorf("generating help: %w", err)
	}

	fmt.Println("\n=== CLI Help ===")
	fmt.Print(helpContent)
	return nil
}
