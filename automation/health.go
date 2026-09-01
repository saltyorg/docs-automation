package automation

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/document"
	"github.com/saltyorg/docs-automation/github"
	"github.com/saltyorg/docs-automation/health"
)

type docRecord struct {
	Repository string
	Role       string
	Path       string
	Relative   string
	Document   *document.Document
	ParseError error
}

type roleTarget struct {
	Repository string
	Role       string
}

type healthCollection struct {
	results    map[health.Kind]*health.Result
	exemptions map[health.Kind]map[string]struct{}
}

func newHealthCollection(coverageEnabled, frontmatterEnabled, editorialEnabled, cliRequested bool) *healthCollection {
	collection := &healthCollection{
		results:    make(map[health.Kind]*health.Result),
		exemptions: make(map[health.Kind]map[string]struct{}),
	}
	collection.addResult(health.RoleAutomationError, true)
	collection.addResult(health.CLIHelpAutomationError, cliRequested)
	for _, kind := range []health.Kind{
		health.MissingDocumentation,
		health.MissingVariablesSection,
		health.MissingOverviewSection,
		health.OrphanedDocumentation,
	} {
		collection.addResult(kind, coverageEnabled)
	}
	collection.addResult(health.InvalidFrontmatter, frontmatterEnabled)
	collection.addResult(health.EditorialAttention, editorialEnabled)
	return collection
}

func (c *healthCollection) addResult(kind health.Kind, enabled bool) {
	c.results[kind] = &health.Result{Kind: kind, Enabled: enabled}
	c.exemptions[kind] = make(map[string]struct{})
}

func (c *healthCollection) addFinding(kind health.Kind, finding health.Finding) {
	if !c.results[kind].Enabled {
		return
	}
	finding.Kind = kind
	c.results[kind].Findings = append(c.results[kind].Findings, finding)
}

func (c *healthCollection) exempt(kind health.Kind, subject string) {
	if !c.results[kind].Enabled {
		return
	}
	c.exemptions[kind][subject] = struct{}{}
}

func (c *healthCollection) report(run health.RunInfo) health.Report {
	results := make([]health.Result, 0, len(c.results))
	for _, result := range c.results {
		result.Exemptions = len(c.exemptions[result.Kind])
		results = append(results, *result)
	}
	return health.NewReport(results, run)
}

func (r *Runner) buildHealthReport(
	ctx context.Context,
	cfg *config.Config,
	summary *github.UpdateSummary,
	cliRequested bool,
	cliErr error,
) (health.Report, error) {
	if err := ctx.Err(); err != nil {
		return health.Report{}, err
	}

	coverageEnabled := cfg.Checks.Coverage.EnabledOr(true)
	frontmatterEnabled := cfg.Checks.Frontmatter.EnabledOr(false)
	editorialEnabled := cfg.Checks.Editorial.EnabledOr(false)
	collection := newHealthCollection(coverageEnabled, frontmatterEnabled, editorialEnabled, cliRequested)
	collectAutomationFindings(collection, summary, cliRequested, cliErr)

	saltboxRoles, err := listRoles(cfg.SaltboxRolesPath())
	if err != nil {
		return health.Report{}, fmt.Errorf("listing saltbox roles: %w", err)
	}
	sandboxRoles, err := listRoles(cfg.SandboxRolesPath())
	if err != nil {
		return health.Report{}, fmt.Errorf("listing sandbox roles: %w", err)
	}
	roles := map[string][]string{
		"saltbox": saltboxRoles,
		"sandbox": sandboxRoles,
	}
	roleSets := map[string]map[string]struct{}{
		"saltbox": toStringSet(saltboxRoles),
		"sandbox": toStringSet(sandboxRoles),
	}
	blacklists := map[string]map[string]struct{}{
		"saltbox": toStringSet(cfg.Blacklist.DocsCoverage.Saltbox),
		"sandbox": toStringSet(cfg.Blacklist.DocsCoverage.Sandbox),
	}

	overrideTargets, err := buildOverrideTargets(cfg)
	if err != nil {
		return health.Report{}, err
	}
	records, err := discoverHealthDocuments(ctx, cfg, overrideTargets, blacklists, coverageEnabled, frontmatterEnabled, editorialEnabled)
	if err != nil {
		return health.Report{}, err
	}
	recordsByPath := make(map[string]docRecord, len(records))
	for _, record := range records {
		recordsByPath[filepath.Clean(record.Path)] = record
	}

	if coverageEnabled {
		collectMissingDocumentation(collection, cfg, roles, blacklists, recordsByPath)
		manager := document.NewManager(document.MarkerConfig{
			Variables: cfg.Markers.Variables,
			CLI:       cfg.Markers.CLI,
			Overview:  cfg.Markers.Overview,
		})
		for _, record := range records {
			if err := ctx.Err(); err != nil {
				return health.Report{}, err
			}
			if err := collectCoverageForDocument(collection, cfg, manager, record, roleSets, blacklists); err != nil {
				return health.Report{}, err
			}
		}
	}

	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return health.Report{}, err
		}
		collectFrontmatterForDocument(collection, cfg, record)
		collectEditorialForDocument(collection, cfg, record)
	}

	run, err := r.healthRunInfo(ctx, cfg)
	if err != nil {
		return health.Report{}, err
	}
	return collection.report(run), nil
}

func collectAutomationFindings(collection *healthCollection, summary *github.UpdateSummary, cliRequested bool, cliErr error) {
	if summary != nil {
		for _, result := range summary.Roles {
			if result.Status != github.StatusError {
				continue
			}
			collection.addFinding(health.RoleAutomationError, health.Finding{
				Repository: result.RepoType,
				Subject:    result.Name,
				SourcePath: filepath.ToSlash(filepath.Join("roles", result.Name)),
				Code:       "role_update_failed",
				Detail:     fmt.Sprintf("role documentation automation failed for %s/%s", result.RepoType, result.Name),
			})
		}
	}
	if cliRequested && cliErr != nil {
		collection.addFinding(health.CLIHelpAutomationError, health.Finding{
			Repository: "docs",
			Subject:    "CLI help",
			Code:       "cli_help_update_failed",
			Detail:     "CLI help documentation automation failed",
		})
	}
}

func buildOverrideTargets(cfg *config.Config) (map[string]roleTarget, error) {
	targets := make(map[string]roleTarget)
	for _, repository := range slices.Sorted(maps.Keys(cfg.PathOverrides)) {
		repositoryOverrides := cfg.PathOverrides[repository]
		for _, role := range slices.Sorted(maps.Keys(repositoryOverrides)) {
			path := filepath.Clean(filepath.Join(cfg.Repositories.Docs, repositoryOverrides[role]))
			info, err := os.Stat(path)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, fmt.Errorf("checking documentation override for %s/%s: %w", repository, role, err)
			}
			if !info.IsDir() {
				targets[path] = roleTarget{Repository: repository, Role: role}
			}
		}
	}
	return targets, nil
}

func discoverHealthDocuments(
	ctx context.Context,
	cfg *config.Config,
	overrideTargets map[string]roleTarget,
	blacklists map[string]map[string]struct{},
	coverageEnabled, frontmatterEnabled, editorialEnabled bool,
) ([]docRecord, error) {
	type discoveredPath struct {
		path       string
		repository string
	}
	paths := make(map[string]discoveredPath)
	for _, source := range []struct {
		repository string
		root       string
	}{
		{repository: "saltbox", root: cfg.SaltboxDocsPath()},
		{repository: "sandbox", root: cfg.SandboxDocsPath()},
	} {
		files, err := document.ListDocFiles(source.root)
		if err != nil {
			return nil, fmt.Errorf("listing %s docs: %w", source.repository, err)
		}
		for _, path := range files {
			clean := filepath.Clean(path)
			paths[clean] = discoveredPath{path: clean, repository: source.repository}
		}
	}
	for path, target := range overrideTargets {
		if _, exists := paths[path]; !exists {
			paths[path] = discoveredPath{path: path, repository: target.Repository}
		}
	}

	orderedPaths := slices.Sorted(maps.Keys(paths))
	records := make([]docRecord, 0, len(orderedPaths))
	for _, path := range orderedPaths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		relative, err := docsRelativePath(cfg.Repositories.Docs, path)
		if err != nil {
			return nil, fmt.Errorf("resolving documentation path: %w", err)
		}
		discovered := paths[path]
		repository := discovered.repository
		role := document.ExtractRoleName(path)
		if target, exists := overrideTargets[path]; exists {
			repository = target.Repository
			role = target.Role
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading documentation %s: %w", relative, err)
		}
		record := docRecord{
			Repository: repository,
			Role:       role,
			Path:       path,
			Relative:   relative,
		}
		_, coverageBlacklisted := blacklists[repository][role]
		needsParse := (coverageEnabled && !coverageBlacklisted && !cfg.Checks.Coverage.Excludes(relative)) ||
			(frontmatterEnabled && !cfg.Checks.Frontmatter.Excludes(relative)) ||
			(editorialEnabled && !cfg.Checks.Editorial.Excludes(relative))
		if needsParse {
			frontmatter, body, parseErr := document.ParseFrontmatter(string(data))
			record.Document = &document.Document{
				Path:        path,
				Content:     string(data),
				Frontmatter: frontmatter,
				Body:        body,
			}
			record.ParseError = parseErr
		}
		records = append(records, record)
	}
	return records, nil
}

func collectMissingDocumentation(
	collection *healthCollection,
	cfg *config.Config,
	roles map[string][]string,
	blacklists map[string]map[string]struct{},
	recordsByPath map[string]docRecord,
) {
	for _, repository := range []string{"saltbox", "sandbox"} {
		for _, role := range roles[repository] {
			key := repository + "/" + role
			if _, blacklisted := blacklists[repository][role]; blacklisted {
				collection.exempt(health.MissingDocumentation, key)
				continue
			}
			docPath := filepath.Clean(getDocPath(cfg, role, repository))
			if _, exists := recordsByPath[docPath]; exists {
				continue
			}
			relative, _ := docsRelativePath(cfg.Repositories.Docs, docPath)
			collection.addFinding(health.MissingDocumentation, health.Finding{
				Repository: repository,
				Subject:    role,
				Path:       relative,
				SourcePath: filepath.ToSlash(filepath.Join("roles", role)),
				Code:       "missing_doc",
				Detail:     fmt.Sprintf("%s role %s has no documentation page", repository, role),
			})
		}
	}
}

func collectCoverageForDocument(
	collection *healthCollection,
	cfg *config.Config,
	manager *document.Manager,
	record docRecord,
	roleSets map[string]map[string]struct{},
	blacklists map[string]map[string]struct{},
) error {
	coverageKinds := []health.Kind{
		health.MissingVariablesSection,
		health.MissingOverviewSection,
		health.OrphanedDocumentation,
	}
	if cfg.Checks.Coverage.Excludes(record.Relative) {
		exemptDocumentKinds(collection, coverageKinds, record.Relative)
		return nil
	}
	if _, blacklisted := blacklists[record.Repository][record.Role]; blacklisted {
		exemptDocumentKinds(collection, coverageKinds, record.Relative)
		return nil
	}
	if record.ParseError != nil {
		collectOrphanedDocumentation(collection, record, roleSets)
		return nil
	}

	var automation *document.SaltboxAutomationConfig
	if record.Document != nil && record.Document.Frontmatter != nil {
		automation = record.Document.Frontmatter.SaltboxAutomation
	}
	if automation != nil && automation.Disabled {
		exemptDocumentKinds(collection, coverageKinds, record.Relative)
		return nil
	}
	if !automation.IsCoverageCheckEnabled() {
		exemptDocumentKinds(collection, coverageKinds, record.Relative)
		return nil
	}

	collectOrphanedDocumentation(collection, record, roleSets)
	if !automation.IsInventorySectionEnabled() {
		collection.exempt(health.MissingVariablesSection, record.Relative)
	} else if _, roleExists := roleSets[record.Repository][record.Role]; roleExists {
		defaultsPath := filepath.Join(repositoryRoot(cfg, record.Repository), "roles", record.Role, "defaults", "main.yml")
		_, err := os.Stat(defaultsPath)
		switch {
		case err == nil && !manager.HasVariablesSection(record.Document):
			collection.addFinding(health.MissingVariablesSection, documentFinding(
				record, "missing_variables_section", "enabled inventory automation has no managed variables section",
			))
		case err != nil && !os.IsNotExist(err):
			return fmt.Errorf("checking defaults for %s/%s: %w", record.Repository, record.Role, err)
		}
	}

	if !automation.IsOverviewSectionEnabled() {
		collection.exempt(health.MissingOverviewSection, record.Relative)
	} else if !manager.HasOverviewSection(record.Document) {
		collection.addFinding(health.MissingOverviewSection, documentFinding(
			record, "missing_overview_section", "enabled overview automation has no managed overview section",
		))
	}
	return nil
}

func collectOrphanedDocumentation(
	collection *healthCollection,
	record docRecord,
	roleSets map[string]map[string]struct{},
) {
	if _, exists := roleSets[record.Repository][record.Role]; exists {
		return
	}
	collection.addFinding(health.OrphanedDocumentation, health.Finding{
		Repository: record.Repository,
		Subject:    record.Role,
		Path:       record.Relative,
		Code:       "orphaned_doc",
		Detail:     "documentation page has no corresponding source role",
	})
}

func collectFrontmatterForDocument(collection *healthCollection, cfg *config.Config, record docRecord) {
	if !collection.results[health.InvalidFrontmatter].Enabled {
		return
	}
	if cfg.Checks.Frontmatter.Excludes(record.Relative) {
		collection.exempt(health.InvalidFrontmatter, record.Relative)
		return
	}
	if record.ParseError != nil {
		collection.addFinding(health.InvalidFrontmatter, health.Finding{
			Repository: record.Repository,
			Subject:    record.Role,
			Path:       record.Relative,
			SourcePath: filepath.ToSlash(filepath.Join("roles", record.Role)),
			Code:       "frontmatter_parse_error",
			Detail:     "frontmatter syntax could not be parsed",
		})
		return
	}
	if record.Document == nil {
		return
	}
	var automation *document.SaltboxAutomationConfig
	if record.Document.Frontmatter != nil {
		automation = record.Document.Frontmatter.SaltboxAutomation
	}
	if automation != nil && automation.Disabled {
		collection.exempt(health.InvalidFrontmatter, record.Relative)
		return
	}
	if !automation.IsFrontmatterCheckEnabled() {
		collection.exempt(health.InvalidFrontmatter, record.Relative)
		return
	}
	if !automation.IsOverviewSectionEnabled() {
		collection.exempt(health.InvalidFrontmatter, record.Relative)
		return
	}
	diagnostics := document.ValidateAutomationFrontmatter(record.Document.Frontmatter)
	if len(diagnostics) == 0 {
		return
	}
	codes := make([]string, 0, len(diagnostics))
	messages := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		codes = append(codes, diagnostic.Code)
		messages = append(messages, diagnostic.Message)
	}
	slices.Sort(codes)
	slices.Sort(messages)
	collection.addFinding(health.InvalidFrontmatter, health.Finding{
		Repository: record.Repository,
		Subject:    record.Role,
		Path:       record.Relative,
		SourcePath: filepath.ToSlash(filepath.Join("roles", record.Role)),
		Code:       strings.Join(codes, ","),
		Detail:     strings.Join(messages, "; "),
	})
}

func collectEditorialForDocument(collection *healthCollection, cfg *config.Config, record docRecord) {
	if !collection.results[health.EditorialAttention].Enabled {
		return
	}
	if cfg.Checks.Editorial.Excludes(record.Relative) {
		collection.exempt(health.EditorialAttention, record.Relative)
		return
	}
	if record.ParseError != nil || record.Document == nil {
		return
	}
	var automation *document.SaltboxAutomationConfig
	if record.Document.Frontmatter != nil {
		automation = record.Document.Frontmatter.SaltboxAutomation
	}
	if automation != nil && automation.Disabled {
		collection.exempt(health.EditorialAttention, record.Relative)
		return
	}
	if !automation.IsEditorialCheckEnabled() {
		collection.exempt(health.EditorialAttention, record.Relative)
		return
	}
	if record.Document.Frontmatter == nil || !slices.Contains(cfg.Checks.Editorial.Statuses, record.Document.Frontmatter.Status) {
		return
	}
	status := record.Document.Frontmatter.Status
	collection.addFinding(health.EditorialAttention, health.Finding{
		Repository: record.Repository,
		Subject:    record.Role,
		Path:       record.Relative,
		SourcePath: filepath.ToSlash(filepath.Join("roles", record.Role)),
		Code:       "editorial_status",
		Detail:     fmt.Sprintf("documentation status is %q", status),
	})
}

func documentFinding(record docRecord, code, detail string) health.Finding {
	return health.Finding{
		Repository: record.Repository,
		Subject:    record.Role,
		Path:       record.Relative,
		SourcePath: filepath.ToSlash(filepath.Join("roles", record.Role)),
		Code:       code,
		Detail:     detail,
	}
}

func exemptDocumentKinds(collection *healthCollection, kinds []health.Kind, relative string) {
	for _, kind := range kinds {
		collection.exempt(kind, relative)
	}
}

func docsRelativePath(root, path string) (string, error) {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(relative), nil
}

func repositoryRoot(cfg *config.Config, repository string) string {
	if repository == "sandbox" {
		return cfg.Repositories.Sandbox
	}
	return cfg.Repositories.Saltbox
}

func toStringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}
