package parser

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

var (
	// Section header: ################################ followed by # SectionName on next line
	sectionHeaderRe = regexp.MustCompile(`^#{10,}$`)
	sectionNameRe   = regexp.MustCompile(`^#\s*(.+?)\s*$`)

	// Subsection markers: # Name - Sub-section Start / # Name - Sub-section End
	subsectionStartRe = regexp.MustCompile(`^#\s*(.+?)\s*-\s*Sub-section Start\s*$`)
	subsectionEndRe   = regexp.MustCompile(`^#\s*(.+?)\s*-\s*Sub-section End\s*$`)

	// Variable line: name: value (not starting with #)
	variableRe = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*)\s*:\s*(.*)$`)

	// Skip markers (comment text without the # prefix)
	skipDocsRe      = regexp.MustCompile(`(?i)^Skip docs`)
	skipInventoryRe = regexp.MustCompile(`(?i)^Do not edit or override using the inventory`)

	// Comment prefixes
	globalPrefixRe   = regexp.MustCompile(`^\[GLOBAL\]\s*`)
	noGlobalPrefixRe = regexp.MustCompile(`^\[NOGLOBAL\]\s*`)
)

// Parser handles parsing of Ansible role defaults files.
type Parser struct {
	roleName string
	repoType string
}

// New creates a new Parser for the given role.
func New(roleName, repoType string) *Parser {
	return &Parser{
		roleName: roleName,
		repoType: repoType,
	}
}

// ParseFile parses a defaults/main.yml file and returns role information.
func (p *Parser) ParseFile(path string) (*RoleInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	role := &RoleInfo{
		Name:         p.roleName,
		RepoType:     p.repoType,
		Sections:     make(map[string]*Section),
		SectionOrder: []string{},
		AllVariables: []Variable{},
	}

	state := &ParserState{}
	scanner := bufio.NewScanner(file)
	lineNum := 0
	var lines []string

	// First pass: collect all lines
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Second pass: parse with lookahead capability
	for lineNum < len(lines) {
		line := lines[lineNum]
		lineNum++

		// Skip empty lines (but don't clear pending comment)
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Check for section header (line of #####)
		if sectionHeaderRe.MatchString(strings.TrimSpace(line)) {
			// Look for section name on next line
			if lineNum < len(lines) {
				nextLine := lines[lineNum]
				if matches := sectionNameRe.FindStringSubmatch(strings.TrimSpace(nextLine)); matches != nil {
					sectionName := matches[1]
					// Skip header/copyright sections
					if !isMetaSection(sectionName) {
						state.CurrentSection = sectionName
						state.CurrentSubsection = ""
						state.InSubsection = false
						state.PendingComment = ""
						state.GlobalComment = ""

						if _, exists := role.Sections[sectionName]; !exists {
							role.Sections[sectionName] = &Section{
								Name:            sectionName,
								Variables:       []Variable{},
								Subsections:     make(map[string][]Variable),
								SubsectionOrder: []string{},
							}
							role.SectionOrder = append(role.SectionOrder, sectionName)

							// Set feature flags based on section name
							switch strings.ToLower(sectionName) {
							case "dns":
								role.HasDNS = true
							case "traefik":
								role.HasTraefik = true
							case "docker":
								role.HasDocker = true
							case "web":
								role.HasWeb = true
							}
						}
					}
					lineNum++ // Skip the section name line
					// Skip the closing #### line if present
					if lineNum < len(lines) && sectionHeaderRe.MatchString(strings.TrimSpace(lines[lineNum])) {
						lineNum++
					}
				}
			}
			continue
		}

		// Check for subsection markers
		if matches := subsectionStartRe.FindStringSubmatch(strings.TrimSpace(line)); matches != nil {
			state.CurrentSubsection = matches[1]
			state.InSubsection = true
			state.PendingComment = ""
			continue
		}

		if matches := subsectionEndRe.FindStringSubmatch(strings.TrimSpace(line)); matches != nil {
			state.CurrentSubsection = ""
			state.InSubsection = false
			state.PendingComment = ""
			state.GlobalComment = ""
			continue
		}

		// Check for comment lines
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "#") && !sectionHeaderRe.MatchString(trimmedLine) {
			commentText := strings.TrimPrefix(trimmedLine, "#")
			commentText = strings.TrimSpace(commentText)

			// Check for [GLOBAL] prefix - accumulate multiple global comments
			if globalPrefixRe.MatchString(commentText) {
				globalText := globalPrefixRe.ReplaceAllString(commentText, "")
				if state.GlobalComment != "" {
					state.GlobalComment += "\n" + globalText
				} else {
					state.GlobalComment = globalText
				}
				continue
			}

			// Accumulate pending comment
			if state.PendingComment != "" {
				state.PendingComment += "\n" + commentText
			} else {
				state.PendingComment = commentText
			}
			continue
		}

		// Check for variable definition
		if matches := variableRe.FindStringSubmatch(line); matches != nil {
			varName := matches[1]
			varValue := matches[2]

			// Check if this variable should be skipped
			if shouldSkipVariable(varName, state.PendingComment) {
				state.PendingComment = ""
				// Still need to consume multiline values
				lineNum = consumeMultilineValue(lines, lineNum, varValue)
				continue
			}

			// Build full value including multiline continuation
			fullValue, valueLines, newLineNum := parseMultilineValue(lines, lineNum-1, varValue)
			lineNum = newLineNum

			// Determine comment to use
			// Check for [NOGLOBAL] prefix - if present, don't apply global comment
			hasNoGlobal := noGlobalPrefixRe.MatchString(state.PendingComment)
			var comment string
			if hasNoGlobal {
				// [NOGLOBAL] marker: use only variable comment, exclude global
				comment = noGlobalPrefixRe.ReplaceAllString(state.PendingComment, "")
			} else if state.PendingComment != "" && state.GlobalComment != "" {
				// Both exist: global first, then variable comment
				comment = state.GlobalComment + "\n" + state.PendingComment
			} else if state.PendingComment != "" {
				// Only variable comment (no global available)
				comment = state.PendingComment
			} else if state.GlobalComment != "" {
				// Only global comment (no variable comment)
				comment = state.GlobalComment
			}

			variable := Variable{
				Name:        varName,
				RawValue:    fullValue,
				Section:     state.CurrentSection,
				Subsection:  state.CurrentSubsection,
				Comment:     comment,
				IsMultiline: len(valueLines) > 1,
				ValueLines:  valueLines,
				LineNumber:  lineNum - len(valueLines),
			}

			// Add to role
			role.AllVariables = append(role.AllVariables, variable)

			// Add to appropriate section
			if section, exists := role.Sections[state.CurrentSection]; exists {
				if state.InSubsection && state.CurrentSubsection != "" {
					if _, subExists := section.Subsections[state.CurrentSubsection]; !subExists {
						section.SubsectionOrder = append(section.SubsectionOrder, state.CurrentSubsection)
					}
					section.Subsections[state.CurrentSubsection] = append(
						section.Subsections[state.CurrentSubsection], variable)
				} else {
					section.Variables = append(section.Variables, variable)
				}
			}

			// Check for instances variable
			if strings.HasSuffix(varName, "_instances") {
				role.HasInstances = true
				role.InstancesVar = varName
			}

			// Check for _default/_custom pattern
			if strings.HasSuffix(varName, "_default") || strings.HasSuffix(varName, "_custom") {
				role.HasDefaultVars = true
			}

			// Check for SSO enabled by default
			if strings.HasSuffix(varName, "_traefik_sso_middleware") {
				if strings.Contains(fullValue, "traefik_default_sso_middleware") {
					role.SSOEnabled = true
				}
			}

			// Check for ThemePark variables
			if strings.Contains(varName, "_themepark_") {
				role.HasThemePark = true
			}

			state.PendingComment = ""
		}
	}

	return role, nil
}

// isMetaSection returns true if the section name is a metadata section to skip.
func isMetaSection(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "title:") ||
		strings.Contains(lower, "author") ||
		strings.Contains(lower, "url:") ||
		strings.Contains(lower, "gnu general public license") ||
		strings.Contains(lower, "copyright")
}

// shouldSkipVariable checks if a variable should be excluded from documentation.
func shouldSkipVariable(name, comment string) bool {
	// Skip _paths_folders_list variables
	if strings.Contains(name, "_paths_folders_list") {
		return true
	}

	// Skip if comment contains skip markers
	if skipDocsRe.MatchString(comment) || skipInventoryRe.MatchString(comment) {
		return true
	}

	return false
}

// parseMultilineValue parses a potentially multiline YAML value.
// Returns the full raw value (value only, not including variable name),
// individual value lines (with normalized indentation), and the new line number.
func parseMultilineValue(lines []string, startLine int, initialValue string) (string, []string, int) {
	// For the first line, we only want the value part (initialValue), not the full line
	valueLines := []string{initialValue}
	currentLine := startLine + 1

	// If value is empty or ends with a block indicator, look for indented continuation
	trimmedInitial := strings.TrimSpace(initialValue)

	// Check for multiline indicators
	if trimmedInitial == "" || trimmedInitial == "|" || trimmedInitial == ">" ||
		trimmedInitial == "|-" || trimmedInitial == ">-" {
		// Block scalar - collect indented lines
		for currentLine < len(lines) {
			line := lines[currentLine]
			if line == "" {
				// Empty line - check if it's truly part of the block or just separator
				// Peek ahead to see if next non-empty line is still indented
				nextNonEmpty := currentLine + 1
				for nextNonEmpty < len(lines) && lines[nextNonEmpty] == "" {
					nextNonEmpty++
				}
				if nextNonEmpty < len(lines) {
					nextLine := lines[nextNonEmpty]
					// If next non-empty line is not indented, this empty line is a separator
					if len(nextLine) > 0 && nextLine[0] != ' ' && nextLine[0] != '\t' {
						break
					}
				}
				// Empty line is part of the block (e.g., in a literal block scalar)
				valueLines = append(valueLines, line)
				currentLine++
			} else if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
				valueLines = append(valueLines, line)
				currentLine++
			} else {
				break
			}
		}
	} else if strings.HasSuffix(trimmedInitial, "[") || strings.HasSuffix(trimmedInitial, "{") {
		// Flow sequence/mapping - look for continuation
		for currentLine < len(lines) {
			line := lines[currentLine]
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				break
			}
			// Check if line is indented (continuation)
			if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
				valueLines = append(valueLines, line)
				currentLine++
				// Check if this line closes the structure
				if strings.HasSuffix(trimmed, "]") || strings.HasSuffix(trimmed, "}") {
					break
				}
			} else {
				break
			}
		}
	} else {
		// Check for quoted string continuation (line ends mid-expression)
		// Look for lines that appear to be continuations based on indentation
		for currentLine < len(lines) {
			line := lines[currentLine]
			if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
				trimmed := strings.TrimSpace(line)
				// Skip if it looks like a new variable or comment
				if strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, "-") &&
					!strings.HasPrefix(trimmed, "\"") && !strings.HasPrefix(trimmed, "'") {
					// Might be a dict key inside a value - check indentation
					if isSignificantlyIndented(line, lines[startLine]) {
						valueLines = append(valueLines, line)
						currentLine++
						continue
					}
					break
				}
				if strings.HasPrefix(trimmed, "#") {
					break
				}
				// Looks like a continuation
				valueLines = append(valueLines, line)
				currentLine++
			} else {
				break
			}
		}
	}

	// Keep continuation lines with their original indentation from the file.
	// The template/adjustment function will handle re-indenting for instance-level variables.
	// First line is just the value part (no var name), continuation lines have original file indentation.

	// Build full raw value - first line is just value, continuation lines are raw from file
	fullValue := strings.Join(valueLines, "\n")

	return fullValue, valueLines, currentLine
}

// isSignificantlyIndented checks if line2 is indented more than line1's variable name.
func isSignificantlyIndented(line, baseLine string) bool {
	baseIndent := len(baseLine) - len(strings.TrimLeft(baseLine, " \t"))
	lineIndent := len(line) - len(strings.TrimLeft(line, " \t"))
	return lineIndent > baseIndent
}

// consumeMultilineValue skips over a multiline value without parsing it.
func consumeMultilineValue(lines []string, lineNum int, initialValue string) int {
	_, _, newLineNum := parseMultilineValue(lines, lineNum-1, initialValue)
	return newLineNum
}
