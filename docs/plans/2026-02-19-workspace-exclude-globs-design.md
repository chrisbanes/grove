# Workspace Exclude Globs Design

Date: 2026-02-19
Status: Approved

## Summary

Add config-only `exclude_globs` support so `grove create` can skip selected ignored-path subsets during clone for faster workspace creation.

The design keeps safety strict: excludes may only apply to paths that Git already ignores, and creation fails if any matched path is tracked or unignored.

## Goals

- Improve `grove create` performance when large ignored directories are unnecessary in child workspaces.
- Keep default behavior unchanged when no excludes are configured.
- Enforce safe excludes so users cannot accidentally remove tracked source files.

## Non-Goals

- No new `grove create` CLI flags in this phase.
- No support for excluding tracked files.
- No changes to workspace lifecycle semantics outside clone filtering.

## Approaches Considered

1. Create-time strict validation + filtered clone (selected)
   - Validate configured globs at create time against the current golden copy.
   - Enforce ignored-only subset semantics.
   - Perform filtered clone only after validation succeeds.
2. Snapshot allowlist
   - Build an ignored-path cache during `grove init`/`grove update` and validate against it.
   - Rejected for now due to stale-cache risk.
3. Best-effort warnings
   - Warn on non-ignored matches but continue.
   - Rejected because it weakens safety guarantees.

## User-Facing Behavior

### Config

`.grove/config.json` gains:

```json
{
  "exclude_globs": [
    ".gradle/caches/**",
    "node_modules/**"
  ]
}
```

### Validation Rules

- Globs are repository-relative.
- Invalid patterns fail `grove create`.
- Absolute paths and `..` traversal are rejected.
- `.git/**` and `.grove/**` are always rejected.
- If a glob matches paths:
  - every matched path must be ignored by Git,
  - otherwise `grove create` fails with the glob and offending path.
- If a glob matches nothing:
  - `grove create` continues,
  - emit a non-fatal warning.

### Clone Behavior

- `exclude_globs` empty: use existing full CoW clone path unchanged.
- `exclude_globs` non-empty: run filtered CoW clone and skip validated excluded paths.

## Technical Design

### Configuration Model

- Extend `internal/config.Config` with:
  - `ExcludeGlobs []string \`json:"exclude_globs,omitempty"\``
- Preserve backward compatibility:
  - missing field behaves as empty list.

### Create Flow

1. Load config.
2. If `exclude_globs` is empty, use current clone flow.
3. Validate glob syntax and safety constraints.
4. Expand globs against golden copy path.
5. Warn for unmatched globs.
6. For matched paths, verify ignored status using Git ignore resolution.
7. If validation passes, run filtered clone.
8. Continue existing flow: write marker, hooks, branch checkout.

### Ignored-Only Verification

- Use git’s own ignore engine to avoid re-implementing rules.
- Batch-check candidate paths where possible for performance.
- Treat any non-ignored match as a hard error.

### Filtered Clone Strategy

- Add a clone path that walks the source tree once and prunes excluded subtrees early.
- Copy included files/directories with CoW-preserving operations.
- Keep progress output phase-aware:
  - `validate-excludes`
  - `filtered-clone`
  - existing post-clone phases

## Error Handling

- Invalid glob syntax: fail create.
- Unsafe glob target (`.git`, `.grove`, absolute, traversal): fail create.
- Matched non-ignored path: fail create.
- Unmatched glob: warning only.
- Existing clone failure cleanup behavior remains unchanged.

## Testing Strategy

1. Config tests
   - Load/save with and without `exclude_globs`.
2. Validation tests
   - valid ignored matches pass,
   - non-ignored matches fail,
   - unmatched globs warn and continue,
   - protected/unsafe globs fail.
3. Workspace tests
   - empty excludes uses legacy path,
   - excluded ignored paths absent in workspace,
   - included paths still cloned correctly.
4. E2E tests
   - fixture repo with tracked + ignored files verifies success/failure matrix.

## Rollout

1. Add config field and parser coverage.
2. Implement validation + warnings.
3. Implement filtered clone path.
4. Add unit + e2e coverage.
5. Document usage in `README.md` with examples and warning semantics.
