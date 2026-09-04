package automation

import (
	"bytes"
	"io"
	"regexp"
	"strings"

	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/document"
	"github.com/saltyorg/docs-automation/parser"
	"go.yaml.in/yaml/v3"
)

func dockerMetadataChanges(doc *document.Document, role *parser.RoleInfo, metadata config.DockerMetadataConfig) document.FrontmatterChanges {
	if doc == nil || !metadata.Enabled() {
		return document.FrontmatterChanges{}
	}
	if _, ok := dockerImageVariable(role); !ok {
		return document.FrontmatterChanges{}
	}

	changes := document.FrontmatterChanges{Icon: &metadata.Icon}
	if doc.Frontmatter == nil || doc.Frontmatter.SaltboxAutomation == nil {
		return changes
	}
	target := resolveDockerRepository(dockerRepository(role), metadata)
	for i, link := range doc.Frontmatter.SaltboxAutomation.AppLinks {
		if link.Purpose != document.AppLinkPurposeRelease {
			continue
		}
		change := document.AppLinkChange{Index: i, Name: &metadata.ReleaseLink.Name}
		if target.URL != "" {
			change.URL = &target.URL
		}
		changes.Links = append(changes.Links, change)
	}
	return changes
}

func dockerRepository(role *parser.RoleInfo) string {
	variable, ok := dockerImageVariable(role)
	if !ok {
		return ""
	}
	return literalDockerRepository(variable.RawValue)
}

func dockerImageVariable(role *parser.RoleInfo) (parser.Variable, bool) {
	if role == nil {
		return parser.Variable{}, false
	}
	section := role.Sections["Docker"]
	if section == nil {
		return parser.Variable{}, false
	}
	name := role.Name + "_role_docker_image_repo"
	for _, variable := range section.Variables {
		if variable.Name == name {
			return variable, true
		}
	}
	for _, subsectionName := range section.SubsectionOrder {
		for _, variable := range section.Subsections[subsectionName] {
			if variable.Name == name {
				return variable, true
			}
		}
	}
	return parser.Variable{}, false
}

func literalDockerRepository(raw string) string {
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(raw)))
	if err := decoder.Decode(&document); err != nil || len(document.Content) != 1 {
		return ""
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err != io.EOF {
		return ""
	}
	node := document.Content[0]
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" || node.Anchor != "" || node.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		return ""
	}
	value := strings.TrimSpace(node.Value)
	if value == "" || containsJinjaDelimiter(value) {
		return ""
	}
	return value
}

func containsJinjaDelimiter(value string) bool {
	for _, delimiter := range []string{"{{", "}}", "{%", "%}", "{#", "#}"} {
		if strings.Contains(value, delimiter) {
			return true
		}
	}
	return false
}

func resolveDockerRepository(repository string, metadata config.DockerMetadataConfig) config.DockerMetadataTarget {
	normalized := config.NormalizeDockerRepository(repository)
	if normalized == "" || !metadata.Enabled() {
		return config.DockerMetadataTarget{}
	}
	for source, target := range metadata.Overrides {
		if config.NormalizeDockerRepository(source) == normalized {
			return target
		}
	}
	for _, ignored := range metadata.Ignore {
		if config.NormalizeDockerRepository(ignored) == normalized {
			return config.DockerMetadataTarget{}
		}
	}
	for _, rule := range metadata.Rules {
		pattern, err := regexp.Compile(rule.Pattern)
		if err != nil {
			continue
		}
		matches := pattern.FindStringSubmatchIndex(normalized)
		if matches == nil || matches[0] != 0 || matches[1] != len(normalized) {
			continue
		}
		return config.DockerMetadataTarget{
			URL:  string(pattern.ExpandString(nil, rule.URL, normalized, matches)),
			Type: rule.Type,
		}
	}
	return config.DockerMetadataTarget{}
}
