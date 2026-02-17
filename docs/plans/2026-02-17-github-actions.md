# GitHub Actions CI/CD Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add CI (test on PR) and Release (build binaries on tag) workflows to the Grove CLI.

**Architecture:** Two GitHub Actions workflows. `ci.yml` runs `go vet`, `go build`, and `go test` on both Linux and macOS runners in parallel. `release.yml` triggers on `v*` tags, runs tests on macOS, then uses GoReleaser to build darwin/amd64 + darwin/arm64 binaries and publish a GitHub Release.

**Tech Stack:** GitHub Actions, `actions/checkout`, `actions/setup-go`, `goreleaser/goreleaser-action`, GoReleaser config.

---

### Task 1: CI Workflow

**Files:**
- Create: `.github/workflows/ci.yml`

**Step 1: Create the CI workflow**

Create `.github/workflows/ci.yml`:

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test-linux:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.25.x"

      - name: Vet
        run: go vet ./...

      - name: Build
        run: go build ./cmd/grove

      - name: Test
        run: go test ./... -count=1

  test-macos:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.25.x"

      - name: Vet
        run: go vet ./...

      - name: Build
        run: go build ./cmd/grove

      - name: Test
        run: go test ./... -count=1
```

**Step 2: Verify syntax**

Run:
```bash
cat .github/workflows/ci.yml | head -5
```

Expected: The file exists and starts with `name: CI`.

**Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add CI workflow with Linux and macOS test jobs"
```

---

### Task 2: GoReleaser Config

**Files:**
- Create: `.goreleaser.yml`

**Step 1: Create the GoReleaser config**

Create `.goreleaser.yml`:

```yaml
version: 2

builds:
  - main: ./cmd/grove
    binary: grove
    env:
      - CGO_ENABLED=0
    goos:
      - darwin
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w -X main.version={{.Version}}

archives:
  - format: tar.gz
    name_template: "grove_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    files:
      - LICENSE
      - README.md

checksum:
  name_template: "checksums.txt"

changelog:
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^chore:"
```

**Step 2: Verify syntax**

Run:
```bash
cat .goreleaser.yml | head -3
```

Expected: File exists and starts with `version: 2`.

**Step 3: Add `.goreleaser.yml` to `.gitignore` dist output**

GoReleaser creates a `dist/` directory when run locally. Add it to `.gitignore`.

Append to `.gitignore`:

```
# GoReleaser output
dist/
```

**Step 4: Commit**

```bash
git add .goreleaser.yml .gitignore
git commit -m "ci: add GoReleaser config for macOS binaries"
```

---

### Task 3: Release Workflow

**Files:**
- Create: `.github/workflows/release.yml`

**Step 1: Create the release workflow**

Create `.github/workflows/release.yml`:

```yaml
name: Release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write

jobs:
  release:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: "1.25.x"

      - name: Test
        run: go test ./... -count=1

      - uses: goreleaser/goreleaser-action@v6
        with:
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

**Step 2: Verify syntax**

Run:
```bash
cat .github/workflows/release.yml | head -5
```

Expected: File exists and starts with `name: Release`.

**Step 3: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add release workflow with GoReleaser"
```

---

### Task 4: Verify Everything

**Step 1: Check all files are in place**

Run:
```bash
ls -la .github/workflows/ci.yml .github/workflows/release.yml .goreleaser.yml
```

Expected: All three files exist.

**Step 2: Verify the project still builds and tests pass**

Run:
```bash
go vet ./... && go build ./cmd/grove && go test ./... -count=1
```

Expected: Clean vet, successful build, all tests pass.

**Step 3: Clean up build artifact**

Run:
```bash
rm -f grove
```
