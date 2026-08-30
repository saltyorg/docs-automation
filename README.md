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

# Update every non-blacklisted role, refresh CLI help, and report coverage.
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
| `--check` | `false` | Report missing docs, missing managed sections, and orphaned docs after an all-role update |
| `--manage-issue` | `false` | Create, update, or close the managed GitHub issue; effective only with `--check` |
| `--issue-label <label>` | `docs-automation` | Label used to find and manage the coverage issue |

`--manage-issue` requires an installed and authenticated `gh` CLI. Coverage
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

#### `sb-docs validate`

`validate config` loads the selected full config or overlay and checks required
paths and cross-field constraints.

`validate frontmatter` recursively checks Markdown below `docs/apps` and
`docs/sandbox/apps`, excluding `index.md`. It validates YAML parsing,
non-empty `app_links[].name`, non-empty `app_links[].url`, and the documented
project-name dependency. Documentation reached only through a path override,
such as `docs/reference/modules`, is not part of this frontmatter scan.

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
| `section_explainers` | map | no | Shared Markdown introductions keyed by exact section name |
| `type_inference` | object | no | Configured suffix, filter, and symbol type rules |
| `docker_variables` | object | no | Authoritative Docker+ suffix-to-type buckets |
| `cli_help` | object | command-specific | CLI binary and destination file |
| `markers` | object | `variables` required | Managed-section names |
| `scaffold` | object | command-specific | Repo-specific output path patterns |

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
shown as global override options. Metadata can enrich those discovered
options but does not create a lookup that the inventory never exposes.

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

`sb-docs` reads only the `saltbox_automation` block. Other page frontmatter,
such as MkDocs `icon`, `tags`, `hide`, `title`, or `status`, is preserved but
does not configure this tool.

### Complete Shape

```yaml
---
saltbox_automation:
  disabled: false
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
| `url` | string | yes | Non-empty target URL |
| `type` | string | no | Icon key owned by the canonical overview template |

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

`sb-docs` scans `inventories/group_vars/all.yml` for `role_var` lookup calls.
Each lookup's `<suffix>` is inferred from its name and context, removed when
listed in `global_overrides.ignore_suffixes`, and enriched from
`global_overrides.variables`.

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
