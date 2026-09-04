package document

import (
	"fmt"
	"strings"
)

// Diagnostic describes one frontmatter validation failure.
type Diagnostic struct {
	Code    string
	Message string
}

// ValidateAutomationFrontmatter validates overview metadata when automation enables it.
func ValidateAutomationFrontmatter(fm *Frontmatter) []Diagnostic {
	if fm == nil || fm.SaltboxAutomation == nil {
		return nil
	}

	automation := fm.SaltboxAutomation
	if !automation.IsFrontmatterCheckEnabled() || !automation.IsOverviewSectionEnabled() {
		return nil
	}

	diagnostics := make([]Diagnostic, 0)
	for i, link := range automation.AppLinks {
		if strings.TrimSpace(link.Name) == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Code:    "app_link_name_required",
				Message: fmt.Sprintf("app_links[%d].name is required", i),
			})
		}

		purpose := AppLinkPurpose(strings.TrimSpace(string(link.Purpose)))
		switch {
		case purpose == "":
			diagnostics = append(diagnostics, Diagnostic{
				Code:    "app_link_purpose_required",
				Message: fmt.Sprintf("app_links[%d].purpose is required", i),
			})
		case !purpose.valid():
			diagnostics = append(diagnostics, Diagnostic{
				Code:    "app_link_purpose_invalid",
				Message: fmt.Sprintf("app_links[%d].purpose %q is invalid", i, link.Purpose),
			})
		case purpose.RequiresURL() && strings.TrimSpace(link.URL) == "":
			diagnostics = append(diagnostics, Diagnostic{
				Code:    "app_link_url_required",
				Message: fmt.Sprintf("app_links[%d].url is required", i),
			})
		}
	}

	if automation.ProjectDescription == nil {
		return append(diagnostics, Diagnostic{
			Code:    "project_description_required",
			Message: "project_description is required",
		})
	}

	if strings.TrimSpace(automation.ProjectDescription.Name) == "" {
		diagnostics = append(diagnostics, Diagnostic{
			Code:    "project_description_name_required",
			Message: "project_description.name is required",
		})
	}
	if strings.TrimSpace(automation.ProjectDescription.Summary) == "" {
		diagnostics = append(diagnostics, Diagnostic{
			Code:    "project_description_summary_required",
			Message: "project_description.summary is required",
		})
	}

	return diagnostics
}
