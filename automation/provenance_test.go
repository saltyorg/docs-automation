package automation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/saltyorg/docs-automation/buildinfo"
	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/github"
	"github.com/saltyorg/docs-automation/health"
)

func TestHealthProvenanceUsesConfiguredSourcesAndBestEffortRevisions(t *testing.T) {
	fixture := newHealthFixture(t)
	fixture.cfg.Issue.SourceRepositories = map[string]config.SourceRepositoryConfig{
		"saltbox": {Slug: "saltyorg/Saltbox", Ref: "master"},
		"sandbox": {Slug: "saltyorg/Sandbox", Ref: "develop"},
	}
	t.Setenv("GITHUB_SERVER_URL", "https://github.example")
	t.Setenv("GITHUB_REPOSITORY", "saltyorg/docs")
	t.Setenv("GITHUB_RUN_ID", "1234")
	t.Setenv("GITHUB_HEAD_REF", "docs-health")

	resolved := make([]string, 0, 3)
	runner := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false)
	runner.resolveRevision = func(_ context.Context, path string) (string, error) {
		resolved = append(resolved, path)
		switch path {
		case fixture.cfg.Repositories.Saltbox:
			return "saltbox-revision", nil
		case fixture.cfg.Repositories.Sandbox:
			return "sandbox-revision", nil
		case fixture.cfg.Repositories.Docs:
			return "", errors.New("not a git checkout")
		default:
			return "", errors.New("unexpected path")
		}
	}

	report, err := runner.buildHealthReport(t.Context(), fixture.cfg, github.NewUpdateSummary(), false, nil)
	if err != nil {
		t.Fatalf("buildHealthReport() error = %v", err)
	}

	if report.Run.CheckedAt.IsZero() || report.Run.CheckedAt.Location() != time.UTC {
		t.Fatalf("CheckedAt = %v, want nonzero UTC time", report.Run.CheckedAt)
	}
	if got, want := report.Run.WorkflowURL, "https://github.example/saltyorg/docs/actions/runs/1234"; got != want {
		t.Fatalf("WorkflowURL = %q, want %q", got, want)
	}
	if got, want := report.Run.Branch, "docs-health"; got != want {
		t.Fatalf("Branch = %q, want %q", got, want)
	}
	if got, want := report.Run.Version, buildinfo.VersionString(); got != want {
		t.Fatalf("Version = %q, want %q", got, want)
	}
	wantSources := []health.SourceRevision{
		{Repository: "saltbox", Slug: "saltyorg/Saltbox", Ref: "master", Revision: "saltbox-revision"},
		{Repository: "sandbox", Slug: "saltyorg/Sandbox", Ref: "develop", Revision: "sandbox-revision"},
		{Repository: "docs", Slug: "saltyorg/docs", Ref: "docs-health"},
	}
	if !slices.Equal(report.Run.Sources, wantSources) {
		t.Fatalf("Sources = %#v, want %#v", report.Run.Sources, wantSources)
	}
	if !slices.Equal(resolved, []string{
		fixture.cfg.Repositories.Saltbox,
		fixture.cfg.Repositories.Sandbox,
		fixture.cfg.Repositories.Docs,
	}) {
		t.Fatalf("resolved paths = %v", resolved)
	}
	if report.Total() != 0 {
		t.Fatalf("best-effort revision error changed findings: %+v", report.Results)
	}
}

func TestBuildHealthReportStopsAndPropagatesRevisionCancellation(t *testing.T) {
	for _, cancellation := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(cancellation.Error(), func(t *testing.T) {
			fixture := newHealthFixture(t)
			fixture.cfg.Issue.SourceRepositories = map[string]config.SourceRepositoryConfig{
				"saltbox": {Slug: "saltyorg/Saltbox", Ref: "master"},
				"sandbox": {Slug: "saltyorg/Sandbox", Ref: "master"},
			}
			var resolved []string
			runner := NewRunner(new(bytes.Buffer), new(bytes.Buffer), false)
			runner.resolveRevision = func(_ context.Context, path string) (string, error) {
				resolved = append(resolved, path)
				return "", fmt.Errorf("resolving fixture revision: %w", cancellation)
			}

			_, err := runner.buildHealthReport(t.Context(), fixture.cfg, github.NewUpdateSummary(), false, nil)
			if !errors.Is(err, cancellation) {
				t.Fatalf("buildHealthReport() error = %v, want %v", err, cancellation)
			}
			if len(resolved) != 1 {
				t.Fatalf("revision resolutions = %v, want resolution to stop after cancellation", resolved)
			}
		})
	}
}

func TestGitRevisionReturnsTrimmedHeadAndRejectsMissingMetadata(t *testing.T) {
	repository := t.TempDir()
	testRunGit(t, repository, "init", "--quiet")
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("temporary revision fixture\n"), 0o644); err != nil {
		t.Fatalf("writing temporary Git fixture: %v", err)
	}
	testRunGit(t, repository, "add", "README.md")
	testRunGit(t, repository,
		"-c", "user.name=Docs Automation Tests",
		"-c", "user.email=docs-automation@example.invalid",
		"-c", "commit.gpgsign=false",
		"commit", "--quiet", "-m", "test: create revision fixture",
	)

	revision, err := gitRevision(t.Context(), repository)
	if err != nil {
		t.Fatalf("gitRevision() error = %v", err)
	}
	if revision == "" || revision != strings.TrimSpace(revision) || strings.ContainsAny(revision, " \t\r\n") {
		t.Fatalf("gitRevision() = %q, want nonempty trimmed revision", revision)
	}

	missingMetadata := t.TempDir()
	if _, err := gitRevision(t.Context(), missingMetadata); err == nil {
		t.Fatal("gitRevision() error = nil for directory without Git metadata")
	}
}

func testRunGit(t *testing.T, repository string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", repository, "-c", "core.hooksPath=/dev/null"}, args...)
	cmd := exec.CommandContext(t.Context(), "git", commandArgs...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "LC_ALL=C")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v error = %v:\n%s", args, err, output)
	}
}
