# Contributing to Grove

Thank you for your interest in contributing to Grove! This document covers the development workflow, from setting up your environment to submitting a pull request.

## Prerequisites

- **Go 1.25.0** -- Grove uses the version specified in `.go-version`. Install via your preferred method ([golang.org/dl](https://golang.org/dl/) or a version manager).
- **macOS with APFS** -- Required for running the full test suite. Most tests use APFS-specific CoW operations and are skipped on other platforms.
- **Git** -- For repository operations.

## Getting the Source

```bash
git clone https://github.com/chrisbanes/grove.git
cd grove
```

## Building

```bash
go build ./cmd/grove
```

This produces a `grove` binary in the current directory. The version will show as `dev` unless you pass ldflags:

```bash
go build -ldflags "-X main.version=local" ./cmd/grove
```

## Running Tests

```bash
# All tests (unit + e2e)
go test ./... -count=1

# Unit tests only
go test ./internal/... -count=1

# E2e tests only (requires macOS/APFS)
go test ./test/... -count=1 -v
```

The `-count=1` flag disables test caching, which is important for e2e tests that create real filesystem clones. Tests requiring APFS are automatically skipped on non-macOS platforms.

## Code Quality

```bash
go vet ./...
```

CI runs `go vet` on both Linux and macOS. All code must pass before merge.

## Project Layout

| Package | Purpose |
|---------|---------|
| `cmd/grove/` | Cobra CLI commands. Each command is in its own file. Commands are thin wrappers that delegate to `internal/` packages. |
| `internal/config/` | Configuration loading, saving, and `.grove/` directory discovery. |
| `internal/workspace/` | Workspace lifecycle: create, list, destroy, get. |
| `internal/clone/` | Platform-abstracted CoW cloning. `Cloner` interface with `APFSCloner` implementation and filesystem detection. |
| `internal/hooks/` | Hook discovery and execution. |
| `internal/git/` | Thin wrapper around git CLI operations. |
| `test/` | End-to-end tests that build the binary and exercise the full CLI. |
| `docs/` | Design document and implementation plans. |

## Making Changes

1. Fork the repository and create a branch from `main`.
2. Make your changes. Follow existing code patterns -- the codebase is intentionally simple with minimal dependencies.
3. Add or update tests. E2e tests go in `test/e2e_test.go`. Unit tests go next to the source files (`*_test.go`).
4. Run `go vet ./...` and `go test ./... -count=1` locally.
5. Commit with a descriptive message following conventional commit style: `feat:`, `fix:`, `test:`, `docs:`, `chore:`.

## Submitting a Pull Request

- Open a PR against `main`.
- Fill out the PR template (what changed, why, how you tested).
- CI runs on both Linux and macOS. Both must pass.
- Keep PRs focused -- one feature or fix per PR.

## Code Style

- Follow standard Go conventions (`gofmt`).
- Error messages: lowercase, no trailing punctuation (Go convention).
- CLI output: `fmt.Printf` for stdout, `fmt.Fprintf(os.Stderr, ...)` for warnings.
- Keep dependencies minimal. The only external dependency is [cobra](https://github.com/spf13/cobra) -- avoid adding new dependencies unless absolutely necessary.

## Reporting Issues

Use the [bug report](https://github.com/chrisbanes/grove/issues/new?template=bug_report.yml) or [feature request](https://github.com/chrisbanes/grove/issues/new?template=feature_request.yml) templates.
