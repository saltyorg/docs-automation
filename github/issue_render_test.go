package github

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/saltyorg/docs-automation/health"
)

var testIssueStateMarker = regexp.MustCompile(`(?m)^<!-- docs-automation-state-v1:[A-Za-z0-9_-]+ -->$`)

func TestIssueRendererTitle(t *testing.T) {
	tests := []struct {
		name   string
		report health.Report
		want   string
	}{
		{
			name: "mixed plural severities",
			report: health.NewReport([]health.Result{
				{Kind: health.MissingDocumentation, Enabled: true, Findings: []health.Finding{{}, {}}},
				{Kind: health.EditorialAttention, Enabled: true, Findings: []health.Finding{{}, {}, {}}},
			}, health.RunInfo{}),
			want: "[Docs Health] 2 errors, 3 notices",
		},
		{
			name: "singular error",
			report: health.NewReport([]health.Result{
				{Kind: health.InvalidFrontmatter, Enabled: true, Findings: []health.Finding{{}}},
			}, health.RunInfo{}),
			want: "[Docs Health] 1 error",
		},
		{
			name: "singular notice",
			report: health.NewReport([]health.Result{
				{Kind: health.EditorialAttention, Enabled: true, Findings: []health.Finding{{}}},
			}, health.RunInfo{}),
			want: "[Docs Health] 1 notice",
		},
		{
			name:   "no findings",
			report: health.NewReport(nil, health.RunInfo{}),
			want:   "[Docs Health] healthy",
		},
	}

	renderer := NewIssueRenderer("saltyorg/docs")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderer.Title(tt.report); got != tt.want {
				t.Fatalf("Title() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIssueRendererBodyGolden(t *testing.T) {
	report := issueRendererGoldenReport()
	body, err := NewIssueRenderer("saltyorg/docs space").Body(report)
	if err != nil {
		t.Fatalf("Body() error = %v", err)
	}

	marker := testIssueStateMarker.FindString(body)
	if marker == "" {
		t.Fatal("Body() has no structured state marker")
	}
	if got := len(testIssueStateMarker.FindAllString(body, -1)); got != 1 {
		t.Fatalf("Body() state marker count = %d, want 1", got)
	}
	state, found, err := decodeIssueState(body)
	if err != nil {
		t.Fatalf("decodeIssueState(Body()) error = %v", err)
	}
	if !found {
		t.Fatal("decodeIssueState(Body()) found = false, want true")
	}
	if want := report.State(); !reflect.DeepEqual(state, want) {
		t.Fatalf("Body() state = %#v, want %#v", state, want)
	}
	normalized := strings.Replace(body, marker, "<!-- docs-automation-state-v1:STATE -->", 1)
	wantBytes, err := os.ReadFile("testdata/issue_body.golden.md")
	if err != nil {
		t.Fatalf("reading golden file: %v", err)
	}
	if normalized != string(wantBytes) {
		t.Fatalf("Body() differs from golden output\n--- got ---\n%s\n--- want ---\n%s", normalized, wantBytes)
	}

	if strings.Contains(body, "- [ ]") {
		t.Fatal("Body() contains task checkboxes")
	}
	if !strings.Contains(body, "[View this documentation-health workflow run]") {
		t.Fatal("Body() workflow link is not descriptive")
	}
	for _, escaped := range []string{
		"helper\\|\\`role\\`\\[beta\\]&lt;one&gt;<br>next",
		`bad\[role\]`,
		"must fix \\`title\\` \\| &lt;unsafe&gt;<br>second line",
	} {
		if !strings.Contains(body, escaped) {
			t.Errorf("Body() does not contain escaped value %q", escaped)
		}
	}
	for _, escapedURL := range []string{
		"https://github.com/saltyorg/saltbox/tree/0123456789abcdef/roles/helper%20%5Bbeta%5D/%3Ctask%3E",
		"https://github.com/saltyorg/docs%20space/blob/docs%2Frelease%20%5Bone%5D/saltbox/bad%20%5Bpage%5D/guide.md",
		"https://github.com/saltyorg/sandbox/tree/feature%2Fdocs%20branch/roles/draft-01",
	} {
		if !strings.Contains(body, escapedURL) {
			t.Errorf("Body() does not contain escaped URL %q", escapedURL)
		}
	}
}

func TestIssueRendererBodyEscapesWorkflowURLControlCharacters(t *testing.T) {
	report := health.NewReport(nil, health.RunInfo{
		WorkflowURL: "https://github.com/saltyorg/docs/actions/runs/1\r\ninjected",
	})

	body, err := NewIssueRenderer("saltyorg/docs").Body(report)
	if err != nil {
		t.Fatalf("Body() error = %v", err)
	}
	if strings.Contains(body, "\ninjected") || !strings.Contains(body, "/1%0D%0Ainjected)") {
		t.Fatalf("Body() did not URL-escape workflow control characters:\n%s", body)
	}
}

func TestIssueRendererBodyEscapesUnknownCheckLabels(t *testing.T) {
	report := health.NewReport([]health.Result{{
		Kind:    health.Kind("custom|<check>"),
		Enabled: true,
		Findings: []health.Finding{{
			Subject: "item",
			Detail:  "resolve it",
		}},
	}}, health.RunInfo{})

	body, err := NewIssueRenderer("saltyorg/docs").Body(report)
	if err != nil {
		t.Fatalf("Body() error = %v", err)
	}
	if !strings.Contains(body, "custom\\|&lt;check&gt;") || strings.Contains(body, "custom|<check>") {
		t.Fatalf("Body() did not escape an unknown check label:\n%s", body)
	}
}

func TestIssueRendererBodyEscapesLiteralMarkdownCharacters(t *testing.T) {
	report := health.NewReport([]health.Result{{
		Kind:    health.MissingDocumentation,
		Enabled: true,
		Findings: []health.Finding{{
			Kind:       health.MissingDocumentation,
			Repository: "saltbox",
			Subject:    "role*under_score~old",
			Path:       "docs/apps/role.md",
			Code:       "missing_doc",
			Detail:     "fix*under_score~old",
		}},
	}}, health.RunInfo{})

	body, err := NewIssueRenderer("saltyorg/docs").Body(report)
	if err != nil {
		t.Fatalf("Body() error = %v", err)
	}
	for _, want := range []string{`role\*under\_score\~old`, `fix\*under\_score\~old`} {
		if !strings.Contains(body, want) {
			t.Errorf("Body() missing literal Markdown text %q:\n%s", want, body)
		}
	}
}

func TestIssueRendererOrphanedDocumentationHasNoSourceLink(t *testing.T) {
	report := health.NewReport([]health.Result{{
		Kind:    health.OrphanedDocumentation,
		Enabled: true,
		Findings: []health.Finding{{
			Kind:       health.OrphanedDocumentation,
			Repository: "sandbox",
			Subject:    "orphan",
			Path:       "docs/sandbox/apps/orphan.md",
			SourcePath: "roles/orphan",
			Code:       "orphaned_doc",
			Detail:     "documentation page has no corresponding source role",
		}},
	}}, health.RunInfo{
		Branch: "main",
		Sources: []health.SourceRevision{{
			Repository: "sandbox",
			Slug:       "saltyorg/Sandbox",
			Revision:   "0123456789abcdef",
		}},
	})

	body, err := NewIssueRenderer("saltyorg/docs").Body(report)
	if err != nil {
		t.Fatalf("Body() error = %v", err)
	}
	if strings.Contains(body, "/roles/orphan") || strings.Contains(body, "[View source]") {
		t.Fatalf("Body() rendered a source link for an orphaned document:\n%s", body)
	}
	if !strings.Contains(body, "| sandbox | orphan | — | [Open Docs page]") {
		t.Fatalf("Body() orphan row does not render an em dash for source:\n%s", body)
	}
}

func TestIssueRendererBodyCapsVisibleFindingsPerKind(t *testing.T) {
	findings := make([]health.Finding, 105)
	for i := range findings {
		findings[i] = health.Finding{
			Kind:       health.EditorialAttention,
			Repository: "saltbox",
			Subject:    "item-" + threeDigits(i),
			Path:       "saltbox/item.md",
			Code:       "editorial_status",
			Detail:     "documentation status is draft",
		}
	}
	report := health.NewReport([]health.Result{{
		Kind:     health.EditorialAttention,
		Enabled:  true,
		Findings: findings,
	}}, health.RunInfo{})

	body, err := NewIssueRenderer("saltyorg/docs").Body(report)
	if err != nil {
		t.Fatalf("Body() error = %v", err)
	}
	if strings.Contains(body, "item-100") {
		t.Fatal("Body() rendered a finding beyond the 100-item cap")
	}
	if !strings.Contains(body, "5 additional findings omitted; view the complete Actions summary.") {
		t.Fatalf("Body() does not report the exact omitted count:\n%s", body)
	}
	if !strings.Contains(body, "<summary>Editorial Attention (105)</summary>") {
		t.Fatal("Body() does not wrap a category with more than ten findings")
	}
}

func TestIssueRendererBodyDoesNotExposeAbsolutePaths(t *testing.T) {
	report := health.NewReport([]health.Result{{
		Kind:    health.MissingDocumentation,
		Enabled: true,
		Findings: []health.Finding{{
			Kind:       health.MissingDocumentation,
			Repository: "saltbox",
			Subject:    "private",
			Path:       "/opt/private/docs.md",
			SourcePath: "/srv/private/role",
			Code:       "missing_doc",
			Detail:     "missing documentation",
		}},
	}}, health.RunInfo{})

	body, err := NewIssueRenderer("saltyorg/docs").Body(report)
	if err != nil {
		t.Fatalf("Body() error = %v", err)
	}
	if strings.Contains(body, "/opt/private") || strings.Contains(body, "/srv/private") {
		t.Fatalf("Body() exposed an absolute filesystem path:\n%s", body)
	}
}

func TestIssueRendererBodyRedactsAbsoluteFallbackLabels(t *testing.T) {
	tests := []struct {
		name      string
		finding   health.Finding
		secret    string
		wantLabel string
	}{
		{
			name: "absolute Path with empty Subject",
			finding: health.Finding{
				Kind:       health.MissingDocumentation,
				Repository: "saltbox",
				Path:       "/opt/private/docs.md",
				SourcePath: "roles/private",
				Code:       "missing_doc",
				Detail:     "missing documentation",
			},
			secret:    "/opt/private/docs.md",
			wantLabel: "(redacted path)",
		},
		{
			name: "absolute SourcePath with empty Subject and Path",
			finding: health.Finding{
				Kind:       health.MissingDocumentation,
				Repository: "saltbox",
				SourcePath: "/srv/private/role",
				Code:       "missing_doc",
				Detail:     "missing documentation",
			},
			secret:    "/srv/private/role",
			wantLabel: "(redacted path)",
		},
		{
			name: "relative Path remains useful",
			finding: health.Finding{
				Kind:       health.MissingDocumentation,
				Repository: "saltbox",
				Path:       "saltbox/private.md",
				SourcePath: "roles/private",
				Code:       "missing_doc",
				Detail:     "missing documentation",
			},
			wantLabel: "saltbox/private.md",
		},
		{
			name: "relative SourcePath remains useful",
			finding: health.Finding{
				Kind:       health.MissingDocumentation,
				Repository: "saltbox",
				SourcePath: "roles/private",
				Code:       "missing_doc",
				Detail:     "missing documentation",
			},
			wantLabel: "roles/private",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := health.NewReport([]health.Result{{
				Kind:     health.MissingDocumentation,
				Enabled:  true,
				Findings: []health.Finding{tt.finding},
			}}, health.RunInfo{})

			body, err := NewIssueRenderer("saltyorg/docs").Body(report)
			if err != nil {
				t.Fatalf("Body() error = %v", err)
			}
			if tt.secret != "" && strings.Contains(body, tt.secret) {
				t.Fatalf("Body() exposed absolute fallback %q:\n%s", tt.secret, body)
			}
			if !strings.Contains(body, tt.wantLabel) {
				t.Fatalf("Body() does not contain fallback label %q:\n%s", tt.wantLabel, body)
			}

			state, found, err := decodeIssueState(body)
			if err != nil {
				t.Fatalf("decodeIssueState(Body()) error = %v", err)
			}
			if !found {
				t.Fatal("decodeIssueState(Body()) found = false, want true")
			}
			got, ok := issueStateFindingByID(state, tt.finding.ID())
			if !ok {
				t.Fatalf("decoded state does not contain original finding ID %q", tt.finding.ID())
			}
			if got.Label != tt.wantLabel {
				t.Fatalf("decoded state label = %q, want %q", got.Label, tt.wantLabel)
			}
			if tt.secret != "" && strings.Contains(got.Label, tt.secret) {
				t.Fatalf("decoded state label exposed absolute fallback %q", tt.secret)
			}
		})
	}
}

func issueStateFindingByID(state health.State, id string) (health.StateFinding, bool) {
	for _, result := range state.Results {
		for _, finding := range result.Findings {
			if finding.ID == id {
				return finding, true
			}
		}
	}
	return health.StateFinding{}, false
}

func issueRendererGoldenReport() health.Report {
	editorial := make([]health.Finding, 11)
	for i := range editorial {
		editorial[i] = health.Finding{
			Kind:       health.EditorialAttention,
			Repository: "sandbox",
			Subject:    "draft-" + twoDigits(i+1),
			Path:       "sandbox/draft-" + twoDigits(i+1) + ".md",
			SourcePath: "roles/draft-" + twoDigits(i+1),
			Code:       "editorial_status",
			Detail:     `documentation status is "draft"`,
		}
	}

	return health.NewReport([]health.Result{
		{Kind: health.RoleAutomationError, Enabled: true},
		{Kind: health.CLIHelpAutomationError, Enabled: false, Exemptions: 9},
		{
			Kind:       health.MissingDocumentation,
			Enabled:    true,
			Exemptions: 2,
			Findings: []health.Finding{{
				Kind:       health.MissingDocumentation,
				Repository: "saltbox",
				Subject:    "helper|`role`[beta]<one>\nnext",
				Path:       "saltbox/helper-role.md",
				SourcePath: "roles/helper [beta]/<task>",
				Code:       "missing_doc",
				Detail:     "saltbox helper role has no documentation page",
			}},
		},
		{
			Kind:       health.InvalidFrontmatter,
			Enabled:    true,
			Exemptions: 1,
			Findings: []health.Finding{{
				Kind:       health.InvalidFrontmatter,
				Repository: "saltbox",
				Subject:    "bad[role]",
				Path:       "saltbox/bad [page]/guide.md",
				SourcePath: "roles/bad-role",
				Code:       "invalid_frontmatter",
				Detail:     "must fix `title` | <unsafe>\nsecond line",
			}},
		},
		{Kind: health.EditorialAttention, Enabled: true, Exemptions: 3, Findings: editorial},
	}, health.RunInfo{
		CheckedAt:   time.Date(2026, 9, 1, 14, 30, 0, 0, time.FixedZone("CEST", 2*60*60)),
		WorkflowURL: "https://github.com/saltyorg/docs/actions/runs/123456",
		Branch:      "docs/release [one]",
		Version:     "v1.2.3|build",
		Sources: []health.SourceRevision{
			{Repository: "saltbox", Slug: "saltyorg/saltbox", Ref: "master", Revision: "0123456789abcdef"},
			{Repository: "sandbox", Slug: "saltyorg/sandbox", Ref: "feature/docs branch"},
		},
	})
}

func twoDigits(value int) string {
	return string([]byte{'0' + byte(value/10), '0' + byte(value%10)})
}

func threeDigits(value int) string {
	return string([]byte{'0' + byte(value/100), '0' + byte(value/10%10), '0' + byte(value%10)})
}
