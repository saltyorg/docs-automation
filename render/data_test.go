package render

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/parser"
)

func TestBuildRoleDataAppliesDockerOverrideMetadata(t *testing.T) {
	variable := parser.Variable{
		Name:     "plex_role_docker_gpu_enabled",
		RawValue: "true",
		Section:  "Docker",
		Comment:  "GPU",
	}
	role := &parser.RoleInfo{
		Name:         "plex",
		RepoType:     "saltbox",
		HasDocker:    true,
		SectionOrder: []string{"Docker"},
		Sections: map[string]*parser.Section{
			"Docker": {
				Name:      "Docker",
				Variables: []parser.Variable{variable},
			},
		},
		AllVariables: []parser.Variable{variable},
	}
	cfg := &config.Config{
		DockerOverrides: config.DockerOverrides{
			Variables: map[string]config.OverrideVarDef{
				"_docker_gpu_enabled": {
					Description: "Enable automatic GPU access",
					Type:        "bool",
				},
			},
		},
	}

	data := BuildRoleData(role, cfg, nil)
	got := data.Sections["Docker"].Variables[0]
	if got.Comment != "GPU" {
		t.Fatalf("comment = %q, want original role comment", got.Comment)
	}
	if !slices.Equal(got.CommentLines, []string{"GPU"}) {
		t.Fatalf("comment lines = %v, want original role comment", got.CommentLines)
	}
	if got.Description != "Enable automatic GPU access" {
		t.Fatalf("description = %q, want configured description", got.Description)
	}
	if got.Type != "bool" {
		t.Fatalf("type = %q, want bool", got.Type)
	}
	if got.RawValue != "true" {
		t.Fatalf("raw value = %q, want role default to remain unchanged", got.RawValue)
	}

	legacyData := BuildRoleData(role, &config.Config{}, nil)
	legacyVar := legacyData.Sections["Docker"].Variables[0]
	if legacyVar.Comment != "GPU" {
		t.Fatalf("legacy comment = %q, want original role comment", legacyVar.Comment)
	}
	if legacyVar.Description != "" {
		t.Fatalf("legacy description = %q, want empty description", legacyVar.Description)
	}
}

func TestBuildDockerInfoPreservesUnconfiguredVariablesAndAddsMetadata(t *testing.T) {
	root := t.TempDir()
	tasksDir := filepath.Join(root, "resources", "tasks", "docker")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatalf("creating Docker task directory: %v", err)
	}
	content := `
- name: Test Docker variables
  ansible.builtin.set_fact:
    gpu: "{{ lookup('docker_var', '_docker_gpu_enabled', default=false) }}"
    custom: "{{ lookup('docker_var', '_docker_custom_option', default='') }}"
`
	if err := os.WriteFile(filepath.Join(tasksDir, "create.yml"), []byte(content), 0o644); err != nil {
		t.Fatalf("writing Docker task fixture: %v", err)
	}
	defaultValue := "false"
	cfg := &config.Config{
		Repositories: config.RepositoryConfig{Saltbox: root},
		DockerOverrides: config.DockerOverrides{
			Variables: map[string]config.OverrideVarDef{
				"_docker_gpu_enabled": {
					Description: "Enable automatic GPU access",
					Default:     &defaultValue,
					Type:        "bool",
				},
			},
		},
	}

	info := buildDockerInfo(cfg, "example", nil)
	if info == nil {
		t.Fatal("DockerInfo is nil")
	}
	if got := info.Overrides["gpu_enabled"]; got == nil {
		t.Fatal("gpu_enabled metadata is missing")
	} else {
		if got.Description != "Enable automatic GPU access" {
			t.Fatalf("description = %q, want configured description", got.Description)
		}
		if !got.HasDefault || got.Default != "false" {
			t.Fatalf("default = %q, has default = %v; want false and true", got.Default, got.HasDefault)
		}
		if got.Type != "bool" {
			t.Fatalf("type = %q, want bool", got.Type)
		}
	}
	if _, exists := info.Overrides["custom_option"]; exists {
		t.Fatal("unconfigured custom option unexpectedly has metadata")
	}

	other := info.Categories["Other Options"]
	if !slices.Contains(other, "custom_option") || !slices.Contains(other, "gpu_enabled") {
		t.Fatalf("other options = %v, want existing and configured variables", other)
	}

	legacyInfo := buildDockerInfo(&config.Config{Repositories: config.RepositoryConfig{Saltbox: root}}, "example", nil)
	if legacyInfo == nil {
		t.Fatal("legacy DockerInfo is nil")
	}
	if len(legacyInfo.Overrides) != 0 {
		t.Fatalf("legacy overrides = %v, want no metadata", legacyInfo.Overrides)
	}
	if !slices.Equal(legacyInfo.Categories["Other Options"], other) {
		t.Fatalf("legacy categories = %v, want %v", legacyInfo.Categories["Other Options"], other)
	}
}

func TestFindDockerOverrideAcceptsFullAndNormalizedSuffixes(t *testing.T) {
	full := config.OverrideVarDef{Description: "full"}
	normalized := config.OverrideVarDef{Description: "normalized"}

	if got, ok := findDockerOverride(map[string]config.OverrideVarDef{"_docker_gpu_enabled": full}, "gpu_enabled"); !ok || got.Description != "full" {
		t.Fatalf("full suffix lookup = %#v, %v; want full metadata", got, ok)
	}
	if got, ok := findDockerOverride(map[string]config.OverrideVarDef{"gpu_enabled": normalized}, "gpu_enabled"); !ok || got.Description != "normalized" {
		t.Fatalf("normalized suffix lookup = %#v, %v; want normalized metadata", got, ok)
	}
}
