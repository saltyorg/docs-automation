package document

import "testing"

func TestValidateAutomationFrontmatter(t *testing.T) {
	no := false

	tests := []struct {
		name        string
		frontmatter *Frontmatter
		wantCodes   []string
	}{
		{
			name: "nil frontmatter is not validated",
		},
		{
			name:        "missing automation block is not validated",
			frontmatter: &Frontmatter{},
		},
		{
			name: "page disabled is not validated",
			frontmatter: &Frontmatter{SaltboxAutomation: &SaltboxAutomationConfig{
				Disabled: true,
			}},
		},
		{
			name: "frontmatter check opt out is not validated",
			frontmatter: &Frontmatter{SaltboxAutomation: &SaltboxAutomationConfig{
				Checks: PageChecks{Frontmatter: &no},
			}},
		},
		{
			name: "overview opt out is not validated",
			frontmatter: &Frontmatter{SaltboxAutomation: &SaltboxAutomationConfig{
				Sections: SectionsConfig{Overview: &no},
				AppLinks: []AppLink{{Name: "Manual"}},
			}},
		},
		{
			name: "lean page permits omitted links",
			frontmatter: &Frontmatter{SaltboxAutomation: &SaltboxAutomationConfig{
				ProjectDescription: &ProjectDescription{Name: "Lean", Summary: "A concise summary."},
			}},
		},
		{
			name:        "requires a project description",
			frontmatter: &Frontmatter{SaltboxAutomation: &SaltboxAutomationConfig{}},
			wantCodes: []string{
				"project_description_required",
			},
		},
		{
			name: "requires trimmed project description fields",
			frontmatter: &Frontmatter{SaltboxAutomation: &SaltboxAutomationConfig{
				ProjectDescription: &ProjectDescription{Name: " \t", Summary: "\n"},
			}},
			wantCodes: []string{
				"project_description_name_required",
				"project_description_summary_required",
			},
		},
		{
			name: "reports every incomplete supplied link and description field",
			frontmatter: &Frontmatter{SaltboxAutomation: &SaltboxAutomationConfig{
				AppLinks: []AppLink{{Name: " \t", URL: "\n", Purpose: AppLinkPurposeRelease}},
				ProjectDescription: &ProjectDescription{
					Name:    " ",
					Summary: "\t",
				},
			}},
			wantCodes: []string{
				"app_link_name_required",
				"app_link_url_required",
				"project_description_name_required",
				"project_description_summary_required",
			},
		},
		{
			name: "complete page is valid",
			frontmatter: &Frontmatter{SaltboxAutomation: &SaltboxAutomationConfig{
				AppLinks: []AppLink{{Name: "Website", URL: "https://example.invalid", Purpose: AppLinkPurposeManual}},
				ProjectDescription: &ProjectDescription{
					Name:       "Full",
					Summary:    "A complete page.",
					Link:       "https://example.invalid",
					Categories: []string{"Utilities"},
				},
			}},
		},
		{
			name: "manual and community links permit blank URLs",
			frontmatter: &Frontmatter{SaltboxAutomation: &SaltboxAutomationConfig{
				AppLinks: []AppLink{
					{Name: "Manual", URL: " \t", Purpose: AppLinkPurposeManual},
					{Name: "Community", Purpose: AppLinkPurposeCommunity},
				},
				ProjectDescription: &ProjectDescription{Name: "Links", Summary: "Optional URLs."},
			}},
		},
		{
			name: "release and other links require URLs",
			frontmatter: &Frontmatter{SaltboxAutomation: &SaltboxAutomationConfig{
				AppLinks: []AppLink{
					{Name: "Releases", Purpose: AppLinkPurposeRelease},
					{Name: "Alternative", URL: "\n", Purpose: AppLinkPurposeOther},
				},
				ProjectDescription: &ProjectDescription{Name: "Links", Summary: "Required URLs."},
			}},
			wantCodes: []string{
				"app_link_url_required",
				"app_link_url_required",
			},
		},
		{
			name: "missing and invalid purposes have distinct diagnostics",
			frontmatter: &Frontmatter{SaltboxAutomation: &SaltboxAutomationConfig{
				AppLinks: []AppLink{
					{Name: "Missing"},
					{Name: "Invalid", Purpose: AppLinkPurpose("documentation")},
				},
				ProjectDescription: &ProjectDescription{Name: "Links", Summary: "Purpose validation."},
			}},
			wantCodes: []string{
				"app_link_purpose_required",
				"app_link_purpose_invalid",
			},
		},
		{
			name: "type remains presentation only",
			frontmatter: &Frontmatter{SaltboxAutomation: &SaltboxAutomationConfig{
				AppLinks: []AppLink{{
					Name:    "Manual",
					Type:    "not-a-policy-value",
					Purpose: AppLinkPurposeManual,
				}},
				ProjectDescription: &ProjectDescription{Name: "Links", Summary: "Type is presentational."},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateAutomationFrontmatter(tt.frontmatter)
			if len(got) != len(tt.wantCodes) {
				t.Fatalf("diagnostics = %+v, want codes %v", got, tt.wantCodes)
			}
			for i, wantCode := range tt.wantCodes {
				if got[i].Code != wantCode {
					t.Errorf("diagnostics[%d].Code = %q, want %q", i, got[i].Code, wantCode)
				}
				if got[i].Message == "" {
					t.Errorf("diagnostics[%d].Message is empty", i)
				}
			}
		})
	}
}

func TestValidateAutomationFrontmatterPurposeWhitespace(t *testing.T) {
	fm := &Frontmatter{SaltboxAutomation: &SaltboxAutomationConfig{
		AppLinks: []AppLink{
			{Name: "Whitespace", Purpose: AppLinkPurpose(" \t")},
			{Name: "Padded", Purpose: AppLinkPurpose(" release ")},
		},
		ProjectDescription: &ProjectDescription{Name: "Links", Summary: "Purpose whitespace."},
	}}

	got := ValidateAutomationFrontmatter(fm)
	want := []string{"app_link_purpose_required", "app_link_purpose_invalid"}
	if len(got) != len(want) {
		t.Fatalf("diagnostics = %+v, want codes %v", got, want)
	}
	for i, code := range want {
		if got[i].Code != code {
			t.Errorf("diagnostics[%d].Code = %q, want %q", i, got[i].Code, code)
		}
	}
}
