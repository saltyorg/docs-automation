package health

import (
	"reflect"
	"testing"
	"time"
)

func TestReportStateContainsOnlyCanonicalSemanticData(t *testing.T) {
	old := NewReport([]Result{{
		Kind:       MissingDocumentation,
		Enabled:    true,
		Exemptions: 2,
		Findings: []Finding{{
			Kind:       MissingDocumentation,
			Repository: "saltbox",
			Subject:    "alpha",
			SourcePath: "/source/alpha",
			Code:       "missing_doc",
			Detail:     "old detail",
		}},
	}}, RunInfo{CheckedAt: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC), WorkflowURL: "https://example.test/old"}).State()
	new := NewReport([]Result{{
		Kind:       MissingDocumentation,
		Enabled:    true,
		Exemptions: 2,
		Findings:   []Finding{{Kind: MissingDocumentation, Repository: "saltbox", Subject: "alpha", SourcePath: "/different/source", Code: "missing_doc", Detail: "new detail"}},
	}}, RunInfo{CheckedAt: time.Date(2026, 9, 1, 11, 0, 0, 0, time.UTC), WorkflowURL: "https://example.test/new"}).State()

	if !reflect.DeepEqual(old, new) {
		t.Fatalf("run metadata or detail changed semantic state:\nold=%+v\nnew=%+v", old, new)
	}
	if old.Version != StateVersion {
		t.Fatalf("state version = %d, want %d", old.Version, StateVersion)
	}
	result, ok := stateResult(old, MissingDocumentation)
	if !ok || len(result.Findings) != 1 || result.Findings[0].Label != "alpha" {
		t.Fatalf("state result = %+v, found = %t", result, ok)
	}
}

func TestDiffReportsAddedResolvedAndChangedResultsDeterministically(t *testing.T) {
	old := NewReport([]Result{{
		Kind:     MissingDocumentation,
		Enabled:  true,
		Findings: []Finding{{Kind: MissingDocumentation, Subject: "alpha", Code: "missing_doc"}},
	}}, RunInfo{}).State()
	new := NewReport([]Result{{
		Kind:       MissingDocumentation,
		Enabled:    false,
		Exemptions: 3,
		Findings:   []Finding{{Kind: MissingDocumentation, Subject: "beta", Code: "missing_doc"}},
	}}, RunInfo{}).State()

	changes := Diff(old, new)
	if len(changes.Added) != 1 || changes.Added[0].Label != "beta" {
		t.Fatalf("added = %+v", changes.Added)
	}
	if len(changes.Resolved) != 1 || changes.Resolved[0].Label != "alpha" {
		t.Fatalf("resolved = %+v", changes.Resolved)
	}
	if !reflect.DeepEqual(changes.ChangedResults, []Kind{MissingDocumentation}) {
		t.Fatalf("changed results = %+v", changes.ChangedResults)
	}
}

func TestDiffIgnoresRunMetadataBecauseStateHasNone(t *testing.T) {
	first := NewReport(nil, RunInfo{WorkflowURL: "https://example.test/one"}).State()
	second := NewReport(nil, RunInfo{WorkflowURL: "https://example.test/two"}).State()

	changes := Diff(first, second)
	if len(changes.Added) != 0 || len(changes.Resolved) != 0 || len(changes.ChangedResults) != 0 {
		t.Fatalf("metadata-only changes = %+v", changes)
	}
}

func stateResult(state State, kind Kind) (StateResult, bool) {
	for _, result := range state.Results {
		if result.Kind == kind {
			return result, true
		}
	}
	return StateResult{}, false
}
