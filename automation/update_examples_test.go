package automation

import (
	"bytes"
	"strings"
	"testing"

	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/github"
	"github.com/saltyorg/docs-automation/render"
)

func TestUpdateNativeExamplesPreservesAuthoredContentAndConverges(t *testing.T) {
	frontmatter := "---\n# authored header\nsaltbox_automation:\n  sections:\n    overview: false\n  inventory:\n    example_overrides:\n      # example guidance\n      sonarr_role_docker_enabled: false\n---\n"
	fixture := newDockerUpdateFixture(t, frontmatter, true, false)
	fixture.cfg.DockerMetadata = config.DockerMetadataConfig{}
	writeDockerUpdateFile(t, fixture.cfg.InventoryTemplatePath(), []byte(`{{ range .Sections }}{{ range .Variables }}
default {{ .Name }}: {{ .RawValue }}
{{ renderExampleOverride $.Config .Name .Name }}
{{ end }}{{ end }}`))
	original := fixture.readDoc(t)
	runner := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false)
	first := runner.updateRoleWithResult(t.Context(), fixture.cfg, render.SourceCatalog{}, "sonarr", "saltbox")
	if first.Status != github.StatusUpdated {
		t.Fatalf("first update: %s, %s", first.Status, first.Error)
	}
	updated := fixture.readDoc(t)
	if !strings.HasPrefix(updated, frontmatter+"# Sonarr\n") || !strings.HasSuffix(updated, "<!-- END VARIABLES -->\n") {
		t.Fatalf("authored content changed:\n%s", updated)
	}
	for _, want := range []string{"default sonarr_role_docker_enabled: true", "```yaml\n# example guidance\nsonarr_role_docker_enabled: false\n```"} {
		if !strings.Contains(updated, want) {
			t.Fatalf("updated page missing %q:\n%s", want, updated)
		}
	}
	if updated == original {
		t.Fatal("managed section did not update")
	}
	second := runner.updateRoleWithResult(t.Context(), fixture.cfg, render.SourceCatalog{}, "sonarr", "saltbox")
	if second.Status != github.StatusUnchanged || fixture.readDoc(t) != updated {
		t.Fatalf("second update did not converge: %s, %s", second.Status, second.Error)
	}
}

func TestUpdateInvalidExamplePreservesPageAndReportsPath(t *testing.T) {
	fixture := newDockerUpdateFixture(t, "---\nsaltbox_automation:\n  inventory:\n    example_overrides:\n      sonarr_role_docker_enabled: {duplicate: 1, duplicate: 2}\n---\n", true, true)
	original := fixture.readDoc(t)
	result := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false).updateRoleWithResult(t.Context(), fixture.cfg, render.SourceCatalog{}, "sonarr", "saltbox")
	if result.Status != github.StatusError || !strings.Contains(result.Error, "sonarr_role_docker_enabled") || !strings.Contains(result.Error, fixture.docPath) {
		t.Fatalf("update error lacks page/example context: %s, %s", result.Status, result.Error)
	}
	if fixture.readDoc(t) != original {
		t.Fatal("failed update modified the page")
	}
}
