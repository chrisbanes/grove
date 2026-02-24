# Optional Init Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make `grove init` optional so all commands work without it, add an interactive wizard to `init`, and change the default workspace directory to `~/.grove/{project}`.

**Architecture:** Add a `LoadOrDefault` function to the config package that returns defaults when no `config.json` exists. Update `FindGroveRoot` to fall back to git root discovery. Rewrite `init` to use `charmbracelet/huh` for interactive prompts. Update all commands to use `LoadOrDefault` instead of `Load`.

**Tech Stack:** Go, charmbracelet/huh (v2) for interactive prompts, cobra CLI framework (existing)

---

### Task 1: Change default workspace directory

**Files:**
- Modify: `internal/config/config.go:38-43`
- Modify: `cmd/grove/init.go:140`

**Step 1: Write a failing test**

Add to `internal/config/config_test.go` (create file):

```go
package config

import "testing"

func TestDefaultConfig_WorkspaceDir(t *testing.T) {
	cfg := DefaultConfig("myapp")
	want := "~/.grove/{project}"
	if cfg.WorkspaceDir != want {
		t.Errorf("DefaultConfig().WorkspaceDir = %q, want %q", cfg.WorkspaceDir, want)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestDefaultConfig_WorkspaceDir -v`
Expected: FAIL — got `/tmp/grove/{project}`, want `~/.grove/{project}`

**Step 3: Update default**

In `internal/config/config.go`, change `DefaultConfig`:

```go
func DefaultConfig(projectName string) *Config {
	return &Config{
		WorkspaceDir:  "~/.grove/{project}",
		MaxWorkspaces: 10,
	}
}
```

**Step 4: Update ExpandWorkspaceDir to handle tilde**

In `internal/config/config.go`, update `ExpandWorkspaceDir`:

```go
func ExpandWorkspaceDir(tmpl, projectName string) string {
	expanded := strings.ReplaceAll(tmpl, "{project}", projectName)
	if strings.HasPrefix(expanded, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			expanded = filepath.Join(home, expanded[2:])
		}
	}
	return expanded
}
```

Add a test for tilde expansion:

```go
func TestExpandWorkspaceDir_Tilde(t *testing.T) {
	result := ExpandWorkspaceDir("~/.grove/{project}", "myapp")
	if strings.HasPrefix(result, "~") {
		t.Errorf("tilde not expanded: %s", result)
	}
	if !strings.HasSuffix(result, "/.grove/myapp") {
		t.Errorf("unexpected expansion: %s", result)
	}
}
```

**Step 5: Update init flag help text**

In `cmd/grove/init.go:140`, update the help text:

```go
initCmd.Flags().String("workspace-dir", "", "Directory for workspaces (default: ~/.grove/{project})")
```

**Step 6: Run all tests**

Run: `go test ./internal/config/ -v && go test ./... -count=1`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go cmd/grove/init.go
git commit -m "feat: change default workspace dir to ~/.grove/{project}"
```

---

### Task 2: Add `LoadOrDefault` to config package

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Step 1: Write failing tests**

Add to `internal/config/config_test.go`:

```go
func TestLoadOrDefault_NoConfig(t *testing.T) {
	// Create a temp dir with .grove/ but no config.json
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, GroveDirName), 0755)

	cfg, err := LoadOrDefault(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WorkspaceDir != "~/.grove/{project}" {
		t.Errorf("expected default workspace dir, got %q", cfg.WorkspaceDir)
	}
	if cfg.CloneBackend != "cp" {
		t.Errorf("expected cp backend, got %q", cfg.CloneBackend)
	}
	if cfg.MaxWorkspaces != 10 {
		t.Errorf("expected 10 max workspaces, got %d", cfg.MaxWorkspaces)
	}
}

func TestLoadOrDefault_WithConfig(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, GroveDirName), 0755)
	cfg := &Config{
		WorkspaceDir:  "/custom/path",
		MaxWorkspaces: 5,
		CloneBackend:  "image",
	}
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadOrDefault(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded.WorkspaceDir != "/custom/path" {
		t.Errorf("expected /custom/path, got %q", loaded.WorkspaceDir)
	}
	if loaded.CloneBackend != "image" {
		t.Errorf("expected image, got %q", loaded.CloneBackend)
	}
}

func TestLoadOrDefault_NoGroveDir(t *testing.T) {
	// Bare directory with no .grove/ at all
	dir := t.TempDir()
	cfg, err := LoadOrDefault(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WorkspaceDir != "~/.grove/{project}" {
		t.Errorf("expected default workspace dir, got %q", cfg.WorkspaceDir)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run TestLoadOrDefault -v`
Expected: FAIL — `LoadOrDefault` not defined

**Step 3: Implement `LoadOrDefault`**

Add to `internal/config/config.go` after the `Load` function:

```go
// LoadOrDefault loads config from .grove/config.json if it exists,
// otherwise returns default config. This enables config-free mode
// where grove works without explicit initialization.
func LoadOrDefault(repoRoot string) (*Config, error) {
	path := filepath.Join(repoRoot, GroveDirName, ConfigFile)
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		projectName := filepath.Base(repoRoot)
		cfg := DefaultConfig(projectName)
		cfg.CloneBackend = "cp"
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	return Load(repoRoot)
}
```

**Step 4: Run tests**

Run: `go test ./internal/config/ -run TestLoadOrDefault -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add LoadOrDefault for config-free mode"
```

---

### Task 3: Update `FindGroveRoot` to fall back to git root

**Files:**
- Modify: `internal/config/config.go:293-310`
- Modify: `internal/config/config_test.go`

**Step 1: Write failing test**

Add to `internal/config/config_test.go`:

```go
func TestFindGroveRoot_GitFallback(t *testing.T) {
	// Create a git repo without .grove/
	dir := t.TempDir()
	cmd := exec.Command("git", "init", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %s\n%s", err, out)
	}

	root, err := FindGroveRoot(dir)
	if err != nil {
		t.Fatalf("expected git fallback, got error: %v", err)
	}
	if root != dir {
		t.Errorf("expected %s, got %s", dir, root)
	}
}

func TestFindGroveRoot_PrefersGroveDirOverGit(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("git", "init", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %s\n%s", err, out)
	}
	os.MkdirAll(filepath.Join(dir, GroveDirName), 0755)

	root, err := FindGroveRoot(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if root != dir {
		t.Errorf("expected %s, got %s", dir, root)
	}
}

func TestFindGroveRoot_NoGitNoGrove(t *testing.T) {
	dir := t.TempDir()
	_, err := FindGroveRoot(dir)
	if err == nil {
		t.Fatal("expected error for non-git, non-grove directory")
	}
}
```

**Step 2: Run tests to verify failure**

Run: `go test ./internal/config/ -run TestFindGroveRoot -v`
Expected: FAIL — `TestFindGroveRoot_GitFallback` fails

**Step 3: Update `FindGroveRoot`**

Replace `FindGroveRoot` in `internal/config/config.go`:

```go
func FindGroveRoot(startPath string) (string, error) {
	absPath, err := filepath.Abs(startPath)
	if err != nil {
		return "", err
	}

	// First pass: look for .grove/ directory (existing behavior)
	dir := absPath
	for {
		candidate := filepath.Join(dir, GroveDirName)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Second pass: fall back to git root
	dir = absPath
	for {
		gitDir := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitDir); err == nil && (info.IsDir() || !info.IsDir()) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("not a git repository (or any parent): %s", startPath)
}
```

**Step 4: Run tests**

Run: `go test ./internal/config/ -run TestFindGroveRoot -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: FindGroveRoot falls back to git root when .grove/ absent"
```

---

### Task 4: Add `EnsureMinimalGroveDir` helper

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Step 1: Write failing test**

```go
func TestEnsureMinimalGroveDir(t *testing.T) {
	dir := t.TempDir()

	if err := EnsureMinimalGroveDir(dir); err != nil {
		t.Fatal(err)
	}

	// .grove/ should exist
	if _, err := os.Stat(filepath.Join(dir, GroveDirName)); err != nil {
		t.Error(".grove/ not created")
	}

	// .gitignore should exist with correct content
	data, err := os.ReadFile(filepath.Join(dir, GroveDirName, ".gitignore"))
	if err != nil {
		t.Fatal(".grove/.gitignore not created")
	}
	if !strings.Contains(string(data), "workspace.json") {
		t.Error(".gitignore missing workspace.json entry")
	}

	// Calling again should not error (idempotent)
	if err := EnsureMinimalGroveDir(dir); err != nil {
		t.Fatalf("second call failed: %v", err)
	}
}
```

**Step 2: Run test to verify failure**

Run: `go test ./internal/config/ -run TestEnsureMinimalGroveDir -v`
Expected: FAIL — function not defined

**Step 3: Implement**

Add to `internal/config/config.go`:

```go
// EnsureMinimalGroveDir creates a .grove/ directory with just a .gitignore.
// This is the lazy-init path: no config.json, no hooks dir. Used by commands
// like create that need to write runtime files (.runtime-id, workspace.json).
func EnsureMinimalGroveDir(repoRoot string) error {
	groveDir := filepath.Join(repoRoot, GroveDirName)
	if err := os.MkdirAll(groveDir, 0755); err != nil {
		return err
	}
	return EnsureGroveGitignore(repoRoot)
}
```

**Step 4: Run test**

Run: `go test ./internal/config/ -run TestEnsureMinimalGroveDir -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add EnsureMinimalGroveDir for lazy init"
```

---

### Task 5: Update `create` command for config-free mode

**Files:**
- Modify: `cmd/grove/create.go:54-74`

**Step 1: Update create to use `LoadOrDefault` and lazy init**

Replace the config loading block in `cmd/grove/create.go`:

```go
		goldenRoot, err := config.FindGroveRoot(cwd)
		if err != nil {
			return err
		}

		// Don't allow creating workspaces from within a workspace
		if workspace.IsWorkspace(goldenRoot) {
			return fmt.Errorf("cannot create a workspace from inside another workspace.\nRun this from the golden copy instead")
		}

		cfg, err := config.LoadOrDefault(goldenRoot)
		if err != nil {
			return err
		}

		// Ensure .grove/ exists for runtime files
		if err := config.EnsureMinimalGroveDir(goldenRoot); err != nil {
			return err
		}

		if err := config.EnsureBackendCompatible(goldenRoot, cfg); err != nil {
			return err
		}
```

**Step 2: Run e2e tests**

Run: `go test ./test/ -run TestFullLifecycle -v -count=1`
Expected: PASS

**Step 3: Commit**

```bash
git add cmd/grove/create.go
git commit -m "feat: create command works without prior init"
```

---

### Task 6: Update `list`, `status`, `destroy`, and `update` commands

**Files:**
- Modify: `cmd/grove/list.go`
- Modify: `cmd/grove/status.go`
- Modify: `cmd/grove/destroy.go`
- Modify: `cmd/grove/update.go`

**Step 1: Update `list` command**

In `cmd/grove/list.go`, replace `config.Load` with `config.LoadOrDefault`:

```go
		cfg, err := config.LoadOrDefault(goldenRoot)
```

**Step 2: Update `status` command**

In `cmd/grove/status.go`, replace `config.Load` with `config.LoadOrDefault`:

```go
		cfg, err := config.LoadOrDefault(goldenRoot)
```

**Step 3: Update `destroy` command**

In `cmd/grove/destroy.go`, replace `config.Load` with `config.LoadOrDefault`:

```go
		cfg, err := config.LoadOrDefault(goldenRoot)
```

**Step 4: Update `update` command**

In `cmd/grove/update.go`, replace `config.Load` with `config.LoadOrDefault`:

```go
		cfg, err := config.LoadOrDefault(goldenRoot)
```

**Step 5: Run all tests**

Run: `go test ./... -count=1`
Expected: PASS

**Step 6: Commit**

```bash
git add cmd/grove/list.go cmd/grove/status.go cmd/grove/destroy.go cmd/grove/update.go
git commit -m "feat: list/status/destroy/update work without prior init"
```

---

### Task 7: Add `charmbracelet/huh` dependency

**Files:**
- Modify: `go.mod`

**Step 1: Add the dependency**

Run: `go get github.com/charmbracelet/huh/v2@latest`

**Step 2: Tidy**

Run: `go mod tidy`

**Step 3: Verify build**

Run: `go build ./cmd/grove`

**Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add charmbracelet/huh for interactive prompts"
```

---

### Task 8: Rewrite `init` command with interactive wizard

**Files:**
- Modify: `cmd/grove/init.go`

**Step 1: Rewrite init command**

Replace `cmd/grove/init.go` with the interactive wizard version. The new init should:

1. Accept the same flags as before (for scriptable/CI use)
2. When a flag is not provided AND stdin is a terminal, prompt interactively using `huh`
3. Support `--defaults` flag to skip all prompts
4. Question flow:
   a. Backend selection (select: cp / image)
   b. Workspace directory (input, default `~/.grove/{project}`)
   c. Image size GB (input, only if image backend selected, default 200)
   d. Warmup command (input, optional)
   e. Exclude patterns (input, comma-separated, optional)
5. Allow re-running `init` on already-initialized repos (update config instead of erroring)
6. After config is written, run warmup and image init as before

Key changes:
- Remove the "grove already initialized" error — instead, load existing config as defaults for the prompts
- Add `--defaults` flag
- Each flag overrides and skips its corresponding prompt
- Non-interactive (pipe/CI) mode uses defaults for unset flags

```go
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh/v2"
	"github.com/chrisbanes/grove/internal/config"
	gitpkg "github.com/chrisbanes/grove/internal/git"
	"github.com/chrisbanes/grove/internal/image"
	"github.com/chrisbanes/grove/internal/termio"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Configure grove for a repository",
	Long: `Sets up grove configuration for a git repository.
Runs an interactive wizard when called without flags.
Can be re-run to update configuration.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		progressEnabled, _ := cmd.Flags().GetBool("progress")
		var progress *progressRenderer
		if progressEnabled {
			progress = newProgressRenderer(os.Stderr, isTerminalFile(os.Stderr), "init")
			defer progress.Done()
		}

		path := "."
		if len(args) > 0 {
			path = args[0]
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			return err
		}

		if !gitpkg.IsRepo(absPath) {
			return fmt.Errorf("%s is not a git repository", absPath)
		}

		projectName := filepath.Base(absPath)

		// Load existing config or start with defaults
		cfg, _ := config.LoadOrDefault(absPath)

		useDefaults, _ := cmd.Flags().GetBool("defaults")
		interactive := isTerminalFile(os.Stdin) && !useDefaults

		// Apply flag overrides
		backendFlag, _ := cmd.Flags().GetString("backend")
		backendSet := cmd.Flags().Changed("backend")
		wsDirFlag, _ := cmd.Flags().GetString("workspace-dir")
		wsDirSet := cmd.Flags().Changed("workspace-dir")
		warmupFlag, _ := cmd.Flags().GetString("warmup-command")
		warmupSet := cmd.Flags().Changed("warmup-command")
		imageSizeFlag, _ := cmd.Flags().GetInt("image-size-gb")
		imageSizeSet := cmd.Flags().Changed("image-size-gb")

		if backendSet {
			switch backendFlag {
			case "cp", "image":
				cfg.CloneBackend = backendFlag
			default:
				return fmt.Errorf("invalid --backend %q: expected cp or image", backendFlag)
			}
		}
		if wsDirSet {
			cfg.WorkspaceDir = wsDirFlag
		}
		if warmupSet {
			cfg.WarmupCommand = warmupFlag
		}

		// Interactive prompts for unset options
		if interactive {
			fmt.Printf("Initializing grove for %s...\n\n", projectName)

			if !backendSet {
				var backendChoice string
				if cfg.CloneBackend != "" {
					backendChoice = cfg.CloneBackend
				} else {
					backendChoice = "cp"
				}
				err := huh.NewSelect[string]().
					Title("Which clone backend?").
					Options(
						huh.NewOption("cp - fast APFS copy-on-write clones (default)", "cp"),
						huh.NewOption("image - sparsebundle-based clones (experimental)", "image"),
					).
					Value(&backendChoice).
					Run()
				if err != nil {
					return err
				}
				cfg.CloneBackend = backendChoice
			}

			if !wsDirSet {
				defaultDir := cfg.WorkspaceDir
				if defaultDir == "" {
					defaultDir = "~/.grove/{project}"
				}
				var wsDir string
				err := huh.NewInput().
					Title("Workspace directory").
					Placeholder(defaultDir).
					Value(&wsDir).
					Run()
				if err != nil {
					return err
				}
				if wsDir != "" {
					cfg.WorkspaceDir = wsDir
				}
			}

			if cfg.CloneBackend == "image" && !imageSizeSet {
				var sizeStr string
				err := huh.NewInput().
					Title("Base image size in GB").
					Placeholder("200").
					Value(&sizeStr).
					Run()
				if err != nil {
					return err
				}
				if sizeStr != "" {
					fmt.Sscanf(sizeStr, "%d", &imageSizeFlag)
				}
			}

			if !warmupSet {
				var warmup string
				err := huh.NewInput().
					Title("Warmup command (optional)").
					Placeholder("e.g. npm install && npm run build").
					Value(&warmup).
					Run()
				if err != nil {
					return err
				}
				if warmup != "" {
					cfg.WarmupCommand = warmup
				}
			}

			// Excludes prompt
			var excludeStr string
			err := huh.NewInput().
				Title("Exclude patterns (comma-separated, optional)").
				Placeholder("e.g. *.lock, __pycache__").
				Value(&excludeStr).
				Run()
			if err != nil {
				return err
			}
			if excludeStr != "" {
				parts := strings.Split(excludeStr, ",")
				cfg.Exclude = nil
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if p != "" {
						cfg.Exclude = append(cfg.Exclude, p)
					}
				}
			}
		}

		// Ensure backend is set
		if cfg.CloneBackend == "" {
			cfg.CloneBackend = "cp"
		}

		// Create full .grove directory structure (including hooks/)
		groveDir := filepath.Join(absPath, config.GroveDirName)
		if err := os.MkdirAll(filepath.Join(groveDir, config.HooksDir), 0755); err != nil {
			return err
		}
		if err := config.EnsureGroveGitignore(absPath); err != nil {
			return fmt.Errorf("writing .grove/.gitignore: %w", err)
		}

		if err := config.Save(absPath, cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}

		// Run warmup if configured
		if cfg.WarmupCommand != "" {
			fmt.Printf("Running warmup: %s\n", cfg.WarmupCommand)
			warmup := exec.Command("sh", "-c", cfg.WarmupCommand)
			warmup.Dir = absPath
			if err := termio.RunInteractive(warmup); err != nil {
				return fmt.Errorf("warmup command failed: %w", err)
			}
		}

		// Initialize image backend if needed
		if cfg.CloneBackend == "image" {
			sizeGB := imageSizeFlag
			if sizeGB == 0 {
				sizeGB = 200
			}
			if progress == nil {
				fmt.Println("Initializing image backend...")
			}
			runtimeRoot, err := config.EnsureImageRuntimeRoot(absPath, cfg)
			if err != nil {
				return fmt.Errorf("resolving image runtime root: %w", err)
			}
			excludes, err := config.BuildImageSyncExcludes(absPath, cfg)
			if err != nil {
				return fmt.Errorf("computing image sync excludes: %w", err)
			}
			var onProgress func(int, string)
			if progress != nil {
				onProgress = func(pct int, phase string) {
					progress.Update(pct, phase)
				}
			}
			if _, err := image.InitBase(runtimeRoot, absPath, nil, sizeGB, excludes, onProgress); err != nil {
				return fmt.Errorf("initializing image backend: %w", err)
			}
		}
		if err := config.SaveBackendState(absPath, cfg.CloneBackend); err != nil {
			return fmt.Errorf("saving backend state: %w", err)
		}

		fmt.Printf("\nGrove initialized at %s\n", absPath)
		fmt.Printf("Workspace dir: %s\n", config.ExpandWorkspaceDir(cfg.WorkspaceDir, projectName))
		return nil
	},
}

func init() {
	initCmd.Flags().String("warmup-command", "", "Command to run for warming up build caches")
	initCmd.Flags().String("workspace-dir", "", "Directory for workspaces (default: ~/.grove/{project})")
	initCmd.Flags().String("backend", "", "Workspace backend: cp or image (experimental)")
	initCmd.Flags().Int("image-size-gb", 200, "Base sparsebundle size in GB when using --backend image")
	initCmd.Flags().Bool("force", false, "Proceed even if golden copy has uncommitted changes")
	initCmd.Flags().Bool("progress", false, "Show progress output during image backend initialization")
	initCmd.Flags().Bool("defaults", false, "Skip interactive prompts and use all defaults")
	rootCmd.AddCommand(initCmd)
}
```

Note: The `--backend` flag default changes from `"cp"` to `""` (empty) so we can detect whether it was explicitly set. The `--force` flag for dirty checks was removed from the init flow since init now focuses on config. If the dirty check is important, add it back gated on `--force`.

**Step 2: Verify build**

Run: `go build ./cmd/grove`

**Step 3: Commit**

```bash
git add cmd/grove/init.go
git commit -m "feat: interactive init wizard with charmbracelet/huh"
```

---

### Task 9: Update e2e tests for config-free mode

**Files:**
- Modify: `test/e2e_test.go`

**Step 1: Add e2e test for config-free create**

```go
func TestCreateWithoutInit(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("APFS tests only run on macOS")
	}

	binary := buildGrove(t)
	repo := setupTestRepo(t)

	// Skip grove init entirely — go straight to create
	out := grove(t, binary, repo, "create", "--json")
	var info workspace.Info
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		t.Fatalf("invalid JSON output: %s\n%s", err, out)
	}
	if info.ID == "" || info.Path == "" {
		t.Fatal("missing ID or Path in output")
	}

	// Verify workspace has golden copy content
	data, err := os.ReadFile(filepath.Join(info.Path, "main.go"))
	if err != nil {
		t.Fatal("main.go not in workspace")
	}
	if string(data) != "package main\n" {
		t.Error("main.go content mismatch")
	}

	// Verify .grove/ was lazily created in golden copy
	if _, err := os.Stat(filepath.Join(repo, ".grove")); err != nil {
		t.Error(".grove/ not created lazily")
	}

	// No config.json should exist (that requires explicit init)
	if _, err := os.Stat(filepath.Join(repo, ".grove", "config.json")); err == nil {
		t.Error("config.json should not exist without explicit init")
	}

	// List should work
	listOut := grove(t, binary, repo, "list")
	if !strings.Contains(listOut, info.ID) {
		t.Errorf("list should show workspace, got: %s", listOut)
	}

	// Status should work
	grove(t, binary, repo, "status")

	// Destroy should work
	grove(t, binary, repo, "destroy", info.ID)
	if _, err := os.Stat(info.Path); !os.IsNotExist(err) {
		t.Error("workspace not cleaned up")
	}
}
```

**Step 2: Update existing tests that check for "grove already initialized" error**

In `TestInitEdgeCases/already-initialized`: update to verify init can be re-run (no longer errors). Update the test to check that re-running init updates the config instead:

```go
	t.Run("reinit-updates-config", func(t *testing.T) {
		repo := setupTestRepo(t)
		grove(t, binary, repo, "init", "--defaults")
		// Re-running init should succeed (not error)
		grove(t, binary, repo, "init", "--defaults", "--warmup-command", "echo updated")

		cfg, err := config.Load(repo)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.WarmupCommand != "echo updated" {
			t.Errorf("expected updated warmup command, got %q", cfg.WarmupCommand)
		}
	})
```

**Step 3: Run e2e tests**

Run: `go test ./test/ -v -count=1`
Expected: PASS

**Step 4: Commit**

```bash
git add test/e2e_test.go
git commit -m "test: add e2e tests for config-free mode and reinit"
```

---

### Task 10: Update migrate command

**Files:**
- Modify: `cmd/grove/migrate.go:40-43`

**Step 1: Update migrate to use LoadOrDefault**

In `cmd/grove/migrate.go`, replace `config.Load` with `config.LoadOrDefault`:

```go
		cfg, err := config.LoadOrDefault(goldenRoot)
```

But also: migrate should still require that config exists (or at least that there's something to migrate from). Add a check:

```go
		// Migrate requires explicit init since it changes backend config
		if _, err := os.Stat(filepath.Join(goldenRoot, config.GroveDirName, config.ConfigFile)); err != nil {
			return fmt.Errorf("grove not initialized. Run `grove init` first to configure a backend before migrating")
		}
```

**Step 2: Run tests**

Run: `go test ./test/ -run TestMigrate -v -count=1`
Expected: PASS

**Step 3: Commit**

```bash
git add cmd/grove/migrate.go
git commit -m "feat: migrate uses LoadOrDefault but requires explicit init"
```

---

### Task 11: Final integration test and cleanup

**Files:**
- All files

**Step 1: Run full test suite**

Run: `go test ./... -count=1 -v`

**Step 2: Run vet and build**

Run: `go vet ./... && go build ./cmd/grove`

**Step 3: Manual smoke test**

```bash
cd /tmp && mkdir test-grove-repo && cd test-grove-repo && git init
echo "hello" > README.md && git add . && git commit -m "init"
grove create --json     # should work without init
grove list              # should show the workspace
grove destroy --all     # cleanup
grove init --defaults   # explicit init should work
grove create --json     # create with config
grove destroy --all
```

**Step 4: Final commit if any cleanup needed**

```bash
git add -A
git commit -m "chore: final cleanup for optional init"
```
