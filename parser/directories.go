package parser

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

const managedDirectoriesTask = "{{ resources_tasks_path }}/directories/create_directories.yml"

// ScanManagedDirectoryRoles returns roles that directly include the shared
// managed-directory task through its fully qualified Ansible action.
func ScanManagedDirectoryRoles(rolesPath string) (map[string]struct{}, error) {
	roles, _, err := scanManagedDirectoryRoles(rolesPath)
	return roles, err
}

func scanManagedDirectoryRoles(rolesPath string) (map[string]struct{}, int, error) {
	roles := make(map[string]struct{})
	occurrences := 0
	err := filepath.WalkDir(rolesPath, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("accessing %s: %w", path, walkErr)
		}
		if entry.IsDir() || filepath.Ext(path) != ".yml" {
			return nil
		}

		rel, err := filepath.Rel(rolesPath, path)
		if err != nil {
			return fmt.Errorf("resolving task path %s: %w", path, err)
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) < 3 || parts[1] != "tasks" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading task file %s: %w", path, err)
		}
		count, err := countManagedDirectoryIncludes(content)
		if err != nil {
			return fmt.Errorf("parsing task file %s: %w", path, err)
		}
		if count > 0 {
			roles[parts[0]] = struct{}{}
			occurrences += count
		}
		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("walking managed-directory roles %s: %w", rolesPath, err)
	}
	return roles, occurrences, nil
}

func countManagedDirectoryIncludes(content []byte) (int, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	count := 0
	for {
		var document yaml.Node
		err := decoder.Decode(&document)
		if err != nil {
			if err == io.EOF {
				return count, nil
			}
			return 0, err
		}
		count += countManagedDirectoryIncludesInNode(&document)
	}
}

func countManagedDirectoryIncludesInNode(node *yaml.Node) int {
	count := 0
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if key.Kind == yaml.ScalarNode && key.Value == "ansible.builtin.include_tasks" &&
				value.Kind == yaml.ScalarNode && strings.TrimSpace(value.Value) == managedDirectoriesTask {
				count++
			}
		}
	}
	for _, child := range node.Content {
		count += countManagedDirectoryIncludesInNode(child)
	}
	return count
}
