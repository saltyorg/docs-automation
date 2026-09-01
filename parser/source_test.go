package parser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestScanInventoryForRoleVarLookupsRequiresInventory(t *testing.T) {
	_, err := ScanInventoryForRoleVarLookups(filepath.Join(t.TempDir(), "missing.yml"), nil)
	if err == nil || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("error = %v, want fs.ErrNotExist", err)
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
