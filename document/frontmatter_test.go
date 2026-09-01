package document

import (
	"strings"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantBody  string
		wantFM    bool
		wantError string
	}{
		{
			name:     "without frontmatter",
			content:  "# Sonarr\n",
			wantBody: "# Sonarr\n",
		},
		{
			name:     "valid",
			content:  "---\nsaltbox_automation:\n  disabled: false\n---\n# Sonarr\n",
			wantBody: "# Sonarr\n",
			wantFM:   true,
		},
		{
			name:      "unclosed",
			content:   "---\nsaltbox_automation: {}\n",
			wantError: "unclosed frontmatter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, body, err := ParseFrontmatter(tt.content)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("error = %v, want substring %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFrontmatter() error = %v", err)
			}
			if body != tt.wantBody {
				t.Fatalf("body = %q, want %q", body, tt.wantBody)
			}
			if (fm != nil) != tt.wantFM {
				t.Fatalf("frontmatter present = %t, want %t", fm != nil, tt.wantFM)
			}
		})
	}
}

func TestSaltboxAutomationSectionsDefaultToEnabled(t *testing.T) {
	var cfg *SaltboxAutomationConfig
	if !cfg.IsInventorySectionEnabled() {
		t.Fatal("nil config disabled inventory, want enabled")
	}
	if !cfg.IsOverviewSectionEnabled() {
		t.Fatal("nil config disabled overview, want enabled")
	}

	cfg = &SaltboxAutomationConfig{}
	if !cfg.IsInventorySectionEnabled() {
		t.Fatal("zero config disabled inventory, want enabled")
	}
	if !cfg.IsOverviewSectionEnabled() {
		t.Fatal("zero config disabled overview, want enabled")
	}
}

func TestSaltboxAutomationExplicitSectionControls(t *testing.T) {
	enabled := true
	disabled := false

	tests := []struct {
		name          string
		cfg           *SaltboxAutomationConfig
		wantInventory bool
		wantOverview  bool
	}{
		{
			name:          "automation disabled",
			cfg:           &SaltboxAutomationConfig{Disabled: true},
			wantInventory: false,
			wantOverview:  false,
		},
		{
			name: "individual controls",
			cfg: &SaltboxAutomationConfig{Sections: SectionsConfig{
				Inventory: &disabled,
				Overview:  &enabled,
			}},
			wantInventory: false,
			wantOverview:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.IsInventorySectionEnabled(); got != tt.wantInventory {
				t.Fatalf("inventory enabled = %t, want %t", got, tt.wantInventory)
			}
			if got := tt.cfg.IsOverviewSectionEnabled(); got != tt.wantOverview {
				t.Fatalf("overview enabled = %t, want %t", got, tt.wantOverview)
			}
		})
	}
}

func TestSaltboxAutomationCheckControls(t *testing.T) {
	fm, _, err := ParseFrontmatter(`---
status: outdated
saltbox_automation:
  checks:
    coverage: true
    frontmatter: false
    editorial: true
---
# Sonarr
`)
	if err != nil {
		t.Fatalf("ParseFrontmatter() error = %v", err)
	}
	if fm.Status != "outdated" {
		t.Fatalf("status = %q, want outdated", fm.Status)
	}
	sa := fm.SaltboxAutomation
	if sa == nil || sa.Checks.Coverage == nil || sa.Checks.Frontmatter == nil || sa.Checks.Editorial == nil {
		t.Fatalf("parsed checks = %#v, want all controls", sa)
	}
	if !sa.IsCoverageCheckEnabled() || sa.IsFrontmatterCheckEnabled() || !sa.IsEditorialCheckEnabled() {
		t.Fatalf("parsed checks enabled = coverage:%t frontmatter:%t editorial:%t, want true false true", sa.IsCoverageCheckEnabled(), sa.IsFrontmatterCheckEnabled(), sa.IsEditorialCheckEnabled())
	}

	no := false
	sa = &SaltboxAutomationConfig{
		Checks: PageChecks{Frontmatter: &no},
	}
	if sa.IsFrontmatterCheckEnabled() {
		t.Fatal("frontmatter check should be disabled")
	}
	if !sa.IsCoverageCheckEnabled() || !sa.IsEditorialCheckEnabled() {
		t.Fatal("unspecified checks should inherit enabled state")
	}

	sa.Disabled = true
	if sa.IsCoverageCheckEnabled() ||
		sa.IsFrontmatterCheckEnabled() ||
		sa.IsEditorialCheckEnabled() {
		t.Fatal("page-wide disable must win")
	}
}

func TestShouldShowSectionGivesHideListPrecedence(t *testing.T) {
	cfg := &SaltboxAutomationConfig{Inventory: InventoryConfig{
		ShowSections: []string{"Docker", "Web"},
		HideSections: []string{"docker"},
	}}

	if cfg.ShouldShowSection("Docker") {
		t.Fatal("Docker shown despite matching hide list")
	}
	if !cfg.ShouldShowSection("web") {
		t.Fatal("Web hidden despite matching show list")
	}
	if cfg.ShouldShowSection("DNS") {
		t.Fatal("DNS shown despite not matching non-empty show list")
	}
}

func TestGetExampleOverride(t *testing.T) {
	cfg := &SaltboxAutomationConfig{Inventory: InventoryConfig{
		ExampleOverrides: map[string]string{"sonarr_port": "8990"},
	}}

	got, ok := cfg.GetExampleOverride("sonarr_port")
	if !ok || got != "8990" {
		t.Fatalf("GetExampleOverride() = %q, %t, want 8990, true", got, ok)
	}
	if _, ok := cfg.GetExampleOverride("sonarr_missing"); ok {
		t.Fatal("missing override reported present")
	}
}
