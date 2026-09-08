package document

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestExampleOverridePreservesEntry(t *testing.T) {
	fm, _, err := ParseFrontmatter("---\nsaltbox_automation:\n  inventory:\n    example_overrides:\n      # keep this guidance\n      app_list:\n        - \"./logs/*\" # keep this too\n      app_empty: null\n---\n")
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := fm.SaltboxAutomation.GetExampleOverride("app_list")
	if !ok {
		t.Fatal("list example missing")
	}
	encoded, err := yaml.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# keep this guidance", "app_list:", `- "./logs/*" # keep this too`} {
		if !strings.Contains(string(encoded), want) {
			t.Errorf("encoded example missing %q: %s", want, encoded)
		}
	}
	if _, ok := fm.SaltboxAutomation.GetExampleOverride("app_empty"); !ok {
		t.Fatal("explicit null example missing")
	}
	if _, ok := fm.SaltboxAutomation.GetExampleOverride("absent"); ok {
		t.Fatal("absent example reported present")
	}
}

func TestExampleOverrideValidation(t *testing.T) {
	tests := []struct {
		name, entries, wantError string
	}{
		{"null container", "null", ""},
		{"empty container", "{}", ""},
		{"sequence container", "[]", "mapping"},
		{"non-string variable", "\n      42: value", "variable name"},
		{"duplicate variable", "\n      app_x: 1\n      app_x: 2", "app_x"},
		{"nested duplicate", "\n      app_x: {a: 1, a: 2}", "app_x"},
		{"equivalent integer keys", "\n      app_x: {1: first, 01: second}", "app_x"},
		{"equivalent bool keys", "\n      app_x: {true: first, True: second}", "app_x"},
		{"tagged int mapping", "\n      app_x: !!int {}", "app_x"},
		{"tagged bool sequence", "\n      app_x: !!bool []", "app_x"},
		{"tagged seq mapping", "\n      app_x: !!seq {}", "app_x"},
		{"tagged map sequence", "\n      app_x: !!map []", "app_x"},
		{"nested valid", "\n      app_x: [{a: 1}, true, null, []]", ""},
		{"local alias and merge", "\n      app_x:\n        base: &base {a: 1}\n        other: {<<: *base, b: 2}", ""},
		{"quoted merge key", "\n      app_x: {\"<<\": literal}", ""},
		{"invalid merge", "\n      app_x: {<<: 3}", "app_x"},
		{"shadowed duplicate", "\n      app_x: {actual: ok, <<: {actual: {duplicate: 1, duplicate: 2}}}", "app_x"},
		{"shadowed scalar tag", "\n      app_x: {actual: ok, <<: {actual: !!int invalid}}", "app_x"},
		{"shadowed merge sequence", "\n      app_x: {<<: [{actual: ok}, {actual: {duplicate: 1, duplicate: 2}}]}", "app_x"},
		{"external alias", "\n      app_base: &base [1]\n      app_x: *base", "app_x"},
		{"recursive alias", "\n      app_x: &self [*self]", "app_x"},
		{"tagged scalar", "\n      app_x: !vault |\n        ciphertext\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Inventory validation must not depend on overview or health switches.
			input := "---\nsaltbox_automation:\n  sections:\n    overview: false\n  checks:\n    frontmatter: false\n  inventory:\n    example_overrides: " + tt.entries + "\n---\n"
			_, _, err := ParseFrontmatter(input)
			if tt.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantError)
			}
		})
	}
}
