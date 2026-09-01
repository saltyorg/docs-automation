# Stack Context

Generated: 2026-09-01

## Stack
- **Language**: Go 1.27.0
- **Framework**: Cobra 1.10.2 CLI; yaml.v3 configuration and document parsing
- **Build**: Go modules, `go build`; Makefile wrapper
- **Test**: standard `testing`; `go test ./...` and `go test -race ./...`
- **Lint**: golangci-lint v2 via unpinned `@latest` Make target [CI gate: no]
- **Format**: gofmt/`go fmt ./...` via Make target [CI gate: no]

## Secondary Languages
- YAML (GitHub Actions workflow)
- Make (local build and quality targets)
- Markdown (maintainer contract)

## Conventions
- Error handling: explicit returns, contextual `%w` wrapping at boundaries
- Module structure: top-level domain packages; `main` wires the `cmd` package
- Naming: idiomatic Go package names and exported PascalCase identifiers
- Tests: co-located `_test.go` files, mostly table-driven with standard library helpers
- Guidance: README is the maintainer contract; no tracked AGENTS.md, CLAUDE.md, or CONTRIBUTING.md

## CI Gates
- Pushes to `main` and tags build one Linux/amd64 binary
- No pull-request trigger, formatting, vet, lint, unit-test, race, or module-verification gate
- Tag builds publish a GitHub release artifact
