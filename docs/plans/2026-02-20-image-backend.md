# Incremental Image Backend Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add an experimental macOS image backend that makes `grove create` mount-time fast by using a shared APFS sparsebundle base plus per-workspace shadow files, with incremental base refresh on `grove update`.

**Architecture:** Keep `cp` as the default backend and add `image` as an opt-in backend in config/flags. Implement image operations in a new `internal/image` package (hdiutil + rsync wrappers, state/metadata). Wire `init`, `create`, `destroy`, and `update` to route to image flows when backend is `image`, while preserving existing workspace marker/list behavior.

**Tech Stack:** Go 1.25, Cobra CLI, `hdiutil` + `rsync` on macOS, existing `test/e2e_test.go` harness, `@test-driven-development`, `@verification-before-completion`.

---

### Task 1: Add Config Surface for Backend Selection

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Step 1: Write the failing tests**

Add tests in `internal/config/config_test.go` for:
- defaulting `clone_backend` to `"cp"` when missing,
- accepting `"image"`,
- rejecting unknown backend values.

```go
func TestLoad_DefaultCloneBackend(t *testing.T) {
  // config without clone_backend should load with "cp"
}

func TestLoad_InvalidCloneBackend(t *testing.T) {
  // clone_backend: "bad" should return clear error
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config -run 'CloneBackend|DefaultCloneBackend' -v`  
Expected: FAIL (missing field/validation/default).

**Step 3: Write minimal implementation**

In `internal/config/config.go`:

```go
type Config struct {
  WarmupCommand string   `json:"warmup_command,omitempty"`
  WorkspaceDir  string   `json:"workspace_dir"`
  MaxWorkspaces int      `json:"max_workspaces"`
  Exclude       []string `json:"exclude,omitempty"`
  CloneBackend  string   `json:"clone_backend,omitempty"`
}

func normalizeCloneBackend(v string) (string, error) {
  if v == "" {
    return "cp", nil
  }
  switch v {
  case "cp", "image":
    return v, nil
  default:
    return "", fmt.Errorf("invalid clone_backend %q: expected cp or image", v)
  }
}
```

Then call `normalizeCloneBackend` from `Load`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config -v`  
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add clone backend selection with validation"
```

### Task 2: Implement Disk Image Command Wrapper (hdiutil/rsync)

**Files:**
- Create: `internal/image/commands.go`
- Create: `internal/image/commands_test.go`

**Step 1: Write the failing tests**

Create `internal/image/commands_test.go` with tests for:
- attach plist parsing to extract device,
- command argument construction for create/attach/detach/sync,
- surface stderr on command failures.

Use an injected runner:

```go
type Runner interface {
  CombinedOutput(name string, args ...string) ([]byte, error)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/image -run 'TestAttach|TestCreate|TestSync' -v`  
Expected: FAIL (package/files missing).

**Step 3: Write minimal implementation**

In `internal/image/commands.go` implement:

```go
type AttachedVolume struct {
  Device     string
  MountPoint string
}

func CreateSparseBundle(r Runner, path, volName string, sizeGB int) error
func AttachWithShadow(r Runner, basePath, shadowPath, mountPoint string) (*AttachedVolume, error)
func Detach(r Runner, device string) error
func SyncBase(r Runner, src, dst string) error
```

Command shapes:
- `hdiutil create -type SPARSEBUNDLE -fs APFS -size <N>g -volname <name> <path>`
- `hdiutil attach <base> -shadow <shadow> -mountpoint <mount> -nobrowse -plist`
- `hdiutil detach <device>`
- `rsync -a --delete --exclude .grove/ <src>/ <dst>/`

**Step 4: Run test to verify it passes**

Run: `go test ./internal/image -v`  
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/image/commands.go internal/image/commands_test.go
git commit -m "feat(image): add hdiutil and rsync command wrappers"
```

### Task 3: Add Image Backend State and Workspace Metadata Store

**Files:**
- Create: `internal/image/state.go`
- Create: `internal/image/state_test.go`

**Step 1: Write the failing tests**

Add tests for:
- state file create/load round-trip (`.grove/images/state.json`),
- workspace metadata add/list/remove (`.grove/workspaces/<id>.json`),
- active workspace count,
- generation increment persistence.

```go
func TestStateRoundTrip(t *testing.T) {}
func TestWorkspaceMetaLifecycle(t *testing.T) {}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/image -run 'State|WorkspaceMeta' -v`  
Expected: FAIL (missing state store).

**Step 3: Write minimal implementation**

In `internal/image/state.go`:

```go
type State struct {
  Backend        string `json:"backend"`
  BasePath       string `json:"base_path"`
  BaseGeneration int    `json:"base_generation"`
  LastSyncCommit string `json:"last_sync_commit,omitempty"`
}

type WorkspaceMeta struct {
  ID             string    `json:"id"`
  Mountpoint     string    `json:"mountpoint"`
  Device         string    `json:"device"`
  ShadowPath     string    `json:"shadow_path"`
  BaseGeneration int       `json:"base_generation"`
  CreatedAt      time.Time `json:"created_at"`
}
```

Plus helpers:
- `LoadState(repoRoot string) (*State, error)`
- `SaveState(repoRoot string, st *State) error`
- `SaveWorkspaceMeta(repoRoot string, meta *WorkspaceMeta) error`
- `LoadWorkspaceMeta(repoRoot, id string) (*WorkspaceMeta, error)`
- `ListWorkspaceMeta(repoRoot string) ([]WorkspaceMeta, error)`
- `DeleteWorkspaceMeta(repoRoot, id string) error`

**Step 4: Run test to verify it passes**

Run: `go test ./internal/image -v`  
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/image/state.go internal/image/state_test.go
git commit -m "feat(image): add persistent state and workspace metadata store"
```

### Task 4: Wire `init` and `update` for Image Backend (Incremental Refresh)

**Files:**
- Modify: `cmd/grove/init.go`
- Modify: `cmd/grove/update.go`
- Create: `internal/image/backend.go`
- Create: `internal/image/backend_test.go`

**Step 1: Write the failing tests**

Add unit tests in `internal/image/backend_test.go`:
- `InitBase` creates sparsebundle + state,
- `RefreshBase` refuses when active workspace metadata exists,
- `RefreshBase` runs attach/sync/detach and increments generation.

Use fake runner and temp dirs.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/image -run 'InitBase|RefreshBase' -v`  
Expected: FAIL (missing backend orchestration).

**Step 3: Write minimal implementation**

In `internal/image/backend.go`, implement:

```go
func InitBase(repoRoot string, runner Runner, baseSizeGB int) (*State, error)
func RefreshBase(repoRoot, goldenRoot string, runner Runner, commit string) (*State, error)
```

Then wire commands:

- `cmd/grove/init.go`:
  - add `--backend` flag (`cp` default),
  - persist `cfg.CloneBackend`,
  - when `image`, call `image.InitBase(...)`.

- `cmd/grove/update.go`:
  - after pull/warmup, if `cfg.CloneBackend == "image"`, call `image.RefreshBase(...)`.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/image ./cmd/grove -v`  
Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/grove/init.go cmd/grove/update.go internal/image/backend.go internal/image/backend_test.go
git commit -m "feat(image): support init/update image backend with incremental refresh"
```

### Task 5: Wire `create` and `destroy` for Image Workspaces

**Files:**
- Modify: `cmd/grove/create.go`
- Modify: `cmd/grove/destroy.go`
- Create: `internal/image/workspace.go`
- Create: `internal/image/workspace_test.go`
- Modify: `internal/workspace/workspace.go`

**Step 1: Write the failing tests**

Add tests for image workspace lifecycle in `internal/image/workspace_test.go`:
- create mounts base with new shadow + writes metadata,
- failure path detaches and cleans shadow/meta,
- destroy detaches and removes shadow/meta.

Add focused e2e in `test/e2e_test.go`:

```go
func TestImageBackendLifecycle(t *testing.T) {
  // init --backend image, create --backend image --json, destroy
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/image ./test -run ImageBackendLifecycle -v`  
Expected: FAIL (create/destroy image route missing).

**Step 3: Write minimal implementation**

In `internal/image/workspace.go`:

```go
func CreateWorkspace(repoRoot, goldenRoot, workspacePath, workspaceID string, st *State, runner Runner) (*WorkspaceMeta, error)
func DestroyWorkspace(repoRoot, workspaceID string, runner Runner) error
```

In `cmd/grove/create.go`:
- if backend is `image`, skip `clone.NewCloner` path and call `image.CreateWorkspace(...)`,
- preserve existing hook + branch checkout + output behavior.

In `cmd/grove/destroy.go`:
- resolve workspace id/path,
- if image metadata exists for workspace, use image destroy path,
- else fallback to existing `workspace.Destroy`.

In `internal/workspace/workspace.go`:
- export marker writer helper for reuse:

```go
func WriteMarker(wsPath string, info *Info) error
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/image ./internal/workspace ./test -v`  
Expected: PASS on macOS.

**Step 5: Commit**

```bash
git add cmd/grove/create.go cmd/grove/destroy.go internal/image/workspace.go internal/image/workspace_test.go internal/workspace/workspace.go test/e2e_test.go
git commit -m "feat(image): add image-backed workspace create and destroy flows"
```

### Task 6: Document and Validate Operational Behavior

**Files:**
- Modify: `README.md`
- Modify: `docs/DESIGN.md`

**Step 1: Write failing documentation checklist**

Checklist:
- `clone_backend` config documented,
- `init --backend image` documented as experimental,
- update behavior notes active-workspace restriction,
- create/destroy behavior for image backend documented.

**Step 2: Validate missing entries**

Run: `rg -n "clone_backend|--backend image|sparsebundle|shadow|active workspaces" README.md docs/DESIGN.md`  
Expected: missing entries before docs update.

**Step 3: Write minimal docs**

Update:
- `README.md` command/config sections,
- `docs/DESIGN.md` architecture and lifecycle sections,
- include explicit warning that image backend update is blocked while image workspaces are active.

**Step 4: Run verification**

Run:
- `go test ./...`
- `rg -n "clone_backend|--backend image|sparsebundle|shadow|active workspaces" README.md docs/DESIGN.md`

Expected: tests pass, docs terms present.

**Step 5: Commit**

```bash
git add README.md docs/DESIGN.md
git commit -m "docs: describe experimental image backend and incremental update flow"
```

