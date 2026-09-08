package automation

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saltyorg/docs-automation/config"
)

func TestGenerateReturnsInvalidExistingFrontmatterError(t *testing.T) {
	fixture := newGenerateFrontmatterFixture(t)
	fixture.addRole(t, "broken")
	docPath := fixture.addDoc(t, "broken", "---\nsaltbox_automation: [\n---\n")

	err := fixture.runner.Generate(t.Context(), fixture.cfg, "broken", GenerateOptions{})

	if err == nil {
		t.Fatal("Generate() error = nil, want invalid frontmatter error")
	}
	if !strings.Contains(err.Error(), docPath) {
		t.Fatalf("Generate() error = %q, want documentation page path %q", err, docPath)
	}
	if !strings.Contains(err.Error(), "parsing frontmatter") {
		t.Fatalf("Generate() error = %q, want frontmatter parse context", err)
	}
}

func TestGenerateAllReportsInvalidFrontmatterAndContinues(t *testing.T) {
	fixture := newGenerateFrontmatterFixture(t)
	fixture.addRole(t, "broken")
	docPath := fixture.addDoc(t, "broken", "---\nsaltbox_automation: [\n---\n")
	fixture.addRole(t, "healthy")

	err := fixture.runner.Generate(t.Context(), fixture.cfg, "", GenerateOptions{})

	if err != nil {
		t.Fatalf("Generate() error = %v, want batch generation to continue", err)
	}
	if got := fixture.errOut.String(); !strings.Contains(got, docPath) || !strings.Contains(got, "parsing frontmatter") {
		t.Fatalf("Generate() stderr = %q, want invalid page and frontmatter parse context", got)
	}
	if got := fixture.out.String(); !strings.Contains(got, "role=healthy") {
		t.Fatalf("Generate() output = %q, want later healthy role output", got)
	}
}

func TestGenerateAllowsMissingDocumentationPage(t *testing.T) {
	fixture := newGenerateFrontmatterFixture(t)
	fixture.addRole(t, "new-role")

	err := fixture.runner.Generate(t.Context(), fixture.cfg, "new-role", GenerateOptions{})

	if err != nil {
		t.Fatalf("Generate() error = %v, want missing documentation page allowed", err)
	}
	if got := fixture.out.String(); !strings.Contains(got, "role=new-role") {
		t.Fatalf("Generate() output = %q, want rendered role", got)
	}
}

func TestGenerateReturnsExistingPageReadError(t *testing.T) {
	fixture := newGenerateFrontmatterFixture(t)
	fixture.addRole(t, "unreadable")
	docPath := filepath.Join(fixture.cfg.SaltboxDocsPath(), "unreadable.md")
	if err := os.MkdirAll(docPath, 0o755); err != nil {
		t.Fatalf("creating directory at documentation page path: %v", err)
	}

	err := fixture.runner.Generate(t.Context(), fixture.cfg, "unreadable", GenerateOptions{})

	if err == nil {
		t.Fatal("Generate() error = nil, want existing page read error")
	}
	if !strings.Contains(err.Error(), docPath) {
		t.Fatalf("Generate() error = %q, want documentation page path %q", err, docPath)
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Generate() error = %q, want non-not-exist read failure", err)
	}
	if !strings.Contains(err.Error(), "reading documentation page") {
		t.Fatalf("Generate() error = %q, want page read context", err)
	}
}

type generateFrontmatterFixture struct {
	cfg    *config.Config
	runner *Runner
	out    bytes.Buffer
	errOut bytes.Buffer
}

func newGenerateFrontmatterFixture(t *testing.T) *generateFrontmatterFixture {
	t.Helper()
	root := t.TempDir()
	saltbox := filepath.Join(root, "saltbox")
	sandbox := filepath.Join(root, "sandbox")
	docs := filepath.Join(root, "docs")
	for _, directory := range []string{
		filepath.Join(saltbox, "inventories", "group_vars"),
		filepath.Join(saltbox, "resources", "tasks", "directories"),
		filepath.Join(saltbox, "resources", "tasks", "docker"),
		filepath.Join(saltbox, "roles"),
		filepath.Join(sandbox, "roles"),
		filepath.Join(docs, "docs", "apps"),
		filepath.Join(docs, "docs", "sandbox", "apps"),
		filepath.Join(docs, "templates"),
	} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("creating fixture directory: %v", err)
		}
	}
	writeGenerateFrontmatterFile(t, filepath.Join(saltbox, "inventories", "group_vars", "all.yml"), nil)
	writeGenerateFrontmatterFile(t, filepath.Join(saltbox, "resources", "tasks", "directories", "create_directories.yml"), nil)
	writeGenerateFrontmatterFile(t, filepath.Join(docs, "templates", "inventory.md.tmpl"), []byte("role={{ .RoleName }}\n"))

	fixture := &generateFrontmatterFixture{
		cfg: &config.Config{
			Repositories: config.RepositoryConfig{Saltbox: saltbox, Sandbox: sandbox, Docs: docs},
		},
	}
	fixture.runner = NewRunner(&fixture.out, &fixture.errOut, false)
	return fixture
}

func (f *generateFrontmatterFixture) addRole(t *testing.T, role string) {
	t.Helper()
	path := filepath.Join(f.cfg.SaltboxRolesPath(), role, "defaults", "main.yml")
	writeGenerateFrontmatterFile(t, path, []byte(role+"_enabled: true\n"))
}

func (f *generateFrontmatterFixture) addDoc(t *testing.T, role, content string) string {
	t.Helper()
	path := filepath.Join(f.cfg.SaltboxDocsPath(), role+".md")
	writeGenerateFrontmatterFile(t, path, []byte(content))
	return path
}

func writeGenerateFrontmatterFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating fixture parent: %v", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("writing fixture file: %v", err)
	}
}
