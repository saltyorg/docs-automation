package github

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// IssueManager handles GitHub issue creation and management.
type IssueManager struct {
	repo        string // Repository in format "owner/repo"
	workflowURL string // URL to the workflow run
	branch      string // Branch name for links
}

// NewIssueManager creates a new GitHub issue manager.
func NewIssueManager(repo, workflowURL string) *IssueManager {
	return &IssueManager{
		repo:        repo,
		workflowURL: workflowURL,
		branch:      GetBranch(),
	}
}

// CheckResult holds the results of coverage checks.
type CheckResult struct {
	MissingDocs             []string // Roles without documentation
	MissingSections         []string // Docs without managed variables sections
	MissingOverviewSections []string // Docs without managed overview sections
	OrphanedDocs            []string // Docs without corresponding roles
}

// HasIssues returns true if there are any problems.
func (r *CheckResult) HasIssues() bool {
	return len(r.MissingDocs) > 0 || len(r.MissingSections) > 0 || len(r.MissingOverviewSections) > 0 || len(r.OrphanedDocs) > 0
}

// TotalIssues returns the total number of issues.
func (r *CheckResult) TotalIssues() int {
	return len(r.MissingDocs) + len(r.MissingSections) + len(r.MissingOverviewSections) + len(r.OrphanedDocs)
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
			link := fmt.Sprintf("https://github.com/%s/blob/%s/%s", m.repo, m.branch, doc)
			builder.WriteString(fmt.Sprintf("- [ ] [%s](%s)\n", docName, link))
		}
		builder.WriteString("\n")
	}

	if len(result.MissingOverviewSections) > 0 {
		builder.WriteString(fmt.Sprintf("### Missing Overview Sections (%d)\n", len(result.MissingOverviewSections)))
		builder.WriteString("Documentation pages without the managed overview section:\n\n")
		for _, doc := range result.MissingOverviewSections {
			// Convert path to GitHub link
			docName := extractDocName(doc)
			link := fmt.Sprintf("https://github.com/%s/blob/%s/%s", m.repo, m.branch, doc)
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
	count := result.TotalIssues()
	if count == 1 {
		return "[Docs Automation] 1 documentation issue found"
	}
	return fmt.Sprintf("[Docs Automation] %d documentation issues found", count)
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
	fmt.Fprintf(f, "missing_overview_sections=%d\n", len(result.MissingOverviewSections))
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

type issueCounts struct {
	MissingDocs             int
	MissingSections         int
	MissingOverviewSections int
	OrphanedDocs            int
}

var (
	missingDocsCountRegex             = regexp.MustCompile(`(?m)^### Missing Documentation \((\d+)\)$`)
	missingSectionsCountRegex         = regexp.MustCompile(`(?m)^### Missing Variables Sections \((\d+)\)$`)
	missingOverviewSectionsCountRegex = regexp.MustCompile(`(?m)^### Missing Overview Sections \((\d+)\)$`)
	orphanedDocsCountRegex            = regexp.MustCompile(`(?m)^### Orphaned Documentation \((\d+)\)$`)
)

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
			previousBody := ""
			bodyLoaded := false
			current, err := m.getIssueBody(existingIssue.Number)
			if err != nil {
				fmt.Printf("Note: could not load existing issue body for diff comment: %v\n", err)
			} else {
				previousBody = current
				bodyLoaded = true
			}

			titleChanged := existingIssue.Title != title
			bodyChanged := bodyLoaded && previousBody != body

			// Update existing issue
			if titleChanged || bodyChanged || !bodyLoaded {
				if err := m.updateIssue(existingIssue.Number, title, body); err != nil {
					return fmt.Errorf("updating issue: %w", err)
				}
				fmt.Printf("Updated issue #%d\n", existingIssue.Number)
			} else {
				fmt.Printf("Issue #%d already up to date\n", existingIssue.Number)
			}

			if bodyChanged {
				bodyHash := hashIssueBody(body)
				alreadyPosted, err := m.hasCommentWithBodyHash(existingIssue.Number, bodyHash)
				if err != nil {
					fmt.Printf("Note: could not check existing update comments: %v\n", err)
				} else if alreadyPosted {
					fmt.Printf("Issue #%d update comment already posted for this body hash\n", existingIssue.Number)
				} else {
					comment := m.GenerateIssueBodyUpdateComment(previousBody, body)
					if err := m.addComment(existingIssue.Number, comment); err != nil {
						fmt.Printf("Note: could not add issue update comment: %v\n", err)
					}
				}
			}

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

func (m *IssueManager) getIssueBody(number int) (string, error) {
	cmd := exec.Command("gh", "issue", "view",
		"--repo", m.repo,
		fmt.Sprintf("%d", number),
		"--json", "body")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %w", stderr.String(), err)
	}

	var issue issueBodyResponse
	if err := json.Unmarshal(stdout.Bytes(), &issue); err != nil {
		return "", fmt.Errorf("parsing issue body: %w", err)
	}

	return issue.Body, nil
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

func (m *IssueManager) hasCommentWithBodyHash(number int, bodyHash string) (bool, error) {
	cmd := exec.Command("gh", "issue", "view",
		"--repo", m.repo,
		fmt.Sprintf("%d", number),
		"--json", "comments")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("%s: %w", stderr.String(), err)
	}

	var response issueCommentsResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return false, fmt.Errorf("parsing issue comments: %w", err)
	}

	marker := issueBodyHashMarker(bodyHash)
	for _, comment := range response.Comments {
		if strings.Contains(comment.Body, marker) {
			return true, nil
		}
	}

	return false, nil
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

// GenerateIssueBodyUpdateComment builds the comment posted when the issue body changes.
func (m *IssueManager) GenerateIssueBodyUpdateComment(oldBody, newBody string) string {
	oldCounts := extractIssueCounts(oldBody)
	newCounts := extractIssueCounts(newBody)

	var sb strings.Builder
	sb.WriteString("### Docs Automation: Main Post Updated\n")
	if m.workflowURL != "" {
		sb.WriteString(fmt.Sprintf("Run: [workflow link](%s)\n", m.workflowURL))
	}
	sb.WriteString(fmt.Sprintf("Branch: `%s`\n", m.branch))
	sb.WriteString(fmt.Sprintf("Timestamp: `%s`\n\n", time.Now().UTC().Format(time.RFC3339)))

	sb.WriteString("| Section | Before | After | Delta |\n")
	sb.WriteString("|---|---:|---:|---:|\n")
	sb.WriteString(fmt.Sprintf("| Missing Documentation | %d | %d | %s |\n",
		oldCounts.MissingDocs, newCounts.MissingDocs, formatDelta(newCounts.MissingDocs-oldCounts.MissingDocs)))
	sb.WriteString(fmt.Sprintf("| Missing Variables Sections | %d | %d | %s |\n",
		oldCounts.MissingSections, newCounts.MissingSections, formatDelta(newCounts.MissingSections-oldCounts.MissingSections)))
	sb.WriteString(fmt.Sprintf("| Missing Overview Sections | %d | %d | %s |\n",
		oldCounts.MissingOverviewSections, newCounts.MissingOverviewSections, formatDelta(newCounts.MissingOverviewSections-oldCounts.MissingOverviewSections)))
	sb.WriteString(fmt.Sprintf("| Orphaned Documentation | %d | %d | %s |\n\n",
		oldCounts.OrphanedDocs, newCounts.OrphanedDocs, formatDelta(newCounts.OrphanedDocs-oldCounts.OrphanedDocs)))

	sb.WriteString("<details>\n")
	sb.WriteString("<summary>Issue body diff</summary>\n\n")
	sb.WriteString("```diff\n")
	sb.WriteString(buildCompactLineDiff(oldBody, newBody, 160))
	sb.WriteString("\n```\n")
	sb.WriteString("</details>\n\n")

	sb.WriteString(issueBodyHashMarker(hashIssueBody(newBody)))

	return sb.String()
}

func extractIssueCounts(body string) issueCounts {
	return issueCounts{
		MissingDocs:             parseIssueCount(body, missingDocsCountRegex),
		MissingSections:         parseIssueCount(body, missingSectionsCountRegex),
		MissingOverviewSections: parseIssueCount(body, missingOverviewSectionsCountRegex),
		OrphanedDocs:            parseIssueCount(body, orphanedDocsCountRegex),
	}
}

func parseIssueCount(body string, pattern *regexp.Regexp) int {
	matches := pattern.FindStringSubmatch(body)
	if len(matches) != 2 {
		return 0
	}

	count, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0
	}
	return count
}

func formatDelta(delta int) string {
	if delta > 0 {
		return fmt.Sprintf("+%d", delta)
	}
	return fmt.Sprintf("%d", delta)
}

func hashIssueBody(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

func issueBodyHashMarker(hash string) string {
	return fmt.Sprintf("<!-- docs-automation-body-sha256:%s -->", hash)
}

type diffOp struct {
	kind byte
	line string
}

func buildCompactLineDiff(oldBody, newBody string, maxChangedLines int) string {
	oldLines := splitLinesPreserveEmpty(oldBody)
	newLines := splitLinesPreserveEmpty(newBody)

	ops := computeDiffOps(oldLines, newLines)

	var sb strings.Builder
	inHunk := false
	oldPos := 1
	newPos := 1
	hunkOldStart := 0
	hunkNewStart := 0
	hunkOldCount := 0
	hunkNewCount := 0
	changedLines := 0

	flushHunk := func(lines []string) {
		if !inHunk {
			return
		}
		sb.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", hunkOldStart, hunkOldCount, hunkNewStart, hunkNewCount))
		for _, line := range lines {
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
		inHunk = false
	}

	hunkLines := make([]string, 0)

	for _, op := range ops {
		switch op.kind {
		case '=':
			if inHunk {
				flushHunk(hunkLines)
				hunkLines = hunkLines[:0]
			}
			oldPos++
			newPos++
		case '-':
			if changedLines >= maxChangedLines {
				continue
			}
			if !inHunk {
				inHunk = true
				hunkOldStart = oldPos
				hunkNewStart = newPos
				hunkOldCount = 0
				hunkNewCount = 0
			}
			hunkOldCount++
			hunkLines = append(hunkLines, "-"+truncateDiffLine(op.line, 300))
			oldPos++
			changedLines++
		case '+':
			if changedLines >= maxChangedLines {
				continue
			}
			if !inHunk {
				inHunk = true
				hunkOldStart = oldPos
				hunkNewStart = newPos
				hunkOldCount = 0
				hunkNewCount = 0
			}
			hunkNewCount++
			hunkLines = append(hunkLines, "+"+truncateDiffLine(op.line, 300))
			newPos++
			changedLines++
		}
	}

	if inHunk {
		flushHunk(hunkLines)
	}

	diff := strings.TrimSpace(sb.String())
	if diff == "" {
		return "- (no line-level diff available)\n+ (issue body changed)"
	}
	if changedLines >= maxChangedLines {
		diff += "\n... (diff truncated)"
	}
	return diff
}

func splitLinesPreserveEmpty(s string) []string {
	if s == "" {
		return []string{}
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(s, "\n")
}

func truncateDiffLine(line string, maxLen int) string {
	if len(line) <= maxLen {
		return line
	}
	if maxLen < 4 {
		return line[:maxLen]
	}
	return line[:maxLen-3] + "..."
}

func computeDiffOps(oldLines, newLines []string) []diffOp {
	n := len(oldLines)
	m := len(newLines)

	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}

	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	i := 0
	j := 0
	ops := make([]diffOp, 0, n+m)
	for i < n && j < m {
		if oldLines[i] == newLines[j] {
			ops = append(ops, diffOp{kind: '=', line: oldLines[i]})
			i++
			j++
		} else if lcs[i+1][j] >= lcs[i][j+1] {
			ops = append(ops, diffOp{kind: '-', line: oldLines[i]})
			i++
		} else {
			ops = append(ops, diffOp{kind: '+', line: newLines[j]})
			j++
		}
	}

	for i < n {
		ops = append(ops, diffOp{kind: '-', line: oldLines[i]})
		i++
	}
	for j < m {
		ops = append(ops, diffOp{kind: '+', line: newLines[j]})
		j++
	}

	return ops
}
