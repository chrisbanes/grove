# Grove Create Progress Design

Date: 2026-02-19
Status: Approved

## Summary

Add an opt-in `--progress` flag to `grove create` so users can see determinate progress during long clone operations in large repositories.

This design keeps existing behavior unchanged unless `--progress` is provided, preserves machine-readable `--json` output on `stdout`, and sends progress output to `stderr`.

## Goals

- Provide visible progress for long-running `create` operations.
- Keep default CLI behavior unchanged.
- Preserve `--json` compatibility for automation and agent workflows.
- Prefer real clone progress signals over synthetic timers.

## Non-Goals

- No global progress system across all commands in this iteration.
- No byte-level copy progress guarantees.
- No UI changes outside `grove create`.

## User-Facing Behavior

### New Flag

- `grove create --progress`

### Output Rules

- `--progress` disabled:
  - Existing output remains unchanged.
- `--progress` enabled + TTY:
  - Show a single-line determinate progress bar with percentage and phase text.
- `--progress` enabled + non-TTY:
  - Show periodic line-based progress updates for log readability.
- `--json` with `--progress`:
  - Progress output on `stderr`.
  - Final JSON output on `stdout` only.

### Progress Phases and Bounds

- `0-5%`: preflight checks and setup.
- `5-95%`: clone progress.
- `95-99%`: post-clone hook.
- `99-100%`: branch checkout and finalize.

## Technical Design

### Clone Progress Model

The clone stage is instrumented through BSD `cp` verbose output:

- Pre-count source filesystem entries before clone (used as denominator).
- Run clone as `cp -c -R -v`.
- Parse verbose output lines as copied entries.
- Map `copied/total` to the `5-95%` range.

This is real operational progress (entry-based), not time-based simulation.

### Internal Interfaces

Keep the existing `clone.Cloner` interface for compatibility and introduce an optional progress-capable interface:

```go
type ProgressEvent struct {
    Copied int
    Total  int
    Phase  string // "scan" | "clone"
}

type ProgressFunc func(ProgressEvent)

type ProgressCloner interface {
    CloneWithProgress(src, dst string, onProgress ProgressFunc) error
}
```

`APFSCloner` implements `CloneWithProgress`. Command code feature-detects support via type assertion and falls back gracefully when unsupported.

### Layering

- `internal/clone`: emits progress events (no UI formatting).
- `cmd/grove/create.go`: owns progress rendering decisions and stream selection (`stderr`).

This preserves clean separation between domain logic and CLI presentation.

## Error Handling

- If source entry counting fails, continue with stage-only progress (no hard failure).
- If verbose parsing misses lines, clamp progress and continue (monotonic, non-regressing `%`).
- Existing clone failure behavior remains:
  - clone error returned,
  - partial workspace cleaned.
- If hook fails, keep existing cleanup + error semantics.
- If branch checkout fails, keep existing warning semantics.
- Progress renderer prints a final failure/completion state before exit.

## Testing Strategy

### Unit Tests

- Parse `cp -v` output line handling.
- Percentage mapping and clamping behavior.
- Renderer monotonicity and phase transitions.
- TTY vs non-TTY mode selection.

### End-to-End Tests

- `create --progress --json` returns valid JSON on `stdout`.
- `create --progress` emits progress to `stderr`.
- `create` without `--progress` remains unchanged.

## Trade-Offs

- Entry-based progress is more accurate than timers, but not byte-accurate.
- Pre-scan adds overhead, but acceptable for opt-in progress mode.
- Parsing command output is platform-coupled, but acceptable given current APFS/macOS scope.

## Rollout

1. Implement `--progress` flag and renderer scaffolding.
2. Add progress-capable clone path for APFS cloner.
3. Add unit and e2e coverage.
4. Validate JSON contract and non-progress regressions.
