package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/saltyorg/docs-automation/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var (
	scaffoldTemplate string
	scaffoldOutput   string
	scaffoldForce    bool
)

var scaffoldCmd = &cobra.Command{
	Use:   "scaffold <role>",
	Short: "Generate new app documentation from template",
	Long: `Generate new app documentation from template.

Creates a starter documentation file at the appropriate path
for the specified role.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load configuration
		cfg, err := config.Load(GetConfigPath())
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		role := args[0]
		return scaffoldRole(cfg, role)
	},
}

func init() {
	scaffoldCmd.Flags().StringVar(&scaffoldTemplate, "template", "", "path to scaffold template (default: from config)")
	scaffoldCmd.Flags().StringVar(&scaffoldOutput, "output", "", "output path override")
	scaffoldCmd.Flags().BoolVar(&scaffoldForce, "force", false, "overwrite existing file if present")
	rootCmd.AddCommand(scaffoldCmd)
}

// ScaffoldData contains data for the scaffold template.
type ScaffoldData struct {
	RoleName  string // e.g., "sonarr"
	RoleTitle string // e.g., "Sonarr" (title case)
	RoleTag   string // e.g., "sonarr" (for install command)
	RepoType  string // "saltbox" or "sandbox"
	TagPrefix string // "" for saltbox, "sandbox-" for sandbox
}

// scaffoldRole creates a new documentation file for a role.
func scaffoldRole(cfg *config.Config, roleName string) error {
	// Determine repo type by checking which repo has the role
	repoType := "saltbox"
	rolesPath := cfg.SaltboxRolesPath()

	if _, err := os.Stat(filepath.Join(rolesPath, roleName)); os.IsNotExist(err) {
		// Try sandbox
		rolesPath = cfg.SandboxRolesPath()
		if _, err := os.Stat(filepath.Join(rolesPath, roleName)); os.IsNotExist(err) {
			return fmt.Errorf("role %q not found in saltbox or sandbox", roleName)
		}
		repoType = "sandbox"
	}

	// Determine output path
	outputPath := scaffoldOutput
	if outputPath == "" {
		pathPattern, ok := cfg.Scaffold.OutputPaths[repoType]
		if !ok {
			return fmt.Errorf("no output path configured for repo type %q", repoType)
		}
		outputPath = filepath.Join(cfg.Repositories.Docs, strings.ReplaceAll(pathPattern, "{role}", roleName))
	}

	// Check if file already exists
	if _, err := os.Stat(outputPath); err == nil && !scaffoldForce {
		return fmt.Errorf("file %s already exists (use --force to overwrite)", outputPath)
	}

	// Prepare template data
	titleCaser := cases.Title(language.English)
	data := ScaffoldData{
		RoleName:  roleName,
		RoleTitle: titleCaser.String(roleName),
		RoleTag:   roleName,
		RepoType:  repoType,
		TagPrefix: "",
	}
	if repoType == "sandbox" {
		data.TagPrefix = "sandbox-"
	}

	// Load template
	templatePath := scaffoldTemplate
	if templatePath == "" {
		templatePath = cfg.ScaffoldTemplatePath()
	}

	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return fmt.Errorf("loading template %s: %w", templatePath, err)
	}

	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	// Create output file
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer file.Close()

	// Execute template
	if err := tmpl.Execute(file, data); err != nil {
		return fmt.Errorf("executing template: %w", err)
	}

	fmt.Printf("Created %s\n", outputPath)
	return nil
}
