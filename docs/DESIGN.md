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

A checkout of the repository with warm build state. Grove is
branch-agnostic — the golden copy is simply "whatever is on disk right
now." If you need golden copies for multiple branches, use git worktrees
to maintain separate checkouts and `grove init` each one independently.

- Has been built recently (warm caches, compiled outputs)
- Contains all gitignored build artifacts (`.gradle/`, `build/`, etc.)
- Grove doesn't manage which branch is checked out — that's up to you

### Workspace

A CoW filesystem clone of the golden copy:

- Created via selected backend:
  - `cp` (default): `cp -c -R` (macOS/APFS)
  - `image` (experimental): APFS sparsebundle base + per-workspace shadow mount
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
grove create [--branch NAME] [--progress]  Create a new workspace from the golden copy
grove list                    List active workspaces
grove destroy <id|path>       Remove a workspace
grove destroy --all           Remove all workspaces
grove status                  Show golden copy info and workspace summary
```

### `grove init [path]`

Registers an existing git repo as a grove-managed golden copy.

- `path` defaults to current directory
- Creates a `.grove/` directory in the repo root to store config and hooks
- Optionally runs an initial warm-up build (configurable command)
- **Error**: if golden copy has uncommitted changes, warn and require `--force`

Config stored in `.grove/config.json`:
```json
{
  "warmup_command": "./gradlew assemble",
  "workspace_dir": "/tmp/grove/{project}",
  "max_workspaces": 10,
  "clone_backend": "cp"
}
```

### `grove update`

A convenience command to refresh the golden copy. Not a core workflow
requirement — workspaces are short-lived, so staleness is managed by
the user re-running `update` when they choose to.

1. `git pull`
2. Run the configured warmup command
3. If `clone_backend` is `image`, refresh the base image incrementally
   (`rsync` into mounted sparsebundle). Refuse refresh while image-backed
   workspaces are active.

### `grove create [--branch NAME] [--progress]`

Creates a new workspace:

1. Verify filesystem supports CoW (error out if not — see [Error Handling](#error-handling))
2. Check golden copy for uncommitted changes (warn + require `--force`)
3. Create workspace using configured backend:
   - `cp`: CoW clone golden copy to workspace directory
   - `image`: attach base sparsebundle with per-workspace shadow
4. Write workspace marker file (`.grove/workspace.json`)
5. Run post-clone hooks (`.grove/hooks/post-clone`)
6. Check out the specified branch (or create one)
7. Output workspace path (plain text default, `--json` for machine-readable)

If `--progress` is set, create emits phase/percent progress updates during
clone. Progress output is sent to `stderr` so `--json` output on `stdout`
remains machine-readable.

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

Removes a workspace. Optionally pushes the branch first with `--push`.

- `cp` backend: remove workspace directory.
- `image` backend: detach mounted device, remove shadow + metadata, remove mountpoint.

## Metadata & State

All grove state lives in the `.grove/` directory within the repo. There
is no global config — grove commands operate on the repo you're in (or
the one you point at with `--path`).

### Project Config (`.grove/config.json`)

Lives in the repo root. Should be committed so all users share the
same config:

```json
{
  "warmup_command": "./gradlew assemble",
  "workspace_dir": "/tmp/grove/{project}",
  "max_workspaces": 10,
  "clone_backend": "cp"
}
```

When `clone_backend` is `image`, additional internal state is stored under:

- `.grove/images/state.json`
- `.grove/workspaces/<id>.json`
- `.grove/shadows/`

### Workspace Marker (`.grove/workspace.json`)

Written into each workspace's `.grove/` directory after cloning. Enables
`grove list` to discover workspaces and track provenance:

```json
{
  "id": "abc1",
  "golden_copy": "/Users/chris/dev/myapp",
  "golden_commit": "3329245",
  "created_at": "2026-02-17T10:20:00Z",
  "branch": "agent/fix-login"
}
```

The presence of `workspace.json` is what distinguishes a workspace from
a golden copy — golden copies don't have this file.

### Design Principle: No Daemon

Grove is stateless between invocations. All metadata is derived from:
- `.grove/config.json` (project settings)
- `.grove/workspace.json` (workspace provenance, only in workspaces)
- `.grove/hooks/` (lifecycle scripts)
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

## Build System Notes

Grove is build-system agnostic. It clones directories and runs hooks —
it has no knowledge of any specific build tool. The `post-clone` hook
is where build-system-specific cleanup belongs.

The main thing to watch for: **non-relocatable caches** that embed
absolute paths. When the workspace path differs from the golden copy
path, these caches become invalid. Most build systems handle this
gracefully (cache miss, rebuild that portion), but stale lock files
can cause errors.

Common patterns for `post-clone` hooks:
- Delete lock files (e.g., `find . -name "*.lock" -delete`)
- Delete caches that embed absolute paths (let the build system
  regenerate them)
- Leave relocatable caches alone (compiled outputs, dependency caches)

Build-system-specific integrations may be explored in the future.

## Project Structure

```
grove/
├── cmd/
│   └── grove/
│       └── main.go           # Entry point, cobra root command
├── internal/
│   ├── config/
│   │   └── config.go         # Config loading/saving (.grove/config.json)
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

**`internal/config`**: Loads/saves `.grove/config.json`. Handles defaults
and validation.

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

## Claude Code Skills Integration

Grove ships a companion repo (`grove-skills`) containing superpowers-compatible
skills for Claude Code. The skills and CLI are separate projects with different
distribution paths:

- **`grove`** — Go CLI tool. Installed via `go install` / `brew` / binary release.
- **`grove-skills`** — Markdown skill files. Installed via `npx skills add` or
  copied to `~/.claude/skills/`.

The coupling between them is the CLI's `--json` output — a stable interface
that skills parse.

### Why separate repos?

- Different audiences: CLI is system-wide, skills are per-Claude-Code-user
- Different distribution: `go install` vs `npx skills add`
- Different versioning: a skill rewrite doesn't need a new CLI release
- Keeps the Go project clean

### Relationship to superpowers `using-git-worktrees`

The `using-grove` skill is a **replacement** for superpowers'
`using-git-worktrees` skill, not a complement. Users should disable
`using-git-worktrees` when using grove. The skill hooks into the same
trigger keywords (isolation, workspace, feature work) so it gets invoked
by the same upstream skills (`brainstorming`, `subagent-driven-development`,
`executing-plans`).

If grove is not initialized in the current repo (no `.grove/` directory),
the skill tells the user to run `grove init` rather than silently falling
back to plain worktrees.

### Skill: `using-grove`

**Trigger**: Starting feature work that needs isolation, creating a workspace
for an agent, before executing implementation plans.

**Replaces**: `using-git-worktrees` (user must disable that skill).

**Frontmatter**:
```yaml
---
name: using-grove
description: "Use when starting feature work that needs isolation or before
  executing implementation plans - creates CoW-cloned workspaces with warm
  build caches via grove CLI"
---
```

**Workflow**:

1. **Verify grove is initialized**
   ```bash
   # Check for .grove/ directory in repo root
   test -d "$(git rev-parse --show-toplevel)/.grove"
   ```
   If not found: "Grove is not initialized in this repo. Run `grove init`
   to set up a golden copy first."

2. **Verify grove CLI is installed**
   ```bash
   command -v grove
   ```
   If not found: "Grove CLI is not installed. See https://github.com/user/grove
   for installation instructions."

3. **Create workspace**
   ```bash
   grove create --branch <branch-name> --json
   ```
   Parse JSON output for workspace path.

4. **Change to workspace directory**
   ```bash
   cd <workspace-path>
   ```

5. **Verify clean baseline**
   Run project's test suite. If tests fail, report and ask whether to proceed.

6. **Report ready**
   ```
   Grove workspace ready at <path>
   Branch: <branch-name>
   Build state: warm (cloned from golden copy)
   Tests: passing (<N> tests)
   ```

**Integration**:
- Called by: `brainstorming` (Phase 4), `subagent-driven-development`,
  `executing-plans`
- Pairs with: `finishing-grove-workspace`

### Skill: `finishing-grove-workspace`

**Trigger**: Implementation is complete, all tests pass, ready to integrate
work and clean up the workspace.

**Replaces**: The worktree cleanup portion of `finishing-a-development-branch`.

**Frontmatter**:
```yaml
---
name: finishing-grove-workspace
description: "Use when implementation is complete and all tests pass in a
  grove workspace - guides branch completion and workspace cleanup via
  grove CLI"
---
```

**Workflow**:

1. **Detect grove workspace**
   ```bash
   # Check for workspace marker
   test -f "$(git rev-parse --show-toplevel)/.grove/workspace.json"
   ```
   If not in a workspace: "Not in a grove workspace. Use
   `finishing-a-development-branch` instead."

2. **Verify tests pass**
   Run project's test suite. If tests fail, stop — don't offer completion
   options.

3. **Read workspace metadata**
   ```bash
   cat .grove/workspace.json
   ```
   Extract workspace ID, golden copy path, branch name.

4. **Present options**
   ```
   Implementation complete. What would you like to do?

   1. Push branch and create a Pull Request
   2. Push branch and destroy workspace
   3. Keep the workspace as-is (I'll handle it later)
   4. Discard this work and destroy workspace
   ```

5. **Execute choice**

   **Option 1: Push + PR**
   ```bash
   git push -u origin <branch>
   gh pr create --title "<title>" --body "<body>"
   grove destroy <workspace-id>
   ```
   Change back to golden copy directory.

   **Option 2: Push + destroy**
   ```bash
   git push -u origin <branch>
   grove destroy <workspace-id>
   ```
   Change back to golden copy directory.

   **Option 3: Keep as-is**
   Report: "Keeping workspace at <path>."

   **Option 4: Discard**
   Require typed confirmation ("discard").
   ```bash
   grove destroy <workspace-id>
   ```
   Change back to golden copy directory.

**Integration**:
- Called by: `subagent-driven-development` (Step 7),
  `executing-plans` (Step 5)
- Pairs with: `using-grove`

### Skill interaction flow

```
brainstorming / executing-plans / subagent-driven-development
  │
  ├─ needs workspace isolation
  │
  ▼
using-grove                          (replaces using-git-worktrees)
  │
  ├─ grove create --branch <name>
  ├─ cd into workspace
  ├─ verify tests
  │
  ▼
[... agent does work ...]
  │
  ▼
finishing-grove-workspace            (replaces worktree cleanup)
  │
  ├─ verify tests
  ├─ present options (PR / push / keep / discard)
  ├─ grove destroy
  └─ cd back to golden copy
```

## Future Possibilities

- **`grove exec <command>`**: Run a command in a new workspace, destroy on exit
- **`grove watch`**: Auto-update golden copy when base branch changes
- **Disk usage tracking**: Show CoW savings (`grove status --disk`)
- **Build system integrations**: Deeper awareness of specific build tools
- **`--json` everywhere**: Machine-readable output for all commands
