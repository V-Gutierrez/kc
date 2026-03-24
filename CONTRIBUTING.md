# Contributing to kc

Thanks for contributing to kc.

## Before you start

- `kc` is a macOS-focused CLI for Keychain secrets management.
- Use Go 1.24.
- Building the main binary requires macOS because Touch ID support depends on Apple frameworks.

## Development setup

```bash
git clone https://github.com/<your-fork>/kc.git
cd kc
go mod download
```

## Build

```bash
go build ./cmd/kc
```

## Test

Run the full verification suite before opening a pull request:

```bash
go test ./...
go vet ./...
```

If `golangci-lint` is installed locally, run:

```bash
golangci-lint run
```

## Pull requests

1. Fork the repository and create a topic branch.
2. Keep changes focused and include tests for behavior changes.
3. Use Conventional Commits when possible, for example `fix:`, `docs:`, or `chore:`.
4. Update docs when CLI behavior or workflows change.
5. Open a pull request describing the problem, approach, and verification steps.

## Release notes and docs

- Update `CHANGELOG.md` for user-visible changes.
- If command help changes, regenerate docs with:

```bash
go run ./cmd/gendocs
```
