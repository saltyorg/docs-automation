package automation

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/github"
	"github.com/saltyorg/docs-automation/health"
)

func TestBuildHealthReportCollectsConfiguredFindings(t *testing.T) {
	fixture := newHealthFixture(t)
	fixture.enableAllChecks()
	fixture.cfg.Blacklist.DocsCoverage.Saltbox = []string{"helper"}
	fixture.cfg.Checks.Coverage.ExcludePaths = []string{"docs/apps/exact.md"}
	fixture.cfg.Checks.Frontmatter.ExcludePaths = []string{"docs/apps/exact.md"}
	fixture.cfg.Checks.Editorial.ExcludePaths = []string{"docs/apps/exact.md"}

	for _, role := range []string{
		"missing", "helper", "overview", "inventory-off", "invalid", "parse-error",
		"outdated", "draft", "page-disabled", "frontmatter-off", "editorial-off", "role-error",
	} {
		fixture.addRole(t, "saltbox", role)
	}

	fixture.addDoc(t, "saltbox", "overview", validHealthFrontmatter("")+variablesMarkers)
	fixture.addDoc(t, "saltbox", "inventory-off", `---
saltbox_automation:
  sections:
    inventory: false
  project_description:
    name: Inventory Off
    summary: Inventory generation is disabled.
---
`+overviewMarkers)
	fixture.addDoc(t, "saltbox", "invalid", `---
saltbox_automation:
  app_links:
    - name: " "
      url: ""
  project_description:
    name: " "
    summary: ""
---
`+variablesMarkers+overviewMarkers)
	fixture.addDoc(t, "saltbox", "parse-error", "---\nsaltbox_automation: [\n---\n")
	fixture.addDoc(t, "saltbox", "outdated", validHealthFrontmatter("outdated")+variablesMarkers+overviewMarkers)
	fixture.addDoc(t, "saltbox", "draft", validHealthFrontmatter("draft")+variablesMarkers+overviewMarkers)
	fixture.addDoc(t, "saltbox", "page-disabled", `---
saltbox_automation:
  disabled: true
---
`)
	fixture.addDoc(t, "saltbox", "frontmatter-off", `---
saltbox_automation:
  checks:
    frontmatter: false
---
`+variablesMarkers+overviewMarkers)
	fixture.addDoc(t, "saltbox", "editorial-off", `---
status: outdated
saltbox_automation:
  checks:
    editorial: false
  project_description:
    name: Editorial Off
    summary: Editorial reporting is disabled.
---
`+variablesMarkers+overviewMarkers)
	fixture.addDoc(t, "saltbox", "role-error", validHealthFrontmatter("")+variablesMarkers+overviewMarkers)
	fixture.addDoc(t, "saltbox", "exact", "---\nsaltbox_automation: [\n---\n")
	fixture.addDoc(t, "sandbox", "orphan", `---
saltbox_automation:
  checks:
    coverage: false
  sections:
    overview: false
---
`)

	summary := github.NewUpdateSummary()
	summary.AddRole(github.RoleResult{
		Name:     "role-error",
		RepoType: "saltbox",
		Status:   github.StatusError,
		Error:    filepath.Join(fixture.root, "secret", "command --token issue-body"),
	})
	summary.AddRole(github.RoleResult{
		Name:       "missing",
		RepoType:   "saltbox",
		Status:     github.StatusSkipped,
		SkipReason: "doc file does not exist",
	})

	runner := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false)
	report, err := runner.buildHealthReport(t.Context(), fixture.cfg, summary, true, errors.New("/private/cli --secret issue-body"))
	if err != nil {
		t.Fatalf("buildHealthReport() error = %v", err)
	}

	assertHealthSubjects(t, report, health.MissingDocumentation, []string{"missing"})
	assertHealthSubjects(t, report, health.MissingVariablesSection, nil)
	assertHealthSubjects(t, report, health.MissingOverviewSection, []string{"overview"})
	assertHealthSubjects(t, report, health.OrphanedDocumentation, nil)
	assertHealthSubjects(t, report, health.InvalidFrontmatter, []string{"invalid", "parse-error"})
	assertHealthSubjects(t, report, health.EditorialAttention, []string{"outdated"})
	assertHealthSubjects(t, report, health.RoleAutomationError, []string{"role-error"})
	assertHealthSubjects(t, report, health.CLIHelpAutomationError, []string{"CLI help"})

	invalid := mustHealthResult(t, report, health.InvalidFrontmatter)
	if got, want := invalid.Findings[0].Code, "app_link_name_required,app_link_url_required,project_description_name_required,project_description_summary_required"; got != want {
		t.Fatalf("invalid frontmatter code = %q, want %q", got, want)
	}
	if got, want := invalid.Findings[0].Detail, "app_links[0].name is required; app_links[0].url is required; project_description.name is required; project_description.summary is required"; got != want {
		t.Fatalf("invalid frontmatter detail = %q, want %q", got, want)
	}
	if got := invalid.Findings[1].Code; got != "frontmatter_parse_error" {
		t.Fatalf("parse-error code = %q, want frontmatter_parse_error", got)
	}
	for _, kind := range []health.Kind{health.RoleAutomationError, health.CLIHelpAutomationError} {
		finding := mustHealthResult(t, report, kind).Findings[0]
		if strings.Contains(finding.Detail, fixture.root) || strings.Contains(finding.Detail, "--secret") || strings.Contains(finding.Detail, "issue-body") {
			t.Fatalf("%s detail leaked private input: %q", kind, finding.Detail)
		}
	}
	if got := mustHealthResult(t, report, health.MissingDocumentation).Exemptions; got != 1 {
		t.Fatalf("missing-documentation exemptions = %d, want 1 role blacklist", got)
	}
	if got := mustHealthResult(t, report, health.MissingVariablesSection).Exemptions; got != 4 {
		t.Fatalf("missing-variables exemptions = %d, want 4 path/page/check/section exemptions", got)
	}
	if got := mustHealthResult(t, report, health.MissingOverviewSection).Exemptions; got != 3 {
		t.Fatalf("missing-overview exemptions = %d, want 3 path/page/check exemptions", got)
	}
	if got := mustHealthResult(t, report, health.OrphanedDocumentation).Exemptions; got != 3 {
		t.Fatalf("orphaned-documentation exemptions = %d, want 3 path/page/check exemptions", got)
	}
	if got := mustHealthResult(t, report, health.InvalidFrontmatter).Exemptions; got != 4 {
		t.Fatalf("invalid-frontmatter exemptions = %d, want 4 path/page/check/section exemptions", got)
	}
	if got := mustHealthResult(t, report, health.EditorialAttention).Exemptions; got != 3 {
		t.Fatalf("editorial exemptions = %d, want 3 path/page/check exemptions", got)
	}
}

func TestHealthCheckPrecedenceKeepsMappingFactsAfterParseFailure(t *testing.T) {
	fixture := newHealthFixture(t)
	fixture.enableAllChecks()
	fixture.addRole(t, "saltbox", "broken")
	fixture.addDoc(t, "saltbox", "broken", "---\nsaltbox_automation: [\n---\n")

	report, err := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false).buildHealthReport(
		t.Context(), fixture.cfg, github.NewUpdateSummary(), false, nil,
	)
	if err != nil {
		t.Fatalf("buildHealthReport() error = %v", err)
	}

	assertHealthSubjects(t, report, health.MissingDocumentation, nil)
	assertHealthSubjects(t, report, health.OrphanedDocumentation, nil)
	assertHealthSubjects(t, report, health.MissingVariablesSection, nil)
	assertHealthSubjects(t, report, health.MissingOverviewSection, nil)
	assertHealthSubjects(t, report, health.InvalidFrontmatter, []string{"broken"})
}

func TestHealthOverrideTargetsRequireExistingSourceRole(t *testing.T) {
	tests := []struct {
		name        string
		addRole     bool
		wantOrphans []string
	}{
		{name: "existing source role maps override", addRole: true},
		{name: "stale source role leaves orphan", wantOrphans: []string{"overridden"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newHealthFixture(t)
			fixture.cfg.PathOverrides["saltbox"] = map[string]string{
				"overridden": "docs/reference/modules/overridden.md",
			}
			if tt.addRole {
				fixture.addRole(t, "saltbox", "overridden")
			}
			writeHealthFixtureFile(
				t,
				filepath.Join(fixture.cfg.Repositories.Docs, "docs", "reference", "modules", "overridden.md"),
				[]byte(validHealthFrontmatter("")+variablesMarkers+overviewMarkers),
			)

			report, err := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false).buildHealthReport(
				t.Context(), fixture.cfg, github.NewUpdateSummary(), false, nil,
			)
			if err != nil {
				t.Fatalf("buildHealthReport() error = %v", err)
			}
			assertHealthSubjects(t, report, health.OrphanedDocumentation, tt.wantOrphans)
		})
	}
}

func TestOrphanedDocumentationHasNoSourceRolePath(t *testing.T) {
	fixture := newHealthFixture(t)
	fixture.addDoc(t, "sandbox", "orphan", validHealthFrontmatter("")+variablesMarkers+overviewMarkers)

	report, err := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false).buildHealthReport(
		t.Context(), fixture.cfg, github.NewUpdateSummary(), false, nil,
	)
	if err != nil {
		t.Fatalf("buildHealthReport() error = %v", err)
	}
	orphans := mustHealthResult(t, report, health.OrphanedDocumentation).Findings
	if len(orphans) != 1 {
		t.Fatalf("orphan findings = %+v, want exactly 1", orphans)
	}
	if orphans[0].SourcePath != "" {
		t.Fatalf("orphan SourcePath = %q, want empty because no source role exists", orphans[0].SourcePath)
	}
}

func TestDiscoverHealthDocumentsSkipsBlacklistedCoverageOnlyParsing(t *testing.T) {
	tests := []struct {
		name               string
		frontmatterEnabled bool
		wantParsed         bool
	}{
		{name: "coverage only", wantParsed: false},
		{name: "frontmatter still requires parse", frontmatterEnabled: true, wantParsed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newHealthFixture(t)
			fixture.cfg.Blacklist.DocsCoverage.Saltbox = []string{"helper"}
			fixture.addRole(t, "saltbox", "helper")
			fixture.addDoc(t, "saltbox", "helper", "---\nsaltbox_automation: [\n---\n")
			blacklists := map[string]map[string]struct{}{
				"saltbox": {"helper": {}},
			}

			records, err := discoverHealthDocuments(
				t.Context(), fixture.cfg, map[string]roleTarget{}, blacklists, true, tt.frontmatterEnabled, false,
			)
			if err != nil {
				t.Fatalf("discoverHealthDocuments() error = %v", err)
			}
			if len(records) != 1 {
				t.Fatalf("record count = %d, want 1", len(records))
			}
			gotParsed := records[0].Document != nil || records[0].ParseError != nil
			if gotParsed != tt.wantParsed {
				t.Fatalf("parsed = %t, want %t; record = %+v", gotParsed, tt.wantParsed, records[0])
			}
		})
	}
}

func TestBuildHealthReportUsesUnifiedDefaultsAndCLIRequestState(t *testing.T) {
	fixture := newHealthFixture(t)

	tests := []struct {
		name         string
		cliRequested bool
		cliErr       error
		wantEnabled  bool
		wantFindings int
	}{
		{name: "requested success", cliRequested: true, wantEnabled: true},
		{name: "requested failure", cliRequested: true, cliErr: errors.New("failed"), wantEnabled: true, wantFindings: 1},
		{name: "not requested", cliErr: errors.New("must be ignored"), wantEnabled: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report, err := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false).buildHealthReport(
				t.Context(), fixture.cfg, github.NewUpdateSummary(), tt.cliRequested, tt.cliErr,
			)
			if err != nil {
				t.Fatalf("buildHealthReport() error = %v", err)
			}
			cli := mustHealthResult(t, report, health.CLIHelpAutomationError)
			if cli.Enabled != tt.wantEnabled || len(cli.Findings) != tt.wantFindings {
				t.Fatalf("CLI result = %+v, want enabled=%t findings=%d", cli, tt.wantEnabled, tt.wantFindings)
			}
			if result := mustHealthResult(t, report, health.RoleAutomationError); !result.Enabled {
				t.Fatal("role automation result is disabled")
			}
			for _, kind := range []health.Kind{
				health.MissingDocumentation,
				health.MissingVariablesSection,
				health.MissingOverviewSection,
				health.OrphanedDocumentation,
			} {
				if result := mustHealthResult(t, report, kind); !result.Enabled {
					t.Fatalf("default coverage result %s is disabled", kind)
				}
			}
			for _, kind := range []health.Kind{health.InvalidFrontmatter, health.EditorialAttention} {
				result := mustHealthResult(t, report, kind)
				if result.Enabled || len(result.Findings) != 0 {
					t.Fatalf("default-disabled result %s = %+v", kind, result)
				}
			}
		})
	}
}

type healthFixture struct {
	root string
	cfg  *config.Config
}

func newHealthFixture(t *testing.T) healthFixture {
	t.Helper()
	root := t.TempDir()
	saltbox := filepath.Join(root, "saltbox")
	sandbox := filepath.Join(root, "sandbox")
	docs := filepath.Join(root, "docs")
	for _, path := range []string{
		filepath.Join(saltbox, "roles"),
		filepath.Join(sandbox, "roles"),
		filepath.Join(docs, "docs", "apps"),
		filepath.Join(docs, "docs", "sandbox", "apps"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("creating fixture directory: %v", err)
		}
	}
	return healthFixture{
		root: root,
		cfg: &config.Config{
			Repositories:  config.RepositoryConfig{Saltbox: saltbox, Sandbox: sandbox, Docs: docs},
			PathOverrides: map[string]map[string]string{},
			Markers: config.MarkersConfig{
				Variables: "SALTBOX MANAGED VARIABLES SECTION",
				Overview:  "SALTBOX MANAGED OVERVIEW SECTION",
			},
		},
	}
}

func (f healthFixture) enableAllChecks() {
	yes := true
	f.cfg.Checks.Coverage.Enabled = &yes
	f.cfg.Checks.Frontmatter.Enabled = &yes
	f.cfg.Checks.Editorial.Enabled = &yes
	f.cfg.Checks.Editorial.Statuses = []string{"draft2", "outdated"}
}

func (f healthFixture) addRole(t *testing.T, repository, role string) {
	t.Helper()
	root := f.cfg.Repositories.Saltbox
	if repository == "sandbox" {
		root = f.cfg.Repositories.Sandbox
	}
	path := filepath.Join(root, "roles", role, "defaults", "main.yml")
	writeHealthFixtureFile(t, path, []byte("example: true\n"))
}

func (f healthFixture) addDoc(t *testing.T, repository, role, content string) {
	t.Helper()
	root := f.cfg.SaltboxDocsPath()
	if repository == "sandbox" {
		root = f.cfg.SandboxDocsPath()
	}
	writeHealthFixtureFile(t, filepath.Join(root, role+".md"), []byte(content))
}

func writeHealthFixtureFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating fixture parent: %v", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
}

func validHealthFrontmatter(status string) string {
	statusLine := ""
	if status != "" {
		statusLine = "status: " + status + "\n"
	}
	return "---\n" + statusLine + `saltbox_automation:
  project_description:
    name: Valid
    summary: Valid summary.
---
`
}

const variablesMarkers = `<!-- BEGIN SALTBOX MANAGED VARIABLES SECTION -->
<!-- END SALTBOX MANAGED VARIABLES SECTION -->
`

const overviewMarkers = `<!-- BEGIN SALTBOX MANAGED OVERVIEW SECTION -->
<!-- END SALTBOX MANAGED OVERVIEW SECTION -->
`

func assertHealthSubjects(t *testing.T, report health.Report, kind health.Kind, want []string) {
	t.Helper()
	result := mustHealthResult(t, report, kind)
	got := make([]string, 0, len(result.Findings))
	for _, finding := range result.Findings {
		got = append(got, finding.Subject)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("%s subjects = %v, want %v", kind, got, want)
	}
}

func mustHealthResult(t *testing.T, report health.Report, kind health.Kind) health.Result {
	t.Helper()
	result, ok := report.Result(kind)
	if !ok {
		t.Fatalf("report has no %s result", kind)
	}
	return result
}
