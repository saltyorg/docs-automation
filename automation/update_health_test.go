package automation

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saltyorg/docs-automation/config"
)

func TestUpdateHealthReportIncludesAutomationFailuresWithoutManagingIssue(t *testing.T) {
	fixture := newUpdateHealthFixture(t)
	summaryPath := filepath.Join(fixture.root, "actions-summary.md")
	ghSentinel := filepath.Join(fixture.root, "gh-called")
	fakeBin := filepath.Join(fixture.root, "bin")
	writeUpdateHealthFixtureFile(t, filepath.Join(fakeBin, "gh"), []byte("#!/bin/sh\ntouch \"$GH_SENTINEL\"\n"), 0o755)

	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)
	t.Setenv("GH_SENTINEL", ghSentinel)
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr, false)
	runner.resolveRevision = func(_ context.Context, _ string) (string, error) {
		return "", errors.New("temporary fixture is not a repository")
	}

	err := runner.Update(t.Context(), fixture.cfg, "", UpdateOptions{RunCheck: true, ManageIssue: false})
	if err != nil {
		t.Fatalf("Update() error = %v, want nil for nonfatal all-role failures", err)
	}

	if _, err := os.Stat(ghSentinel); !os.IsNotExist(err) {
		t.Fatalf("gh command was invoked; sentinel stat error = %v", err)
	}

	if got := stderr.String(); !strings.Contains(got, "Error: failed to update broken") || !strings.Contains(got, "Warning: failed to update CLI help") {
		t.Fatalf("stderr = %q, want role error and CLI warning", got)
	}

	summary, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", summaryPath, err)
	}
	for _, want := range []string{
		"Documentation Health",
		"Role Automation Errors",
		"CLI Help Automation Errors",
		"| Role Automation Errors | Enabled | 1 | 0 |",
		"| CLI Help Automation Errors | Enabled | 1 | 0 |",
		"| saltbox | broken |  | role documentation automation failed for saltbox/broken |",
		"| docs | CLI help |  | CLI help documentation automation failed |",
	} {
		if !strings.Contains(string(summary), want) {
			t.Errorf("Actions summary missing %q:\n%s", want, summary)
		}
	}
}

func TestUpdatePropagatesHealthProvenanceCancellation(t *testing.T) {
	for _, cancellation := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(cancellation.Error(), func(t *testing.T) {
			fixture := newUpdateHealthFixture(t)
			var stderr bytes.Buffer
			runner := NewRunner(io.Discard, &stderr, false)
			runner.resolveRevision = func(_ context.Context, _ string) (string, error) {
				return "", cancellation
			}

			err := runner.Update(t.Context(), fixture.cfg, "", UpdateOptions{NoCLI: true, RunCheck: true})
			if !errors.Is(err, cancellation) {
				t.Fatalf("Update() error = %v, want %v", err, cancellation)
			}
			if strings.Contains(stderr.String(), "failed to build documentation health report") {
				t.Fatalf("Update() logged cancellation as a health warning: %q", stderr.String())
			}
		})
	}
}

type updateHealthFixture struct {
	root string
	cfg  *config.Config
}

func newUpdateHealthFixture(t *testing.T) updateHealthFixture {
	t.Helper()
	root := t.TempDir()
	saltboxRoot := filepath.Join(root, "saltbox")
	sandboxRoot := filepath.Join(root, "sandbox")
	docsRoot := filepath.Join(root, "docs")
	for _, path := range []string{
		filepath.Join(saltboxRoot, "roles", "broken", "defaults"),
		filepath.Join(saltboxRoot, "inventories", "group_vars"),
		filepath.Join(saltboxRoot, "resources", "tasks", "docker"),
		filepath.Join(sandboxRoot, "roles"),
		filepath.Join(docsRoot, "docs", "apps"),
		filepath.Join(docsRoot, "docs", "sandbox", "apps"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("creating fixture directory %q: %v", path, err)
		}
	}
	writeUpdateHealthFixtureFile(t, filepath.Join(saltboxRoot, "roles", "broken", "defaults", "main.yml"), []byte("broken_enabled: true\n"), 0o644)
	writeUpdateHealthFixtureFile(t, filepath.Join(saltboxRoot, "inventories", "group_vars", "all.yml"), []byte("{}\n"), 0o644)
	writeUpdateHealthFixtureFile(t, filepath.Join(docsRoot, "docs", "apps", "broken.md"), []byte("---\nsaltbox_automation: [\n---\n"), 0o644)

	return updateHealthFixture{
		root: root,
		cfg: &config.Config{
			Repositories:  config.RepositoryConfig{Saltbox: saltboxRoot, Sandbox: sandboxRoot, Docs: docsRoot},
			PathOverrides: map[string]map[string]string{},
			CLIHelp: config.CLIHelpConfig{
				BinaryPath: filepath.Join(root, "missing-cli"),
			},
			Markers: config.MarkersConfig{
				Variables: "SALTBOX MANAGED VARIABLES SECTION",
				CLI:       "SALTBOX MANAGED CLI SECTION",
				Overview:  "SALTBOX MANAGED OVERVIEW SECTION",
			},
		},
	}
}

func writeUpdateHealthFixtureFile(t *testing.T, path string, content []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating fixture parent %q: %v", path, err)
	}
	if err := os.WriteFile(path, content, mode); err != nil {
		t.Fatalf("writing fixture file %q: %v", path, err)
	}
}
