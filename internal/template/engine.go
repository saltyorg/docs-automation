package template

import (
	"bytes"
	"fmt"
	"os"
	"text/template"
)

// Engine handles template loading and rendering.
type Engine struct {
	templates map[string]*template.Template
}

// New creates a new template engine.
func New() *Engine {
	return &Engine{
		templates: make(map[string]*template.Template),
	}
}

// LoadFile loads a template from a file path.
func (e *Engine) LoadFile(name, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading template file: %w", err)
	}

	return e.LoadString(name, string(content))
}

// LoadString loads a template from a string.
func (e *Engine) LoadString(name, content string) error {
	tmpl, err := template.New(name).Funcs(FuncMap()).Parse(content)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	e.templates[name] = tmpl
	return nil
}

// Render renders a template with the given data.
func (e *Engine) Render(name string, data interface{}) (string, error) {
	tmpl, ok := e.templates[name]
	if !ok {
		return "", fmt.Errorf("template %q not found", name)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}

// RenderString parses and renders a template string in one step.
func (e *Engine) RenderString(content string, data interface{}) (string, error) {
	tmpl, err := template.New("inline").Funcs(FuncMap()).Parse(content)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}

// LoadRoleTemplate loads the role documentation template.
// If templatePath is provided and exists, it loads from file.
// Otherwise, it uses the built-in default template.
func (e *Engine) LoadRoleTemplate(templatePath string) error {
	if templatePath != "" {
		if _, err := os.Stat(templatePath); err == nil {
			return e.LoadFile("role", templatePath)
		}
	}
	return e.LoadString("role", DefaultRoleTemplate())
}

// DefaultRoleTemplate returns the default template for role documentation.
func DefaultRoleTemplate() string {
	return `{{- /* Role Variables Section Template */ -}}
{{- if .Config }}{{- if .Config.GetIntro }}
{{ .Config.GetIntro }}
{{- end }}{{- end }}

{{- if .HasDefaultVars }}
!!! warning "Avoid Overriding _default Variables"

    Variables ending in ` + "`_default`" + ` are meant to provide baseline configuration.
    Override the corresponding ` + "`_custom`" + ` variable instead to add your customizations
    while preserving the defaults.
{{- end }}

{{- if .HasInstances }}
=== "Role Level"
{{- end }}

{{- range $sectionName := .SectionOrder }}
{{- $section := index $.Sections $sectionName }}
{{- if $section }}

### {{ $sectionName }}

{{- range $var := $section.Variables }}

` + "```yaml" + `
{{- range $line := $var.CommentLines }}
# {{ $line }}
{{- end }}
{{ $var.Name }}: {{ if $var.IsMultiline }}{{ renderMultilineValue $var.Name $var.ValueLines }}{{- else }}{{ $var.RawValue }}{{- end }}
` + "```" + `

{{- end }}

{{- range $subName := $section.SubsectionOrder }}
{{- $subVars := index $section.Subsections $subName }}
{{- if $subVars }}

#### {{ $subName }}

{{- range $var := $subVars }}

` + "```yaml" + `
{{- range $line := $var.CommentLines }}
# {{ $line }}
{{- end }}
{{ $var.Name }}: {{ if $var.IsMultiline }}{{ renderMultilineValue $var.Name $var.ValueLines }}{{- else }}{{ $var.RawValue }}{{- end }}
` + "```" + `

{{- end }}
{{- end }}
{{- end }}

{{- end }}
{{- end }}

{{- if .HasInstances }}

=== "Instance Level"

For a {{ .RoleName }} instance named ` + "`{{ .InstanceName }}`" + `:

{{- range $sectionName := .SectionOrder }}
{{- $section := index $.Sections $sectionName }}
{{- if $section }}

### {{ $sectionName }}

{{- range $var := $section.Variables }}

` + "```yaml" + `
{{- range $line := $var.CommentLines }}
# {{ $line }}
{{- end }}
{{ $var.InstanceName }}: {{ if $var.IsMultiline }}{{ renderMultilineValueAdjusted $var.Name $var.InstanceName $var.ValueLines }}{{- else }}{{ $var.RawValue }}{{- end }}
` + "```" + `

{{- end }}

{{- range $subName := $section.SubsectionOrder }}
{{- $subVars := index $section.Subsections $subName }}
{{- if $subVars }}

#### {{ $subName }}

{{- range $var := $subVars }}

` + "```yaml" + `
{{- range $line := $var.CommentLines }}
# {{ $line }}
{{- end }}
{{ $var.InstanceName }}: {{ if $var.IsMultiline }}{{ renderMultilineValueAdjusted $var.Name $var.InstanceName $var.ValueLines }}{{- else }}{{ $var.RawValue }}{{- end }}
` + "```" + `

{{- end }}
{{- end }}
{{- end }}

{{- end }}
{{- end }}

{{- end }}
`
}

// DefaultScaffoldTemplate returns the default template for scaffolding new docs.
func DefaultScaffoldTemplate() string {
	return `---
saltbox_automation:
  app_links:
    - name: Project home
      url: "https://{{ .RoleName }}.com"
      icon: ":material-home:"
  project_description:
    name: "{{ .RoleName | title }}"
    summary: "TODO: Add description"
    link: "https://{{ .RoleName }}.com"
---

# {{ .RoleName | title }}

## Overview

TODO: Add overview

## Deployment

` + "```shell" + `
sb install {{ .TagPrefix }}{{ .RoleTag }}
` + "```" + `

<!-- BEGIN SALTBOX MANAGED VARIABLES SECTION -->
<!-- END SALTBOX MANAGED VARIABLES SECTION -->
`
}
