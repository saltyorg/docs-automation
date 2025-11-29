package parser

import (
	"os"
	"testing"
)

func TestParseFile(t *testing.T) {
	// Skip if test file doesn't exist
	testFile := "/srv/git/saltbox/roles/plex/defaults/main.yml"
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("Test file not found:", testFile)
	}

	p := New("plex", "saltbox")
	role, err := p.ParseFile(testFile)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	// Check basic role info
	if role.Name != "plex" {
		t.Errorf("Expected role name 'plex', got '%s'", role.Name)
	}

	if role.RepoType != "saltbox" {
		t.Errorf("Expected repo type 'saltbox', got '%s'", role.RepoType)
	}

	// Check that we found sections
	if len(role.SectionOrder) == 0 {
		t.Error("Expected to find sections, got none")
	}

	t.Logf("Found %d sections: %v", len(role.SectionOrder), role.SectionOrder)

	// Check that we found variables
	if len(role.AllVariables) == 0 {
		t.Error("Expected to find variables, got none")
	}

	t.Logf("Found %d variables total", len(role.AllVariables))

	// Check instances detection
	if !role.HasInstances {
		t.Error("Expected plex to have instances")
	}

	if role.InstancesVar != "plex_instances" {
		t.Errorf("Expected instances var 'plex_instances', got '%s'", role.InstancesVar)
	}

	// Check _default/_custom detection
	if !role.HasDefaultVars {
		t.Error("Expected plex to have default/custom vars")
	}

	// Check SSO detection - plex should have SSO disabled (empty string)
	if role.SSOEnabled {
		t.Error("Expected plex to have SSO disabled by default")
	}

	// Check section-based feature flags
	if !role.HasDocker {
		t.Error("Expected plex to have HasDocker=true")
	}
	if !role.HasTraefik {
		t.Error("Expected plex to have HasTraefik=true")
	}
	if !role.HasWeb {
		t.Error("Expected plex to have HasWeb=true")
	}
	// Plex should have ThemePark variables
	if !role.HasThemePark {
		t.Error("Expected plex to have HasThemePark=true")
	}

	// Check that _paths_folders_list was filtered
	for _, v := range role.AllVariables {
		if v.Name == "plex_role_paths_folders_list" {
			t.Error("Expected plex_role_paths_folders_list to be filtered out")
		}
	}

	// Log some variables for inspection
	for _, section := range role.SectionOrder[:min(3, len(role.SectionOrder))] {
		s := role.Sections[section]
		t.Logf("\nSection: %s", section)
		for _, v := range s.Variables[:min(3, len(s.Variables))] {
			t.Logf("  - %s (multiline: %v, comment: %q)", v.Name, v.IsMultiline, truncate(v.Comment, 50))
		}
	}
}

func TestFilterVariables(t *testing.T) {
	vars := []Variable{
		{Name: "role_docker_envs_default"},
		{Name: "role_docker_envs_custom"},
		{Name: "role_docker_envs"},
		{Name: "role_other_var"},
	}

	filtered := FilterVariables(vars, "role")

	// Should have 3 vars (role_docker_envs filtered out)
	if len(filtered) != 3 {
		t.Errorf("Expected 3 filtered vars, got %d", len(filtered))
	}

	// Check that role_docker_envs was filtered
	for _, v := range filtered {
		if v.Name == "role_docker_envs" {
			t.Error("role_docker_envs should have been filtered out")
		}
	}
}

func TestGenerateInstanceName(t *testing.T) {
	tests := []struct {
		varName      string
		roleName     string
		instanceName string
		expected     string
	}{
		{"plex_role_web_subdomain", "plex", "plex2", "plex2_web_subdomain"},
		{"plex_role_docker_envs", "plex", "plex2", "plex2_docker_envs"},
		{"plex_instances", "plex", "plex2", "plex_instances"}, // Should not change
		{"plex_name", "plex", "plex2", "plex2_name"},
	}

	for _, tt := range tests {
		result := GenerateInstanceName(tt.varName, tt.roleName, tt.instanceName)
		if result != tt.expected {
			t.Errorf("GenerateInstanceName(%q, %q, %q) = %q, want %q",
				tt.varName, tt.roleName, tt.instanceName, result, tt.expected)
		}
	}
}

func TestAdjustMultilineIndent(t *testing.T) {
	lines := []string{
		`plex_role_docker_envs: "{{ lookup('role_var', '_docker_envs_default', role='plex')`,
		`                           | combine(lookup('role_var', '_docker_envs_custom', role='plex')) }}"`,
	}

	adjusted := AdjustMultilineIndent(lines, "plex_role_docker_envs", "plex2_docker_envs")

	// The new name is 4 chars shorter, so indentation should decrease by 4
	expectedSecondLine := `                       | combine(lookup('role_var', '_docker_envs_custom', role='plex')) }}"`

	if adjusted[1] != expectedSecondLine {
		t.Errorf("Adjusted indentation incorrect.\nGot:      %q\nExpected: %q", adjusted[1], expectedSecondLine)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func TestDockerEnvsType(t *testing.T) {
	// Test that block dict is detected as dict
	typeInfer := NewTypeInferrer(nil)

	// Simulating block dict with empty first line
	value := "\n  PLEX_UID: \"{{ uid }}\"\n  PLEX_GID: \"{{ gid }}\""
	typ := typeInfer.InferType("plex_role_docker_envs_default", value)
	if typ != "dict" {
		t.Errorf("Expected 'dict', got %q for block dict value", typ)
	}
}

func TestSSODetection(t *testing.T) {
	// Test SSO enabled detection with bazarr (has traefik_default_sso_middleware)
	testFile := "/srv/git/saltbox/roles/bazarr/defaults/main.yml"
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("Test file not found:", testFile)
	}

	p := New("bazarr", "saltbox")
	role, err := p.ParseFile(testFile)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if !role.SSOEnabled {
		t.Error("Expected bazarr to have SSO enabled by default")
	}

	// Test SSO disabled detection with plex (has empty string)
	testFile = "/srv/git/saltbox/roles/plex/defaults/main.yml"
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("Test file not found:", testFile)
	}

	p = New("plex", "saltbox")
	role, err = p.ParseFile(testFile)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if role.SSOEnabled {
		t.Error("Expected plex to have SSO disabled by default")
	}
}

func TestParseSubsections(t *testing.T) {
	// Test subsection parsing with authelia
	testFile := "/srv/git/saltbox/roles/authelia/defaults/main.yml"
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("Test file not found:", testFile)
	}

	p := New("authelia", "saltbox")
	role, err := p.ParseFile(testFile)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	// Check that Settings section has subsections
	settings, ok := role.Sections["Settings"]
	if !ok {
		t.Fatal("Expected to find Settings section")
	}

	t.Logf("Settings section:")
	t.Logf("  Variables (no subsection): %d", len(settings.Variables))
	t.Logf("  SubsectionOrder: %v", settings.SubsectionOrder)

	if len(settings.SubsectionOrder) == 0 {
		t.Error("Expected Settings section to have subsections")
	}

	for _, subName := range settings.SubsectionOrder {
		vars := settings.Subsections[subName]
		t.Logf("  Subsection '%s': %d vars", subName, len(vars))
		for _, v := range vars {
			t.Logf("    - %s", v.Name)
		}
	}
}

func TestFeatureFlags(t *testing.T) {
	// Test feature flags with traefik role (has DNS section)
	testFile := "/srv/git/saltbox/roles/traefik/defaults/main.yml"
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("Test file not found:", testFile)
	}

	p := New("traefik", "saltbox")
	role, err := p.ParseFile(testFile)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	// Traefik role should have DNS section
	if !role.HasDNS {
		t.Error("Expected traefik to have HasDNS=true")
	}

	t.Logf("traefik feature flags: HasDNS=%v, HasTraefik=%v, HasDocker=%v, HasWeb=%v, HasThemePark=%v",
		role.HasDNS, role.HasTraefik, role.HasDocker, role.HasWeb, role.HasThemePark)
}
