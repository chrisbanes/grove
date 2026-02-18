---
name: grove-init
description: Use when setting up Grove for a repository that does not yet have a `.grove/config.json`, or when a user asks to initialize Grove in their project
---

I'm using the grove-init skill to set up Grove for this project.

## Workflow

### Step 1: Verify git repo

```bash
git rev-parse --show-toplevel
```

If the command fails, stop. Grove requires a git repository.

### Step 2: Check if already initialized

```bash
test -f .grove/config.json
```

If `.grove/config.json` exists, report that Grove is already initialized and ask the user whether they want to re-initialize. Stop unless they confirm.

### Step 3: Detect build system

Scan for marker files in the following priority order. Use the first match. If multiple top-level markers exist, ask the user which is the primary build system.

| Build System | Marker File(s) | Warmup Command | Post-Clone Hook |
|---|---|---|---|
| Gradle | `build.gradle.kts` or `build.gradle` | `./gradlew assemble` | Clean lock files, `configuration-cache` |
| Node.js | `package.json` | `npm run build` | Clean `node_modules/.cache` |
| Rust | `Cargo.toml` | `cargo build` | Clean `target/debug/incremental` |
| Go | `go.mod` | `go build ./...` | Minimal — Go handles relocatable caches well |
| Python | `pyproject.toml` or `requirements.txt` | `poetry install` or `pip install -e .` | Clean `__pycache__` dirs |
| C/C++ | `Makefile` or `CMakeLists.txt` | `make` or `cmake --build build` | Project-specific |

If no marker is found, ask the user to provide a warmup command.

### Step 4: Present proposed config for confirmation

Show the user:
- Warmup command
- Workspace directory (default: system temp under a grove subdirectory)
- Post-clone hook content

Ask for confirmation before proceeding.

### Step 5: Run grove init

```bash
grove init --warmup-command "<cmd>"
```

### Step 6: Write post-clone hook

Write the post-clone hook script to `.grove/hooks/post-clone` and make it executable:

```bash
chmod +x .grove/hooks/post-clone
```

### Step 7: Suggest git add

Tell the user to commit the Grove configuration:

```bash
git add .grove/config.json .grove/hooks/
git commit -m "chore: add Grove configuration"
```

## Quick Reference

| Command | Purpose |
|---|---|
| `grove init --warmup-command "<cmd>"` | Initialize Grove |
| `grove update` | Refresh golden copy build state |

## Common Mistakes

- **Running `grove init` outside a git repo** — Grove requires git; always verify first.
- **Skipping the post-clone hook** — without it, workspaces inherit stale lock files or cache artifacts.
- **Not committing `.grove/`** — commit so collaborators and CI get the same warmup behavior.

## Red Flags

- `grove init` fails with a permissions error — check `grove` is installed and the user has write access.
- Warmup command exits non-zero during `grove init` — fix unresolved dependencies before initializing.
- Multiple `package.json` and `build.gradle` at the root — polyglot monorepo; ask which is primary.

## Integration

- **Standalone** — not called by other skills automatically
- **Suggested by:** `grove:using-grove` when `.grove/config.json` is not found
- **Invoked via:** `/grove-init` slash command
