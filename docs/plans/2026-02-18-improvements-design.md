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

Use `codesign` and `notarytool` (Apple's current tooling) integrated into the GoReleaser release workflow.

### Release workflow changes

**Certificate setup** (new step in `.github/workflows/release.yml`):
1. Decode a base64-encoded `.p12` certificate from a GitHub secret.
2. Import it into a temporary keychain created for the CI run.
3. Set the keychain as the default and unlock it.

**Signing** (GoReleaser `signs` config or post-build step):
1. `codesign --force --options runtime --sign "${APPLE_DEVELOPER_ID}" grove` on each built binary.

**Notarization** (post-archive step):
1. Zip the signed binary.
2. Submit to Apple via `xcrun notarytool submit --apple-id --password --team-id --wait`.
3. Staple the notarization ticket if distributing a `.dmg` or `.app` bundle (for raw binaries, notarization alone suffices — Gatekeeper checks the ticket online).

### Required GitHub Actions secrets

| Secret | Purpose |
|--------|---------|
| `APPLE_CERTIFICATE_P12` | Base64-encoded Developer ID Application certificate |
| `APPLE_CERTIFICATE_PASSWORD` | Password for the .p12 file |
| `APPLE_DEVELOPER_ID` | Signing identity (e.g., `Developer ID Application: Name (TEAMID)`) |
| `APPLE_ID` | Apple ID email for notarytool |
| `APPLE_ID_PASSWORD` | App-specific password for notarytool |
| `APPLE_TEAM_ID` | Apple Developer team ID |

### Notes

- GoReleaser's `signs` block can handle codesigning each binary after build.
- Notarization applies to the final archive, not individual binaries — this runs as a post-GoReleaser step.
- The `--options runtime` flag enables the hardened runtime, which Apple requires for notarization.
- Raw command-line binaries do not need stapling; Gatekeeper verifies notarization online when the binary is first run.
