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
	writeInventorySource(t, root)
	writeDockerSource(t, root)
	cfg := &config.Config{Repositories: config.RepositoryConfig{Saltbox: root}}

	sources, err := loadSourceCatalog(cfg)
	if err != nil {
		t.Fatalf("loadSourceCatalog() error = %v", err)
	}
	if got := sources.RoleVarLookups["_web_host_override"]; got != parser.String {
		t.Fatalf("role lookup type = %q, want %q", got, parser.String)
	}
	if !slices.Contains(sources.DockerVarSuffixes, "envs") {
		t.Fatalf("DockerVarSuffixes = %v, want envs", sources.DockerVarSuffixes)
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
	cfg := &config.Config{Repositories: config.RepositoryConfig{Saltbox: root}}

	_, err := loadSourceCatalog(cfg)
	if err == nil || !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "scanning docker variables") {
		t.Fatalf("loadSourceCatalog() error = %v, want scanning docker variables with fs.ErrNotExist", err)
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
