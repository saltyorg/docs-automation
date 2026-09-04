package automation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/document"
	"go.yaml.in/yaml/v3"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// ScaffoldOptions controls scaffold input and overwrite behavior.
type ScaffoldOptions struct {
	TemplatePath string
	OutputPath   string
	Force        bool
}

// ScaffoldData contains data for the scaffold template.
type ScaffoldData struct {
	RoleName  string // e.g., "sonarr"
	RoleTitle string // e.g., "Sonarr" (title case)
	RoleTag   string // e.g., "sonarr" (for install command)
	RepoType  string // "saltbox" or "sandbox"
	TagPrefix string // "" for saltbox, "sandbox-" for sandbox
	IsDocker  bool
	Icon      string
	AppLinks  []document.AppLink
}

var sandboxInstallTagPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type sandboxPlay struct {
	Roles []sandboxRoleRegistration `yaml:"roles"`
}

type sandboxRoleRegistration struct {
	Role string   `yaml:"role"`
	Tags []string `yaml:"tags"`
}

// Scaffold creates a new documentation file for a role.
func (r *Runner) Scaffold(ctx context.Context, cfg *config.Config, roleName string, opts ScaffoldOptions) (err error) {
	defer func() { err = r.result(err) }()
	if err := ctx.Err(); err != nil {
		return err
	}
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
	outputPath := opts.OutputPath
	if outputPath == "" {
		pathPattern, ok := cfg.Scaffold.OutputPaths[repoType]
		if !ok {
			return fmt.Errorf("no output path configured for repo type %q", repoType)
		}
		outputPath = filepath.Join(cfg.Repositories.Docs, strings.ReplaceAll(pathPattern, "{role}", roleName))
	}

	// Check if file already exists
	if _, err := os.Stat(outputPath); err == nil && !opts.Force {
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
		roleTag, err := sandboxInstallTag(filepath.Join(cfg.Repositories.Sandbox, "sandbox.yml"), roleName)
		if err != nil {
			return fmt.Errorf("resolving Sandbox install tag: %w", err)
		}
		data.RoleTag = roleTag
	}
	defaultsPath := filepath.Join(rolesPath, roleName, "defaults", "main.yml")
	if _, err := os.Stat(defaultsPath); err == nil {
		roleInfo, err := r.parseRole(roleName, repoType, defaultsPath)
		if err != nil {
			return fmt.Errorf("parsing role: %w", err)
		}
		_, data.IsDocker = dockerImageVariable(roleInfo)
		if data.IsDocker && cfg.DockerMetadata.Enabled() {
			data.Icon = cfg.DockerMetadata.Icon
		}
		data.AppLinks = scaffoldAppLinks(data.IsDocker, dockerRepository(roleInfo), cfg.DockerMetadata)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking role defaults: %w", err)
	} else {
		data.AppLinks = scaffoldAppLinks(false, "", cfg.DockerMetadata)
	}

	// Load template
	templatePath := opts.TemplatePath
	if templatePath == "" {
		templatePath = cfg.ScaffoldTemplatePath()
	}

	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return fmt.Errorf("loading template %s: %w", templatePath, err)
	}

	var output bytes.Buffer
	if err := tmpl.Execute(&output, data); err != nil {
		return fmt.Errorf("executing template: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	if err := document.WriteFileAtomic(outputPath, output.Bytes(), 0o644, opts.Force); err != nil {
		if !opts.Force && errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("file %s already exists (use --force to overwrite)", outputPath)
		}
		return fmt.Errorf("writing output file: %w", err)
	}

	r.printf("Created %s\n", outputPath)
	return nil
}

func sandboxInstallTag(playbookPath, roleName string) (string, error) {
	content, err := os.ReadFile(playbookPath)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", playbookPath, err)
	}

	var plays []sandboxPlay
	if err := yaml.Unmarshal(content, &plays); err != nil {
		return "", fmt.Errorf("parsing %s: %w", playbookPath, err)
	}

	registrations := make([]sandboxRoleRegistration, 0, 1)
	for _, play := range plays {
		for _, registration := range play.Roles {
			if registration.Role == roleName {
				registrations = append(registrations, registration)
			}
		}
	}

	if len(registrations) == 0 {
		return "", fmt.Errorf("sandbox role %q is not registered in sandbox.yml", roleName)
	}
	if len(registrations) > 1 {
		return "", fmt.Errorf("sandbox role %q is registered more than once in sandbox.yml", roleName)
	}

	for _, tag := range registrations[0].Tags {
		if tag == "always" || tag == "never" {
			continue
		}
		if !sandboxInstallTagPattern.MatchString(tag) {
			return "", fmt.Errorf("sandbox role %q tag %q must use lowercase kebab-case", roleName, tag)
		}
		return tag, nil
	}

	return "", fmt.Errorf("sandbox role %q does not declare an installable tag", roleName)
}

func scaffoldAppLinks(isDocker bool, repository string, metadata config.DockerMetadataConfig) []document.AppLink {
	release := document.AppLink{
		Name:    "Releases",
		Type:    "releases",
		Purpose: document.AppLinkPurposeRelease,
	}
	if isDocker && metadata.Enabled() {
		release.Name = metadata.ReleaseLink.Name
		if target := resolveDockerRepository(repository, metadata); target.URL != "" {
			release.URL = target.URL
			release.Type = target.Type
		}
	}
	return []document.AppLink{
		{Name: "Manual", Type: "documentation", Purpose: document.AppLinkPurposeManual},
		release,
		{Name: "Community", Type: "community", Purpose: document.AppLinkPurposeCommunity},
	}
}
