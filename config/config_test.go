package config

import (
	"testing"

	"go.yaml.in/yaml/v3"
)

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
