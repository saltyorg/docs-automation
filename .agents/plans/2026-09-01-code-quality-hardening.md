# Code Quality Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Delegation is not authorized for this run. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make source discovery, document persistence, command output, GitHub subprocesses, and release checks fail safely without changing successful generated documentation.

**Architecture:** `automation` loads authoritative source data once and passes a plain `render.SourceCatalog` into a pure renderer. `document` owns atomic file publication, while `Runner` and `github.IssueManager` return I/O and subprocess failures through context-aware boundaries. Local Make targets and GitHub Actions enforce the same pinned quality gates.

**Tech Stack:** Go 1.27.0, Cobra 1.10.2, yaml.v3, standard `testing`, GNU Make, GitHub Actions, golangci-lint v2.13.2, actionlint v1.7.12.

**Spec:** `.agents/specs/2026-09-01-code-quality-hardening-design.md`

## Global Constraints

- Preserve successful CLI commands, flags, help text, and generated Markdown bytes.
- Preserve documented all-role continuation for role-local parse/render failures.
- Treat shared inventory or Docker-source discovery failure as a pre-mutation startup error.
- Do not change parser/type-inference semantics or repair frontmatter corpus debt.
- Do not mutate `/opt/git/docs`, `/srv/git/saltbox`, or `/opt/sandbox`.
- Preserve all dirty related-worktree state; use it only as read-only generation input.
- Do not commit, push, tag, release, or change Git credentials.
- Use `apply_patch` for project file edits and test each behavior red before green.
- Keep `/tmp/docs-automation-before-sb-docs` for final same-input A/B comparison.
- Current baseline is stable across immediate reruns: Sonarr stdout SHA-256 `6d60a54dee5b6ea65a469b9d6992a7a9e86c276fa5008ee192bb8e674346f024`; all-role stdout SHA-256 `aac643cac440120efc471cb61f585a3ee327a7de07fdf516e77772d279ab5c59`.
- Related inputs are dirty and may change independently, so final parity comes from rerunning the preserved old binary and new binary back-to-back against the same then-current inputs.

---

### Task 1: Lock Existing Document and CLI-Help Behavior

**Files:**
- Create: `document/frontmatter_test.go`
- Create: `document/section_test.go`
- Create: `clihelp/help_test.go`

**Interfaces:**
- Consumes: existing `ParseFrontmatter`, `UpdateManagedSection`, `ValidateManagedSections`, and `HelpGenerator` APIs.
- Produces: characterization coverage protecting unchanged parsing, marker replacement, cancellation, and template rendering.

- [x] **Step 1: Add frontmatter characterization tests**

Cover no frontmatter, valid `saltbox_automation`, unclosed frontmatter, default-enabled sections, explicit disabled sections, hide-list precedence, and example overrides. Use table-driven tests and exact body/error assertions.

```go
func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantBody  string
		wantFM    bool
		wantError string
	}{
		{name: "without frontmatter", content: "# Sonarr\n", wantBody: "# Sonarr\n"},
		{name: "valid", content: "---\nsaltbox_automation:\n  disabled: false\n---\n# Sonarr\n", wantBody: "# Sonarr\n", wantFM: true},
		{name: "unclosed", content: "---\nsaltbox_automation: {}\n", wantError: "unclosed frontmatter"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, body, err := ParseFrontmatter(tt.content)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("error = %v, want substring %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFrontmatter() error = %v", err)
			}
			if body != tt.wantBody {
				t.Fatalf("body = %q, want %q", body, tt.wantBody)
			}
			if (fm != nil) != tt.wantFM {
				t.Fatalf("frontmatter present = %t, want %t", fm != nil, tt.wantFM)
			}
		})
	}
}
```

- [x] **Step 2: Add managed-section characterization tests**

Test exact discovery indexes, replacement content, missing markers, creation newline behavior, and unmatched-marker validation. Assert complete strings for replacement and creation.

- [x] **Step 3: Add CLI-help characterization tests**

Create an executable shell script in `t.TempDir()` that prints deterministic help. Assert `LoadTemplate` plus `Generate(t.Context())` renders that help. Add a canceled-context script case and assert `errors.Is(err, context.Canceled)`.

- [x] **Step 4: Run the characterization tests**

Run `go test -count=1 ./document ./clihelp`.

Expected: PASS. These tests lock existing behavior; no production behavior changes in this task.

- [x] **Step 5: Review checkpoint**

Run `git diff --check` and inspect only the three new test files before continuing.

---

### Task 2: Make Authoritative Source Scanners Fail Closed

**Files:**
- Modify: `parser/inference.go:438`
- Modify: `parser/docker.go:34`
- Modify: `parser/docker_test.go`
- Create: `parser/source_test.go`

**Interfaces:**
- Consumes: existing `ScanInventoryForRoleVarLookups` and `DockerVarScanner.FindDockerVarLookups` APIs.
- Produces: the same return types with strict I/O errors and cache publication only after successful scanning.

- [x] **Step 1: Write failing inventory and Docker-source tests**

```go
func TestScanInventoryForRoleVarLookupsRequiresInventory(t *testing.T) {
	_, err := ScanInventoryForRoleVarLookups(filepath.Join(t.TempDir(), "missing.yml"), nil)
	if err == nil || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("error = %v, want fs.ErrNotExist", err)
	}
}

func TestDockerVarScannerRequiresDockerTasksDirectory(t *testing.T) {
	scanner := NewDockerVarScanner(t.TempDir())
	_, err := scanner.FindDockerVarLookups()
	if err == nil || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("error = %v, want fs.ErrNotExist", err)
	}
}
```

For an individual-file failure, create `resources/tasks/docker/broken.yml` as a broken symlink and assert an error. Then create the symlink target with a valid lookup and call the same scanner again; assert the suffix is returned, proving the failed scan did not cache an empty result.

- [x] **Step 2: Run focused tests to verify RED**

Run `go test -count=1 ./parser -run 'TestScanInventoryForRoleVarLookupsRequiresInventory|TestDockerVarScannerRequiresDockerTasksDirectory|TestDockerVarScannerDoesNotCacheFailedScan'`.

Expected: FAIL because missing sources currently return success and individual read failures are skipped.

- [x] **Step 3: Implement strict scanner semantics**

In `ScanInventoryForRoleVarLookups`, return every `os.Open` error, including `fs.ErrNotExist`.

In `FindDockerVarLookups`, populate a local cache, return `os.ReadDir` and individual file errors, and assign `s.cache = cache` only after the complete scan succeeds. Wrap reads as `fmt.Errorf("reading docker task %s: %w", filePath, err)`. Keep `_docker_var_specs` YAML extraction best-effort because Ansible/Jinja is not guaranteed to be plain YAML.

- [x] **Step 4: Run parser tests to verify GREEN**

Run `go test -count=1 ./parser`.

Expected: PASS.

- [x] **Step 5: Review checkpoint**

Confirm `s.cache` is assigned exactly once, after every directory entry succeeds.

---

### Task 3: Propagate Runner Output Errors

**Files:**
- Modify: `automation/runner.go`
- Modify: `automation/runner_test.go`
- Modify: `automation/index.go`
- Modify: `automation/cli.go`
- Modify: `automation/generate.go`
- Modify: `automation/scaffold.go`
- Modify: `automation/update.go`
- Modify: `automation/validate.go`
- Modify: `cmd/index.go`
- Modify: `cmd/root_test.go`

**Interfaces:**
- Produces:
  - unexported `trackingWriter` retaining the first `Write` error while returning it to `fmt`;
  - `func (r *Runner) result(err error) error` joining operation, stdout, and stderr errors;
  - `func (r *Runner) Index() error`

- [x] **Step 1: Write failing writer tests**

Define a test writer that always returns `io.ErrClosedPipe`. Assert `runner.Index()` returns an error satisfying `errors.Is(err, io.ErrClosedPipe)`. Add an in-memory root-command test whose output writer fails and assert executing `index` returns the same error.

- [x] **Step 2: Run focused tests to verify RED**

Run `go test -count=1 ./automation ./cmd -run 'TestRunnerIndexReturnsOutputError|TestIndexCommandReturnsOutputError'`.

Expected: FAIL to compile because `Index` does not return an error.

- [x] **Step 3: Add first-error tracking writers**

Wrap each supplied writer in `NewRunner`. `trackingWriter.Write` delegates to the underlying writer, stores only the first non-nil error, and returns the original `(n, err)`. Keep `printf`, `errorf`, and `verbosef` concise and void; their writes flow through the trackers.

- [x] **Step 4: Join tracked errors at public boundaries**

Implement `result` with `errors.Join(err, r.out.err, r.errOut.err)`. Give `Generate`, `Update`, `UpdateCLIHelp`, `Scaffold`, and `ValidateFrontmatter` named error returns and a deferred assignment through `result`. Change `Index` to return `r.result(nil)` after writing. The trackers are not cleared; this ensures a nested operation cannot have its output failure demoted to an ordinary warning.

Change the index Cobra handler to `return runner.Index()`.

- [x] **Step 5: Run output and command tests to verify GREEN**

Run `go test -count=1 ./automation ./cmd`.

Expected: PASS.

- [x] **Step 6: Reproduce the original symptom**

Build a temporary binary and run it as `sb-docs index >/dev/full`. Expected: nonzero exit status and a write error on stderr.

---

### Task 4: Make GitHub Issue Commands Context-Aware and Injectable

**Files:**
- Modify: `github/issue.go`
- Modify: `github/issue_comment_test.go`
- Create: `github/issue_manager_test.go`
- Modify: `automation/update.go`

**Interfaces:**
- Produces:
  - unexported `commandRunner` with `LookPath(string) (string, error)` and `Run(context.Context, string, ...string) ([]byte, []byte, error)`;
  - production `execCommandRunner` using `exec.CommandContext`;
  - `NewIssueManager(repo, workflowURL string, out, errOut io.Writer) *IssueManager`;
  - unexported `newIssueManager(repo, workflowURL string, out, errOut io.Writer, commands commandRunner) *IssueManager` for package tests;
  - `ManageIssue(ctx context.Context, result *CheckResult, label string) error`;
  - `OutputGitHubActions(result *CheckResult) error`.

- [x] **Step 1: Write failing context and writer tests**

Create a fake runner with queued stdout/stderr/error results and captured contexts/argv. Add tests that:

- cancel a context before `ManageIssue` and assert `errors.Is(err, context.Canceled)`;
- return `[]` for `gh issue list`, pass a no-issues result, and assert status uses only injected stdout;
- return stderr plus an execution error and assert command context and stderr appear in the returned error;
- use a writer returning `io.ErrClosedPipe` and assert that writer error is returned.

- [x] **Step 2: Run focused tests to verify RED**

Run `go test -count=1 ./github -run TestIssueManager`.

Expected: FAIL to compile because context, writers, and runner injection do not exist.

- [x] **Step 3: Implement the execution boundary**

Add `execCommandRunner`, the test constructor, and one `runGH(ctx, args...)` helper. Preserve stdout for parsing. On failure, trim stderr and return `fmt.Errorf("gh %s: %s: %w", strings.Join(args, " "), stderr, err)` when stderr is non-empty; otherwise wrap the command and error without an empty colon.

- [x] **Step 4: Thread context through every helper**

Update `findExistingIssue`, `getIssueBody`, `createIssue`, `updateIssue`, `closeIssue`, `reopenIssue`, `addComment`, `hasCommentWithBodyHash`, `pinIssue`, and `unpinIssue` to accept and pass `context.Context`.

- [x] **Step 5: Replace process-global output**

Add `printf` and `errorf` methods writing to injected streams and returning errors. Replace every `fmt.Printf` and `fmt.Println`. Return output errors instead of suppressing them. Change `OutputGitHubActions` to return open/write/close errors and remove direct stderr printing.

- [x] **Step 6: Update automation wiring**

Construct the manager with `r.out` and `r.errOut`, then call `ManageIssue(ctx, checkResult, opts.IssueLabel)`. Preserve the current warning policy for GitHub operation errors. Because the supplied writers are Runner trackers, any failed warning or manager output is returned at the Runner's public boundary.

- [x] **Step 7: Run GitHub and automation tests to verify GREEN**

Run `go test -count=1 ./github ./automation`.

Expected: PASS and no test invokes a real `gh` binary.

---

### Task 5: Make Local Quality Gates Reproducible

**Files:**
- Modify: `go.mod`
- Modify if generated: `go.sum`
- Modify: `Makefile`

**Interfaces:**
- Produces Make targets `fmt-check`, `tidy-check`, and `check`, with `GOLANGCI_LINT_VERSION := v2.13.2`.

- [x] **Step 1: Record the current RED dependency check**

Run `go mod tidy -diff`.

Expected: exit 1 showing `github.com/spf13/pflag v1.0.10` moving from indirect to direct.

- [x] **Step 2: Tidy module metadata**

Run `go mod tidy`, inspect the exact module diff, then run `go mod tidy -diff` and `go mod verify`.

Expected: both checks pass.

- [x] **Step 3: Update Makefile targets**

Set these semantics:

```make
GOLANGCI_LINT_VERSION := v2.13.2

fmt-check:
	@test -z "$$(gofmt -l $$(git ls-files '*.go'))" || \
		(gofmt -l $$(git ls-files '*.go'); exit 1)

tidy-check:
	go mod tidy -diff
	go mod verify

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

check: fmt-check tidy-check vet lint test-race
```

Keep `fmt` as the explicit mutating formatter, make `build` depend on non-mutating `check`, correct Cloudplow text to `sb-docs`, and list every declared target in `.PHONY`.

- [x] **Step 4: Run local gates**

Run `make check`.

Expected: PASS without modifying tracked files.

---

### Task 6: Gate Pull Requests and Releases in GitHub Actions

**Files:**
- Modify: `.github/workflows/build.yml`

**Interfaces:**
- Produces a `quality` job on pull requests and pushes and a push-only `build_and_release` job that needs `quality`.

- [x] **Step 1: Add the quality workflow contract**

Use these triggers and top-level permissions:

```yaml
on:
  pull_request:
  push:
    branches: [main]
    tags: ['*']

permissions:
  contents: read
```

Add a `quality` job that checks out full source, uses the Go version from `go.mod`, runs `make check`, runs `go build -trimpath ./...`, and runs `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12`.

- [x] **Step 2: Gate build and release**

Set on `build_and_release`:

```yaml
if: github.event_name == 'push'
needs: quality
permissions:
  contents: write
```

Keep versioning, artifact naming, release notes, artifact upload, and release behavior unchanged.

- [x] **Step 3: Validate workflow syntax**

Run `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12`.

Expected: no output and exit 0.

- [x] **Step 4: Review event behavior**

Confirm pull requests execute only `quality`; main pushes run quality then upload an artifact; tag pushes run quality then publish the existing release artifact.

---

### Task 7: Load Sources Once and Make Rendering Pure

**Files:**
- Create: `automation/sources.go`
- Create: `automation/sources_test.go`
- Modify: `automation/generate.go`
- Modify: `automation/update.go`
- Modify: `render/data.go`
- Modify: `render/data_test.go`

**Interfaces:**
- Consumes: strict scanners from Task 2.
- Produces:
  - `render.SourceCatalog{RoleVarLookups map[string]string, DockerVarSuffixes []string}`
  - `automation.loadSourceCatalog(cfg *config.Config) (render.SourceCatalog, error)`
  - `render.BuildRoleData(role *parser.RoleInfo, cfg *config.Config, fmConfig *document.SaltboxAutomationConfig, sources render.SourceCatalog) *render.RoleData`

- [x] **Step 1: Write failing source-loader tests**

Create a minimal repository layout under `t.TempDir()` with `inventories/group_vars/all.yml` and `resources/tasks/docker/*.yml`. Assert the loader returns both an inventory lookup and normalized Docker suffix. Add missing-inventory and missing-Docker-directory cases; assert contextual messages contain `scanning inventory` or `scanning docker variables` while preserving `fs.ErrNotExist`.

```go
sources, err := loadSourceCatalog(cfg)
if err != nil {
	t.Fatalf("loadSourceCatalog() error = %v", err)
}
if got := sources.RoleVarLookups["_web_host_override"]; got != parser.String {
	t.Fatalf("role lookup type = %q, want %q", got, parser.String)
}
if !slices.Contains(sources.DockerVarSuffixes, "envs") {
	t.Fatalf("DockerVarSuffixes = %v, want envs", sources.DockerVarSuffixes)
}
```

- [x] **Step 2: Run loader tests to verify RED**

Run `go test -count=1 ./automation -run TestLoadSourceCatalog`.

Expected: FAIL to compile because the loader and catalog do not exist.

- [x] **Step 3: Add the catalog and loader**

Define the catalog beside render data types. Implement the automation loader by calling `ScanInventoryForRoleVarLookups` and one `DockerVarScanner.FindDockerVarLookups` scan against `<saltbox>/resources`. Keep suffixes sorted.

- [x] **Step 4: Convert render tests to explicit inputs**

Update every `BuildRoleData` call. Tests needing Docker+ content pass exact suffixes; tests needing inventory overrides pass exact lookups; unrelated tests pass `render.SourceCatalog{}`. Add a regression case where `cfg.Repositories.Saltbox` is missing but a non-empty explicit catalog is supplied; rendering must still succeed.

- [x] **Step 5: Run render tests to verify RED at the old seam**

Run `go test -count=1 ./render`.

Expected: FAIL until `BuildRoleData` accepts and uses the catalog.

- [x] **Step 6: Remove discovery from rendering**

Use `sources.RoleVarLookups` directly. Change `buildDockerInfo` to filter `sources.DockerVarSuffixes` against configured ignores and role-defined Docker suffixes before categorization. Remove scanner calls from `render`.

- [x] **Step 7: Load once at operation boundaries**

`Runner.Generate` and `Runner.Update` load immediately after the context check and pass the catalog through all helpers. Use these signatures consistently:

```go
func (r *Runner) generateRole(ctx context.Context, cfg *config.Config, sources render.SourceCatalog, roleName string) error
func (r *Runner) generateAllRoles(ctx context.Context, cfg *config.Config, sources render.SourceCatalog, includeCLI bool) error
func (r *Runner) updateRole(ctx context.Context, cfg *config.Config, sources render.SourceCatalog, roleName string) error
func (r *Runner) updateRoleWithResult(ctx context.Context, cfg *config.Config, sources render.SourceCatalog, roleName, repoType string) github.RoleResult
```

- [x] **Step 8: Run affected packages to verify GREEN**

Run `go test -count=1 ./parser ./render ./automation`.

Expected: PASS.

- [x] **Step 9: Verify immediate output parity**

Build `/tmp/docs-automation-after-sources-sb-docs`, run old and new binaries back-to-back for `sonarr` and all roles, and compare both stdout and stderr with `cmp`. Expected: all comparisons return 0.

---

### Task 8: Publish Documents Atomically

**Files:**
- Create: `document/atomic.go`
- Create: `document/atomic_test.go`
- Create: `document/manager_test.go`
- Create: `automation/scaffold_test.go`
- Modify: `document/manager.go`
- Modify: `automation/scaffold.go`

**Interfaces:**
- Produces: `document.WriteFileAtomic(path string, data []byte, perm fs.FileMode, overwrite bool) error`.

- [x] **Step 1: Write failing atomic-publication tests**

Test real filesystem behavior under `t.TempDir()`:

```go
func TestWriteFileAtomicRefusesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := WriteFileAtomic(path, []byte("new"), 0o644, false)
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("error = %v, want fs.ErrExist", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "old" {
		t.Fatalf("content = %q, error = %v, want old", got, err)
	}
}

func TestWriteFileAtomicReplacesCompleteRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.md")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("new"), 0o644, true); err != nil {
		t.Fatalf("WriteFileAtomic() error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "new" {
		t.Fatalf("content = %q, error = %v", got, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestWriteFileAtomicRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	link := filepath.Join(dir, "app.md")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(link, []byte("new"), 0o644, true); err == nil {
		t.Fatal("WriteFileAtomic() error = nil, want non-regular destination error")
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "old" {
		t.Fatalf("target content = %q, error = %v, want old", got, err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link mode = %v, want symlink", info.Mode())
	}
}
```

Assert no temporary files remain after success or refusal.

- [x] **Step 2: Run atomic tests to verify RED**

Run `go test -count=1 ./document -run TestWriteFileAtomic`.

Expected: FAIL to compile because `WriteFileAtomic` does not exist.

- [x] **Step 3: Implement atomic publication**

Use `os.Lstat`, `os.CreateTemp` in the destination directory, `Chmod`, `Write`, `Sync`, and `Close`. Use `os.Rename` for replacement and `os.Link` for no-clobber publication. Remove the temporary name and sync an opened parent directory. Refuse non-regular destinations and wrap every failure with path and operation context.

- [x] **Step 4: Route Manager saves through the publisher**

Add a manager test saving changed content to a `0600` file and asserting bytes and mode. Change `SaveDocument` to call `WriteFileAtomic(doc.Path, []byte(doc.Content), 0o644, true)`.

- [x] **Step 5: Write the failing scaffold preservation test**

Create valid role roots, an existing output containing `original\n`, and a template containing `{{call .RoleName}}`, which parses but fails during execution. Call `Scaffold` with `Force: true`; assert the error contains `executing template` and the destination remains `original\n`.

- [x] **Step 6: Run scaffold test to verify RED**

Run `go test -count=1 ./automation -run TestScaffoldPreservesExistingFileWhenTemplateExecutionFails`.

Expected: FAIL because `os.Create` truncates the destination.

- [x] **Step 7: Render before publishing**

Execute into `bytes.Buffer`, recheck `ctx.Err()`, create the parent directory, and call `document.WriteFileAtomic(outputPath, buf.Bytes(), 0o644, opts.Force)`. Translate `fs.ErrExist` to the current friendly overwrite message.

- [x] **Step 8: Run document and scaffold tests to verify GREEN**

Run `go test -count=1 ./document ./automation`.

Expected: PASS.

---

### Task 9: Full Acceptance and Same-Input A/B Validation

**Files:**
- Verify all intended modified and new files.
- Do not alter related repositories.

**Interfaces:**
- Consumes the complete implementation.
- Produces final evidence only; no commit or external mutation.

- [x] **Step 1: Format and inspect scope**

Run `gofmt -w` on the exact changed Go files, then `git diff --check`, `git status --short`, and `git diff --stat`. Confirm `.agents`, Go source/tests, `go.mod`/`go.sum`, Makefile, and workflow are the only paths in scope.

- [x] **Step 2: Run fresh full tests and static gates**

Run each command separately:

```bash
go mod tidy -diff
go mod verify
go vet ./...
go test -count=1 -race ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2 run ./...
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
make build
```

Expected: every command exits 0 and lint tools report no issues.

- [x] **Step 3: Run local integration**

Run the newly built binary with `--config config.yml validate config`. Expected: `✅ Config is valid` and exit 0.

- [x] **Step 4: Compare Sonarr output on identical inputs**

Run `/tmp/docs-automation-before-sb-docs` and the new binary back-to-back into separate `/tmp` stdout/stderr files, then `cmp` both streams. Expected: both comparisons return 0.

- [x] **Step 5: Compare all-role output on identical inputs**

Repeat without a role argument. Expected: stdout and stderr comparisons return 0. If a related worktree changes between runs, rerun both binaries; do not edit or reset that worktree.

- [x] **Step 6: Verify original regressions directly**

Run the new binary with `index >/dev/full` and assert a nonzero status. Run focused tests for missing discovery, scaffold preservation, atomic publication, and canceled GitHub commands with `-count=1`.

- [x] **Step 7: Final worktree audit**

Run `git status --short --branch`, inspect every diff, and report that no commits, pushes, tags, releases, live GitHub issue calls, related-repository writes, or VM operations occurred.
