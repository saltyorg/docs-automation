# Saltbox Automation Front-Matter Format

This document describes the YAML front-matter format used by the docs-automation tool to generate documentation sections, plus the tool's `config.yml`.

## Overview

The `saltbox_automation` block is added to the YAML front-matter of markdown documentation files. It controls which sections are auto-generated and provides metadata for those sections.

There are two distinct configuration surfaces:
1. Front-matter: per-document settings under `saltbox_automation`.
2. Config file: global tool settings in `config.yml` (or `--config`).

The CLI reads `config.yml` for repository paths, templates, markers, and type inference settings. The section below documents all configuration options.

## Configuration File (`config.yml`)

`sb-docs` loads `config.yml` by default. Override the path with `--config`.

### Top-Level Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `repositories` | object | yes | Paths to the Saltbox, Sandbox, and Docs repositories |
| `blacklist` | object | no | Role lists excluded from coverage checks and generation |
| `path_overrides` | map | no | Per-repo overrides for documentation file paths |
| `global_overrides` | object | no | Metadata for `role_var` overrides (inventory scan + docs) |
| `type_inference` | object | no | Variable type inference rules |
| `docker_variables` | object | no | Docker variable suffix lists by type |
| `cli_help` | object | no | CLI help generation settings |
| `frontmatter_docs` | list | no | Docs files that only use frontmatter-managed sections |
| `markers` | object | yes (`variables` required) | Managed section marker names |
| `scaffold` | object | no | Output path patterns for scaffolding |

### repositories

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `saltbox` | string | yes | Path to the Saltbox repo (used for roles and templates) |
| `sandbox` | string | yes | Path to the Sandbox repo (used for roles) |
| `docs` | string | yes | Path to the Docs repo (used for templates and output) |

### blacklist.docs_coverage

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `saltbox` | list | no | Saltbox role names to skip during coverage/generation |
| `sandbox` | list | no | Sandbox role names to skip during coverage/generation |

### path_overrides

Override the default docs path for specific roles. Paths are relative to `repositories.docs`.

```yaml
path_overrides:
  saltbox:
    plex: "docs/media-servers/plex.md"
  sandbox:
    custom_role: "docs/special/custom_role.md"
```

### global_overrides

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `ignore_suffixes` | list | no | Suffixes to ignore when scanning inventory for `role_var` lookups |
| `variables` | map | no | Per-suffix override metadata used in docs |

Each entry in `variables` is keyed by the `role_var` suffix (for example `_web_host_override`) and can include:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `description` | string | no | Text shown in generated docs |
| `default` | string or null | no | Default value; `null` means "no default shown" |
| `type` | string | no | Override type label for docs |
| `example` | string | no | Example snippet for docs (supports block scalars) |

### type_inference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `exact` | map | no | Suffix-to-type map checked first |
| `overrides` | map | no | Additional suffix overrides checked after `exact` |
| `patterns` | list | no | Pattern rules matched by substring |

Each `patterns` entry includes:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `suffix_contains` | string | yes | Substring to match in the variable name |
| `type` | string | yes | Type label to use |

### docker_variables

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `bool` | list | no | Docker variables treated as booleans |
| `int` | list | no | Docker variables treated as integers |
| `list` | list | no | Docker variables treated as lists |
| `dict` | list | no | Docker variables treated as dictionaries |

### cli_help

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `binary_path` | string | no | Path to the `sb`/`sb2` binary used for `sb-docs cli` |
| `docs_file` | string | no | Docs file (relative to `repositories.docs`) containing the CLI marker |

### frontmatter_docs

List of documentation files (relative to `repositories.docs`) that should be updated using frontmatter-only sections. These files will not attempt inventory generation.

```yaml
frontmatter_docs:
  - "docs/index.md"
```

### markers

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `variables` | string | yes | Marker name for variable sections |
| `cli` | string | no | Marker name for CLI sections |
| `overview` | string | no | Marker name for overview sections |

### scaffold

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `output_paths` | map | no | Output path patterns by repo type (supports `{role}`) |

## Frontmatter: Basic Structure

```yaml
---
saltbox_automation:
  disabled: false                    # Disable all automation for this file
  sections:
    inventory: true                  # Generate inventory section
    overview: true                   # Generate overview section
  inventory:
    show_sections: []                # Only show these variable sections
    hide_sections: []                # Hide these variable sections
    example_overrides: {}            # Override example values
  app_links: []                      # Links for the overview table
  project_description: null          # Project metadata
---
```

## Frontmatter: Field Reference

### Top-Level Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `disabled` | bool | `false` | When `true`, disables all automation for this file |
| `sections` | object | - | Controls which sections to generate |
| `inventory` | object | - | Configures inventory section generation |
| `app_links` | array | `[]` | Links displayed in the overview table |
| `project_description` | object | `null` | Project metadata for overview |

### Sections Configuration

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `inventory` | bool | `true` | Generate the inventory/variables documentation section |
| `overview` | bool | `true` | Generate the overview section |

### Inventory Configuration

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `show_sections` | list | `[]` | If non-empty, only show these variable sections |
| `hide_sections` | list | `[]` | Hide these variable sections from output |
| `example_overrides` | map | `{}` | Override example values for specific variables |

### App Links

Each app link has the following structure:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Display name for the link |
| `url` | string | yes | URL target |
| `type` | string | no | Link type for icon mapping (see below) |

#### Link Types

The `type` field maps to icons defined in the docs repo template (`templates/overview.md.tmpl`):

### Project Description

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Project display name |
| `summary` | string | yes | Brief project description |
| `link` | string | no | Primary project URL |
| `categories` | list | no | Category tags for the project |

## Frontmatter: Examples

### Minimal: Disable Inventory Section

```yaml
---
saltbox_automation:
  sections:
    inventory: false
---
```

### Disable All Automation

```yaml
---
saltbox_automation:
  disabled: true
---
```

### Filter Variable Sections

Only show specific sections:

```yaml
---
saltbox_automation:
  inventory:
    show_sections:
      - General
      - Docker
---
```

Hide specific sections:

```yaml
---
saltbox_automation:
  inventory:
    hide_sections:
      - Internal
      - Paths
---
```

### Override Example Values

```yaml
---
saltbox_automation:
  inventory:
    example_overrides:
      sonarr_web_port: "8989"
      sonarr_api_key: "your-api-key-here"
---
```

### Full Overview Configuration

```yaml
---
saltbox_automation:
  project_description:
    name: Sonarr
    summary: A smart PVR for TV shows
    link: https://sonarr.tv
    categories:
      - Media Management
      - PVR
  app_links:
    - name: Home
      url: https://sonarr.tv
      type: home
    - name: Manual
      url: https://wiki.sonarr.tv
      type: manual
    - name: Community
      url: https://discord.sonarr.tv
      type: discord
---
```

## Frontmatter: Required Section Anchors

For the automation tool to inject content, the markdown file must contain anchor comments marking where each section should be placed.

### Overview Section

```html
<!-- BEGIN SALTBOX MANAGED OVERVIEW SECTION -->
<!-- END SALTBOX MANAGED OVERVIEW SECTION -->
```

### Variables Section

```html
<!-- BEGIN SALTBOX MANAGED VARIABLES SECTION -->
<!-- END SALTBOX MANAGED VARIABLES SECTION -->
```

The tool will replace everything between the BEGIN and END markers with the generated content.

## Frontmatter: Section Precedence

1. If `disabled: true`, no sections are generated
2. Individual section flags (`sections.inventory`, `sections.overview`) control each section
3. Sections default to `true` (enabled) if not specified

## Frontmatter: Variable Section Filtering

The `show_sections` and `hide_sections` lists work as follows:

1. `hide_sections` is checked first - if a section is in this list, it is hidden
2. If `show_sections` is non-empty, only sections in that list are shown
3. `hide_sections` takes precedence over `show_sections`
4. Section names are matched case-insensitively
