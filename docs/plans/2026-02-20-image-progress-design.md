# Image Backend Progress Reporting

## Problem

`grove init --backend image`, `grove migrate --to image`, and `grove update` all perform a slow rsync of the entire golden copy into a sparse bundle. The user sees only "Initializing image backend..." with no indication of progress.

## Design

### Runner interface

Add a `Stream` method alongside the existing `CombinedOutput`:

```go
type Runner interface {
    CombinedOutput(name string, args ...string) ([]byte, error)
    Stream(name string, args []string, onLine func(string)) error
}
```

`Stream` starts the process, scans stdout splitting on `\r` and `\n`, and calls `onLine` for each chunk. The `execRunner` implementation pipes stdout through a `bufio.Scanner` with a custom split function. Mock implementations call `onLine` with canned output.

### Progress callback

Thread an optional `onProgress func(int, string)` through:

- `InitBase(repoRoot string, runner Runner, baseSizeGB int, onProgress func(int, string))`
- `RefreshBase(repoRoot, goldenRoot string, runner Runner, commit string, onProgress func(int, string))`

When non-nil, these functions call `onProgress(percent, phase)` at each stage:

| Phase | Percent range |
|-------|--------------|
| creating sparse bundle | 0% |
| syncing golden copy | 5–95% |
| done | 100% |

`RefreshBase` skips the "creating sparse bundle" phase.

### rsync progress parsing

Add `syncBaseWithProgress` that:

1. Appends `--info=progress2 --no-inc-recursive` to rsync args
2. Calls `Runner.Stream` to read output line-by-line
3. Parses `(\d+)%` from each line
4. Maps the rsync percentage into the 5–95% callback range

When `onProgress` is nil, `InitBase` and `RefreshBase` call the existing `SyncBase` unchanged.

### Backend interface

Extend `RefreshBase` to accept the callback:

```go
RefreshBase(goldenRoot, commit string, onProgress func(int, string)) error
```

The `cpBackend` ignores it (no-op). The `imageBackend` passes it through.

### CLI commands

Add `--progress` (opt-in) to `init`, `migrate`, and `update`. Each creates a `progressRenderer` with a command-specific label ("init", "migrate", "update") and passes the callback into the image functions.

Make `newFancyProgress` accept a label parameter instead of hardcoding "create".

### What stays the same

- `grove create --progress` behavior for both backends
- `SyncBase` (the non-progress code path)
- Tests using mock `Runner.CombinedOutput`
