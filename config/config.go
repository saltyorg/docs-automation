package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
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
	DockerMetadata    DockerMetadataConfig         `yaml:"docker_metadata"`
	SectionExplainers map[string]string            `yaml:"section_explainers"`
	TypeInference     TypeInferenceConfig          `yaml:"type_inference"`
	DockerVariables   DockerVariables              `yaml:"docker_variables"`
	CLIHelp           CLIHelpConfig                `yaml:"cli_help"`
	Markers           MarkersConfig                `yaml:"markers"`
	Scaffold          ScaffoldConfig               `yaml:"scaffold"`
	Checks            ChecksConfig                 `yaml:"checks"`
	Issue             IssueConfig                  `yaml:"issue"`
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

// DockerMetadataConfig configures metadata derived from a role's primary image repository.
type DockerMetadataConfig struct {
	Icon        string                          `yaml:"icon"`
	ReleaseLink DockerMetadataReleaseLink       `yaml:"release_link"`
	Overrides   map[string]DockerMetadataTarget `yaml:"overrides"`
	Rules       []DockerMetadataRule            `yaml:"rules"`
	Ignore      []string                        `yaml:"ignore"`
}

// DockerMetadataReleaseLink configures the authored release-link label.
type DockerMetadataReleaseLink struct {
	Name string `yaml:"name"`
}

// DockerMetadataTarget is a resolved link destination and presentation type.
type DockerMetadataTarget struct {
	URL  string `yaml:"url"`
	Type string `yaml:"type"`
}

// DockerMetadataRule maps a full image repository match to a link destination.
type DockerMetadataRule struct {
	Pattern string `yaml:"pattern"`
	URL     string `yaml:"url"`
	Type    string `yaml:"type"`
}

// Enabled reports whether Docker metadata derivation is configured.
func (c DockerMetadataConfig) Enabled() bool {
	return c.Icon != "" || c.ReleaseLink.Name != "" || len(c.Overrides) > 0 || len(c.Rules) > 0 || len(c.Ignore) > 0
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
	Filters   map[string]string `yaml:"filters"`
	Symbols   map[string]string `yaml:"symbols"`
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

// CheckConfig configures one docs-health check category.
type CheckConfig struct {
	Enabled      *bool    `yaml:"enabled"`
	ExcludePaths []string `yaml:"exclude_paths"`
	Statuses     []string `yaml:"statuses"`
}

// ChecksConfig configures the docs-health check categories.
type ChecksConfig struct {
	Coverage    CheckConfig `yaml:"coverage"`
	Frontmatter CheckConfig `yaml:"frontmatter"`
	Editorial   CheckConfig `yaml:"editorial"`
}

// IssueConfig configures issue metadata for docs-health reporting.
type IssueConfig struct {
	SourceRepositories map[string]SourceRepositoryConfig `yaml:"source_repositories"`
}

// SourceRepositoryConfig identifies a source repository for issue links.
type SourceRepositoryConfig struct {
	Slug string `yaml:"slug"`
	Ref  string `yaml:"ref"`
}

// EnabledOr returns the configured enabled value or defaultValue when it is unset.
func (c CheckConfig) EnabledOr(defaultValue bool) bool {
	if c.Enabled == nil {
		return defaultValue
	}
	return *c.Enabled
}

// Excludes reports whether relPath is excluded by this check configuration.
func (c CheckConfig) Excludes(relPath string) bool {
	clean := filepath.ToSlash(filepath.Clean(relPath))
	return slices.Contains(c.ExcludePaths, clean)
}

type pathOverlay struct {
	Extends      string           `yaml:"extends"`
	Repositories RepositoryConfig `yaml:"repositories"`
}

// Load reads and parses a config file from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	hasExtends, err := hasTopLevelKey(data, "extends")
	if err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}
	if hasExtends {
		return loadPathOverlay(path, data)
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

func loadPathOverlay(path string, data []byte) (*Config, error) {
	var overlay pathOverlay
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&overlay); err != nil {
		return nil, fmt.Errorf("parsing config overlay: %w", err)
	}
	if strings.TrimSpace(overlay.Extends) == "" {
		return nil, fmt.Errorf("config overlay extends must not be empty")
	}
	if overlay.Repositories == (RepositoryConfig{}) {
		return nil, fmt.Errorf("config overlay must override at least one repository path")
	}

	basePath := overlay.Extends
	if !filepath.IsAbs(basePath) {
		basePath = filepath.Join(filepath.Dir(path), basePath)
	}
	basePath, err := filepath.Abs(filepath.Clean(basePath))
	if err != nil {
		return nil, fmt.Errorf("resolving base config path: %w", err)
	}
	overlayPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolving config overlay path: %w", err)
	}
	if basePath == overlayPath {
		return nil, fmt.Errorf("config overlay must not extend itself")
	}
	baseData, err := os.ReadFile(basePath)
	if err != nil {
		return nil, fmt.Errorf("reading base config %s: %w", basePath, err)
	}
	baseExtends, err := hasTopLevelKey(baseData, "extends")
	if err != nil {
		return nil, fmt.Errorf("parsing base config %s: %w", basePath, err)
	}
	if baseExtends {
		return nil, fmt.Errorf("base config %s contains extends; nested extends is not supported", basePath)
	}
	var cfg Config
	if err := yaml.Unmarshal(baseData, &cfg); err != nil {
		return nil, fmt.Errorf("parsing base config %s: %w", basePath, err)
	}
	if overlay.Repositories.Saltbox != "" {
		cfg.Repositories.Saltbox = overlay.Repositories.Saltbox
	}
	if overlay.Repositories.Sandbox != "" {
		cfg.Repositories.Sandbox = overlay.Repositories.Sandbox
	}
	if overlay.Repositories.Docs != "" {
		cfg.Repositories.Docs = overlay.Repositories.Docs
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}
	return &cfg, nil
}

func hasTopLevelKey(data []byte, key string) (bool, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return false, err
	}
	if len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return false, nil
	}
	mapping := document.Content[0]
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return true, nil
		}
	}
	return false, nil
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
	if err := validateDockerVariableTypes(c.DockerVariables); err != nil {
		return err
	}
	if err := validateDockerMetadata(c.DockerMetadata); err != nil {
		return err
	}
	for _, check := range []struct {
		name   string
		config *CheckConfig
	}{
		{name: "coverage", config: &c.Checks.Coverage},
		{name: "frontmatter", config: &c.Checks.Frontmatter},
		{name: "editorial", config: &c.Checks.Editorial},
	} {
		if err := validateCheckConfig(check.name, check.config); err != nil {
			return err
		}
	}
	if err := validateIssueConfig(c.Issue); err != nil {
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

func validateDockerMetadata(metadata DockerMetadataConfig) error {
	if !metadata.Enabled() {
		return nil
	}
	if strings.TrimSpace(metadata.Icon) == "" {
		return fmt.Errorf("docker_metadata.icon is required")
	}
	if strings.TrimSpace(metadata.ReleaseLink.Name) == "" {
		return fmt.Errorf("docker_metadata.release_link.name is required")
	}

	overrides := make(map[string]string, len(metadata.Overrides))
	for repository, target := range metadata.Overrides {
		normalized := NormalizeDockerRepository(repository)
		if normalized == "" {
			return fmt.Errorf("docker_metadata.overrides contains an empty repository")
		}
		if existing, ok := overrides[normalized]; ok {
			return fmt.Errorf("docker_metadata.overrides contains duplicate repositories %q and %q", existing, repository)
		}
		overrides[normalized] = repository
		if strings.TrimSpace(target.URL) == "" {
			return fmt.Errorf("docker_metadata.overrides[%q].url is required", repository)
		}
		if strings.TrimSpace(target.Type) == "" {
			return fmt.Errorf("docker_metadata.overrides[%q].type is required", repository)
		}
	}

	for i, rule := range metadata.Rules {
		if !strings.HasPrefix(rule.Pattern, "^") || !strings.HasSuffix(rule.Pattern, "$") {
			return fmt.Errorf("docker_metadata.rules[%d].pattern must be anchored with ^ and $", i)
		}
		compiled, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return fmt.Errorf("docker_metadata.rules[%d].pattern is invalid: %w", i, err)
		}
		if strings.TrimSpace(rule.URL) == "" {
			return fmt.Errorf("docker_metadata.rules[%d].url is required", i)
		}
		if strings.TrimSpace(rule.Type) == "" {
			return fmt.Errorf("docker_metadata.rules[%d].type is required", i)
		}
		if err := validateReplacementCaptures(rule.URL, compiled); err != nil {
			return fmt.Errorf("docker_metadata.rules[%d].url capture reference: %w", i, err)
		}
	}

	ignored := make(map[string]string, len(metadata.Ignore))
	for i, repository := range metadata.Ignore {
		normalized := NormalizeDockerRepository(repository)
		if normalized == "" {
			return fmt.Errorf("docker_metadata.ignore[%d] must not be empty", i)
		}
		if existing, ok := ignored[normalized]; ok {
			return fmt.Errorf("docker_metadata.ignore contains duplicate repositories %q and %q", existing, repository)
		}
		ignored[normalized] = repository
		if override, ok := overrides[normalized]; ok {
			return fmt.Errorf("docker repository %q is present in both overrides and ignore", override)
		}
	}
	return nil
}

func validateReplacementCaptures(replacement string, pattern *regexp.Regexp) error {
	for i := 0; i < len(replacement); i++ {
		if replacement[i] != '$' || i+1 >= len(replacement) {
			continue
		}
		start := i + 1
		end := start
		if replacement[start] == '{' {
			start++
			end = strings.IndexByte(replacement[start:], '}')
			if end < 0 {
				return fmt.Errorf("has an unterminated reference")
			}
			end += start
			i = end
		} else {
			for end < len(replacement) && (isASCIILetterOrDigit(replacement[end]) || replacement[end] == '_') {
				end++
			}
			if end == start {
				continue
			}
			i = end - 1
		}
		name := replacement[start:end]
		if name == "" {
			return fmt.Errorf("is empty")
		}
		if index, err := strconv.Atoi(name); err == nil {
			if index < 0 || index > pattern.NumSubexp() {
				return fmt.Errorf("$%s does not exist", name)
			}
			continue
		}
		if pattern.SubexpIndex(name) < 0 {
			return fmt.Errorf("$%s does not exist", name)
		}
	}
	return nil
}

// NormalizeDockerRepository returns the comparison form for exact repository entries.
func NormalizeDockerRepository(repository string) string {
	return strings.ToLower(strings.TrimSpace(repository))
}

func validateDockerVariableTypes(variables DockerVariables) error {
	owners := make(map[string]string)
	for _, group := range []struct {
		typ      string
		suffixes []string
	}{
		{typ: "bool", suffixes: variables.Bool},
		{typ: "int", suffixes: variables.Int},
		{typ: "list", suffixes: variables.List},
		{typ: "dict", suffixes: variables.Dict},
	} {
		for _, suffix := range group.suffixes {
			normalized := NormalizeDockerSuffix(suffix)
			if normalized == "" {
				return fmt.Errorf("docker_variables.%s contains an empty suffix", group.typ)
			}
			if owner, exists := owners[normalized]; exists {
				return fmt.Errorf("docker variable %q belongs to both %s and %s type groups", suffix, owner, group.typ)
			}
			owners[normalized] = group.typ
		}
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

func validateCheckConfig(name string, check *CheckConfig) error {
	if name != "editorial" && len(check.Statuses) > 0 {
		return fmt.Errorf("checks.%s.statuses is only supported for editorial checks", name)
	}
	if name == "editorial" && check.EnabledOr(false) && len(check.Statuses) == 0 {
		return fmt.Errorf("checks.editorial.statuses is required when editorial checks are enabled")
	}
	for i, status := range check.Statuses {
		if strings.TrimSpace(status) == "" {
			return fmt.Errorf("checks.%s.statuses[%d] must not be empty", name, i)
		}
	}

	exclusions := make(map[string]struct{}, len(check.ExcludePaths))
	for i, relPath := range check.ExcludePaths {
		trimmed := strings.TrimSpace(relPath)
		if trimmed == "" {
			return fmt.Errorf("checks.%s.exclude_paths[%d] must not be empty", name, i)
		}
		if filepath.IsAbs(trimmed) {
			return fmt.Errorf("checks.%s.exclude_paths[%d] must be relative", name, i)
		}

		clean := filepath.ToSlash(filepath.Clean(trimmed))
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("checks.%s.exclude_paths[%d] must not escape the docs root", name, i)
		}
		if _, exists := exclusions[clean]; exists {
			return fmt.Errorf("checks.%s.exclude_paths contains duplicate path %q", name, clean)
		}

		exclusions[clean] = struct{}{}
		check.ExcludePaths[i] = clean
	}

	return nil
}

func validateIssueConfig(issue IssueConfig) error {
	for source, repository := range issue.SourceRepositories {
		owner, name, found := strings.Cut(repository.Slug, "/")
		if !found || strings.Contains(name, "/") || !validGitHubOwner(owner) || !validGitHubRepository(name) {
			return fmt.Errorf("issue.source_repositories.%s.slug must use GitHub owner/repository syntax", source)
		}
		if strings.TrimSpace(repository.Ref) == "" {
			return fmt.Errorf("issue.source_repositories.%s.ref is required", source)
		}
	}

	return nil
}

func validGitHubOwner(owner string) bool {
	if len(owner) == 0 || len(owner) > 39 || owner[0] == '-' || owner[len(owner)-1] == '-' {
		return false
	}
	for i := range len(owner) {
		if isASCIILetterOrDigit(owner[i]) {
			continue
		}
		if owner[i] != '-' || (i > 0 && owner[i-1] == '-') {
			return false
		}
	}
	return true
}

func validGitHubRepository(name string) bool {
	if len(name) == 0 || len(name) > 100 {
		return false
	}
	for i := range len(name) {
		if isASCIILetterOrDigit(name[i]) || name[i] == '.' || name[i] == '-' || name[i] == '_' {
			continue
		}
		return false
	}
	return true
}

func isASCIILetterOrDigit(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9'
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
