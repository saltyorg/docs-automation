package template

import (
	"sort"
	"strings"

	"github.com/saltyorg/docs-automation/internal/config"
	"github.com/saltyorg/docs-automation/internal/docs"
	"github.com/saltyorg/docs-automation/internal/parser"
)

// RoleData contains all data needed to render role documentation.
type RoleData struct {
	// Role identification
	RoleName string
	RepoType string // "saltbox" or "sandbox"

	// Multi-instance support
	HasInstances bool
	InstancesVar string
	InstanceName string // Example instance name (e.g., "plex2")

	// Variables organized by section
	Sections     map[string]*SectionData
	SectionOrder []string

	// Flags
	HasDefaultVars bool

	// Global override options found via role_var lookups
	RoleVarLookups map[string]*GlobalOverrideVar // suffix -> override var info

	// Docker+ variables (from resources/tasks/docker/*.yml)
	DockerInfo *DockerInfo

	// Example override (first variable for example display)
	ExampleVar   string
	ExampleValue string

	// Frontmatter configuration (from doc file)
	Config *docs.SaltboxAutomationConfig

	// Global config for type inference
	GlobalConfig *config.Config
}

// SectionData represents a section for template rendering.
type SectionData struct {
	Name            string
	Variables       []*VariableData
	Subsections     map[string][]*VariableData
	SubsectionOrder []string
}

// HasContent returns true if the section has any variables.
func (s *SectionData) HasContent() bool {
	if len(s.Variables) > 0 {
		return true
	}
	for _, vars := range s.Subsections {
		if len(vars) > 0 {
			return true
		}
	}
	return false
}

// HasSubsections returns true if the section has named subsections.
func (s *SectionData) HasSubsections() bool {
	return len(s.SubsectionOrder) > 0
}

// VariableData represents a variable for template rendering.
type VariableData struct {
	Name         string
	RawValue     string
	Type         string
	Comment      string
	CommentLines []string
	IsMultiline  bool
	ValueLines   []string
	InstanceName string // Instance-level variable name
}

// DockerInfo contains Docker+ variable information.
type DockerInfo struct {
	Categories    map[string][]string // category -> list of var suffixes
	CategoryOrder []string
}

// GlobalOverrideVar contains information about a global override variable.
type GlobalOverrideVar struct {
	Suffix      string // The suffix (e.g., "_web_host_override")
	Type        string // Inferred or configured type
	Description string // Description from config (optional)
	Default     string // Default value from config (optional)
	HasDefault  bool   // Whether a default was explicitly set in config
	Example     string // Example from config (optional)
}

// BuildRoleData creates RoleData from parsed role information.
func BuildRoleData(role *parser.RoleInfo, cfg *config.Config, fmConfig *docs.SaltboxAutomationConfig) *RoleData {
	data := &RoleData{
		RoleName:       role.Name,
		RepoType:       role.RepoType,
		HasInstances:   role.HasInstances,
		InstancesVar:   role.InstancesVar,
		InstanceName:   role.Name + "2", // Default instance name
		Sections:       make(map[string]*SectionData),
		SectionOrder:   role.SectionOrder,
		HasDefaultVars: role.HasDefaultVars,
		RoleVarLookups: make(map[string]*GlobalOverrideVar),
		Config:         fmConfig,
		GlobalConfig:   cfg,
	}

	// Build type inferrer
	var typeInfer *parser.TypeInferrer
	if cfg != nil {
		typeInfer = parser.NewTypeInferrer(&cfg.TypeInference)
	} else {
		typeInfer = parser.NewTypeInferrer(nil)
	}

	// Build set of base variables to hide (those with both _default and _custom variants)
	hideBase := parser.BuildHideBaseSet(role.AllVariables)

	// Collect docker variables defined in the role
	var roleDockerVars []string
	for _, v := range role.AllVariables {
		if strings.Contains(v.Name, "_docker_") {
			roleDockerVars = append(roleDockerVars, v.Name)
		}
	}

	// Process sections
	for _, sectionName := range role.SectionOrder {
		section := role.Sections[sectionName]
		if section == nil {
			continue
		}

		// Check if section should be shown
		if fmConfig != nil && !fmConfig.ShouldShowSection(sectionName) {
			continue
		}

		sectionData := &SectionData{
			Name:            sectionName,
			Variables:       make([]*VariableData, 0),
			Subsections:     make(map[string][]*VariableData),
			SubsectionOrder: section.SubsectionOrder,
		}

		// Process section variables
		for _, v := range section.Variables {
			// Skip base variables that have _default/_custom variants
			if hideBase[v.Name] {
				continue
			}
			varData := buildVariableData(&v, role.Name, data.InstanceName, typeInfer, fmConfig)
			sectionData.Variables = append(sectionData.Variables, varData)

			// Collect role_var lookups (will be enriched later with config data)
			for _, suffix := range parser.ExtractRoleVarLookups(v.RawValue) {
				if _, exists := data.RoleVarLookups[suffix]; !exists {
					data.RoleVarLookups[suffix] = &GlobalOverrideVar{
						Suffix: suffix,
						Type:   parser.InferRoleVarType(suffix, v.RawValue),
					}
				}
			}
		}

		// Process subsections
		for _, subName := range section.SubsectionOrder {
			vars := section.Subsections[subName]
			for _, v := range vars {
				// Skip base variables that have _default/_custom variants
				if hideBase[v.Name] {
					continue
				}
				varData := buildVariableData(&v, role.Name, data.InstanceName, typeInfer, fmConfig)
				sectionData.Subsections[subName] = append(sectionData.Subsections[subName], varData)

				// Collect role_var lookups (will be enriched later with config data)
				for _, suffix := range parser.ExtractRoleVarLookups(v.RawValue) {
					if _, exists := data.RoleVarLookups[suffix]; !exists {
						data.RoleVarLookups[suffix] = &GlobalOverrideVar{
							Suffix: suffix,
							Type:   parser.InferRoleVarType(suffix, v.RawValue),
						}
					}
				}
			}
		}

		data.Sections[sectionName] = sectionData
	}

	// Build DockerInfo if the role has docker variables
	if len(roleDockerVars) > 0 && cfg != nil {
		data.DockerInfo = buildDockerInfo(cfg, role.Name, roleDockerVars)
	}

	// Scan inventory file for all role_var lookups and merge them in
	if cfg != nil {
		inventoryLookups, err := parser.ScanInventoryForRoleVarLookups(
			cfg.InventoryPath(),
			cfg.GlobalOverrides.IgnoreSuffixes,
		)
		if err == nil {
			for suffix, varType := range inventoryLookups {
				if _, exists := data.RoleVarLookups[suffix]; !exists {
					data.RoleVarLookups[suffix] = &GlobalOverrideVar{
						Suffix: suffix,
						Type:   varType,
					}
				}
			}
		}

		// Enrich all RoleVarLookups with config data (description, default, example)
		for suffix, overrideVar := range data.RoleVarLookups {
			if varDef, exists := cfg.GlobalOverrides.Variables[suffix]; exists {
				overrideVar.Description = varDef.Description
				// Handle pointer for default - nil means no default, non-nil means explicit default (even if empty)
				if varDef.Default != nil {
					overrideVar.Default = *varDef.Default
					overrideVar.HasDefault = true
				}
				overrideVar.Example = varDef.Example
				// Use config type if available (more accurate than inferred)
				if varDef.Type != "" {
					overrideVar.Type = varDef.Type
				}
			}
		}

		// Filter RoleVarLookups based on role sections
		// Remove overrides that don't apply to this role
		data.RoleVarLookups = filterRoleVarLookups(data.RoleVarLookups, role)
	}

	// Set ExampleVar and ExampleValue for non-instance roles
	if !role.HasInstances {
		// Find the first variable to use as example
		for _, sectionName := range data.SectionOrder {
			section := data.Sections[sectionName]
			if section == nil {
				continue
			}
			// Check section variables first
			if len(section.Variables) > 0 {
				data.ExampleVar = section.Variables[0].Name
				data.ExampleValue = getExampleValue(section.Variables[0].Type)
				break
			}
			// Then check subsections
			for _, subName := range section.SubsectionOrder {
				if vars := section.Subsections[subName]; len(vars) > 0 {
					data.ExampleVar = vars[0].Name
					data.ExampleValue = getExampleValue(vars[0].Type)
					break
				}
			}
			if data.ExampleVar != "" {
				break
			}
		}
	}

	return data
}

// getExampleValue returns an appropriate example value for a given type.
func getExampleValue(varType string) string {
	switch varType {
	case "bool":
		return "true"
	case "int":
		return "42"
	case "list":
		return "[\"item1\", \"item2\"]"
	case "dict":
		return "{}"
	default:
		return "\"custom_value\""
	}
}

// buildDockerInfo creates DockerInfo with additional docker variables not defined in the role.
func buildDockerInfo(cfg *config.Config, roleName string, roleDockerVars []string) *DockerInfo {
	// Get the resources path from config (saltbox repo)
	resourcesPath := cfg.Repositories.Saltbox + "/resources"

	scanner := parser.NewDockerVarScanner(resourcesPath)
	additionalVars, err := scanner.GetDockerVarSuffixes(roleName, roleDockerVars)
	if err != nil || len(additionalVars) == 0 {
		return nil
	}

	// Sort for consistent output
	sort.Strings(additionalVars)

	// Categorize the variables
	categories := parser.CategorizeDockerVars(additionalVars)

	// Only include non-empty categories
	filteredCategories := make(map[string][]string)
	for cat, vars := range categories {
		if len(vars) > 0 {
			sort.Strings(vars)
			filteredCategories[cat] = vars
		}
	}

	if len(filteredCategories) == 0 {
		return nil
	}

	return &DockerInfo{
		Categories:    filteredCategories,
		CategoryOrder: parser.DockerVarCategoryOrder(),
	}
}

// buildVariableData creates VariableData from a parsed Variable.
func buildVariableData(v *parser.Variable, roleName, instanceName string, typeInfer *parser.TypeInferrer, fmConfig *docs.SaltboxAutomationConfig) *VariableData {
	// Check for example override
	rawValue := v.RawValue
	if fmConfig != nil {
		if override, ok := fmConfig.GetExampleOverride(v.Name); ok {
			rawValue = override
		}
	}

	// Infer type
	typ := typeInfer.InferType(v.Name, rawValue)

	// Generate instance name
	instName := parser.GenerateInstanceName(v.Name, roleName, instanceName)

	// Split comment into lines
	var commentLines []string
	if v.Comment != "" {
		commentLines = splitLines(v.Comment)
	}

	return &VariableData{
		Name:         v.Name,
		RawValue:     rawValue,
		Type:         typ,
		Comment:      v.Comment,
		CommentLines: commentLines,
		IsMultiline:  v.IsMultiline,
		ValueLines:   v.ValueLines,
		InstanceName: instName,
	}
}

// splitLines splits a string into lines.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// filterRoleVarLookups removes global override options that don't apply to the role
// based on which sections the role has.
func filterRoleVarLookups(lookups map[string]*GlobalOverrideVar, role *parser.RoleInfo) map[string]*GlobalOverrideVar {
	filtered := make(map[string]*GlobalOverrideVar)
	for suffix, overrideVar := range lookups {
		// Check if this override applies to the role based on its sections
		suffixLower := strings.ToLower(suffix)

		// Web-related overrides require Web section
		if strings.Contains(suffixLower, "_web_") && !role.HasWeb {
			continue
		}

		// Traefik-related overrides require Traefik section
		// This includes _traefik_* and _themepark_* suffixes
		if (strings.Contains(suffixLower, "_traefik_") || strings.Contains(suffixLower, "_themepark_")) && !role.HasTraefik {
			continue
		}

		// Docker-related overrides require Docker section
		// This includes _docker_*, _autoheal_*, _depends_on*, and _diun_* suffixes
		if !role.HasDocker {
			if strings.Contains(suffixLower, "_docker_") ||
				strings.Contains(suffixLower, "_autoheal_") ||
				strings.Contains(suffixLower, "_depends_on") ||
				strings.Contains(suffixLower, "_diun_") {
				continue
			}
		}

		// DNS-related overrides require DNS section
		if strings.Contains(suffixLower, "_dns_") && !role.HasDNS {
			continue
		}

		filtered[suffix] = overrideVar
	}

	return filtered
}
