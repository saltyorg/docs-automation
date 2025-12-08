# Saltbox Automation Front-Matter Format

This document describes the YAML front-matter format used by the docs-automation tool to generate documentation sections.

## Overview

The `saltbox_automation` block is added to the YAML front-matter of markdown documentation files. It controls which sections are auto-generated and provides metadata for those sections.

## Basic Structure

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

## Field Reference

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

The `type` field maps to icons defined in the template (`templates/overview.md.tmpl`):

| Type | Icon | Description |
|------|------|-------------|
| `manual` | `:material-bookshelf:` | Manual/documentation |
| `docs` | `:material-bookshelf:` | Documentation |
| `home` | `:material-home:` | Project home page |
| `releases` | `:fontawesome-solid-newspaper:` | Generic releases |
| `releases-docker` | `:fontawesome-brands-docker:` | Docker Hub releases |
| `releases-github` | `:fontawesome-brands-github:` | GitHub container registry |
| `releases-gcloud` | `:simple-googlecloud:` | Google Cloud releases |
| `community` | `:fontawesome-solid-people-group:` | Generic community forums |
| `community-discord` | `:fontawesome-brands-discord:` | Discord server |
| `community-slack` | `:fontawesome-brands-slack:` | Slack workspace |
| `community-plex` | `:material-plex:` | Plex forums |
| `discord` | `:fontawesome-brands-discord:` | Discord (shorthand) |
| `github` | `:fontawesome-brands-github:` | GitHub (shorthand) |
| `docker` | `:fontawesome-brands-docker:` | Docker (shorthand) |

### Project Description

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Project display name |
| `summary` | string | yes | Brief project description |
| `link` | string | no | Primary project URL |
| `categories` | list | no | Category tags for the project |

## Examples

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

## Required Section Anchors

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

## Section Precedence

1. If `disabled: true`, no sections are generated
2. Individual section flags (`sections.inventory`, `sections.overview`) control each section
3. Sections default to `true` (enabled) if not specified

## Variable Section Filtering

The `show_sections` and `hide_sections` lists work as follows:

1. `hide_sections` is checked first - if a section is in this list, it is hidden
2. If `show_sections` is non-empty, only sections in that list are shown
3. `hide_sections` takes precedence over `show_sections`
4. Section names are matched case-insensitively
