package document

import (
	"fmt"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// GetExampleOverride returns a one-entry YAML mapping, preserving key comments
// as well as the value's type and style. Its descendants must be treated as
// immutable. An explicit null value is present; a missing entry is not.
func (c *SaltboxAutomationConfig) GetExampleOverride(varName string) (yaml.Node, bool) {
	if c != nil {
		entries := c.Inventory.ExampleOverrides.Content
		for i := 0; i+1 < len(entries); i += 2 {
			if entries[i].Value == varName {
				return yaml.Node{
					Kind:    yaml.MappingNode,
					Tag:     "!!map",
					Content: []*yaml.Node{entries[i], entries[i+1]},
				}, true
			}
		}
	}
	return yaml.Node{}, false
}

func validateExampleOverrides(node *yaml.Node) error {
	if node.Kind == 0 || node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("example_overrides at line %d must be a mapping of variable names to YAML values", node.Line)
	}
	seen := make(map[string]bool)
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || strings.TrimSpace(key.Value) == "" {
			return fmt.Errorf("example_overrides at line %d requires a non-empty string variable name", key.Line)
		}
		if seen[key.Value] {
			return fmt.Errorf("example_overrides.%s at line %d: duplicate variable name", key.Value, key.Line)
		}
		seen[key.Value] = true
		if err := validateExampleAliases(value); err != nil {
			return fmt.Errorf("example_overrides.%s at line %d: %w", key.Value, value.Line, err)
		}
		// Decoding into Node bypasses the library's value validation. Decode a
		// disposable value to check duplicates, scalar tags, and merge shapes;
		// keep the original nodes for rendering, never the decoded Go value.
		var decoded any
		if err := value.Decode(&decoded); err != nil {
			return fmt.Errorf("example_overrides.%s at line %d: %w", key.Value, value.Line, err)
		}
		if err := validateExampleMerges(value); err != nil {
			return fmt.Errorf("example_overrides.%s at line %d: %w", key.Value, value.Line, err)
		}
		if err := validateExampleNodes(value); err != nil {
			return fmt.Errorf("example_overrides.%s at line %d: %w", key.Value, value.Line, err)
		}
	}
	return nil
}

// Node decoding also accepts collection tags without checking their kind and
// compares duplicate keys by spelling. Check the physical nodes so the rendered
// YAML cannot contain incompatible core tags or keys with identical values.
func validateExampleNodes(node *yaml.Node) error {
	var kind yaml.Kind
	switch node.Tag {
	case "!!map", "!!set":
		kind = yaml.MappingNode
	case "!!seq", "!!omap", "!!pairs":
		kind = yaml.SequenceNode
	case "!!str", "!!bool", "!!int", "!!float", "!!null", "!!timestamp", "!!binary", "!!merge":
		kind = yaml.ScalarNode
	}
	if kind != 0 && node.Kind != kind {
		return fmt.Errorf("tag %s is incompatible with its YAML node at line %d", node.Tag, node.Line)
	}
	if node.Kind == yaml.MappingNode {
		keys := make(map[string]bool)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			var value any
			if err := key.Decode(&value); err != nil {
				return err
			}
			// Canonicalize scalar representations, including equivalent times
			// and signed zero, without losing the original YAML tag identity.
			switch v := value.(type) {
			case time.Time:
				value = v.UTC()
			case float64:
				if v == 0 {
					value = float64(0)
				}
			}
			canonical, err := yaml.Marshal(value)
			if err != nil {
				return err
			}
			identity := key.ShortTag() + ":" + string(canonical)
			if keys[identity] {
				return fmt.Errorf("duplicate mapping key %q at line %d", key.Value, key.Line)
			}
			keys[identity] = true
		}
	}
	for _, child := range node.Content {
		if err := validateExampleNodes(child); err != nil {
			return err
		}
	}
	return nil
}

// The decoder skips values shadowed by merge precedence. Validate each merge
// operand independently so invalid content cannot survive in the displayed
// example merely because a different value wins when the YAML is loaded.
func validateExampleMerges(node *yaml.Node) error {
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			if node.Content[i].Tag == "!!merge" {
				var decoded any
				if err := node.Content[i+1].Decode(&decoded); err != nil {
					return err
				}
			}
		}
	}
	for _, child := range node.Content {
		if err := validateExampleMerges(child); err != nil {
			return err
		}
	}
	return nil
}

// Each example is copied independently, so aliases must not depend on another
// example or unrelated frontmatter. Follow aliases only after collecting the
// nodes physically contained in this value.
func validateExampleAliases(root *yaml.Node) error {
	members := make(map[*yaml.Node]bool)
	var collect func(*yaml.Node)
	collect = func(node *yaml.Node) {
		members[node] = true
		for _, child := range node.Content {
			collect(child)
		}
	}
	collect(root)
	active := make(map[*yaml.Node]bool)
	done := make(map[*yaml.Node]bool)
	var visit func(*yaml.Node) error
	visit = func(node *yaml.Node) error {
		if active[node] {
			return fmt.Errorf("recursive alias at line %d", node.Line)
		}
		if done[node] {
			return nil
		}
		active[node] = true
		if node.Kind == yaml.AliasNode {
			if !members[node.Alias] {
				return fmt.Errorf("alias %q at line %d refers outside this example", node.Value, node.Line)
			}
			if err := visit(node.Alias); err != nil {
				return err
			}
		}
		for _, child := range node.Content {
			if err := visit(child); err != nil {
				return err
			}
		}
		delete(active, node)
		done[node] = true
		return nil
	}
	return visit(root)
}
