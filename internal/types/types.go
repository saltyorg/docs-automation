// Package types defines variable type constants used throughout the application.
package types

// Variable type constants for consistent type representation.
const (
	// Basic types
	Bool   = "bool"
	Int    = "int"
	String = "string"
	List   = "list"
	Dict   = "dict"

	// Compound types
	DictOmit = "dict/omit"

	// String subtypes with constraints
	StringTrueFalse = "string (true/false)"
	StringNumber    = "string (number)"
	StringHTTPHTTPS = "string (http/https)"
)

// TypeComment returns a user-friendly type comment for documentation.
func TypeComment(typ string) string {
	switch typ {
	case Bool:
		return "# Type: bool (true/false)"
	case StringTrueFalse:
		return `# Type: string ("true"/"false")`
	case StringNumber:
		return `# Type: string (quoted number)`
	case StringHTTPHTTPS:
		return `# Type: string ("http"/"https")`
	case "":
		return ""
	default:
		return "# Type: " + typ
	}
}

// keywordOverrides maps specific types to their annotation keywords.
// This allows compound types to display a simpler keyword in admonitions.
var keywordOverrides = map[string]string{
	DictOmit: Dict, // "dict/omit" -> "dict" in annotations
}

// Keyword extracts just the type keyword for use in annotations.
// For example "bool (true/false)" -> "bool", "dict/omit" -> "dict"
func Keyword(typ string) string {
	if typ == "" {
		return String
	}
	// Check for explicit overrides first
	if keyword, ok := keywordOverrides[typ]; ok {
		return keyword
	}
	// Take just the first word (before any space or parenthesis)
	for i, ch := range typ {
		if ch == ' ' || ch == '(' {
			return typ[:i]
		}
	}
	return typ
}
