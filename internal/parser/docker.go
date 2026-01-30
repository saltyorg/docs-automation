package parser

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	// docker_var lookup pattern: lookup('docker_var', '_docker_suffix')
	dockerVarLookupRe = regexp.MustCompile(`lookup\s*\(\s*['"]docker_var['"]\s*,\s*['"]([^'"]+)['"]`)
)

// DockerVarScanner scans for docker_var lookups in resources/tasks/docker/*.yml files.
type DockerVarScanner struct {
	resourcesPath string
	cache         map[string]bool
}

// NewDockerVarScanner creates a new scanner for the given resources path.
func NewDockerVarScanner(resourcesPath string) *DockerVarScanner {
	return &DockerVarScanner{
		resourcesPath: resourcesPath,
	}
}

// FindDockerVarLookups scans docker task files and returns all docker_var suffixes found.
// Suffixes are returned without the leading '_docker_' prefix.
func (s *DockerVarScanner) FindDockerVarLookups() ([]string, error) {
	if s.cache != nil {
		return mapKeys(s.cache), nil
	}

	s.cache = make(map[string]bool)
	dockerTasksPath := filepath.Join(s.resourcesPath, "tasks", "docker")

	entries, err := os.ReadDir(dockerTasksPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}

		filePath := filepath.Join(dockerTasksPath, entry.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		matches := dockerVarLookupRe.FindAllStringSubmatch(string(content), -1)
		for _, match := range matches {
			if len(match) > 1 {
				// Strip the leading _docker_ prefix if present
				suffix := strings.TrimPrefix(match[1], "_docker_")
				s.cache[suffix] = true
			}
		}

		addDockerVarSpecsToCache(s.cache, content)
	}

	return mapKeys(s.cache), nil
}

// addDockerVarSpecsToCache collects docker var suffixes from _docker_var_specs mappings.
func addDockerVarSpecsToCache(cache map[string]bool, content []byte) {
	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil {
		return
	}

	var walk func(*yaml.Node)
	walk = func(node *yaml.Node) {
		switch node.Kind {
		case yaml.DocumentNode, yaml.SequenceNode:
			for _, child := range node.Content {
				walk(child)
			}
		case yaml.MappingNode:
			for i := 0; i+1 < len(node.Content); i += 2 {
				key := node.Content[i]
				value := node.Content[i+1]

				if key.Kind == yaml.ScalarNode && key.Value == "_docker_var_specs" && value.Kind == yaml.MappingNode {
					for j := 0; j+1 < len(value.Content); j += 2 {
						specKey := value.Content[j]
						if specKey.Kind != yaml.ScalarNode {
							continue
						}
						if !strings.HasPrefix(specKey.Value, "_docker_") {
							continue
						}
						suffix := strings.TrimPrefix(specKey.Value, "_docker_")
						if suffix != "" {
							cache[suffix] = true
						}
					}
				}

				walk(value)
			}
		}
	}

	walk(&root)
}

// GetDockerVarSuffixes returns docker variables that are NOT defined in the role's defaults.
// This identifies "additional" docker options available via create_docker_container
// but not explicitly defined in the role.
// The roleName is used to match the pattern {role}_role_docker_{suffix}.
func (s *DockerVarScanner) GetDockerVarSuffixes(roleName string, roleDockerVars []string) ([]string, error) {
	allDockerVars, err := s.FindDockerVarLookups()
	if err != nil {
		return nil, err
	}

	// Build a set of suffixes defined in the role
	// Match pattern: {role}_role_docker_{suffix}
	// e.g., plex_role_docker_envs -> envs
	//       plex_role_docker_ports_default -> ports_default
	prefix := roleName + "_role_docker_"
	roleVarSuffixes := make(map[string]bool)
	for _, varName := range roleDockerVars {
		if after, ok := strings.CutPrefix(varName, prefix); ok {
			suffix := after
			roleVarSuffixes[suffix] = true
		}
	}

	// Filter out suffixes that are already defined in the role
	var additionalVars []string
	for _, suffix := range allDockerVars {
		if !roleVarSuffixes[suffix] {
			additionalVars = append(additionalVars, suffix)
		}
	}

	return additionalVars, nil
}

// CategorizeDockerVars groups docker variable suffixes into categories.
func CategorizeDockerVars(suffixes []string) map[string][]string {
	categories := make(map[string][]string)

	for _, suffix := range suffixes {
		category := getDockerVarCategory(suffix)
		categories[category] = append(categories[category], suffix)
	}

	return categories
}

// DockerVarCategoryOrder returns the preferred order for docker variable categories.
func DockerVarCategoryOrder() []string {
	return []string{
		"Resource Limits",
		"Security & Devices",
		"Networking",
		"Storage",
		"Monitoring & Lifecycle",
		"Other Options",
	}
}

// getDockerVarCategory determines the category for a docker variable suffix.
func getDockerVarCategory(suffix string) string {
	switch {
	case containsAny(suffix, "cpu", "memory", "blkio", "kernel", "shm"):
		return "Resource Limits"
	case containsAny(suffix, "device", "cap_", "privileged", "security", "user", "groups", "userns", "cgroupns"):
		return "Security & Devices"
	case containsAny(suffix, "network", "dns", "hostname", "hosts", "domainname", "ports", "exposed", "links", "ipc", "pid", "uts"):
		return "Networking"
	case containsAny(suffix, "volume", "mount", "working_dir", "tmpfs", "storage"):
		return "Storage"
	case containsAny(suffix, "log", "healthcheck", "init", "restart", "stop", "kill", "recreate", "cleanup", "keep", "oom", "paused", "detach", "output", "auto_remove", "healthy"):
		return "Monitoring & Lifecycle"
	default:
		return "Other Options"
	}
}

// containsAny checks if the string contains any of the substrings.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// mapKeys returns all keys from a map as a slice.
func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// GetDockerVarType returns the type for a docker variable suffix based on Ansible docker_container module.
func GetDockerVarType(suffix string) string {
	switch suffix {
	// Boolean options
	case "auto_remove", "cleanup", "detach", "init", "keep_volumes", "oom_killer",
		"output_logs", "paused", "privileged", "read_only", "recreate",
		"image_pull", "hosts_use_common", "labels_use_common", "volumes_global":
		return "bool"

	// Integer options
	case "blkio_weight", "cpu_period", "cpu_quota", "cpu_shares", "healthy_wait_timeout",
		"memory_swappiness", "oom_score_adj", "restart_retries", "stop_timeout", "create_timeout":
		return "int"

	// List options
	case "capabilities", "cap_drop", "commands", "device_cgroup_rules", "device_read_bps",
		"device_read_iops", "device_requests", "device_write_bps", "device_write_iops",
		"devices", "dns_opts", "dns_search_domains", "dns_servers", "exposed_ports",
		"groups", "links", "mounts", "networks", "ports", "security_opts", "sysctls",
		"tmpfs", "ulimits", "volumes", "volumes_from":
		return "list"

	// Dict options
	case "envs", "healthcheck", "hosts", "labels", "log_options", "storage_opts":
		return "dict"

	// Everything else is string
	default:
		return "string"
	}
}

// GetDockerVarTypeComment returns a formatted type comment for a docker variable.
func GetDockerVarTypeComment(suffix string) string {
	varType := GetDockerVarType(suffix)
	switch varType {
	case "bool":
		return "# Type: bool (true/false)"
	case "int":
		return "# Type: int"
	case "list":
		return "# Type: list"
	case "dict":
		return "# Type: dict"
	default:
		return "# Type: string"
	}
}
