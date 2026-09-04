package automation

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/parser"
)

func TestLoadSourceCatalog(t *testing.T) {
	root := t.TempDir()
	saltboxRoot := filepath.Join(root, "saltbox")
	sandboxRoot := filepath.Join(root, "sandbox")
	writeInventorySource(t, saltboxRoot)
	writeDirectorySource(t, saltboxRoot)
	writeDockerSource(t, saltboxRoot)
	writeManagedDirectoryTask(t, saltboxRoot, "saltbox_only", validManagedDirectoryTask)
	writeManagedDirectoryTask(t, sandboxRoot, "sandbox_only", validManagedDirectoryTask)
	cfg := &config.Config{
		Repositories:    config.RepositoryConfig{Saltbox: saltboxRoot, Sandbox: sandboxRoot},
		GlobalOverrides: config.GlobalOverrides{IgnoreSuffixes: []string{"_paths_folders_list"}},
	}

	sources, err := loadSourceCatalog(cfg)
	if err != nil {
		t.Fatalf("loadSourceCatalog() error = %v", err)
	}
	if got := sources.RoleVarLookups["_web_host_override"]; got != parser.String {
		t.Fatalf("role lookup type = %q, want %q", got, parser.String)
	}
	if _, exists := sources.RoleVarLookups["_paths_folders_list"]; exists {
		t.Fatalf("RoleVarLookups = %v, want ignored _paths_folders_list", sources.RoleVarLookups)
	}
	if got := sources.RoleVarLookups["_paths_folders_list_custom"]; got != parser.List {
		t.Fatalf("custom directory lookup type = %q, want %q", got, parser.List)
	}
	if !slices.Contains(sources.DockerVarSuffixes, "envs") {
		t.Fatalf("DockerVarSuffixes = %v, want envs", sources.DockerVarSuffixes)
	}
	if !sources.HasManagedDirectories("saltbox", "saltbox_only") {
		t.Fatal("saltbox_only managed-directory capability = false, want true")
	}
	if !sources.HasManagedDirectories("sandbox", "sandbox_only") {
		t.Fatal("sandbox_only managed-directory capability = false, want true")
	}
}

func TestLoadSourceCatalogReportsMissingInventory(t *testing.T) {
	root := t.TempDir()
	writeDockerSource(t, root)
	cfg := &config.Config{Repositories: config.RepositoryConfig{Saltbox: root}}

	_, err := loadSourceCatalog(cfg)
	if err == nil || !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "scanning inventory") {
		t.Fatalf("loadSourceCatalog() error = %v, want scanning inventory with fs.ErrNotExist", err)
	}
}

func TestLoadSourceCatalogReportsMissingDockerSources(t *testing.T) {
	root := t.TempDir()
	writeInventorySource(t, root)
	writeDirectorySource(t, root)
	cfg := &config.Config{Repositories: config.RepositoryConfig{Saltbox: root}}

	_, err := loadSourceCatalog(cfg)
	if err == nil || !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "scanning docker variables") {
		t.Fatalf("loadSourceCatalog() error = %v, want scanning docker variables with fs.ErrNotExist", err)
	}
}

func TestLoadSourceCatalogReportsMissingDirectoryRoleVarSource(t *testing.T) {
	root := t.TempDir()
	writeInventorySource(t, root)
	writeDockerSource(t, root)
	directoryPath := filepath.Join(root, "resources", "tasks", "directories", "create_directories.yml")
	cfg := &config.Config{Repositories: config.RepositoryConfig{Saltbox: root}}

	_, err := loadSourceCatalog(cfg)
	if err == nil || !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), directoryPath) {
		t.Fatalf("loadSourceCatalog() error = %v, want contextual %s fs.ErrNotExist", err, directoryPath)
	}
}

func TestLoadSourceCatalogReportsManagedDirectoryTaskParseErrors(t *testing.T) {
	for _, repoType := range []string{"saltbox", "sandbox"} {
		t.Run(repoType, func(t *testing.T) {
			root := t.TempDir()
			saltboxRoot := filepath.Join(root, "saltbox")
			sandboxRoot := filepath.Join(root, "sandbox")
			writeInventorySource(t, saltboxRoot)
			writeDirectorySource(t, saltboxRoot)
			writeDockerSource(t, saltboxRoot)
			writeRoleRoot(t, saltboxRoot)
			writeRoleRoot(t, sandboxRoot)
			brokenRoot := saltboxRoot
			if repoType == "sandbox" {
				brokenRoot = sandboxRoot
			}
			writeManagedDirectoryTask(t, brokenRoot, "broken", "- block: [\n")
			cfg := &config.Config{
				Repositories: config.RepositoryConfig{Saltbox: saltboxRoot, Sandbox: sandboxRoot},
			}

			_, err := loadSourceCatalog(cfg)
			brokenPath := filepath.Join(brokenRoot, "roles", "broken", "tasks", "main.yml")
			if err == nil || !strings.Contains(err.Error(), "scanning "+repoType+" managed-directory roles") ||
				!strings.Contains(err.Error(), brokenPath) {
				t.Fatalf("loadSourceCatalog() error = %v, want contextual %s parse failure", err, repoType)
			}
		})
	}
}

func writeInventorySource(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, "inventories", "group_vars", "all.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `host: "{{ lookup('role_var', '_web_host_override', default='') }}"` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeDirectorySource(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, "resources", "tasks", "directories", "create_directories.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `
- name: create directories
  ansible.builtin.file:
    path: "{{ item.path }}"
    owner: "{{ lookup('role_var', '_paths_owner', default='') }}"
    group: "{{ lookup('role_var', '_paths_group', default='') }}"
    mode: "{{ lookup('role_var', '_paths_permissions', default='0775') }}"
    recurse: "{{ lookup('role_var', '_paths_recursive', default=false) }}"
  loop: "{{ lookup('role_var', '_paths_folders_list', default=[]) + lookup('role_var', '_paths_folders_list_custom', default=[]) }}"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeDockerSource(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, "resources", "tasks", "docker", "create.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `envs: "{{ lookup('docker_var', '_docker_envs', default={}) }}"` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const validManagedDirectoryTask = `
- ansible.builtin.include_tasks: "{{ resources_tasks_path }}/directories/create_directories.yml"
`

func writeManagedDirectoryTask(t *testing.T, root, roleName, content string) {
	t.Helper()
	path := filepath.Join(root, "roles", roleName, "tasks", "main.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeRoleRoot(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "roles"), 0o755); err != nil {
		t.Fatal(err)
	}
}
