package document

import (
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Frontmatter represents the parsed frontmatter from a documentation file.
type Frontmatter struct {
	Raw               string                   // Raw frontmatter YAML
	SaltboxAutomation *SaltboxAutomationConfig `yaml:"saltbox_automation"`
	Status            string                   `yaml:"status"`
}

// SaltboxAutomationConfig represents the saltbox_automation frontmatter section.
type SaltboxAutomationConfig struct {
	Disabled           bool                `yaml:"disabled"`
	Sections           SectionsConfig      `yaml:"sections"`
	Inventory          InventoryConfig     `yaml:"inventory"`
	AppLinks           []AppLink           `yaml:"app_links"`
	ProjectDescription *ProjectDescription `yaml:"project_description"`
	Checks             PageChecks          `yaml:"checks"`
}

// PageChecks controls which docs-health checks apply to a page.
type PageChecks struct {
	Coverage    *bool `yaml:"coverage"`
	Frontmatter *bool `yaml:"frontmatter"`
	Editorial   *bool `yaml:"editorial"`
}

// SectionsConfig controls which automated sections to include.
type SectionsConfig struct {
	Inventory *bool `yaml:"inventory"`
	Overview  *bool `yaml:"overview"`
}

// InventoryConfig controls the inventory section generation.
type InventoryConfig struct {
	ShowSections     []string          `yaml:"show_sections"`
	HideSections     []string          `yaml:"hide_sections"`
	ExampleOverrides map[string]string `yaml:"example_overrides"`
}

// AppLink represents a project link for the overview table.
type AppLink struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
	Type string `yaml:"type,omitempty"`
}

// ProjectDescription contains project metadata.
type ProjectDescription struct {
	Name       string   `yaml:"name"`
	Summary    string   `yaml:"summary"`
	Link       string   `yaml:"link"`
	Categories []string `yaml:"categories"`
}

// ParseFrontmatter extracts and parses the YAML frontmatter from markdown content.
// Returns the frontmatter, the remaining content, and any error.
func ParseFrontmatter(content string) (*Frontmatter, string, error) {
	// Check for frontmatter delimiter
	if !strings.HasPrefix(content, "---") {
		return nil, content, nil
	}

	// Find the closing delimiter
	rest := content[3:]
	endIdx := strings.Index(rest, "\n---")
	if endIdx == -1 {
		return nil, content, fmt.Errorf("unclosed frontmatter: missing closing ---")
	}

	rawFrontmatter := strings.TrimSpace(rest[:endIdx])
	remainingContent := rest[endIdx+4:] // Skip past \n---

	// Skip leading newline in remaining content
	remainingContent = strings.TrimPrefix(remainingContent, "\n")

	// Parse the YAML
	var fm Frontmatter
	fm.Raw = rawFrontmatter

	if err := yaml.Unmarshal([]byte(rawFrontmatter), &fm); err != nil {
		return nil, content, fmt.Errorf("parsing frontmatter YAML: %w", err)
	}

	return &fm, remainingContent, nil
}

// IsInventorySectionEnabled returns whether the inventory section should be generated.
func (c *SaltboxAutomationConfig) IsInventorySectionEnabled() bool {
	if c == nil {
		return true
	}
	if c.Disabled {
		return false
	}
	if c.Sections.Inventory == nil {
		return true
	}
	return *c.Sections.Inventory
}

// IsOverviewSectionEnabled returns whether the overview section should be generated.
func (c *SaltboxAutomationConfig) IsOverviewSectionEnabled() bool {
	if c == nil {
		return true
	}
	if c.Disabled {
		return false
	}
	if c.Sections.Overview == nil {
		return true
	}
	return *c.Sections.Overview
}

// IsCoverageCheckEnabled reports whether coverage checking is enabled for this page.
func (c *SaltboxAutomationConfig) IsCoverageCheckEnabled() bool {
	return pageCheckEnabled(c, func(checks PageChecks) *bool {
		return checks.Coverage
	})
}

// IsFrontmatterCheckEnabled reports whether frontmatter checking is enabled for this page.
func (c *SaltboxAutomationConfig) IsFrontmatterCheckEnabled() bool {
	return pageCheckEnabled(c, func(checks PageChecks) *bool {
		return checks.Frontmatter
	})
}

// IsEditorialCheckEnabled reports whether editorial checking is enabled for this page.
func (c *SaltboxAutomationConfig) IsEditorialCheckEnabled() bool {
	return pageCheckEnabled(c, func(checks PageChecks) *bool {
		return checks.Editorial
	})
}

func pageCheckEnabled(c *SaltboxAutomationConfig, selectCheck func(PageChecks) *bool) bool {
	if c == nil {
		return true
	}
	if c.Disabled {
		return false
	}
	value := selectCheck(c.Checks)
	return value == nil || *value
}

// ShouldShowSection returns whether a given section should be shown.
func (c *SaltboxAutomationConfig) ShouldShowSection(sectionName string) bool {
	if c == nil {
		return true
	}

	// Check hide list first
	for _, s := range c.Inventory.HideSections {
		if strings.EqualFold(s, sectionName) {
			return false
		}
	}

	// If show list is specified, only show those
	if len(c.Inventory.ShowSections) > 0 {
		for _, s := range c.Inventory.ShowSections {
			if strings.EqualFold(s, sectionName) {
				return true
			}
		}
		return false
	}

	return true
}

// GetExampleOverride returns the example override for a variable, if any.
func (c *SaltboxAutomationConfig) GetExampleOverride(varName string) (string, bool) {
	if c == nil || c.Inventory.ExampleOverrides == nil {
		return "", false
	}
	val, ok := c.Inventory.ExampleOverrides[varName]
	return val, ok
}
