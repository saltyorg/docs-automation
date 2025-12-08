package overview

import (
	"bytes"
	"fmt"
	"os"
	"text/template"

	"github.com/saltyorg/docs-automation/internal/docs"
)

// TableGenerator generates overview tables from frontmatter.
type TableGenerator struct {
	templatePath string
	tmpl         *template.Template
}

// TableData holds data for the overview table template.
type TableData struct {
	Description *docs.ProjectDescription // Project description
	Links       []docs.AppLink           // All links passed to template
}

// templateFuncs provides helper functions for templates.
// The icon mapping is defined in the template file itself.
var templateFuncs = template.FuncMap{}

// NewTableGenerator creates a new overview table generator.
func NewTableGenerator(templatePath string) *TableGenerator {
	return &TableGenerator{templatePath: templatePath}
}

// LoadTemplate loads the template from the configured path.
func (g *TableGenerator) LoadTemplate() error {
	if g.templatePath == "" {
		return fmt.Errorf("no template path configured")
	}

	content, err := os.ReadFile(g.templatePath)
	if err != nil {
		return fmt.Errorf("reading template: %w", err)
	}

	tmpl, err := template.New("overview").Funcs(templateFuncs).Parse(string(content))
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	g.tmpl = tmpl
	return nil
}

// Generate creates an overview table from app links in frontmatter.
// Returns empty string if no app links are defined.
func (g *TableGenerator) Generate(automation *docs.SaltboxAutomationConfig) (string, error) {
	if automation == nil || len(automation.AppLinks) == 0 {
		return "", nil
	}

	// Check if overview section is enabled
	if !automation.IsOverviewSectionEnabled() {
		return "", nil
	}

	if g.tmpl == nil {
		return "", fmt.Errorf("template not loaded")
	}

	data := TableData{
		Description: automation.ProjectDescription,
		Links:       automation.AppLinks,
	}

	var buf bytes.Buffer
	if err := g.tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}

// GenerateFromDocument generates an overview table for a document.
// Returns empty string if document has no frontmatter or no app links.
func (g *TableGenerator) GenerateFromDocument(doc *docs.Document) (string, error) {
	if doc.Frontmatter == nil || doc.Frontmatter.SaltboxAutomation == nil {
		return "", nil
	}

	return g.Generate(doc.Frontmatter.SaltboxAutomation)
}
