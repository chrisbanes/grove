# Incremental Image Backend for Fast Workspace Create

Date: 2026-02-20  
Status: Approved

## Summary

Add an experimental image-based backend for workspace creation on macOS that replaces per-workspace `cp -c -R` clones with fast `hdiutil` attach operations using a shared base image plus per-workspace shadow files.

The base image is updated incrementally via `rsync` to avoid full rebuild cost. Workspace create becomes mount-time fast.

## Problem

For very large file-count repositories, raw APFS CoW directory clone (`cp -c -R`) can still take several minutes because it is metadata/inode heavy. Measured timing:

- `cp -c -R . ~/.grove/foo`
- `3.95s user`
- `260.60s system`
- `5:44.70 total`

This makes current synchronous clone-on-create unsuitable for latency-sensitive workspace creation in some repos.

## Goals

- Make `grove create` fast (seconds-scale, mount-time).
- Keep existing `cp` backend as default and fallback.
- Support incremental base refresh.
- Preserve workspace isolation semantics.
- Roll out incrementally behind an explicit backend selection.

## Non-Goals

- Replacing `cp` backend immediately.
- Cross-platform image backend in this phase.
- Byte-perfect zero-cost updates while active workspaces exist.

## Design

### Backend Selection

Add config field:

```json
{
  "clone_backend": "cp"
}
```

Allowed values:

- `cp` (default): existing behavior.
- `image`: experimental image backend.

CLI can override for testing:

- `grove create --backend image`

### Image Model

- Base image: `.grove/images/base.sparsebundle` (mutable APFS sparsebundle).
- Workspace shadow: `.grove/shadows/<workspace-id>.shadow`.
- Workspace metadata: `.grove/workspaces/<workspace-id>.json`.

Each workspace mounts the same base with its own shadow file:

```bash
hdiutil attach .grove/images/base.sparsebundle \
  -shadow .grove/shadows/<id>.shadow \
  -mountpoint <workspace_path> \
  -nobrowse -plist
```

### Core Safety Rule

Do not mutate base content while any image-backed workspace is active.

Reason: changing base blocks while shadows reference the same logical base can cause correctness and recovery complexity.

Enforcement:

- `grove update` for image backend fails with a clear message if active image workspaces exist.

### Lifecycle

#### `grove init` (image backend)

1. Create sparsebundle:
   - `hdiutil create -type SPARSEBUNDLE -fs APFS -volname grove-base ...`
2. Attach base at temporary mountpoint (for example `.grove/mnt/base`).
3. Seed base with `rsync -a --delete <golden>/ <base-mount>/`.
4. Detach base.
5. Write backend config and backend state file.

#### `grove create` (image backend)

1. Allocate workspace id/path.
2. Ensure mountpoint directory exists.
3. Attach base + shadow at workspace path.
4. Persist workspace metadata (device id, mount path, shadow path, base generation).
5. Write workspace marker.
6. Run post-clone hook.
7. Checkout requested branch.

#### `grove destroy` (image backend)

1. Resolve workspace metadata.
2. Detach mounted device.
3. Remove shadow file.
4. Remove workspace metadata.
5. Remove empty mountpoint directory.

#### `grove update` (image backend incremental refresh)

1. Confirm zero active image workspaces.
2. Attach base at temp mountpoint.
3. Run `rsync -a --delete <golden>/ <base-mount>/`.
4. Detach base.
5. Increment `base_generation` in backend state.

### State Files

#### Backend state

Path: `.grove/images/state.json`

```json
{
  "backend": "image",
  "base_path": ".grove/images/base.sparsebundle",
  "base_generation": 3,
  "last_sync_commit": "abc1234"
}
```

#### Workspace image metadata

Path: `.grove/workspaces/<id>.json`

```json
{
  "id": "main-a1b2",
  "mountpoint": "/tmp/grove/project/main-a1b2",
  "device": "/dev/disk7s1",
  "shadow_path": "/path/to/.grove/shadows/main-a1b2.shadow",
  "base_generation": 3,
  "created_at": "2026-02-20T16:22:00Z"
}
```

## Error Handling

- Base image missing:
  - Error with remediation (`grove init --backend image` or `grove update`).
- Attach fails:
  - Return error; no workspace record written.
- Hook or branch checkout fails:
  - Detach mount and clean shadow + metadata.
- Destroy detach fails (busy):
  - Return explicit error and keep metadata for recovery.
- Update with active workspaces:
  - Fail fast and list active image workspaces.

## Recovery and Doctoring

Add doctor checks for image backend:

- metadata exists but mount missing,
- mount exists but metadata missing,
- shadow missing,
- stale mounted devices under grove paths.

Provide safe repair actions:

- detach stale mounts,
- prune orphan metadata,
- report unresolved busy mounts.

## Incremental Rollout Plan

1. Add backend plumbing and config (`cp` default, no behavior change).
2. Add image backend state + init/update image management.
3. Add `create --backend image`.
4. Add image-aware destroy path.
5. Add doctor checks and cleanup tooling.
6. Add docs and mark feature experimental.

## Trade-Offs

Pros:

- `create` latency becomes mount-time fast.
- Update cost is incremental via `rsync`.
- `cp` backend remains available as stable fallback.

Cons:

- More lifecycle complexity (mounts/devices/shadows).
- Requires robust crash recovery.
- Active workspace rule blocks in-place base updates.

## Testing Strategy

Unit:

- metadata/state load/save validation,
- backend selection and fallback logic,
- active workspace gating for update.

Integration/E2E (macOS):

- image init/create/destroy happy path,
- update incremental refresh when no active workspaces,
- update refusal when active workspaces exist,
- recovery from partial failures.

