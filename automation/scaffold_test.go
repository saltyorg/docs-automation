package automation

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/document"
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

func TestScaffoldProvidesDockerMetadataAndSemanticLinksToCustomTemplates(t *testing.T) {
	tests := []struct {
		name         string
		defaults     string
		wantDocker   bool
		wantIcon     string
		wantName     string
		wantURL      string
		wantLinkType string
	}{
		{
			name:         "resolved docker",
			defaults:     "####################\n# Docker\n####################\nwidget_role_docker_image_repo: ghcr.io/acme/widget\n",
			wantDocker:   true,
			wantIcon:     "material/docker",
			wantName:     "Image tags",
			wantURL:      "https://github.com/acme/widget/pkgs/container/widget",
			wantLinkType: "github",
		},
		{
			name:         "unresolved docker",
			defaults:     "####################\n# Docker\n####################\nwidget_role_docker_image_repo: \"{{ widget_image }}\"\n",
			wantDocker:   true,
			wantIcon:     "material/docker",
			wantName:     "Image tags",
			wantLinkType: "releases",
		},
		{
			name:         "non docker",
			defaults:     "####################\n# Basics\n####################\nwidget_enabled: true\n",
			wantName:     "Releases",
			wantLinkType: "releases",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			saltbox := filepath.Join(root, "saltbox")
			sandbox := filepath.Join(root, "sandbox")
			docs := filepath.Join(root, "docs")
			rolePath := filepath.Join(saltbox, "roles", "widget", "defaults")
			for _, directory := range []string{rolePath, filepath.Join(sandbox, "roles"), docs} {
				if err := os.MkdirAll(directory, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(rolePath, "main.yml"), []byte(tt.defaults), 0o644); err != nil {
				t.Fatal(err)
			}
			templatePath := filepath.Join(root, "custom.tmpl")
			if err := os.WriteFile(templatePath, []byte("docker={{.IsDocker}} icon={{.Icon}}\n{{range .AppLinks}}{{.Name}}|{{.URL}}|{{.Type}}|{{.Purpose}}\n{{end}}role={{.RoleName}}"), 0o644); err != nil {
				t.Fatal(err)
			}
			outputPath := filepath.Join(docs, "widget.md")
			cfg := &config.Config{
				Repositories: config.RepositoryConfig{Saltbox: saltbox, Sandbox: sandbox, Docs: docs},
				DockerMetadata: config.DockerMetadataConfig{
					Icon:        "material/docker",
					ReleaseLink: config.DockerMetadataReleaseLink{Name: "Image tags"},
					Rules: []config.DockerMetadataRule{{
						Pattern: `^ghcr\.io/([^/]+)/([^/]+)$`,
						URL:     "https://github.com/$1/$2/pkgs/container/$2",
						Type:    "github",
					}},
				},
			}

			if err := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false).Scaffold(t.Context(), cfg, "widget", ScaffoldOptions{TemplatePath: templatePath, OutputPath: outputPath}); err != nil {
				t.Fatalf("Scaffold() error = %v", err)
			}
			content, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatal(err)
			}
			wantHeader := "docker=false icon=\n"
			if tt.wantDocker {
				wantHeader = "docker=true icon=" + tt.wantIcon + "\n"
			}
			if !strings.HasPrefix(string(content), wantHeader) {
				t.Fatalf("scaffold header = %q, want prefix %q", content, wantHeader)
			}
			wantLinks := []document.AppLink{
				{Name: "Manual", Type: "documentation", Purpose: document.AppLinkPurposeManual},
				{Name: tt.wantName, URL: tt.wantURL, Type: tt.wantLinkType, Purpose: document.AppLinkPurposeRelease},
				{Name: "Community", Type: "community", Purpose: document.AppLinkPurposeCommunity},
			}
			for _, link := range wantLinks {
				want := link.Name + "|" + link.URL + "|" + link.Type + "|" + string(link.Purpose)
				if !strings.Contains(string(content), want) {
					t.Errorf("scaffold missing %q:\n%s", want, content)
				}
			}
			if strings.Count(string(content), "|") != 9 {
				t.Fatalf("scaffold link count is not exactly three:\n%s", content)
			}
		})
	}
}
