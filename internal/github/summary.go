package github

import (
	"fmt"
	"os"
	"strings"
)

// RoleStatus represents the processing status of a role.
type RoleStatus string

const (
	StatusUpdated   RoleStatus = "updated"
	StatusUnchanged RoleStatus = "unchanged"
	StatusSkipped   RoleStatus = "skipped"
	StatusError     RoleStatus = "error"
)

// RoleResult holds the result of processing a single role.
type RoleResult struct {
	Name       string
	RepoType   string     // "saltbox" or "sandbox"
	Status     RoleStatus // processing status
	SkipReason string     // reason if skipped
	Error      string     // error message if failed
	Sections   []string   // which sections were updated (e.g., "variables", "overview")
}

// UpdateSummary holds the complete summary of an update run.
type UpdateSummary struct {
	Roles       []RoleResult
	CLIUpdated  bool
	TotalRoles  int
	Updated     int
	Unchanged   int
	Skipped     int
	Errors      int
	CheckResult *CheckResult // optional check results
}

// NewUpdateSummary creates a new UpdateSummary.
func NewUpdateSummary() *UpdateSummary {
	return &UpdateSummary{
		Roles: make([]RoleResult, 0),
	}
}

// AddRole adds a role result to the summary.
func (s *UpdateSummary) AddRole(result RoleResult) {
	s.Roles = append(s.Roles, result)
	s.TotalRoles++

	switch result.Status {
	case StatusUpdated:
		s.Updated++
	case StatusUnchanged:
		s.Unchanged++
	case StatusSkipped:
		s.Skipped++
	case StatusError:
		s.Errors++
	}
}

// SetCheckResult sets the check results for the summary.
func (s *UpdateSummary) SetCheckResult(result *CheckResult) {
	s.CheckResult = result
}

// WriteGitHubSummary writes the summary to GITHUB_STEP_SUMMARY if running in GitHub Actions.
func (s *UpdateSummary) WriteGitHubSummary() error {
	// Check if we're running in GitHub Actions
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		return nil
	}

	summaryFile := os.Getenv("GITHUB_STEP_SUMMARY")
	if summaryFile == "" {
		return nil
	}

	f, err := os.OpenFile(summaryFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("opening summary file: %w", err)
	}
	defer f.Close()

	var sb strings.Builder

	sb.WriteString("## 📚 Documentation Automation Results\n\n")

	// Statistics table
	sb.WriteString("### Statistics\n\n")
	sb.WriteString("| Metric | Count |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Roles Processed | %d |\n", s.TotalRoles))
	sb.WriteString(fmt.Sprintf("| ✅ Updated | %d |\n", s.Updated))
	sb.WriteString(fmt.Sprintf("| ➖ Unchanged | %d |\n", s.Unchanged))
	sb.WriteString(fmt.Sprintf("| ⏭️ Skipped | %d |\n", s.Skipped))
	sb.WriteString(fmt.Sprintf("| ❌ Errors | %d |\n", s.Errors))
	if s.CLIUpdated {
		sb.WriteString("| 🖥️ CLI Help | Updated |\n")
	}
	sb.WriteString("\n")

	// Updated roles (collapsible if many)
	if s.Updated > 0 {
		updatedRoles := s.getRolesByStatus(StatusUpdated)
		if len(updatedRoles) > 10 {
			sb.WriteString("<details>\n")
			sb.WriteString(fmt.Sprintf("<summary><strong>Updated Documentation (%d roles)</strong></summary>\n\n", len(updatedRoles)))
		} else {
			sb.WriteString(fmt.Sprintf("### Updated Documentation (%d)\n\n", len(updatedRoles)))
		}

		sb.WriteString("| Role | Repository | Sections |\n")
		sb.WriteString("|------|------------|----------|\n")
		for _, r := range updatedRoles {
			sections := "variables"
			if len(r.Sections) > 0 {
				sections = strings.Join(r.Sections, ", ")
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", r.Name, r.RepoType, sections))
		}
		sb.WriteString("\n")

		if len(updatedRoles) > 10 {
			sb.WriteString("</details>\n\n")
		}
	}

	// Skipped roles (collapsible)
	if s.Skipped > 0 {
		skippedRoles := s.getRolesByStatus(StatusSkipped)
		sb.WriteString("<details>\n")
		sb.WriteString(fmt.Sprintf("<summary><strong>Skipped Roles (%d)</strong></summary>\n\n", len(skippedRoles)))

		sb.WriteString("| Role | Repository | Reason |\n")
		sb.WriteString("|------|------------|--------|\n")
		for _, r := range skippedRoles {
			sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", r.Name, r.RepoType, r.SkipReason))
		}
		sb.WriteString("\n</details>\n\n")
	}

	// Errors
	if s.Errors > 0 {
		errorRoles := s.getRolesByStatus(StatusError)
		sb.WriteString(fmt.Sprintf("### ❌ Errors (%d)\n\n", len(errorRoles)))

		sb.WriteString("| Role | Repository | Error |\n")
		sb.WriteString("|------|------------|-------|\n")
		for _, r := range errorRoles {
			// Escape pipe characters in error messages
			errMsg := strings.ReplaceAll(r.Error, "|", "\\|")
			sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", r.Name, r.RepoType, errMsg))
		}
		sb.WriteString("\n")
	}

	// Check results if available
	if s.CheckResult != nil && s.CheckResult.HasIssues() {
		sb.WriteString("### 🔍 Coverage Check Results\n\n")

		if len(s.CheckResult.MissingDocs) > 0 {
			sb.WriteString(fmt.Sprintf("**Missing Documentation:** %d roles\n", len(s.CheckResult.MissingDocs)))
			sb.WriteString("<details>\n<summary>Show roles</summary>\n\n")
			for _, role := range s.CheckResult.MissingDocs {
				sb.WriteString(fmt.Sprintf("- `%s`\n", role))
			}
			sb.WriteString("\n</details>\n\n")
		}

		if len(s.CheckResult.MissingSections) > 0 {
			sb.WriteString(fmt.Sprintf("**Missing Variables Sections:** %d docs\n", len(s.CheckResult.MissingSections)))
			sb.WriteString("<details>\n<summary>Show docs</summary>\n\n")
			for _, doc := range s.CheckResult.MissingSections {
				sb.WriteString(fmt.Sprintf("- `%s`\n", doc))
			}
			sb.WriteString("\n</details>\n\n")
		}

		if len(s.CheckResult.MissingOverviewSections) > 0 {
			sb.WriteString(fmt.Sprintf("**Missing Overview Sections:** %d docs\n", len(s.CheckResult.MissingOverviewSections)))
			sb.WriteString("<details>\n<summary>Show docs</summary>\n\n")
			for _, doc := range s.CheckResult.MissingOverviewSections {
				sb.WriteString(fmt.Sprintf("- `%s`\n", doc))
			}
			sb.WriteString("\n</details>\n\n")
		}

		if len(s.CheckResult.OrphanedDocs) > 0 {
			sb.WriteString(fmt.Sprintf("**Orphaned Documentation:** %d docs\n", len(s.CheckResult.OrphanedDocs)))
			sb.WriteString("<details>\n<summary>Show docs</summary>\n\n")
			for _, doc := range s.CheckResult.OrphanedDocs {
				sb.WriteString(fmt.Sprintf("- `%s`\n", doc))
			}
			sb.WriteString("\n</details>\n\n")
		}
	}

	_, err = f.WriteString(sb.String())
	return err
}

// getRolesByStatus returns all roles with the given status.
func (s *UpdateSummary) getRolesByStatus(status RoleStatus) []RoleResult {
	var results []RoleResult
	for _, r := range s.Roles {
		if r.Status == status {
			results = append(results, r)
		}
	}
	return results
}
