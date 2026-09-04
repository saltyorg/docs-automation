package automation

import (
	"testing"

	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/parser"
)

func TestResolveDockerMetadataUsesOverrideIgnoreAndOrderedRules(t *testing.T) {
	cfg := config.DockerMetadataConfig{
		Icon:        "material/docker",
		ReleaseLink: config.DockerMetadataReleaseLink{Name: "Image tags"},
		Overrides: map[string]config.DockerMetadataTarget{
			"ghcr.io/imagegenius/immich": {URL: "https://override.invalid/tags", Type: "override"},
		},
		Rules: []config.DockerMetadataRule{
			{Pattern: `^ghcr\.io/([^/]+)/([^/]+)$`, URL: "https://first.invalid/$1/$2", Type: "first"},
			{Pattern: `^ghcr\.io/(.+)$`, URL: "https://second.invalid/$1", Type: "second"},
			{Pattern: `^lscr\.io/linuxserver/([^/]+)$`, URL: "https://hub.docker.com/r/linuxserver/$1/tags", Type: "docker"},
			{Pattern: `^public\.ecr\.aws/([^/]+)/([^/]+)$`, URL: "https://gallery.ecr.aws/$1/$2", Type: "ecr"},
			{Pattern: `^codeberg\.org/([^/]+)/([^/]+)$`, URL: "https://codeberg.org/$1/-/packages/container/$2/versions", Type: "codeberg"},
			{Pattern: `^docker\.io/([^/]+)/([^/]+)$`, URL: "https://hub.docker.com/r/$1/$2/tags", Type: "docker"},
			{Pattern: `^docker\.io/([^/]+)$`, URL: "https://hub.docker.com/_/$1/tags", Type: "docker"},
			{Pattern: `^([^/]+)/([^/]+)$`, URL: "https://hub.docker.com/r/$1/$2/tags", Type: "docker"},
			{Pattern: `^([^/]+)$`, URL: "https://hub.docker.com/_/$1/tags", Type: "docker"},
		},
		Ignore: []string{"ignored/image"},
	}
	tests := []struct {
		name     string
		repo     string
		wantURL  string
		wantType string
	}{
		{name: "exact override precedes rules", repo: " GHCR.IO/ImageGenius/Immich ", wantURL: "https://override.invalid/tags", wantType: "override"},
		{name: "ignore suppresses rules", repo: "IGNORED/IMAGE"},
		{name: "first matching rule wins", repo: "ghcr.io/acme/widget", wantURL: "https://first.invalid/acme/widget", wantType: "first"},
		{name: "linuxserver", repo: "lscr.io/linuxserver/sonarr", wantURL: "https://hub.docker.com/r/linuxserver/sonarr/tags", wantType: "docker"},
		{name: "public ecr", repo: "public.ecr.aws/team/widget", wantURL: "https://gallery.ecr.aws/team/widget", wantType: "ecr"},
		{name: "codeberg", repo: "codeberg.org/team/widget", wantURL: "https://codeberg.org/team/-/packages/container/widget/versions", wantType: "codeberg"},
		{name: "explicit docker owner", repo: "docker.io/team/widget", wantURL: "https://hub.docker.com/r/team/widget/tags", wantType: "docker"},
		{name: "explicit docker official", repo: "docker.io/nginx", wantURL: "https://hub.docker.com/_/nginx/tags", wantType: "docker"},
		{name: "unqualified owner", repo: "team/widget", wantURL: "https://hub.docker.com/r/team/widget/tags", wantType: "docker"},
		{name: "unqualified official", repo: "nginx", wantURL: "https://hub.docker.com/_/nginx/tags", wantType: "docker"},
		{name: "unsupported registry", repo: "registry.gitlab.com/team/widget"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveDockerRepository(tt.repo, cfg)
			if got.URL != tt.wantURL || got.Type != tt.wantType {
				t.Fatalf("resolveDockerRepository(%q) = %#v, want URL %q type %q", tt.repo, got, tt.wantURL, tt.wantType)
			}
		})
	}
}

func TestResolveDockerMetadataExpandsValidatedTemplateExactly(t *testing.T) {
	cfg := config.DockerMetadataConfig{
		Icon:        "material/docker",
		ReleaseLink: config.DockerMetadataReleaseLink{Name: "Image tags"},
		Rules: []config.DockerMetadataRule{{
			Pattern: `^(?P<owner>[^/]+)/([^/]+)$`,
			URL:     "https://example.invalid/$0/${owner}/$2/$$literal",
			Type:    "example",
		}},
	}

	got := resolveDockerRepository("Acme/Widget", cfg)
	want := config.DockerMetadataTarget{
		URL:  "https://example.invalid/acme/widget/acme/widget/$literal",
		Type: "example",
	}
	if got != want {
		t.Fatalf("resolveDockerRepository() = %#v, want %#v", got, want)
	}
}

func TestDockerRepositoryRequiresExactPrimaryVariableInExactDockerSection(t *testing.T) {
	tests := []struct {
		name string
		role *parser.RoleInfo
		want string
	}{
		{name: "primary direct", role: dockerMetadataRole("sonarr", "Docker", parser.Variable{Name: "sonarr_role_docker_image_repo", RawValue: `lscr.io/linuxserver/sonarr`}), want: "lscr.io/linuxserver/sonarr"},
		{name: "primary subsection", role: dockerMetadataSubsectionRole("sonarr", parser.Variable{Name: "sonarr_role_docker_image_repo", RawValue: `'lscr.io/linuxserver/sonarr'`}), want: "lscr.io/linuxserver/sonarr"},
		{name: "secondary only", role: dockerMetadataRole("sonarr", "Docker", parser.Variable{Name: "sonarr_role_docker_image_repo_postgres", RawValue: "postgres"})},
		{name: "wrong section", role: dockerMetadataRole("sonarr", "Images", parser.Variable{Name: "sonarr_role_docker_image_repo", RawValue: "sonarr"})},
		{name: "wrong section case", role: dockerMetadataRole("sonarr", "docker", parser.Variable{Name: "sonarr_role_docker_image_repo", RawValue: "sonarr"})},
		{name: "other role variable", role: dockerMetadataRole("sonarr", "Docker", parser.Variable{Name: "radarr_role_docker_image_repo", RawValue: "radarr"})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dockerRepository(tt.role); got != tt.want {
				t.Fatalf("dockerRepository() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDockerRepositoryRejectsJinjaAndNonliteralYAML(t *testing.T) {
	values := []string{
		"", "   ", "null", "true", "42", "[]", "{}", "[repo]", "{repo: value}",
		"|\n  ghcr.io/acme/widget", ">\n  ghcr.io/acme/widget",
		`"{{ role_image }}"`, `'{% if enabled %}repo{% endif %}'`, "&repo ghcr.io/acme/widget", "*repo",
		"ghcr.io/acme/widget\n---\nother",
	}
	for _, value := range values {
		t.Run(value, func(t *testing.T) {
			role := dockerMetadataRole("widget", "Docker", parser.Variable{Name: "widget_role_docker_image_repo", RawValue: value})
			if got := dockerRepository(role); got != "" {
				t.Fatalf("dockerRepository() = %q for RawValue %q, want unresolved", got, value)
			}
		})
	}
}

func dockerMetadataRole(name, sectionName string, variable parser.Variable) *parser.RoleInfo {
	section := &parser.Section{Name: sectionName, Variables: []parser.Variable{variable}}
	return &parser.RoleInfo{Name: name, Sections: map[string]*parser.Section{sectionName: section}}
}

func dockerMetadataSubsectionRole(name string, variable parser.Variable) *parser.RoleInfo {
	section := &parser.Section{Name: "Docker", Subsections: map[string][]parser.Variable{"Images": {variable}}, SubsectionOrder: []string{"Images"}}
	return &parser.RoleInfo{Name: name, Sections: map[string]*parser.Section{"Docker": section}}
}
