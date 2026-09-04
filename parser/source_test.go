package parser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestScanRoleVarLookupsUnionsAuthoritativeSources(t *testing.T) {
	root := t.TempDir()
	inventoryPath := filepath.Join(root, "inventories", "group_vars", "all.yml")
	directoriesPath := filepath.Join(root, "resources", "tasks", "directories", "create_directories.yml")
	writeSourceFile(t, inventoryPath, `
recursive: "{{ lookup('role_var', '_paths_recursive', default='') }}"
`)
	writeSourceFile(t, directoriesPath, `
folders: "{{ lookup('role_var', '_paths_folders_list', default=[]) }}"
folders_custom: "{{ lookup('role_var', '_paths_folders_list_custom', default=[]) }}"
owner: "{{ lookup('role_var', '_paths_owner', default='') }}"
group: "{{ lookup('role_var', '_paths_group', default='') }}"
permissions: "{{ lookup('role_var', '_paths_permissions', default='0775') }}"
recursive: "{{ lookup('role_var', '_paths_recursive', default=false) }}"
`)

	got, err := ScanRoleVarLookups(
		[]string{inventoryPath, directoriesPath},
		[]string{"_paths_folders_list"},
	)
	if err != nil {
		t.Fatalf("ScanRoleVarLookups() error = %v", err)
	}
	want := map[string]string{
		"_paths_folders_list_custom": List,
		"_paths_owner":               String,
		"_paths_group":               String,
		"_paths_permissions":         String,
		"_paths_recursive":           Bool,
	}
	if len(got) != len(want) {
		t.Fatalf("ScanRoleVarLookups() = %v, want %v", got, want)
	}
	for suffix, wantType := range want {
		if got[suffix] != wantType {
			t.Errorf("ScanRoleVarLookups()[%q] = %q, want %q", suffix, got[suffix], wantType)
		}
	}
}

func TestScanRoleVarLookupsReportsSourcePath(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing.yml")
	_, err := ScanRoleVarLookups([]string{missingPath}, nil)
	if err == nil || !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), missingPath) {
		t.Fatalf("error = %v, want contextual %s fs.ErrNotExist", err, missingPath)
	}
}

func TestDockerVarScannerRequiresDockerTasksDirectory(t *testing.T) {
	scanner := NewDockerVarScanner(t.TempDir())
	_, err := scanner.FindDockerVarLookups()
	if err == nil || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("error = %v, want fs.ErrNotExist", err)
	}
}

func TestDockerVarScannerDoesNotCacheFailedScan(t *testing.T) {
	resourcesPath := t.TempDir()
	dockerTasksPath := filepath.Join(resourcesPath, "tasks", "docker")
	if err := os.MkdirAll(dockerTasksPath, 0o755); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(resourcesPath, "late.yml")
	brokenPath := filepath.Join(dockerTasksPath, "broken.yml")
	if err := os.Symlink(targetPath, brokenPath); err != nil {
		t.Fatal(err)
	}

	scanner := NewDockerVarScanner(resourcesPath)
	if _, err := scanner.FindDockerVarLookups(); err == nil {
		t.Fatal("first FindDockerVarLookups() error = nil, want broken-link read error")
	}

	content := `
- name: test
  ansible.builtin.set_fact:
    _docker_envs: "{{ lookup('docker_var', '_docker_envs', default={}) }}"
`
	if err := os.WriteFile(targetPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := scanner.FindDockerVarLookups()
	if err != nil {
		t.Fatalf("second FindDockerVarLookups() error = %v", err)
	}
	if !slices.Contains(got, "envs") {
		t.Fatalf("second FindDockerVarLookups() = %v, want envs", got)
	}
}

func writeSourceFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
