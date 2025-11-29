package parser

import (
	"strings"
)

// BuildHideBaseSet builds a set of variable names that should be hidden
// because they have _default or _custom variants (or both).
func BuildHideBaseSet(variables []Variable) map[string]bool {
	hideBase := make(map[string]bool)

	for _, v := range variables {
		if strings.HasSuffix(v.Name, "_default") {
			base := strings.TrimSuffix(v.Name, "_default")
			hideBase[base] = true
		}
		if strings.HasSuffix(v.Name, "_custom") {
			base := strings.TrimSuffix(v.Name, "_custom")
			hideBase[base] = true
		}
	}

	return hideBase
}

// FilterVariables applies filtering rules to hide internal variables.
// It returns a new slice with filtered variables.
func FilterVariables(variables []Variable, roleName string) []Variable {
	hideBase := BuildHideBaseSet(variables)

	var filtered []Variable
	for _, v := range variables {
		// Skip variables that are base of _default/_custom pairs
		if hideBase[v.Name] {
			continue
		}

		filtered = append(filtered, v)
	}

	return filtered
}

// GenerateInstanceName converts a role-level variable name to an instance-level name.
// Example: plex_role_docker_envs -> plex2_docker_envs
func GenerateInstanceName(varName, roleName, instanceName string) string {
	// The role-level pattern is: {role}_role_{suffix}
	// The instance-level pattern is: {instance}_{suffix}
	rolePrefix := roleName + "_role_"

	if strings.HasPrefix(varName, rolePrefix) {
		suffix := strings.TrimPrefix(varName, rolePrefix)
		return instanceName + "_" + suffix
	}

	// Also handle variables like {role}_instances -> {instance}_instances doesn't make sense
	// but {role}_{suffix} -> {instance}_{suffix} does
	roleSimplePrefix := roleName + "_"
	if strings.HasPrefix(varName, roleSimplePrefix) {
		suffix := strings.TrimPrefix(varName, roleSimplePrefix)
		// Don't transform if it's the instances variable itself
		if suffix == "instances" {
			return varName
		}
		return instanceName + "_" + suffix
	}

	return varName
}

// AdjustMultilineIndent adjusts the indentation of continuation lines
// when the variable name length changes (for instance-level variables).
func AdjustMultilineIndent(lines []string, originalName, newName string) []string {
	if len(lines) <= 1 {
		return lines
	}

	// Calculate the difference in name length
	diff := len(newName) - len(originalName)

	result := make([]string, len(lines))
	result[0] = strings.Replace(lines[0], originalName, newName, 1)

	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if diff == 0 {
			result[i] = line
			continue
		}

		// Find the leading whitespace
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" {
			result[i] = line
			continue
		}

		// Calculate current indentation (spaces only for simplicity)
		currentIndent := len(line) - len(trimmed)

		// Adjust indentation
		newIndent := currentIndent + diff
		if newIndent < 0 {
			newIndent = 0
		}

		result[i] = strings.Repeat(" ", newIndent) + trimmed
	}

	return result
}

// IsInternalVariable checks if a variable should be considered internal.
func IsInternalVariable(name string) bool {
	// Variables with these patterns are typically internal
	internalPatterns := []string{
		"_lookup",
		"_dict",
		"_proxy_dict",
		"_network_modes",
	}

	for _, pattern := range internalPatterns {
		if strings.Contains(name, pattern) {
			return true
		}
	}

	return false
}
