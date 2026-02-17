package github

import (
	"strings"
	"testing"
)

func TestExtractIssueCounts(t *testing.T) {
	body := `## 📝 Documentation Status

### Missing Documentation (3)
Roles without corresponding documentation pages:

### Missing Variables Sections (4)
Documentation pages without the managed variables section:

### Missing Overview Sections (2)
Documentation pages without the managed overview section:

### Orphaned Documentation (1)
Documentation pages without corresponding roles:
`

	counts := extractIssueCounts(body)

	if counts.MissingDocs != 3 {
		t.Fatalf("expected MissingDocs=3, got %d", counts.MissingDocs)
	}
	if counts.MissingSections != 4 {
		t.Fatalf("expected MissingSections=4, got %d", counts.MissingSections)
	}
	if counts.MissingOverviewSections != 2 {
		t.Fatalf("expected MissingOverviewSections=2, got %d", counts.MissingOverviewSections)
	}
	if counts.OrphanedDocs != 1 {
		t.Fatalf("expected OrphanedDocs=1, got %d", counts.OrphanedDocs)
	}
}

func TestExtractIssueCountsMissingHeadings(t *testing.T) {
	counts := extractIssueCounts("## no section counts here")

	if counts.MissingDocs != 0 || counts.MissingSections != 0 || counts.MissingOverviewSections != 0 || counts.OrphanedDocs != 0 {
		t.Fatalf("expected all counts to default to zero, got %+v", counts)
	}
}

func TestBuildCompactLineDiff(t *testing.T) {
	oldBody := "line1\nline2\nline3"
	newBody := "line1\nline2-changed\nline3"

	diff := buildCompactLineDiff(oldBody, newBody, 100)

	if !strings.Contains(diff, "@@") {
		t.Fatalf("expected unified hunk header in diff, got: %s", diff)
	}
	if !strings.Contains(diff, "-line2") {
		t.Fatalf("expected removed line in diff, got: %s", diff)
	}
	if !strings.Contains(diff, "+line2-changed") {
		t.Fatalf("expected added line in diff, got: %s", diff)
	}
}

func TestGenerateIssueBodyUpdateComment(t *testing.T) {
	t.Setenv("GITHUB_HEAD_REF", "")
	t.Setenv("GITHUB_REF_NAME", "main")

	oldBody := `## 📝 Documentation Status

### Missing Documentation (2)

### Missing Variables Sections (1)

### Missing Overview Sections (1)

### Orphaned Documentation (0)
`
	newBody := `## 📝 Documentation Status

### Missing Documentation (1)

### Missing Variables Sections (3)

### Missing Overview Sections (1)

### Orphaned Documentation (2)
`

	manager := NewIssueManager("owner/repo", "https://example.com/run/123")
	comment := manager.GenerateIssueBodyUpdateComment(oldBody, newBody)

	if !strings.Contains(comment, "### Docs Automation: Main Post Updated") {
		t.Fatalf("missing update comment heading")
	}
	if !strings.Contains(comment, "Run: [workflow link](https://example.com/run/123)") {
		t.Fatalf("missing workflow link")
	}
	if !strings.Contains(comment, "| Missing Documentation | 2 | 1 | -1 |") {
		t.Fatalf("missing or incorrect Missing Documentation delta row")
	}
	if !strings.Contains(comment, "| Missing Variables Sections | 1 | 3 | +2 |") {
		t.Fatalf("missing or incorrect Missing Variables Sections delta row")
	}
	if !strings.Contains(comment, "```diff") {
		t.Fatalf("missing diff code block")
	}
	if !strings.Contains(comment, "<!-- docs-automation-body-sha256:") {
		t.Fatalf("missing body hash marker")
	}
}
