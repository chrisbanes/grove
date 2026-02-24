# Rename `init` to `config` Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Rename `grove init` to `grove config` across the CLI, tests, docs, and skills.

**Architecture:** Pure rename. The cobra command variable changes from `initCmd` to `configCmd`, the file moves from `init.go` to `config.go`, and all references update. No behavioral changes.

**Tech Stack:** Go (cobra CLI), shell scripts, markdown docs.

---

### Task 1: Rename command file and update cobra registration

**Files:**
- Delete: `cmd/grove/init.go`
- Create: `cmd/grove/config.go`

**Step 1: Create `config.go` from `init.go` with all renames applied**

Copy `cmd/grove/init.go` to `cmd/grove/config.go` with these changes:
- `initCmd` -> `configCmd` (all occurrences)
- `Use: "init [path]"` -> `Use: "config [path]"`
- `Short: "Configure grove for a repository"` (unchanged — already accurate)
- `Long:` update to remove "Sets up" framing:
  ```go
  Long: `Configure grove settings for a git repository.
  Runs an interactive wizard when called without flags.
  Can be re-run to update configuration.`,
  ```
- `newProgressRenderer(os.Stderr, isTerminalFile(os.Stderr), "init")` -> `"config"`
- `fmt.Printf("Initializing grove for %s...\n\n", projectName)` -> `fmt.Printf("Configuring grove for %s...\n\n", projectName)`
- `fmt.Printf("Grove initialized at %s\n", absPath)` -> `fmt.Printf("Grove configured for %s\n", absPath)`
- In `func init()`: `rootCmd.AddCommand(initCmd)` -> `rootCmd.AddCommand(configCmd)`

**Step 2: Delete `init.go`**

```bash
rm cmd/grove/init.go
```

**Step 3: Build to verify compilation**

Run: `cd /private/var/folders/_w/lwqygqhd6197zzpc53gzz4t80000gn/T/vibe-kanban/worktrees/2648-workspace/grove && go build ./cmd/grove/`
Expected: Compiles with no errors.

**Step 4: Verify `grove config --help` works**

```bash
./grove config --help
```

Expected: Shows help with `grove config [path]` usage.

**Step 5: Commit**

```bash
git add cmd/grove/config.go
git rm cmd/grove/init.go
git commit -m "rename init command to config"
```

---

### Task 2: Update `migrate.go` error message

**Files:**
- Modify: `cmd/grove/migrate.go:48`

**Step 1: Update the error message**

Change:
```go
return fmt.Errorf("grove not initialized. Run `grove init` first to configure a backend before migrating")
```
To:
```go
return fmt.Errorf("grove not configured. Run `grove config` first to configure a backend before migrating")
```

**Step 2: Build to verify**

Run: `go build ./cmd/grove/`
Expected: Compiles.

**Step 3: Commit**

```bash
git add cmd/grove/migrate.go
git commit -m "update migrate error message to reference grove config"
```

---

### Task 3: Update e2e tests

**Files:**
- Modify: `test/e2e_test.go`

**Step 1: Replace all `"init"` command arguments with `"config"`**

Every call like `grove(t, binary, repo, "init")` or `grove(t, binary, repo, "init", "--backend", "image", ...)` must change `"init"` to `"config"`.

Do NOT change `"git", "init"` calls — those are `git init`, not grove init.

Specific lines to change (all `grove(...)` and `groveExpectErr(...)` calls with `"init"` as the subcommand):
- Line 123 comment: update "Skip grove init" -> "Skip grove config"
- Line 175: `grove(t, binary, repo, "init")` -> `"config"`
- Line 238: `grove(t, binary, repo, "init")` -> `"config"`
- Line 237 comment: `// grove init` -> `// grove config`
- Line 317: `"init"` -> `"config"`
- Line 357: `"init"` -> `"config"`
- Line 392: `"init"` -> `"config"`
- Line 432: `"init"` -> `"config"`
- Line 480: `"init"` -> `"config"`
- Line 508: `"init"` -> `"config"` and update comment
- Line 539: `groveExpectErr(t, binary, dir, "init")` -> `"config"`
- Line 547: `"init"` -> `"config"`
- Line 549: `"init"` -> `"config"`
- Line 557: `"init"` -> `"config"`
- Line 559: `"init"` -> `"config"`
- Line 572: `"init"` -> `"config"`
- Line 584: `"init"` -> `"config"`
- Line 602: `"init"` -> `"config"`
- Line 618: `"init"` -> `"config"`
- Line 639: `"init"` -> `"config"`
- Line 660: `"init"` -> `"config"`
- Line 686: `"init"` -> `"config"`
- Line 699: `"init"` -> `"config"`
- Line 708: `"init"` -> `"config"`
- Line 717: `"init"` -> `"config"`
- Line 738: `"init"` -> `"config"`
- Line 752: `"init"` -> `"config"`
- Line 816: `"init"` -> `"config"`
- Line 825: `"init"` -> `"config"`
- Line 880: `"init"` -> `"config"`
- Line 898: `"init"` -> `"config"`
- Line 934: `"init"` -> `"config"`
- Line 975: `"init"` -> `"config"`
- Line 1009: `"init"` -> `"config"`
- Line 1026: `"init"` -> `"config"`
- Line 1052: `"init"` -> `"config"`
- Line 1074: `"init"` -> `"config"`

Also update any test function names containing "Init" -> "Config" if applicable (check for `TestInit`, `TestReinit`, etc.).

**Step 2: Build tests to verify compilation**

Run: `go test -c ./test/ -o /dev/null`
Expected: Compiles.

**Step 3: Run tests**

Run: `go test ./test/ -v -count=1 -timeout 300s`
Expected: All tests pass.

**Step 4: Commit**

```bash
git add test/e2e_test.go
git commit -m "update e2e tests for init -> config rename"
```

---

### Task 4: Rename skill directory and update skill files

**Files:**
- Rename: `skills/grove-init/` -> `skills/grove-config/`
- Rename: `commands/grove-init.md` -> `commands/grove-config.md`
- Modify: `skills/grove-config/SKILL.md` (update all references)
- Modify: `commands/grove-config.md` (update references)
- Modify: `skills/using-grove/SKILL.md`
- Modify: `skills/grove-multi-agent/SKILL.md`
- Modify: `skills/grove-doctor/SKILL.md`
- Modify: `hooks/session-start.sh`

**Step 1: Rename directories and files**

```bash
git mv skills/grove-init skills/grove-config
git mv commands/grove-init.md commands/grove-config.md
```

**Step 2: Update `skills/grove-config/SKILL.md`**

- Header: `name: grove-init` -> `name: grove-config`
- Description: "Use when setting up Grove" -> "Use when configuring Grove"
- Announcement: "grove-init skill" -> "grove-config skill"
- Step 5: `grove init --warmup-command` -> `grove config --warmup-command`
- Step 7 commit message: keep as-is (it's about Grove configuration)
- Quick Reference table: `grove init` -> `grove config`
- Common Mistakes: `grove init` -> `grove config`
- Red Flags: `grove init` -> `grove config`
- Integration: `grove:grove-init` -> `grove:grove-config`, `/grove-init` -> `/grove-config`

**Step 3: Update `commands/grove-config.md`**

- Description: keep as-is (already says "Set up Grove for this project")
- Body: `grove:grove-init` -> `grove:grove-config`

**Step 4: Update `skills/using-grove/SKILL.md`**

- `grove:grove-init` -> `grove:grove-config`

**Step 5: Update `skills/grove-multi-agent/SKILL.md`**

- `grove init` -> `grove config`
- `grove:grove-init` -> `grove:grove-config`

**Step 6: Update `skills/grove-doctor/SKILL.md`**

- `grove:grove-init` -> `grove:grove-config`

**Step 7: Update `hooks/session-start.sh`**

- `grove:grove-init` -> `grove:grove-config`

**Step 8: Commit**

```bash
git add skills/ commands/ hooks/
git commit -m "rename grove-init skill to grove-config"
```

---

### Task 5: Update README and docs

**Files:**
- Modify: `README.md`
- Modify: `docs/DESIGN.md`
- Modify: `.github/ISSUE_TEMPLATE/bug_report.yml`

**Step 1: Update `README.md`**

Replace all `grove init` with `grove config` (lines 64, 81, 86, 92, 257, 407). Also update the `grove-init` skill reference to `grove-config`.

**Step 2: Update `docs/DESIGN.md`**

Replace all `grove init` with `grove config` (lines 28, 57, 66, 391, 418). Update descriptions as needed.

**Step 3: Update `.github/ISSUE_TEMPLATE/bug_report.yml`**

Replace `grove init` with `grove config` (line 45).

**Step 4: Commit**

```bash
git add README.md docs/DESIGN.md .github/ISSUE_TEMPLATE/bug_report.yml
git commit -m "update docs for init -> config rename"
```

---

### Task 6: Final verification

**Step 1: Full build**

```bash
go build ./cmd/grove/
```

**Step 2: Run full test suite**

```bash
go test ./... -count=1 -timeout 300s
```

**Step 3: Grep for stale references**

```bash
grep -r "grove init" --include="*.go" --include="*.md" --include="*.sh" --include="*.yml" .
```

Ignore hits in `docs/plans/` (historical design docs). Fix any remaining references.

**Step 4: Verify CLI help**

```bash
./grove --help
./grove config --help
```

Expected: `config` appears in the command list; `init` does not.
