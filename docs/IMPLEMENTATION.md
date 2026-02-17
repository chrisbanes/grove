# Grove — Implementation Plan

Technical implementation plan for the Grove CLI, ordered by dependency
and build sequence. Each milestone produces a working, testable artifact.

## Dependencies & Tooling

```
go 1.22+
github.com/spf13/cobra       # CLI framework
```

No other external dependencies to start. We can add libraries as needed,
but the goal is to keep the dependency tree minimal — most of what grove
does is `os/exec` and `os` filesystem operations.

## Milestone 1: Project Scaffold + CoW Clone

**Goal**: `go build` produces a binary. The core CoW clone mechanism works.

### 1.1 Go module init

```
go mod init github.com/AmpInc/grove
```

### 1.2 `internal/clone` — CoW clone abstraction

This is the foundation everything else builds on.

**`clone.go`** — Interface:
```go
type Cloner interface {
    // Clone performs a CoW clone from src to dst.
    // Returns an error if the filesystem doesn't support CoW.
    Clone(src, dst string) error

    // Supported checks whether CoW cloning is available
    // on the filesystem containing the given path.
    Supported(path string) (bool, error)
}
```

**`apfs.go`** — macOS implementation:
- `Clone`: exec `cp -c -R src dst`
- `Supported`: exec `diskutil info <mount_point>` and check for
  "File System Personality: APFS" in output
- Need to resolve the mount point for a given path (use `stat` or
  `df` to find which volume a path is on)

**`reflink.go`** — Linux implementation:
- `Clone`: exec `cp --reflink=always -R src dst`
- `Supported`: attempt a test reflink clone of a temp file. If
  `cp --reflink=always` succeeds, the filesystem supports it. Clean up
  the test file.

**`detect.go`** — Auto-detection:
```go
// NewCloner returns the appropriate Cloner for the current platform
// and the filesystem at the given path. Returns an error if CoW
// is not supported.
func NewCloner(path string) (Cloner, error)
```
- On darwin: try APFS
- On linux: try reflink
- Otherwise: return error

**Tests**:
- `clone_test.go`: integration test that clones a temp directory and
  verifies all files are present in the destination. Verifies that
  modifying a file in the clone doesn't affect the source (CoW
  correctness).
- `detect_test.go`: test that `NewCloner` returns the right
  implementation for the current platform (or an error on unsupported).

### 1.3 Minimal `cmd/grove/main.go`

Cobra root command with version flag. No subcommands yet — just enough
to verify the build works.

```bash
grove --version
# grove v0.1.0
```

**Deliverable**: `go build ./cmd/grove` produces a working binary.
Clone package has passing tests.

---

## Milestone 2: Config + Init

**Goal**: `grove init` creates `.grove/` directory with config.

### 2.1 `internal/config`

**`config.go`**:
```go
type Config struct {
    WarmupCommand string `json:"warmup_command,omitempty"`
    WorkspaceDir  string `json:"workspace_dir"`
    MaxWorkspaces int    `json:"max_workspaces"`
}
```

Functions:
- `Load(repoRoot string) (*Config, error)` — read `.grove/config.json`,
  apply defaults for missing fields
- `Save(repoRoot string, cfg *Config) error` — write `.grove/config.json`
- `DefaultConfig(projectName string) *Config` — returns config with
  sensible defaults (`/tmp/grove/{project}`, max 10)
- `FindRepoRoot(startPath string) (string, error)` — walk up from
  `startPath` looking for `.grove/` directory. Falls back to
  `git rev-parse --show-toplevel` to find the git root.

Default resolution for `{project}` template variable in `workspace_dir`:
use the basename of the repo root directory.

**Tests**:
- Load/save round-trip
- Default values applied for missing fields
- `{project}` template expansion

### 2.2 `internal/git`

**`git.go`**:
- `IsRepo(path string) bool` — check if path is a git repo
- `IsDirty(path string) (bool, error)` — `git status --porcelain`
- `CurrentBranch(path string) (string, error)` — `git branch --show-current`
- `CurrentCommit(path string) (string, error)` — `git rev-parse --short HEAD`
- `Pull(path string) error` — `git pull`
- `Checkout(path, branch string, create bool) error` — `git checkout [-b] <branch>`

All functions take a `path` argument and run git with `-C <path>`.

**Tests**: unit tests against a temp git repo created in test setup.

### 2.3 `grove init` command

Wire up cobra subcommand:
```
grove init [path] [--warmup-command CMD] [--workspace-dir DIR] [--force]
```

Steps:
1. Resolve path (default: cwd)
2. Verify it's a git repo (`git.IsRepo`)
3. Check for uncommitted changes (`git.IsDirty`), error unless `--force`
4. Create `.grove/` directory
5. Write default config (override with flags if provided)
6. If `--warmup-command` is set, run it
7. Print success message

**Tests**: integration test that inits a temp repo and verifies
`.grove/config.json` exists with expected content.

**Deliverable**: `grove init` works end-to-end. Config is persisted.

---

## Milestone 3: Create + Destroy Workspaces

**Goal**: `grove create` and `grove destroy` work. This is the core
workflow.

### 3.1 `internal/workspace`

**`workspace.go`**:

```go
type WorkspaceInfo struct {
    ID          string    `json:"id"`
    GoldenCopy  string    `json:"golden_copy"`
    GoldenCommit string   `json:"golden_commit"`
    CreatedAt   time.Time `json:"created_at"`
    Branch      string    `json:"branch"`
    Path        string    `json:"path"`
}
```

Functions:
- `Create(cfg *Config, cloner clone.Cloner, opts CreateOpts) (*WorkspaceInfo, error)`
  1. Check max workspace limit
  2. Generate workspace ID (short random hex, e.g., 8 chars)
  3. Compute destination path: `cfg.WorkspaceDir/id`
  4. Call `cloner.Clone(goldenRoot, destPath)`
  5. Write `.grove/workspace.json` in the workspace
  6. Return workspace info

- `List(cfg *Config) ([]WorkspaceInfo, error)`
  1. Scan `cfg.WorkspaceDir` for subdirectories
  2. For each, try to read `.grove/workspace.json`
  3. Return list of valid workspaces

- `Destroy(cfg *Config, idOrPath string) error`
  1. Resolve workspace (by ID or path)
  2. `os.RemoveAll` the workspace directory
  3. Return success

- `Get(cfg *Config, idOrPath string) (*WorkspaceInfo, error)`
  1. Resolve workspace (by ID or path)
  2. Read and return its marker file

- `IsWorkspace(path string) bool` — check for `.grove/workspace.json`

ID generation: `crypto/rand` + hex encoding, 8 characters. Collision
probability is negligible at max 10 workspaces.

### 3.2 `internal/hooks`

**`hooks.go`**:
- `Run(repoRoot, hookName string) error`
  1. Check if `.grove/hooks/<hookName>` exists
  2. Verify it's executable
  3. Run it with `os/exec`, working directory set to `repoRoot`
  4. Stream stdout/stderr to caller
  5. Return error if exit code != 0
  6. If hook doesn't exist, silently succeed (hooks are optional)

### 3.3 `grove create` command

```
grove create [--branch NAME] [--force] [--json]
```

Steps:
1. Find repo root (walk up to `.grove/`)
2. Load config
3. Verify golden copy (not dirty, or `--force`)
4. Verify CoW support (`clone.NewCloner`)
5. Call `workspace.Create`
6. Run `hooks.Run(workspacePath, "post-clone")`
7. If `--branch`: `git.Checkout(workspacePath, branch, true)`
8. Output result (text or JSON)

Error handling:
- If clone fails (disk full, etc.), clean up partial directory
- If hook fails, clean up workspace and report hook error
- If branch checkout fails, leave workspace (clone succeeded, user can fix)

### 3.4 `grove destroy` command

```
grove destroy <id|path> [--all] [--push]
```

Steps:
1. If `--all`: list all workspaces, destroy each
2. Otherwise: resolve workspace by ID or path
3. If `--push`: `git push -u origin <branch>` from the workspace
4. Call `workspace.Destroy`
5. Print confirmation

### 3.5 Integration test

End-to-end test that:
1. Creates a temp git repo with some files
2. Runs `grove init`
3. Runs `grove create --branch test-branch`
4. Verifies workspace exists with all files
5. Verifies `.grove/workspace.json` has correct metadata
6. Modifies a file in the workspace, verifies golden copy unchanged
7. Runs `grove destroy`
8. Verifies workspace directory is gone

**Deliverable**: full create/destroy lifecycle works. Post-clone hooks
execute. JSON output works.

---

## Milestone 4: List + Status

**Goal**: `grove list` and `grove status` provide visibility.

### 4.1 `grove list` command

```
grove list [--json]
```

Text output:
```
ID        BRANCH                  CREATED      PATH
a1b2c3d4  agent/fix-login         2m ago       /tmp/grove/myapp/a1b2c3d4
e5f6g7h8  agent/add-feature       15m ago      /tmp/grove/myapp/e5f6g7h8
```

JSON output: array of `WorkspaceInfo` objects.

Uses `workspace.List` internally.

### 4.2 `grove status` command

```
grove status
```

Shows:
- Golden copy path and current branch/commit
- Number of active workspaces
- Whether golden copy has uncommitted changes
- Config summary (workspace dir, max workspaces)

```
Golden copy: /Users/chris/dev/myapp
Branch:      main
Commit:      a1b2c3d (2 hours ago)
Status:      clean

Workspaces:  2 / 10 (max)
Directory:   /tmp/grove/myapp
```

**Deliverable**: full visibility into grove state.

---

## Milestone 5: Update

**Goal**: `grove update` convenience command works.

### 5.1 `grove update` command

```
grove update
```

Steps:
1. Find repo root
2. Load config
3. `git.Pull(repoRoot)`
4. If `cfg.WarmupCommand` is set, run it via `os/exec`
5. Print summary (new commit, build output)

This is intentionally simple — just a convenience wrapper.

**Deliverable**: golden copy can be refreshed with one command.

---

## Milestone 6: Polish

**Goal**: production-quality CLI.

### 6.1 Error messages

Review all error paths and ensure messages are clear and actionable.
Include the "what happened", "why", and "what to do" pattern:

```
Error: Cannot create workspace — golden copy has uncommitted changes.
Uncommitted changes would be included in the clone.
Run `grove create --force` to proceed anyway, or commit/stash changes first.
```

### 6.2 `.gitignore` handling

`grove init` should:
- Add `.grove/workspace.json` to `.gitignore` if not already present
- Add `.grove/hooks/` to `.gitignore` only if the user wants hooks
  to be private (default: don't ignore, so hooks are shared)

Actually — reconsider: `.grove/workspace.json` only exists in workspaces,
not in the golden copy. The golden copy is what gets committed. So
nothing needs to be gitignored by default, since the golden copy
won't have a `workspace.json`.

### 6.3 Help text

Ensure all commands have useful `--help` output with examples:

```
$ grove create --help
Create a new workspace from the golden copy.

The workspace is a copy-on-write clone of the current repository,
including all build caches and gitignored files. Builds in the
workspace start warm.

Usage:
  grove create [flags]

Flags:
  --branch string   Branch name to create/checkout in the workspace
  --force           Proceed even if golden copy has uncommitted changes
  --json            Output workspace info as JSON

Examples:
  grove create --branch feature/auth
  grove create --branch fix/login --json
```

### 6.4 Cross-compilation CI

GitHub Actions workflow:
- Build for `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`
- Run tests on macOS and Linux runners
- Attach binaries to GitHub releases

### 6.5 Homebrew formula (future)

Stretch goal. Create a Homebrew tap for `brew install grove`.

**Deliverable**: polished, well-documented CLI ready for users.

---

## Implementation Order Summary

```
M1: Scaffold + Clone   ← foundation, no dependencies
M2: Config + Init       ← depends on M1 (needs clone for detection)
M3: Create + Destroy    ← depends on M1 + M2 (core workflow)
M4: List + Status       ← depends on M3 (reads workspace state)
M5: Update              ← independent, simple
M6: Polish              ← after everything works
```

Milestones 1-3 are the critical path. Once those work, grove is
usable. Milestones 4-6 add quality and convenience.

## Testing Strategy

**Unit tests**: Each `internal/` package has its own tests. Mock
filesystem operations where practical, but prefer real temp directories
for filesystem-dependent code (CoW, config I/O).

**Integration tests**: End-to-end tests in a `test/` directory that
exercise the full CLI binary. Use `os/exec` to run the built `grove`
binary against temp repos.

**Platform tests**: CI runs on both macOS and Linux. The clone package's
tests verify CoW behavior on each platform's native filesystem.

**What NOT to test**: Don't test cobra wiring or flag parsing — that's
cobra's job. Test the business logic in internal packages.
