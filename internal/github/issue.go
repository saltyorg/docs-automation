package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// IssueManager handles GitHub issue creation and management.
type IssueManager struct {
	repo        string // Repository in format "owner/repo"
	workflowURL string // URL to the workflow run
}

// NewIssueManager creates a new GitHub issue manager.
func NewIssueManager(repo, workflowURL string) *IssueManager {
	return &IssueManager{
		repo:        repo,
		workflowURL: workflowURL,
	}
}

// CheckResult holds the results of coverage checks.
type CheckResult struct {
	MissingDocs            []string // Roles without documentation
	MissingSections        []string // Docs without managed variables sections
	MissingDetailsSections []string // Docs without managed details sections
	OrphanedDocs           []string // Docs without corresponding roles
}

// HasIssues returns true if there are any problems.
func (r *CheckResult) HasIssues() bool {
	return len(r.MissingDocs) > 0 || len(r.MissingSections) > 0 || len(r.MissingDetailsSections) > 0 || len(r.OrphanedDocs) > 0
}

// TotalIssues returns the total number of issues.
func (r *CheckResult) TotalIssues() int {
	return len(r.MissingDocs) + len(r.MissingSections) + len(r.MissingDetailsSections) + len(r.OrphanedDocs)
}

// GenerateIssueBody generates the markdown body for a GitHub issue.
func (m *IssueManager) GenerateIssueBody(result *CheckResult) string {
	var builder strings.Builder

	builder.WriteString("## 📝 Documentation Status\n\n")

	if len(result.MissingDocs) > 0 {
		builder.WriteString(fmt.Sprintf("### Missing Documentation (%d)\n", len(result.MissingDocs)))
		builder.WriteString("Roles without corresponding documentation pages:\n\n")
		for _, role := range result.MissingDocs {
			builder.WriteString(fmt.Sprintf("- [ ] `%s`\n", role))
		}
		builder.WriteString("\n")
	}

	if len(result.MissingSections) > 0 {
		builder.WriteString(fmt.Sprintf("### Missing Variables Sections (%d)\n", len(result.MissingSections)))
		builder.WriteString("Documentation pages without the managed variables section:\n\n")
		for _, doc := range result.MissingSections {
			// Convert path to GitHub link
			docName := extractDocName(doc)
			link := fmt.Sprintf("https://github.com/%s/blob/main/%s", m.repo, doc)
			builder.WriteString(fmt.Sprintf("- [ ] [%s](%s)\n", docName, link))
		}
		builder.WriteString("\n")
	}

	if len(result.MissingDetailsSections) > 0 {
		builder.WriteString(fmt.Sprintf("### Missing Details Sections (%d)\n", len(result.MissingDetailsSections)))
		builder.WriteString("Documentation pages without the managed details section:\n\n")
		for _, doc := range result.MissingDetailsSections {
			// Convert path to GitHub link
			docName := extractDocName(doc)
			link := fmt.Sprintf("https://github.com/%s/blob/main/%s", m.repo, doc)
			builder.WriteString(fmt.Sprintf("- [ ] [%s](%s)\n", docName, link))
		}
		builder.WriteString("\n")
	}

	if len(result.OrphanedDocs) > 0 {
		builder.WriteString(fmt.Sprintf("### Orphaned Documentation (%d)\n", len(result.OrphanedDocs)))
		builder.WriteString("Documentation pages without corresponding roles:\n\n")
		for _, doc := range result.OrphanedDocs {
			builder.WriteString(fmt.Sprintf("- [ ] `%s`\n", doc))
		}
		builder.WriteString("\n")
	}

	builder.WriteString("---\n")
	if m.workflowURL != "" {
		builder.WriteString(fmt.Sprintf("**Workflow run:** [link](%s)\n", m.workflowURL))
	}
	builder.WriteString("*This issue is automatically managed by docs-automation*\n")

	return builder.String()
}

// GenerateIssueTitle generates the issue title.
func (m *IssueManager) GenerateIssueTitle(result *CheckResult) string {
	return fmt.Sprintf("[Docs Automation] %d documentation issue(s) found", result.TotalIssues())
}

// OutputGitHubActions outputs GitHub Actions workflow commands.
// These can be used to set outputs for subsequent workflow steps.
func (m *IssueManager) OutputGitHubActions(result *CheckResult) {
	// Check if we're running in GitHub Actions
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		return
	}

	// Get the GITHUB_OUTPUT file path
	outputFile := os.Getenv("GITHUB_OUTPUT")
	if outputFile == "" {
		return
	}

	// Open the output file in append mode
	f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write to GITHUB_OUTPUT: %v\n", err)
		return
	}
	defer f.Close()

	// Write outputs
	fmt.Fprintf(f, "has_issues=%t\n", result.HasIssues())
	fmt.Fprintf(f, "total_issues=%d\n", result.TotalIssues())
	fmt.Fprintf(f, "missing_docs=%d\n", len(result.MissingDocs))
	fmt.Fprintf(f, "missing_sections=%d\n", len(result.MissingSections))
	fmt.Fprintf(f, "missing_details_sections=%d\n", len(result.MissingDetailsSections))
	fmt.Fprintf(f, "orphaned_docs=%d\n", len(result.OrphanedDocs))

	// For multiline output (issue body), use delimiter
	if result.HasIssues() {
		issueBody := m.GenerateIssueBody(result)
		fmt.Fprintf(f, "issue_title=%s\n", m.GenerateIssueTitle(result))
		fmt.Fprintf(f, "issue_body<<EOF\n%s\nEOF\n", issueBody)
	}
}

// extractDocName extracts a clean document name from a path.
func extractDocName(path string) string {
	// Remove directory prefix and .md suffix
	parts := strings.Split(path, "/")
	name := parts[len(parts)-1]
	return strings.TrimSuffix(name, ".md")
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

// ghIssue represents a GitHub issue from gh CLI JSON output.
type ghIssue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	NodeID string `json:"id"` // GraphQL node ID for pinning
}

// ManageIssue creates, updates, or closes a GitHub issue based on check results.
// It uses the gh CLI which must be installed and authenticated.
func (m *IssueManager) ManageIssue(result *CheckResult, label string) error {
	// Check if gh CLI is available
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("gh CLI not found: %w", err)
	}

	// Find existing issue with the label
	existingIssue, err := m.findExistingIssue(label)
	if err != nil {
		return fmt.Errorf("finding existing issue: %w", err)
	}

	if result.HasIssues() {
		// Create or update issue
		title := m.GenerateIssueTitle(result)
		body := m.GenerateIssueBody(result)

		if existingIssue != nil {
			// Update existing issue
			if err := m.updateIssue(existingIssue.Number, title, body); err != nil {
				return fmt.Errorf("updating issue: %w", err)
			}
			fmt.Printf("Updated issue #%d\n", existingIssue.Number)

			// Reopen if closed
			if existingIssue.State == "CLOSED" {
				if err := m.reopenIssue(existingIssue.Number); err != nil {
					return fmt.Errorf("reopening issue: %w", err)
				}
				fmt.Printf("Reopened issue #%d\n", existingIssue.Number)
			}

			// Pin if not already pinned
			if err := m.pinIssue(existingIssue.Number); err != nil {
				// Don't fail on pin errors - it might already be pinned or user lacks permission
				fmt.Printf("Note: could not pin issue: %v\n", err)
			}
		} else {
			// Create new issue
			issueNum, err := m.createIssue(title, body, label)
			if err != nil {
				return fmt.Errorf("creating issue: %w", err)
			}
			fmt.Printf("Created issue #%d\n", issueNum)

			// Pin the new issue
			if err := m.pinIssue(issueNum); err != nil {
				fmt.Printf("Note: could not pin issue: %v\n", err)
			}
		}
	} else {
		// No issues - close existing issue if present
		if existingIssue != nil && existingIssue.State != "CLOSED" {
			// Unpin first
			if err := m.unpinIssue(existingIssue.Number); err != nil {
				fmt.Printf("Note: could not unpin issue: %v\n", err)
			}

			// Add closing comment
			closeMsg := "✅ All documentation checks passed! Closing this issue."
			if err := m.addComment(existingIssue.Number, closeMsg); err != nil {
				fmt.Printf("Note: could not add closing comment: %v\n", err)
			}

			// Close the issue
			if err := m.closeIssue(existingIssue.Number); err != nil {
				return fmt.Errorf("closing issue: %w", err)
			}
			fmt.Printf("Closed issue #%d\n", existingIssue.Number)
		} else {
			fmt.Println("No issues found and no open tracking issue exists")
		}
	}

	return nil
}

// findExistingIssue finds an existing issue with the given label.
func (m *IssueManager) findExistingIssue(label string) (*ghIssue, error) {
	cmd := exec.Command("gh", "issue", "list",
		"--repo", m.repo,
		"--label", label,
		"--state", "all",
		"--limit", "1",
		"--json", "number,title,state,id")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: %w", stderr.String(), err)
	}

	var issues []ghIssue
	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		return nil, fmt.Errorf("parsing issue list: %w", err)
	}

	if len(issues) == 0 {
		return nil, nil
	}

	return &issues[0], nil
}

// createIssue creates a new GitHub issue and returns its number.
func (m *IssueManager) createIssue(title, body, label string) (int, error) {
	cmd := exec.Command("gh", "issue", "create",
		"--repo", m.repo,
		"--title", title,
		"--body", body,
		"--label", label)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("%s: %w", stderr.String(), err)
	}

	// Parse issue number from URL output (e.g., "https://github.com/owner/repo/issues/123")
	output := strings.TrimSpace(stdout.String())
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
func (m *IssueManager) updateIssue(number int, title, body string) error {
	cmd := exec.Command("gh", "issue", "edit",
		"--repo", m.repo,
		fmt.Sprintf("%d", number),
		"--title", title,
		"--body", body)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", stderr.String(), err)
	}

	return nil
}

// closeIssue closes a GitHub issue.
func (m *IssueManager) closeIssue(number int) error {
	cmd := exec.Command("gh", "issue", "close", "--repo", m.repo, fmt.Sprintf("%d", number))

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", stderr.String(), err)
	}

	return nil
}

// reopenIssue reopens a closed GitHub issue.
func (m *IssueManager) reopenIssue(number int) error {
	cmd := exec.Command("gh", "issue", "reopen", "--repo", m.repo, fmt.Sprintf("%d", number))

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", stderr.String(), err)
	}

	return nil
}

// addComment adds a comment to a GitHub issue.
func (m *IssueManager) addComment(number int, body string) error {
	cmd := exec.Command("gh", "issue", "comment",
		"--repo", m.repo,
		fmt.Sprintf("%d", number),
		"--body", body)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", stderr.String(), err)
	}

	return nil
}

// pinIssue pins an issue to the repository.
func (m *IssueManager) pinIssue(number int) error {
	cmd := exec.Command("gh", "issue", "pin", "--repo", m.repo, fmt.Sprintf("%d", number))

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", stderr.String(), err)
	}

	return nil
}

// unpinIssue unpins an issue from the repository.
func (m *IssueManager) unpinIssue(number int) error {
	cmd := exec.Command("gh", "issue", "unpin", "--repo", m.repo, fmt.Sprintf("%d", number))

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", stderr.String(), err)
	}

	return nil
}
