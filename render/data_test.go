package render

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/parser"
)

func TestBuildRoleDataAppliesDockerOverrideMetadata(t *testing.T) {
	variable := parser.Variable{
		Name:     "plex_role_docker_gpu_enabled",
		RawValue: "true",
		Section:  "Docker",
		Comment:  "GPU",
	}
	role := &parser.RoleInfo{
		Name:         "plex",
		RepoType:     "saltbox",
		HasDocker:    true,
		SectionOrder: []string{"Docker"},
		Sections: map[string]*parser.Section{
			"Docker": {
				Name:      "Docker",
				Variables: []parser.Variable{variable},
			},
		},
		AllVariables: []parser.Variable{variable},
	}
	cfg := &config.Config{
		DockerOverrides: config.DockerOverrides{
			Variables: map[string]config.OverrideVarDef{
				"_docker_gpu_enabled": {
					Description: "Enable automatic GPU access",
					Type:        "bool",
				},
			},
		},
	}

	data := BuildRoleData(role, cfg, nil, SourceCatalog{})
	got := data.Sections["Docker"].Variables[0]
	if got.Comment != "GPU" {
		t.Fatalf("comment = %q, want original role comment", got.Comment)
	}
	if !slices.Equal(got.CommentLines, []string{"GPU"}) {
		t.Fatalf("comment lines = %v, want original role comment", got.CommentLines)
	}
	if got.Description != "Enable automatic GPU access" {
		t.Fatalf("description = %q, want configured description", got.Description)
	}
	if got.Type != "bool" {
		t.Fatalf("type = %q, want bool", got.Type)
	}
	if got.RawValue != "true" {
		t.Fatalf("raw value = %q, want role default to remain unchanged", got.RawValue)
	}

	legacyData := BuildRoleData(role, &config.Config{}, nil, SourceCatalog{})
	legacyVar := legacyData.Sections["Docker"].Variables[0]
	if legacyVar.Comment != "GPU" {
		t.Fatalf("legacy comment = %q, want original role comment", legacyVar.Comment)
	}
	if legacyVar.Description != "" {
		t.Fatalf("legacy description = %q, want empty description", legacyVar.Description)
	}
}

func TestBuildRoleDataAppliesSectionExplainer(t *testing.T) {
	variable := parser.Variable{
		Name:     "example_role_port_conflict_policy",
		RawValue: `"fail"`,
		Section:  "Ports",
	}
	role := &parser.RoleInfo{
		Name:         "example",
		RepoType:     "saltbox",
		SectionOrder: []string{"Ports"},
		Sections: map[string]*parser.Section{
			"Ports": {
				Name:      "Ports",
				Variables: []parser.Variable{variable},
			},
		},
		AllVariables: []parser.Variable{variable},
	}
	cfg := &config.Config{
		SectionExplainers: map[string]string{
			"Ports": "  Ports stay stable across runs.\n\n  Explicit conflicts always fail.  ",
		},
	}

	data := BuildRoleData(role, cfg, nil, SourceCatalog{})
	if got, want := data.Sections["Ports"].Explainer, "Ports stay stable across runs.\n\n  Explicit conflicts always fail."; got != want {
		t.Fatalf("Ports explainer = %q, want %q", got, want)
	}

	legacyData := BuildRoleData(role, &config.Config{}, nil, SourceCatalog{})
	if got := legacyData.Sections["Ports"].Explainer; got != "" {
		t.Fatalf("legacy Ports explainer = %q, want empty", got)
	}
}

func TestBuildDockerInfoPreservesUnconfiguredVariablesAndAddsMetadata(t *testing.T) {
	defaultValue := "false"
	cfg := &config.Config{
		DockerOverrides: config.DockerOverrides{
			Variables: map[string]config.OverrideVarDef{
				"_docker_gpu_enabled": {
					Description: "Enable automatic GPU access",
					Default:     &defaultValue,
					Type:        "bool",
				},
			},
		},
	}

	suffixes := []string{"custom_option", "gpu_enabled"}
	info := buildDockerInfo(cfg, "example", nil, suffixes)
	if info == nil {
		t.Fatal("DockerInfo is nil")
	}
	if got := info.Overrides["gpu_enabled"]; got == nil {
		t.Fatal("gpu_enabled metadata is missing")
	} else {
		if got.Description != "Enable automatic GPU access" {
			t.Fatalf("description = %q, want configured description", got.Description)
		}
		if !got.HasDefault || got.Default != "false" {
			t.Fatalf("default = %q, has default = %v; want false and true", got.Default, got.HasDefault)
		}
		if got.Type != "bool" {
			t.Fatalf("type = %q, want bool", got.Type)
		}
	}
	if _, exists := info.Overrides["custom_option"]; exists {
		t.Fatal("unconfigured custom option unexpectedly has metadata")
	}

	other := info.Categories["Other Options"]
	if !slices.Contains(other, "custom_option") || !slices.Contains(other, "gpu_enabled") {
		t.Fatalf("other options = %v, want existing and configured variables", other)
	}

	legacyInfo := buildDockerInfo(&config.Config{}, "example", nil, suffixes)
	if legacyInfo == nil {
		t.Fatal("legacy DockerInfo is nil")
	}
	if len(legacyInfo.Overrides) != 0 {
		t.Fatalf("legacy overrides = %v, want no metadata", legacyInfo.Overrides)
	}
	if !slices.Equal(legacyInfo.Categories["Other Options"], other) {
		t.Fatalf("legacy categories = %v, want %v", legacyInfo.Categories["Other Options"], other)
	}
}

func TestFindDockerOverrideAcceptsFullAndNormalizedSuffixes(t *testing.T) {
	full := config.OverrideVarDef{Description: "full"}
	normalized := config.OverrideVarDef{Description: "normalized"}

	if got, ok := findDockerOverride(map[string]config.OverrideVarDef{"_docker_gpu_enabled": full}, "gpu_enabled"); !ok || got.Description != "full" {
		t.Fatalf("full suffix lookup = %#v, %v; want full metadata", got, ok)
	}
	if got, ok := findDockerOverride(map[string]config.OverrideVarDef{"gpu_enabled": normalized}, "gpu_enabled"); !ok || got.Description != "normalized" {
		t.Fatalf("normalized suffix lookup = %#v, %v; want normalized metadata", got, ok)
	}
}

func TestBuildRoleDataPromotesCompleteDockerOverrideGroup(t *testing.T) {
	tests := []struct {
		name         string
		primaryValue string
	}{
		{name: "primary enabled", primaryValue: "true"},
		{name: "primary disabled", primaryValue: "false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			variables := []parser.Variable{
				{Name: "plex_role_docker_container", RawValue: `"plex"`, Section: "Docker", Comment: "Container"},
				{Name: "plex_role_docker_gpu_enabled", RawValue: tt.primaryValue, Section: "Docker", Comment: "GPU"},
				{Name: "plex_role_docker_image", RawValue: `"plex:latest"`, Section: "Docker", Comment: "Image"},
			}
			if tt.primaryValue == "true" {
				variables = append(variables, parser.Variable{
					Name:     "plex_role_docker_nvidia_disabled",
					RawValue: "true",
					Section:  "Docker",
				})
			}

			role := dockerTestRole("plex", variables)
			role.HasInstances = true
			role.InstancesVar = "plex_instances"
			cfg := dockerGroupTestConfig("")
			data := BuildRoleData(role, cfg, nil, SourceCatalog{})
			got := data.Sections["Docker"].Variables

			wantNames := []string{
				"plex_role_docker_container",
				"plex_role_docker_gpu_enabled",
				"plex_role_docker_nvidia_disabled",
				"plex_role_docker_dev_dri_disabled",
				"plex_role_docker_image",
			}
			if names := variableNames(got); !slices.Equal(names, wantNames) {
				t.Fatalf("variable order = %v, want %v", names, wantNames)
			}
			if got[1].Comment != "GPU" || !slices.Equal(got[1].CommentLines, []string{"GPU"}) {
				t.Fatalf("primary heading = %q, %v; want GPU", got[1].Comment, got[1].CommentLines)
			}
			if got[2].Comment != "" || got[3].Comment != "" {
				t.Fatalf("companion comments = %q, %q; want empty", got[2].Comment, got[3].Comment)
			}
			if got[1].RawValue != tt.primaryValue {
				t.Fatalf("primary value = %q, want %q", got[1].RawValue, tt.primaryValue)
			}
			wantNvidia := "false"
			if tt.primaryValue == "true" {
				wantNvidia = "true"
			}
			if got[2].RawValue != wantNvidia {
				t.Fatalf("NVIDIA opt-out value = %q, want %q", got[2].RawValue, wantNvidia)
			}
			if got[3].RawValue != "false" {
				t.Fatalf("DRI opt-out value = %q, want false", got[3].RawValue)
			}
			if got[1].InstanceName != "plex2_docker_gpu_enabled" ||
				got[2].InstanceName != "plex2_docker_nvidia_disabled" ||
				got[3].InstanceName != "plex2_docker_dev_dri_disabled" {
				t.Fatalf("instance names = %q, %q, %q", got[1].InstanceName, got[2].InstanceName, got[3].InstanceName)
			}
		})
	}
}

func TestBuildRoleDataPlacesCompleteGroupInDockerPlusWhenPrimaryIsAbsent(t *testing.T) {
	dockerVarSuffixes := []string{
		"gpu_enabled",
		"nvidia_disabled",
		"dev_dri_disabled",
		"custom_option",
	}
	cfg := dockerGroupTestConfig("")
	role := dockerTestRole("example", []parser.Variable{{
		Name:     "example_role_docker_container",
		RawValue: `"example"`,
		Section:  "Docker",
		Comment:  "Container",
	}})

	data := BuildRoleData(role, cfg, nil, SourceCatalog{DockerVarSuffixes: dockerVarSuffixes})
	if data.DockerInfo == nil {
		t.Fatal("DockerInfo is nil")
	}
	wantGPU := []string{"gpu_enabled", "nvidia_disabled", "dev_dri_disabled"}
	if got := data.DockerInfo.Categories["GPU"]; !slices.Equal(got, wantGPU) {
		t.Fatalf("GPU category = %v, want %v", got, wantGPU)
	}
	if len(data.DockerInfo.CategoryOrder) == 0 || data.DockerInfo.CategoryOrder[0] != "GPU" {
		t.Fatalf("category order = %v, want GPU first", data.DockerInfo.CategoryOrder)
	}
	if got := data.DockerInfo.Categories["Other Options"]; !slices.Equal(got, []string{"custom_option"}) {
		t.Fatalf("other options = %v, want custom_option", got)
	}
	if len(data.Sections["Docker"].Variables) != 1 {
		t.Fatalf("Docker variables = %v, want only role-defined container", variableNames(data.Sections["Docker"].Variables))
	}
}

func TestBuildRoleDataUsesConfiguredDockerVariableType(t *testing.T) {
	cfg := dockerGroupTestConfig("")
	cfg.DockerVariables.Dict = []string{"sysctls"}
	role := dockerTestRole("example", []parser.Variable{
		{Name: "example_role_docker_container", RawValue: `"example"`, Section: "Docker"},
	})

	data := BuildRoleData(role, cfg, nil, SourceCatalog{DockerVarSuffixes: []string{"sysctls"}})
	if data.DockerInfo == nil {
		t.Fatal("BuildRoleData() DockerInfo is nil")
	}
	if got := data.DockerInfo.Types["sysctls"]; got != parser.Dict {
		t.Fatalf("BuildRoleData() sysctls type = %q, want %q", got, parser.Dict)
	}
}

func TestBuildRoleDataUsesExplicitSourceCatalog(t *testing.T) {
	role := dockerTestRole("example", []parser.Variable{
		{Name: "example_role_docker_container", RawValue: `"example"`, Section: "Docker"},
	})
	role.HasWeb = true
	cfg := &config.Config{
		Repositories: config.RepositoryConfig{Saltbox: filepath.Join(t.TempDir(), "missing")},
	}
	sources := SourceCatalog{
		RoleVarLookups:    map[string]string{"_web_host_override": parser.String},
		DockerVarSuffixes: []string{"custom_option"},
	}

	data := BuildRoleData(role, cfg, nil, sources)
	if got := data.RoleVarLookups["_web_host_override"]; got == nil || got.Type != parser.String {
		t.Fatalf("RoleVarLookups = %v, want string web host override", data.RoleVarLookups)
	}
	if data.DockerInfo == nil || !slices.Contains(data.DockerInfo.Categories["Other Options"], "custom_option") {
		t.Fatalf("DockerInfo = %#v, want custom_option", data.DockerInfo)
	}
}

func TestBuildRoleDataGatesPathLookupsByRepositoryCallerCapability(t *testing.T) {
	cfg := &config.Config{}
	sources := SourceCatalog{
		RoleVarLookups: map[string]string{
			"_paths_owner":       parser.String,
			"_paths_recursive":   parser.Bool,
			"_web_host_override": parser.String,
		},
		ManagedDirectoryRoles: map[string]map[string]struct{}{
			"saltbox": {"shared": {}},
		},
	}
	role := &parser.RoleInfo{Name: "shared", RepoType: "saltbox", HasWeb: true}

	capable := BuildRoleData(role, cfg, nil, sources)
	if !sources.HasManagedDirectories("saltbox", "shared") {
		t.Fatal("HasManagedDirectories(saltbox, shared) = false, want true")
	}
	if capable.RoleVarLookups["_paths_owner"] == nil || capable.RoleVarLookups["_paths_recursive"] == nil {
		t.Fatalf("capable RoleVarLookups = %v, want path overrides", capable.RoleVarLookups)
	}
	if capable.RoleVarLookups["_web_host_override"] == nil {
		t.Fatalf("capable RoleVarLookups = %v, want unchanged web override", capable.RoleVarLookups)
	}

	role.RepoType = "sandbox"
	incapable := BuildRoleData(role, cfg, nil, sources)
	if sources.HasManagedDirectories("sandbox", "shared") {
		t.Fatal("HasManagedDirectories(sandbox, shared) = true, want false")
	}
	if incapable.RoleVarLookups["_paths_owner"] != nil || incapable.RoleVarLookups["_paths_recursive"] != nil {
		t.Fatalf("incapable RoleVarLookups = %v, want no path overrides", incapable.RoleVarLookups)
	}
	if incapable.RoleVarLookups["_web_host_override"] == nil {
		t.Fatalf("incapable RoleVarLookups = %v, want unchanged web override", incapable.RoleVarLookups)
	}
}

func TestBuildRoleDataRemovesPromotedGroupFromDockerPlus(t *testing.T) {
	dockerVarSuffixes := []string{
		"gpu_enabled",
		"nvidia_disabled",
		"dev_dri_disabled",
		"custom_option",
	}
	cfg := dockerGroupTestConfig("")
	role := dockerTestRole("plex", []parser.Variable{
		{Name: "plex_role_docker_gpu_enabled", RawValue: "true", Section: "Docker", Comment: "GPU"},
		{Name: "plex_role_docker_container", RawValue: `"plex"`, Section: "Docker", Comment: "Container"},
	})

	data := BuildRoleData(role, cfg, nil, SourceCatalog{DockerVarSuffixes: dockerVarSuffixes})
	if data.DockerInfo == nil {
		t.Fatal("DockerInfo is nil")
	}
	if _, exists := data.DockerInfo.Categories["GPU"]; exists {
		t.Fatalf("Docker+ categories = %v, want no GPU category", data.DockerInfo.Categories)
	}
	for category, suffixes := range data.DockerInfo.Categories {
		for _, suffix := range suffixes {
			if slices.Contains([]string{"gpu_enabled", "nvidia_disabled", "dev_dri_disabled"}, suffix) {
				t.Fatalf("%s contains promoted suffix %q", category, suffix)
			}
		}
	}
	if got := variableNames(data.Sections["Docker"].Variables); !slices.Equal(got[:3], []string{
		"plex_role_docker_gpu_enabled",
		"plex_role_docker_nvidia_disabled",
		"plex_role_docker_dev_dri_disabled",
	}) {
		t.Fatalf("promoted variables = %v", got)
	}
}

func TestBuildDockerInfoUsesConfiguredGroupWithoutGPUNames(t *testing.T) {
	defaultValue := "false"
	cfg := &config.Config{
		DockerOverrides: config.DockerOverrides{
			Groups: []config.DockerOverrideGroup{{
				Name:       "Acceleration",
				Primary:    "_docker_accelerator_enabled",
				Companions: []string{"_docker_accelerator_mode"},
			}},
			Variables: map[string]config.OverrideVarDef{
				"_docker_accelerator_enabled": {Default: &defaultValue, Type: "bool"},
				"_docker_accelerator_mode":    {Type: "string"},
			},
		},
	}

	info := buildDockerInfo(cfg, "example", nil, []string{"accelerator_enabled", "accelerator_mode"})
	if info == nil {
		t.Fatal("DockerInfo is nil")
	}
	want := []string{"accelerator_enabled", "accelerator_mode"}
	if got := info.Categories["Acceleration"]; !slices.Equal(got, want) {
		t.Fatalf("Acceleration category = %v, want %v", got, want)
	}
}

func TestBuildDockerInfoPreservesLegacyNonRolePrefixBehavior(t *testing.T) {
	info := buildDockerInfo(
		&config.Config{},
		"n8n",
		[]string{"n8n_docker_create_timeout"},
		[]string{"create_timeout"},
	)
	if info == nil || !slices.Contains(info.Categories["Other Options"], "create_timeout") {
		t.Fatalf("DockerInfo = %#v, want legacy create_timeout Docker+ option", info)
	}
}

func dockerGroupTestConfig(root string) *config.Config {
	defaultValue := "false"
	return &config.Config{
		Repositories: config.RepositoryConfig{Saltbox: root},
		DockerOverrides: config.DockerOverrides{
			Groups: []config.DockerOverrideGroup{{
				Name:       "GPU",
				Primary:    "_docker_gpu_enabled",
				Companions: []string{"_docker_nvidia_disabled", "_docker_dev_dri_disabled"},
			}},
			Variables: map[string]config.OverrideVarDef{
				"_docker_gpu_enabled":      {Description: "Enable GPU access", Default: &defaultValue, Type: "bool"},
				"_docker_nvidia_disabled":  {Description: "Disable NVIDIA access", Default: &defaultValue, Type: "bool"},
				"_docker_dev_dri_disabled": {Description: "Disable DRI access", Default: &defaultValue, Type: "bool"},
			},
		},
	}
}

func dockerTestRole(name string, variables []parser.Variable) *parser.RoleInfo {
	return &parser.RoleInfo{
		Name:         name,
		RepoType:     "saltbox",
		HasDocker:    true,
		SectionOrder: []string{"Docker"},
		Sections: map[string]*parser.Section{
			"Docker": {Name: "Docker", Variables: variables},
		},
		AllVariables: variables,
	}
}

func variableNames(variables []*VariableData) []string {
	names := make([]string, 0, len(variables))
	for _, variable := range variables {
		names = append(names, variable.Name)
	}
	return names
}
