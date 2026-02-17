# GitHub Actions CI/CD Design

## Overview

Two workflows: CI for pull requests, Release for version tags.

## Workflow 1: `ci.yml`

**Triggers:** Push to `main`, pull requests targeting `main`.

**Jobs (parallel):**

| Job | Runner | Purpose |
|-----|--------|---------|
| `test-linux` | `ubuntu-latest` | Fast, cheap feedback on platform-agnostic code. CoW/E2E tests auto-skip via `t.Skip`. |
| `test-macos` | `macos-latest` | Full test suite including APFS CoW clone and E2E integration tests. |

**Steps (both jobs):**

1. Checkout code
2. Setup Go 1.25.x (`actions/setup-go` with built-in module caching)
3. `go vet ./...`
4. `go build ./cmd/grove`
5. `go test ./... -count=1`

Both jobs required to pass before merging.

## Workflow 2: `release.yml`

**Trigger:** Push of tags matching `v*` (e.g., `v0.1.0`).

**Single job on `macos-latest`:**

1. Checkout code
2. Setup Go 1.25.x
3. Run full test suite (`go test ./... -count=1`)
4. Run GoReleaser (`goreleaser/goreleaser-action`)

## GoReleaser Config (`.goreleaser.yml`)

- Binary name: `grove`
- Targets: `darwin_amd64`, `darwin_arm64` (macOS only — Linux has no CoW support yet)
- Archives: tar.gz with LICENSE and README
- Checksum file (sha256)
- GitHub Release with auto-generated changelog

## Decisions

- **Go version:** Pin to `1.25.x` to match `go.mod`.
- **Caching:** `actions/setup-go` handles module caching automatically.
- **macOS-only binaries:** Grove requires APFS (or Btrfs/XFS reflink on Linux, not yet implemented). Add Linux targets when reflink support lands.
- **No Makefile:** The workflow calls `go` commands directly. No need for a build wrapper at this stage.
