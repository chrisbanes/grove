# Optional Init, Interactive Wizard, and New Workspace Directory

## Problem

Grove requires `grove init` before any other command, even when the user wants plain defaults. This adds friction for the common case (cp backend, no customization). The default workspace directory (`/tmp/grove/{project}`) doesn't survive reboots.

## Design

### Config-Free Mode (Lazy Init)

Every grove command works without `grove init`. Running `grove create` in any git repo:

1. Detects it's a git repo (or errors: "not a git repository")
2. Creates a minimal `.grove/` with `.runtime-id`, `workspace.json`, and `.gitignore`
3. Uses defaults: `cp` backend, `~/.grove/{project}/` workspace dir, max 10 workspaces
4. Creates the workspace

`grove init` becomes optional. It's the "customize your setup" command.

**Config resolution order:**

1. Command-line flags (highest priority)
2. `.grove/config.json` if it exists (from explicit `init`)
3. Built-in defaults (lowest priority)

**`FindGroveRoot` change:** Falls back to finding the git root when `.grove/` doesn't exist. Creates `.grove/` lazily on first write (e.g., first `grove create`).

### New Default Workspace Directory

Default changes from `/tmp/grove/{project}` to `~/.grove/{project}`.

- Survives reboots (unlike `/tmp/` on macOS)
- Natural home for all grove state

**Directory layout:**

```
~/.grove/
└── myapp/
    ├── feature-auth-f7e8/       # workspace (cp backend)
    ├── fix-bug-a1b2/            # workspace (cp backend)
    └── runtimes/                # image backend only
        └── <runtime-id>/
            ├── images/
            │   └── base.sparsebundle
            └── shadows/
                └── <id>.shadow
```

`~/.grove/{project}/` becomes the project home. Workspaces live directly inside it. The image backend stores runtime data in `runtimes/` alongside them.

### Interactive `init` Wizard

Running `grove init` with no flags starts a smart interactive wizard:

```
$ grove init

Initializing grove for myapp...

? Which clone backend? (use arrow keys)
> cp    - fast APFS copy-on-write clones (recommended, default)
  image - sparsebundle-based clones (experimental, macOS only)

? Workspace directory (~/.grove/myapp): _

# Only if image backend selected:
? Base image size in GB (200): _

? Warmup command (optional, runs after init to build caches):
> npm install && npm run build

? Exclude patterns (comma-separated, optional):
> *.lock, __pycache__

Done! Initialized grove in /Users/chris/dev/myapp/.grove/config.json
```

**Behaviors:**

- Each question shows the default in parentheses. Enter accepts it.
- Flags override and skip their corresponding question (`grove init --backend cp` skips backend prompt).
- If all flags are provided, no questions are asked (fully scriptable).
- Image size question appears only when the image backend is selected.
- `--defaults` flag skips all questions and uses defaults.

**Dependency:** A Go library for interactive prompts (e.g., `charmbracelet/huh` or `AlecAivazis/survey`).

### Command Behavior Changes

| Command | Before | After |
|---------|--------|-------|
| `grove init` | Required first step, creates config with defaults | Optional, runs interactive wizard, writes config |
| `grove create` | Fails without `init` | Works in any git repo, auto-creates minimal `.grove/` |
| `grove list` | Fails without `init` | Works, scans workspace dir, returns empty if none exist |
| `grove destroy` | Fails without `init` | Works, finds workspace by ID/path, removes it |
| `grove update` | Fails without `init` | Works, pulls golden copy, re-runs warmup if configured |
| `grove status` | Fails without `init` | Works, shows golden copy info and any workspaces |
| `grove migrate` | Requires `init` | Still requires `init` (needs config to know migration target) |

**Principle:** Reading commands (`list`, `status`) never create state. Writing commands (`create`) lazily create the minimal `.grove/` on first use.

### Migration and Backwards Compatibility

- **Existing projects** with `.grove/config.json`: work as before. The config file is respected.
- **Existing workspaces in `/tmp/grove/{project}/`:** honored if `workspace_dir` is set in config. No migration needed.
- **No breaking changes.** Commands stop requiring `.grove/config.json` to exist, but still respect it when present.
