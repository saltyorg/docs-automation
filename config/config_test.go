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
