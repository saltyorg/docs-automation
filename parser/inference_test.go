package parser

import (
	"testing"

	"github.com/saltyorg/docs-automation/config"
)

func TestTypeInferrerInfersDDNSConditionalDictionary(t *testing.T) {
	value := `"{{ {'CLOUDFLARE_API_TOKEN': cloudflare_scoped_token}
                                   if cloudflare_scoped_token_is_enabled
                                   else {'CLOUDFLARE_API_KEY': cloudflare_api_key,
                                         'CLOUDFLARE_EMAIL': cloudflare_email} }}"`

	got := NewTypeInferrer(nil).InferType("ddns_role_docker_envs_cloudflare", value)
	if got != Dict {
		t.Fatalf("InferType() = %q, want %q", got, Dict)
	}
}

func TestTypeInferrerInfersConditionalList(t *testing.T) {
	value := `"{{ ['one', {'nested': true}]
                   if feature_enabled
                   else [] }}"`

	got := NewTypeInferrer(nil).InferType("example_role_docker_ports", value)
	if got != List {
		t.Fatalf("InferType() = %q, want %q", got, List)
	}
}

func TestTypeInferrerInfersParenthesizedCollectionExpression(t *testing.T) {
	value := `"{{ ({'one': 1} if feature_enabled else {}) }}"`

	got := NewTypeInferrer(nil).InferType("example_role_docker_envs_generated", value)
	if got != Dict {
		t.Fatalf("InferType() = %q, want %q", got, Dict)
	}
}

func TestTypeInferrerRejectsMismatchedConditionalDelimiters(t *testing.T) {
	value := `"{{ {} if (feature_enabled] else {} }}"`

	got := NewTypeInferrer(nil).InferType("example_role_docker_envs_generated", value)
	if got != String {
		t.Fatalf("InferType() = %q, want %q", got, String)
	}
}

func TestTypeInferrerRejectsPunctuatedConditionalKeyword(t *testing.T) {
	value := `"{{ {} if.feature_enabled else {} }}"`

	got := NewTypeInferrer(nil).InferType("example_role_docker_envs_generated", value)
	if got != String {
		t.Fatalf("InferType() = %q, want %q", got, String)
	}
}

func TestTypeInferrerInfersPureJinjaCollections(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "direct dictionary",
			value: `"{{ {'key': ['nested', {'enabled': true}]} }}"`,
			want:  Dict,
		},
		{
			name:  "direct list",
			value: `"{{ ['value', {'nested': [1, 2]}] }}"`,
			want:  List,
		},
		{
			name:  "unquoted expression",
			value: `{{ {'key': 'value'} }}`,
			want:  Dict,
		},
		{
			name:  "conditional keywords inside collections",
			value: `"{{ {'text': 'if this else that'} if enabled else {'else': 'if'} }}"`,
			want:  Dict,
		},
	}

	inferrer := NewTypeInferrer(nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inferrer.InferType("example_role_value", tt.value); got != tt.want {
				t.Fatalf("InferType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTypeInferrerPreservesStringFallbackForUnprovenJinja(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "mixed conditional branches", value: `"{{ {} if enabled else [] }}"`},
		{name: "unknown conditional branch", value: `"{{ {} if enabled else existing_value }}"`},
		{name: "ordinary variable", value: `"{{ example_name }}"`},
		{name: "embedded interpolation", value: `"prefix-{{ {} }}"`},
		{name: "collection text in string", value: `"{{ '{not a dictionary}' }}"`},
		{name: "unbalanced dictionary", value: `"{{ {'key': ['value'} }}"`},
		{name: "combine filter", value: `"{{ defaults | combine(overrides) }}"`},
		{name: "list filter", value: `"{{ values | list }}"`},
	}

	inferrer := NewTypeInferrer(nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inferrer.InferType("example_role_value", tt.value); got != String {
				t.Fatalf("InferType() = %q, want %q", got, String)
			}
		})
	}
}

func TestTypeInferrerPreservesNetworkAliasAsString(t *testing.T) {
	got := NewTypeInferrer(nil).InferType(
		"example_role_docker_networks_alias",
		`"{{ example_name }}"`,
	)
	if got != String {
		t.Fatalf("InferType() = %q, want %q", got, String)
	}
}

func TestTypeInferrerConfiguredTypesOverrideStructuralJinja(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.TypeInferenceConfig
		want string
	}{
		{
			name: "exact",
			cfg: config.TypeInferenceConfig{
				Exact: map[string]string{"_docker_envs_generated": String},
			},
			want: String,
		},
		{
			name: "override",
			cfg: config.TypeInferenceConfig{
				Overrides: map[string]string{"_docker_envs_generated": List},
			},
			want: List,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inferrer := NewTypeInferrer(&tt.cfg)
			got := inferrer.InferType(
				"example_role_docker_envs_generated",
				`"{{ {'key': 'value'} }}"`,
			)
			if got != tt.want {
				t.Fatalf("InferType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func FuzzTypeInferrerStructuralJinja(f *testing.F) {
	for _, seed := range []string{
		`"{{ {} if enabled else {} }}"`,
		`"{{ ['value'] }}"`,
		`"{{ {'quoted': '} if else {'} }}"`,
		`"{{ {} if (enabled] else {} }}"`,
		`prefix-{{ {} }}`,
	} {
		f.Add(seed)
	}

	inferrer := NewTypeInferrer(nil)
	f.Fuzz(func(t *testing.T, value string) {
		first := inferrer.InferType("example_role_value", value)
		second := inferrer.InferType("example_role_value", value)
		if first != second {
			t.Fatalf("InferType() is not deterministic: first %q, second %q", first, second)
		}
	})
}
