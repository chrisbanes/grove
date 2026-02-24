# Rename `init` to `config`

## Problem

The `grove init` command no longer serves as a mandatory initialization step. Since commit 86d1537, all commands work without prior initialization via `LoadOrDefault`. The name `init` misleads users into thinking they must run it before using grove.

The command configures preferences: backend, workspace directory, state directory, warmup command, and exclude patterns. The name `config` describes this accurately.

## Design

Rename `grove init [path]` to `grove config [path]`. Keep all flags, the interactive wizard, and behavior unchanged. No alias for the old name.

### What changes

- `cmd/grove/init.go` renamed to `cmd/grove/config.go`; cobra command name updated to `config`
- Command description and help text updated
- All tests referencing `init` updated
- Skill files (`commands/grove-init.md`, related skills) updated
- README updated

### What does not change

- The interactive wizard UX
- Flag names and defaults
- Config-free mode (`LoadOrDefault`)
- `.grove/config.json` format
- Any other command
