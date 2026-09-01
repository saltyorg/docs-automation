package automation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/saltyorg/docs-automation/clihelp"
	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/document"
	"github.com/saltyorg/docs-automation/parser"
	"github.com/saltyorg/docs-automation/render"
)

// GenerateOptions controls optional output included during generation.
type GenerateOptions struct {
	IncludeCLI bool
}

// Generate renders one role or all configured roles to the runner's output.
func (r *Runner) Generate(ctx context.Context, cfg *config.Config, role string, opts GenerateOptions) (err error) {
	defer func() { err = r.result(err) }()
	if err := ctx.Err(); err != nil {
		return err
	}
	sources, err := loadSourceCatalog(cfg)
	if err != nil {
		return err
	}
	if role != "" {
		return r.generateRole(ctx, cfg, sources, role)
	}
	return r.generateAllRoles(ctx, cfg, sources, opts.IncludeCLI)
}

// generateRole generates documentation for a single role.
func (r *Runner) generateRole(ctx context.Context, cfg *config.Config, sources render.SourceCatalog, roleName string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
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
	var fmConfig *document.SaltboxAutomationConfig
	docPath := getDocPath(cfg, roleName, repoType)
	if docPath != "" {
		if content, err := os.ReadFile(docPath); err == nil {
			if fm, _, err := document.ParseFrontmatter(string(content)); err == nil && fm != nil {
				fmConfig = fm.SaltboxAutomation
			}
		}
	}

	// Build template data
	data := render.BuildRoleData(roleInfo, cfg, fmConfig, sources)

	// Create template engine and render
	engine := render.New()
	if err := engine.LoadFile("inventory", cfg.InventoryTemplatePath()); err != nil {
		return fmt.Errorf("loading template: %w", err)
	}

	output, err := engine.Render("inventory", data)
	if err != nil {
		return fmt.Errorf("rendering template: %w", err)
	}

	r.printf("%s", output)
	return nil
}

// generateAllRoles generates documentation for all roles.
func (r *Runner) generateAllRoles(ctx context.Context, cfg *config.Config, sources render.SourceCatalog, includeCLI bool) error {
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

	// Generate each role
	for _, role := range saltboxRoles {
		if err := ctx.Err(); err != nil {
			return err
		}
		r.verbosef("Generating: %s (saltbox)\n", role)
		if err := r.generateRoleWithType(ctx, cfg, sources, role, "saltbox"); err != nil {
			r.errorf("Warning: failed to generate %s: %v\n", role, err)
		}
	}

	for _, role := range sandboxRoles {
		if err := ctx.Err(); err != nil {
			return err
		}
		r.verbosef("Generating: %s (sandbox)\n", role)
		if err := r.generateRoleWithType(ctx, cfg, sources, role, "sandbox"); err != nil {
			r.errorf("Warning: failed to generate %s: %v\n", role, err)
		}
	}

	// Generate CLI help if --cli was specified
	if includeCLI {
		if err := r.generateCLIHelp(ctx, cfg); err != nil {
			r.errorf("Warning: failed to generate CLI help: %v\n", err)
		}
	}

	return nil
}

// generateRoleWithType generates documentation for a role with known repo type.
func (r *Runner) generateRoleWithType(ctx context.Context, cfg *config.Config, sources render.SourceCatalog, roleName, repoType string) error {
	if err := ctx.Err(); err != nil {
		return err
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
		r.verbosef("  Skipping %s: no documentable variables\n", roleName)
		return nil
	}

	// Try to load frontmatter from existing doc
	var fmConfig *document.SaltboxAutomationConfig
	docPath := getDocPath(cfg, roleName, repoType)
	if docPath != "" {
		if content, err := os.ReadFile(docPath); err == nil {
			if fm, _, err := document.ParseFrontmatter(string(content)); err == nil && fm != nil {
				fmConfig = fm.SaltboxAutomation
			}
		}
	}

	// Build template data
	data := render.BuildRoleData(roleInfo, cfg, fmConfig, sources)

	// Create template engine and render
	engine := render.New()
	if err := engine.LoadFile("inventory", cfg.InventoryTemplatePath()); err != nil {
		return fmt.Errorf("loading template: %w", err)
	}

	output, err := engine.Render("inventory", data)
	if err != nil {
		return fmt.Errorf("rendering: %w", err)
	}

	// Print with role header for clarity
	r.printf("\n=== %s (%s) ===\n", roleName, repoType)
	r.printf("%s", output)

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
func (r *Runner) generateCLIHelp(ctx context.Context, cfg *config.Config) error {
	binaryPath := cfg.CLIHelp.BinaryPath
	if binaryPath == "" {
		return fmt.Errorf("no binary path configured")
	}

	templatePath := cfg.CLIHelpTemplatePath()

	generator := clihelp.NewHelpGenerator(binaryPath, templatePath)
	if !generator.BinaryExists() {
		return fmt.Errorf("binary not found at %s", binaryPath)
	}

	if err := generator.LoadTemplate(); err != nil {
		return fmt.Errorf("loading template: %w", err)
	}

	helpContent, err := generator.Generate(ctx)
	if err != nil {
		return fmt.Errorf("generating help: %w", err)
	}

	r.printf("\n=== CLI Help ===\n")
	r.printf("%s", helpContent)
	return nil
}
