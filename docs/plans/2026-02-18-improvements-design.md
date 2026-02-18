# Grove Improvements Design

Three targeted improvements to the CLI, workspace naming, and distribution.

## 1. Clearer `--branch` documentation

### Problem

The `--branch` flag's help text ("Branch to create/checkout in the workspace") does not explain what happens when it is omitted or how the flag changes the workspace's git state.

### Changes

**Cobra help text** (in `cmd/grove/create.go`):

```
Create and checkout a new git branch in the workspace (optional — defaults to the golden copy's current branch)
```

**README** (`README.md`): Update the `create` command section to explain:
- Without `--branch`, the workspace starts on whatever branch the golden copy is on.
- With `--branch`, Grove runs `git checkout -b <branch>` inside the new workspace.

**Long description** (`create` command): Add a line clarifying the branch behavior.

## 2. Branch-based workspace IDs

### Problem

Workspace IDs are random hex strings (e.g., `f7e8d9c0`). When listing or referencing workspaces, these IDs give no indication of what the workspace is for.

### Design

Replace the random ID with `{branch-slug}-{short-random}`:

- **Branch source**: Use the `--branch` value if provided; otherwise detect the golden copy's current branch via git.
- **Slugify**: Lowercase the branch name, replace `/` and non-alphanumeric characters with `-`, collapse consecutive hyphens, truncate to 20 characters, trim trailing hyphens.
- **Random suffix**: 4 hex characters (2 random bytes) for uniqueness.
- **Separator**: Single `-` between slug and suffix.

### Examples

| Command | Branch | Workspace ID |
|---------|--------|-------------|
| `grove create --branch feat/login-page` | `feat/login-page` | `feat-login-page-a1b2` |
| `grove create` (golden copy on `main`) | `main` | `main-c3d4` |
| `grove create --branch agent/fix-auth-module-refactor` | truncated at 20 chars | `agent-fix-auth-modul-e5f6` |

### Code changes

- `generateID()` becomes `generateID(branch string) (string, error)`.
- `Create()` detects the current branch when `opts.Branch` is empty, passing it to `generateID`.
- `resolveWorkspace` continues to match by exact ID — no changes needed.
- Add a `slugify(branch string) string` helper in `workspace.go`.

## 3. Apple notarization

### Problem

The macOS binary is unsigned. Gatekeeper warns users or blocks execution on first run.

### Approach

Use GoReleaser's built-in `notarize.macos` support, which uses [quill](https://github.com/anchore/quill) to handle both signing and notarization declaratively. This keeps everything in `.goreleaser.yml` — no manual `codesign` or `xcrun notarytool` steps in the workflow.

### GoReleaser config

The `notarize.macos` block signs each binary with the `.p12` certificate and submits to Apple for notarization via an App Store Connect API key:

```yaml
notarize:
  macos:
    - enabled: '{{ isEnvSet "MACOS_SIGN_P12" }}'
      ids:
        - grove
      sign:
        certificate: "{{.Env.MACOS_SIGN_P12}}"
        password: "{{.Env.MACOS_SIGN_PASSWORD}}"
      notarize:
        issuer_id: "{{.Env.MACOS_NOTARY_ISSUER_ID}}"
        key_id: "{{.Env.MACOS_NOTARY_KEY_ID}}"
        key: "{{.Env.MACOS_NOTARY_KEY}}"
        wait: true
        timeout: 20m
```

The `enabled` guard (`isEnvSet "MACOS_SIGN_P12"`) allows local/unsigned builds when the secrets are not set.

### Required GitHub Actions secrets

| Secret | Purpose |
|--------|---------|
| `MACOS_SIGN_P12` | Base64-encoded Developer ID Application `.p12` certificate |
| `MACOS_SIGN_PASSWORD` | Password for the `.p12` file |
| `MACOS_NOTARY_ISSUER_ID` | App Store Connect API issuer ID |
| `MACOS_NOTARY_KEY_ID` | App Store Connect API key ID |
| `MACOS_NOTARY_KEY` | App Store Connect API private key (`.p8` contents) |

### Notes

- GoReleaser's quill-based notarization works cross-platform (no need for native `codesign`/`xcrun`).
- No manual keychain setup or cleanup steps in the workflow.
- Raw command-line binaries do not need stapling; Gatekeeper verifies notarization online when the binary is first run.
