package render

import (
	"reflect"
	"strings"
	"testing"

	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/document"
	"github.com/saltyorg/docs-automation/parser"
	"go.yaml.in/yaml/v3"
)

func exampleConfig(t *testing.T, entries string) *document.SaltboxAutomationConfig {
	t.Helper()
	fm, _, err := document.ParseFrontmatter("---\nsaltbox_automation:\n  inventory:\n    example_overrides:\n" + entries + "\n---\n")
	if err != nil {
		t.Fatal(err)
	}
	return fm.SaltboxAutomation
}

func TestExamplesDoNotChangeDefaults(t *testing.T) {
	for _, variable := range []parser.Variable{
		{Name: "app_value", RawValue: "[]"},
		{Name: "app_value", RawValue: "|\n  source\n", IsMultiline: true, ValueLines: []string{"|", "  source"}},
	} {
		role := &parser.RoleInfo{Name: "app", AllVariables: []parser.Variable{variable}}
		baseline := BuildRoleData(role, &config.Config{}, nil, SourceCatalog{})
		fm := exampleConfig(t, "      app_value: true")
		withExample := BuildRoleData(role, &config.Config{}, fm, SourceCatalog{})
		if !reflect.DeepEqual(baseline.Sections, withExample.Sections) {
			t.Fatalf("example changed defaults: got %+v, want %+v", withExample.Sections["General"].Variables[0], baseline.Sections["General"].Variables[0])
		}
	}
}

func TestRenderNativeExamples(t *testing.T) {
	tests := []struct {
		name, value, wantYAML, tag, valueText string
	}{
		{"string bool", `"true"`, `app_value: "true"`, "!!str", "true"},
		{"string number", `'42'`, `app_value: '42'`, "!!str", "42"},
		{"string collection", `'[]'`, `app_value: '[]'`, "!!str", "[]"},
		{"bool", "false", "app_value: false", "!!bool", "false"},
		{"integer", "0", "app_value: 0", "!!int", "0"},
		{"float", "1.5", "app_value: 1.5", "!!float", "1.5"},
		{"null", "null", "app_value: null", "!!null", "null"},
		{"implicit null", "", "app_value:", "!!null", ""},
		{"empty string", `""`, `app_value: ""`, "!!str", ""},
		{"empty list", "[]", "app_value: []", "!!seq", ""},
		{"empty map", "{}", "app_value: {}", "!!map", ""},
		{"block list", "\n        - \"./logs/*\"\n        - false", "app_value:\n  - \"./logs/*\"\n  - false", "!!seq", ""},
		{"flow", `{z: [1, true], a: {x: null}}`, `app_value: {z: [1, true], a: {x: null}}`, "!!map", ""},
		{"block map", "\n        z: true # flag\n        a:\n          - 1", "app_value:\n  z: true # flag\n  a:\n    - 1", "!!map", ""},
		{"local anchors", "\n        base: &base {a: 1}\n        other: {<<: *base, b: 2}", "base: &base {a: 1}", "!!map", ""},
		{"literal", "|-\n        first\n        second", "app_value: |-\n  first\n  second", "!!str", "first\nsecond"},
		{"folded", ">-\n        first\n        second", "app_value: >-", "!!str", "first second"},
		{"keep newlines", "|+\n        first\n", "app_value: |+", "!!str", "first\n\n"},
		{"jinja", `"{{ user.domain }} {role} {variable}"`, `app_value: "{{ user.domain }} {role} {variable}"`, "!!str", "{{ user.domain }} {role} {variable}"},
		{"tag", "!vault |\n        ciphertext", "app_value: !vault |", "!vault", "ciphertext\n"},
		{"fences", "|-\n        ```\n        text", "app_value: |-\n  ```\n  text", "!!str", "```\ntext"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm := exampleConfig(t, "      app_value: "+tt.value)
			out, err := New().RenderString(`{{ renderExampleOverride . "app_value" "app_value" }}`, fm)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, tt.wantYAML) {
				t.Fatalf("output %q missing %q", out, tt.wantYAML)
			}
			lines := strings.Split(out, "\n")
			fence := strings.TrimSuffix(lines[0], "yaml")
			if len(fence) < 3 || lines[len(lines)-1] != fence {
				t.Fatalf("invalid code fences: %q", out)
			}
			payload := strings.Join(lines[1:len(lines)-1], "\n") + "\n"
			if strings.Contains(payload, fence) {
				t.Fatalf("payload can close its fence: %q", out)
			}
			var node yaml.Node
			if err := yaml.Unmarshal([]byte(payload), &node); err != nil {
				t.Fatal(err)
			}
			value := node.Content[0].Content[1]
			if value.Tag != tt.tag || value.Value != tt.valueText {
				t.Fatalf("parsed value = %s %q, want %s %q", value.Tag, value.Value, tt.tag, tt.valueText)
			}
			if tt.name == "local anchors" {
				var decoded map[string]any
				if err := value.Decode(&decoded); err != nil {
					t.Fatal(err)
				}
				want := map[string]any{"base": map[string]any{"a": 1}, "other": map[string]any{"a": 1, "b": 2}}
				if !reflect.DeepEqual(decoded, want) {
					t.Fatalf("copied example resolves to %#v, want %#v", decoded, want)
				}
			}
		})
	}
}

func TestAbsentExampleProducesNoBlock(t *testing.T) {
	for _, cfg := range []*document.SaltboxAutomationConfig{nil, {}} {
		out, err := New().RenderString(`{{ renderExampleOverride . "app_role_value" "app2_value" }}`, cfg)
		if err != nil || out != "" {
			t.Fatalf("absent example = %q, %v", out, err)
		}
	}
}

func TestExampleScopeAndImmutability(t *testing.T) {
	fm := exampleConfig(t, "      # guidance\n      app_role_value: {z: \"{{ app_role_value }}\", a: 1} # inline\n      app2_value: null")
	for _, tc := range []struct{ canonical, display, want string }{
		{"app_role_value", "app2_value", "app2_value: null"},
		{"app_role_value", "app3_value", "app3_value: {z: \"{{ app_role_value }}\", a: 1} # inline"},
		{"app_role_value", "app_role_value", "app_role_value: {z: \"{{ app_role_value }}\", a: 1} # inline"},
		{"missing", "missing", ""},
	} {
		out, err := New().RenderString(`{{ renderExampleOverride .Config .Canonical .Display }}`, struct {
			Config             *document.SaltboxAutomationConfig
			Canonical, Display string
		}{fm, tc.canonical, tc.display})
		if err != nil {
			t.Fatal(err)
		}
		if tc.want == "" && out != "" || !strings.Contains(out, tc.want) {
			t.Fatalf("%s output = %q, want %q", tc.display, out, tc.want)
		}
		if tc.display == "app_role_value" && !strings.Contains(out, "# guidance") {
			t.Fatal("key comment lost")
		}
	}
}
