package automation

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/saltyorg/docs-automation/buildinfo"
	"github.com/saltyorg/docs-automation/config"
	"github.com/saltyorg/docs-automation/github"
	"github.com/saltyorg/docs-automation/health"
)

type revisionResolver func(context.Context, string) (string, error)

func gitRevision(ctx context.Context, path string) (string, error) {
	output, err := exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("resolving Git revision: %w", err)
	}
	revision := strings.TrimSpace(string(output))
	if revision == "" {
		return "", fmt.Errorf("resolving Git revision: empty output")
	}
	return revision, nil
}

func (r *Runner) healthRunInfo(ctx context.Context, cfg *config.Config) (health.RunInfo, error) {
	if err := ctx.Err(); err != nil {
		return health.RunInfo{}, err
	}
	run := health.RunInfo{
		CheckedAt:   time.Now().UTC(),
		WorkflowURL: github.GetWorkflowURL(),
		Branch:      github.GetBranch(),
		Version:     buildinfo.VersionString(),
	}
	resolve := r.resolveRevision
	if resolve == nil {
		resolve = gitRevision
	}

	for _, repository := range []string{"saltbox", "sandbox"} {
		metadata, configured := cfg.Issue.SourceRepositories[repository]
		if !configured {
			continue
		}
		path := cfg.Repositories.Saltbox
		if repository == "sandbox" {
			path = cfg.Repositories.Sandbox
		}
		source := health.SourceRevision{
			Repository: repository,
			Slug:       metadata.Slug,
			Ref:        metadata.Ref,
		}
		revision, err := resolve(ctx, path)
		if cancellation := provenanceCancellation(ctx, err); cancellation != nil {
			return health.RunInfo{}, cancellation
		}
		if err == nil {
			source.Revision = strings.TrimSpace(revision)
		}
		run.Sources = append(run.Sources, source)
	}

	docs := health.SourceRevision{
		Repository: "docs",
		Slug:       github.GetRepository(),
		Ref:        run.Branch,
	}
	revision, err := resolve(ctx, cfg.Repositories.Docs)
	if cancellation := provenanceCancellation(ctx, err); cancellation != nil {
		return health.RunInfo{}, cancellation
	}
	if err == nil {
		docs.Revision = strings.TrimSpace(revision)
	}
	run.Sources = append(run.Sources, docs)
	return run, nil
}

func provenanceCancellation(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}
