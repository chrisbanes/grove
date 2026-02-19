# Exclude Globs for Workspace Creation

## Problem

Grove clones the entire golden copy via `cp -c -R`, including files that are either expensive to clone (large caches) or cause correctness issues when cloned (lock files, PID files, sockets). Post-clone hooks can delete these after the fact, but for performance and correctness the files should never be copied at all.

## Design

### Configuration

Add an `exclude` field to `.grove/config.json`:

```json
{
  "workspace_dir": "/tmp/grove/{project}",
  "max_workspaces": 10,
  "exclude": [
    "*.lock",
    "__pycache__",
    ".gradle/configuration-cache"
  ]
}
```

In Go:

```go
type Config struct {
    WarmupCommand string   `json:"warmup_command,omitempty"`
    WorkspaceDir  string   `json:"workspace_dir"`
    MaxWorkspaces int      `json:"max_workspaces"`
    Exclude       []string `json:"exclude,omitempty"`
}
```

Empty or absent `exclude` preserves current behavior (clone everything).

### Glob Matching Rules

Patterns use `filepath.Match` semantics (no external dependencies).

- **No `/` in pattern** — match against the basename of each entry at any depth. `*.lock` matches `yarn.lock` anywhere in the tree. `__pycache__` matches any directory with that name.
- **Contains `/`** — match against the full relative path from the repo root. `.gradle/configuration-cache` matches only that specific path.

This mirrors `.gitignore` behavior and feels familiar.

### Validation

All exclude patterns are validated at config load time via `filepath.Match`. Invalid patterns produce a clear error:

```
grove: invalid exclude pattern ".gradle/[bad": syntax error in pattern
```

The `.grove` directory itself is never excludable — it's required for the workspace marker.

### Clone Algorithm

Replace the single `cp -c -R` with a planned, selective clone.

**Phase 1 — Plan & Count**

Walk the golden copy with `filepath.WalkDir`. For each entry:

1. Compute the relative path from the repo root.
2. Check against exclude globs using the matching rules above.
3. If excluded and a directory, return `fs.SkipDir` (never descend).
4. If excluded and a file, skip.
5. If not excluded, count it.

Outputs:
- Total non-excluded entry count (for progress).
- Set of directories that contain excluded descendants (for phase 2).

**Phase 2 — Selective Clone**

Clone at the highest possible subtree granularity:

1. Create the destination root directory.
2. List direct children of the source root.
3. For each child:
   - If excluded — skip entirely.
   - If it contains no excluded descendants — `cp -c -R` the whole subtree (fast path, same as today).
   - If it contains excluded descendants — `mkdir` in destination, recurse into it.
4. Track `-v` output lines across all `cp -c -R` calls for progress.

This minimizes the number of `cp -c -R` invocations. A repo with excludes only in `.gradle/` would run one `cp -c -R` per top-level directory except `.gradle/`, then recurse into `.gradle/` and `cp -c -R` its non-excluded children.

### Progress (`--progress`)

The plan phase replaces the existing `countEntries` scan — it walks the tree once and produces both the clone plan and the total entry count. No second walk needed.

During the clone phase, each `cp -c -R -v` call emits per-file output lines. These are summed across all calls to produce a single progress counter against the pre-computed total, preserving the existing progress UX.

### Cloner Interface

`APFSCloner` and the `Cloner` interface remain unchanged. The selective clone orchestration lives in new functions in the `clone` package:

```go
func SelectiveClone(cloner Cloner, src, dst string, excludes []string) error

func SelectiveCloneWithProgress(cloner Cloner, src, dst string, excludes []string, onProgress ProgressFunc) error
```

When `excludes` is empty, these fall back to a single `cloner.Clone(src, dst)` — zero overhead.

### Workspace Create Integration

`workspace.Create` calls `clone.SelectiveClone` instead of `cloner.Clone` directly, passing `cfg.Exclude` through. No changes to `CreateOpts` or the CLI flags — excludes are purely config-driven.

### Error Handling

- **Invalid globs** — validated at config load time; clear error message.
- **Partial clone failure** — clean up the entire destination directory (`os.RemoveAll`), same as today.
- **`.grove` in excludes** — ignored silently (or error); the workspace marker must always be written.
