package docs

import (
	"fmt"
	"regexp"
	"strings"
)

// ManagedSection represents a section of content managed by automation.
type ManagedSection struct {
	Name       string // Section name (e.g., "SALTBOX MANAGED VARIABLES SECTION")
	Content    string // Content between markers
	StartLine  int    // Line number of start marker
	EndLine    int    // Line number of end marker
	StartIndex int    // Character index of start marker
	EndIndex   int    // Character index of end marker (after end marker)
}

// MarkerConfig defines the marker names for different section types.
type MarkerConfig struct {
	Variables string
	CLI       string
	Overview  string
}

// DefaultMarkers returns the default marker configuration.
func DefaultMarkers() MarkerConfig {
	return MarkerConfig{
		Variables: "SALTBOX MANAGED VARIABLES SECTION",
		CLI:       "SALTBOX MANAGED CLI SECTION",
		Overview:  "SALTBOX MANAGED OVERVIEW SECTION",
	}
}

// FindManagedSection finds a managed section in the given content.
// Returns nil if the section is not found.
func FindManagedSection(content, sectionName string) *ManagedSection {
	startMarker := fmt.Sprintf("<!-- BEGIN %s -->", sectionName)
	endMarker := fmt.Sprintf("<!-- END %s -->", sectionName)

	startIdx := strings.Index(content, startMarker)
	if startIdx == -1 {
		return nil
	}

	endIdx := strings.Index(content[startIdx:], endMarker)
	if endIdx == -1 {
		return nil
	}
	endIdx += startIdx + len(endMarker)

	// Extract content between markers (after start marker, before end marker)
	contentStart := startIdx + len(startMarker)
	contentEnd := endIdx - len(endMarker)

	// Calculate line numbers
	startLine := strings.Count(content[:startIdx], "\n") + 1
	endLine := strings.Count(content[:endIdx], "\n") + 1

	return &ManagedSection{
		Name:       sectionName,
		Content:    content[contentStart:contentEnd],
		StartLine:  startLine,
		EndLine:    endLine,
		StartIndex: startIdx,
		EndIndex:   endIdx,
	}
}

// UpdateManagedSection replaces the content of a managed section.
// Returns the updated full content.
func UpdateManagedSection(content, sectionName, newContent string) (string, error) {
	section := FindManagedSection(content, sectionName)
	if section == nil {
		return "", fmt.Errorf("managed section %q not found", sectionName)
	}

	startMarker := fmt.Sprintf("<!-- BEGIN %s -->", sectionName)
	endMarker := fmt.Sprintf("<!-- END %s -->", sectionName)

	// Build new content
	var builder strings.Builder
	builder.WriteString(content[:section.StartIndex])
	builder.WriteString(startMarker)
	builder.WriteString("\n")
	builder.WriteString(newContent)
	if !strings.HasSuffix(newContent, "\n") {
		builder.WriteString("\n")
	}
	builder.WriteString(endMarker)
	builder.WriteString(content[section.EndIndex:])

	return builder.String(), nil
}

// HasManagedSection checks if a managed section exists in the content.
func HasManagedSection(content, sectionName string) bool {
	return FindManagedSection(content, sectionName) != nil
}

// CreateManagedSection creates a new managed section with the given content.
// Returns the markers with content, ready to be inserted.
func CreateManagedSection(sectionName, content string) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("<!-- BEGIN %s -->\n", sectionName))
	builder.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		builder.WriteString("\n")
	}
	builder.WriteString(fmt.Sprintf("<!-- END %s -->", sectionName))
	return builder.String()
}

// ValidateManagedSections checks that all managed sections have matching markers.
func ValidateManagedSections(content string) []string {
	var errors []string

	// Find all BEGIN markers
	beginRe := regexp.MustCompile(`<!-- BEGIN ([^>]+) -->`)
	endRe := regexp.MustCompile(`<!-- END ([^>]+) -->`)

	begins := beginRe.FindAllStringSubmatch(content, -1)
	ends := endRe.FindAllStringSubmatch(content, -1)

	beginNames := make(map[string]bool)
	endNames := make(map[string]bool)

	for _, match := range begins {
		beginNames[match[1]] = true
	}
	for _, match := range ends {
		endNames[match[1]] = true
	}

	// Check for unmatched markers
	for name := range beginNames {
		if !endNames[name] {
			errors = append(errors, fmt.Sprintf("missing END marker for %q", name))
		}
	}
	for name := range endNames {
		if !beginNames[name] {
			errors = append(errors, fmt.Sprintf("missing BEGIN marker for %q", name))
		}
	}

	return errors
}
