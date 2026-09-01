package health

import (
	"testing"
)

func TestReportCanonicalSortsResultsAndFindingsAndReportsTotals(t *testing.T) {
	report := NewReport([]Result{{
		Kind:    MissingDocumentation,
		Enabled: true,
		Findings: []Finding{
			{Kind: MissingDocumentation, Repository: "sandbox", Subject: "zeta", Code: "missing_doc"},
			{Kind: MissingDocumentation, Repository: "saltbox", Subject: "alpha", Code: "missing_doc"},
		},
	}}, RunInfo{})

	got := report.Canonical()
	missing, ok := got.Result(MissingDocumentation)
	if !ok || missing.Findings[0].Subject != "alpha" {
		t.Fatalf("first missing-documentation subject = %q", missing.Findings[0].Subject)
	}
	if got.Total() != 2 || got.TotalSeverity(Error) != 2 || !got.HasFindings() {
		t.Fatalf("unexpected totals: %+v", got)
	}

	wantKinds := []Kind{
		RoleAutomationError,
		CLIHelpAutomationError,
		MissingDocumentation,
		InvalidFrontmatter,
		MissingVariablesSection,
		MissingOverviewSection,
		OrphanedDocumentation,
		EditorialAttention,
	}
	if len(got.Results) != len(wantKinds) {
		t.Fatalf("result count = %d, want %d", len(got.Results), len(wantKinds))
	}
	for i, want := range wantKinds {
		if got.Results[i].Kind != want {
			t.Errorf("result %d kind = %q, want %q", i, got.Results[i].Kind, want)
		}
	}
}

func TestReportCanonicalDoesNotMutateInput(t *testing.T) {
	input := []Result{{
		Kind: MissingDocumentation,
		Findings: []Finding{
			{Kind: MissingDocumentation, Subject: "zeta"},
			{Kind: MissingDocumentation, Subject: "alpha"},
		},
	}}

	_ = NewReport(input, RunInfo{}).Canonical()
	if input[0].Findings[0].Subject != "zeta" {
		t.Fatalf("canonicalization mutated input order: %+v", input[0].Findings)
	}
}

func TestFindingSeverityIdentityAndLabel(t *testing.T) {
	if EditorialAttention.Severity() != Notice {
		t.Fatalf("editorial severity = %q, want %q", EditorialAttention.Severity(), Notice)
	}
	for _, kind := range []Kind{
		MissingDocumentation,
		MissingVariablesSection,
		MissingOverviewSection,
		OrphanedDocumentation,
		InvalidFrontmatter,
		RoleAutomationError,
		CLIHelpAutomationError,
	} {
		if kind.Severity() != Error {
			t.Errorf("%q severity = %q, want %q", kind, kind.Severity(), Error)
		}
	}

	finding := Finding{
		Kind:       MissingDocumentation,
		Repository: "saltbox",
		Subject:    "radarr",
		Path:       "roles/radarr.md",
		SourcePath: "roles/radarr",
		Code:       "missing_doc",
		Detail:     "first detail",
	}
	if finding.Label() != "radarr" {
		t.Fatalf("label = %q, want subject", finding.Label())
	}
	if finding.ID() != (Finding{Kind: MissingDocumentation, Repository: "saltbox", Subject: "radarr", Path: "roles/radarr.md", SourcePath: "roles/radarr", Code: "missing_doc", Detail: "second detail"}).ID() {
		t.Fatal("changing detail changed finding identity")
	}
	for name, changed := range map[string]Finding{
		"kind":       {Kind: OrphanedDocumentation, Repository: finding.Repository, Subject: finding.Subject, Path: finding.Path, SourcePath: finding.SourcePath, Code: finding.Code, Detail: finding.Detail},
		"repository": {Kind: finding.Kind, Repository: "sandbox", Subject: finding.Subject, Path: finding.Path, SourcePath: finding.SourcePath, Code: finding.Code, Detail: finding.Detail},
		"subject":    {Kind: finding.Kind, Repository: finding.Repository, Subject: "sonarr", Path: finding.Path, SourcePath: finding.SourcePath, Code: finding.Code, Detail: finding.Detail},
		"path":       {Kind: finding.Kind, Repository: finding.Repository, Subject: finding.Subject, Path: "other.md", SourcePath: finding.SourcePath, Code: finding.Code, Detail: finding.Detail},
		"code":       {Kind: finding.Kind, Repository: finding.Repository, Subject: finding.Subject, Path: finding.Path, SourcePath: finding.SourcePath, Code: "other", Detail: finding.Detail},
	} {
		if changed.ID() == finding.ID() {
			t.Errorf("changing %s did not change finding identity", name)
		}
	}

	if (Finding{Path: "docs/page.md"}).Label() != "docs/page.md" {
		t.Fatal("path should be the fallback label")
	}
	if (Finding{SourcePath: "roles/foo"}).Label() != "roles/foo" {
		t.Fatal("source path should be the fallback label")
	}
	if (Finding{Code: "invalid"}).Label() != "invalid" {
		t.Fatal("code should be the final fallback label")
	}
}

func TestReportResultLookupAndSeverityTotals(t *testing.T) {
	report := NewReport([]Result{
		{Kind: EditorialAttention, Enabled: true, Findings: []Finding{{Kind: EditorialAttention, Subject: "draft"}}},
		{Kind: MissingDocumentation, Enabled: false, Exemptions: 3},
	}, RunInfo{})

	result, ok := report.Result(EditorialAttention)
	if !ok || len(result.Findings) != 1 {
		t.Fatalf("editorial result = %+v, found = %t", result, ok)
	}
	if report.TotalSeverity(Notice) != 1 || report.TotalSeverity(Error) != 0 {
		t.Fatalf("severity totals = error:%d notice:%d", report.TotalSeverity(Error), report.TotalSeverity(Notice))
	}
	if _, ok := report.Result(Kind("unknown")); ok {
		t.Fatal("unknown result unexpectedly found")
	}
}
