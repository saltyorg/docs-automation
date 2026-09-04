package parser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanManagedDirectoryRolesFindsOnlyExactTaskActions(t *testing.T) {
	rolesPath := filepath.Join(t.TempDir(), "roles")
	writeTaskFile(t, rolesPath, "capable", "tasks/main.yml", `
- name: create directories
  block:
    - name: primary include
      ansible.builtin.include_tasks: >-
        {{ resources_tasks_path }}/directories/create_directories.yml
      when: capable_enabled
  rescue:
    - ansible.builtin.include_tasks: "{{ resources_tasks_path }}/directories/create_directories.yml"
  always:
    - debug:
        msg: done
`)
	writeTaskFile(t, rolesPath, "capable", "tasks/nested/more.yml", `
- ansible.builtin.include_tasks: "  {{ resources_tasks_path }}/directories/create_directories.yml  "
`)
	writeTaskFile(t, rolesPath, "multi_document", "tasks/main.yml", `
---
- ansible.builtin.include_tasks: "{{ resources_tasks_path }}/directories/create_directories.yml"
---
- block:
    - ansible.builtin.include_tasks: "{{ resources_tasks_path }}/directories/create_directories.yml"
`)
	writeTaskFile(t, rolesPath, "short", "tasks/main.yml", `
- include_tasks: "{{ resources_tasks_path }}/directories/create_directories.yml"
`)
	writeTaskFile(t, rolesPath, "imported", "tasks/main.yml", `
- ansible.builtin.import_tasks: "{{ resources_tasks_path }}/directories/create_directories.yml"
`)
	writeTaskFile(t, rolesPath, "relative", "tasks/main.yml", `
- ansible.builtin.include_tasks: "../../resources/tasks/directories/create_directories.yml"
`)
	writeTaskFile(t, rolesPath, "indirect", "tasks/main.yml", `
- ansible.builtin.include_tasks: other_role.yml
- debug:
    msg: "{{ resources_tasks_path }}/directories/create_directories.yml"
# ansible.builtin.include_tasks: "{{ resources_tasks_path }}/directories/create_directories.yml"
`)
	writeTaskFile(t, rolesPath, "ignored_extension", "tasks/main.yaml", `
- ansible.builtin.include_tasks: "{{ resources_tasks_path }}/directories/create_directories.yml"
`)
	writeTaskFile(t, rolesPath, "ignored_extension", "tasks/notes.txt", "not: [valid")
	if err := os.MkdirAll(filepath.Join(rolesPath, "missing_tasks"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := ScanManagedDirectoryRoles(rolesPath)
	if err != nil {
		t.Fatalf("ScanManagedDirectoryRoles() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ScanManagedDirectoryRoles() = %v, want capable and multi_document", got)
	}
	for _, roleName := range []string{"capable", "multi_document"} {
		if _, exists := got[roleName]; !exists {
			t.Errorf("ScanManagedDirectoryRoles() = %v, want %s", got, roleName)
		}
	}

	_, occurrences, err := scanManagedDirectoryRoles(rolesPath)
	if err != nil {
		t.Fatalf("scanManagedDirectoryRoles() error = %v", err)
	}
	if occurrences != 5 {
		t.Fatalf("scanManagedDirectoryRoles() occurrences = %d, want 5", occurrences)
	}
}

func TestScanManagedDirectoryRolesIgnoresActionShapedTaskData(t *testing.T) {
	rolesPath := filepath.Join(t.TempDir(), "roles")
	writeTaskFile(t, rolesPath, "module_arguments", "tasks/main.yml", `
- name: debug nested data
  ansible.builtin.debug:
    msg:
      ansible.builtin.include_tasks: "{{ resources_tasks_path }}/directories/create_directories.yml"
`)
	writeTaskFile(t, rolesPath, "task_vars", "tasks/main.yml", `
- name: define action-shaped variable data
  vars:
    ansible.builtin.include_tasks: "{{ resources_tasks_path }}/directories/create_directories.yml"
  ansible.builtin.debug:
    msg: done
`)

	roles, occurrences, err := scanManagedDirectoryRoles(rolesPath)
	if err != nil {
		t.Fatalf("scanManagedDirectoryRoles() error = %v", err)
	}
	if len(roles) != 0 {
		t.Fatalf("scanManagedDirectoryRoles() = %v, want no capable roles", roles)
	}
	if occurrences != 0 {
		t.Fatalf("scanManagedDirectoryRoles() occurrences = %d, want 0", occurrences)
	}
}

func TestScanManagedDirectoryRolesReportsRootAndTaskFailures(t *testing.T) {
	t.Run("missing root", func(t *testing.T) {
		rolesPath := filepath.Join(t.TempDir(), "missing")
		_, err := ScanManagedDirectoryRoles(rolesPath)
		if err == nil || !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), rolesPath) {
			t.Fatalf("error = %v, want contextual %s fs.ErrNotExist", err, rolesPath)
		}
	})

	t.Run("unreadable task", func(t *testing.T) {
		rolesPath := filepath.Join(t.TempDir(), "roles")
		taskPath := filepath.Join(rolesPath, "broken", "tasks", "main.yml")
		if err := os.MkdirAll(filepath.Dir(taskPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(rolesPath, "missing.yml"), taskPath); err != nil {
			t.Fatal(err)
		}

		_, err := ScanManagedDirectoryRoles(rolesPath)
		if err == nil || !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), taskPath) {
			t.Fatalf("error = %v, want contextual %s fs.ErrNotExist", err, taskPath)
		}
	})

	t.Run("invalid task YAML", func(t *testing.T) {
		rolesPath := filepath.Join(t.TempDir(), "roles")
		taskPath := filepath.Join(rolesPath, "broken", "tasks", "main.yml")
		writeTaskFile(t, rolesPath, "broken", "tasks/main.yml", "- block: [\n")

		_, err := ScanManagedDirectoryRoles(rolesPath)
		if err == nil || !strings.Contains(err.Error(), taskPath) {
			t.Fatalf("error = %v, want contextual YAML failure for %s", err, taskPath)
		}
	})
}

func TestScanManagedDirectoryRolesCurrentCorpus(t *testing.T) {
	if os.Getenv("DOCS_AUTOMATION_CORPUS") != "1" {
		t.Skip("set DOCS_AUTOMATION_CORPUS=1 with the reviewed Saltbox and Sandbox checkouts")
	}

	repositories := []struct {
		name         string
		rolesPath    string
		negativeRole string
	}{
		{name: "saltbox", rolesPath: "/srv/git/saltbox/roles", negativeRole: "transfer"},
		{name: "sandbox", rolesPath: "/opt/sandbox/roles", negativeRole: "booklore"},
	}
	totalRoles := 0
	totalOccurrences := 0
	for _, repository := range repositories {
		roles, occurrences, err := scanManagedDirectoryRoles(repository.rolesPath)
		if err != nil {
			t.Fatalf("scanManagedDirectoryRoles(%s) error = %v", repository.name, err)
		}
		if _, exists := roles[repository.negativeRole]; exists {
			t.Errorf("%s role %q unexpectedly has managed-directory capability", repository.name, repository.negativeRole)
		}
		totalRoles += len(roles)
		totalOccurrences += occurrences
	}

	if totalRoles != 198 {
		t.Errorf("managed-directory roles = %d, want 198", totalRoles)
	}
	if totalOccurrences != 201 {
		t.Errorf("managed-directory include occurrences = %d, want 201", totalOccurrences)
	}
}

func writeTaskFile(t *testing.T, rolesPath, roleName, relativePath, content string) {
	t.Helper()
	path := filepath.Join(rolesPath, roleName, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
