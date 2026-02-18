# Grove Skills Plugin — Design Document

## Background

Grove creates instant, isolated workspaces with warm build caches via
copy-on-write filesystem clones. Its primary audience is multi-agent AI
workflows where each agent needs its own workspace.

Claude Code's superpowers plugin includes `using-git-worktrees` and
`finishing-a-development-branch` skills for workspace isolation. These
create fresh worktrees that lack build caches — Grove's whole value
proposition. This design adds a Claude Code plugin to the Grove repo
that replaces those skills and adds Grove-specific capabilities.

## Goal

Ship a standalone Claude Code plugin in the Grove repo that:

1. Replaces `using-git-worktrees` and `finishing-a-development-branch`
   with Grove-aware equivalents
2. Adds setup, diagnostics, and multi-agent orchestration skills
3. Integrates with the existing superpowers workflow
4. Works as a Claude Marketplace plugin (installable alongside superpowers)

## Repository Layout

The plugin files live alongside the existing Go CLI code:

```
grove/
├── .claude-plugin/
│   ├── plugin.json              # Plugin metadata
│   └── marketplace.json         # Marketplace distribution config
├── skills/
│   ├── using-grove/
│   │   └── SKILL.md
│   ├── finishing-grove-workspace/
│   │   └── SKILL.md
│   ├── grove-init/
│   │   └── SKILL.md
│   ├── grove-doctor/
│   │   └── SKILL.md
│   └── grove-multi-agent/
│       └── SKILL.md
├── hooks/
│   ├── hooks.json               # SessionStart hook config
│   └── session-start.sh         # Detects .grove/, injects context
├── commands/
│   └── grove-init.md            # Slash command: /grove-init
├── cmd/grove/                   # (existing CLI — untouched)
├── internal/                    # (existing CLI — untouched)
└── ...
```

Skills, hooks, and commands sit at the repo root, matching the
superpowers convention. The CLI code is untouched.

## Plugin Metadata

### `.claude-plugin/plugin.json`

```json
{
  "name": "grove",
  "description": "Grove skills for Claude Code: instant isolated workspaces with warm build caches via copy-on-write clones",
  "version": "0.1.0",
  "author": {
    "name": "Chris Banes"
  },
  "homepage": "https://github.com/chrisbanes/grove",
  "repository": "https://github.com/chrisbanes/grove",
  "license": "Apache-2.0",
  "keywords": ["grove", "workspaces", "cow", "isolation", "multi-agent"]
}
```

### `.claude-plugin/marketplace.json`

```json
{
  "name": "grove",
  "description": "Grove skills for Claude Code: instant isolated workspaces with warm build caches",
  "owner": {
    "name": "Chris Banes"
  },
  "plugins": [
    {
      "name": "grove",
      "description": "Grove skills for Claude Code: instant isolated workspaces with warm build caches via copy-on-write clones",
      "version": "0.1.0",
      "source": "./",
      "author": {
        "name": "Chris Banes"
      }
    }
  ]
}
```

## SessionStart Hook

`hooks/session-start.sh` fires on every conversation start. It detects
the Grove context and injects a short message so Claude knows which
skills are relevant.

**Detection logic:**

1. Check if `grove` CLI is on PATH
2. Walk up from cwd looking for `.grove/workspace.json` (workspace) or
   `.grove/config.json` (golden copy)
3. Inject context based on what's found:

| Detected State | Injected Context |
|---|---|
| In a workspace | "You are in a Grove workspace (ID: X). Golden copy: Y. Branch: Z. Use grove:finishing-grove-workspace when done." |
| In a golden copy | "This project uses Grove. Use grove:using-grove to create isolated workspaces." |
| CLI not installed | "Grove CLI is not installed. Use grove:grove-init for setup guidance." |
| No .grove/ at all | (silent — inject nothing) |

### `hooks/hooks.json`

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume|clear|compact",
        "hooks": [
          {
            "type": "command",
            "command": "${CLAUDE_PLUGIN_ROOT}/hooks/session-start.sh",
            "async": false
          }
        ]
      }
    ]
  }
}
```

## Slash Command

### `/grove-init`

```yaml
---
description: "Set up Grove for this project - detects build system, configures golden copy"
disable-model-invocation: true
---

Invoke the grove:grove-init skill and follow it exactly as presented to you
```

The other skills don't need slash commands — they're triggered
automatically by the superpowers workflow or by the session-start hook.

## Skills

### 1. `using-grove`

**Replaces:** `superpowers:using-git-worktrees`

**Trigger:** Starting feature work that needs isolation, before executing
implementation plans.

**Workflow:**

1. Verify `.grove/config.json` exists in repo root. If not, suggest
   `grove:grove-init` and stop.
2. Verify `grove` CLI is on PATH. If not, provide install instructions
   and stop.
3. Run `grove create --branch <branch-name> --json`. Parse JSON output
   for workspace path and ID.
4. `cd` into the workspace.
5. Run project test suite to verify clean baseline. If tests fail,
   report and ask whether to proceed.
6. Report ready with workspace path, branch, and build state.

**Called by:** brainstorming, subagent-driven-development, executing-plans
(anywhere superpowers would invoke `using-git-worktrees`).

**Pairs with:** `grove:finishing-grove-workspace`

### 2. `finishing-grove-workspace`

**Replaces:** `superpowers:finishing-a-development-branch`

**Trigger:** Implementation complete, tests pass, ready to integrate work
and clean up.

**Workflow:**

1. Detect workspace: check for `.grove/workspace.json`. If not in a
   workspace, tell user to use `finishing-a-development-branch` instead.
2. Verify tests pass. If tests fail, stop.
3. Read workspace metadata from `.grove/workspace.json` — extract ID,
   golden copy path, branch.
4. Present 4 options:
   - Push branch and create a Pull Request
   - Push branch and destroy workspace
   - Keep workspace as-is
   - Discard work and destroy workspace
5. Execute choice. For options that destroy, run `grove destroy <id>`
   and cd back to the golden copy directory.

**Called by:** subagent-driven-development, executing-plans (anywhere
superpowers would invoke `finishing-a-development-branch`).

**Pairs with:** `grove:using-grove`

### 3. `grove-init`

**Trigger:** User wants to set up Grove for a project. Also suggested by
`using-grove` when `.grove/` is missing.

**Workflow:**

1. Verify this is a git repo.
2. Detect build system by scanning for marker files:
   - `build.gradle` / `build.gradle.kts` → Gradle
   - `package.json` → Node.js (npm/yarn/pnpm/bun)
   - `Cargo.toml` → Rust
   - `go.mod` → Go
   - `pyproject.toml` / `requirements.txt` → Python
   - `Makefile` / `CMakeLists.txt` → C/C++
   - Multiple matches → ask user which is primary
3. Propose warmup command based on detected build system (e.g.,
   `./gradlew assemble` for Gradle, `npm run build` for Node).
   Ask for confirmation.
4. Propose post-clone hook content based on build system (e.g., clean
   Gradle lock files, delete `__pycache__`). Ask for confirmation.
5. Run `grove init` with the chosen warmup command.
6. Write the post-clone hook to `.grove/hooks/post-clone` and make it
   executable.
7. Suggest adding `.grove/` config and hooks to git.

**Standalone** — invoked via `/grove-init` command or suggested by
`using-grove`.

### 4. `grove-doctor`

**Trigger:** Grove commands failing, workspace issues, debugging Grove
setup problems.

**Diagnostic checks:**

1. **APFS support** — verify filesystem supports CoW
2. **CLI installed** — `command -v grove`
3. **CLI version** — `grove version`
4. **Golden copy health** — `.grove/config.json` exists, git repo is
   clean (or report dirty state), current branch and commit
5. **Workspace directory** — exists, writable, disk space available
6. **Hooks** — `.grove/hooks/post-clone` exists and is executable
7. **Active workspaces** — `grove list --json`, report count and any
   stale workspaces

**Output:** A checklist with pass/fail/warn for each check, plus fix
suggestions for any failures.

**Standalone** — invoked manually when things go wrong.

### 5. `grove-multi-agent`

**Trigger:** Multiple independent tasks that can be parallelized across
isolated workspaces.

**Workflow:**

1. **Receive task list** — from a plan, from `dispatching-parallel-agents`,
   or directly from the user.
2. **Validate independence** — check that tasks don't touch overlapping
   files or have ordering dependencies.
3. **Create N workspaces** — `grove create --branch agent/<task-slug> --json`
   for each task.
4. **Dispatch N subagents** — one `Task` tool call per workspace. Each
   subagent gets:
   - The workspace path (told to `cd` into it)
   - The full task description
   - Instructions to implement, test, commit, and report back
5. **All subagents run in parallel.**
6. **Collect results** — as subagents complete, gather their summaries.
7. **Review for conflicts** — verify no overlapping file edits across
   workspaces.
8. **Present results** — report what each agent accomplished, flag any
   issues.
9. **Clean up** — for each workspace, present options:
   - Push all branches and create PRs
   - Keep workspaces for manual review
   - Destroy all workspaces

**Branch naming:** `agent/<parent-branch>/<task-slug>` for traceability.

**Key difference from `dispatching-parallel-agents`:** Each agent gets
its own isolated workspace with warm build state. Agents can build,
modify files, and run tests without interfering with each other.

## Skill Integration Map

```
superpowers:brainstorming
superpowers:executing-plans
superpowers:subagent-driven-development
  │
  ├─ needs workspace isolation
  │
  ▼
grove:using-grove               (replaces superpowers:using-git-worktrees)
  │
  ├─ .grove/ not found? ──► suggests grove:grove-init
  │
  ├─ grove create --branch <name> --json
  ├─ cd into workspace
  ├─ verify test baseline
  │
  ▼
[... agent does work ...]
  │
  ▼
grove:finishing-grove-workspace  (replaces superpowers:finishing-a-development-branch)
  │
  ├─ verify tests
  ├─ present options (PR / push / keep / discard)
  ├─ grove destroy
  └─ cd back to golden copy


grove:grove-multi-agent          (new — no superpowers equivalent)
  │
  ├─ takes N independent tasks
  ├─ grove create × N
  ├─ dispatches Task subagent into each workspace
  ├─ monitors completion, collects results
  └─ grove destroy × N


grove:grove-doctor               (standalone diagnostic)
grove:grove-init                 (standalone setup, also via /grove-init)
```

**Conflict handling:** Users with both superpowers and Grove installed
will have both `using-git-worktrees` and `using-grove` available. The
session-start hook injects "This project uses Grove" context, which
makes Claude prefer the Grove skills. The `using-grove` skill explicitly
states it replaces `using-git-worktrees`.

## What This Design Does NOT Include

- Changes to the Grove CLI — the plugin is purely additive
- Automatic disabling of superpowers worktree skills — users manage
  this themselves (the hook context steers Claude to prefer Grove)
- Skill testing infrastructure — follows the `writing-skills` TDD
  process during implementation
