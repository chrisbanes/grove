# Configuration-Less Grove Design

## Goal

Make Grove feel configuration-less for first-time users while preserving
deterministic, repo-local behavior.

Primary target:

- Blend of zero-command onboarding and smart defaults.
- `grove create` should work in an unconfigured repo.
- Persist minimal local config for reproducibility.

## Approved Product Decisions

- First run in an unconfigured repo: `grove create` auto-initializes silently.
- Auto-init should not run warmup by default.
- Persist minimal config only (`workspace_dir` plus non-defaults).

## Approaches Considered

1. **Auto-init on create (recommended)**
   - Detect missing `.grove/config.json` inside `grove create`.
   - Infer safe defaults.
   - Persist minimal config.
   - Continue standard create flow.

2. **Ephemeral-first (not chosen)**
   - Infer config in-memory for `create`.
   - Defer persistence to later commands.
   - Rejected due to reduced reproducibility and surprising behavior.

3. **Global profile + local override (not chosen)**
   - Add user-level Grove config as baseline.
   - Rejected for now to preserve Grove's no-global-config simplicity.

## Recommended Architecture

Add an auto-config path in `create`:

1. Resolve repo root.
2. Attempt config load.
3. If config missing, synthesize minimal `internal/config.Config`.
4. Persist minimal config to `.grove/config.json`.
5. Resume normal `create` pipeline.

Design constraints:

- Keep one canonical config model (`internal/config.Config`).
- Treat auto-init as a producer of config, not a parallel mode.
- Use only local repository context for v1 inference.

## Inference and Persistence Rules (v1)

- Infer only `workspace_dir`.
- Default inferred `workspace_dir`: existing default template
  (`~/grove-workspaces/{project}`), unless an explicit CLI flag overrides.
- Leave `warmup_command` empty.
- Keep existing defaults for other fields.
- Persist only minimal config:
  - Always include `workspace_dir`.
  - Include other fields only when non-default.

## UX and Safety

### UX

- `grove create` succeeds on first use without requiring `grove config`.
- Emit a one-time informational note after auto-init:
  - Initialized Grove config at `.grove/config.json` using defaults.
  - Run `grove config` to customize.

### Safety

- Preserve current non-git repo error behavior.
- Preserve current dirty-tree safety checks.
- If `.grove/` write fails, return a clear actionable error.
- Auto-init must not bypass existing `create` validation and limits.

## Data Flow

`grove create`
-> load config
-> (missing) infer minimal config
-> save config
-> clone backend selection
-> workspace creation
-> marker/hook/branch flow

## Error Handling

- **Config missing**: trigger auto-init path.
- **Config malformed**: fail; do not auto-overwrite user file.
- **Permission denied while writing `.grove/`**: fail with fix guidance.
- **Existing create errors (filesystem, dirty tree, limits)**: unchanged.

## Testing Strategy

1. `create` on repo without `.grove/config.json` creates workspace and writes
   minimal config.
2. Saved config omits default values except `workspace_dir`.
3. Auto-init path does not set `warmup_command`.
4. Existing `create` behavior unchanged when config already exists.
5. Failure tests for write-permission and malformed config scenarios.

## Rollout

- Implement as a non-breaking enhancement to `grove create`.
- Update README command docs to reflect automatic first-run initialization.
- Keep `grove config` as the explicit customization command.
