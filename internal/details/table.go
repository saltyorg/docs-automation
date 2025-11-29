package details

import (
	"bytes"
	"fmt"
	"os"
	"strings"
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
	Rows [][]string // Each row contains up to 3 formatted link cells
}

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

	tmpl, err := template.New("details").Parse(string(content))
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

	// Build rows of formatted links (3 per row)
	links := automation.AppLinks
	var rows [][]string

	for i := 0; i < len(links); i += 3 {
		row := make([]string, 3)
		for j := range 3 {
			if i+j < len(links) {
				row[j] = formatLink(links[i+j])
			} else {
				row[j] = ""
			}
		}
		rows = append(rows, row)
	}

	data := TableData{Rows: rows}

	var buf bytes.Buffer
	if err := g.tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}

// formatLink formats a single app link for the table.
func formatLink(link docs.AppLink) string {
	var builder strings.Builder

	builder.WriteString("[")

	// Add icon if present
	if link.Icon != "" {
		builder.WriteString(link.Icon)
		builder.WriteString(" ")
	}

	builder.WriteString(link.Name)
	builder.WriteString("](")
	builder.WriteString(link.URL)
	builder.WriteString("){: .header-icons }")

	return builder.String()
}

// GenerateFromDocument generates an overview table for a document.
// Returns empty string if document has no frontmatter or no app links.
func (g *TableGenerator) GenerateFromDocument(doc *docs.Document) (string, error) {
	if doc.Frontmatter == nil || doc.Frontmatter.SaltboxAutomation == nil {
		return "", nil
	}

	return g.Generate(doc.Frontmatter.SaltboxAutomation)
}
