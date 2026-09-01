// Package health contains the pure data model for documentation health.
package health

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"time"
)

// Severity describes how urgently a finding needs attention.
type Severity string

const (
	Error  Severity = "error"
	Notice Severity = "notice"
)

// Kind identifies a documentation-health check.
type Kind string

const (
	MissingDocumentation    Kind = "missing_documentation"
	MissingVariablesSection Kind = "missing_variables_section"
	MissingOverviewSection  Kind = "missing_overview_section"
	OrphanedDocumentation   Kind = "orphaned_documentation"
	InvalidFrontmatter      Kind = "invalid_frontmatter"
	EditorialAttention      Kind = "editorial_attention"
	RoleAutomationError     Kind = "role_automation_error"
	CLIHelpAutomationError  Kind = "cli_help_automation_error"
)

var resultOrder = []Kind{
	RoleAutomationError,
	CLIHelpAutomationError,
	MissingDocumentation,
	InvalidFrontmatter,
	MissingVariablesSection,
	MissingOverviewSection,
	OrphanedDocumentation,
	EditorialAttention,
}

// Severity returns the severity associated with a check kind.
func (k Kind) Severity() Severity {
	if k == EditorialAttention {
		return Notice
	}
	return Error
}

// Finding is one actionable health diagnostic.
type Finding struct {
	Kind       Kind
	Repository string
	Subject    string
	Path       string
	SourcePath string
	Code       string
	Detail     string
}

// ID returns a stable identity for the semantic part of a finding.
//
// Detail and SourcePath are deliberately excluded: changing presentation or
// the source-side path must not turn an existing semantic finding into a new
// one. NUL separators make the component boundaries unambiguous.
func (f Finding) ID() string {
	identity := strings.Join([]string{
		string(f.Kind),
		f.Repository,
		f.Subject,
		f.Path,
		f.Code,
	}, "\x00")
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:])
}

// Label returns the most useful short display label for a finding.
func (f Finding) Label() string {
	if f.Subject != "" {
		return f.Subject
	}
	if f.Path != "" {
		return f.Path
	}
	if f.SourcePath != "" {
		return f.SourcePath
	}
	return f.Code
}

// Result contains the findings and exemptions for one check kind.
type Result struct {
	Kind       Kind
	Enabled    bool
	Exemptions int
	Findings   []Finding
}

// SourceRevision identifies the revision used for source links.
type SourceRevision struct {
	Repository string
	Slug       string
	Ref        string
	Revision   string
}

// RunInfo contains non-semantic provenance for a report.
type RunInfo struct {
	CheckedAt   time.Time
	WorkflowURL string
	Branch      string
	Version     string
	Sources     []SourceRevision
}

// Report is the structured result of all health checks.
type Report struct {
	Results []Result
	Run     RunInfo
}

// NewReport creates a report with exactly one row for every defined kind.
// Duplicate input rows are combined so callers can collect findings in stages.
func NewReport(results []Result, run RunInfo) Report {
	byKind := make(map[Kind]Result, len(results))
	for _, input := range results {
		result := byKind[input.Kind]
		result.Kind = input.Kind
		result.Enabled = result.Enabled || input.Enabled
		result.Exemptions += input.Exemptions
		for _, finding := range input.Findings {
			if finding.Kind == "" {
				finding.Kind = input.Kind
			}
			result.Findings = append(result.Findings, finding)
		}
		byKind[input.Kind] = result
	}

	ordered := make([]Result, 0, len(resultOrder)+len(byKind))
	seen := make(map[Kind]bool, len(resultOrder))
	for _, kind := range resultOrder {
		result := byKind[kind]
		result.Kind = kind
		ordered = append(ordered, result)
		seen[kind] = true
	}
	unknown := make([]Kind, 0, len(byKind))
	for kind := range byKind {
		if !seen[kind] {
			unknown = append(unknown, kind)
		}
	}
	slices.Sort(unknown)
	for _, kind := range unknown {
		result := byKind[kind]
		result.Kind = kind
		ordered = append(ordered, result)
	}

	return (Report{Results: ordered, Run: cloneRunInfo(run)}).Canonical()
}

// Canonical returns a sorted deep copy suitable for comparison and encoding.
func (r Report) Canonical() Report {
	clone := Report{Run: cloneRunInfo(r.Run), Results: make([]Result, len(r.Results))}
	copy(clone.Results, r.Results)
	for i := range clone.Results {
		clone.Results[i].Findings = slices.Clone(clone.Results[i].Findings)
	}
	slices.SortFunc(clone.Results, func(a, b Result) int {
		return compareKinds(a.Kind, b.Kind)
	})
	for i := range clone.Results {
		slices.SortFunc(clone.Results[i].Findings, func(a, b Finding) int {
			return compareFindings(a, b)
		})
	}
	return clone
}

// HasFindings reports whether an enabled check has at least one finding.
func (r Report) HasFindings() bool {
	for _, result := range r.Results {
		if result.Enabled && len(result.Findings) > 0 {
			return true
		}
	}
	return false
}

// Total returns the number of findings in enabled checks.
func (r Report) Total() int {
	total := 0
	for _, result := range r.Results {
		if result.Enabled {
			total += len(result.Findings)
		}
	}
	return total
}

// TotalSeverity returns the number of findings with the requested severity.
func (r Report) TotalSeverity(severity Severity) int {
	total := 0
	for _, result := range r.Results {
		if result.Enabled && result.Kind.Severity() == severity {
			total += len(result.Findings)
		}
	}
	return total
}

// Result looks up a result by kind.
func (r Report) Result(kind Kind) (Result, bool) {
	for _, result := range r.Results {
		if result.Kind == kind {
			return result, true
		}
	}
	return Result{}, false
}

func cloneRunInfo(run RunInfo) RunInfo {
	run.Sources = slices.Clone(run.Sources)
	return run
}

func compareKinds(a, b Kind) int {
	ai, aKnown := kindIndex(a)
	bi, bKnown := kindIndex(b)
	if aKnown && bKnown {
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
		return 0
	}
	if aKnown {
		return -1
	}
	if bKnown {
		return 1
	}
	return strings.Compare(string(a), string(b))
}

func kindIndex(kind Kind) (int, bool) {
	for index, known := range resultOrder {
		if kind == known {
			return index, true
		}
	}
	return 0, false
}

func compareFindings(a, b Finding) int {
	for _, pair := range [][2]string{
		{a.Repository, b.Repository},
		{a.Subject, b.Subject},
		{a.Path, b.Path},
		{a.SourcePath, b.SourcePath},
		{a.Code, b.Code},
		{a.Detail, b.Detail},
	} {
		if comparison := strings.Compare(pair[0], pair[1]); comparison != 0 {
			return comparison
		}
	}
	return strings.Compare(string(a.Kind), string(b.Kind))
}
