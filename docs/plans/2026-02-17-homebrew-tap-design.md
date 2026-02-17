# Homebrew Tap via GoReleaser

## Decision

Distribute Grove through a Homebrew tap (`chrisbanes/homebrew-tap`) with
GoReleaser auto-publishing the formula on every release.

## Why a tap, not Homebrew Core

Homebrew Core requires notable adoption (GitHub stars, active users) and
a PR review process. A tap is self-hosted with no gatekeeping. The formula
is nearly identical — migrating to Core later is straightforward.

## What changes

### `.goreleaser.yml` — add `brews` section

GoReleaser generates a Homebrew formula and pushes it to
`chrisbanes/homebrew-tap` after each release. The config specifies:

- Tap repository (`chrisbanes/homebrew-tap`)
- Formula install and test blocks
- Commit author for formula updates

### `release.yml` — use PAT for tap access

The built-in `GITHUB_TOKEN` only has access to the current repo. To push
a formula to `chrisbanes/homebrew-tap`, the release workflow passes a PAT
(`HOMEBREW_TAP_TOKEN`) to GoReleaser via environment variable.

### `chrisbanes/homebrew-tap` — new repo (manual)

An empty public repo. GoReleaser pushes `Formula/grove.rb` automatically.
No manual formula authoring needed.

## What stays the same

- Release trigger: `v*` tags
- Build targets: `darwin/amd64`, `darwin/arm64`
- Archives, checksums, changelog
- CI workflow

## User experience

```
brew tap chrisbanes/tap
brew install grove
grove version
```

## One-time setup (manual)

1. Create `chrisbanes/homebrew-tap` repo on GitHub (empty, public)
2. Create a fine-grained GitHub PAT scoped to `chrisbanes/homebrew-tap`
   with Contents read-and-write permission
3. Add the PAT as a repo secret named `HOMEBREW_TAP_TOKEN` on the grove repo

## Approach alternatives considered

**Separate GitHub Action step**: A custom workflow step generates the
formula from a template and pushes it. More moving parts than GoReleaser's
native support, with no added benefit.

**Cross-repo workflow**: A workflow in `chrisbanes/homebrew-tap` watches
for grove releases and updates the formula. Most complex, hardest to debug.
