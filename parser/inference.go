package parser

import (
	"bufio"
	"os"
	"regexp"
	"strings"

	"github.com/saltyorg/docs-automation/config"
)

var (
	// Value patterns for direct type detection
	boolTrueRe  = regexp.MustCompile(`^(true|True|TRUE|yes|Yes|YES)$`)
	boolFalseRe = regexp.MustCompile(`^(false|False|FALSE|no|No|NO)$`)
	intRe       = regexp.MustCompile(`^-?\d+$`)
	floatRe     = regexp.MustCompile(`^-?\d+\.\d+$`)
	listRe      = regexp.MustCompile(`^\[.*\]$`)
	dictRe      = regexp.MustCompile(`^\{.*\}$`)

	// role_var lookup pattern
	roleVarLookupRe = regexp.MustCompile(`lookup\s*\(\s*['"]role_var['"]\s*,\s*['"]([^'"]+)['"]`)

	// Line context patterns for role_var type inference
	defaultQuotedRe   = regexp.MustCompile(`default=['"]`)
	defaultBoolRe     = regexp.MustCompile(`(?i)default=(false|true)\b`)
	defaultDictOmitRe = regexp.MustCompile(`default=(\{\}|omit)`)
	defaultListRe     = regexp.MustCompile(`default=\[\]`)
)

// TypeInferrer handles variable type inference.
type TypeInferrer struct {
	cfg *config.TypeInferenceConfig
}

// NewTypeInferrer creates a new type inferrer with the given configuration.
func NewTypeInferrer(cfg *config.TypeInferenceConfig) *TypeInferrer {
	return &TypeInferrer{cfg: cfg}
}

// InferType determines the type of a variable based on its name and value.
func (t *TypeInferrer) InferType(name, value string) string {
	// First, check for exact suffix matches in config
	if t.cfg != nil {
		for suffix, typ := range t.cfg.Exact {
			if strings.HasSuffix(name, suffix) {
				return typ
			}
		}
	}

	// Check for type overrides in config
	if t.cfg != nil {
		for suffix, typ := range t.cfg.Overrides {
			if strings.HasSuffix(name, suffix) {
				return typ
			}
		}
	}

	// Try to infer from value (pass original value to preserve leading newlines for block detection)
	if typ := t.inferFromValue(value); typ != "" {
		return typ
	}

	// Check pattern-based inference from config
	if t.cfg != nil {
		for _, pattern := range t.cfg.Patterns {
			if strings.Contains(name, pattern.SuffixContains) {
				return pattern.Type
			}
		}
	}

	// Fallback pattern-based inference
	return t.inferFromNamePattern(name)
}

// InferRoleTypes infers a role's variables together so expressions can reuse
// types already proven for same-role variables and declared external suffixes.
func (t *TypeInferrer) InferRoleTypes(roleName string, variables []Variable, referenceTypes map[string]string) map[string]string {
	types := make(map[string]string, len(variables))
	filterTypes := map[string]string(nil)
	if t.cfg != nil {
		filterTypes = t.cfg.Filters
	}

	for _, variable := range variables {
		analysisValue := stripInlineYAMLComment(variable.RawValue)
		typ := t.InferType(variable.Name, analysisValue)
		_, pureJinja := unwrapPureJinjaExpression(analysisValue)
		if typ != String || !pureJinja || t.hasConfiguredType(variable.Name) {
			types[variable.Name] = typ
		}
	}

	for range len(variables) {
		progress := false
		resolver := func(expr string) (string, bool) {
			expr = strings.TrimSpace(expr)
			if isJinjaIdentifier(expr) {
				if typ, ok := types[expr]; ok {
					return typ, true
				}
				typ, ok := referenceTypes[expr]
				return typ, ok
			}
			matches := roleVarLookupRe.FindStringSubmatch(expr)
			if matches == nil || !strings.HasPrefix(expr, "lookup") {
				return "", false
			}
			suffix := matches[1]
			for _, candidate := range []string{roleName + "_role" + suffix, roleName + suffix} {
				if typ, ok := types[candidate]; ok {
					return typ, true
				}
			}
			typ, ok := referenceTypes[suffix]
			return typ, ok
		}

		for _, variable := range variables {
			if _, exists := types[variable.Name]; exists {
				continue
			}
			typ, ok := inferJinjaExpressionType(stripInlineYAMLComment(variable.RawValue), filterTypes, resolver)
			if !ok {
				continue
			}
			types[variable.Name] = typ
			progress = true
		}
		if !progress {
			break
		}
	}

	for _, variable := range variables {
		if _, exists := types[variable.Name]; !exists {
			types[variable.Name] = t.InferType(variable.Name, variable.RawValue)
		}
	}
	return types
}

func (t *TypeInferrer) hasConfiguredType(name string) bool {
	if t.cfg == nil {
		return false
	}
	for suffix := range t.cfg.Exact {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	for suffix := range t.cfg.Overrides {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func isJinjaIdentifier(expr string) bool {
	if expr == "" || !isJinjaIdentifierStart(expr[0]) {
		return false
	}
	for i := 1; i < len(expr); i++ {
		if !isJinjaIdentifierPart(expr[i]) {
			return false
		}
	}
	return true
}

func isJinjaIdentifierStart(ch byte) bool {
	return ch == '_' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z'
}

func isJinjaIdentifierPart(ch byte) bool {
	return isJinjaIdentifierStart(ch) || ch >= '0' && ch <= '9'
}

// inferFromValue attempts to determine type from the raw value.
// This follows Python's approach: infer primarily from value type, not name patterns.
func (t *TypeInferrer) inferFromValue(value string) string {
	value = stripInlineYAMLComment(value)
	// Check for multiline values first
	if strings.Contains(value, "\n") {
		lines := strings.Split(value, "\n")
		firstLine := strings.TrimSpace(lines[0])

		// Empty first line with indented content = block dict or list
		if firstLine == "" && len(lines) > 1 {
			for _, line := range lines[1:] {
				firstContent := strings.TrimSpace(line)
				if firstContent == "" || strings.HasPrefix(firstContent, "#") {
					continue
				}
				// Block list starts with -
				if strings.HasPrefix(firstContent, "-") {
					return List
				}
				// Block dict has key: value pairs
				if strings.Contains(firstContent, ":") {
					return Dict
				}
				break
			}
		}

		// Block list indicator
		if strings.HasPrefix(firstLine, "-") {
			return List
		}
	}

	trimmedValue := strings.TrimSpace(value)

	// Null values
	if trimmedValue == "" || trimmedValue == "~" || trimmedValue == "null" {
		return "null"
	}

	// Empty strings (quoted)
	if trimmedValue == "\"\"" || trimmedValue == "''" {
		return String
	}

	// Boolean literals
	if boolTrueRe.MatchString(trimmedValue) || boolFalseRe.MatchString(trimmedValue) {
		return Bool
	}

	// Integer
	if intRe.MatchString(trimmedValue) {
		return Int
	}

	// Float
	if floatRe.MatchString(trimmedValue) {
		return "float"
	}

	// List (flow style)
	if listRe.MatchString(trimmedValue) {
		return List
	}

	// Dict (flow style)
	if dictRe.MatchString(trimmedValue) {
		return Dict
	}

	// Block list (starts with -)
	if strings.HasPrefix(trimmedValue, "-") || strings.HasPrefix(trimmedValue, "  -") {
		return List
	}

	// Pure Jinja expressions can preserve native collection types in Ansible.
	filterTypes := map[string]string(nil)
	if t.cfg != nil {
		filterTypes = t.cfg.Filters
	}
	if typ, ok := inferJinjaExpressionType(trimmedValue, filterTypes, nil); ok {
		return typ
	}

	// Quoted strings or Jinja expressions are strings
	if strings.HasPrefix(trimmedValue, "\"") || strings.HasPrefix(trimmedValue, "'") ||
		strings.Contains(trimmedValue, "{{") {
		return String
	}

	// Default: treat as string (matches Python behavior for unknown types)
	return String
}

func stripInlineYAMLComment(value string) string {
	var quote byte
	escaped := false
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		if ch == '#' && (i == 0 || value[i-1] == ' ' || value[i-1] == '\t') && strings.TrimSpace(value[:i]) != "" {
			return strings.TrimRight(value[:i], " \t\r\n")
		}
	}
	return value
}

// inferFromNamePattern infers type from variable name patterns.
func (t *TypeInferrer) inferFromNamePattern(name string) string {
	lower := strings.ToLower(name)

	// Boolean patterns
	if strings.HasSuffix(lower, "_enabled") ||
		strings.HasSuffix(lower, "_proxy") ||
		strings.HasSuffix(lower, "_insecure") {
		return "bool (true/false)"
	}

	// String patterns
	if strings.HasSuffix(lower, "_domain") ||
		strings.HasSuffix(lower, "_subdomain") ||
		strings.HasSuffix(lower, "_url") ||
		strings.HasSuffix(lower, "_path") ||
		strings.HasSuffix(lower, "_location") ||
		strings.HasSuffix(lower, "_folder") ||
		strings.HasSuffix(lower, "_name") ||
		strings.HasSuffix(lower, "_container") ||
		strings.HasSuffix(lower, "_image") ||
		strings.HasSuffix(lower, "_tag") ||
		strings.HasSuffix(lower, "_repo") ||
		strings.HasSuffix(lower, "_record") ||
		strings.HasSuffix(lower, "_zone") ||
		strings.HasSuffix(lower, "_token") ||
		strings.HasSuffix(lower, "_theme") {
		return String
	}

	// Numeric patterns
	if strings.HasSuffix(lower, "_port") ||
		strings.HasSuffix(lower, "_timeout") {
		return StringNumber
	}

	// Scheme pattern
	if strings.HasSuffix(lower, "_scheme") {
		return StringHTTPHTTPS
	}

	// List patterns
	if strings.HasSuffix(lower, "_list") ||
		strings.HasSuffix(lower, "_ports") ||
		strings.HasSuffix(lower, "_volumes") ||
		strings.HasSuffix(lower, "_networks") ||
		strings.HasSuffix(lower, "_labels") ||
		strings.HasSuffix(lower, "_devices") ||
		strings.HasSuffix(lower, "_addons") ||
		strings.HasSuffix(lower, "_instances") {
		return List
	}

	// Dict patterns
	if strings.HasSuffix(lower, "_envs") ||
		strings.HasSuffix(lower, "_dict") ||
		strings.HasSuffix(lower, "_options") ||
		strings.HasSuffix(lower, "_labels") {
		return Dict
	}

	// Default to string
	return String
}

// ExtractRoleVarLookups finds all role_var lookup suffixes in a value.
func ExtractRoleVarLookups(value string) []string {
	matches := roleVarLookupRe.FindAllStringSubmatch(value, -1)
	var suffixes []string
	seen := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 1 && !seen[match[1]] {
			suffixes = append(suffixes, match[1])
			seen[match[1]] = true
		}
	}
	return suffixes
}

// InferRoleVarType determines the type for a role_var lookup suffix.
// This uses the suffix name and line context to infer the type, matching Python's TYPE_INFERENCE_RULES.
func InferRoleVarType(suffix, line string) string {
	// Exact suffix matches first (order matters - most specific first)
	if suffix == "_depends_on_healthchecks" {
		return StringTrueFalse
	}
	if suffix == "_depends_on_delay" {
		return StringNumber
	}
	if suffix == "_depends_on" {
		return String
	}

	// Pattern matches on suffix
	if strings.Contains(suffix, "_scheme") {
		return StringHTTPHTTPS
	}
	if strings.Contains(suffix, "_enabled") || strings.Contains(suffix, "_proxy") {
		return Bool
	}
	if strings.Contains(suffix, "_domain") || strings.Contains(suffix, "_subdomain") || strings.Contains(suffix, "_url") {
		return String
	}
	if strings.Contains(suffix, "_port") || strings.Contains(suffix, "_timeout") {
		return StringNumber
	}

	// Line context checks
	if strings.Contains(line, "| bool") {
		return Bool
	}
	if defaultQuotedRe.MatchString(line) {
		return String
	}
	if defaultBoolRe.MatchString(line) {
		return Bool
	}
	if defaultDictOmitRe.MatchString(line) {
		return DictOmit
	}
	if defaultListRe.MatchString(line) {
		return List
	}

	// Default to string (matches Python behavior)
	return String
}

// ScanInventoryForRoleVarLookups scans the inventory file for all role_var lookups.
// It returns a map of suffix -> inferred type, excluding ignored suffixes.
func ScanInventoryForRoleVarLookups(inventoryPath string, ignoreSuffixes []string) (map[string]string, error) {
	lookups := make(map[string]string)

	file, err := os.Open(inventoryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return lookups, nil // Return empty map if file doesn't exist
		}
		return nil, err
	}
	defer func() { _ = file.Close() }()

	// Build ignore set for quick lookup
	ignoreSet := make(map[string]bool)
	for _, suffix := range ignoreSuffixes {
		ignoreSet[suffix] = true
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Find all role_var lookups in this line
		for _, match := range roleVarLookupRe.FindAllStringSubmatch(line, -1) {
			if len(match) > 1 {
				suffix := match[1]

				// Skip ignored suffixes
				if ignoreSet[suffix] {
					continue
				}

				// Infer type from context
				inferredType := InferRoleVarType(suffix, line)

				// Store or update (keep more specific type if already exists)
				if existing, exists := lookups[suffix]; !exists || existing == "string" {
					lookups[suffix] = inferredType
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lookups, nil
}
