# sb-docs

`sb-docs` generates and maintains Saltbox documentation from Saltbox and
Sandbox Ansible roles. It reads role defaults and shared Docker resources,
combines them with per-page frontmatter and one canonical configuration, and
renders managed Markdown sections in the Docs repository.

This README is the reference for the controls available to maintainers:

1. CLI commands and flags.
2. Canonical configuration and path-only local overlays.
3. Per-document `saltbox_automation` frontmatter.
4. Managed-section markers and template placeholders.
5. Role-default section, subsection, and comment directives.
6. Variable discovery, visibility, and type inference.

The canonical config owns evolving data such as role blacklists, override
metadata, inference entries, and Docker suffix lists. This README documents
the schema and fixed behavior without duplicating those lists.

## Quick Start

`sb-docs` loads `config.yml` by default. In a local checkout, use a path-only
overlay that extends the canonical Docs config:

```yaml
extends: ../docs/.docs-automation.yml

repositories:
  saltbox: /srv/git/saltbox
  sandbox: /opt/sandbox
  docs: /opt/git/docs
```

The local `config.yml` is intentionally ignored by Git. Typical commands are:

```shell
# Validate the merged configuration.
sb-docs validate config

# Preview one role's generated inventory section.
sb-docs generate sonarr

# Update one role's existing managed sections.
sb-docs update sonarr

# Update every non-blacklisted role, refresh CLI help, and run configured documentation-health checks.
sb-docs update --check
```

## CLI Reference

### Root and Common Flags

| Flag | Availability | Default | Description |
|------|--------------|---------|-------------|
| `--config <path>` | persistent | `config.yml` | Full canonical config or path-only overlay |
| `-v`, `--verbose` | persistent | `false` | Print verbose progress and diagnostics |
| `-h`, `--help` | root and subcommands | - | Show help for the selected command |
| `--version` | root only | - | Print the version, commit, and build time |

### Commands

| Command | Purpose |
|---------|---------|
| `cli` | Update managed `sb` CLI help in place |
| `generate [role]` | Render role documentation to stdout |
| `scaffold <role>` | Create a starter documentation page |
| `update [role]` | Update managed documentation sections in place |
| `validate config` | Load and validate the selected configuration |
| `validate frontmatter` | Validate app-page frontmatter |
| `index` | Reserved for category index generation; currently not implemented |
| `version` | Print the version, commit, and build time |
| `completion <shell>` | Generate Bash, Fish, PowerShell, or Zsh completion |
| `help [command]` | Show help for a command path |

#### `sb-docs update [role]`

Without a role, `update` processes every non-blacklisted Saltbox and Sandbox
role and updates CLI help unless `--no-cli` is set. With a role, it updates
only that role; CLI generation, coverage checks, and issue management do not
run. A role that exists in both repositories resolves to Saltbox first.

| Flag | Default | Description |
|------|---------|-------------|
| `--no-cli` | `false` | Skip CLI help during an all-role update |
| `--check` | `false` | Build and report the configured documentation-health checks after an all-role update |
| `--manage-issue` | `false` | Create, update, or close the managed GitHub issue; effective only with `--check` |
| `--issue-label <label>` | `docs-automation` | Label used to find and manage the Docs-health issue |

`--manage-issue` requires an installed and authenticated `gh` CLI. Health
findings are reported; they are not converted into a nonzero exit status.
All-role processing also reports individual role and integration failures as
warnings so the final summary remains available.

#### `sb-docs generate [role]`

Without a role, `generate` renders all non-blacklisted roles to stdout. Add
`--cli` to append CLI help. With a role, only that role is rendered and
`--cli` has no effect. Per-role failures during all-role generation are
reported as warnings.

| Flag | Default | Description |
|------|---------|-------------|
| `--cli` | `false` | Include CLI help after an all-role generation |

`generate` is an explicit preview operation. It can render inventory content
even when an existing page has `disabled: true` or
`sections.inventory: false`; those fields control in-place automation and
coverage enforcement.

#### `sb-docs cli`

Runs the selected `sb` binary with `-h`, formats the output with
`templates/cli_help.md.tmpl`, and replaces the configured CLI managed section.

| Flag | Default | Description |
|------|---------|-------------|
| `--binary <path-or-name>` | `cli_help.binary_path` | Override the `sb` executable resolved through `PATH` |

#### `sb-docs scaffold <role>`

The role must exist in Saltbox or Sandbox. Saltbox wins when the name exists
in both. The default template is `templates/app_scaffold.md.tmpl` under the
Docs repository.

| Flag | Default | Description |
|------|---------|-------------|
| `--template <path>` | Docs scaffold template | Use a custom Go template |
| `--output <path>` | Configured repo-specific pattern | Write to an explicit path relative to the current working directory unless absolute |
| `--force` | `false` | Overwrite an existing output file |

Custom scaffold templates receive these fields:

| Field | Meaning |
|-------|---------|
| `.RoleName` | Exact role directory name |
| `.RoleTitle` | English title-cased role name |
| `.RoleTag` | Install tag without the Sandbox prefix |
| `.RepoType` | `saltbox` or `sandbox` |
| `.TagPrefix` | Empty for Saltbox or `sandbox-` for Sandbox |
| `.IsDocker` | Whether the role defines its exact primary image variable in the Docker section |
| `.Icon` | Configured Docker icon for an eligible Docker role, otherwise empty |
| `.AppLinks` | Three ordered semantic link objects containing `Name`, `URL`, `Type`, and `Purpose` |

The canonical scaffold renders exactly three links in this order: Manual
(`manual`/`documentation`), the release link (`release`), and Community
(`community`/`community`). Manual and Community begin with explicit blank
URLs. A non-Docker release uses `Releases`, a blank URL, and type `releases`.
An eligible Docker role receives the configured top-level icon and release
label; a literal repository resolved by `docker_metadata` also supplies the
release URL and type. An unresolved, Jinja-derived, ignored, or unsupported
repository leaves that required release URL blank for an author to complete.
The top-level `icon` key is omitted when `.Icon` is empty. The canonical
template retains both managed marker pairs and the existing role title,
deployment tag, and project-description placeholders.

#### `sb-docs validate`

`validate config` loads the selected full config or overlay and checks required
paths and cross-field constraints.

`validate frontmatter` recursively checks Markdown below `docs/apps` and
`docs/sandbox/apps`, excluding `index.md`. It validates YAML parsing,
non-empty `app_links[].name`, exact semantic purposes, purpose-dependent URL
requirements, and the documented `project_description` name and summary
requirements when enabled. It honors frontmatter path and page opt-outs.
Documentation reached only through a path override, such as
`docs/reference/modules`, is not part of this frontmatter scan.

#### `sb-docs index`

`index` currently prints the intended category-index workflow and makes no
files. `project_description.categories` is reserved for this command.

#### Completion and Help

`completion` supports `bash`, `fish`, `powershell`, and `zsh`. Each shell
subcommand accepts `--no-descriptions`. Use
`sb-docs completion <shell> --help` for shell-specific installation
instructions.

## Configuration

### Full Config and Local Overlay

A full config supplies behavior and repository paths. Repository paths in a
full config are interpreted relative to the process working directory unless
they are absolute.

A local overlay has exactly these fields:

| Field | Required | Description |
|-------|----------|-------------|
| `extends` | yes | Path to one full canonical config, resolved relative to the overlay file |
| `repositories` | at least one child | Local replacements for `saltbox`, `sandbox`, or `docs` |

An overlay must replace at least one repository path. Behavioral keys,
unknown fields, an empty or self-referential `extends`, and nested overlays are
rejected. The base config is loaded first, local repository paths are applied,
and the merged result is then validated. Only `extends` is resolved relative
to the overlay; replacement repository paths keep the normal process-working-
directory semantics of a full config.

### Top-Level Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `repositories` | object | yes | Saltbox, Sandbox, and Docs repository paths |
| `blacklist` | object | no | Roles excluded from all-role automation and coverage |
| `path_overrides` | map | no | Per-repository role-to-doc path replacements |
| `global_overrides` | object | no | Inventory `role_var` exclusions and rendered metadata |
| `docker_overrides` | object | no | Docker+ exclusions, presentation groups, and metadata |
| `docker_metadata` | object | no | Fill-only Docker icon and release-link derivation |
| `section_explainers` | map | no | Shared Markdown introductions keyed by exact section name |
| `type_inference` | object | no | Configured suffix, filter, and symbol type rules |
| `docker_variables` | object | no | Authoritative Docker+ suffix-to-type buckets |
| `cli_help` | object | command-specific | CLI binary and destination file |
| `markers` | object | `variables` required | Managed-section names |
| `scaffold` | object | command-specific | Repo-specific output path patterns |
| `checks` | object | coverage enabled; other categories disabled | Documentation-health policy |
| `issue` | object | empty source metadata | Source-link metadata for documentation-health issues |

### `repositories`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `saltbox` | string | yes | Saltbox repository containing `roles` and shared resources |
| `sandbox` | string | yes | Sandbox repository containing `roles` |
| `docs` | string | yes | Docs repository containing templates and generated pages |

All three paths must exist and be directories. The Saltbox and Sandbox paths
must each contain a `roles` directory.

### `blacklist.docs_coverage`

`docs_coverage` is the only supported child of `blacklist`.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `saltbox` | list | no | Exact Saltbox role names skipped by all-role generation, update, and coverage |
| `sandbox` | list | no | Exact Sandbox role names skipped by all-role generation, update, and coverage |

An explicit single-role `generate`, `update`, or `scaffold` is not blocked by
the blacklist.

### Documentation Health

`update --check` builds one structured documentation-health report after an
all-role update. It includes role and CLI automation failures when those steps
were requested, plus the enabled policy categories below. The canonical config
is the authority for the current blacklist, path exceptions, editorial
statuses, and source repositories; this reference deliberately documents the
schema rather than repeating those evolving values.

#### `checks`

Each category accepts this shape:

```yaml
checks:
  coverage:
    enabled: true
    exclude_paths: []
  frontmatter:
    enabled: false
    exclude_paths: []
  editorial:
    enabled: false
    exclude_paths: []
    statuses: []
```

| Category | Default when omitted | Findings when enabled |
|----------|----------------------|-----------------------|
| `coverage` | `true` | Missing role pages, missing variables or overview marker pairs, and orphaned app pages |
| `frontmatter` | `false` | Unparseable frontmatter and invalid enabled overview metadata |
| `editorial` | `false` | Pages whose top-level `status` exactly matches a configured editorial status |

`enabled` is optional in every category. `exclude_paths` is an optional list
of non-empty, docs-root-relative paths; each entry is normalized and then
matched as one exact path, not as a prefix or glob. Absolute paths, paths that
escape the Docs root, and duplicate normalized paths are rejected. An empty
list means there are no config-level exceptions for that category.

Only `editorial` accepts `statuses`. When editorial checking is enabled, its
status list is required and every value must be non-empty. A page's ordinary
top-level `status` is preserved for the Docs site and is consulted only by the
editorial category; it is not a `saltbox_automation` field.

Coverage first checks source roles against `blacklist.docs_coverage`; a
blacklisted role is exempt from missing-page detection and its corresponding
page is exempt from coverage page checks. Use this only for a role that has no
standalone page. If a helper is documented under its parent, retain an
owner-commented blacklist entry for the helper instead of broadly excluding
its parent or its documentation path.

#### Page-level controls and precedence

The optional page controls live in `saltbox_automation`:

```yaml
saltbox_automation:
  disabled: false
  checks:
    coverage: true
    frontmatter: true
    editorial: true
```

An omitted page check inherits enabled behavior for that page. For an existing
page, the effective precedence is: a disabled global category produces no
findings; an exact category `exclude_paths` entry exempts that category;
coverage then applies its role blacklist; `disabled: true` exempts all
page-level checks and in-place automation; and `checks.<category>: false`
exempts only that category. The specific section toggles are applied last:
`sections.inventory: false` exempts only the variables-marker coverage check,
while `sections.overview: false` exempts the overview-marker coverage check
and overview metadata validation. Neither section setting exempts editorial
attention. Missing source-role pages have no page frontmatter, so they require
a coverage blacklist rather than a page-level opt-out.

Frontmatter syntax and frontmatter semantics are distinct. Invalid YAML or an
unclosed frontmatter block is a syntax finding. For an enabled overview,
stored-file validation requires a `project_description` object with non-empty
`name` and `summary`. Every configured `app_links` entry requires a
non-empty `name` and one exact `purpose` value. `release` and `other` links
also require a non-empty URL; `manual` and `community` links may deliberately
store a blank URL. A syntactically valid page without `saltbox_automation` has
no automation metadata to validate.

`sb-docs validate frontmatter` is an explicit corpus validator, not a
docs-health report switch: it runs even when `checks.frontmatter.enabled` is
false. It validates the values currently stored in each file; it does not run
an update or assume a later metadata repair. An exact config path exclusion is
applied before parsing. After parsing, `disabled: true`,
`checks.frontmatter: false`, or `sections.overview: false` excludes that page,
in that precedence. It prints valid, invalid, no-frontmatter, and excluded
totals and exits nonzero when any non-excluded file is syntactically or
semantically invalid. In contrast, `sb-docs validate config` loads the
selected full config or path-only overlay, validates its schema and required
repository directories, and prints `✅ Config is valid` on success.

#### `issue`

`issue.source_repositories` supplies provenance and source-link metadata for
health findings:

```yaml
issue:
  source_repositories:
    saltbox:
      slug: owner/repository
      ref: branch-or-tag
```

Each configured source has a GitHub `owner/repository` `slug` and a non-empty
`ref`. The report uses the checked-out revision when it can resolve one and
falls back to `ref`; Docs links use the GitHub workflow repository and branch.

#### Health issue, summaries, and exits

`--manage-issue` is effective only with all-role `update --check` and requires
an authenticated `gh` CLI. The managed issue body is regenerated, beginning
with an alert that manual task state is not retained. It contains a per-check
status/findings/exemptions table, remediation and finding tables for each
actionable category, run context, and an opaque versioned state marker.

The manager creates and pins an issue when findings exist, updates and reopens
the labelled issue as needed, and unpins, comments, then closes it when every
enabled category is healthy. For an existing issue with a valid prior state,
it adds at most one bounded comment per new semantic state: the comment has
before/after/delta counts plus added and resolved finding identities. It does
not create a comment merely because rendering or run provenance changed, and
an absent or malformed legacy state is migrated by regenerating the body
without a transition comment.

In GitHub Actions, the update summary is appended to `GITHUB_STEP_SUMMARY`.
The GitHub Actions issue-output adapter writes the following names to
`GITHUB_OUTPUT` when it is invoked in a GitHub Actions run: `has_issues`,
`total_issues`, `missing_docs`, `missing_sections`,
`missing_overview_sections`, `orphaned_docs`, `invalid_frontmatter`,
`editorial_attention`, `role_automation_errors`, `cli_automation_errors`,
`error_findings`, `notice_findings`, `total_findings`, and, when findings
exist, `issue_title` and multiline `issue_body`.

Health findings remain reports, not a new exit-status gate. As before,
per-role automation failures, CLI-help failures, health-report construction,
managed-issue failures, and GitHub-summary write failures are emitted as
warnings so the final summary remains available. Configuration and explicit
validator failures still return errors.

### `path_overrides`

Keys below `path_overrides` are repository types, normally `saltbox` and
`sandbox`. Each child maps a role name to a Markdown path relative to
`repositories.docs`:

```yaml
path_overrides:
  saltbox:
    backup: docs/reference/modules/backup.md
  sandbox:
    main_tag: docs/reference/modules/main_tag.md
```

Overrides are used to find a role's existing page during generation, update,
and role-to-doc coverage checks. Scaffold output is controlled separately by
`scaffold.output_paths`. Without an override, Saltbox pages resolve below
`docs/apps` and Sandbox pages below `docs/sandbox/apps`.

### Override Variable Metadata

Both `global_overrides.variables` and `docker_overrides.variables` use the
same metadata object, keyed by a lookup suffix:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `description` | string | no | Comment text rendered with the variable; multiline Markdown/YAML scalars are supported |
| `default` | string or null | no | Displayed default; empty string renders as `""`, while omitted/null shows no default |
| `type` | string | no | Explicit rendered type label |
| `example` | string | no | Example Markdown; supports `{variable}` and `{role}` placeholders |

`{variable}` becomes the full role- or instance-scoped variable name.
`{role}` becomes the source role name.

### `global_overrides`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `ignore_suffixes` | list | no | Exact `role_var` suffixes omitted from inventory discovery |
| `variables` | map | no | Metadata keyed by exact `role_var` suffix |

Inventory lookups that are not ignored or already documented by the role are
shown as global override options. Discovery unions `role_var` lookups from
Saltbox `inventories/group_vars/all.yml` and the shared
`resources/tasks/directories/create_directories.yml`; failure to read either
authoritative source fails generation or update. Metadata can enrich those
discovered options but does not create a lookup that neither source exposes.

Options with a `_paths_` suffix have one additional gate. The source-role
scanner parses YAML task lists below each repository's
`roles/<role>/tasks` tree and records the repository-qualified role only when
an `ansible.builtin.include_tasks` scalar is exactly
`{{ resources_tasks_path }}/directories/create_directories.yml` after outer
whitespace is trimmed. The same action inside `block`, `rescue`, or `always`
task lists counts. Comments, defaults, task data, short action names, other
include/import actions, and roles of the same name in the other repository do
not grant the capability. Discovered `_paths_` options are rendered only for
that exact caller; the canonical config remains authoritative for which
suffixes are ignored or enriched.

### `docker_overrides`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `ignore_suffixes` | list | no | Docker+ suffixes omitted from discovery |
| `groups` | list | no | Ordered presentation groups for related overrides |
| `variables` | map | no | Metadata for role-defined or Docker+ variables |

Docker suffixes accept `_docker_name`, `_name`, or `name`; all normalize to
`name`.

Each `groups` entry has:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Case-insensitively unique rendered group name |
| `primary` | string | yes | Primary Docker suffix controlling group placement |
| `companions` | list | no | Ordered suffixes rendered after the primary |

A normalized suffix can occur only once across all groups. A group cannot
repeat its primary as a companion or repeat another member. If the role
defines the primary, the complete group is promoted into its Docker section;
otherwise the group appears in Docker+. Missing members use configured
metadata/defaults when present.

### `docker_metadata`

`docker_metadata` derives a Docker icon and a release destination from the
role's primary image repository. The canonical config owns the evolving
override, rule, and ignore entries; their schema is:

| Field | Type | Required when enabled | Description |
|-------|------|-----------------------|-------------|
| `icon` | string | yes | Top-level MkDocs icon offered to Docker pages and scaffolds |
| `release_link.name` | string | yes | Label offered to an existing semantic release link |
| `overrides` | map | no | Normalized repository to an object containing required non-empty `url` and `type` |
| `rules` | list | no | Ordered objects containing an anchored `pattern`, capture-aware `url`, and non-empty `type` |
| `ignore` | list | no | Repositories for which automatic URL resolution is suppressed |

The feature is enabled when any child is configured, at which point `icon`
and `release_link.name` are required. Repository comparisons trim surrounding
whitespace and are case-insensitive. Each rule pattern must begin with `^`,
end with `$`, compile as a Go regular expression, and reference only valid
captures in its URL replacement. Normalized override and ignore keys must be
unique, and a repository cannot appear in both.

Only the exact `<role>_role_docker_image_repo` variable in the exact `Docker`
section (including its subsections) is primary. Resolution accepts a plain,
single-quoted, or double-quoted scalar string only. Jinja delimiters,
non-string YAML, aliases or anchored values, literal/folded block scalars,
empty values, and multiple YAML documents remain unresolved. For a literal
repository, precedence is: exact override, exact ignore, then the first
full-string matching rule. No match is unresolved; there is no heuristic
registry fallback outside the canonical rules.

During update, metadata repair is surgical and fill-only. For an eligible
Docker page with overview automation enabled, it can fill a missing or blank
top-level `icon` and the `name` or `url` of an existing link whose stored
`purpose` is `release`. A URL is offered only after successful repository
resolution. Existing non-empty values always win. Repair never changes
`type`, changes `purpose`, creates an app-link entry or list, or infers which
link is a release. The scalar patch preserves unrelated frontmatter bytes,
comments, quoting, and line endings, reparses before publication, and the
document is saved atomically. The resolved `type` is used when scaffolding a
new page, not to rewrite an authored stored link.

### `section_explainers`

Each key must exactly match a parsed role-default section name. The value is
trimmed Markdown rendered once below the matching section tab:

```yaml
section_explainers:
  Ports: |-
    Ports are allocated automatically and retained for future runs.
```

### `type_inference`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `exact` | map | no | Highest-priority variable-name suffix to type mappings |
| `overrides` | map | no | Additional suffix mappings checked after `exact` |
| `patterns` | list | no | Ordered substring rules with `suffix_contains` and `type` |
| `filters` | map | no | Additional Jinja filter names mapped to proven return types |
| `symbols` | map | no | External symbol names mapped to declared types for reference inference |

Each pattern has:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `suffix_contains` | string | yes | Substring searched anywhere in the variable name |
| `type` | string | yes | Type returned by the matching rule |

Inference proceeds in this order:

1. `exact` suffix rules.
2. `overrides` suffix rules.
3. Native YAML values and structurally proven pure-Jinja expressions.
4. Configured `patterns` in order.
5. Built-in name-suffix fallbacks, then `string`.

Role variables are also resolved together so already-proven same-role
references, `role_var` references, configured external `symbols`, and proven
collection branches can propagate a type without evaluating Ansible.
Ambiguous or quoted runtime expressions retain the `string` fallback.

Built-in Jinja filter return types are:

| Filter | Type |
|--------|------|
| `bool` | `bool` |
| `combine` | `dict` |
| `dict2items` | `list` |
| `float` | `float` |
| `int` | `int` |
| `items2dict` | `dict` |
| `list` | `list` |
| `string` | `string` |

Configured filters extend this set; built-in filter names remain
authoritative. Common rendered labels include `bool`, `int`, `float`,
`string`, `list`, `dict`, `dict/omit`, `string (true/false)`,
`string (number)`, and `string (http/https)`.

### `docker_variables`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `bool` | list | no | Docker suffixes rendered as booleans |
| `int` | list | no | Docker suffixes rendered as integers |
| `list` | list | no | Docker suffixes rendered as lists |
| `dict` | list | no | Docker suffixes rendered as dictionaries |

Suffixes use the same normalization as Docker overrides and may belong to
only one bucket. These buckets are authoritative for Docker+; unlisted
suffixes fall back to `string`, unless per-variable metadata supplies a more
specific type.

Docker+ variables are grouped into these fixed presentation categories:

1. Resource Limits.
2. Security & Devices.
3. Networking.
4. Storage.
5. Monitoring & Lifecycle.
6. Other Options.

### `cli_help`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `binary_path` | string | for CLI generation | Executable name or path used by `cli` and all-role `update` |
| `docs_file` | string | for in-place CLI update | Destination relative to `repositories.docs` |

The Docs repository must contain `templates/cli_help.md.tmpl`, and the
destination must contain the configured CLI marker pair.

### `markers`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `variables` | string | yes | Marker name for generated role variables |
| `cli` | string | for CLI update | Marker name for generated CLI help |
| `overview` | string | for overview update | Marker name for generated project overviews |

The canonical values are `SALTBOX MANAGED VARIABLES SECTION`, `SALTBOX
MANAGED CLI SECTION`, and `SALTBOX MANAGED OVERVIEW SECTION`.

### `scaffold.output_paths`

`output_paths` maps repo type to an output pattern relative to
`repositories.docs`. The only supported placeholder is `{role}`:

```yaml
scaffold:
  output_paths:
    saltbox: docs/apps/{role}.md
    sandbox: docs/sandbox/apps/{role}.md
```

## Document Frontmatter

Behavioral page controls live only in the `saltbox_automation` block. The tool
also reads the top-level MkDocs `icon` to preserve or fill it during eligible
Docker metadata repair. Other page frontmatter, such as `tags`, `hide`,
`title`, or `status`, is preserved but does not configure this tool.

### Complete Shape

```yaml
---
saltbox_automation:
  disabled: false
  checks:
    coverage: true
    frontmatter: true
    editorial: true
  sections:
    inventory: true
    overview: true
  inventory:
    show_sections: []
    hide_sections: []
    example_overrides: {}
  app_links:
    - name: Project home
      url: https://example.com
      type: home
      purpose: manual
  project_description:
    name: Example
    summary: an example application.
    link: https://example.com
    categories: []
---
```

### Top-Level `saltbox_automation` Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `disabled` | bool | `false` | Skip in-place updates and managed-section coverage checks for the page |
| `checks` | object | enabled defaults | Per-page docs-health category opt-outs |
| `sections` | object | enabled defaults | Enable inventory and overview updates independently |
| `inventory` | object | empty filters | Filter rendered sections and override examples |
| `app_links` | list | `[]` | Buttons rendered in the managed overview |
| `project_description` | object or null | `null` | Project metadata rendered in the managed overview |

### `sections`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `inventory` | bool | `true` | Update and require the variables managed section |
| `overview` | bool | `true` | Update and require the overview managed section |

`disabled: true` takes precedence over both. A section is updated only when it
is enabled and the corresponding marker pair exists.

### `checks`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `coverage` | bool | `true` | Include the page in coverage checks |
| `frontmatter` | bool | `true` | Include the page in frontmatter validation |
| `editorial` | bool | `true` | Include the page in editorial-status checking |

`disabled: true` wins over every page check. See [Documentation Health](#documentation-health)
for category-specific path exclusions, section-level effects, and the full
precedence order.

### `inventory`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `show_sections` | list | `[]` | When non-empty, allow only matching section names |
| `hide_sections` | list | `[]` | Hide matching section names |
| `example_overrides` | map | `{}` | Replace the rendered example value for an exact variable name |

Section matching is case-insensitive. `hide_sections` is checked first and
wins over `show_sections`. The role parser always omits `Paths` and metadata
sections, so those cannot be restored with `show_sections`.

### `app_links`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Button label |
| `url` | string | for `release` and `other` | Target URL; it may be blank for `manual` and `community` |
| `type` | string | no | Icon key owned by the canonical overview template |
| `purpose` | string | yes | Exact semantic destination: `manual`, `release`, `community`, or `other` |

`purpose` controls validation and automation ownership; `type` controls only
presentation. The fixed purpose policy is:

| Purpose | URL policy | Meaning |
|---------|------------|---------|
| `manual` | optional | User documentation or project home |
| `release` | required | Release, package, or image versions |
| `community` | optional | Community or support destination |
| `other` | required | Any other authored destination |

Purpose values are exact and case-sensitive. Missing, blank,
whitespace-padded, or unknown values are invalid. Automation does not infer
purpose from a link's name, position, URL, or presentation type.

The canonical `templates/overview.md.tmpl` recognizes:

| Type | Intended destination |
|------|----------------------|
| `manual`, `documentation` | Manual or documentation |
| `home` | Project home |
| `releases` | Generic releases |
| `releases-docker` | Docker releases |
| `releases-github` | GitHub releases |
| `releases-gcloud` | Google Cloud releases |
| `community` | Generic community |
| `community-discord` | Discord community |
| `community-slack` | Slack community |
| `community-plex` | Plex community |
| `docker` | Docker Hub or Docker resource |
| `github` | GitHub resource |
| `gitlab` | GitLab resource |
| `discord` | Discord resource |

An unknown or omitted type still renders the link but has no mapped icon.
Custom overview templates may define a different mapping.

### `project_description`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | for a complete overview | Display name; validation requires it when `summary` is set |
| `summary` | string | for a complete overview | Text following “is” in the generated overview |
| `link` | string | no | Makes the project name a link when set |
| `categories` | list | no | Reserved category hierarchy for the unimplemented `index` command; hierarchy segments use ` > ` |

An overview with links requires `project_description`. With neither links nor
a description, overview generation produces no content.

## Managed Section Markers

The names come from `markers` in the selected config. With the canonical
values, use these exact pairs.

### Overview

```html
<!-- BEGIN SALTBOX MANAGED OVERVIEW SECTION -->
<!-- END SALTBOX MANAGED OVERVIEW SECTION -->
```

### Variables

```html
<!-- BEGIN SALTBOX MANAGED VARIABLES SECTION -->
<!-- END SALTBOX MANAGED VARIABLES SECTION -->
```

### CLI Help

```html
<!-- BEGIN SALTBOX MANAGED CLI SECTION -->
<!-- END SALTBOX MANAGED CLI SECTION -->
```

The tool replaces everything between the first matching BEGIN/END pair and
preserves the markers themselves. A missing pair prevents that section from
being updated and is reported by the relevant command or coverage check.

## Role Defaults Authoring

### Discovery and Sections

For each role, `sb-docs` reads the top-level variables in
`roles/<role>/defaults/main.yml`. It starts after the YAML document marker
`---` when one is present, allowing the standard license header to remain
above it.

A section is a line of at least ten `#` characters followed by a comment line
containing its name. A closing hash line is conventional but optional to the
parser:

```yaml
################################
# Settings
################################

example_role_enabled: true
```

When a file has no parsed sections, variables are rendered under `General`.
Only unindented, top-level YAML variable definitions are discovered.

These section names are treated as metadata and omitted:

- Names containing `Title:`, `Author`, `URL:`, `GNU General Public License`,
  or `Copyright`, matched case-insensitively.
- The exact section name `Paths`, matched case-insensitively.

The presence of `Web`, `DNS`, `Traefik`, and `Docker` sections also controls
which discovered global override families apply to a role.

### Subsections

Subsection markers are case-sensitive and must use this exact grammar:

```yaml
# Authentication Backend - Sub-section Start
example_role_authentication_backend: "file"
# Authentication Backend - Sub-section End
```

The text before ` - Sub-section Start` becomes the rendered subsection name.
Always close a subsection before starting the next one.

### Comments and Directives

Ordinary comments immediately associated with a variable are carried into
the generated example. In Docker sections, associated comments are rendered
as headings; configured `docker_overrides.variables.*.description` supplies
the explanatory YAML comments.

| Directive | Scope | Behavior |
|-----------|-------|----------|
| `# Skip docs` | next variable | Preferred marker for excluding an internal/computed value from generated docs |
| `# Do not edit or override using the inventory` | next variable | Supported legacy exclusion marker with the same parser behavior |
| `# [GLOBAL] text` | subsequent variables in the current section/subsection scope | Accumulate reusable comment text |
| `# [NOGLOBAL] text` | next variable | Suppress active global comments and use only the supplied local text |

Author exclusion directives using the Saltbox/Sandbox `_lookup` convention
must be one of the two exact lines directly above the variable, with no blank
or intervening comment:

```yaml
# Skip docs
example_role_database_url_lookup: "{{ lookup('role_var', '_database_url', role='example') }}"
```

Use `# Skip docs` for new variables. Preserve the legacy directive
`# Do not edit or override using the inventory` when it already exists rather
than stacking both directives. These are documentation controls, not YAML or
Ansible semantics.

The docs parser itself accepts case-insensitive comment text beginning with
either exclusion phrase, provided the phrase leads the associated comment
block. The Saltbox defaults linter deliberately enforces the narrower exact,
immediately-preceding authoring contract for role-local `*_lookup` values.

`[GLOBAL]` and `[NOGLOBAL]` are case-sensitive. Multiple `[GLOBAL]` lines are
joined in order. Global text resets at a new section or subsection end.
`[NOGLOBAL]` should lead the next variable's local comment block.

### Automatic Visibility and Presentation

- A variable ending in `_instances` enables role/instance toggle output and
  supplies the instance list name.
- When `<base>_default` or `<base>_custom` exists, the aggregate `<base>` is
  hidden. The generated docs warn authors to extend `_custom` rather than
  replacing `_default`.
- Variables excluded by comment directives are removed before multiline
  values are parsed into the rendered model.
- `show_sections` and `hide_sections` apply after role parsing.
- A role-defined Docker variable is shown in its role section. Docker options
  discovered from shared resources but absent from the role appear in
  Docker+ unless ignored or promoted into a configured group.

### Global Override Discovery

`sb-docs` unions `role_var` lookup calls from
`inventories/group_vars/all.yml` and
`resources/tasks/directories/create_directories.yml`. Each lookup's
`<suffix>` is inferred from its name and context, removed when listed in
`global_overrides.ignore_suffixes`, and enriched from
`global_overrides.variables`. As described in [`global_overrides`](#global_overrides),
rendering a discovered `_paths_` suffix additionally requires an exact,
repository-qualified managed-directory task caller.

An override already represented by `<role>_role<suffix>` in a visible role
section is not repeated. Web, Traefik/ThemePark, Docker/dependency, and DNS
suffix families are shown only when the role has their corresponding section.

### Docker+ Discovery

`sb-docs` scans top-level `*.yml` files in
`resources/tasks/docker` for both:

- `lookup('docker_var', '_docker_<suffix>', ...)` calls.
- `_docker_var_specs` mapping keys beginning with `_docker_`.

Discovered suffixes already defined as `<role>_role_docker_<suffix>` are
removed. Remaining options are filtered by `docker_overrides.ignore_suffixes`,
typed by `docker_variables` and per-variable metadata, grouped when
configured, and categorized for Docker+ rendering.

The canonical inventory template walks authored `SectionOrder` without
reordering it. When a visible Docker section has Docker+ content, Docker+ is
emitted immediately after Docker inside that loop. Hiding Docker also prevents
Docker+ from appearing. Later authored sections retain their relative order,
and Global Override Options is emitted after the authored loop, so it remains
last whenever present.

## Architecture

The project uses a flat, command-first Go layout:

- `cmd/` builds Cobra commands, binds flags, validates arguments, and routes
  calls.
- `automation/` owns update, generation, validation, scaffolding, indexing,
  and CLI-help workflows behind `Runner`.
- `config/`, `document/`, `parser/`, and `render/` own configuration,
  frontmatter, role discovery, inference, and rendering.
- `github/` and `clihelp/` are concrete external adapters.
- `buildinfo/` contains immutable build metadata.
- `main.go` wires process context, signals, errors, and exit status.

Application code does not use an `internal/` hierarchy, and command workflow
logic belongs in `automation/`, not `cmd/`.
