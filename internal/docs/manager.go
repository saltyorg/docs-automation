package docs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Document represents a parsed documentation file.
type Document struct {
	Path        string
	Content     string
	Frontmatter *Frontmatter
	Body        string // Content after frontmatter
}

// Manager handles documentation file operations.
type Manager struct {
	markers MarkerConfig
}

// NewManager creates a new documentation manager.
func NewManager(markers MarkerConfig) *Manager {
	return &Manager{markers: markers}
}

// LoadDocument reads and parses a documentation file.
func (m *Manager) LoadDocument(path string) (*Document, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	fm, body, err := ParseFrontmatter(string(content))
	if err != nil {
		return nil, fmt.Errorf("parsing frontmatter: %w", err)
	}

	return &Document{
		Path:        path,
		Content:     string(content),
		Frontmatter: fm,
		Body:        body,
	}, nil
}

// SaveDocument writes the document back to disk.
func (m *Manager) SaveDocument(doc *Document) error {
	return os.WriteFile(doc.Path, []byte(doc.Content), 0644)
}

// UpdateVariablesSection updates the managed variables section in a document.
func (m *Manager) UpdateVariablesSection(doc *Document, newContent string) error {
	updated, err := UpdateManagedSection(doc.Content, m.markers.Variables, newContent)
	if err != nil {
		return err
	}
	doc.Content = updated
	return nil
}

// UpdateCLISection updates the managed CLI section in a document.
func (m *Manager) UpdateCLISection(doc *Document, newContent string) error {
	updated, err := UpdateManagedSection(doc.Content, m.markers.CLI, newContent)
	if err != nil {
		return err
	}
	doc.Content = updated
	return nil
}

// UpdateOverviewSection updates the managed overview section in a document.
func (m *Manager) UpdateOverviewSection(doc *Document, newContent string) error {
	updated, err := UpdateManagedSection(doc.Content, m.markers.Overview, newContent)
	if err != nil {
		return err
	}
	doc.Content = updated
	return nil
}

// HasVariablesSection checks if the document has the variables section markers.
func (m *Manager) HasVariablesSection(doc *Document) bool {
	return HasManagedSection(doc.Content, m.markers.Variables)
}

// HasCLISection checks if the document has the CLI section markers.
func (m *Manager) HasCLISection(doc *Document) bool {
	return HasManagedSection(doc.Content, m.markers.CLI)
}

// HasOverviewSection checks if the document has the overview section markers.
func (m *Manager) HasOverviewSection(doc *Document) bool {
	return HasManagedSection(doc.Content, m.markers.Overview)
}

// ListDocFiles returns all markdown files in a directory.
func ListDocFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(path), ".md") {
			// Skip index files
			if strings.ToLower(filepath.Base(path)) == "index.md" {
				return nil
			}
			files = append(files, path)
		}
		return nil
	})

	return files, err
}

// ExtractRoleName extracts the role name from a documentation file path.
func ExtractRoleName(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// IsAutomationDisabled checks if automation is disabled for a document.
func (m *Manager) IsAutomationDisabled(doc *Document) bool {
	if doc.Frontmatter == nil || doc.Frontmatter.SaltboxAutomation == nil {
		return false
	}
	return doc.Frontmatter.SaltboxAutomation.Disabled
}
