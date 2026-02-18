# Grove

[![CI](https://github.com/chrisbanes/grove/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/chrisbanes/grove/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/chrisbanes/grove)](https://github.com/chrisbanes/grove/releases)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://golang.org)
[![License](https://img.shields.io/github/license/chrisbanes/grove)](LICENSE)
[![macOS](https://img.shields.io/badge/platform-macOS_(APFS)-000000?logo=apple)](#platform-support)

Instant workspaces with warm build caches, powered by copy-on-write clones.

## The Problem

When you need multiple copies of a repository -- for parallel features, code review, or experimentation -- the standard approaches (`git worktree`, `git clone`) create fresh working trees. They share git objects, but **lose all gitignored local state**: build caches, compiled outputs, dependency metadata, `node_modules/`, `.gradle/`, `build/` directories.

Every new working copy starts cold. For large projects, rebuilding takes minutes.

## How Grove Works

Grove maintains a **golden copy** of your repository -- a checkout with warm build state (compiled outputs, dependency caches, everything). When you need a new workspace, Grove creates a **copy-on-write filesystem clone** of the entire golden copy, including all gitignored files.

Regardless of repo size, CoW clones complete in under a second and share disk blocks with the original. Blocks duplicate only when one side writes to them. Each workspace gets its own git branch, working tree, and build state.

```
Golden Copy (warm build state)
    |
    +-- grove create --> Workspace 1 (CoW clone, own branch)
    +-- grove create --> Workspace 2 (CoW clone, own branch)
    +-- grove create --> Workspace 3 (CoW clone, own branch)
```

## Comparison

|                          | Grove          | git worktree    | git clone --shared | Full git clone |
|--------------------------|----------------|-----------------|--------------------|----------------|
| Clone speed              | ~1s (any size) | ~1s             | Seconds            | Minutes        |
| Build caches preserved   | Yes            | No              | No                 | No             |
| Gitignored files cloned  | Yes            | No              | No                 | No             |
| Disk usage               | Shared (CoW)   | Shared (.git)   | Shared (objects)   | Full copy      |
| Isolation                | Full           | Shared .git     | Partial            | Full           |
| Branch independence      | Yes            | Yes (limited)   | Yes                | Yes            |

Grove uses git under the hood. It replaces only the step where you create a new working copy.

## Quick Start

### Install

```bash
# Homebrew (macOS)
brew install chrisbanes/tap/grove

# Go install
go install github.com/chrisbanes/grove/cmd/grove@latest

# Or download a binary from the latest release:
# https://github.com/chrisbanes/grove/releases
```

### Basic Workflow

```bash
# 1. Register your repo as a golden copy
cd ~/dev/myproject
grove init --warmup-command "./gradlew assemble"

# 2. Create a workspace (instant CoW clone)
grove create --branch feature/new-login
# Workspace created: feature-new-login-a1b2
# Path: /tmp/grove/myproject/feature-new-login-a1b2
# Branch: feature/new-login

# 3. Work in the workspace -- builds start warm
cd /tmp/grove/myproject/feature-new-login-a1b2

# 4. Clean up when done
grove destroy feature-new-login-a1b2
```

## Commands

### `grove init [path]`

Register a git repository as a grove-managed golden copy. Creates a `.grove/` directory with configuration and a hooks directory. Defaults to the current directory.

```bash
grove init --warmup-command "npm run build" --workspace-dir ~/workspaces/myproject
# Running warmup: npm run build
# Grove initialized at /Users/you/dev/myproject
# Workspace dir: /Users/you/workspaces/myproject
```

| Flag | Description |
|------|-------------|
| `--warmup-command` | Shell command to warm build caches (runs during init and update) |
| `--workspace-dir` | Directory for workspaces (default: `/tmp/grove/{project}`) |
| `--force` | Proceed even if the golden copy has uncommitted changes |

### `grove create`

Create a CoW clone workspace from the golden copy. Without `--branch`, the
workspace stays on the golden copy's current branch. With `--branch`, Grove
creates and checks out a new git branch in the workspace.

```bash
grove create --branch feature/auth
# Workspace created: feature-auth-f7e8
# Path: /tmp/grove/myproject/feature-auth-f7e8
# Branch: feature/auth

grove create
# Workspace created: main-d9c0
# Path: /tmp/grove/myproject/main-d9c0
```

For machine-readable output:

```bash
grove create --branch agent/fix-bug --json
```
```json
{
  "id": "agent-fix-bug-f7e8",
  "golden_copy": "/Users/you/dev/myproject",
  "golden_commit": "abc1234",
  "created_at": "2026-02-17T10:30:00Z",
  "branch": "agent/fix-bug",
  "path": "/tmp/grove/myproject/agent-fix-bug-f7e8"
}
```

| Flag | Description |
|------|-------------|
| `--branch` | Create and checkout a new git branch in the workspace (default: golden copy's current branch) |
| `--force` | Proceed even if the golden copy has uncommitted changes |
| `--json` | Output workspace info as JSON |

### `grove list`

List active workspaces.

```bash
grove list
# ID                     BRANCH              CREATED    PATH
# feature-auth-f7e8      feature/auth        5m ago     /tmp/grove/myproject/feature-auth-f7e8
# feature-new-login-a1b2 feature/new-login   2h ago     /tmp/grove/myproject/feature-new-login-a1b2
```

| Flag | Description |
|------|-------------|
| `--json` | Output workspace list as JSON |

### `grove destroy <id|path>`

Remove a workspace. Takes a workspace ID or absolute path.

```bash
# Destroy a single workspace
grove destroy feature-auth-f7e8
# Destroyed: feature-auth-f7e8

# Push the branch to origin before destroying
grove destroy --push feature-auth-f7e8

# Destroy all workspaces
grove destroy --all
# Destroyed: feature-auth-f7e8
# Destroyed: feature-new-login-a1b2
```

| Flag | Description |
|------|-------------|
| `--push` | Push the workspace branch to origin before destroying |
| `--all` | Destroy all workspaces |

### `grove update`

Pull the latest changes and re-run the warmup command on the golden copy.

```bash
grove update
# Pulling latest...
# Running warmup: ./gradlew assemble
# Golden copy updated to abc1234
```

### `grove status`

Show golden copy info and workspace summary.

```bash
grove status
# Golden copy: /Users/you/dev/myproject
# Branch:      main
# Commit:      abc1234
# Status:      clean
#
# Workspaces:  2 / 10 (max)
# Directory:   /tmp/grove/myproject
```

### `grove version`

Print the grove version.

```bash
grove version
# grove v0.1.0
```

## Configuration

Grove stores its configuration in `.grove/config.json` inside the golden copy:

```json
{
  "warmup_command": "./gradlew assemble",
  "workspace_dir": "/tmp/grove/{project}",
  "max_workspaces": 10
}
```

| Field | Description | Default |
|-------|-------------|---------|
| `warmup_command` | Shell command to warm build caches. Runs during `grove init` and `grove update`. | *(none)* |
| `workspace_dir` | Where workspaces are created. `{project}` expands to the golden copy's directory name. | `/tmp/grove/{project}` |
| `max_workspaces` | Maximum concurrent workspaces. Prevents disk exhaustion. | `10` |

**Warmup command examples by ecosystem:**

| Ecosystem | Warmup command |
|-----------|---------------|
| Gradle | `./gradlew assemble` |
| npm | `npm run build` |
| Cargo | `cargo build` |
| Go | `go build ./...` |

## Hooks

Grove runs executable scripts from `.grove/hooks/` at specific lifecycle points.

### `post-clone`

Runs inside each new workspace after the CoW clone completes, before branch checkout. Use it to clean up non-relocatable state (lock files, caches with embedded absolute paths).

**Gradle example** (`.grove/hooks/post-clone`):
```bash
#!/bin/bash
# Clean Gradle lock files and non-relocatable caches
find . -name "*.lock" -path "*/.gradle/*" -delete
rm -rf .gradle/configuration-cache
```

**Python example** (`.grove/hooks/post-clone`):
```bash
#!/bin/bash
# Remove bytecode caches (they embed absolute paths)
find . -type d -name "__pycache__" -exec rm -rf {} +
find . -name "*.pyc" -delete
```

Hooks must be executable (`chmod +x .grove/hooks/post-clone`). Grove errors if a hook file exists but lacks execute permission. Commit your hooks to the repo so all contributors share them.

## Use with AI Agents

Grove targets multi-agent AI workflows where each agent needs an isolated workspace with warm build state. The `--json` flag on `create` and `list` provides machine-readable output for programmatic consumers.

**Typical agent workflow:**

```bash
# Agent creates its own workspace
grove create --branch agent/fix-login --json
# Parse JSON for workspace path

# Agent works in the isolated workspace
cd /tmp/grove/myproject/agent-fix-login-a1b2
# ... make changes, run tests ...

# Push branch and clean up
grove destroy --push agent-fix-login-a1b2
```

Multiple agents can work in parallel, each in its own workspace. Every workspace starts with the same warm build state from the golden copy.

Beyond AI agents, Grove serves any workflow that needs parallel workspaces: CI pipelines, code review, and experimentation.

## Claude Code Skills

Grove includes a Claude Code plugin with skills that integrate Grove into Claude Code workflows. When installed, these skills replace the built-in worktree-based skills with Grove-aware equivalents.

### Install

```bash
claude plugin add chrisbanes/grove
```

### Skills

| Skill | Description |
|-------|-------------|
| `grove:using-grove` | Creates a warm workspace before feature work. Replaces `superpowers:using-git-worktrees`. |
| `grove:finishing-grove-workspace` | Guides completion and cleanup when work is done. Replaces `superpowers:finishing-a-development-branch`. |
| `grove:grove-init` | Opinionated first-time setup with build system detection. Also available as `/grove-init`. |
| `grove:grove-doctor` | Diagnoses Grove setup issues (APFS, CLI, hooks, disk space). |
| `grove:grove-multi-agent` | Orchestrates parallel agents across isolated workspaces. |

### How it works

A **SessionStart hook** runs at the beginning of each conversation. It detects whether you're in a Grove golden copy or workspace and tells Claude which skills are relevant. In a golden copy, Claude will use `grove:using-grove` to create workspaces. In a workspace, it knows to use `grove:finishing-grove-workspace` when done.

The skills integrate with the [superpowers](https://github.com/obra/superpowers) workflow -- `brainstorming`, `subagent-driven-development`, and `executing-plans` all invoke `grove:using-grove` instead of `using-git-worktrees` when Grove is detected.

## Platform Support

| Platform | Filesystem | Status |
|----------|-----------|--------|
| macOS | APFS | **Supported** |
| Linux | Btrfs / XFS (reflink) | Planned |
| Linux | ext4 | Not supported (no CoW) |
| Windows | NTFS / ReFS | Not supported |

Grove requires a filesystem with copy-on-write support. All modern Macs (macOS High Sierra / 2017 and later) use APFS. Linux support for Btrfs and XFS reflink is planned -- the `Cloner` interface and filesystem detection scaffolding are already in place.

Grove errors with a clear message on unsupported filesystems. It never silently falls back to a regular copy.

## How It Works

On macOS, Grove uses `cp -c -R` to create an APFS clone. This filesystem-level operation shares disk blocks between the clone and the original; blocks duplicate only when one side writes to them.

Before cloning, Grove verifies APFS support by querying `diskutil info` at runtime.

All state lives in `.grove/` within the repo -- no daemon, no global config, no database. Each workspace contains a `.grove/workspace.json` marker file, which `grove list` discovers by scanning the workspace directory.

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, testing, and PR guidelines.

## License

[Apache 2.0](LICENSE)
