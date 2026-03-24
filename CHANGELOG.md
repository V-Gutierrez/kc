# Changelog

All notable changes to this project will be documented in this file.

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
