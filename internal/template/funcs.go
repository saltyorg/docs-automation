package template

import (
	"strings"
	"text/template"

	"github.com/saltyorg/docs-automation/internal/parser"
	"github.com/saltyorg/docs-automation/internal/types"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// FuncMap returns the template function map.
func FuncMap() template.FuncMap {
	titleCaser := cases.Title(language.English)
	return template.FuncMap{
		// String functions
		"lower":     strings.ToLower,
		"upper":     strings.ToUpper,
		"title":     titleCaser.String,
		"trimSpace": strings.TrimSpace,
		"contains":  strings.Contains,
		"hasPrefix": strings.HasPrefix,
		"hasSuffix": strings.HasSuffix,
		"replace":   strings.ReplaceAll,
		"join":      strings.Join,
		"split":     strings.Split,

		// Formatting functions
		"indent":             indent,
		"formatTypeComment":  formatTypeComment,
		"formatDefaultValue": formatDefaultValue,
		"typeKeyword":        typeKeyword,

		// Role variable functions
		"adjustMultilineIndent":        adjustMultilineIndent,
		"generateInstanceName":         generateInstanceName,
		"renderMultilineValue":         renderMultilineValue,
		"renderMultilineValueAdjusted": renderMultilineValueAdjusted,
		"getValueLines":                getValueLines,

		// Docker variable functions
		"getDockerVarType":        parser.GetDockerVarType,
		"getDockerVarTypeComment": parser.GetDockerVarTypeComment,

		// Global override variable functions
		"replaceVariable":       replaceVariable,
		"replaceRole":           replaceRole,
		"replacePlural":         replacePlural,
		"formatOverrideDefault": formatOverrideDefault,
		"hasOverrideDefault":    hasOverrideDefault,
	}
}

// indent adds n spaces of indentation to each line.
func indent(n int, s string) string {
	prefix := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

// renderMultilineValue renders just the first line of a multiline value.
// Continuation lines should be rendered separately in the template with proper indentation.
func renderMultilineValue(varName string, valueLines []string) string {
	if len(valueLines) == 0 {
		return ""
	}
	return valueLines[0]
}

// getValueLines returns the continuation lines (all lines after the first) for template iteration.
func getValueLines(valueLines []string) []string {
	if len(valueLines) <= 1 {
		return nil
	}
	return valueLines[1:]
}

// renderMultilineValueAdjusted renders a multiline value with indentation adjusted
// for a new variable name. Used for instance-level variables where the name length changes.
// Each continuation line is prefixed with 8 spaces for code block indentation, then the
// line content with adjusted indentation based on the variable name length change.
func renderMultilineValueAdjusted(originalName, newName string, valueLines []string) string {
	if len(valueLines) == 0 {
		return ""
	}

	if len(valueLines) == 1 {
		return valueLines[0]
	}

	// Calculate indentation adjustment based on variable name length change
	// Format: "var_name: " = name + ": " = len(name) + 2
	originalIndent := len(originalName) + 2
	newIndent := len(newName) + 2
	indentDiff := originalIndent - newIndent

	var result strings.Builder
	result.WriteString(valueLines[0])

	for i := 1; i < len(valueLines); i++ {
		result.WriteString("\n        ") // 8 spaces for code block indentation
		line := valueLines[i]

		// Only adjust indentation for lines that appear to be indented
		if len(line) > 0 && line[0] == ' ' {
			// Count leading spaces
			leadingSpaces := 0
			for _, ch := range line {
				if ch == ' ' {
					leadingSpaces++
				} else {
					break
				}
			}

			// Only adjust if the indentation is >= original alignment
			// (this preserves dict/list structure indentation that's less than the value alignment)
			if leadingSpaces >= originalIndent {
				newSpaces := max(leadingSpaces-indentDiff, 0)
				result.WriteString(strings.Repeat(" ", newSpaces))
				result.WriteString(strings.TrimLeft(line, " "))
			} else {
				// Structural indentation, keep as-is
				result.WriteString(line)
			}
		} else {
			// No leading space, keep as-is
			result.WriteString(line)
		}
	}

	return result.String()
}

// formatTypeComment formats a type as a YAML comment.
// This matches Python's format_type_comment behavior.
func formatTypeComment(typ string) string {
	return types.TypeComment(typ)
}

// typeKeyword extracts just the type keyword for use in ??? variable annotations.
// For example "bool (true/false)" -> "bool", "list" -> "list"
func typeKeyword(typ string) string {
	return types.Keyword(typ)
}

// formatDefaultValue formats a default value for display.
func formatDefaultValue(value, typ string) string {
	if value == "" || value == "null" {
		return "# No default"
	}

	// For bools, show the actual value
	if typ == "bool" {
		return value
	}

	return value
}

// adjustMultilineIndent adjusts the indentation of continuation lines.
func adjustMultilineIndent(lines []string, originalName, newName string) []string {
	if len(lines) <= 1 {
		return lines
	}

	diff := len(newName) - len(originalName)
	result := make([]string, len(lines))
	result[0] = strings.Replace(lines[0], originalName, newName, 1)

	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if diff == 0 {
			result[i] = line
			continue
		}

		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" {
			result[i] = line
			continue
		}

		currentIndent := len(line) - len(trimmed)
		newIndent := max(currentIndent+diff, 0)

		result[i] = strings.Repeat(" ", newIndent) + trimmed
	}

	return result
}

// generateInstanceName converts a role-level variable name to instance-level.
func generateInstanceName(varName, roleName, instanceName string) string {
	rolePrefix := roleName + "_role_"
	if after, ok := strings.CutPrefix(varName, rolePrefix); ok {
		suffix := after
		return instanceName + "_" + suffix
	}

	roleSimplePrefix := roleName + "_"
	if after, ok := strings.CutPrefix(varName, roleSimplePrefix); ok {
		suffix := after
		if suffix == "instances" {
			return varName
		}
		return instanceName + "_" + suffix
	}

	return varName
}

// replaceVariable replaces {variable} placeholder with the actual variable name.
func replaceVariable(varName, text string) string {
	return strings.ReplaceAll(text, "{variable}", varName)
}

// replaceRole replaces {role} placeholder with the actual role name.
func replaceRole(roleName, text string) string {
	return strings.ReplaceAll(text, "{role}", roleName)
}

// replacePlural replaces "containers" with "the container" for non-instance roles.
func replacePlural(text string) string {
	return strings.ReplaceAll(text, "containers", "the container")
}

// hasOverrideDefault returns true if the override variable has a default value to display.
// This handles the case where default is explicitly set (even to empty string "").
func hasOverrideDefault(defaultVal string, hasDefault bool) bool {
	// If we have a non-empty default, show it
	// The hasDefault flag indicates whether a default was explicitly set in config
	return defaultVal != "" || hasDefault
}

// formatOverrideDefault formats a default value for display in Global Override Options.
// For string types, it ensures proper quoting.
func formatOverrideDefault(defaultVal, varType string) string {
	// If default is empty string, show it as ""
	if defaultVal == "" {
		return `""`
	}

	// For string types (not bool, not list, not dict), ensure the value is quoted
	// if it's not already quoted
	if strings.HasPrefix(varType, "string") {
		// Check if already quoted
		if (strings.HasPrefix(defaultVal, `"`) && strings.HasSuffix(defaultVal, `"`)) ||
			(strings.HasPrefix(defaultVal, `'`) && strings.HasSuffix(defaultVal, `'`)) {
			return defaultVal
		}
		// Quote the value
		return `"` + defaultVal + `"`
	}

	return defaultVal
}
