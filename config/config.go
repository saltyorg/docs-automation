package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Config represents the complete configuration for docs automation.
type Config struct {
	Repositories      RepositoryConfig             `yaml:"repositories"`
	Blacklist         BlacklistConfig              `yaml:"blacklist"`
	PathOverrides     map[string]map[string]string `yaml:"path_overrides"`
	GlobalOverrides   GlobalOverrides              `yaml:"global_overrides"`
	DockerOverrides   DockerOverrides              `yaml:"docker_overrides"`
	SectionExplainers map[string]string            `yaml:"section_explainers"`
	TypeInference     TypeInferenceConfig          `yaml:"type_inference"`
	DockerVariables   DockerVariables              `yaml:"docker_variables"`
	CLIHelp           CLIHelpConfig                `yaml:"cli_help"`
	Markers           MarkersConfig                `yaml:"markers"`
	Scaffold          ScaffoldConfig               `yaml:"scaffold"`
}

// RepositoryConfig defines paths to the repositories.
type RepositoryConfig struct {
	Saltbox string `yaml:"saltbox"`
	Sandbox string `yaml:"sandbox"`
	Docs    string `yaml:"docs"`
}

// BlacklistConfig defines roles/apps excluded from automation.
type BlacklistConfig struct {
	DocsCoverage RepoBlacklist `yaml:"docs_coverage"`
}

// RepoBlacklist defines blacklisted roles per repository.
type RepoBlacklist struct {
	Saltbox []string `yaml:"saltbox"`
	Sandbox []string `yaml:"sandbox"`
}

// GlobalOverrides configures role_var global override variables.
type GlobalOverrides struct {
	IgnoreSuffixes []string                  `yaml:"ignore_suffixes"`
	Variables      map[string]OverrideVarDef `yaml:"variables"`
}

// DockerOverrides configures Docker+ docs generation overrides.
type DockerOverrides struct {
	IgnoreSuffixes []string                  `yaml:"ignore_suffixes"`
	Groups         []DockerOverrideGroup     `yaml:"groups"`
	Variables      map[string]OverrideVarDef `yaml:"variables"`
}

// DockerOverrideGroup keeps related Docker override variables together in generated docs.
type DockerOverrideGroup struct {
	Name       string   `yaml:"name"`
	Primary    string   `yaml:"primary"`
	Companions []string `yaml:"companions"`
}

// OverrideVarDef defines reusable override metadata.
type OverrideVarDef struct {
	Description string  `yaml:"description"`
	Default     *string `yaml:"default"` // Pointer to distinguish null/missing from empty string
	Type        string  `yaml:"type"`
	Example     string  `yaml:"example,omitempty"`
}

// TypeInferenceConfig defines rules for inferring variable types.
type TypeInferenceConfig struct {
	Exact     map[string]string `yaml:"exact"`
	Patterns  []TypePattern     `yaml:"patterns"`
	Overrides map[string]string `yaml:"overrides"`
}

// TypePattern defines a pattern-based type inference rule.
type TypePattern struct {
	SuffixContains string `yaml:"suffix_contains"`
	Type           string `yaml:"type"`
}

// DockerVariables categorizes docker container module variables.
type DockerVariables struct {
	Bool []string `yaml:"bool"`
	Int  []string `yaml:"int"`
	List []string `yaml:"list"`
	Dict []string `yaml:"dict"`
}

// CLIHelpConfig configures CLI help generation.
type CLIHelpConfig struct {
	BinaryPath string `yaml:"binary_path"`
	DocsFile   string `yaml:"docs_file"`
}

// MarkersConfig defines managed section marker names.
type MarkersConfig struct {
	Variables string `yaml:"variables"`
	CLI       string `yaml:"cli"`
	Overview  string `yaml:"overview"`
}

// ScaffoldConfig configures documentation scaffolding.
type ScaffoldConfig struct {
	OutputPaths map[string]string `yaml:"output_paths"`
}

// Load reads and parses a config file from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

// Validate checks the configuration for required fields and consistency.
func (c *Config) Validate() error {
	// Check required string fields
	if c.Repositories.Saltbox == "" {
		return fmt.Errorf("repositories.saltbox is required")
	}
	if c.Repositories.Sandbox == "" {
		return fmt.Errorf("repositories.sandbox is required")
	}
	if c.Repositories.Docs == "" {
		return fmt.Errorf("repositories.docs is required")
	}
	if c.Markers.Variables == "" {
		return fmt.Errorf("markers.variables is required")
	}
	if err := validateDockerOverrideGroups(c.DockerOverrides.Groups); err != nil {
		return err
	}

	// Validate repository directories exist
	if err := validateDirectory(c.Repositories.Saltbox, "repositories.saltbox"); err != nil {
		return err
	}
	if err := validateDirectory(c.Repositories.Sandbox, "repositories.sandbox"); err != nil {
		return err
	}
	if err := validateDirectory(c.Repositories.Docs, "repositories.docs"); err != nil {
		return err
	}

	// Validate roles directories exist
	if err := validateDirectory(c.SaltboxRolesPath(), "saltbox roles directory"); err != nil {
		return err
	}
	if err := validateDirectory(c.SandboxRolesPath(), "sandbox roles directory"); err != nil {
		return err
	}

	return nil
}

func validateDockerOverrideGroups(groups []DockerOverrideGroup) error {
	groupNames := make(map[string]bool)
	members := make(map[string]string)

	for i, group := range groups {
		name := strings.TrimSpace(group.Name)
		if name == "" {
			return fmt.Errorf("docker_overrides.groups[%d].name is required", i)
		}
		nameKey := strings.ToLower(name)
		if groupNames[nameKey] {
			return fmt.Errorf("docker_overrides group name %q is duplicated", name)
		}
		groupNames[nameKey] = true

		primary := NormalizeDockerSuffix(group.Primary)
		if primary == "" {
			return fmt.Errorf("docker_overrides group %q primary is required", name)
		}

		groupMembers := make(map[string]bool)
		for memberIndex, member := range append([]string{group.Primary}, group.Companions...) {
			normalized := NormalizeDockerSuffix(member)
			if normalized == "" {
				return fmt.Errorf("docker_overrides group %q member %d is empty", name, memberIndex)
			}
			if groupMembers[normalized] {
				if normalized == primary {
					return fmt.Errorf("docker_overrides group %q repeats primary %q as a companion", name, group.Primary)
				}
				return fmt.Errorf("docker_overrides group %q repeats member %q", name, member)
			}
			groupMembers[normalized] = true

			if existingGroup, exists := members[normalized]; exists {
				return fmt.Errorf("docker override %q belongs to both %q and %q groups", member, existingGroup, name)
			}
			members[normalized] = name
		}
	}

	return nil
}

// NormalizeDockerSuffix converts supported Docker override suffix forms to Docker+ form.
func NormalizeDockerSuffix(suffix string) string {
	normalized := strings.TrimSpace(suffix)
	normalized = strings.TrimPrefix(normalized, "_docker_")
	normalized = strings.TrimPrefix(normalized, "_")
	return normalized
}

// validateDirectory checks that a path exists and is a directory.
func validateDirectory(path, name string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s does not exist: %s", name, path)
		}
		return fmt.Errorf("%s: %w", name, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory: %s", name, path)
	}
	return nil
}

// InventoryPath returns the full path to the inventory file.
func (c *Config) InventoryPath() string {
	return filepath.Join(c.Repositories.Saltbox, "inventories", "group_vars", "all.yml")
}

// SaltboxRolesPath returns the path to saltbox roles directory.
func (c *Config) SaltboxRolesPath() string {
	return filepath.Join(c.Repositories.Saltbox, "roles")
}

// SandboxRolesPath returns the path to sandbox roles directory.
func (c *Config) SandboxRolesPath() string {
	return filepath.Join(c.Repositories.Sandbox, "roles")
}

// SaltboxDocsPath returns the path to saltbox app docs.
func (c *Config) SaltboxDocsPath() string {
	return filepath.Join(c.Repositories.Docs, "docs", "apps")
}

// SandboxDocsPath returns the path to sandbox app docs.
func (c *Config) SandboxDocsPath() string {
	return filepath.Join(c.Repositories.Docs, "docs", "sandbox", "apps")
}

// InventoryTemplatePath returns the path to the inventory template.
func (c *Config) InventoryTemplatePath() string {
	return filepath.Join(c.Repositories.Docs, "templates", "inventory.md.tmpl")
}

// OverviewTemplatePath returns the path to the overview template.
func (c *Config) OverviewTemplatePath() string {
	return filepath.Join(c.Repositories.Docs, "templates", "overview.md.tmpl")
}

// CLIHelpTemplatePath returns the path to the CLI help template.
func (c *Config) CLIHelpTemplatePath() string {
	return filepath.Join(c.Repositories.Docs, "templates", "cli_help.md.tmpl")
}

// ScaffoldTemplatePath returns the path to the scaffold template.
func (c *Config) ScaffoldTemplatePath() string {
	return filepath.Join(c.Repositories.Docs, "templates", "app_scaffold.md.tmpl")
}
