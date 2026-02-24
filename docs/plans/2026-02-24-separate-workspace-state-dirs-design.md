# Separate workspace_dir and state_dir

## Problem

`workspace_dir` serves double duty: it holds both mounted workspaces (where users work) and image backend runtime state (sparsebundles, shadows, metadata). These have different lifetimes, ownership semantics, and visibility requirements. Workspaces are user-facing artifacts; runtime state is internal plumbing.

## Design

Add a `state_dir` config field alongside `workspace_dir`. Each field has one job:

- **`workspace_dir`**: where workspace directories are created (user-facing)
- **`state_dir`**: where grove stores internal runtime state (image backend data)

### Defaults

| Field | Default |
|-------|---------|
| `state_dir` | `~/.grove` |
| `workspace_dir` | `~/grove-workspaces/{project}` |

`state_dir` is flat with no `{project}` template. Runtime IDs already provide per-project uniqueness.

`workspace_dir` retains the `{project}` template for human-readable workspace paths.

### Config struct

```go
type Config struct {
    WarmupCommand string   `json:"warmup_command,omitempty"`
    WorkspaceDir  string   `json:"workspace_dir"`
    StateDir      string   `json:"state_dir"`
    MaxWorkspaces int      `json:"max_workspaces"`
    Exclude       []string `json:"exclude,omitempty"`
    CloneBackend  string   `json:"clone_backend,omitempty"`
}
```

### Directory layout

```
~/.grove/                              # state_dir
├── runtimes/
│   ├── a1b2c3d4e5f6/                 # runtime ID from .grove/.runtime-id
│   │   ├── images/
│   │   │   ├── base.sparsebundle
│   │   │   └── state.json
│   │   ├── shadows/
│   │   │   └── feature-auth-f7e8.shadow
│   │   └── workspaces/
│   │       └── feature-auth-f7e8.json
│   └── f6e5d4c3b2a1/                 # another project
│       └── ...

~/grove-workspaces/myproject/          # workspace_dir (expanded)
├── feature-auth-f7e8/                 # workspace directory
└── main-d9c0/                         # workspace directory
```

### Code changes

1. **`config.go`**: Add `StateDir` field. `DefaultConfig()` sets `StateDir: "~/.grove"` and `WorkspaceDir: "~/grove-workspaces/{project}"`.
2. **`ExpandStateDir()`**: New function. Expands `~/` to home directory. No `{project}` token.
3. **`ImageRuntimeRoot()`**: Derive path from `state_dir` instead of `workspace_dir`. Result: `state_dir/runtimes/{runtime-id}`.
4. **`BuildImageSyncExcludes()`**: Exclude both `workspace_dir` and `state_dir` from rsync when they fall inside the repo tree.
5. **`init.go`**: Add `--state-dir` flag. Include state dir in interactive wizard.
6. **`create.go`**: Pass expanded `state_dir` where needed.

### Migration

When grove loads a config without `state_dir` and finds `runtimes/` under the old `workspace_dir`, it auto-migrates:

1. Set `state_dir` to `~/.grove` in config.
2. Move `workspace_dir/runtimes/` to `state_dir/runtimes/`.
3. Save updated config.
4. Print a message explaining the migration.

No user intervention required.

### CLI surface

- `grove init --state-dir <path>`: override state directory
- `grove status`: display both `workspace_dir` and `state_dir`
- Interactive wizard: prompt for state dir with default shown
