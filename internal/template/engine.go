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
func (e *Engine) Render(name string, data any) (string, error) {
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
func (e *Engine) RenderString(content string, data any) (string, error) {
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
