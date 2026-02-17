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
- Kept up to date periodically

### Workspace

A CoW filesystem clone of the golden copy:

- Created near-instantly via `cp -c -R` (macOS/APFS) or `cp --reflink=always` (Linux/Btrfs)
- Fully isolated — each agent gets its own working tree
- Short-lived: created for a task, destroyed when done
- Inherits warm build state, so incremental builds work immediately
- Checked out on its own branch

### Why not git worktree?

`git worktree` shares the `.git` directory but creates a fresh working tree.
Gitignored files (build caches, compiled outputs) are not copied. Grove
clones the *entire* directory including gitignored state, giving each
workspace a warm build cache from the start.

## CLI Design

```
grove init [path]             Initialize a golden copy from an existing repo
grove update                  Pull latest + rebuild the golden copy
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

Config stored in `.grove/config.json`:
```json
{
  "warmup_command": "./gradlew assemble",
  "branch": "main",
  "workspace_dir": "/tmp/grove/{project}",
  "max_workspaces": 10,
  "cleanup_paths": ["*.lock", ".gradle/lock/*"]
}
```

### `grove update`

Refreshes the golden copy:

1. Ensure no workspaces are active (or `--force`)
2. `git pull` on the configured base branch
3. Run the configured warmup command
4. Clean lock files (configured patterns)

### `grove create [--branch NAME]`

Creates a new workspace:

1. CoW clone golden copy to workspace directory
2. Clean lock files and other non-relocatable state
3. Check out the specified branch (or create one)
4. Print the workspace path

### `grove list`

Shows active workspaces:
```
ID     BRANCH                  CREATED      PATH
abc1   agent/fix-login         2m ago       /tmp/grove/myapp/abc1
def2   agent/add-feature       15m ago      /tmp/grove/myapp/def2
```

### `grove destroy <id|path>`

Removes a workspace via `rm -rf`. Optionally pushes the branch first
with `--push`.

## Project Structure

```
grove/
├── cmd/
│   └── grove/
│       └── main.go           # Entry point
├── internal/
│   ├── config/
│   │   └── config.go         # Config loading/saving (.grove/ and ~/.grove/)
│   ├── golden/
│   │   └── golden.go         # Golden copy management (init, update)
│   ├── workspace/
│   │   └── workspace.go      # Workspace lifecycle (create, list, destroy)
│   ├── clone/
│   │   ├── clone.go          # Interface for CoW cloning
│   │   ├── apfs.go           # macOS APFS implementation (cp -c -R)
│   │   └── reflink.go        # Linux reflink implementation (cp --reflink)
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

### Key Design Decisions

**`internal/clone` abstraction**: The CoW cloning mechanism is behind an
interface so macOS (APFS `cp -c`) and Linux (Btrfs `cp --reflink`) are
swappable. A fallback to plain `cp -R` can be provided with a warning.

**Config at two levels**:
- `.grove/config.json` in the golden repo — project-specific settings
  (warmup command, cleanup paths)
- `~/.grove/config.json` global — registry of golden copies, defaults

**Workspace directory**: Defaults to `/tmp/grove/{project}/{id}` so
workspaces are naturally cleaned on reboot. Configurable for persistence.

**No daemon**: Grove is stateless between invocations. Workspace metadata
is derived from the filesystem (existence of directories, git branch state).

## Platform Support

| Feature | macOS (APFS) | Linux (Btrfs/XFS) | Linux (ext4) |
|---|---|---|---|
| CoW clone | `cp -c -R` | `cp --reflink=always` | Fallback: `cp -R` (slow) |
| Instant? | Yes | Yes | No |
| Disk efficient? | Yes | Yes | No |

## Build System Considerations

### Gradle-specific

Gradle stores state that may contain absolute paths or lock files:

| Path | Issue | Mitigation |
|---|---|---|
| `.gradle/*.lock` | Stale locks from golden copy | Delete during `grove create` |
| `.gradle/configuration-cache/` | May contain absolute paths | Delete or use `--no-configuration-cache` on first build |
| `.gradle/buildOutputCleanup/` | Tracks outputs by path | Usually relocatable, monitor |
| `build/` directories | Compiled outputs | Fully relocatable, no action needed |
| `local.properties` | SDK path, should be same | No action needed if agents share SDK |

### Generic (non-Gradle)

The `cleanup_paths` config allows specifying patterns to clean after cloning,
making grove build-system agnostic. Examples:

- Node.js: clean nothing (node_modules is relocatable)
- Rust: clean nothing (target/ is relocatable)
- Python: clean `__pycache__/` if absolute paths are cached

## Future Possibilities

- **`grove exec <command>`**: Run a command in a new workspace, destroy on exit
- **`grove watch`**: Auto-update golden copy when base branch changes
- **Parallel builds**: Warm up multiple golden copies for different base branches
- **Disk usage tracking**: Show CoW savings (`grove status --disk`)
- **Integration with AI agent frameworks**: Provide workspace paths via API/env vars
