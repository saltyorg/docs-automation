package config

import (
	"testing"

	"go.yaml.in/yaml/v3"
)

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
