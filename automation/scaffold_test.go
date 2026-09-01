package automation

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saltyorg/docs-automation/config"
)

func TestScaffoldPreservesExistingFileWhenTemplateExecutionFails(t *testing.T) {
	root := t.TempDir()
	saltboxRoot := filepath.Join(root, "saltbox")
	sandboxRoot := filepath.Join(root, "sandbox")
	docsRoot := filepath.Join(root, "docs")
	rolePath := filepath.Join(saltboxRoot, "roles", "sonarr")
	if err := os.MkdirAll(rolePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sandboxRoot, "roles"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(docsRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(docsRoot, "sonarr.md")
	if err := os.WriteFile(outputPath, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	templatePath := filepath.Join(root, "scaffold.tmpl")
	if err := os.WriteFile(templatePath, []byte("{{call .RoleName}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Repositories: config.RepositoryConfig{
		Saltbox: saltboxRoot,
		Sandbox: sandboxRoot,
		Docs:    docsRoot,
	}}
	runner := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false)

	err := runner.Scaffold(t.Context(), cfg, "sonarr", ScaffoldOptions{
		TemplatePath: templatePath,
		OutputPath:   outputPath,
		Force:        true,
	})
	if err == nil || !strings.Contains(err.Error(), "executing template") {
		t.Fatalf("Scaffold() error = %v, want executing template", err)
	}
	content, readErr := os.ReadFile(outputPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if got, want := string(content), "original\n"; got != want {
		t.Fatalf("output content = %q, want %q", got, want)
	}
}
