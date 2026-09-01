# Docs Health Issue Design

Date: 2026-09-01
Status: approved

## Objective

Turn the single managed GitHub issue into an actionable documentation-health
tracker without imposing one universal page or frontmatter format.

The governing rule is:

> A check validates only a contract that configuration or a page has enabled.
> A page may opt out at the check, role/path, page, section, or optional-field
> level. Once a feature is enabled and data is supplied for it, that data must
> be internally valid.

The issue remains an automatically managed, pinned, single source of current
findings. It is not a manually maintained checklist.

## Scope

This change covers:

- a structured documentation-health report independent of GitHub Markdown;
- inherited global, path, page, and section check controls;
- coverage, frontmatter, editorial-status, role-update, and CLI-update findings;
- actionable issue Markdown with summary, remediation, links, and provenance;
- semantic added/resolved comments based on structured state;
- canonical Docs configuration for enabled checks and source links;
- explicit coverage exclusions for the parent-owned `n8n_runner` and
  `sure_worker` helper roles;
- public README and specification updates;
- focused unit, integration, migration, and corpus acceptance tests.

It does not cover:

- repairing the existing frontmatter corpus;
- creating or writing the missing Autoplow documentation page;
- requiring fixed headings, page length, link counts, or a complete universal
  frontmatter shape;
- checking external HTTP availability;
- duplicating MkDocs navigation or internal-link validation;
- changing the documented all-role warning and process-exit policy;
- mutating the live GitHub issue during development;
- commits, pushes, tags, releases, or publication.

## Existing Contracts Preserved

- `update --check` remains the runtime opt-in for unified health checks.
- `--manage-issue` remains effective only with `--check`.
- `blacklist.docs_coverage` remains the role-level opt-out for roles that do
  not require standalone pages.
- `saltbox_automation.disabled: true` remains the page-wide automation
  opt-out and is broadened consistently to all automation-owned checks.
- `sections.inventory` and `sections.overview` remain the section-level
  controls.
- Omitting `app_links`, `project_description`, `status`, or another optional
  metadata feature remains valid.
- Path-only local overlays continue to override repository paths only; all
  behavior remains inherited from the canonical configuration.
- Coverage findings and per-role integration failures remain reportable
  without changing the command's existing exit-status policy.

## Configuration

### Global checks

Add a top-level `checks` object to the full configuration:

```yaml
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
```

`enabled` controls whether the category participates in unified
`update --check` results and the managed issue. Defaults preserve existing
unified-check behavior unless the canonical configuration opts into a new
category:

- coverage defaults to enabled;
- frontmatter defaults to disabled in unified checks;
- editorial reporting defaults to disabled because status vocabulary is owned
  by each Docs configuration.

The canonical Docs configuration explicitly sets all three values, so intent
is visible rather than inferred from defaults.

`validate frontmatter` remains an explicit diagnostic command: invoking it
runs frontmatter validation even when unified frontmatter checking is globally
disabled. It still honors config path exclusions, `disabled: true`, page-level
frontmatter opt-outs, and overview section controls. The explicit command is
therefore an invocation-level opt-in, not a hidden managed-issue check.

`exclude_paths` contains exact, slash-separated Markdown paths relative to
`repositories.docs`. Exclusions are applied before frontmatter parsing for the
selected check, which allows a deliberately nonstandard page to opt out even
when it cannot declare an exemption inside `saltbox_automation`. Generation
still requires syntactically parseable frontmatter unless the role itself is
excluded from automation. Paths must be relative, clean, and must not escape
the Docs root. Duplicates are rejected after normalization. Globs are
deliberately unsupported.

`editorial.statuses` contains exact, case-sensitive status values that become
non-blocking editorial notices. An enabled editorial check requires at least
one configured status. The canonical values are `draft2` and `outdated`;
ordinary `draft` pages are not reported.

### Source repository links

Add optional issue-link metadata:

```yaml
issue:
  source_repositories:
    saltbox:
      slug: saltyorg/Saltbox
      ref: master
    sandbox:
      slug: saltyorg/Sandbox
      ref: master
```

The slug must use `owner/repository` form. The fallback ref is used when a
local Git revision cannot be determined. Omitting an entry disables links for
that source without disabling its checks.

### Role-level exclusions

Keep missing-page exclusions under `blacklist.docs_coverage`. Add
`n8n_runner` and `sure_worker` to the Sandbox list with comments that they are
internal roles owned by the n8n and Sure pages. Autoplow remains a finding.

## Page-Level Controls

Extend `saltbox_automation` with optional check overrides:

```yaml
saltbox_automation:
  checks:
    coverage: false
    frontmatter: false
    editorial: false
```

Each field is optional. Omission inherits the global category setting. `false`
opts the page out; a page cannot re-enable a globally disabled category.

Precedence, from strongest to weakest, is:

1. A globally disabled category does not run.
2. A matching config `exclude_paths` entry does not run.
3. `saltbox_automation.disabled: true` disables generation and all
   automation-owned checks for the page.
4. `saltbox_automation.checks.<category>: false` disables that page's category.
5. `sections.inventory: false` or `sections.overview: false` disables the
   corresponding generator, marker check, and section-owned semantic checks.
6. Otherwise the enabled contract is checked.

For missing documentation, no page exists to carry frontmatter. The existing
repository-specific role blacklist is therefore the authoritative opt-out.

A lean inventory-only page is valid with:

```yaml
saltbox_automation:
  sections:
    inventory: true
    overview: false
```

It is not required to define `app_links` or `project_description`.

## Semantic Validation

YAML syntax must still be valid whenever the application must parse a page.
Syntax is not a presentation preference. A category path exclusion prevents
that check from parsing the page; a role blacklist is required when generation
itself must not load the page.

When frontmatter checking is enabled for a page:

- no frontmatter remains valid and receives no automation-metadata semantic
  checks;
- frontmatter without a `saltbox_automation` block remains valid, subject to
  ordinary coverage rules;
- overview-owned metadata is ignored when `sections.overview` is false;
- `app_links` is optional, but each supplied entry requires a trimmed nonempty
  name and URL;
- `app_links[].type` remains optional and open-ended;
- within a present `saltbox_automation` block, `project_description` is
  optional only when overview generation is disabled;
- a present `saltbox_automation` block with overview enabled requires a
  supplied description with trimmed nonempty `name` and `summary`;
- `project_description.link` and `categories` remain optional;
- URLs are checked for nonempty values only, not restricted to a scheme or
  tested over the network.

Validation returns all diagnostics for a document but creates one stable
frontmatter finding per document with the diagnostics grouped in its detail.
This keeps issue counts document-oriented and preserves the existing
`valid`/`invalid` command summary.

## Health Report Module

Add a top-level `health` package. It is an in-process deep module whose small
interface is shared by automation, GitHub issue rendering, GitHub Actions
summary rendering, and tests.

The module owns:

- fixed finding kinds;
- `error` and `notice` severities;
- a stable finding identity;
- enabled/disabled category results;
- exemption counts;
- deterministic sorting;
- totals by kind and severity;
- run provenance;
- a canonical serializable state used for issue change detection.

Finding kinds are:

- missing documentation;
- missing variables section;
- missing overview section;
- orphaned documentation;
- invalid frontmatter;
- editorial attention;
- role automation error;
- CLI-help automation error.

A finding carries only domain data: kind, repository, role or subject, Docs
path, source path, stable diagnostic code, and human detail. GitHub URLs and
Markdown remain implementation details of the GitHub issue module.

The report's interface provides report construction, canonicalization,
category lookup, `HasFindings`, totals, and structured diffing. Callers do not
parse rendered Markdown or maintain parallel count fields.

## Automation Flow

`Runner.Update` continues to collect `RoleResult` values. After role and CLI
processing it builds one health report:

1. Add role automation errors, excluding roles already omitted by the role
   blacklist or page/section controls.
2. Add a CLI-help error only when CLI generation was requested and failed.
3. Run enabled coverage checks and add their four finding kinds.
4. Run enabled frontmatter checks with the inherited exclusions.
5. Run enabled editorial-status checks.
6. Attach exemption counts and best-effort provenance.
7. Print the report, attach it to the Actions summary, and optionally manage
   the GitHub issue.

Missing documentation remains a coverage finding rather than also appearing
as a role automation error for the expected `doc file does not exist` skip.
Other expected skips remain visible in the Actions run summary but are not
issue findings.

Fatal startup errors that prevent report construction still return before
issue management. The managed issue cannot be its own outage monitor; the
workflow result remains authoritative for those failures.

Best-effort source revisions are resolved through a small internal seam in
automation. The production adapter runs `git -C <repository> rev-parse HEAD`;
tests use a fake adapter. Missing Git metadata omits the revision and falls
back to the configured source ref without changing check results.

## Issue Presentation

### Title

Use severity-aware wording:

```text
[Docs Health] 12 errors, 3 notices
```

Zero-valued severities are omitted with correct singular/plural grammar. The
issue closes only when every enabled category has zero findings. Notices do
not change command exit behavior, but they keep the health tracker open while
configured editorial work remains.

### Body

The body begins with a GitHub generated-content alert explaining that source,
Docs, or configuration must be changed rather than manual task state.

It then renders a summary table for every category:

```text
Check | Status | Findings | Exemptions
```

Enabled zero-result categories show `Passed`; disabled categories show
`Disabled`; nonzero categories show `Action required`. This distinguishes a
passing check from one that did not run.

Each exemption total counts unique roles or Docs-relative paths suppressed for
that row by the role blacklist, category path exclusions, page-wide disable,
page check override, or section control. A globally disabled row displays an
em dash instead of treating every possible subject as an exemption.

Only nonzero categories receive detail sections. Findings use linked tables or
bullets, never checkboxes. Each section states the available remediation:

- create a standalone page, document a helper under its parent page and
  explicitly exclude it, or intentionally exclude a role;
- restore or disable a managed section;
- remove, map, or exclude an orphaned page;
- correct, omit, disable, or path-exclude invalid optional metadata;
- resolve or deliberately clear an editorial status;
- inspect the linked workflow for automation errors.

Source role links use configured slugs and the detected revision or fallback
ref. Existing Docs paths link to the Docs repository and branch. Missing
expected Docs paths are rendered as code rather than broken links. All dynamic
table content is Markdown-escaped.

More than ten findings in a category are wrapped in `<details>`. At most 100
findings per category are rendered in the body; any remainder is counted and
directed to the complete Actions summary. This keeps the body below GitHub's
size limit without hiding the total.

Run context includes UTC check time, descriptive workflow link, Docs branch,
`sb-docs` version/commit, and available source revision links. Run metadata is
not part of semantic finding identity.

### Embedded state and update comments

Embed one versioned, compressed, base64url-encoded JSON state marker containing
stable finding identities and display labels. It contains no secrets, command
bodies, or filesystem-absolute paths.

The GitHub issue module decodes this state and diffs it against the new report.
It no longer parses its own headings with regular expressions and no longer
generates raw line diffs.

Comments contain only changed categories and capped `Added`/`Resolved` lists,
plus the workflow link and timestamp. Workflow URL, timestamp, version, or
source-revision-only changes update the main body without creating comments.

The first update of a legacy body without structured state performs a silent
migration: it replaces the body and embeds state but does not post a misleading
bulk-change comment. Existing hash markers remain readable only for migration;
new duplicate-comment detection hashes canonical structured state.

## GitHub Actions Outputs and Summary

Preserve existing output names for compatibility and add:

- `invalid_frontmatter`;
- `editorial_attention`;
- `role_automation_errors`;
- `cli_automation_errors`;
- `error_findings`;
- `notice_findings`;
- `total_findings`.

`has_issues` remains true when any enabled category has findings. The Actions
summary uses the same report, displays all findings without the issue body's
100-item presentation cap, and retains role update/skip statistics.

## Canonical Corpus Changes

The Docs repository changes are limited to `.docs-automation.yml`:

- explicitly enable coverage and frontmatter checks;
- enable editorial reporting for `draft2` and `outdated`;
- add Saltbox and Sandbox source repository link metadata;
- exclude `n8n_runner` with a parent-owned comment;
- exclude `sure_worker` with a parent-owned comment.

No Markdown pages, templates, generated sections, navigation, or frontmatter
records are repaired in this change. The current invalid and editorial pages
remain visible findings that prove the new tracker without being silently
rewritten.

## Test Strategy

Implementation follows red-green-refactor. Tests exercise the same module
interfaces used by production.

### Configuration tests

- category defaults and explicit enable/disable behavior;
- path-only overlay inheritance;
- exact normalized path exclusions;
- rejection of absolute, escaping, and duplicate exclusions;
- editorial enabled-without-status rejection;
- source slug/ref validation.

### Frontmatter and precedence tests

- global disable wins over page settings;
- config path exclusion occurs before parsing;
- `disabled: true` disables all semantic checks;
- check-specific page opt-outs are independent;
- section disable skips section-owned checks only;
- omitted optional metadata is valid;
- present app links require name and URL;
- enabled overview requires a complete description;
- disabled overview permits absent or deliberately lean metadata;
- one document with multiple diagnostics becomes one finding.

### Report tests

- stable identity and deterministic ordering;
- enabled, disabled, passed, error, notice, and exemption totals;
- added/resolved structured diffs;
- run metadata does not affect semantic equality.

### Issue tests

- golden title/body for mixed findings and disabled categories;
- no task checkboxes and descriptive links;
- Markdown escaping;
- visible finding cap with accurate totals;
- structured state round-trip;
- silent legacy-body migration;
- semantic comments list only added/resolved findings;
- workflow-only changes produce no comment;
- existing context, injected-writer, and body-redaction tests remain green.

### Automation and corpus tests

- missing docs do not duplicate expected skip errors;
- role and CLI errors enter the report when their operations were enabled;
- helper-role blacklist leaves Autoplow as the current missing-page finding;
- current corpus frontmatter and editorial debt appears in the locally rendered
  report without mutating Docs;
- an isolated temporary Docs copy proves `update --check` behavior.

## Verification

Run fresh:

- `gofmt` check;
- `go mod tidy -diff` and `go mod verify`;
- `go vet ./...`;
- pinned golangci-lint;
- `go test -count=1 -race ./...`;
- `make build` and `make check`;
- canonical and local-overlay config validation;
- focused report/issue golden tests;
- isolated current-corpus health rendering with no live issue management;
- `git diff --check` in Docs Automation and Docs;
- status inspection proving no Saltbox/Sandbox changes and no live GitHub
  mutation.

The existing 149 invalid frontmatter files are an expected non-green corpus
result, not a failed implementation verification. Verification must compare
the complete categorized report with the inspected baseline and call out any
drift.

## Rejected Alternatives

### Universal required frontmatter schema

Rejected because deliberately lean pages are valid and the user explicitly
requires opt-outs at every level.

### Per-field waiver flags such as `allow_empty_url`

Rejected because they render broken output while expanding the interface. A
page opts out by omitting or disabling the feature.

### Parsing human Markdown to recover state

Rejected because formatting changes would continue to break count extraction
and comments. Structured hidden state keeps the human presentation free to
evolve.

### Globs for path exclusions

Rejected because exact paths are auditable and consistent with the existing
exact role blacklist. Patterns can be added only if real repetition proves the
need.

### Repairing all reported pages in the same change

Rejected because it would combine application behavior, policy, and a large
editorial migration. The tracker must first report the real backlog without
silently rewriting it.

## Approval Boundary

Implementation begins only after the user reviews and approves this written
specification. Approval authorizes the described project and canonical Docs
configuration edits, tests, and local verification. It does not authorize live
issue mutation, corpus repair, commits, pushes, tags, releases, or VM use.
