# Docs Health Issue Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an actionable, structured, opt-out-aware documentation-health report and use it to render the single managed GitHub issue without forcing a universal page format.

**Architecture:** A new pure health package owns finding identity, severity, canonical state, totals, and semantic diffs. Automation applies configuration/frontmatter precedence and produces one report; the GitHub package renders and manages that report without parsing its own Markdown. Existing controls remain authoritative, with exact-path and check-specific opt-outs added at their owning seams.

**Tech Stack:** Go 1.27, Cobra, go.yaml.in/yaml/v3, Go standard-library JSON/gzip/base64/crypto packages, the existing injected GitHub CLI command runner, Make, and GitHub Actions.

**Spec:** .agents/specs/2026-09-01-docs-health-issue-design.md

## Global Constraints

- Preserve every existing uncommitted code-quality-hardening change in /opt/git/docs-automation; this plan builds on that worktree.
- Do not commit, push, tag, release, create or switch branches, or mutate Git credentials without separate authorization.
- Do not call live GitHub issue mutation commands during development or verification.
- Do not repair Markdown pages, frontmatter records, templates, navigation, or generated sections in /opt/git/docs.
- The only permitted Docs repository edit is /opt/git/docs/.docs-automation.yml.
- Do not edit /srv/git/saltbox or /opt/sandbox; both contain unrelated user changes.
- Preserve update --check, --manage-issue, path-only overlays, and the all-role warning/exit policy.
- Missing optional metadata is valid; supplied data for an enabled feature must be internally valid.
- Apply opt-out precedence exactly as specified.
- Use red-green-refactor for every behavior change.
- Use review checkpoints instead of commits.

---

### Task 1: Add Global and Page-Level Check Controls

**Files:**
- Modify: config/config.go
- Modify: config/config_test.go
- Modify: document/frontmatter.go
- Modify: document/frontmatter_test.go

**Interfaces:**
- Produces config.CheckConfig, config.ChecksConfig, config.IssueConfig, and config.SourceRepositoryConfig.
- Produces CheckConfig.EnabledOr(defaultValue bool) bool and CheckConfig.Excludes(relPath string) bool.
- Produces document.PageChecks and IsCoverageCheckEnabled, IsFrontmatterCheckEnabled, and IsEditorialCheckEnabled.
- Tasks 3 through 8 consume these exact interfaces.

- [x] **Step 1: Write failing configuration parsing tests**

Add table-driven tests that load:

~~~yaml
checks:
  coverage:
    enabled: true
    exclude_paths:
      - docs/apps/lean.md
  frontmatter:
    enabled: false
  editorial:
    enabled: true
    statuses: [draft2, outdated]
issue:
  source_repositories:
    saltbox:
      slug: saltyorg/Saltbox
      ref: master
~~~

Assert enabled/default behavior, normalized exclusions, editorial statuses, and source metadata. Add rejection cases for absolute paths, parent escapes, normalized duplicates, statuses on non-editorial checks, enabled editorial reporting without statuses, invalid owner/repository slugs, and missing refs.

- [x] **Step 2: Run the focused tests and observe failure**

~~~bash
go test ./config -run 'TestLoadChecks|TestValidateCheck|TestValidateIssue' -count=1
~~~

Expected: compile failure because the new configuration interface does not exist.

- [x] **Step 3: Implement configuration types and methods**

Add these fields to Config:

~~~go
Checks ChecksConfig
Issue  IssueConfig
~~~

Define:

~~~go
type CheckConfig struct {
    Enabled      *bool
    ExcludePaths []string
    Statuses     []string
}

type ChecksConfig struct {
    Coverage    CheckConfig
    Frontmatter CheckConfig
    Editorial   CheckConfig
}

type IssueConfig struct {
    SourceRepositories map[string]SourceRepositoryConfig
}

type SourceRepositoryConfig struct {
    Slug string
    Ref  string
}

func (c CheckConfig) EnabledOr(defaultValue bool) bool {
    if c.Enabled == nil {
        return defaultValue
    }
    return *c.Enabled
}

func (c CheckConfig) Excludes(relPath string) bool {
    clean := filepath.ToSlash(filepath.Clean(relPath))
    return slices.Contains(c.ExcludePaths, clean)
}
~~~

Apply YAML field tags in config/config.go exactly as named in the approved schema. Add validateCheckConfig and validateIssueConfig calls from Config.Validate. Normalize paths in place and preserve path-only overlay inheritance.

Defaults used by unified checks are coverage=true, frontmatter=false, editorial=false. The canonical config explicitly overrides all three.

- [x] **Step 4: Write failing page-precedence tests**

Add tests that parse all three page check fields and assert:

~~~go
no := false
sa := &SaltboxAutomationConfig{
    Checks: PageChecks{Frontmatter: &no},
}
if sa.IsFrontmatterCheckEnabled() {
    t.Fatal("frontmatter check should be disabled")
}
if !sa.IsCoverageCheckEnabled() || !sa.IsEditorialCheckEnabled() {
    t.Fatal("unspecified checks should inherit enabled state")
}

sa.Disabled = true
if sa.IsCoverageCheckEnabled() ||
    sa.IsFrontmatterCheckEnabled() ||
    sa.IsEditorialCheckEnabled() {
    t.Fatal("page-wide disable must win")
}
~~~

- [x] **Step 5: Run the document tests and observe failure**

~~~bash
go test ./document -run 'TestSaltboxAutomationCheckControls|TestParseFrontmatter' -count=1
~~~

Expected: compile failure because PageChecks and the predicates do not exist.

- [x] **Step 6: Implement page controls**

Add Status string to Frontmatter with the YAML field name status. Add PageChecks to SaltboxAutomationConfig with coverage, frontmatter, and editorial pointer booleans. Implement the three predicates through one private helper:

~~~go
func pageCheckEnabled(
    c *SaltboxAutomationConfig,
    selectCheck func(PageChecks) *bool,
) bool {
    if c == nil {
        return true
    }
    if c.Disabled {
        return false
    }
    value := selectCheck(c.Checks)
    return value == nil || *value
}
~~~

- [x] **Step 7: Run package tests and checkpoint**

~~~bash
gofmt -w config/config.go config/config_test.go document/frontmatter.go document/frontmatter_test.go
go test ./config ./document -count=1
git diff --check -- config document
git status --short -- config document
~~~

Expected: PASS. Confirm only intended files plus pre-existing changes. Do not commit.

---

### Task 2: Create the Structured Health Report Module

**Files:**
- Create: health/report.go
- Create: health/report_test.go
- Create: health/state.go
- Create: health/state_test.go

**Interfaces:**
- Produces health.Kind, health.Severity, health.Finding, health.Result, health.RunInfo, health.SourceRevision, and health.Report.
- Produces NewReport, Report.Canonical, Report.HasFindings, Report.Total, Report.TotalSeverity, Report.Result, Report.State, and health.Diff.
- Automation produces these values; GitHub issue and summary modules consume them.

- [x] **Step 1: Write failing report tests**

Test deterministic result/finding sorting, error and notice totals, all-kind row creation, and stable identity. The main fixture is:

~~~go
report := NewReport([]Result{{
    Kind:    MissingDocumentation,
    Enabled: true,
    Findings: []Finding{
        {Kind: MissingDocumentation, Repository: "sandbox", Subject: "zeta", Code: "missing_doc"},
        {Kind: MissingDocumentation, Repository: "saltbox", Subject: "alpha", Code: "missing_doc"},
    },
}}, RunInfo{})

got := report.Canonical()
if got.Results[0].Findings[0].Subject != "alpha" {
    t.Fatalf("first subject = %q", got.Results[0].Findings[0].Subject)
}
if got.Total() != 2 || got.TotalSeverity(Error) != 2 || !got.HasFindings() {
    t.Fatalf("unexpected totals: %+v", got)
}
~~~

Assert EditorialAttention is Notice severity and all other defined finding kinds are Error. Detail changes must not change Finding.ID; kind, repository, subject, path, or code changes must.

- [x] **Step 2: Run and observe failure**

~~~bash
go test ./health -count=1
~~~

Expected: failure because the package does not exist.

- [x] **Step 3: Implement report types**

Define these severities and kinds:

~~~go
type Severity string

const (
    Error  Severity = "error"
    Notice Severity = "notice"
)

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
~~~

Define Finding with Kind, Repository, Subject, Path, SourcePath, Code, and Detail strings. Define Result with Kind, Enabled, Exemptions, and Findings. Define SourceRevision with Repository, Slug, Ref, and Revision. Define RunInfo with CheckedAt, WorkflowURL, Branch, Version, and Sources. Define Report with Results and Run.

Implement Kind.Severity, SHA-256-based Finding.ID, Finding.Label, clone-before-sort canonicalization using slices.SortFunc, and all total/lookup methods. NewReport must ensure every defined kind appears once so passed and disabled rows remain visible. Canonical result order is role automation errors, CLI-help automation errors, missing documentation, invalid frontmatter, missing variables sections, missing overview sections, orphaned documentation, then editorial attention.

- [x] **Step 4: Write failing semantic-state tests**

Define a test where alpha is replaced by beta and only the workflow URL changes:

~~~go
changes := Diff(oldReport.State(), newReport.State())
if len(changes.Added) != 1 || changes.Added[0].Label != "beta" {
    t.Fatalf("added = %+v", changes.Added)
}
if len(changes.Resolved) != 1 || changes.Resolved[0].Label != "alpha" {
    t.Fatalf("resolved = %+v", changes.Resolved)
}
~~~

Also assert enabled-state and exemption-count changes appear in ChangedResults without treating run metadata as semantic.

- [x] **Step 5: Run and observe failure**

~~~bash
go test ./health -run 'TestDiff|TestState' -count=1
~~~

Expected: compile failure because state interfaces do not exist.

- [x] **Step 6: Implement state and diff**

Define:

~~~go
const StateVersion = 1

type StateFinding struct {
    ID    string
    Kind  Kind
    Label string
}

type StateResult struct {
    Kind       Kind
    Enabled    bool
    Exemptions int
    Findings   []StateFinding
}

type State struct {
    Version int
    Results []StateResult
}

type Changes struct {
    Added          []StateFinding
    Resolved       []StateFinding
    ChangedResults []Kind
}
~~~

Apply JSON tags in health/state.go. Report.State derives only from canonical results and excludes RunInfo and Detail. Diff compares IDs using maps and sorts all output deterministically.

- [x] **Step 7: Run tests and checkpoint**

~~~bash
gofmt -w health
go test ./health -count=1
git diff --check -- health
git status --short -- health
~~~

Expected: PASS. Do not commit.

---

### Task 3: Centralize Conditional Frontmatter Validation

**Files:**
- Create: document/validation.go
- Create: document/validation_test.go
- Modify: automation/validate.go

**Interfaces:**
- Consumes page predicates from Task 1.
- Produces document.Diagnostic and document.ValidateAutomationFrontmatter(*Frontmatter) []Diagnostic.
- The standalone validator and Task 4 consume this function.

- [x] **Step 1: Write failing semantic-validation tests**

Cover nil frontmatter, no automation block, page disabled, frontmatter opt-out, overview disabled, omitted links, incomplete supplied links, missing description, blank description fields, and complete lean/full pages.

Required cases include:

~~~go
no := false
fm := &Frontmatter{SaltboxAutomation: &SaltboxAutomationConfig{
    Sections: SectionsConfig{Overview: &no},
    AppLinks: []AppLink{{Name: "Manual"}},
}}
if got := ValidateAutomationFrontmatter(fm); len(got) != 0 {
    t.Fatalf("diagnostics = %+v, want none", got)
}
~~~

and a fully enabled invalid overview returning these codes:

~~~text
app_link_name_required
app_link_url_required
project_description_name_required
project_description_summary_required
~~~

- [x] **Step 2: Run and observe failure**

~~~bash
go test ./document -run 'TestValidateAutomationFrontmatter' -count=1
~~~

Expected: compile failure.

- [x] **Step 3: Implement the pure validator**

Define Diagnostic with Code and Message. ValidateAutomationFrontmatter returns nil for nil frontmatter, no automation block, page/frontmatter disable, or overview disable. Otherwise:

- validate every supplied link's trimmed name and URL;
- require ProjectDescription;
- require trimmed project name and summary;
- leave type, link, categories, scheme, and reachability open;
- return all diagnostics in deterministic order.

Use stable codes from the tests and messages that identify the exact field/index.

- [x] **Step 4: Refactor the standalone validator**

Replace the automation-local validator. The command must:

1. Compute Docs-relative slash paths.
2. Apply frontmatter exclude_paths before parsing.
3. Parse frontmatter.
4. Count missing frontmatter separately.
5. Treat page disable/check disable as excluded.
6. Print every diagnostic for an invalid document.
7. Count one invalid file regardless of diagnostic count.

Print:

~~~text
Validation complete: N valid, N invalid, N without frontmatter, N excluded
~~~

Return found N invalid files only when invalid is nonzero. Explicit validate frontmatter runs even when unified frontmatter checking is globally disabled, but honors all path/page/section opt-outs.

- [x] **Step 5: Run tests and checkpoint**

~~~bash
gofmt -w document/validation.go document/validation_test.go automation/validate.go
go test ./document ./automation -count=1
git diff --check -- document automation/validate.go
git status --short -- document automation/validate.go
~~~

Expected: PASS. Do not commit.

---

### Task 4: Collect Coverage, Metadata, Editorial, and Automation Findings

**Files:**
- Create: automation/health.go
- Create: automation/health_test.go
- Create: automation/provenance.go
- Create: automation/provenance_test.go
- Modify: automation/runner.go
- Modify: automation/update.go

**Interfaces:**
- Consumes config controls, document diagnostics, health types, github.UpdateSummary, and buildinfo.VersionString.
- Produces Runner.buildHealthReport(context.Context, *config.Config, *github.UpdateSummary, bool, error) (health.Report, error), where the bool records whether CLI generation was requested.
- Produces an internal revisionResolver function seam.

- [x] **Step 1: Write failing collector tests**

Build temporary Saltbox, Sandbox, and Docs fixtures containing:

- one missing Saltbox role;
- one blacklisted helper role;
- one orphan page with coverage disabled;
- one page with inventory disabled and no variables marker;
- one enabled overview missing its marker;
- one invalid frontmatter page;
- one outdated page;
- one ordinary draft page;
- one exact path exclusion.

Assert exact subjects for MissingDocumentation, MissingOverviewSection, InvalidFrontmatter, and EditorialAttention. Assert blacklist/path/page/section exemption counts and disabled-category rows.

- [x] **Step 2: Run and observe failure**

~~~bash
go test ./automation -run 'TestBuildHealthReport|TestHealthCheckPrecedence' -count=1
~~~

Expected: compile failure because buildHealthReport does not exist.

- [x] **Step 3: Implement one-pass document discovery**

Create:

~~~go
type docRecord struct {
    Repository string
    Role       string
    Path       string
    Relative   string
    Document   *document.Document
    ParseError error
}

func (r *Runner) buildHealthReport(
    ctx context.Context,
    cfg *config.Config,
    summary *github.UpdateSummary,
    cliRequested bool,
    cliErr error,
) (health.Report, error)
~~~

The collector must:

- list roles and Docs files with contextual fatal errors;
- build stable role/doc maps and override targets;
- read every document's bytes once, compute category path exclusions, and parse frontmatter at most once only when at least one non-excluded enabled check needs it;
- create all four coverage result kinds when coverage is enabled;
- apply role blacklists before missing-document findings;
- apply global, exact-path, page-wide, check-specific, and section precedence;
- preserve role-to-doc mapping checks when semantic validation is excluded;
- skip marker checks after a parse error but report mapping facts;
- group document diagnostics into one InvalidFrontmatter finding;
- report only configured editorial statuses;
- count unique exemptions per result;
- canonicalize through health.NewReport.

RoleAutomationError is enabled for every all-role update. CLIHelpAutomationError is enabled only when CLI generation was requested. The four coverage kinds inherit checks.coverage; InvalidFrontmatter inherits checks.frontmatter; EditorialAttention inherits checks.editorial. Disabled kinds contain no findings and retain Enabled=false for presentation.

Join sorted diagnostic codes for a stable finding Code and join messages with a semicolon for Detail.

- [x] **Step 4: Add role and CLI errors without duplication**

Add RoleAutomationError only for StatusError. Never duplicate the expected doc file does not exist skip. Add CLIHelpAutomationError only when CLI generation was requested and returned an error.

Public details must identify operation, repository, and subject but omit command arguments, issue bodies, and absolute runner paths.

- [x] **Step 5: Write failing provenance tests**

Inject fixed revisions for Saltbox/Sandbox and an error for Docs. Assert configured slug/ref and available revisions appear, and missing Git metadata does not fail checks.

- [x] **Step 6: Run and observe failure**

~~~bash
go test ./automation -run 'TestHealthProvenance' -count=1
~~~

Expected: failure because Runner has no resolver.

- [x] **Step 7: Implement provenance**

Extend Runner with:

~~~go
type revisionResolver func(context.Context, string) (string, error)

resolveRevision revisionResolver
~~~

NewRunner assigns gitRevision. gitRevision runs:

~~~go
exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "HEAD")
~~~

Trim and reject empty output. Build RunInfo with UTC time, workflow URL, branch, buildinfo.VersionString, and source metadata. Resolve the Docs checkout too, using github.GetRepository and the Docs branch as its slug/ref; use configured slugs/refs for Saltbox and Sandbox. Omit unavailable revisions and fall back to configured refs.

- [x] **Step 8: Run tests and checkpoint**

~~~bash
gofmt -w automation/health.go automation/health_test.go automation/provenance.go automation/provenance_test.go automation/runner.go automation/update.go
go test ./automation ./health -count=1
git diff --check -- automation health
git status --short -- automation health
~~~

Expected: PASS. Do not commit.

---

### Task 5: Wire Reports into Update Output and Actions Summary

**Files:**
- Modify: automation/update.go
- Create: automation/update_health_test.go
- Modify: github/summary.go
- Create: github/summary_test.go

**Interfaces:**
- Consumes buildHealthReport and health.Report.
- Produces UpdateSummary.SetHealthReport(*health.Report).
- Supplies reports to Tasks 6 and 7.

- [x] **Step 1: Write failing update-flow tests**

Create a fixture where one role update and CLI-help generation fail while checks complete. Assert both error kinds enter the report and the documented warning/exit policy is unchanged. Use fakes/temp fixtures; never execute gh.

- [x] **Step 2: Write a failing Actions-summary test**

Set GITHUB_STEP_SUMMARY to a temporary file. Supply one role error plus a report with a missing page and editorial notice. Assert the file contains Documentation Automation Results, Documentation Health, Missing Documentation, and Editorial Attention. Assert pipes/newlines are escaped.

- [x] **Step 3: Run and observe failure**

~~~bash
go test ./automation ./github -run 'TestUpdateHealthReport|TestWriteGitHubSummaryIncludesHealth' -count=1
~~~

Expected: failure because summaries still use CheckResult.

- [x] **Step 4: Integrate report construction**

Retain cliErr when UpdateCLIHelp fails. When RunCheck is true, build one report, attach it to the summary, and print compact error/notice and nonzero category counts. Preserve warnings and return behavior.

Keep the legacy CheckResult and existing issue-management call as a temporary compatibility path in this checkpoint. When ManageIssue is true, the old coverage collector may run only for the legacy manager. Task 7 switches the manager and update call together, then deletes this compatibility path so the final implementation constructs one report only.

- [x] **Step 5: Extend UpdateSummary with the health report**

Add this field and setter alongside the temporary legacy CheckResult field and SetCheckResult method:

~~~go
HealthReport *health.Report

func (s *UpdateSummary) SetHealthReport(report *health.Report) {
    s.HealthReport = report
}
~~~

Render all enabled/passed/disabled results and complete findings in the Actions summary without the issue-body cap. Preserve role stats, skipped/error details, and writer error propagation.

Task 7 removes the legacy field and setter at the same checkpoint that removes the legacy issue path.

- [x] **Step 6: Run summary and integration tests**

~~~bash
gofmt -w automation/update.go github/summary.go github/summary_test.go
go test ./automation ./github ./health -count=1
git diff --check -- automation github
git status --short -- automation github
~~~

Expected: PASS. Do not commit.

---

### Task 6: Render the Issue and Embedded Structured State

**Files:**
- Create: github/issue_render.go
- Create: github/issue_render_test.go
- Create: github/issue_state.go
- Create: github/issue_state_test.go
- Create: github/testdata/issue_body.golden.md

**Interfaces:**
- Consumes canonical health.Report and health.State.
- Produces github.IssueRenderer, NewIssueRenderer(string), IssueRenderer.Title(health.Report) string, and IssueRenderer.Body(health.Report) (string, error).
- Produces encodeIssueState and decodeIssueState.

- [x] **Step 1: Write a failing golden rendering test**

Use a report with one missing source role, one invalid Docs page, eleven editorial notices, one passed row, one disabled row, exemptions, and fixed RunInfo. Assert exact golden output, no task checkboxes, descriptive workflow links, and escaping of pipes, backticks, brackets, angle brackets, and newlines.

The golden starts:

~~~markdown
> [!IMPORTANT]
> This issue is regenerated by sb-docs. Fix the linked source, documentation,
> or coverage configuration; manual task state is not retained.

| Check | Status | Findings | Exemptions |
|---|---:|---:|---:|
~~~

- [x] **Step 2: Run and observe failure**

~~~bash
go test ./github -run 'TestIssueRendererBodyGolden|TestIssueRendererTitle' -count=1
~~~

Expected: compile failure because IssueRenderer does not exist.

- [x] **Step 3: Implement rendering**

Create a pure renderer that can coexist with the legacy manager until Task 7:

~~~go
type IssueRenderer struct {
    docsRepository string
}

func NewIssueRenderer(docsRepository string) *IssueRenderer {
    return &IssueRenderer{docsRepository: docsRepository}
}

func (r *IssueRenderer) Title(report health.Report) string
func (r *IssueRenderer) Body(report health.Report) (string, error)
~~~

Implement:

- severity-aware singular/plural title wording;
- one row per result with Passed, Disabled, or Action required;
- remediation text per kind, describing create-or-exclude for missing roles and document-under-parent-plus-explicit-exclusion for helper roles;
- code paths for missing expected Docs files;
- links only for existing Docs paths;
- configured source links using revision then fallback ref;
- details blocks above ten findings;
- a 100-visible-item cap per kind with exact omitted count;
- UTC run context;
- Markdown and URL escaping;
- no absolute filesystem paths.

- [x] **Step 4: Write failing state tests**

Test encode/decode round-trip, no-marker behavior, corrupt base64/gzip/JSON, unsupported version, trailing JSON data, a base64 payload above 256 KiB, and decompressed JSON above 1 MiB. Compare decoded State exactly.

- [x] **Step 5: Implement compressed state**

Use encoding/json, compress/gzip, and base64.RawURLEncoding with marker:

~~~text
<!-- docs-automation-state-v1:BASE64URL -->
~~~

Encode canonical state only. Reject encoded payloads above 256 KiB before allocation, and read decompressed JSON through an io.LimitReader capped at 1 MiB plus one sentinel byte. Decode with an anchored multiline regexp, checked gzip close, version validation, trailing-data rejection, and no body echo in errors.

- [x] **Step 6: Append state and run tests**

IssueRenderer.Body appends exactly one state marker after human run context. Do not modify the legacy IssueManager methods in this task; Task 7 adopts the renderer and removes them atomically.

~~~bash
gofmt -w github/issue_render.go github/issue_render_test.go github/issue_state.go github/issue_state_test.go
go test ./github -run 'TestIssueRenderer|TestIssueState' -count=1
git diff --check -- github
git status --short -- github
~~~

Expected: PASS. Do not commit.

---

### Task 7: Replace Markdown Diffs with Semantic Issue Updates

**Files:**
- Modify: github/issue.go
- Replace tests in: github/issue_comment_test.go
- Modify: github/issue_manager_test.go
- Modify: github/summary.go
- Modify: automation/update.go
- Create: github/testdata/issue_comment.golden.md

**Interfaces:**
- Consumes Task 6 rendering/state and health.Diff.
- Produces NewIssueManager(string, io.Writer, io.Writer) *IssueManager.
- Produces IssueManager.ManageIssue(context.Context, health.Report, string) error.
- Produces IssueRenderer.UpdateComment(health.State, health.State, health.RunInfo) string.

- [x] **Step 1: Write failing lifecycle/comment tests**

Cover:

- legacy body without state updates silently;
- workflow-only changes update body without comments;
- one added and one resolved finding posts one semantic comment;
- only changed rows appear;
- added/resolved lists cap at 25 with omitted counts;
- duplicate state hashes suppress comments;
- corrupt prior state logs a note and migrates silently.

The golden comment begins:

~~~markdown
### Docs health changed

| Check | Before | After | Delta |
|---|---:|---:|---:|
~~~

- [x] **Step 2: Run and observe failure**

~~~bash
go test ./github -run 'TestIssueManager.*State|TestIssueRendererUpdateComment' -count=1
~~~

Expected: failure because the manager still parses Markdown and emits raw diffs.

- [x] **Step 3: Refactor ManageIssue**

Change the signature to:

~~~go
func (m *IssueManager) ManageIssue(
    ctx context.Context,
    report health.Report,
    label string,
) error
~~~

Change the constructor to NewIssueManager(repo string, out, errOut io.Writer); IssueManager owns an IssueRenderer created from repo. Generate title/body before gh calls. For an existing issue: load body, decode state, update presentation, silently migrate absent/corrupt state, diff valid state, comment only on semantic changes, hash canonical state for duplicate suppression, and preserve reopen/pin/close/unpin behavior. Use report.HasFindings for lifecycle decisions so configured notices keep the issue open while disabled categories and exemptions do not.

Keep context-aware gh calls, injected writers, stderr context, and body redaction.

- [x] **Step 4: Implement semantic comments and delete obsolete code**

Generate changed-row counts and capped Added/Resolved labels plus workflow/timestamp. Append a duplicate marker in the exact form <!-- docs-automation-state-sha256:HEX -->, where HEX hashes canonical new state JSON. Remove count-heading regexes, extractIssueCounts, LCS/raw diff helpers, and workflow-line normalization. Retain legacy marker recognition only for silent migration.

- [x] **Step 5: Update GitHub Actions outputs**

Accept health.Report. Preserve:

~~~text
has_issues
total_issues
missing_docs
missing_sections
missing_overview_sections
orphaned_docs
issue_title
issue_body
~~~

Add:

~~~text
invalid_frontmatter
editorial_attention
role_automation_errors
cli_automation_errors
error_findings
notice_findings
total_findings
~~~

Keep existing file-open/write/close error propagation.

Set total_issues and total_findings to report.Total() so the legacy name remains compatible. Update automation/update.go to pass the already-built health report to the new manager and remove CheckResult, UpdateSummary.CheckResult/SetCheckResult, runCoverageChecks, checkDocManagedSections, printCoverageCheckResults, and the Task 5 compatibility call. The final path must construct one report only.

- [x] **Step 6: Run tests and checkpoint**

~~~bash
gofmt -w github/issue.go github/issue_comment_test.go github/issue_manager_test.go github/summary.go automation/update.go
go test ./automation ./github ./health -count=1
git diff --check -- automation github
git status --short -- automation github
~~~

Expected: PASS and no live gh call. Do not commit.

---

### Task 8: Update Canonical Policy and Maintainer Documentation

**Files:**
- Modify: /opt/git/docs/.docs-automation.yml
- Modify: README.md
- Delete: SPECS.md
- Modify: .gitignore
- Modify approved spec only if implementation exposes a necessary clarification

**Interfaces:**
- Consumes final config/frontmatter interfaces.
- Produces canonical policy inherited by local config.yml.

- [x] **Step 1: Update canonical configuration**

Add:

~~~yaml
checks:
  coverage:
    enabled: true
    exclude_paths: []
  frontmatter:
    enabled: true
    exclude_paths: []
  editorial:
    enabled: true
    exclude_paths: []
    statuses:
      - draft2
      - outdated

issue:
  source_repositories:
    saltbox:
      slug: saltyorg/Saltbox
      ref: master
    sandbox:
      slug: saltyorg/Sandbox
      ref: master
~~~

Add Sandbox blacklist entries:

~~~yaml
- "n8n_runner"  # internal helper documented by n8n
- "sure_worker"  # internal helper documented by sure
~~~

Do not exclude Autoplow and do not edit any Markdown page.

- [x] **Step 2: Update README**

Document the checks and issue schemas, defaults, exact-path rules, full precedence, page checks, omission semantics, syntax-vs-semantic validation, issue/state/comment behavior, outputs, explicit validator behavior, and unchanged exit policy. Do not duplicate evolving canonical lists.

- [x] **Step 3: Remove the obsolete ignored SPECS file**

Delete the ignored legacy SPECS.md. Its durable managed-issue contract lives in README.md and the approved docs-health design, so do not create another duplicate reference document. Remove the stale SPECS.md ignore rule.

- [x] **Step 4: Validate configuration and formatting**

~~~bash
make build
build/sb-docs --config config.yml validate config
git diff --check -- README.md .gitignore .agents
git -C /opt/git/docs diff --check -- .docs-automation.yml
~~~

Expected: config validation PASS.

- [x] **Step 5: Check repository boundaries**

~~~bash
git status --short -- README.md .gitignore .agents
git -C /opt/git/docs status --short
git -C /srv/git/saltbox status --short
git -C /opt/sandbox status --short
~~~

Docs must contain only .docs-automation.yml. Saltbox/Sandbox must contain no plan-attributable changes. Do not commit.

---

### Task 9: Verify Full Behavior Without Publication

**Files:**
- Verify all files from Tasks 1 through 8.
- Create no tracked files.

**Interfaces:**
- Consumes the complete implementation and every acceptance criterion in the approved spec.
- Produces fresh verification evidence and the final uncommitted handoff; it introduces no production interface.

- [x] **Step 1: Run format and module gates**

~~~bash
test -z "$(gofmt -l .)"
go mod tidy -diff
go mod verify
~~~

Expected: exit 0 with no format/tidy diff.

- [x] **Step 2: Run analysis, lint, race tests, and builds**

~~~bash
go vet ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2 run ./...
go test -count=1 -race ./...
make check
make build
~~~

Expected: zero failures.

- [x] **Step 3: Validate workflow syntax**

~~~bash
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
~~~

Expected: exit 0.

- [x] **Step 4: Validate canonical/overlay configuration**

~~~bash
build/sb-docs --config config.yml validate config
~~~

Expected: Config is valid.

- [x] **Step 5: Run the real frontmatter diagnostic read-only**

~~~bash
build/sb-docs --config config.yml validate frontmatter
~~~

Expected: nonzero because current corpus debt remains. Record valid, invalid, no-frontmatter, and excluded counts. Verify findings are document-oriented and disabled/excluded pages are absent.

- [x] **Step 6: Exercise combined reporting in a temporary Docs copy**

~~~bash
docs_health_tmp=$(mktemp -d /tmp/docs-health-verify.XXXXXX)
cp -a /opt/git/docs "$docs_health_tmp/docs"
cp /opt/git/docs/.docs-automation.yml "$docs_health_tmp/.docs-automation.yml"
cp config.yml "$docs_health_tmp/config.yml"
sed -i   -e 's#extends: ../docs/.docs-automation.yml#extends: .docs-automation.yml#'   -e "s#docs: /opt/git/docs#docs: $docs_health_tmp/docs#"   "$docs_health_tmp/config.yml"
build/sb-docs --config "$docs_health_tmp/config.yml" update --check --no-cli
~~~

Never pass --manage-issue. Expected shape:

- Autoplow is the only missing documentation role.
- n8n_runner and sure_worker are exemptions.
- invalid frontmatter is reported without modifying source Docs;
- draft2/outdated pages are notices;
- missing-doc skips are not duplicated as automation errors.

Counts may drift from 149 invalid and 30 notices only if the corpus changed; investigate instead of forcing historical values.

- [x] **Step 7: Prove semantic migration through fakes**

~~~bash
go test ./github -run 'TestIssueManager.*State|TestIssueRendererBodyGolden|TestIssueRendererUpdateComment' -count=1 -v
~~~

Expected: legacy migration, workflow-only suppression, semantic comment, and duplicate suppression all PASS without live gh.

- [x] **Step 8: Verify boundaries and whitespace**

~~~bash
git diff --check
git -C /opt/git/docs diff --check
git status --short --branch
git -C /opt/git/docs status --short --branch
git -C /srv/git/saltbox status --short --branch
git -C /opt/sandbox status --short --branch
~~~

Expected:

- Docs Automation contains pre-existing hardening, this feature, and agent artifacts.
- Docs contains only .docs-automation.yml.
- Saltbox/Sandbox have no plan-attributable changes.
- No commit, push, tag, release, VM operation, or live issue mutation occurred.

- [x] **Step 9: Update plan checkboxes and hand off evidence**

Use apply_patch to mark completed steps. Report changed files, opt-out behavior, actual corpus counts, gates, retained debt, and uncommitted state. Never claim the corpus is green while configured findings remain.
