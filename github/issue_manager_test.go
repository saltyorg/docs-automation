package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/saltyorg/docs-automation/health"
)

type fakeCommandResult struct {
	stdout string
	stderr string
	err    error
}

type fakeCommandCall struct {
	name string
	args []string
}

type fakeCommandRunner struct {
	results       []fakeCommandResult
	calls         []fakeCommandCall
	afterLookPath func()
}

type lifecycleCommandRunner struct {
	issue    *ghIssue
	body     string
	comments []string
	calls    []fakeCommandCall
	failures map[string][]error
}

func (f *fakeCommandRunner) LookPath(string) (string, error) {
	if f.afterLookPath != nil {
		f.afterLookPath()
	}
	return "/usr/bin/gh", nil
}

func (f *fakeCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	f.calls = append(f.calls, fakeCommandCall{name: name, args: append([]string(nil), args...)})
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if len(f.results) == 0 {
		return nil, nil, errors.New("unexpected command")
	}
	result := f.results[0]
	f.results = f.results[1:]
	return []byte(result.stdout), []byte(result.stderr), result.err
}

func (f *lifecycleCommandRunner) LookPath(string) (string, error) {
	return "/usr/bin/gh", nil
}

func (f *lifecycleCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	f.calls = append(f.calls, fakeCommandCall{name: name, args: append([]string(nil), args...)})
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	operation := lifecycleOperation(args)
	if failures := f.failures[operation]; len(failures) > 0 {
		err := failures[0]
		f.failures[operation] = failures[1:]
		return nil, []byte("transient " + operation + " failure"), err
	}

	switch operation {
	case "issue list":
		issues := []ghIssue{}
		if f.issue != nil {
			issues = append(issues, *f.issue)
		}
		return lifecycleJSON(issues)
	case "issue create":
		f.issue = &ghIssue{
			Number: 7,
			Title:  testCommandArgument(args, "--title"),
			State:  "OPEN",
			NodeID: "node-id",
		}
		f.body = testCommandArgument(args, "--body")
		return []byte("https://github.com/owner/repo/issues/7\n"), nil, nil
	case "issue view body":
		return lifecycleJSON(issueBodyResponse{Body: f.body})
	case "issue view comments":
		comments := make([]ghComment, len(f.comments))
		for i, body := range f.comments {
			comments[i] = ghComment{Body: body}
		}
		return lifecycleJSON(issueCommentsResponse{Comments: comments})
	case "issue edit":
		f.issue.Title = testCommandArgument(args, "--title")
		f.body = testCommandArgument(args, "--body")
		return nil, nil, nil
	case "issue comment":
		f.comments = append(f.comments, testCommandArgument(args, "--body"))
		return nil, nil, nil
	case "issue close":
		f.issue.State = "CLOSED"
		return nil, nil, nil
	case "issue reopen":
		f.issue.State = "OPEN"
		return nil, nil, nil
	case "issue pin", "issue unpin":
		return nil, nil, nil
	default:
		return nil, nil, fmt.Errorf("unexpected command: gh %s", strings.Join(args, " "))
	}
}

func (f *lifecycleCommandRunner) failNext(operation string, err error) {
	if f.failures == nil {
		f.failures = make(map[string][]error)
	}
	f.failures[operation] = append(f.failures[operation], err)
}

func lifecycleOperation(args []string) string {
	if len(args) < 2 {
		return strings.Join(args, " ")
	}
	operation := strings.Join(args[:2], " ")
	if operation == "issue view" {
		operation += " " + testCommandArgument(args, "--json")
	}
	return operation
}

func lifecycleJSON(value any) ([]byte, []byte, error) {
	encoded, err := json.Marshal(value)
	return encoded, nil, err
}

type issueClosedWriter struct{}

func (issueClosedWriter) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func TestIssueManagerPassesCancellationToCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	runner := &fakeCommandRunner{afterLookPath: cancel}
	manager := newIssueManager("owner/repo", io.Discard, io.Discard, runner)

	err := manager.ManageIssue(ctx, health.NewReport(nil, health.RunInfo{}), "docs-automation")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ManageIssue() error = %v, want context.Canceled", err)
	}
}

func TestIssueManagerUsesInjectedOutput(t *testing.T) {
	runner := &fakeCommandRunner{results: []fakeCommandResult{{stdout: "[]"}}}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	manager := newIssueManager("owner/repo", &stdout, &stderr, runner)

	if err := manager.ManageIssue(t.Context(), health.NewReport(nil, health.RunInfo{}), "docs-automation"); err != nil {
		t.Fatalf("ManageIssue() error = %v", err)
	}
	if got, want := stdout.String(), "No issues found and no open tracking issue exists\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestIssueManagerReturnsCommandStderr(t *testing.T) {
	commandErr := errors.New("exit status 1")
	runner := &fakeCommandRunner{results: []fakeCommandResult{{
		stderr: "authentication failed\n",
		err:    commandErr,
	}}}
	manager := newIssueManager("owner/repo", io.Discard, io.Discard, runner)

	err := manager.ManageIssue(t.Context(), health.NewReport(nil, health.RunInfo{}), "docs-automation")
	if !errors.Is(err, commandErr) {
		t.Fatalf("ManageIssue() error = %v, want wrapped command error", err)
	}
	for _, want := range []string{"gh issue list", "authentication failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ManageIssue() error = %q, want substring %q", err, want)
		}
	}
}

func TestIssueManagerDoesNotExposeIssueBodyInCommandError(t *testing.T) {
	commandErr := errors.New("exit status 1")
	runner := &fakeCommandRunner{results: []fakeCommandResult{
		{stdout: "[]"},
		{stderr: "creation failed\n", err: commandErr},
	}}
	manager := newIssueManager("owner/repo", io.Discard, io.Discard, runner)
	report := testIssueManagerReport(health.RunInfo{}, health.Result{
		Kind:    health.MissingDocumentation,
		Enabled: true,
		Findings: []health.Finding{
			testIssueManagerFinding(health.MissingDocumentation, "sensitive-role-name"),
		},
	})

	err := manager.ManageIssue(t.Context(), report, "docs-automation")
	if err == nil {
		t.Fatal("ManageIssue() error = nil")
	}
	if strings.Contains(err.Error(), "sensitive-role-name") {
		t.Fatalf("ManageIssue() error exposed issue body: %q", err)
	}
	for _, want := range []string{"gh issue create", "creation failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ManageIssue() error = %q, want substring %q", err, want)
		}
	}
}

func TestIssueManagerReturnsOutputError(t *testing.T) {
	runner := &fakeCommandRunner{results: []fakeCommandResult{{stdout: "[]"}}}
	manager := newIssueManager("owner/repo", issueClosedWriter{}, io.Discard, runner)

	err := manager.ManageIssue(t.Context(), health.NewReport(nil, health.RunInfo{}), "docs-automation")
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("ManageIssue() error = %v, want io.ErrClosedPipe", err)
	}
}

func TestIssueManagerLegacyStateUpdatesSilently(t *testing.T) {
	runner := &fakeCommandRunner{results: []fakeCommandResult{
		{stdout: testExistingIssueJSON("legacy title", "OPEN")},
		{stdout: testIssueBodyJSON(t, "legacy issue body without state")},
		{}, // edit
		{}, // pin
	}}
	var stderr bytes.Buffer
	manager := newIssueManager("owner/repo", io.Discard, &stderr, runner)
	report := testIssueManagerReport(health.RunInfo{}, health.Result{
		Kind: health.MissingDocumentation, Enabled: true,
		Findings: []health.Finding{testIssueManagerFinding(health.MissingDocumentation, "radarr")},
	})

	if err := manager.ManageIssue(t.Context(), report, "docs-automation"); err != nil {
		t.Fatalf("ManageIssue() error = %v", err)
	}
	if !testRunnerCalled(runner, "issue", "edit") {
		t.Fatal("ManageIssue() did not replace the legacy issue body")
	}
	if testRunnerCalled(runner, "issue", "comment") {
		t.Fatal("ManageIssue() commented while silently migrating legacy state")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want silent migration", stderr.String())
	}
}

func TestIssueManagerWorkflowOnlyStateChangeUpdatesWithoutComment(t *testing.T) {
	oldRun := health.RunInfo{
		CheckedAt:   time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
		WorkflowURL: "https://github.com/saltyorg/docs/actions/runs/1",
	}
	newRun := health.RunInfo{
		CheckedAt:   time.Date(2026, 9, 1, 11, 0, 0, 0, time.UTC),
		WorkflowURL: "https://github.com/saltyorg/docs/actions/runs/2",
	}
	result := health.Result{
		Kind: health.MissingDocumentation, Enabled: true,
		Findings: []health.Finding{testIssueManagerFinding(health.MissingDocumentation, "radarr")},
	}
	oldReport := testIssueManagerReport(oldRun, result)
	newReport := testIssueManagerReport(newRun, result)
	oldBody := testRenderedIssueBody(t, oldReport)
	runner := &fakeCommandRunner{results: []fakeCommandResult{
		{stdout: testExistingIssueJSON(NewIssueRenderer("owner/repo").Title(newReport), "OPEN")},
		{stdout: testIssueBodyJSON(t, oldBody)},
		{}, // edit
		{}, // pin
	}}
	manager := newIssueManager("owner/repo", io.Discard, io.Discard, runner)

	if err := manager.ManageIssue(t.Context(), newReport, "docs-automation"); err != nil {
		t.Fatalf("ManageIssue() error = %v", err)
	}
	if !testRunnerCalled(runner, "issue", "edit") {
		t.Fatal("ManageIssue() did not refresh run presentation")
	}
	if testRunnerCalled(runner, "issue", "comment") {
		t.Fatal("ManageIssue() commented for a run-context-only change")
	}
}

func TestIssueManagerSemanticStateChangePostsOneComment(t *testing.T) {
	oldReport := testIssueManagerReport(health.RunInfo{}, health.Result{
		Kind: health.MissingDocumentation, Enabled: true,
		Findings: []health.Finding{testIssueManagerFinding(health.MissingDocumentation, "sonarr")},
	})
	newReport := testIssueManagerReport(health.RunInfo{
		CheckedAt:   time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
		WorkflowURL: "https://github.com/saltyorg/docs/actions/runs/3",
	}, health.Result{
		Kind: health.MissingDocumentation, Enabled: true,
		Findings: []health.Finding{testIssueManagerFinding(health.MissingDocumentation, "radarr")},
	})
	runner := &fakeCommandRunner{results: []fakeCommandResult{
		{stdout: testExistingIssueJSON(NewIssueRenderer("owner/repo").Title(newReport), "OPEN")},
		{stdout: testIssueBodyJSON(t, testRenderedIssueBody(t, oldReport))},
		{stdout: `{"comments":[]}`}, // duplicate check
		{},                          // comment
		{},                          // edit
		{},                          // pin
	}}
	manager := newIssueManager("owner/repo", io.Discard, io.Discard, runner)

	if err := manager.ManageIssue(t.Context(), newReport, "docs-automation"); err != nil {
		t.Fatalf("ManageIssue() error = %v", err)
	}
	comments := testRunnerCalls(runner, "issue", "comment")
	if len(comments) != 1 {
		t.Fatalf("gh issue comment calls = %d, want 1", len(comments))
	}
	body := testCommandArgument(comments[0].args, "--body")
	for _, want := range []string{"### Docs health changed", "radarr", "sonarr", "docs-automation-state-sha256:"} {
		if !strings.Contains(body, want) {
			t.Errorf("semantic comment missing %q:\n%s", want, body)
		}
	}
}

func TestIssueManagerRetriesSemanticPublicationBeforeAdvancingState(t *testing.T) {
	for _, tt := range []struct {
		name      string
		operation string
	}{
		{name: "comment lookup failure", operation: "issue view comments"},
		{name: "comment publication failure", operation: "issue comment"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			oldReport := testIssueManagerReport(health.RunInfo{}, health.Result{
				Kind: health.MissingDocumentation, Enabled: true,
				Findings: []health.Finding{testIssueManagerFinding(health.MissingDocumentation, "sonarr")},
			})
			newReport := testIssueManagerReport(health.RunInfo{}, health.Result{
				Kind: health.MissingDocumentation, Enabled: true,
				Findings: []health.Finding{testIssueManagerFinding(health.MissingDocumentation, "radarr")},
			})
			oldBody := testRenderedIssueBody(t, oldReport)
			newBody := testRenderedIssueBody(t, newReport)
			runner := &lifecycleCommandRunner{
				issue: &ghIssue{Number: 7, Title: NewIssueRenderer("owner/repo").Title(oldReport), State: "OPEN", NodeID: "node-id"},
				body:  oldBody,
			}
			runner.failNext(tt.operation, errors.New("temporary failure"))
			var stderr bytes.Buffer
			manager := newIssueManager("owner/repo", io.Discard, &stderr, runner)

			if err := manager.ManageIssue(t.Context(), newReport, "docs-automation"); err != nil {
				t.Fatalf("first ManageIssue() error = %v, want soft publication failure", err)
			}
			if runner.body != oldBody {
				t.Fatal("first ManageIssue() advanced the embedded issue state before semantic publication was confirmed")
			}
			if !strings.Contains(stderr.String(), "could not") {
				t.Fatalf("stderr = %q, want soft publication note", stderr.String())
			}

			if err := manager.ManageIssue(t.Context(), newReport, "docs-automation"); err != nil {
				t.Fatalf("retry ManageIssue() error = %v", err)
			}
			if runner.body != newBody {
				t.Fatal("retry ManageIssue() did not advance the issue body after semantic publication succeeded")
			}
			if got := len(testSemanticComments(runner.comments)); got != 1 {
				t.Fatalf("semantic comments = %d, want exactly 1 after retry", got)
			}
		})
	}
}

func TestIssueManagerRetriesBodyEditWithoutDuplicatingPublishedComment(t *testing.T) {
	oldReport := testIssueManagerReport(health.RunInfo{}, health.Result{
		Kind: health.MissingDocumentation, Enabled: true,
		Findings: []health.Finding{testIssueManagerFinding(health.MissingDocumentation, "sonarr")},
	})
	newReport := testIssueManagerReport(health.RunInfo{}, health.Result{
		Kind: health.MissingDocumentation, Enabled: true,
		Findings: []health.Finding{testIssueManagerFinding(health.MissingDocumentation, "radarr")},
	})
	oldBody := testRenderedIssueBody(t, oldReport)
	newBody := testRenderedIssueBody(t, newReport)
	runner := &lifecycleCommandRunner{
		issue: &ghIssue{Number: 7, Title: NewIssueRenderer("owner/repo").Title(oldReport), State: "OPEN", NodeID: "node-id"},
		body:  oldBody,
	}
	runner.failNext("issue edit", errors.New("temporary edit failure"))
	manager := newIssueManager("owner/repo", io.Discard, io.Discard, runner)

	if err := manager.ManageIssue(t.Context(), newReport, "docs-automation"); err == nil {
		t.Fatal("first ManageIssue() error = nil, want body edit failure")
	}
	if runner.body != oldBody {
		t.Fatal("failed issue edit changed the stored body")
	}
	if got := len(testSemanticComments(runner.comments)); got != 1 {
		t.Fatalf("semantic comments after failed edit = %d, want 1 published before the edit", got)
	}

	if err := manager.ManageIssue(t.Context(), newReport, "docs-automation"); err != nil {
		t.Fatalf("retry ManageIssue() error = %v", err)
	}
	if runner.body != newBody {
		t.Fatal("retry ManageIssue() did not advance the issue body")
	}
	if got := len(testSemanticComments(runner.comments)); got != 1 {
		t.Fatalf("semantic comments after retry = %d, want the state hash to suppress a duplicate", got)
	}
}

func TestIssueManagerDuplicateStateHashSuppressesComment(t *testing.T) {
	oldReport := testIssueManagerReport(health.RunInfo{}, health.Result{
		Kind: health.MissingDocumentation, Enabled: true,
		Findings: []health.Finding{testIssueManagerFinding(health.MissingDocumentation, "sonarr")},
	})
	newReport := testIssueManagerReport(health.RunInfo{}, health.Result{
		Kind: health.MissingDocumentation, Enabled: true,
		Findings: []health.Finding{testIssueManagerFinding(health.MissingDocumentation, "radarr")},
	})
	marker := "<!-- docs-automation-state-sha256:" + testIssueStateHash(t, newReport.State()) + " -->"
	runner := &fakeCommandRunner{results: []fakeCommandResult{
		{stdout: testExistingIssueJSON(NewIssueRenderer("owner/repo").Title(newReport), "OPEN")},
		{stdout: testIssueBodyJSON(t, testRenderedIssueBody(t, oldReport))},
		{stdout: testCommentsJSON(t, marker)},
		{}, // edit
		{}, // pin
	}}
	manager := newIssueManager("owner/repo", io.Discard, io.Discard, runner)

	if err := manager.ManageIssue(t.Context(), newReport, "docs-automation"); err != nil {
		t.Fatalf("ManageIssue() error = %v", err)
	}
	if testRunnerCalled(runner, "issue", "comment") {
		t.Fatal("ManageIssue() posted a duplicate semantic comment")
	}
}

func TestIssueManagerCorruptStateLogsNoteAndMigratesSilently(t *testing.T) {
	const secret = "DO-NOT-ECHO-CORRUPT-ISSUE-BODY"
	corruptBody := secret + "\n<!-- docs-automation-state-v1:not-valid-base64%%% -->"
	runner := &fakeCommandRunner{results: []fakeCommandResult{
		{stdout: testExistingIssueJSON("old title", "OPEN")},
		{stdout: testIssueBodyJSON(t, corruptBody)},
		{}, // edit
		{}, // pin
	}}
	var stderr bytes.Buffer
	manager := newIssueManager("owner/repo", io.Discard, &stderr, runner)
	report := testIssueManagerReport(health.RunInfo{}, health.Result{
		Kind: health.MissingDocumentation, Enabled: true,
		Findings: []health.Finding{testIssueManagerFinding(health.MissingDocumentation, "radarr")},
	})

	if err := manager.ManageIssue(t.Context(), report, "docs-automation"); err != nil {
		t.Fatalf("ManageIssue() error = %v", err)
	}
	if !strings.Contains(stderr.String(), "could not decode existing issue state") {
		t.Fatalf("stderr = %q, want corrupt-state migration note", stderr.String())
	}
	if strings.Contains(stderr.String(), secret) {
		t.Fatalf("stderr exposed raw issue body: %q", stderr.String())
	}
	if testRunnerCalled(runner, "issue", "comment") {
		t.Fatal("ManageIssue() commented while migrating corrupt state")
	}
}

func TestIssueManagerPreservesReopenPinCloseAndUnpinLifecycle(t *testing.T) {
	t.Run("reopens and pins findings", func(t *testing.T) {
		runner := &fakeCommandRunner{results: []fakeCommandResult{
			{stdout: testExistingIssueJSON("old title", "CLOSED")},
			{stdout: testIssueBodyJSON(t, "legacy body")},
			{}, // edit
			{}, // reopen
			{}, // pin
		}}
		manager := newIssueManager("owner/repo", io.Discard, io.Discard, runner)
		report := testIssueManagerReport(health.RunInfo{}, health.Result{
			Kind: health.EditorialAttention, Enabled: true,
			Findings: []health.Finding{testIssueManagerFinding(health.EditorialAttention, "draft")},
		})

		if err := manager.ManageIssue(t.Context(), report, "docs-automation"); err != nil {
			t.Fatalf("ManageIssue() error = %v", err)
		}
		for _, operation := range []string{"reopen", "pin"} {
			if !testRunnerCalled(runner, "issue", operation) {
				t.Errorf("ManageIssue() did not %s issue", operation)
			}
		}
	})

	t.Run("unpins and closes healthy report", func(t *testing.T) {
		runner := &fakeCommandRunner{results: []fakeCommandResult{
			{stdout: testExistingIssueJSON("old title", "OPEN")},
			{}, // edit to healthy state
			{}, // unpin
			{}, // closing comment
			{}, // close
		}}
		manager := newIssueManager("owner/repo", io.Discard, io.Discard, runner)

		if err := manager.ManageIssue(t.Context(), health.NewReport(nil, health.RunInfo{}), "docs-automation"); err != nil {
			t.Fatalf("ManageIssue() error = %v", err)
		}
		for _, operation := range []string{"unpin", "comment", "close"} {
			if !testRunnerCalled(runner, "issue", operation) {
				t.Errorf("ManageIssue() did not %s issue", operation)
			}
		}
	})
}

func TestIssueManagerFindingHealthyRecurrenceUsesHealthyState(t *testing.T) {
	report := testIssueManagerReport(health.RunInfo{}, health.Result{
		Kind: health.MissingDocumentation, Enabled: true,
		Findings: []health.Finding{testIssueManagerFinding(health.MissingDocumentation, "radarr")},
	})
	healthy := health.NewReport(nil, health.RunInfo{})
	runner := &lifecycleCommandRunner{}
	manager := newIssueManager("owner/repo", io.Discard, io.Discard, runner)

	if err := manager.ManageIssue(t.Context(), report, "docs-automation"); err != nil {
		t.Fatalf("finding ManageIssue() error = %v", err)
	}
	healthyCallStart := len(runner.calls)
	if err := manager.ManageIssue(t.Context(), healthy, "docs-automation"); err != nil {
		t.Fatalf("healthy ManageIssue() error = %v", err)
	}
	if runner.issue.Title != NewIssueRenderer("owner/repo").Title(healthy) {
		t.Fatalf("closed issue title = %q, want healthy title", runner.issue.Title)
	}
	state, found, err := decodeIssueState(runner.body)
	if err != nil || !found {
		t.Fatalf("closed issue body state found = %t, error = %v", found, err)
	}
	if got := issueStateFindingCount(state, health.MissingDocumentation); got != 0 {
		t.Fatalf("closed issue state has %d findings, want 0", got)
	}
	testAssertIssueOperationOrder(t, runner.calls[healthyCallStart:], "edit", "unpin", "comment", "close")

	if err := manager.ManageIssue(t.Context(), report, "docs-automation"); err != nil {
		t.Fatalf("recurrence ManageIssue() error = %v", err)
	}
	comments := testSemanticComments(runner.comments)
	if len(comments) != 1 {
		t.Fatalf("semantic comments = %d, want 1 recurrence comment", len(comments))
	}
	for _, want := range []string{"### Added (1)", "radarr"} {
		if !strings.Contains(comments[0], want) {
			t.Errorf("recurrence comment missing %q:\n%s", want, comments[0])
		}
	}
	if strings.Contains(comments[0], "### Resolved") {
		t.Fatalf("recurrence comment diffed against stale pre-close findings:\n%s", comments[0])
	}
}

func TestIssueManagerFindingLifecycleUsesEnabledFindings(t *testing.T) {
	t.Run("notice keeps issue open", func(t *testing.T) {
		runner := &fakeCommandRunner{results: []fakeCommandResult{
			{stdout: "[]"},
			{stdout: "https://github.com/owner/repo/issues/9"},
			{}, // pin
		}}
		manager := newIssueManager("owner/repo", io.Discard, io.Discard, runner)
		report := testIssueManagerReport(health.RunInfo{}, health.Result{
			Kind: health.EditorialAttention, Enabled: true,
			Findings: []health.Finding{testIssueManagerFinding(health.EditorialAttention, "draft")},
		})

		if err := manager.ManageIssue(t.Context(), report, "docs-automation"); err != nil {
			t.Fatalf("ManageIssue() error = %v", err)
		}
		if !testRunnerCalled(runner, "issue", "create") {
			t.Fatal("ManageIssue() did not create issue for notice finding")
		}
	})

	for _, tt := range []struct {
		name   string
		result health.Result
	}{
		{
			name: "disabled findings do not open issue",
			result: health.Result{Kind: health.MissingDocumentation, Enabled: false,
				Findings: []health.Finding{testIssueManagerFinding(health.MissingDocumentation, "disabled")}},
		},
		{
			name:   "exemptions alone do not open issue",
			result: health.Result{Kind: health.MissingDocumentation, Enabled: true, Exemptions: 4},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeCommandRunner{results: []fakeCommandResult{{stdout: "[]"}}}
			manager := newIssueManager("owner/repo", io.Discard, io.Discard, runner)
			if err := manager.ManageIssue(t.Context(), testIssueManagerReport(health.RunInfo{}, tt.result), "docs-automation"); err != nil {
				t.Fatalf("ManageIssue() error = %v", err)
			}
			if testRunnerCalled(runner, "issue", "create") {
				t.Fatal("ManageIssue() created issue without enabled findings")
			}
		})
	}
}

func TestOutputGitHubActionsWritesHealthOutputs(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "github-output")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_OUTPUT", outputPath)
	manager := NewIssueManager("owner/repo", io.Discard, io.Discard)
	report := testIssueManagerReport(health.RunInfo{},
		testIssueManagerResult(health.MissingDocumentation, 1),
		testIssueManagerResult(health.MissingVariablesSection, 2),
		testIssueManagerResult(health.MissingOverviewSection, 3),
		testIssueManagerResult(health.OrphanedDocumentation, 4),
		testIssueManagerResult(health.InvalidFrontmatter, 1),
		testIssueManagerResult(health.EditorialAttention, 1),
		testIssueManagerResult(health.RoleAutomationError, 1),
		testIssueManagerResult(health.CLIHelpAutomationError, 1),
	)

	if err := manager.OutputGitHubActions(report); err != nil {
		t.Fatalf("OutputGitHubActions() error = %v", err)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"has_issues=true\n",
		"total_issues=14\n",
		"missing_docs=1\n",
		"missing_sections=2\n",
		"missing_overview_sections=3\n",
		"orphaned_docs=4\n",
		"invalid_frontmatter=1\n",
		"editorial_attention=1\n",
		"role_automation_errors=1\n",
		"cli_automation_errors=1\n",
		"error_findings=13\n",
		"notice_findings=1\n",
		"total_findings=14\n",
		"issue_title=[Docs Health] 13 errors, 1 notice\n",
		"issue_body<<EOF\n",
	} {
		if !bytes.Contains(content, []byte(want)) {
			t.Fatalf("GITHUB_OUTPUT = %q, want substring %q", content, want)
		}
	}
}

func testIssueManagerReport(run health.RunInfo, results ...health.Result) health.Report {
	return health.NewReport(results, run)
}

func testIssueManagerFinding(kind health.Kind, subject string) health.Finding {
	return health.Finding{
		Kind: kind, Repository: "saltbox", Subject: subject, Code: "test_finding",
	}
}

func testIssueManagerResult(kind health.Kind, count int) health.Result {
	findings := make([]health.Finding, count)
	for i := range count {
		findings[i] = testIssueManagerFinding(kind, string(kind)+"-"+twoDigits(i))
	}
	return health.Result{Kind: kind, Enabled: true, Findings: findings}
}

func testRenderedIssueBody(t *testing.T, report health.Report) string {
	t.Helper()
	body, err := NewIssueRenderer("owner/repo").Body(report)
	if err != nil {
		t.Fatalf("Body() fixture error = %v", err)
	}
	return body
}

func testExistingIssueJSON(title, state string) string {
	encoded, _ := json.Marshal([]ghIssue{{Number: 7, Title: title, State: state, NodeID: "node-id"}})
	return string(encoded)
}

func testIssueBodyJSON(t *testing.T, body string) string {
	t.Helper()
	encoded, err := json.Marshal(issueBodyResponse{Body: body})
	if err != nil {
		t.Fatalf("marshalling issue body fixture: %v", err)
	}
	return string(encoded)
}

func testCommentsJSON(t *testing.T, bodies ...string) string {
	t.Helper()
	comments := make([]ghComment, len(bodies))
	for i, body := range bodies {
		comments[i] = ghComment{Body: body}
	}
	encoded, err := json.Marshal(issueCommentsResponse{Comments: comments})
	if err != nil {
		t.Fatalf("marshalling comments fixture: %v", err)
	}
	return string(encoded)
}

func testRunnerCalled(runner *fakeCommandRunner, first, second string) bool {
	return len(testRunnerCalls(runner, first, second)) > 0
}

func testRunnerCalls(runner *fakeCommandRunner, first, second string) []fakeCommandCall {
	var matches []fakeCommandCall
	for _, call := range runner.calls {
		if call.name == "gh" && len(call.args) >= 2 && call.args[0] == first && call.args[1] == second {
			matches = append(matches, call)
		}
	}
	return matches
}

func testCommandArgument(args []string, name string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name {
			return args[index+1]
		}
	}
	return ""
}

func testSemanticComments(comments []string) []string {
	var semantic []string
	for _, comment := range comments {
		if strings.Contains(comment, "docs-automation-state-sha256:") {
			semantic = append(semantic, comment)
		}
	}
	return semantic
}

func testAssertIssueOperationOrder(t *testing.T, calls []fakeCommandCall, operations ...string) {
	t.Helper()
	next := 0
	for _, call := range calls {
		if call.name != "gh" || len(call.args) < 2 || call.args[0] != "issue" {
			continue
		}
		if next < len(operations) && call.args[1] == operations[next] {
			next++
		}
	}
	if next != len(operations) {
		t.Fatalf("issue operation order did not contain %v in sequence; calls = %+v", operations, calls)
	}
}
