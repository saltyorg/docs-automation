package automation

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/github"
	"github.com/saltyorg/docs-automation/render"
)

func TestUpdateClearsExplicitlyDisabledManagedSections(t *testing.T) {
	frontmatter := updateSectionsFrontmatter("    inventory: false\n    overview: false\n")
	fixture := newDockerUpdateFixture(t, frontmatter, true, true)
	fixture.cfg.DockerMetadata = config.DockerMetadataConfig{}
	runner := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false)

	result := runner.updateRoleWithResult(t.Context(), fixture.cfg, render.SourceCatalog{}, "sonarr", "saltbox")
	if result.Status != github.StatusUpdated || !slices.Equal(result.Sections, []string{"variables", "overview"}) {
		t.Fatalf("first update = status %s sections %v error %q", result.Status, result.Sections, result.Error)
	}
	want := frontmatter + "# Sonarr\n" +
		"<!-- BEGIN VARIABLES -->\n<!-- END VARIABLES -->\n" +
		"<!-- BEGIN OVERVIEW -->\n<!-- END OVERVIEW -->\n"
	if got := fixture.readDoc(t); got != want {
		t.Fatalf("first update content:\n%s\nwant:\n%s", got, want)
	}

	result = runner.updateRoleWithResult(t.Context(), fixture.cfg, render.SourceCatalog{}, "sonarr", "saltbox")
	if result.Status != github.StatusUnchanged {
		t.Fatalf("second update = status %s sections %v error %q, want unchanged", result.Status, result.Sections, result.Error)
	}
	if got := fixture.readDoc(t); got != want {
		t.Fatalf("second update content changed:\n%s", got)
	}
}

func TestUpdateClearsOnlyTheDisabledManagedSection(t *testing.T) {
	tests := []struct {
		name          string
		sectionConfig string
		wantCleared   string
		wantRendered  string
		unwantedOld   string
	}{
		{
			name:          "inventory disabled",
			sectionConfig: "    inventory: false\n",
			wantCleared:   "<!-- BEGIN VARIABLES -->\n<!-- END VARIABLES -->",
			wantRendered:  "overview=Manual|https://manual.example|documentation",
			unwantedOld:   "old inventory",
		},
		{
			name:          "overview disabled",
			sectionConfig: "    overview: false\n",
			wantCleared:   "<!-- BEGIN OVERVIEW -->\n<!-- END OVERVIEW -->",
			wantRendered:  "inventory=Manual|https://manual.example|",
			unwantedOld:   "old overview",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newDockerUpdateFixture(t, updateSectionsFrontmatter(tt.sectionConfig), true, true)
			fixture.cfg.DockerMetadata = config.DockerMetadataConfig{}
			result := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false).updateRoleWithResult(
				t.Context(), fixture.cfg, render.SourceCatalog{}, "sonarr", "saltbox",
			)
			if result.Status != github.StatusUpdated || !slices.Equal(result.Sections, []string{"variables", "overview"}) {
				t.Fatalf("update = status %s sections %v error %q", result.Status, result.Sections, result.Error)
			}
			content := fixture.readDoc(t)
			for _, want := range []string{tt.wantCleared, tt.wantRendered} {
				if !strings.Contains(content, want) {
					t.Errorf("updated content missing %q:\n%s", want, content)
				}
			}
			if strings.Contains(content, tt.unwantedOld) {
				t.Errorf("updated content retained %q:\n%s", tt.unwantedOld, content)
			}
		})
	}
}

func TestUpdatePreservesManagedSectionsWhenPageAutomationIsDisabled(t *testing.T) {
	frontmatter := strings.Replace(updateSectionsFrontmatter(""), "saltbox_automation:\n", "saltbox_automation:\n  disabled: true\n", 1)
	fixture := newDockerUpdateFixture(t, frontmatter, true, true)
	fixture.cfg.DockerMetadata = config.DockerMetadataConfig{}
	original := fixture.readDoc(t)

	result := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false).updateRoleWithResult(
		t.Context(), fixture.cfg, render.SourceCatalog{}, "sonarr", "saltbox",
	)
	if result.Status != github.StatusSkipped {
		t.Fatalf("update = status %s sections %v error %q, want skipped", result.Status, result.Sections, result.Error)
	}
	if got := fixture.readDoc(t); got != original {
		t.Fatalf("disabled page changed:\n%s", got)
	}
}

func updateSectionsFrontmatter(sectionConfig string) string {
	return "---\n" +
		"saltbox_automation:\n" +
		"  sections:\n" + sectionConfig +
		"  app_links:\n" +
		"    - name: Manual\n" +
		"      url: https://manual.example\n" +
		"      type: documentation\n" +
		"      purpose: manual\n" +
		"  project_description:\n" +
		"    name: Sonarr\n" +
		"    summary: Summary\n" +
		"---\n"
}
