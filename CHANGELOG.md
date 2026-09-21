# Changelog

All notable changes to this project will be documented in this file.

## [v1.3.0]

### Secret versioning
- `kc set` now records the value it replaces; history lives in `kc:{vault}:__history__` and is removed with the vault.
- Added `kc history`, `kc get --version`, `kc diff --version`, and `kc rollback` — rollback records the value it replaces, so it is itself reversible.
- Added `--no-history` and `--keep-versions` on writes, and `~/.kc/config` with `history.retention` (default 5).
- Added a `stale-rotation` audit rule for credentials outside the rotation window.

### Vault management
- Added `kc vault clone`, `rename`, `protect`/`unprotect`, `describe`, and `info`.
- Added `kc mv` and `kc cp` for single secrets, carrying the protection level across.
- `kc vault delete` is now a soft delete: keys are archived and restorable via `kc vault restore`, destroyed by `kc vault purge`.

### Per-directory vaults
- A `.kc-vault` marker pins a directory tree to a vault, resolved on every invocation. Added `kc vault use`, `unuse`, and `which`.
- Added `kc hook <shell>` plus `kc env --sync` to follow the pinned vault in the shell environment. Stale exports are unset before any Keychain read, so a declined Touch ID prompt leaves the shell clean instead of holding the previous vault's secrets.

### Fixed
- TUI: editing a secret without revealing it first no longer overwrites the stored value with an empty string. An untouched value field now means "unchanged".
- TUI: renaming a key or changing its vault in the edit form now moves the secret instead of leaving a duplicate behind.
- TUI: a blank key name is rejected with an inline error instead of being written.
- TUI: toggling protection edits the existing row instead of adding a second one for the same key.
- `ShellQuote("")` now emits `''` instead of an empty token.

## [v1.2.1]

- Released upstream; not recorded here at the time.

## [v1.2.0]

- Released upstream; not recorded here at the time.

## [v1.1.0]

- Added `kc resolve` command — native Consi/OpenClaw exec provider protocol for batch secret resolution.
- Single Keychain session resolves multiple keys via JSON stdin/stdout.
- Protected keys trigger a single Touch ID prompt before resolution.

## [v0.4.0]

- Added a Touch ID boot-session grace period for protected reads, cached until logout or restart.
- Applied protected-read authorization to `kc export` and tightened bulk-read command auth ordering.

## [v0.3.0]

- Added Touch ID protection by default for stored secrets.
- Added fuzzy search, diff, audit, and JSON output workflows.
- Added interactive TUI usage improvements and updated README assets.

## [v0.2.0]

- Improved developer experience and shell integration workflows.
- Expanded release and packaging support for Homebrew usage.

## [v0.1.0]

- Initial public release of the macOS Keychain CLI.
- Added CI, Homebrew packaging, setup flow, and release pipeline.
