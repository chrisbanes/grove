# Grove Improvements Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Improve workspace IDs to include branch names, clarify `--branch` documentation, and notarize macOS binaries.

**Architecture:** Three independent changes: (1) modify `generateID` to accept a branch name, slugify it, and prepend it to the random suffix; (2) update help text and README for `--branch`; (3) add codesigning and notarization to the GoReleaser release workflow.

**Tech Stack:** Go, Cobra, GoReleaser, Apple `codesign` + `notarytool`, GitHub Actions

---

### Task 1: Add `slugify` helper and update `generateID`

**Files:**
- Modify: `internal/workspace/workspace.go:172-178`

**Step 1: Write the failing test**

Add to `internal/workspace/workspace_test.go`:

```go
func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"main", "main"},
		{"feat/login-page", "feat-login-page"},
		{"agent/fix-auth-module-refactor-something", "agent-fix-auth-modul"},
		{"UPPER/Case", "upper-case"},
		{"feat//double--slash", "feat-double-slash"},
		{"trailing-slash/", "trailing-slash"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := workspace.Slugify(tt.input)
			if got != tt.want {
				t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGenerateID_WithBranch(t *testing.T) {
	id, err := workspace.GenerateID("feat/login")
	if err != nil {
		t.Fatal(err)
	}
	// Should be "feat-login-XXXX" where XXXX is 4 hex chars
	if !strings.HasPrefix(id, "feat-login-") {
		t.Errorf("expected prefix 'feat-login-', got %q", id)
	}
	// Total length: slug + "-" + 4 hex = 11 + 4 = 15
	suffix := id[len("feat-login-"):]
	if len(suffix) != 4 {
		t.Errorf("expected 4-char hex suffix, got %q", suffix)
	}
}

func TestGenerateID_Empty(t *testing.T) {
	id, err := workspace.GenerateID("")
	if err != nil {
		t.Fatal(err)
	}
	// No branch: just 4 hex chars
	if len(id) != 4 {
		t.Errorf("expected 4-char hex ID, got %q (len %d)", id, len(id))
	}
}
```

Note: add `"strings"` to the import block.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/workspace/ -run "TestSlugify|TestGenerateID" -v`
Expected: FAIL — `Slugify` and `GenerateID` are not exported / don't exist yet.

**Step 3: Write minimal implementation**

In `internal/workspace/workspace.go`, replace `generateID`:

```go
// Slugify converts a branch name to a URL/filesystem-safe slug.
// Lowercases, replaces non-alphanumeric chars with hyphens, collapses
// consecutive hyphens, truncates to 20 chars, and trims trailing hyphens.
func Slugify(branch string) string {
	if branch == "" {
		return ""
	}
	var b strings.Builder
	branch = strings.ToLower(branch)
	prev := false
	for _, r := range branch {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			prev = false
		} else if !prev && b.Len() > 0 {
			b.WriteByte('-')
			prev = true
		}
	}
	s := b.String()
	s = strings.TrimRight(s, "-")
	if len(s) > 20 {
		s = s[:20]
		s = strings.TrimRight(s, "-")
	}
	return s
}

// GenerateID creates a workspace ID. If branch is non-empty, the ID is
// "{branch-slug}-{4-hex}". Otherwise it is just "{4-hex}".
func GenerateID(branch string) (string, error) {
	bytes := make([]byte, 2)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	suffix := hex.EncodeToString(bytes)
	slug := Slugify(branch)
	if slug == "" {
		return suffix, nil
	}
	return slug + "-" + suffix, nil
}
```

Add `"strings"` to the import block in `workspace.go`.

Delete the old unexported `generateID` function (lines 172-178).

Update the call site in `Create` (line 43) from `generateID()` to `GenerateID(opts.Branch)`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/workspace/ -run "TestSlugify|TestGenerateID" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/workspace/workspace.go internal/workspace/workspace_test.go
git commit -m "feat: branch-based workspace IDs with slugified branch names"
```

---

### Task 2: Detect current branch when `--branch` is omitted

**Files:**
- Modify: `cmd/grove/create.go:69-73`

**Step 1: Write the failing test**

Add to `test/e2e_test.go`:

```go
func TestCreateIDIncludesBranch(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}
	binary := buildGrove(t)
	repo := setupTestRepo(t)
	grove(t, binary, repo, "init")

	// Create with explicit branch
	out := grove(t, binary, repo, "create", "--json", "--branch", "feat/my-feature")
	var info workspace.Info
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		t.Fatalf("invalid JSON: %s\n%s", err, out)
	}
	if !strings.HasPrefix(info.ID, "feat-my-feature-") {
		t.Errorf("expected ID to start with 'feat-my-feature-', got %q", info.ID)
	}
	grove(t, binary, repo, "destroy", "--all")

	// Create without branch — should use golden copy's current branch
	out = grove(t, binary, repo, "create", "--json")
	var info2 workspace.Info
	if err := json.Unmarshal([]byte(out), &info2); err != nil {
		t.Fatalf("invalid JSON: %s\n%s", err, out)
	}
	// The golden copy is on whatever branch git init defaults to (main/master)
	currentBranch := run(t, repo, "git", "branch", "--show-current")
	expectedPrefix := strings.ReplaceAll(strings.ToLower(currentBranch), "/", "-") + "-"
	if !strings.HasPrefix(info2.ID, expectedPrefix) {
		t.Errorf("expected ID to start with %q, got %q", expectedPrefix, info2.ID)
	}
	grove(t, binary, repo, "destroy", "--all")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./test/ -run TestCreateIDIncludesBranch -v -count=1`
Expected: FAIL — `Create` does not yet detect the current branch when `--branch` is omitted.

**Step 3: Write minimal implementation**

In `cmd/grove/create.go`, after the `branch` variable is read (line 69), add current-branch detection:

```go
branch, _ := cmd.Flags().GetString("branch")

// If no branch specified, detect the golden copy's current branch
branchForID := branch
if branchForID == "" {
	if detected, err := gitpkg.CurrentBranch(goldenRoot); err == nil {
		branchForID = detected
	}
}

opts := workspace.CreateOpts{
	Branch:       branch,
	BranchForID:  branchForID,
	GoldenCommit: commit,
}
```

Update `CreateOpts` in `internal/workspace/workspace.go`:

```go
type CreateOpts struct {
	Branch       string
	BranchForID  string
	GoldenCommit string
}
```

Update the `Create` function to use `opts.BranchForID` for ID generation:

```go
id, err := GenerateID(opts.BranchForID)
```

**Step 4: Run test to verify it passes**

Run: `go test ./test/ -run TestCreateIDIncludesBranch -v -count=1`
Expected: PASS

**Step 5: Run full test suite**

Run: `go test ./... -count=1`
Expected: PASS — some existing tests may need ID assertion updates since IDs now have branch prefixes.

**Step 6: Fix any broken tests**

Existing tests that assert on ID format (e.g., `TestCreate` checking `info.ID != ""`) should still pass since branch-based IDs are non-empty. But tests that use IDs for destroy/resolve may need to parse the new format. Check and fix as needed.

**Step 7: Commit**

```bash
git add cmd/grove/create.go internal/workspace/workspace.go internal/workspace/workspace_test.go test/e2e_test.go
git commit -m "feat: include current branch in workspace ID when --branch is omitted"
```

---

### Task 3: Clarify `--branch` documentation

**Files:**
- Modify: `cmd/grove/create.go:118`
- Modify: `cmd/grove/create.go:20-21`
- Modify: `README.md:98-129`

**Step 1: Update Cobra flag description**

In `cmd/grove/create.go`, change line 118:

```go
createCmd.Flags().String("branch", "", "Create and checkout a new git branch in the workspace (default: golden copy's current branch)")
```

**Step 2: Update long description**

In `cmd/grove/create.go`, update the `Long` field:

```go
Long: `Creates a copy-on-write clone of the golden copy, including all build
caches and gitignored files. Builds in the workspace start warm.

Without --branch, the workspace stays on the golden copy's current branch.
With --branch, a new git branch is created and checked out in the workspace.`,
```

**Step 3: Update README**

In `README.md`, update the `grove create` section (~lines 98-129):

```markdown
### `grove create`

Create a CoW clone workspace from the golden copy. Without `--branch`, the
workspace stays on the golden copy's current branch. With `--branch`, Grove
creates and checks out a new git branch in the workspace.

` ` `bash
grove create --branch feature/auth
# Workspace created: feature-auth-f7e8
# Path: /tmp/grove/myproject/feature-auth-f7e8
# Branch: feature/auth
` ` `

Without `--branch`:

` ` `bash
grove create
# Workspace created: main-d9c0
# Path: /tmp/grove/myproject/main-d9c0
` ` `
```

Also update the `--branch` flag description in the table:

```markdown
| `--branch` | Create and checkout a new git branch in the workspace (default: golden copy's current branch) |
```

Update the example output in the Quick Start and AI agent sections to use the new ID format (e.g., `feature-new-login-a1b2` instead of `a1b2c3d4`).

**Step 4: Commit**

```bash
git add cmd/grove/create.go README.md
git commit -m "docs: clarify --branch flag behavior and update examples for branch-based IDs"
```

---

### Task 4: Add Apple code signing to GoReleaser

**Files:**
- Modify: `.goreleaser.yml`

**Step 1: Add `signs` configuration**

In `.goreleaser.yml`, add a `signs` block after the `builds` section:

```yaml
signs:
  - cmd: codesign
    args:
      - "--force"
      - "--options"
      - "runtime"
      - "--sign"
      - "{{ .Env.APPLE_DEVELOPER_ID }}"
      - "${artifact}"
    artifacts: binary
```

**Step 2: Commit**

```bash
git add .goreleaser.yml
git commit -m "feat: add macOS code signing to GoReleaser config"
```

---

### Task 5: Add notarization to release workflow

**Files:**
- Modify: `.github/workflows/release.yml`

**Step 1: Add certificate import step**

In `.github/workflows/release.yml`, add steps before GoReleaser:

```yaml
- name: Import Apple certificate
  env:
    APPLE_CERTIFICATE_P12: ${{ secrets.APPLE_CERTIFICATE_P12 }}
    APPLE_CERTIFICATE_PASSWORD: ${{ secrets.APPLE_CERTIFICATE_PASSWORD }}
  run: |
    CERT_PATH=$RUNNER_TEMP/certificate.p12
    KEYCHAIN_PATH=$RUNNER_TEMP/app-signing.keychain-db
    echo -n "$APPLE_CERTIFICATE_P12" | base64 --decode -o $CERT_PATH
    security create-keychain -p "" $KEYCHAIN_PATH
    security set-keychain-settings $KEYCHAIN_PATH
    security unlock-keychain -p "" $KEYCHAIN_PATH
    security import $CERT_PATH -k $KEYCHAIN_PATH -P "$APPLE_CERTIFICATE_PASSWORD" -T /usr/bin/codesign
    security set-key-partition-list -S apple-tool:,apple: -k "" $KEYCHAIN_PATH
    security list-keychains -d user -s $KEYCHAIN_PATH login.keychain-db
```

**Step 2: Pass signing env to GoReleaser**

Update the GoReleaser step to include the signing identity:

```yaml
- uses: goreleaser/goreleaser-action@v6
  with:
    version: "~> v2"
    args: release --clean
  env:
    GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
    HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}
    APPLE_DEVELOPER_ID: ${{ secrets.APPLE_DEVELOPER_ID }}
```

**Step 3: Add notarization step after GoReleaser**

```yaml
- name: Notarize binaries
  env:
    APPLE_ID: ${{ secrets.APPLE_ID }}
    APPLE_ID_PASSWORD: ${{ secrets.APPLE_ID_PASSWORD }}
    APPLE_TEAM_ID: ${{ secrets.APPLE_TEAM_ID }}
  run: |
    for archive in dist/*.tar.gz; do
      echo "Notarizing $archive..."
      xcrun notarytool submit "$archive" \
        --apple-id "$APPLE_ID" \
        --password "$APPLE_ID_PASSWORD" \
        --team-id "$APPLE_TEAM_ID" \
        --wait
    done
```

**Step 4: Add keychain cleanup**

```yaml
- name: Cleanup keychain
  if: always()
  run: |
    security delete-keychain $RUNNER_TEMP/app-signing.keychain-db || true
```

**Step 5: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "feat: add Apple notarization to release workflow"
```

---

### Task 6: Run full test suite and verify

**Step 1: Run all tests**

Run: `go test ./... -count=1 -v`
Expected: All pass.

**Step 2: Build and verify locally**

Run: `go build ./cmd/grove && ./grove version`
Expected: Builds and runs.

**Step 3: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "fix: address test issues from improvements"
```
