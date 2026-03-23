<p align="center">
  <img src="assets/logo.png" alt="kc logo" width="200" />
</p>

<h1 align="center">kc</h1>

<p align="center">
  <strong>A human-friendly CLI for macOS Keychain.</strong><br/>
  Because <code>security find-generic-password -s service -a account -w</code> is not human-friendly.
</p>

<p align="center">
  <a href="#install">Install</a> •
  <a href="#quick-start">Quick Start</a> •
  <a href="#commands">Commands</a> •
  <a href="#vaults">Vaults</a> •
  <a href="#shell-integration">Shell Integration</a>
</p>

<p align="center">
  <a href="https://github.com/v-gutierrez/kc/actions/workflows/ci.yml"><img src="https://github.com/v-gutierrez/kc/actions/workflows/ci.yml/badge.svg" alt="CI" /></a>
</p>

---

## Why

Every developer on macOS has API keys in `.env` files, `.zshrc`, or `.bashrc` — **plaintext, git-committable, screenshot-visible.**

The native Keychain is the right solution, but the `security` command UX is hostile. **kc** fixes that.

Your secrets never leave the Secure Enclave. Zero external dependencies. Zero network calls. Ever.

## Install

```bash
brew tap v-gutierrez/kc https://github.com/v-gutierrez/kc
brew install kc
```

Or build from source:

```bash
git clone https://github.com/v-gutierrez/kc.git
cd kc
go build -o kc ./cmd/kc
sudo mv kc /usr/local/bin/
```

## Quick Start

```bash
# Store a secret
kc set API_KEY "sk-proj-abc123"

# Read it (copies to clipboard, auto-clears in 30s)
kc get API_KEY

# List all keys (values masked)
kc list

# Import from .env file
kc import .env

# Load all secrets into your shell
eval "$(kc env)"

# Print the shell snippet for your shell config
kc init zsh

# Migrate plaintext secrets from your shell rc file
kc setup

# Interactive TUI
kc
```

## Commands

| Command | Description |
|---------|-------------|
| `kc get <key>` | Read a secret (copied to clipboard + printed masked) |
| `kc set <key> [value]` | Store/update a secret (prompts if no value given) |
| `kc del <key>` | Delete a secret |
| `kc list` | List all keys (values masked) |
| `kc import <file>` | Import from .env file → Keychain |
| `kc export` | Export active vault as .env to stdout |
| `kc export -o <file>` | Export to file |
| `kc env` | Print `export` statements for shell integration |
| `kc init <shell>` | Print the shell init snippet for zsh, bash, or fish |
| `kc setup` | Migrate plaintext shell secrets into Keychain and install shell init |
| `kc migrate --from <service>` | Migrate existing Keychain entries to kc format |
| `kc vault list` | List all vaults |
| `kc vault create <name>` | Create a new vault |
| `kc vault switch <name>` | Set active vault |

## Vaults

Vaults are logical groups for your secrets. Under the hood, they map to Keychain "service" fields prefixed with `kc:`.

```bash
kc vault create prod
kc vault create staging
kc vault switch prod
kc set DB_PASS "super-secret"     # saved in vault "prod"
kc get DB_PASS --vault staging    # read from specific vault
```

## Shell Integration

Generate the right shell snippet:

```bash
kc init zsh
kc init bash
kc init fish
```

For zsh or bash, add to your shell rc file:

```bash
eval "$(kc env)"
```

For fish:

```fish
kc env | source
```

That's it. All secrets from your active vault are loaded as environment variables on shell startup.

### One-command onboarding

If you already have plaintext secrets in `~/.zshrc`, `~/.bash_profile`, or fish config, run:

```bash
kc setup
```

`kc setup` detects your shell, shows the secrets it found, imports them into your active vault, comments the old lines with `#kc-migrated#`, and installs the correct init snippet.

### Per-vault loading

```bash
eval "$(kc env --vault prod)"
```

## Security

- **Offline only.** No network calls. Ever.
- **Keychain-native.** AES-256 encryption via macOS Secure Enclave.
- **Clipboard auto-clear.** After `kc get`, clipboard clears in 30s.
- **No plaintext config.** Vault list is inferred from Keychain — no files to leak.
- **No shell history exposure.** `kc set` with no value prompts interactively (hidden input).

## Data Model

```
macOS Keychain (AES-256 via Secure Enclave)
  └── Service = "kc:{vault_name}"
        └── Account = key_name
              └── Password = secret_value
```

## License

MIT

---

<p align="center">
  Built with 🏈 by <a href="https://github.com/v-gutierrez">Victor Gutierrez</a>
</p>
