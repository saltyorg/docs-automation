package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestLoadPathOverlayExtendsCanonicalConfig(t *testing.T) {
	root := t.TempDir()
	basePath := filepath.Join(root, "canonical", ".docs-automation.yml")
	overlayPath := filepath.Join(root, "local", "config.yml")
	saltboxPath := filepath.Join(root, "repos", "saltbox")
	sandboxPath := filepath.Join(root, "repos", "sandbox")
	docsPath := filepath.Join(root, "repos", "docs")
	for _, path := range []string{
		filepath.Join(saltboxPath, "roles"),
		filepath.Join(sandboxPath, "roles"),
		docsPath,
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("creating fixture directory: %v", err)
		}
	}
	writeConfigFixture(t, basePath, `
repositories:
  saltbox: saltbox
  sandbox: sandbox
  docs: docs
blacklist:
  docs_coverage:
    saltbox: [internal_role]
global_overrides:
  variables:
    _example_list:
      default: null
      type: list
docker_overrides:
  groups:
    - name: GPU
      primary: _docker_gpu_enabled
      companions:
        - _docker_nvidia_disabled
        - _docker_dev_dri_disabled
type_inference:
  filters:
    example_filter: dict
markers:
  variables: VARIABLES
`)
	writeConfigFixture(t, overlayPath, `
extends: ../canonical/.docs-automation.yml
repositories:
  saltbox: `+saltboxPath+`
  sandbox: `+sandboxPath+`
  docs: `+docsPath+`
`)
	t.Chdir(t.TempDir())

	cfg, err := Load(overlayPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Repositories != (RepositoryConfig{Saltbox: saltboxPath, Sandbox: sandboxPath, Docs: docsPath}) {
		t.Fatalf("repositories = %#v", cfg.Repositories)
	}
	if len(cfg.DockerOverrides.Groups) != 1 || cfg.DockerOverrides.Groups[0].Name != "GPU" {
		t.Fatalf("docker override groups = %#v, want inherited GPU group", cfg.DockerOverrides.Groups)
	}
	if len(cfg.Blacklist.DocsCoverage.Saltbox) != 1 || cfg.Blacklist.DocsCoverage.Saltbox[0] != "internal_role" {
		t.Fatalf("blacklist = %#v, want inherited list", cfg.Blacklist.DocsCoverage.Saltbox)
	}
	if got := cfg.GlobalOverrides.Variables["_example_list"]; got.Type != "list" || got.Default != nil {
		t.Fatalf("global override = %#v, want inherited null/list metadata", got)
	}
	if got := cfg.TypeInference.Filters["example_filter"]; got != "dict" {
		t.Fatalf("filter type = %q, want inherited dict", got)
	}
}

func TestLoadPathOverlayInheritsChecksAndIssueMetadata(t *testing.T) {
	root := t.TempDir()
	basePath := filepath.Join(root, "canonical", ".docs-automation.yml")
	overlayPath := filepath.Join(root, "local", "config.yml")
	saltboxPath := filepath.Join(root, "repos", "saltbox")
	sandboxPath := filepath.Join(root, "repos", "sandbox")
	docsPath := filepath.Join(root, "repos", "docs")
	for _, path := range []string{
		filepath.Join(saltboxPath, "roles"),
		filepath.Join(sandboxPath, "roles"),
		docsPath,
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("creating fixture directory: %v", err)
		}
	}
	writeConfigFixture(t, basePath, `
repositories:
  saltbox: /canonical/saltbox
  sandbox: /canonical/sandbox
  docs: /canonical/docs
markers:
  variables: VARIABLES
checks:
  coverage:
    enabled: true
    exclude_paths: [docs/apps/lean.md]
  editorial:
    enabled: true
    statuses: [draft2, outdated]
issue:
  source_repositories:
    saltbox:
      slug: saltyorg/Saltbox
      ref: master
`)
	writeConfigFixture(t, overlayPath, `
extends: ../canonical/.docs-automation.yml
repositories:
  saltbox: `+saltboxPath+`
  sandbox: `+sandboxPath+`
  docs: `+docsPath+`
`)

	cfg, err := Load(overlayPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := cfg.Repositories; got != (RepositoryConfig{Saltbox: saltboxPath, Sandbox: sandboxPath, Docs: docsPath}) {
		t.Fatalf("repositories = %#v, want overlay paths", got)
	}
	if !cfg.Checks.Coverage.EnabledOr(false) || !cfg.Checks.Coverage.Excludes("docs/apps/lean.md") {
		t.Fatalf("coverage checks = %#v, want inherited enabled exclusion", cfg.Checks.Coverage)
	}
	if !cfg.Checks.Editorial.EnabledOr(false) || len(cfg.Checks.Editorial.Statuses) != 2 {
		t.Fatalf("editorial checks = %#v, want inherited enabled statuses", cfg.Checks.Editorial)
	}
	if got := cfg.Issue.SourceRepositories["saltbox"]; got != (SourceRepositoryConfig{Slug: "saltyorg/Saltbox", Ref: "master"}) {
		t.Fatalf("saltbox source repository = %#v, want inherited metadata", got)
	}
}

func TestLoadPathOverlayRejectsEmptyExtends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	writeConfigFixture(t, path, "extends: \"\"\nrepositories:\n  docs: /tmp/docs\n")

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "extends must not be empty") {
		t.Fatalf("Load() error = %v, want empty extends error", err)
	}
}

func TestLoadPathOverlayRequiresRepositoryOverride(t *testing.T) {
	root := t.TempDir()
	basePath := filepath.Join(root, "base.yml")
	overlayPath := filepath.Join(root, "config.yml")
	writeConfigFixture(t, basePath, "repositories: {}\nmarkers:\n  variables: VARIABLES\n")
	writeConfigFixture(t, overlayPath, "extends: base.yml\n")

	_, err := Load(overlayPath)
	if err == nil || !strings.Contains(err.Error(), "at least one repository path") {
		t.Fatalf("Load() error = %v, want missing repository override error", err)
	}
}

func TestLoadPathOverlayRejectsSelfExtension(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.yml")
	writeConfigFixture(t, path, `
extends: config.yml
repositories:
  saltbox: /tmp/saltbox
`)

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "must not extend itself") {
		t.Fatalf("Load() error = %v, want self-extension error", err)
	}
}

func TestLoadPathOverlayRejectsNestedExtension(t *testing.T) {
	root := t.TempDir()
	basePath := filepath.Join(root, "base.yml")
	overlayPath := filepath.Join(root, "config.yml")
	saltboxPath := filepath.Join(root, "saltbox")
	sandboxPath := filepath.Join(root, "sandbox")
	docsPath := filepath.Join(root, "docs")
	for _, path := range []string{filepath.Join(saltboxPath, "roles"), filepath.Join(sandboxPath, "roles"), docsPath} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeConfigFixture(t, basePath, "extends: deeper.yml\nmarkers:\n  variables: VARIABLES\n")
	writeConfigFixture(t, overlayPath, "extends: base.yml\nrepositories:\n  saltbox: "+saltboxPath+"\n  sandbox: "+sandboxPath+"\n  docs: "+docsPath+"\n")

	_, err := Load(overlayPath)
	if err == nil || !strings.Contains(err.Error(), "nested extends is not supported") {
		t.Fatalf("Load() error = %v, want nested-extension error", err)
	}
}

func TestLoadPathOverlayRejectsBehavioralKeys(t *testing.T) {
	root := t.TempDir()
	basePath := filepath.Join(root, "base.yml")
	overlayPath := filepath.Join(root, "config.yml")
	writeConfigFixture(t, basePath, "markers:\n  variables: VARIABLES\n")
	writeConfigFixture(t, overlayPath, `
extends: base.yml
repositories:
  docs: /tmp/docs
docker_overrides:
  ignore_suffixes: []
`)

	_, err := Load(overlayPath)
	if err == nil || !strings.Contains(err.Error(), "field docker_overrides not found") {
		t.Fatalf("Load() error = %v, want behavioral-key error", err)
	}
}

func TestLoadPathOverlayRejectsUnknownRepositoryKey(t *testing.T) {
	root := t.TempDir()
	basePath := filepath.Join(root, "base.yml")
	overlayPath := filepath.Join(root, "config.yml")
	writeConfigFixture(t, basePath, "markers:\n  variables: VARIABLES\n")
	writeConfigFixture(t, overlayPath, "extends: base.yml\nrepositories:\n  docs: /tmp/docs\n  unknown: /tmp/unknown\n")

	_, err := Load(overlayPath)
	if err == nil || !strings.Contains(err.Error(), "field unknown not found") {
		t.Fatalf("Load() error = %v, want unknown repository-key error", err)
	}
}

func TestLoadPathOverlayReportsMissingBase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	writeConfigFixture(t, path, "extends: missing.yml\nrepositories:\n  docs: /tmp/docs\n")

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "reading base config") || !strings.Contains(err.Error(), "missing.yml") {
		t.Fatalf("Load() error = %v, want contextual missing-base error", err)
	}
}

func TestLoadPathOverlayMergesPartialRepositoryOverride(t *testing.T) {
	root := t.TempDir()
	basePath := filepath.Join(root, "base.yml")
	overlayPath := filepath.Join(root, "config.yml")
	saltboxPath := filepath.Join(root, "local-saltbox")
	sandboxPath := filepath.Join(root, "base-sandbox")
	docsPath := filepath.Join(root, "base-docs")
	for _, path := range []string{filepath.Join(saltboxPath, "roles"), filepath.Join(sandboxPath, "roles"), docsPath} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeConfigFixture(t, basePath, "repositories:\n  saltbox: /missing/saltbox\n  sandbox: "+sandboxPath+"\n  docs: "+docsPath+"\nmarkers:\n  variables: VARIABLES\n")
	writeConfigFixture(t, overlayPath, "extends: base.yml\nrepositories:\n  saltbox: "+saltboxPath+"\n")

	cfg, err := Load(overlayPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Repositories != (RepositoryConfig{Saltbox: saltboxPath, Sandbox: sandboxPath, Docs: docsPath}) {
		t.Fatalf("repositories = %#v", cfg.Repositories)
	}
}

func TestLoadStandaloneConfigRemainsSupported(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.yml")
	saltboxPath := filepath.Join(root, "saltbox")
	sandboxPath := filepath.Join(root, "sandbox")
	docsPath := filepath.Join(root, "docs")
	for _, directory := range []string{filepath.Join(saltboxPath, "roles"), filepath.Join(sandboxPath, "roles"), docsPath} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeConfigFixture(t, path, "repositories:\n  saltbox: "+saltboxPath+"\n  sandbox: "+sandboxPath+"\n  docs: "+docsPath+"\nmarkers:\n  variables: VARIABLES\n")

	if _, err := Load(path); err != nil {
		t.Fatalf("Load() standalone config error = %v", err)
	}
}

func TestLoadChecks(t *testing.T) {
	root := t.TempDir()
	saltboxPath := filepath.Join(root, "saltbox")
	sandboxPath := filepath.Join(root, "sandbox")
	docsPath := filepath.Join(root, "docs")
	for _, directory := range []string{filepath.Join(saltboxPath, "roles"), filepath.Join(sandboxPath, "roles"), docsPath} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	path := filepath.Join(root, "config.yml")
	writeConfigFixture(t, path, `
repositories:
  saltbox: `+saltboxPath+`
  sandbox: `+sandboxPath+`
  docs: `+docsPath+`
markers:
  variables: VARIABLES
checks:
  coverage:
    enabled: true
    exclude_paths:
      - docs/apps/../apps/lean.md
  frontmatter:
    enabled: false
  editorial:
    enabled: true
    statuses: [draft2, outdated]
issue:
  source_repositories:
    saltbox:
      slug: saltyorg/Saltbox
      ref: master
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.Checks.Coverage.EnabledOr(false) {
		t.Fatal("coverage enabled = false, want true")
	}
	if cfg.Checks.Frontmatter.EnabledOr(true) {
		t.Fatal("frontmatter enabled = true, want false")
	}
	if !cfg.Checks.Editorial.EnabledOr(false) {
		t.Fatal("editorial enabled = false, want true")
	}
	if got := (CheckConfig{}).EnabledOr(true); !got {
		t.Fatalf("unset enabled = %t, want default true", got)
	}
	if got := cfg.Checks.Coverage.ExcludePaths; len(got) != 1 || got[0] != "docs/apps/lean.md" {
		t.Fatalf("coverage exclusions = %#v, want normalized docs/apps/lean.md", got)
	}
	if !cfg.Checks.Coverage.Excludes("docs/apps/lean.md") {
		t.Fatal("coverage exclusion did not match normalized path")
	}
	if got := cfg.Checks.Editorial.Statuses; len(got) != 2 || got[0] != "draft2" || got[1] != "outdated" {
		t.Fatalf("editorial statuses = %#v, want draft2 and outdated", got)
	}
	if got := cfg.Issue.SourceRepositories["saltbox"]; got != (SourceRepositoryConfig{Slug: "saltyorg/Saltbox", Ref: "master"}) {
		t.Fatalf("saltbox source repository = %#v", got)
	}
}

func TestValidateCheckConfig(t *testing.T) {
	enabled := true
	tests := []struct {
		name      string
		checkName string
		check     CheckConfig
		wantErr   bool
	}{
		{name: "valid coverage exclusion", checkName: "coverage", check: CheckConfig{ExcludePaths: []string{"docs/apps/lean.md"}}},
		{name: "absolute exclusion", checkName: "coverage", check: CheckConfig{ExcludePaths: []string{"/docs/apps/lean.md"}}, wantErr: true},
		{name: "parent escape exclusion", checkName: "coverage", check: CheckConfig{ExcludePaths: []string{"../apps/lean.md"}}, wantErr: true},
		{name: "normalized duplicate exclusion", checkName: "coverage", check: CheckConfig{ExcludePaths: []string{"docs/apps/lean.md", "docs/apps/../apps/lean.md"}}, wantErr: true},
		{name: "statuses on coverage", checkName: "coverage", check: CheckConfig{Statuses: []string{"draft2"}}, wantErr: true},
		{name: "enabled editorial without statuses", checkName: "editorial", check: CheckConfig{Enabled: &enabled}, wantErr: true},
		{name: "empty editorial status", checkName: "editorial", check: CheckConfig{Statuses: []string{""}}, wantErr: true},
		{name: "whitespace editorial status", checkName: "editorial", check: CheckConfig{Statuses: []string{" \t "}}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCheckConfig(tt.checkName, &tt.check)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateCheckConfig() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}

func TestValidateIssueConfig(t *testing.T) {
	tests := []struct {
		name    string
		issue   IssueConfig
		wantErr bool
	}{
		{
			name: "valid source repository",
			issue: IssueConfig{SourceRepositories: map[string]SourceRepositoryConfig{
				"saltbox": {Slug: "saltyorg/Saltbox", Ref: "master"},
			}},
		},
		{
			name: "invalid owner repository slug",
			issue: IssueConfig{SourceRepositories: map[string]SourceRepositoryConfig{
				"saltbox": {Slug: "saltyorg", Ref: "master"},
			}},
			wantErr: true,
		},
		{
			name: "owner with internal whitespace",
			issue: IssueConfig{SourceRepositories: map[string]SourceRepositoryConfig{
				"saltbox": {Slug: "salty org/Saltbox", Ref: "master"},
			}},
			wantErr: true,
		},
		{
			name: "repository with unsupported character",
			issue: IssueConfig{SourceRepositories: map[string]SourceRepositoryConfig{
				"saltbox": {Slug: "saltyorg/Saltbox!", Ref: "master"},
			}},
			wantErr: true,
		},
		{
			name: "valid GitHub owner and repository characters",
			issue: IssueConfig{SourceRepositories: map[string]SourceRepositoryConfig{
				"saltbox": {Slug: "salty-org/Salt_box.1", Ref: "master"},
			}},
		},
		{
			name: "missing source repository ref",
			issue: IssueConfig{SourceRepositories: map[string]SourceRepositoryConfig{
				"saltbox": {Slug: "saltyorg/Saltbox"},
			}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateIssueConfig(tt.issue)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateIssueConfig() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}

func writeConfigFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating config fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing config fixture: %v", err)
	}
}

func TestSectionExplainersParse(t *testing.T) {
	const input = `
section_explainers:
  Ports: |-
    Ports are assigned automatically.
    Explicit conflicts always fail.
`

	var cfg Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("unmarshalling section explainers: %v", err)
	}

	want := "Ports are assigned automatically.\nExplicit conflicts always fail."
	if got := cfg.SectionExplainers["Ports"]; got != want {
		t.Fatalf("Ports explainer = %q, want %q", got, want)
	}
}

func TestTypeInferenceParsesFiltersAndSymbols(t *testing.T) {
	const input = `
type_inference:
  filters:
    traefik_certificate_domains: list
  symbols:
    traefik_http: bool
`

	var cfg Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("unmarshalling type inference config: %v", err)
	}
	if got := cfg.TypeInference.Filters["traefik_certificate_domains"]; got != "list" {
		t.Fatalf("filter type = %q, want list", got)
	}
	if got := cfg.TypeInference.Symbols["traefik_http"]; got != "bool" {
		t.Fatalf("symbol type = %q, want bool", got)
	}
}

func TestDockerOverridesRemainBackwardCompatible(t *testing.T) {
	const input = `
docker_overrides:
  ignore_suffixes:
    - _docker_dev_dri
`

	var cfg Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("unmarshalling legacy docker overrides: %v", err)
	}

	if len(cfg.DockerOverrides.IgnoreSuffixes) != 1 {
		t.Fatalf("ignore suffixes = %v, want one entry", cfg.DockerOverrides.IgnoreSuffixes)
	}
	if len(cfg.DockerOverrides.Variables) != 0 {
		t.Fatalf("variables = %v, want no entries", cfg.DockerOverrides.Variables)
	}
	if len(cfg.DockerOverrides.Groups) != 0 {
		t.Fatalf("groups = %v, want no entries", cfg.DockerOverrides.Groups)
	}
}

func TestDockerOverridesParseVariableMetadata(t *testing.T) {
	const input = `
docker_overrides:
  variables:
    _docker_gpu_enabled:
      description: Enable GPU access
      default: "false"
      type: bool
      example: |
        example_role_docker_gpu_enabled: true
`

	var cfg Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("unmarshalling docker override metadata: %v", err)
	}

	got, exists := cfg.DockerOverrides.Variables["_docker_gpu_enabled"]
	if !exists {
		t.Fatal("_docker_gpu_enabled metadata was not parsed")
	}
	if got.Description != "Enable GPU access" {
		t.Fatalf("description = %q, want %q", got.Description, "Enable GPU access")
	}
	if got.Default == nil || *got.Default != "false" {
		t.Fatalf("default = %v, want false", got.Default)
	}
	if got.Type != "bool" {
		t.Fatalf("type = %q, want bool", got.Type)
	}
	if got.Example != "example_role_docker_gpu_enabled: true\n" {
		t.Fatalf("example = %q, want rendered block content", got.Example)
	}
}

func TestDockerOverridesParseGroups(t *testing.T) {
	const input = `
docker_overrides:
  groups:
    - name: GPU
      primary: _docker_gpu_enabled
      companions:
        - _docker_nvidia_disabled
        - _docker_dev_dri_disabled
`

	var cfg Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("unmarshalling docker override groups: %v", err)
	}

	if len(cfg.DockerOverrides.Groups) != 1 {
		t.Fatalf("groups = %v, want one entry", cfg.DockerOverrides.Groups)
	}
	group := cfg.DockerOverrides.Groups[0]
	if group.Name != "GPU" || group.Primary != "_docker_gpu_enabled" {
		t.Fatalf("group = %#v, want GPU primary", group)
	}
	if len(group.Companions) != 2 {
		t.Fatalf("companions = %v, want two entries", group.Companions)
	}
}

func TestValidateDockerOverrideGroups(t *testing.T) {
	tests := []struct {
		name    string
		groups  []DockerOverrideGroup
		wantErr bool
	}{
		{
			name: "valid normalized forms",
			groups: []DockerOverrideGroup{{
				Name:       "GPU",
				Primary:    "_docker_gpu_enabled",
				Companions: []string{"nvidia_disabled", "_dev_dri_disabled"},
			}},
		},
		{name: "missing name", groups: []DockerOverrideGroup{{Primary: "gpu_enabled"}}, wantErr: true},
		{name: "missing primary", groups: []DockerOverrideGroup{{Name: "GPU"}}, wantErr: true},
		{
			name: "primary repeated as companion",
			groups: []DockerOverrideGroup{{
				Name:       "GPU",
				Primary:    "_docker_gpu_enabled",
				Companions: []string{"gpu_enabled"},
			}},
			wantErr: true,
		},
		{
			name: "member repeated across groups",
			groups: []DockerOverrideGroup{
				{Name: "GPU", Primary: "_docker_gpu_enabled", Companions: []string{"_docker_nvidia_disabled"}},
				{Name: "Accelerator", Primary: "nvidia_disabled"},
			},
			wantErr: true,
		},
		{
			name: "duplicate group name",
			groups: []DockerOverrideGroup{
				{Name: "GPU", Primary: "gpu_enabled"},
				{Name: "gpu", Primary: "other_gpu"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDockerOverrideGroups(tt.groups)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateDockerOverrideGroups() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateDockerVariableTypesRejectsDuplicateSuffix(t *testing.T) {
	variables := DockerVariables{
		List: []string{"sysctls"},
		Dict: []string{"_docker_sysctls"},
	}

	if err := validateDockerVariableTypes(variables); err == nil {
		t.Fatal("validateDockerVariableTypes() error is nil, want duplicate suffix error")
	}
}

func TestDockerMetadataParsesTypedResolutionConfig(t *testing.T) {
	const input = `
docker_metadata:
  icon: material/docker
  release_link:
    name: Image tags
  overrides:
    ghcr.io/imagegenius/immich:
      url: https://github.com/imagegenius/docker-immich/pkgs/container/immich
      type: github
  rules:
    - pattern: ^ghcr.io/([^/]+)/([^/]+)$
      url: https://github.com/$1/$2/pkgs/container/$2
      type: github
  ignore:
    - docker.elastic.co/elasticsearch/elasticsearch
`

	var cfg Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("unmarshalling docker metadata: %v", err)
	}
	if cfg.DockerMetadata.Icon != "material/docker" || cfg.DockerMetadata.ReleaseLink.Name != "Image tags" {
		t.Fatalf("DockerMetadata = %#v", cfg.DockerMetadata)
	}
	override := cfg.DockerMetadata.Overrides["ghcr.io/imagegenius/immich"]
	if override.URL == "" || override.Type != "github" {
		t.Fatalf("override = %#v", override)
	}
	if len(cfg.DockerMetadata.Rules) != 1 || cfg.DockerMetadata.Rules[0].Pattern == "" || cfg.DockerMetadata.Rules[0].Type != "github" {
		t.Fatalf("rules = %#v", cfg.DockerMetadata.Rules)
	}
	if len(cfg.DockerMetadata.Ignore) != 1 {
		t.Fatalf("ignore = %#v", cfg.DockerMetadata.Ignore)
	}
}

func TestValidateDockerMetadataAcceptsEmptyOrCompleteConfig(t *testing.T) {
	if err := validateDockerMetadata(DockerMetadataConfig{}); err != nil {
		t.Fatalf("empty docker metadata error = %v", err)
	}
	valid := DockerMetadataConfig{
		Icon:        "material/docker",
		ReleaseLink: DockerMetadataReleaseLink{Name: "Image tags"},
		Overrides: map[string]DockerMetadataTarget{
			"ghcr.io/imagegenius/immich": {URL: "https://example.invalid/immich", Type: "github"},
		},
		Rules: []DockerMetadataRule{{
			Pattern: `^ghcr\.io/([^/]+)/([^/]+)$`,
			URL:     "https://github.com/$1/$2/pkgs/container/$2",
			Type:    "github",
		}},
		Ignore: []string{"docker.elastic.co/elasticsearch/elasticsearch"},
	}
	if err := validateDockerMetadata(valid); err != nil {
		t.Fatalf("complete docker metadata error = %v", err)
	}
}

func TestValidateDockerMetadataRejectsInvalidValues(t *testing.T) {
	base := DockerMetadataConfig{
		Icon:        "material/docker",
		ReleaseLink: DockerMetadataReleaseLink{Name: "Image tags"},
	}
	tests := []struct {
		name   string
		mutate func(*DockerMetadataConfig)
		want   string
	}{
		{name: "missing icon", mutate: func(c *DockerMetadataConfig) { c.Icon = " " }, want: "icon"},
		{name: "missing release name", mutate: func(c *DockerMetadataConfig) { c.ReleaseLink.Name = " " }, want: "release_link.name"},
		{name: "blank override repository", mutate: func(c *DockerMetadataConfig) {
			c.Overrides = map[string]DockerMetadataTarget{" ": {URL: "https://example.invalid", Type: "docker"}}
		}, want: "overrides"},
		{name: "blank override URL", mutate: func(c *DockerMetadataConfig) { c.Overrides = map[string]DockerMetadataTarget{"repo": {Type: "docker"}} }, want: "url"},
		{name: "blank override type", mutate: func(c *DockerMetadataConfig) {
			c.Overrides = map[string]DockerMetadataTarget{"repo": {URL: "https://example.invalid"}}
		}, want: "type"},
		{name: "unanchored rule", mutate: func(c *DockerMetadataConfig) {
			c.Rules = []DockerMetadataRule{{Pattern: "repo/(.+)", URL: "https://example.invalid/$1", Type: "docker"}}
		}, want: "anchored"},
		{name: "invalid regexp", mutate: func(c *DockerMetadataConfig) {
			c.Rules = []DockerMetadataRule{{Pattern: "^[$", URL: "https://example.invalid", Type: "docker"}}
		}, want: "pattern"},
		{name: "blank rule URL", mutate: func(c *DockerMetadataConfig) { c.Rules = []DockerMetadataRule{{Pattern: "^repo$", Type: "docker"}} }, want: "url"},
		{name: "blank rule type", mutate: func(c *DockerMetadataConfig) {
			c.Rules = []DockerMetadataRule{{Pattern: "^repo$", URL: "https://example.invalid"}}
		}, want: "type"},
		{name: "unknown numeric capture", mutate: func(c *DockerMetadataConfig) {
			c.Rules = []DockerMetadataRule{{Pattern: "^repo/(.+)$", URL: "https://example.invalid/$2", Type: "docker"}}
		}, want: "capture"},
		{name: "unknown named capture", mutate: func(c *DockerMetadataConfig) {
			c.Rules = []DockerMetadataRule{{Pattern: "^repo/(?P<name>.+)$", URL: "https://example.invalid/${missing}", Type: "docker"}}
		}, want: "capture"},
		{name: "blank ignore", mutate: func(c *DockerMetadataConfig) { c.Ignore = []string{" "} }, want: "ignore"},
		{name: "normalized duplicate override", mutate: func(c *DockerMetadataConfig) {
			c.Overrides = map[string]DockerMetadataTarget{"Repo/Image": {URL: "https://one.invalid", Type: "docker"}, " repo/image ": {URL: "https://two.invalid", Type: "docker"}}
		}, want: "duplicate"},
		{name: "normalized duplicate ignore", mutate: func(c *DockerMetadataConfig) { c.Ignore = []string{"Repo/Image", " repo/image "} }, want: "duplicate"},
		{name: "override ignore conflict", mutate: func(c *DockerMetadataConfig) {
			c.Overrides = map[string]DockerMetadataTarget{"Repo/Image": {URL: "https://example.invalid", Type: "docker"}}
			c.Ignore = []string{" repo/image "}
		}, want: "both"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			tt.mutate(&cfg)
			err := validateDockerMetadata(cfg)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.want)) {
				t.Fatalf("validateDockerMetadata() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}
