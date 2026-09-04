package document

import (
	"bytes"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Frontmatter represents the parsed frontmatter from a documentation file.
type Frontmatter struct {
	Raw               string                   // Raw frontmatter YAML
	Icon              string                   `yaml:"icon"`
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

// AppLinkPurpose describes the semantic destination of an app link.
type AppLinkPurpose string

const (
	AppLinkPurposeManual    AppLinkPurpose = "manual"
	AppLinkPurposeRelease   AppLinkPurpose = "release"
	AppLinkPurposeCommunity AppLinkPurpose = "community"
	AppLinkPurposeOther     AppLinkPurpose = "other"
)

// RequiresURL reports whether links with this purpose require a URL.
func (p AppLinkPurpose) RequiresURL() bool {
	return p == AppLinkPurposeRelease || p == AppLinkPurposeOther
}

func (p AppLinkPurpose) valid() bool {
	switch p {
	case AppLinkPurposeManual, AppLinkPurposeRelease, AppLinkPurposeCommunity, AppLinkPurposeOther:
		return true
	default:
		return false
	}
}

// AppLink represents a project link for the overview table.
type AppLink struct {
	Name    string         `yaml:"name"`
	URL     string         `yaml:"url"`
	Type    string         `yaml:"type,omitempty"`
	Purpose AppLinkPurpose `yaml:"purpose"`
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
	data := []byte(content)
	span, found, err := locateFrontmatter(data)
	if err != nil {
		return nil, content, err
	}
	if !found {
		return nil, content, nil
	}

	rawFrontmatter := data[span.yamlStart:span.yamlEnd]
	remainingContent := string(data[span.bodyStart:])

	// Parse the YAML
	var fm Frontmatter
	fm.Raw = string(rawFrontmatter)

	if err := yaml.Unmarshal(rawFrontmatter, &fm); err != nil {
		return nil, content, fmt.Errorf("parsing frontmatter YAML: %w", err)
	}

	return &fm, remainingContent, nil
}

type frontmatterSpan struct {
	yamlStart int
	yamlEnd   int
	bodyStart int
	lineBreak []byte
}

func locateFrontmatter(content []byte) (frontmatterSpan, bool, error) {
	if !bytes.HasPrefix(content, []byte("---")) {
		return frontmatterSpan{}, false, nil
	}

	openingEnd, next, lineBreak := frontmatterLine(content, 0)
	if !bytes.Equal(content[:openingEnd], []byte("---")) {
		return frontmatterSpan{}, true, fmt.Errorf("malformed frontmatter: opening delimiter must be a standalone ---")
	}
	if len(lineBreak) == 0 {
		return frontmatterSpan{}, true, fmt.Errorf("unclosed frontmatter: missing closing ---")
	}

	for lineStart := next; lineStart <= len(content); {
		lineEnd, following, closingBreak := frontmatterLine(content, lineStart)
		if bytes.Equal(content[lineStart:lineEnd], []byte("---")) {
			return frontmatterSpan{
				yamlStart: next,
				yamlEnd:   lineStart,
				bodyStart: following,
				lineBreak: lineBreak,
			}, true, nil
		}
		if len(closingBreak) == 0 {
			break
		}
		lineStart = following
	}

	return frontmatterSpan{}, true, fmt.Errorf("unclosed frontmatter: missing closing ---")
}

func frontmatterLine(content []byte, start int) (end, next int, lineBreak []byte) {
	if start >= len(content) {
		return len(content), len(content), nil
	}
	newline := bytes.IndexByte(content[start:], '\n')
	if newline == -1 {
		return len(content), len(content), nil
	}
	end = start + newline
	if end > start && content[end-1] == '\r' {
		return end - 1, end + 1, content[end-1 : end+1]
	}
	return end, end + 1, content[end : end+1]
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
