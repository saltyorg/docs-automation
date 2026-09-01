# Code Quality Hardening Design

Date: 2026-09-01
Status: approved

## Objective

Make documentation generation fail closed when authoritative inputs cannot be
read, publish documentation changes atomically, propagate output failures,
make GitHub subprocess work context-aware and testable, and enforce the same
quality gates locally and in CI.

Successful documentation output and the existing CLI flags, commands, and
documented all-role error policy must remain compatible.

## Scope

This change covers:

- inventory and shared Docker-source discovery;
- render inputs and orchestration boundaries;
- document and scaffold persistence;
- runner output-error propagation;
- GitHub issue subprocess and output boundaries;
- focused tests for those behaviors;
- Go module, Makefile, and GitHub Actions quality gates.

It does not cover:

- repairing the existing frontmatter corpus;
- changing generated Markdown schemas or presentation;
- changing role parsing or type-inference rules;
- mutating the related Docs checkout during implementation;
- live GitHub issue mutations;
- commits, pushes, tags, or releases.

## Design Principles

- Fail before mutation when a shared authoritative source is unavailable.
- Keep rendering deterministic and free of filesystem access.
- Publish only complete files and preserve existing content after failures.
- Route all command output through injected writers and return write errors.
- Pass cancellation through every external process boundary.
- Add only small interfaces at real substitution points.
- Preserve existing per-role continuation behavior for all-role operations.

## Source Discovery and Rendering

### Source catalog

Add one source-catalog type owned by `render`. It contains the already-scanned
inventory role-variable lookups and normalized shared Docker variable suffixes.
It is plain data and has no loading behavior.

Add an orchestration loader in `automation` that builds this catalog from the
configured Saltbox checkout. The loader scans the inventory and Docker task
sources exactly once per `generate` or `update` invocation.

Missing directories, missing inventory, directory-read errors, and individual
Docker task read errors are returned with path and operation context. The
Docker scanner publishes its cache only after a complete successful scan, so a
failed first scan cannot become a successful empty cached result.

### Operation behavior

`Runner.Generate` and `Runner.Update` load the catalog before selecting or
processing roles. A catalog error returns before any document is written.

After catalog loading:

- single-role failures return nonzero;
- all-role parse or render failures remain per-role results and processing
  continues, matching the documented behavior;
- a shared-source failure is a startup failure, not a per-role warning.

### Pure rendering

`render.BuildRoleData` receives the source catalog explicitly. It performs no
file or directory access. Docker filtering, configured grouping, inventory
lookup enrichment, and template-data construction remain deterministic pure
transformations.

Existing render tests pass explicit catalogs. Tests that formerly relied on
temporary filesystem discovery move that setup to automation/parser tests.

## Atomic Persistence

Add a focused atomic file publisher in `document` with these semantics:

1. Create a temporary file in the destination directory.
2. Use the existing regular file's permission bits when replacing it, or the
   caller's requested mode for a new destination.
3. Write all bytes, sync the file, and close it successfully.
4. For replacement, rename the complete temporary file over the destination.
5. For no-clobber creation, link the complete temporary file to the destination
   so an existing destination is never replaced.
6. Remove the temporary name and sync the parent directory.
7. Remove unpublished temporary files after every failure.

The publisher refuses to replace non-regular existing destinations rather than
silently replacing symlinks or directories.

`document.Manager.SaveDocument` uses replacement mode. Scaffold renders its
template into memory first, checks cancellation again, then uses replacement
mode only with `--force`. Without `--force`, an existing destination produces
the current friendly error.

## Output Error Propagation

`Runner` wraps stdout and stderr with small first-error tracking writers.
Formatting helpers remain concise, but the first write error is retained.
Every public runner operation joins that retained error into its return value.

`Runner.Index` changes from a void method to `error`; its Cobra handler returns
that error through `RunE`. A command whose configured writer rejects output
therefore exits nonzero.

Each Cobra invocation continues to construct a fresh runner, so error state
does not leak between commands.

## GitHub Issue Boundary

Define a minimal command-runner interface inside the `github` package, where it
is consumed. It supports executable lookup and one context-aware command run
that returns stdout, stderr, and an error.

The production implementation uses `exec.CommandContext`. Tests inject a
hand-written fake. `IssueManager` also receives stdout and stderr writers.

`ManageIssue` accepts `context.Context`; every `gh` helper receives and uses the
same context. Repeated command-buffer setup is centralized in one `runGH`
helper. Direct `fmt.Printf`, `fmt.Println`, and process-global stderr writes are
removed. Writer failures are returned.

`OutputGitHubActions` returns file-open, write, and close errors rather than
printing and suppressing them. It has no current callers; its error-returning
API leaves failure policy at the calling boundary.

The existing `update --manage-issue` policy remains: GitHub management errors
are reported as warnings during an all-role update, while a runner writer error
still makes the command return nonzero at its public boundary.

## Test Strategy

Behavior changes use red-green-refactor cycles. Regression tests cover:

- missing inventory fails catalog loading;
- missing Docker task sources fail catalog loading;
- an unreadable or broken Docker task entry is not skipped;
- scanner cache is not published after a failed scan;
- render output uses only the supplied catalog;
- a forced scaffold template-execution failure preserves the original file;
- atomic create refuses an existing destination;
- atomic replacement preserves regular-file mode and complete content;
- atomic publication refuses non-regular destinations;
- a failing runner writer is returned through `index` and Cobra;
- GitHub command execution receives cancellation;
- GitHub status output uses injected writers;
- GitHub command stderr is preserved in returned errors.

Characterization tests cover managed-section, frontmatter, CLI-help, and
document-manager paths touched by the refactor. Tests use `t.TempDir`,
`t.Context`, `t.Setenv`, and simple fakes; no live GitHub access is used.

## Local and CI Quality Gates

Run `go mod tidy` so the directly imported test dependency `pflag` is direct.

The Makefile gains:

- a pinned golangci-lint version (`v2.13.2`);
- non-mutating `fmt-check` and `tidy-check` targets;
- a complete `.PHONY` declaration;
- corrected `sb-docs` naming;
- a `check` target covering format, tidy, vet, lint, and race tests.

GitHub Actions runs a read-only quality job for pull requests and pushes. It
checks formatting, tidy state, module integrity, vet, pinned lint, race tests,
build, and workflow syntax with actionlint `v1.7.12`. Artifact and release work
runs only for pushes, depends on the quality job, and receives `contents:
write` only in that job.

## Acceptance Criteria

- Every new behavior test is observed failing before its implementation.
- `gofmt` reports no files.
- `go mod tidy -diff` and `go mod verify` pass.
- `go vet ./...` passes.
- pinned golangci-lint reports zero issues.
- `go test -count=1 -race ./...` passes.
- `go build` and the Makefile build gate pass.
- local overlay configuration validation passes.
- representative `sonarr` output remains byte-identical to the pre-change
  baseline.
- all-role generated output remains byte-identical when authoritative inputs
  are readable.
- actionlint `v1.7.12` reports no workflow errors.
- Git status contains only the intended project and `.agents` files.
