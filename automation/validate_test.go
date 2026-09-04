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

func TestValidateFrontmatterHonorsStandaloneCheckControls(t *testing.T) {
	docsRoot := filepath.Join(t.TempDir(), "docs")
	saltboxDocs := filepath.Join(docsRoot, "docs", "apps")
	sandboxDocs := filepath.Join(docsRoot, "docs", "sandbox", "apps")
	for _, dir := range []string{saltboxDocs, sandboxDocs} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("creating Docs fixture directory: %v", err)
		}
	}

	disabled := false
	cfg := &config.Config{
		Repositories: config.RepositoryConfig{Docs: docsRoot},
		Checks: config.ChecksConfig{Frontmatter: config.CheckConfig{
			Enabled:      &disabled,
			ExcludePaths: []string{"docs/apps/excluded-malformed.md"},
		}},
	}
	files := map[string]string{
		filepath.Join(saltboxDocs, "excluded-malformed.md"): "---\nsaltbox_automation: [\n---\n",
		filepath.Join(saltboxDocs, "page-off.md"): `---
saltbox_automation:
  checks:
    frontmatter: false
  app_links:
    - name: ""
      url: ""
---
`,
		filepath.Join(sandboxDocs, "overview-off.md"): `---
saltbox_automation:
  sections:
    overview: false
  app_links:
    - name: ""
      url: ""
---
`,
		filepath.Join(sandboxDocs, "invalid.md"): `---
saltbox_automation:
  app_links:
    - name: ""
      url: ""
      purpose: release
  project_description:
    name: ""
    summary: ""
---
`,
		filepath.Join(sandboxDocs, "valid.md"): `---
saltbox_automation:
  project_description:
    name: Valid
    summary: Complete metadata.
---
`,
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("writing Docs fixture %q: %v", path, err)
		}
	}

	var output bytes.Buffer
	runner := NewRunner(&output, new(bytes.Buffer), true)
	err := runner.ValidateFrontmatter(t.Context(), cfg)
	if err == nil || !strings.Contains(err.Error(), "found 1 invalid files") {
		t.Fatalf("ValidateFrontmatter() error = %v, want one invalid file", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ValidateFrontmatter() returned filesystem error: %v", err)
	}

	got := output.String()
	for _, want := range []string{
		"excluded-malformed.md: excluded by config",
		"page-off.md: excluded by frontmatter",
		"overview-off.md: excluded by frontmatter",
		"app_links[0].name is required",
		"app_links[0].url is required",
		"project_description.name is required",
		"project_description.summary is required",
		"Validation complete: 1 valid, 1 invalid, 0 without frontmatter, 3 excluded",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ValidateFrontmatter() output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "excluded-malformed.md: parsing frontmatter") {
		t.Fatalf("path-excluded malformed page was parsed:\n%s", got)
	}
	if count := strings.Count(got, filepath.Join(sandboxDocs, "invalid.md")+":"); count != 4 {
		t.Fatalf("invalid page diagnostics = %d, want 4 diagnostics counted as one invalid file:\n%s", count, got)
	}
}
