package automation

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/document"
	"github.com/saltyorg/docs-automation/github"
	"github.com/saltyorg/docs-automation/parser"
	"github.com/saltyorg/docs-automation/render"
)

func TestUpdateRepairsDockerMetadataBeforeRenderingAndSavesOnce(t *testing.T) {
	fixture := newDockerUpdateFixture(t, dockerUpdateFrontmatter("", "", "", "author-type", "release"), true, true)
	runner := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false)
	parses := 0
	saves := 0
	runner.parseRole = func(roleName, repoType, path string) (*parser.RoleInfo, error) {
		parses++
		return parser.New(roleName, repoType).ParseFile(path)
	}
	runner.saveDocument = func(manager *document.Manager, doc *document.Document) error {
		saves++
		return manager.SaveDocument(doc)
	}

	result := runner.updateRoleWithResult(t.Context(), fixture.cfg, render.SourceCatalog{}, "sonarr", "saltbox")
	if result.Status != github.StatusUpdated {
		t.Fatalf("first update status = %s, error = %q, skip = %q", result.Status, result.Error, result.SkipReason)
	}
	content := fixture.readDoc(t)
	for _, want := range []string{
		"icon: material/docker",
		"name: Image tags",
		"url: https://hub.docker.com/r/linuxserver/sonarr/tags",
		"type: author-type",
		"inventory=Image tags|https://hub.docker.com/r/linuxserver/sonarr/tags|material/docker",
		"overview=Image tags|https://hub.docker.com/r/linuxserver/sonarr/tags|author-type",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("updated document missing %q:\n%s", want, content)
		}
	}
	if parses != 1 || saves != 1 {
		t.Fatalf("first update parses = %d saves = %d, want 1 and 1", parses, saves)
	}

	result = runner.updateRoleWithResult(t.Context(), fixture.cfg, render.SourceCatalog{}, "sonarr", "saltbox")
	if result.Status != github.StatusUnchanged {
		t.Fatalf("second update status = %s, error = %q, skip = %q", result.Status, result.Error, result.SkipReason)
	}
	if got := fixture.readDoc(t); got != content {
		t.Fatalf("second update changed bytes:\nfirst:\n%s\nsecond:\n%s", content, got)
	}
	if parses != 2 || saves != 1 {
		t.Fatalf("two updates parses = %d saves = %d, want 2 and 1", parses, saves)
	}
}

func TestUpdateDockerMetadataPreservesEachNonEmptyAuthorField(t *testing.T) {
	fixture := newDockerUpdateFixture(t, dockerUpdateFrontmatter("author/icon", "Author releases", "", "author-type", "release"), false, false)
	result := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false).updateRoleWithResult(
		t.Context(), fixture.cfg, render.SourceCatalog{}, "sonarr", "saltbox",
	)
	if result.Status != github.StatusUpdated {
		t.Fatalf("update status = %s, error = %q, skip = %q", result.Status, result.Error, result.SkipReason)
	}
	content := fixture.readDoc(t)
	for _, want := range []string{
		"icon: author/icon",
		"name: Author releases",
		"url: https://hub.docker.com/r/linuxserver/sonarr/tags",
		"type: author-type",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("updated document missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "name: Image tags") || strings.Contains(content, "type: docker") {
		t.Fatalf("author fields were replaced:\n%s", content)
	}
}

func TestUpdateDockerMetadataRequiresEnabledPageAndOverviewAutomation(t *testing.T) {
	tests := []struct {
		name       string
		disabled   bool
		overviewOn bool
	}{
		{name: "page opt out", disabled: true, overviewOn: true},
		{name: "overview opt out", overviewOn: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frontmatter := dockerUpdateFrontmatter("", "", "", "author-type", "release")
			if tt.disabled {
				frontmatter = strings.Replace(frontmatter, "saltbox_automation:\n", "saltbox_automation:\n  disabled: true\n", 1)
			}
			if !tt.overviewOn {
				frontmatter = strings.Replace(frontmatter, "saltbox_automation:\n", "saltbox_automation:\n  sections:\n    overview: false\n", 1)
			}
			fixture := newDockerUpdateFixture(t, frontmatter, false, false)
			original := fixture.readDoc(t)
			result := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false).updateRoleWithResult(
				t.Context(), fixture.cfg, render.SourceCatalog{}, "sonarr", "saltbox",
			)
			if result.Status != github.StatusSkipped {
				t.Fatalf("status = %s, error = %q", result.Status, result.Error)
			}
			if got := fixture.readDoc(t); got != original {
				t.Fatalf("opted-out document changed:\n%s", got)
			}
		})
	}
}

func TestUpdateDockerMetadataRunsWithoutOverviewMarkers(t *testing.T) {
	fixture := newDockerUpdateFixture(t, dockerUpdateFrontmatter("", "", "", "author-type", "release"), false, false)
	result := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false).updateRoleWithResult(
		t.Context(), fixture.cfg, render.SourceCatalog{}, "sonarr", "saltbox",
	)
	if result.Status != github.StatusUpdated || !strings.Contains(fixture.readDoc(t), "icon: material/docker") {
		t.Fatalf("status = %s error = %q content:\n%s", result.Status, result.Error, fixture.readDoc(t))
	}
}

func TestUpdateDockerMetadataDoesNotCreateOrInferReleaseLinks(t *testing.T) {
	tests := []struct {
		name        string
		frontmatter string
	}{
		{name: "missing list", frontmatter: "---\nsaltbox_automation:\n  project_description:\n    name: Sonarr\n    summary: Summary\n---\n"},
		{name: "missing purpose", frontmatter: dockerUpdateFrontmatter("", "", "", "author-type", "")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newDockerUpdateFixture(t, tt.frontmatter, false, false)
			result := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false).updateRoleWithResult(
				t.Context(), fixture.cfg, render.SourceCatalog{}, "sonarr", "saltbox",
			)
			if result.Status != github.StatusUpdated {
				t.Fatalf("status = %s, error = %q", result.Status, result.Error)
			}
			content := fixture.readDoc(t)
			if !strings.Contains(content, "icon: material/docker") {
				t.Fatalf("independent icon was not filled:\n%s", content)
			}
			if strings.Contains(content, "purpose: release") || strings.Contains(content, "name: Image tags") || strings.Contains(content, "hub.docker.com") {
				t.Fatalf("release link was created or inferred:\n%s", content)
			}
		})
	}
}

func TestUpdateDockerMetadataIgnoreSuppressesOnlyURL(t *testing.T) {
	fixture := newDockerUpdateFixture(t, dockerUpdateFrontmatter("", "", "", "author-type", "release"), false, false)
	fixture.cfg.DockerMetadata.Ignore = []string{"lscr.io/linuxserver/sonarr"}
	result := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false).updateRoleWithResult(
		t.Context(), fixture.cfg, render.SourceCatalog{}, "sonarr", "saltbox",
	)
	content := fixture.readDoc(t)
	if result.Status != github.StatusUpdated || !strings.Contains(content, "icon: material/docker") || !strings.Contains(content, "name: Image tags") {
		t.Fatalf("status = %s error = %q content:\n%s", result.Status, result.Error, content)
	}
	if strings.Contains(content, "hub.docker.com") {
		t.Fatalf("ignored URL was filled:\n%s", content)
	}
}

func TestUpdateDockerMetadataLeavesUnresolvedURLBlank(t *testing.T) {
	fixture := newDockerUpdateFixture(t, dockerUpdateFrontmatter("", "", "", "author-type", "release"), false, false)
	writeDockerUpdateFile(t, filepath.Join(fixture.cfg.SaltboxRolesPath(), "sonarr", "defaults", "main.yml"), []byte("####################\n# Docker\n####################\nsonarr_role_docker_image_repo: \"{{ sonarr_image }}\"\n"))
	result := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false).updateRoleWithResult(
		t.Context(), fixture.cfg, render.SourceCatalog{}, "sonarr", "saltbox",
	)
	content := fixture.readDoc(t)
	if result.Status != github.StatusUpdated || !strings.Contains(content, "icon: material/docker") || !strings.Contains(content, "name: Image tags") {
		t.Fatalf("status = %s error = %q content:\n%s", result.Status, result.Error, content)
	}
	if strings.Contains(content, "url: http") {
		t.Fatalf("unresolved URL was filled:\n%s", content)
	}
}

func TestUpdateDockerMetadataDoesNotWritePartialChangesAfterRenderFailure(t *testing.T) {
	fixture := newDockerUpdateFixture(t, dockerUpdateFrontmatter("", "", "", "author-type", "release"), true, true)
	writeDockerUpdateFile(t, fixture.cfg.OverviewTemplatePath(), []byte("{{"))
	original := fixture.readDoc(t)
	runner := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false)
	saves := 0
	runner.saveDocument = func(*document.Manager, *document.Document) error {
		saves++
		return nil
	}
	result := runner.updateRoleWithResult(t.Context(), fixture.cfg, render.SourceCatalog{}, "sonarr", "saltbox")
	if result.Status != github.StatusError || !strings.Contains(result.Error, "loading overview template") {
		t.Fatalf("status = %s error = %q", result.Status, result.Error)
	}
	if saves != 0 {
		t.Fatalf("save calls = %d, want 0", saves)
	}
	if got := fixture.readDoc(t); got != original {
		t.Fatalf("failed update changed stored bytes:\n%s", got)
	}
}

type dockerUpdateFixture struct {
	cfg     *config.Config
	docPath string
}

func newDockerUpdateFixture(t *testing.T, frontmatter string, variablesMarker, overviewMarker bool) dockerUpdateFixture {
	t.Helper()
	root := t.TempDir()
	saltbox := filepath.Join(root, "saltbox")
	sandbox := filepath.Join(root, "sandbox")
	docs := filepath.Join(root, "docs")
	for _, directory := range []string{
		filepath.Join(saltbox, "roles", "sonarr", "defaults"),
		filepath.Join(sandbox, "roles"),
		filepath.Join(docs, "docs", "apps"),
		filepath.Join(docs, "templates"),
	} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeDockerUpdateFile(t, filepath.Join(saltbox, "roles", "sonarr", "defaults", "main.yml"), []byte("####################\n# Docker\n####################\nsonarr_role_docker_image_repo: lscr.io/linuxserver/sonarr\nsonarr_role_docker_enabled: true\n"))
	writeDockerUpdateFile(t, filepath.Join(docs, "templates", "inventory.md.tmpl"), []byte("inventory={{ (index .Config.AppLinks 0).Name }}|{{ (index .Config.AppLinks 0).URL }}|{{ .GlobalConfig.DockerMetadata.Icon }}\n"))
	writeDockerUpdateFile(t, filepath.Join(docs, "templates", "overview.md.tmpl"), []byte("overview={{ (index .Links 0).Name }}|{{ (index .Links 0).URL }}|{{ (index .Links 0).Type }}\n"))
	body := "# Sonarr\n"
	if variablesMarker {
		body += "<!-- BEGIN VARIABLES -->\nold inventory\n<!-- END VARIABLES -->\n"
	}
	if overviewMarker {
		body += "<!-- BEGIN OVERVIEW -->\nold overview\n<!-- END OVERVIEW -->\n"
	}
	docPath := filepath.Join(docs, "docs", "apps", "sonarr.md")
	writeDockerUpdateFile(t, docPath, []byte(frontmatter+body))
	return dockerUpdateFixture{
		cfg: &config.Config{
			Repositories: config.RepositoryConfig{Saltbox: saltbox, Sandbox: sandbox, Docs: docs},
			Markers:      config.MarkersConfig{Variables: "VARIABLES", Overview: "OVERVIEW"},
			DockerMetadata: config.DockerMetadataConfig{
				Icon:        "material/docker",
				ReleaseLink: config.DockerMetadataReleaseLink{Name: "Image tags"},
				Rules: []config.DockerMetadataRule{{
					Pattern: `^lscr\.io/linuxserver/([^/]+)$`,
					URL:     "https://hub.docker.com/r/linuxserver/$1/tags",
					Type:    "docker",
				}},
			},
		},
		docPath: docPath,
	}
}

func dockerUpdateFrontmatter(icon, name, url, linkType, purpose string) string {
	return "---\nicon: " + icon + "\nsaltbox_automation:\n  app_links:\n    - name: " + name + "\n      url: " + url + "\n      type: " + linkType + "\n      purpose: " + purpose + "\n  project_description:\n    name: Sonarr\n    summary: Summary\n---\n"
}

func (f dockerUpdateFixture) readDoc(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile(f.docPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func writeDockerUpdateFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}
