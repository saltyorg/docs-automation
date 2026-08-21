package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeDockerSuffix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{input: "_docker_dev_dri", expected: "dev_dri"},
		{input: "dev_dri", expected: "dev_dri"},
		{input: "_dev_dri", expected: "dev_dri"},
		{input: "  _docker_memory  ", expected: "memory"},
		{input: "", expected: ""},
	}

	for _, tt := range tests {
		if got := NormalizeDockerSuffix(tt.input); got != tt.expected {
			t.Fatalf("NormalizeDockerSuffix(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestGetDockerVarSuffixes_AppliesIgnoreList(t *testing.T) {
	tmpDir := t.TempDir()
	dockerTasksDir := filepath.Join(tmpDir, "tasks", "docker")
	if err := os.MkdirAll(dockerTasksDir, 0o755); err != nil {
		t.Fatalf("creating docker tasks dir: %v", err)
	}

	content := `
- name: test
  ansible.builtin.set_fact:
    _docker_dev_dri: "{{ lookup('docker_var', '_docker_dev_dri', default=false) }}"
    _docker_network_mode: "{{ lookup('docker_var', '_docker_network_mode', default='bridge') }}"
    _docker_var_specs:
      _docker_extra_hosts:
        type: list
`
	filePath := filepath.Join(dockerTasksDir, "create.yml")
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing docker task file: %v", err)
	}

	scanner := NewDockerVarScanner(tmpDir)

	roleName := "myrole"
	// Simulate role already defining network_mode so it should be excluded regardless.
	roleDockerVars := []string{"myrole_role_docker_network_mode"}
	// Ignore dev_dri via full suffix and extra_hosts via short suffix.
	ignoreSuffixes := []string{"_docker_dev_dri", "extra_hosts"}

	got, err := scanner.GetDockerVarSuffixes(roleName, roleDockerVars, ignoreSuffixes)
	if err != nil {
		t.Fatalf("GetDockerVarSuffixes returned error: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("expected no additional vars after role-defined + ignored filtering, got: %v", got)
	}
}
