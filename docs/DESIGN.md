# Grove — Design Document

## Background

Multi-agent AI workflows need multiple concurrent working copies of the same
git repository, each agent working on a different task/branch simultaneously.

The standard approaches (`git worktree`, `git clone --shared`, full clones)
all create working trees from scratch. While git objects can be shared, the
**gitignored local state** — build caches, compiled outputs, dependency
metadata — is lost every time. For build systems like Gradle, this means
every new worktree starts with a cold build that can take minutes.

## Goal

Provide a CLI tool that manages short-lived, isolated workspaces cloned from
a "golden" copy of a repository that includes warm build state. Each workspace
gets the full build cache via copy-on-write filesystem clones, so builds are
incremental from the start.

## Core Concepts

### Golden Copy

A maintained checkout of the repository with warm build state:

- Checked out on a base branch (typically `main`)
- Has been built recently (`./gradlew assemble` or equivalent)
- Contains all gitignored build artifacts (`.gradle/`, `build/`, etc.)
- One golden copy per registered repo (multiple base branches not in scope for v1)

### Workspace

A CoW filesystem clone of the golden copy:

- Created near-instantly via `cp -c -R` (macOS/APFS) or `cp --reflink=always` (Linux/Btrfs)
- Fully isolated — each agent gets its own working tree
- Short-lived: created for a task, destroyed when done
- Inherits warm build state, so incremental builds work immediately
- Checked out on its own branch
- Max 10 concurrent workspaces (configurable)

### Why not git worktree?

`git worktree` shares the `.git` directory but creates a fresh working tree.
Gitignored files (build caches, compiled outputs) are not copied. Grove
clones the *entire* directory including gitignored state, giving each
workspace a warm build cache from the start.

## CLI Design

```
grove init [path]             Initialize a golden copy from an existing repo
grove update                  Pull latest + rebuild the golden copy (convenience)
grove create [--branch NAME]  Create a new workspace from the golden copy
grove list                    List active workspaces
grove destroy <id|path>       Remove a workspace
grove destroy --all           Remove all workspaces
grove status                  Show golden copy info and workspace summary
```

### `grove init [path]`

Registers an existing git repo as a grove-managed golden copy.

- `path` defaults to current directory
- Creates a `.grove/` directory in the repo root to store config
- Records the repo path in the global grove config (`~/.grove/config.json`)
- Optionally runs an initial warm-up build (configurable command)
- **Error**: if golden copy has uncommitted changes, warn and require `--force`

Config stored in `.grove/config.json`:
```json
{
  "warmup_command": "./gradlew assemble",
  "branch": "main",
  "workspace_dir": "/tmp/grove/{project}",
  "max_workspaces": 10
}
```

### `grove update`

A convenience command to refresh the golden copy. Not a core workflow
requirement — workspaces are short-lived, so staleness is managed by
the user re-running `update` when they choose to.

1. `git pull` on the configured base branch
2. Run the configured warmup command
3. Run post-update hooks

### `grove create [--branch NAME]`

Creates a new workspace:

1. Verify filesystem supports CoW (error out if not — see [Error Handling](#error-handling))
2. Check golden copy for uncommitted changes (warn + require `--force`)
3. CoW clone golden copy to workspace directory
4. Write workspace marker file (`.grove-workspace.json`)
5. Run post-clone hooks (`.grove/hooks/post-clone`)
6. Check out the specified branch (or create one)
7. Output workspace path (plain text default, `--json` for machine-readable)

JSON output mode (for programmatic consumers):
```json
{
  "id": "abc1",
  "path": "/tmp/grove/myapp/abc1",
  "branch": "agent/fix-login",
  "created_at": "2026-02-17T10:20:00Z",
  "golden_copy": "/Users/chris/dev/myapp"
}
```

### `grove list`

Shows active workspaces by scanning the workspace directory and reading
marker files:
```
ID     BRANCH                  CREATED      PATH
abc1   agent/fix-login         2m ago       /tmp/grove/myapp/abc1
def2   agent/add-feature       15m ago      /tmp/grove/myapp/def2
```

Supports `--json` for machine-readable output.

### `grove destroy <id|path>`

Removes a workspace via `rm -rf`. Optionally pushes the branch first
with `--push`.

## Metadata & State

### Global Config (`~/.grove/config.json`)

Registry of golden copies and global defaults:

```json
{
  "golden_copies": {
    "/Users/chris/dev/myapp": {
      "project": "myapp",
      "workspace_dir": "/tmp/grove/myapp"
    }
  }
}
```

### Project Config (`.grove/config.json`)

Lives in the golden copy repo root. Project-specific settings:

```json
{
  "warmup_command": "./gradlew assemble",
  "branch": "main",
  "workspace_dir": "/tmp/grove/{project}",
  "max_workspaces": 10
}
```

Should be committed to the repo so all users share the same config.

### Workspace Marker (`.grove-workspace.json`)

Written into each workspace after cloning. Enables `grove list` to
discover workspaces and track provenance:

```json
{
  "id": "abc1",
  "golden_copy": "/Users/chris/dev/myapp",
  "golden_commit": "3329245",
  "created_at": "2026-02-17T10:20:00Z",
  "branch": "agent/fix-login"
}
```

This file should be in `.gitignore` so it's never committed.

### Design Principle: No Daemon

Grove is stateless between invocations. All metadata is derived from:
- Global config file (golden copy registry)
- Project config file (per-repo settings)
- Workspace marker files (per-workspace provenance)
- Filesystem state (directory existence, git branch)

## Hooks

Grove supports a `.grove/hooks/` directory in the golden copy repo,
similar to git hooks. These are executable scripts that run at specific
lifecycle points.

### `post-clone`

Runs inside each workspace immediately after the CoW clone completes,
before branch checkout. This is where you clean up non-relocatable state.

Example for a Gradle project (`.grove/hooks/post-clone`):
```bash
#!/bin/bash
# Clean Gradle lock files and non-relocatable caches
find . -name "*.lock" -path "*/.gradle/*" -delete
rm -rf .gradle/configuration-cache
rm -rf .gradle/file-system.probe
```

Example for a generic project:
```bash
#!/bin/bash
# Clean Python bytecode caches with embedded paths
find . -name "__pycache__" -type d -exec rm -rf {} + 2>/dev/null || true
```

### `post-update` (future)

Runs after `grove update` refreshes the golden copy. Could be used to
re-warm caches or notify systems.

### Why hooks instead of config patterns?

The previous design used `cleanup_paths` config patterns. Hooks are more
flexible:
- Can run arbitrary logic, not just file deletion
- Can be committed to the repo (shared across team)
- Familiar pattern (git hooks, npm scripts)
- Build-system agnostic — the repo author writes what they need

## Error Handling

### Non-CoW Filesystem

**Behavior**: Error out immediately with a clear message.

```
Error: Filesystem at /tmp does not support copy-on-write clones.
Grove requires APFS (macOS) or Btrfs/XFS with reflink support (Linux).
```

No silent fallback to `cp -R`. CoW is the whole point of grove — a slow
copy defeats the purpose. If someone needs non-CoW support, they can use
`cp -R` themselves.

### Disk Full During Clone

**Behavior**: Error out, clean up any partial workspace directory.

```
Error: Clone failed (disk full). Cleaned up partial workspace at /tmp/grove/myapp/abc1.
```

### Uncommitted Changes on Golden Copy

**Behavior**: Warn and refuse by default. Allow with `--force`.

```
Warning: Golden copy has uncommitted changes.
These changes will be included in the workspace clone.
Use --force to proceed anyway.
```

This prevents accidentally cloning work-in-progress state that was meant
for the golden copy only.

## Platform Support

| Feature | macOS (APFS) | Linux (Btrfs/XFS) | Linux (ext4) |
|---|---|---|---|
| CoW clone | `cp -c -R` | `cp --reflink=always` | **Not supported** |
| Instant? | Yes | Yes | N/A |
| Disk efficient? | Yes | Yes | N/A |

Grove will detect the filesystem at runtime and select the appropriate
clone strategy. Unsupported filesystems produce an error (no fallback).

### CoW Detection

- **macOS**: Run `diskutil info /` and check for APFS
- **Linux**: Check `/proc/mounts` or `stat -f` for Btrfs/XFS, then
  attempt a test reflink to verify support

## Build System Considerations

### Gradle

Gradle's configuration cache **is not relocatable** — it embeds absolute
paths with no rewrite mechanism. The Gradle team acknowledges this as a
limitation with no timeline for a fix.

Recommended `post-clone` hook strategy:

| Cache | Relocatable? | Action |
|---|---|---|
| `.gradle/configuration-cache/` | **No** — absolute paths | Delete in post-clone hook |
| `.gradle/*.lock` | N/A — stale locks | Delete in post-clone hook |
| `.gradle/caches/` | Yes | Keep (dependency metadata, huge speedup) |
| `.gradle/buildOutputCleanup/` | Mostly | Keep, monitor |
| `build/` directories | Yes | Keep (compiled classes, main speedup) |
| `local.properties` | Yes (if SDK path is stable) | Keep |

Deleting the configuration cache adds ~5-10s to the first build in each
workspace. This is a good tradeoff: the dependency cache and compiled
outputs (which *are* kept) save minutes.

### Other Build Systems

| Build System | What to clean | Notes |
|---|---|---|
| Node.js | Nothing | `node_modules` is relocatable |
| Rust/Cargo | Nothing | `target/` is relocatable |
| Python | `__pycache__/` | May contain absolute path references |
| CMake | `CMakeCache.txt` | Contains absolute paths to compilers |
| Bazel | Nothing (usually) | Outputs are content-addressed |

## Project Structure

```
grove/
├── cmd/
│   └── grove/
│       └── main.go           # Entry point, cobra root command
├── internal/
│   ├── config/
│   │   └── config.go         # Config loading/saving (.grove/ and ~/.grove/)
│   ├── golden/
│   │   └── golden.go         # Golden copy management (init, update)
│   ├── workspace/
│   │   └── workspace.go      # Workspace lifecycle (create, list, destroy)
│   ├── clone/
│   │   ├── clone.go          # Cloner interface
│   │   ├── apfs.go           # macOS: cp -c -R
│   │   ├── reflink.go        # Linux: cp --reflink=always
│   │   └── detect.go         # Filesystem detection
│   ├── hooks/
│   │   └── hooks.go          # Hook discovery and execution
│   └── git/
│       └── git.go            # Git operations (checkout, pull, branch)
├── docs/
│   └── DESIGN.md             # This file
├── go.mod
├── go.sum
├── LICENSE
├── README.md
└── .gitignore
```

### Package Responsibilities

**`cmd/grove`**: Cobra CLI setup. Thin — delegates to internal packages.

**`internal/config`**: Loads/saves both global (`~/.grove/config.json`) and
project (`.grove/config.json`) configs. Handles defaults and validation.

**`internal/golden`**: Manages the golden copy — init (register + warmup),
update (pull + rebuild), status (commit info, freshness).

**`internal/workspace`**: Workspace lifecycle — create (clone + hooks +
checkout), list (scan + read markers), destroy (cleanup + optional push).

**`internal/clone`**: Platform-abstracted CoW cloning. Defines a `Cloner`
interface with `Clone(src, dst string) error`. Implementations for APFS
and reflink. Includes filesystem detection to select the right one.

**`internal/hooks`**: Discovers and runs hooks from `.grove/hooks/`.
Handles permissions, timeouts, error reporting.

**`internal/git`**: Thin wrapper around `git` CLI for checkout, pull,
branch operations, dirty-state detection.

## Future Possibilities

- **`grove exec <command>`**: Run a command in a new workspace, destroy on exit
- **`grove watch`**: Auto-update golden copy when base branch changes
- **Multiple golden copies**: Support different base branches per repo
- **Disk usage tracking**: Show CoW savings (`grove status --disk`)
- **Integration with AI agent frameworks**: Skills, MCP tools, env vars
- **`--json` everywhere**: Machine-readable output for all commands
