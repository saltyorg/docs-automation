package github

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/saltyorg/docs-automation/health"
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
	Roles        []RoleResult
	CLIUpdated   bool
	TotalRoles   int
	Updated      int
	Unchanged    int
	Skipped      int
	Errors       int
	HealthReport *health.Report
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

// SetHealthReport sets the canonical documentation health report.
func (s *UpdateSummary) SetHealthReport(report *health.Report) {
	s.HealthReport = report
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

	var sb strings.Builder

	sb.WriteString("## 📚 Documentation Automation Results\n\n")

	// Statistics table
	sb.WriteString("### Statistics\n\n")
	sb.WriteString("| Metric | Count |\n")
	sb.WriteString("|--------|-------|\n")
	fmt.Fprintf(&sb, "| Roles Processed | %d |\n", s.TotalRoles)
	fmt.Fprintf(&sb, "| ✅ Updated | %d |\n", s.Updated)
	fmt.Fprintf(&sb, "| ➖ Unchanged | %d |\n", s.Unchanged)
	fmt.Fprintf(&sb, "| ⏭️ Skipped | %d |\n", s.Skipped)
	fmt.Fprintf(&sb, "| ❌ Errors | %d |\n", s.Errors)
	if s.CLIUpdated {
		sb.WriteString("| 🖥️ CLI Help | Updated |\n")
	}
	sb.WriteString("\n")

	// Updated roles (collapsible if many)
	if s.Updated > 0 {
		updatedRoles := s.getRolesByStatus(StatusUpdated)
		if len(updatedRoles) > 10 {
			sb.WriteString("<details>\n")
			fmt.Fprintf(&sb, "<summary><strong>Updated Documentation (%d roles)</strong></summary>\n\n", len(updatedRoles))
		} else {
			fmt.Fprintf(&sb, "### Updated Documentation (%d)\n\n", len(updatedRoles))
		}

		sb.WriteString("| Role | Repository | Sections |\n")
		sb.WriteString("|------|------------|----------|\n")
		for _, r := range updatedRoles {
			sections := "variables"
			if len(r.Sections) > 0 {
				sections = strings.Join(r.Sections, ", ")
			}
			fmt.Fprintf(&sb, "| %s | %s | %s |\n", markdownTableCell(r.Name), markdownTableCell(r.RepoType), markdownTableCell(sections))
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
		fmt.Fprintf(&sb, "<summary><strong>Skipped Roles (%d)</strong></summary>\n\n", len(skippedRoles))

		sb.WriteString("| Role | Repository | Reason |\n")
		sb.WriteString("|------|------------|--------|\n")
		for _, r := range skippedRoles {
			fmt.Fprintf(&sb, "| %s | %s | %s |\n", markdownTableCell(r.Name), markdownTableCell(r.RepoType), markdownTableCell(r.SkipReason))
		}
		sb.WriteString("\n</details>\n\n")
	}

	// Errors
	if s.Errors > 0 {
		errorRoles := s.getRolesByStatus(StatusError)
		fmt.Fprintf(&sb, "### ❌ Errors (%d)\n\n", len(errorRoles))

		sb.WriteString("| Role | Repository | Error |\n")
		sb.WriteString("|------|------------|-------|\n")
		for _, r := range errorRoles {
			fmt.Fprintf(&sb, "| %s | %s | %s |\n", markdownTableCell(r.Name), markdownTableCell(r.RepoType), markdownTableCell(r.Error))
		}
		sb.WriteString("\n")
	}

	if s.HealthReport != nil {
		writeHealthReport(&sb, *s.HealthReport)
	}

	_, writeErr := f.WriteString(sb.String())
	closeErr := f.Close()
	return errors.Join(writeErr, closeErr)
}

func writeHealthReport(sb *strings.Builder, report health.Report) {
	report = report.Canonical()
	sb.WriteString("### 🩺 Documentation Health\n\n")
	sb.WriteString("| Check | Status | Findings | Exemptions |\n")
	sb.WriteString("|-------|--------|----------|------------|\n")
	for _, result := range report.Results {
		status := "Disabled"
		if result.Enabled && len(result.Findings) == 0 {
			status = "Passed"
		} else if result.Enabled {
			status = "Enabled"
		}
		fmt.Fprintf(sb, "| %s | %s | %d | %d |\n", markdownTableCell(healthResultLabel(result.Kind)), status, len(result.Findings), result.Exemptions)
	}
	sb.WriteString("\n")

	for _, result := range report.Results {
		if !result.Enabled || len(result.Findings) == 0 {
			continue
		}
		fmt.Fprintf(sb, "### %s (%d)\n\n", markdownIssueText(healthResultLabel(result.Kind)), len(result.Findings))
		sb.WriteString("| Repository | Subject | Path | Detail |\n")
		sb.WriteString("|------------|---------|------|--------|\n")
		for _, finding := range result.Findings {
			fmt.Fprintf(sb, "| %s | %s | %s | %s |\n",
				markdownTableCell(finding.Repository),
				markdownTableCell(finding.Label()),
				markdownTableCell(finding.Path),
				markdownTableCell(finding.Detail),
			)
		}
		sb.WriteString("\n")
	}
}

func healthResultLabel(kind health.Kind) string {
	switch kind {
	case health.RoleAutomationError:
		return "Role Automation Errors"
	case health.CLIHelpAutomationError:
		return "CLI Help Automation Errors"
	case health.MissingDocumentation:
		return "Missing Documentation"
	case health.InvalidFrontmatter:
		return "Invalid Frontmatter"
	case health.MissingVariablesSection:
		return "Missing Variables Sections"
	case health.MissingOverviewSection:
		return "Missing Overview Sections"
	case health.OrphanedDocumentation:
		return "Orphaned Documentation"
	case health.EditorialAttention:
		return "Editorial Attention"
	default:
		return string(kind)
	}
}

func markdownTableCell(value string) string {
	return markdownIssueText(value)
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
