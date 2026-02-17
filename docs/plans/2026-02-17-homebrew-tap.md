# Homebrew Tap Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Auto-publish a Homebrew cask to `chrisbanes/homebrew-tap` on every GoReleaser release.

**Architecture:** Add a `homebrew_casks` section to `.goreleaser.yml` and pass a PAT via the release workflow. GoReleaser handles formula generation and push to the tap repo.

**Tech Stack:** GoReleaser v2 (`homebrew_casks`), GitHub Actions, Homebrew

---

### Task 1: Add `homebrew_casks` to GoReleaser config

GoReleaser v2.10+ deprecated `brews` in favor of `homebrew_casks`. Since Grove
builds macOS-only unsigned binaries, the cask needs a `postflight` stanza to
remove the quarantine attribute.

**Files:**
- Modify: `.goreleaser.yml:16` (insert after `archives` section)

**Step 1: Add the homebrew_casks section**

Add the following after the `archives` block (after line 21) in `.goreleaser.yml`:

```yaml
homebrew_casks:
  - name: grove
    repository:
      owner: chrisbanes
      name: homebrew-tap
      token: "{{ .Env.HOMEBREW_TAP_TOKEN }}"
    directory: Casks
    homepage: "https://github.com/chrisbanes/grove"
    description: "Manage CoW-cloned workspaces with warm build caches"
    license: "MIT"
    commit_author:
      name: goreleaserbot
      email: bot@goreleaser.com
```

**Step 2: Validate the config locally**

Run: `goreleaser check`
Expected: no errors (or goreleaser not installed locally — that's fine, CI will validate)

**Step 3: Commit**

```bash
git add .goreleaser.yml
git commit -m "feat: add Homebrew cask config to GoReleaser"
```

---

### Task 2: Pass `HOMEBREW_TAP_TOKEN` in the release workflow

The built-in `GITHUB_TOKEN` cannot push to a different repo. Pass the PAT
(already stored as a repo secret) to GoReleaser.

**Files:**
- Modify: `.github/workflows/release.yml:30-31`

**Step 1: Add the token to the env block**

Change the `env` block of the goreleaser-action step from:

```yaml
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

to:

```yaml
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}
```

**Step 2: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: pass HOMEBREW_TAP_TOKEN to release workflow"
```

---

### Task 3: Verify end-to-end (manual)

This task is manual — it validates the full pipeline.

**Step 1: Ensure `chrisbanes/homebrew-tap` repo exists on GitHub (empty, public)**

**Step 2: Ensure `HOMEBREW_TAP_TOKEN` secret is set on the grove repo**

**Step 3: Push a test tag**

```bash
git tag v0.1.0-rc.1
git push origin v0.1.0-rc.1
```

**Step 4: Watch the release workflow**

Check GitHub Actions for the Release workflow. Verify:
- GoReleaser succeeds
- A `Casks/grove.rb` file appears in `chrisbanes/homebrew-tap`

**Step 5: Test the tap**

```bash
brew tap chrisbanes/tap
brew install grove
grove version
```

Expected output: `grove 0.1.0-rc.1`

**Step 6: Clean up the RC tag if desired**

```bash
git tag -d v0.1.0-rc.1
git push origin :refs/tags/v0.1.0-rc.1
```
