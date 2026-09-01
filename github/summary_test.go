package github

import (
	"os"
	"strings"
	"testing"

	"github.com/saltyorg/docs-automation/health"
)

func TestWriteGitHubSummaryIncludesHealthReport(t *testing.T) {
	summaryPath := t.TempDir() + "/summary.md"
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)

	summary := NewUpdateSummary()
	summary.AddRole(RoleResult{
		Name:     "role|failure",
		RepoType: "saltbox",
		Status:   StatusError,
		Error:    "update|failed\nretry required",
	})
	summary.SetHealthReport(&health.Report{Results: []health.Result{
		{Kind: health.RoleAutomationError, Enabled: true},
		{
			Kind:    health.MissingDocumentation,
			Enabled: true,
			Findings: []health.Finding{{
				Kind:       health.MissingDocumentation,
				Repository: "saltbox",
				Subject:    "radarr|beta",
				Path:       "docs/apps/radarr|beta.md",
				Detail:     "documentation|missing\nadd a page",
			}},
		},
		{
			Kind:    health.EditorialAttention,
			Enabled: true,
			Findings: []health.Finding{{
				Kind:       health.EditorialAttention,
				Repository: "saltbox",
				Subject:    "sonarr",
				Path:       "docs/apps/sonarr.md",
				Detail:     "editorial|review\nneeded",
			}},
		},
		{Kind: health.InvalidFrontmatter},
	}})

	if err := summary.WriteGitHubSummary(); err != nil {
		t.Fatalf("WriteGitHubSummary() error = %v", err)
	}

	contents, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", summaryPath, err)
	}
	got := string(contents)
	for _, want := range []string{
		"Documentation Automation Results",
		"Documentation Health",
		"Missing Documentation",
		"Editorial Attention",
		"Passed",
		"Disabled",
		"role\\|failure",
		"update\\|failed<br>retry required",
		"radarr\\|beta",
		"docs/apps/radarr\\|beta.md",
		"documentation\\|missing<br>add a page",
		"editorial\\|review<br>needed",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q:\n%s", want, got)
		}
	}
}

func TestWriteGitHubSummaryEscapesCompleteLiteralCells(t *testing.T) {
	summaryPath := t.TempDir() + "/summary.md"
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)

	summary := NewUpdateSummary()
	summary.AddRole(RoleResult{
		Name:     "role\\&<>|`[]*_~",
		RepoType: "saltbox",
		Status:   StatusError,
		Error:    "literal Markdown",
	})
	if err := summary.WriteGitHubSummary(); err != nil {
		t.Fatalf("WriteGitHubSummary() error = %v", err)
	}

	contents, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", summaryPath, err)
	}
	want := "role\\\\&amp;&lt;&gt;\\|\\`\\[\\]\\*\\_\\~"
	if !strings.Contains(string(contents), want) {
		t.Fatalf("summary missing complete literal cell %q:\n%s", want, contents)
	}
}
