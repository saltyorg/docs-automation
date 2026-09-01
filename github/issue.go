package github

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/saltyorg/docs-automation/health"
)

type commandRunner interface {
	LookPath(file string) (string, error)
	Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error)
}

type execCommandRunner struct{}

func (execCommandRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (execCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// IssueManager handles GitHub issue creation and management.
type IssueManager struct {
	repo     string // Repository in format "owner/repo"
	renderer *IssueRenderer
	out      io.Writer
	errOut   io.Writer
	commands commandRunner
}

// NewIssueManager creates a new GitHub issue manager.
func NewIssueManager(repo string, out, errOut io.Writer) *IssueManager {
	return newIssueManager(repo, out, errOut, execCommandRunner{})
}

func newIssueManager(repo string, out, errOut io.Writer, commands commandRunner) *IssueManager {
	return &IssueManager{
		repo:     repo,
		renderer: NewIssueRenderer(repo),
		out:      out,
		errOut:   errOut,
		commands: commands,
	}
}

// OutputGitHubActions outputs GitHub Actions workflow commands.
// These can be used to set outputs for subsequent workflow steps.
func (m *IssueManager) OutputGitHubActions(report health.Report) error {
	// Check if we're running in GitHub Actions
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		return nil
	}

	// Get the GITHUB_OUTPUT file path
	outputFile := os.Getenv("GITHUB_OUTPUT")
	if outputFile == "" {
		return nil
	}

	var output strings.Builder

	// Write outputs
	fmt.Fprintf(&output, "has_issues=%t\n", report.HasFindings())
	fmt.Fprintf(&output, "total_issues=%d\n", report.Total())
	fmt.Fprintf(&output, "missing_docs=%d\n", reportFindingCount(report, health.MissingDocumentation))
	fmt.Fprintf(&output, "missing_sections=%d\n", reportFindingCount(report, health.MissingVariablesSection))
	fmt.Fprintf(&output, "missing_overview_sections=%d\n", reportFindingCount(report, health.MissingOverviewSection))
	fmt.Fprintf(&output, "orphaned_docs=%d\n", reportFindingCount(report, health.OrphanedDocumentation))
	fmt.Fprintf(&output, "invalid_frontmatter=%d\n", reportFindingCount(report, health.InvalidFrontmatter))
	fmt.Fprintf(&output, "editorial_attention=%d\n", reportFindingCount(report, health.EditorialAttention))
	fmt.Fprintf(&output, "role_automation_errors=%d\n", reportFindingCount(report, health.RoleAutomationError))
	fmt.Fprintf(&output, "cli_automation_errors=%d\n", reportFindingCount(report, health.CLIHelpAutomationError))
	fmt.Fprintf(&output, "error_findings=%d\n", report.TotalSeverity(health.Error))
	fmt.Fprintf(&output, "notice_findings=%d\n", report.TotalSeverity(health.Notice))
	fmt.Fprintf(&output, "total_findings=%d\n", report.Total())

	// For multiline output (issue body), use delimiter
	if report.HasFindings() {
		issueBody, err := m.renderer.Body(report)
		if err != nil {
			return err
		}
		fmt.Fprintf(&output, "issue_title=%s\n", m.renderer.Title(report))
		fmt.Fprintf(&output, "issue_body<<EOF\n%s\nEOF\n", issueBody)
	}

	f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("opening GITHUB_OUTPUT: %w", err)
	}
	_, writeErr := f.WriteString(output.String())
	closeErr := f.Close()
	return errors.Join(writeErr, closeErr)
}

func reportFindingCount(report health.Report, kind health.Kind) int {
	result, ok := report.Result(kind)
	if !ok || !result.Enabled {
		return 0
	}
	return len(result.Findings)
}

// GetWorkflowURL attempts to construct the workflow URL from environment variables.
func GetWorkflowURL() string {
	serverURL := os.Getenv("GITHUB_SERVER_URL")
	repo := os.Getenv("GITHUB_REPOSITORY")
	runID := os.Getenv("GITHUB_RUN_ID")

	if serverURL == "" || repo == "" || runID == "" {
		return ""
	}

	return fmt.Sprintf("%s/%s/actions/runs/%s", serverURL, repo, runID)
}

// GetRepository returns the repository from environment variables.
func GetRepository() string {
	return os.Getenv("GITHUB_REPOSITORY")
}

// GetBranch returns the current branch name from environment variables.
// For pull requests, it uses GITHUB_HEAD_REF; otherwise GITHUB_REF_NAME.
// Falls back to "main" if not running in GitHub Actions.
func GetBranch() string {
	// For pull requests, GITHUB_HEAD_REF contains the source branch
	if headRef := os.Getenv("GITHUB_HEAD_REF"); headRef != "" {
		return headRef
	}
	// For push events, GITHUB_REF_NAME contains the branch name
	if refName := os.Getenv("GITHUB_REF_NAME"); refName != "" {
		return refName
	}
	return "main"
}

// ghIssue represents a GitHub issue from gh CLI JSON output.
type ghIssue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	NodeID string `json:"id"` // GraphQL node ID for pinning
}

type issueBodyResponse struct {
	Body string `json:"body"`
}

type issueCommentsResponse struct {
	Comments []ghComment `json:"comments"`
}

type ghComment struct {
	Body string `json:"body"`
}

// ManageIssue creates, updates, or closes a GitHub issue based on a health report.
// It uses the gh CLI which must be installed and authenticated.
func (m *IssueManager) ManageIssue(ctx context.Context, report health.Report, label string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	title := m.renderer.Title(report)
	body, err := m.renderer.Body(report)
	if err != nil {
		return fmt.Errorf("rendering issue body: %w", err)
	}
	newState := issueStateForReport(report.Canonical())

	// Check if gh CLI is available
	if _, err := m.commands.LookPath("gh"); err != nil {
		return fmt.Errorf("gh CLI not found: %w", err)
	}

	// Find existing issue with the label
	existingIssue, err := m.findExistingIssue(ctx, label)
	if err != nil {
		return fmt.Errorf("finding existing issue: %w", err)
	}

	if report.HasFindings() {
		// Create or update issue
		if existingIssue != nil {
			previousBody := ""
			bodyLoaded := false
			previousState := health.State{}
			stateValid := false
			current, err := m.getIssueBody(ctx, existingIssue.Number)
			if err != nil {
				if writeErr := m.errorf("Note: could not load existing issue body for state migration: %v\n", err); writeErr != nil {
					return writeErr
				}
			} else {
				previousBody = current
				bodyLoaded = true
				decoded, found, decodeErr := decodeIssueState(previousBody)
				if decodeErr != nil {
					if writeErr := m.errorf("Note: could not decode existing issue state; migrating without comment: %v\n", decodeErr); writeErr != nil {
						return writeErr
					}
				} else if found {
					previousState = decoded
					stateValid = true
				}
			}

			changes := health.Diff(previousState, newState)
			stateAdvanceConfirmed := true
			if stateValid && hasSemanticChanges(changes) {
				stateAdvanceConfirmed = false
				stateHash := hashIssueState(newState)
				alreadyPosted, err := m.hasCommentWithStateHash(ctx, existingIssue.Number, stateHash)
				if err != nil {
					if writeErr := m.errorf("Note: could not check existing update comments: %v\n", err); writeErr != nil {
						return writeErr
					}
				} else if alreadyPosted {
					if err := m.printf("Issue #%d update comment already posted for this state hash\n", existingIssue.Number); err != nil {
						return err
					}
					stateAdvanceConfirmed = true
				} else {
					comment := m.renderer.UpdateComment(previousState, newState, report.Run)
					if err := m.addComment(ctx, existingIssue.Number, comment); err != nil {
						if writeErr := m.errorf("Note: could not add issue update comment: %v\n", err); writeErr != nil {
							return writeErr
						}
					} else {
						stateAdvanceConfirmed = true
					}
				}
			}

			titleChanged := existingIssue.Title != title
			bodyChanged := bodyLoaded && previousBody != body
			if stateAdvanceConfirmed {
				if titleChanged || bodyChanged || !bodyLoaded {
					if err := m.updateIssue(ctx, existingIssue.Number, title, body); err != nil {
						return fmt.Errorf("updating issue: %w", err)
					}
					if err := m.printf("Updated issue #%d\n", existingIssue.Number); err != nil {
						return err
					}
				} else {
					if err := m.printf("Issue #%d already up to date\n", existingIssue.Number); err != nil {
						return err
					}
				}
			}

			// Reopen if closed
			if existingIssue.State == "CLOSED" {
				if err := m.reopenIssue(ctx, existingIssue.Number); err != nil {
					return fmt.Errorf("reopening issue: %w", err)
				}
				if err := m.printf("Reopened issue #%d\n", existingIssue.Number); err != nil {
					return err
				}
			}

			// Pin if not already pinned
			if err := m.pinIssue(ctx, existingIssue.Number); err != nil {
				// Don't fail on pin errors - it might already be pinned or user lacks permission
				if writeErr := m.errorf("Note: could not pin issue: %v\n", err); writeErr != nil {
					return writeErr
				}
			}
		} else {
			// Create new issue
			issueNum, err := m.createIssue(ctx, title, body, label)
			if err != nil {
				return fmt.Errorf("creating issue: %w", err)
			}
			if err := m.printf("Created issue #%d\n", issueNum); err != nil {
				return err
			}

			// Pin the new issue
			if err := m.pinIssue(ctx, issueNum); err != nil {
				if writeErr := m.errorf("Note: could not pin issue: %v\n", err); writeErr != nil {
					return writeErr
				}
			}
		}
	} else {
		// No issues - close existing issue if present
		if existingIssue != nil && existingIssue.State != "CLOSED" {
			if err := m.updateIssue(ctx, existingIssue.Number, title, body); err != nil {
				return fmt.Errorf("updating healthy issue: %w", err)
			}

			// Unpin first
			if err := m.unpinIssue(ctx, existingIssue.Number); err != nil {
				if writeErr := m.errorf("Note: could not unpin issue: %v\n", err); writeErr != nil {
					return writeErr
				}
			}

			// Add closing comment
			closeMsg := "✅ All documentation checks passed! Closing this issue."
			if err := m.addComment(ctx, existingIssue.Number, closeMsg); err != nil {
				if writeErr := m.errorf("Note: could not add closing comment: %v\n", err); writeErr != nil {
					return writeErr
				}
			}

			// Close the issue
			if err := m.closeIssue(ctx, existingIssue.Number); err != nil {
				return fmt.Errorf("closing issue: %w", err)
			}
			if err := m.printf("Closed issue #%d\n", existingIssue.Number); err != nil {
				return err
			}
		} else {
			if err := m.printf("No issues found and no open tracking issue exists\n"); err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *IssueManager) printf(format string, args ...any) error {
	_, err := fmt.Fprintf(m.out, format, args...)
	return err
}

func (m *IssueManager) errorf(format string, args ...any) error {
	_, err := fmt.Fprintf(m.errOut, format, args...)
	return err
}

func (m *IssueManager) runGH(ctx context.Context, args ...string) ([]byte, error) {
	stdout, stderr, err := m.commands.Run(ctx, "gh", args...)
	if err == nil {
		return stdout, nil
	}
	operationArgs := args
	if len(operationArgs) > 2 {
		operationArgs = operationArgs[:2]
	}
	operation := strings.TrimSpace("gh " + strings.Join(operationArgs, " "))
	detail := strings.TrimSpace(string(stderr))
	if detail != "" {
		return nil, fmt.Errorf("%s: %s: %w", operation, detail, err)
	}
	return nil, fmt.Errorf("%s: %w", operation, err)
}

// findExistingIssue finds an existing issue with the given label.
func (m *IssueManager) findExistingIssue(ctx context.Context, label string) (*ghIssue, error) {
	stdout, err := m.runGH(ctx, "issue", "list",
		"--repo", m.repo,
		"--label", label,
		"--state", "all",
		"--limit", "1",
		"--json", "number,title,state,id")
	if err != nil {
		return nil, err
	}

	var issues []ghIssue
	if err := json.Unmarshal(stdout, &issues); err != nil {
		return nil, fmt.Errorf("parsing issue list: %w", err)
	}
	if len(issues) == 0 {
		return nil, nil
	}
	return &issues[0], nil
}

func (m *IssueManager) getIssueBody(ctx context.Context, number int) (string, error) {
	stdout, err := m.runGH(ctx, "issue", "view",
		"--repo", m.repo,
		fmt.Sprintf("%d", number),
		"--json", "body")
	if err != nil {
		return "", err
	}

	var issue issueBodyResponse
	if err := json.Unmarshal(stdout, &issue); err != nil {
		return "", fmt.Errorf("parsing issue body: %w", err)
	}
	return issue.Body, nil
}

// createIssue creates a new GitHub issue and returns its number.
func (m *IssueManager) createIssue(ctx context.Context, title, body, label string) (int, error) {
	stdout, err := m.runGH(ctx, "issue", "create",
		"--repo", m.repo,
		"--title", title,
		"--body", body,
		"--label", label)
	if err != nil {
		return 0, err
	}

	output := strings.TrimSpace(string(stdout))
	parts := strings.Split(output, "/")
	if len(parts) > 0 {
		var num int
		if _, err := fmt.Sscanf(parts[len(parts)-1], "%d", &num); err == nil {
			return num, nil
		}
	}
	return 0, fmt.Errorf("could not parse issue number from: %s", output)
}

// updateIssue updates an existing GitHub issue.
func (m *IssueManager) updateIssue(ctx context.Context, number int, title, body string) error {
	_, err := m.runGH(ctx, "issue", "edit",
		"--repo", m.repo,
		fmt.Sprintf("%d", number),
		"--title", title,
		"--body", body)
	return err
}

// closeIssue closes a GitHub issue.
func (m *IssueManager) closeIssue(ctx context.Context, number int) error {
	_, err := m.runGH(ctx, "issue", "close", "--repo", m.repo, fmt.Sprintf("%d", number))
	return err
}

// reopenIssue reopens a closed GitHub issue.
func (m *IssueManager) reopenIssue(ctx context.Context, number int) error {
	_, err := m.runGH(ctx, "issue", "reopen", "--repo", m.repo, fmt.Sprintf("%d", number))
	return err
}

// addComment adds a comment to a GitHub issue.
func (m *IssueManager) addComment(ctx context.Context, number int, body string) error {
	_, err := m.runGH(ctx, "issue", "comment",
		"--repo", m.repo,
		fmt.Sprintf("%d", number),
		"--body", body)
	return err
}

func (m *IssueManager) hasCommentWithStateHash(ctx context.Context, number int, stateHash string) (bool, error) {
	stdout, err := m.runGH(ctx, "issue", "view",
		"--repo", m.repo,
		fmt.Sprintf("%d", number),
		"--json", "comments")
	if err != nil {
		return false, err
	}

	var response issueCommentsResponse
	if err := json.Unmarshal(stdout, &response); err != nil {
		return false, fmt.Errorf("parsing issue comments: %w", err)
	}

	marker := issueStateHashMarker(stateHash)
	for _, comment := range response.Comments {
		if strings.Contains(comment.Body, marker) {
			return true, nil
		}
	}
	return false, nil
}

// pinIssue pins an issue to the repository.
func (m *IssueManager) pinIssue(ctx context.Context, number int) error {
	_, err := m.runGH(ctx, "issue", "pin", "--repo", m.repo, fmt.Sprintf("%d", number))
	return err
}

// unpinIssue unpins an issue from the repository.
func (m *IssueManager) unpinIssue(ctx context.Context, number int) error {
	_, err := m.runGH(ctx, "issue", "unpin", "--repo", m.repo, fmt.Sprintf("%d", number))
	return err
}

const issueCommentFindingLimit = 25

// UpdateComment renders a bounded summary of semantic issue-state changes.
func (r *IssueRenderer) UpdateComment(oldState, newState health.State, run health.RunInfo) string {
	changes := health.Diff(oldState, newState)

	var comment strings.Builder
	comment.WriteString("### Docs health changed\n\n")
	comment.WriteString("| Check | Before | After | Delta |\n")
	comment.WriteString("|---|---:|---:|---:|\n")
	for _, kind := range changes.ChangedResults {
		before := issueStateFindingCount(oldState, kind)
		after := issueStateFindingCount(newState, kind)
		fmt.Fprintf(&comment, "| %s | %d | %d | %s |\n",
			markdownIssueText(issueResultLabel(kind)), before, after, formatIssueDelta(after-before))
	}
	comment.WriteString("\n")

	writeIssueFindingChanges(&comment, "Added", "added", changes.Added)
	writeIssueFindingChanges(&comment, "Resolved", "resolved", changes.Resolved)

	if run.WorkflowURL != "" {
		fmt.Fprintf(&comment, "- Workflow: [View run](%s)\n", escapeIssueLinkURL(run.WorkflowURL))
	} else {
		comment.WriteString("- Workflow: unavailable\n")
	}
	checkedAt := "unknown"
	if !run.CheckedAt.IsZero() {
		checkedAt = run.CheckedAt.UTC().Format("2006-01-02 15:04:05 UTC")
	}
	fmt.Fprintf(&comment, "- Checked at: %s\n\n", checkedAt)
	comment.WriteString(issueStateHashMarker(hashIssueState(newState)))
	comment.WriteString("\n")
	return comment.String()
}

func writeIssueFindingChanges(comment *strings.Builder, heading, adjective string, findings []health.StateFinding) {
	if len(findings) == 0 {
		return
	}
	fmt.Fprintf(comment, "### %s (%d)\n\n", heading, len(findings))
	for _, finding := range findings[:min(len(findings), issueCommentFindingLimit)] {
		fmt.Fprintf(comment, "- %s: %s\n",
			markdownIssueText(issueResultLabel(finding.Kind)), markdownIssueText(finding.Label))
	}
	if omitted := len(findings) - issueCommentFindingLimit; omitted > 0 {
		fmt.Fprintf(comment, "\n%d additional %s findings omitted.\n", omitted, adjective)
	}
	comment.WriteString("\n")
}

func issueStateFindingCount(state health.State, kind health.Kind) int {
	for _, result := range state.Results {
		if result.Kind == kind {
			return len(result.Findings)
		}
	}
	return 0
}

func hasSemanticChanges(changes health.Changes) bool {
	return len(changes.ChangedResults) > 0 || len(changes.Added) > 0 || len(changes.Resolved) > 0
}

func formatIssueDelta(delta int) string {
	if delta > 0 {
		return fmt.Sprintf("+%d", delta)
	}
	return fmt.Sprintf("%d", delta)
}

func hashIssueState(state health.State) string {
	data, _ := json.Marshal(state)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func issueStateHashMarker(hash string) string {
	return fmt.Sprintf("<!-- docs-automation-state-sha256:%s -->", hash)
}
