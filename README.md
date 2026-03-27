<p align="center">
  <img src="assets/logo.png" alt="kc logo" width="200" />
</p>

<h1 align="center">kc</h1>

<p align="center">
  <strong>A human-friendly CLI for macOS Keychain.</strong><br/>
  Because <code>security find-generic-password -s service -a account -w</code> is not human-friendly.
</p>

<p align="center">
  <a href="#why">Why</a> •
  <a href="#how-it-works">How It Works</a> •
  <a href="#install">Install</a> •
  <a href="#quick-start">Quick Start</a> •
  <a href="#commands">Commands</a> •
  <a href="#touch-id">Touch ID</a> •
  <a href="#vaults">Vaults</a> •
  <a href="#shell-integration">Shell Integration</a> •
  <a href="#security">Security</a>
</p>

<p align="center">
  <a href="https://github.com/v-gutierrez/kc/actions/workflows/ci.yml"><img src="https://github.com/v-gutierrez/kc/actions/workflows/ci.yml/badge.svg" alt="CI" /></a>
  <img src="https://img.shields.io/github/v/release/v-gutierrez/kc" alt="Version" />
  <img src="https://img.shields.io/badge/platform-macOS-lightgrey" alt="macOS" />
</p>

---

<p align="center">
  <img src="assets/demo.gif" alt="kc demo" width="700" />
</p>

---

## Why

```bash
# Before
export OPENAI_API_KEY="sk-proj-abc123"  # in your .zshrc, visible to everything

# After
kc set OPENAI_API_KEY    # AES-256 in Secure Enclave, Touch ID to access
```

kc doesn't add security macOS doesn't already have. It makes the secure path the easy path.

## How It Works

kc is a Go wrapper around macOS `security` CLI. No custom cryptography. No network calls. No config files. Your secrets stay in Apple's encryption stack.

## Install

```bash
brew tap v-gutierrez/kc
brew install kc
```

Or build from source:

```bash
git clone https://github.com/v-gutierrez/kc.git
cd kc
go build -ldflags "-X github.com/v-gutierrez/kc/internal/cli.Version=v0.4.0" -o kc ./cmd/kc
sudo mv kc /usr/local/bin/
```

## Quick Start

```bash
# Interactive TUI (recommended for secret entry)
kc

# Store a secret without putting the value in shell history
kc set OPENAI_API_KEY

# Read it (copies to clipboard, auto-clears in 30s)
kc get OPENAI_API_KEY

# Run a command with secrets injected (process-scoped — recommended)
kc run -- node server.js
kc run --vault prod -- npm start

# Inject a single secret inline
curl -H "Authorization: Bearer $(kc inject --key OPENAI_API_KEY)" https://api.openai.com/v1/models

# Search across all vaults
kc search openai

# Import from .env file
kc import .env

# Load all secrets into your shell (single Touch ID prompt — use for interactive shells)
eval "$(kc env)"

# Compare vaults
kc diff prod staging
```

## Commands

| Command | Description |
|---------|-------------|
| `kc get <key>` | Read a secret (copied to clipboard + printed masked) |
| `kc set <key> [value]` | Store/update a secret — Touch ID protected by default |
| `kc set <key> --no-protect` | Store without Touch ID protection |
| `kc del <key>` | Delete a secret |
| `kc list` | List all keys (values masked) |
| `kc list --json` | List as JSON for scripting |
| `kc run -- <cmd> [args]` | **Run command with vault secrets injected (process-scoped)** |
| `kc run --vault V -- <cmd>` | Run command with specific vault secrets |
| `kc inject --key <key>` | **Print a single secret to stdout (no newline, no clipboard)** |
| `kc inject --vault V --key K` | Inject from specific vault |
| `kc search <query>` | Fuzzy search across all vaults |
| `kc diff <vault1> <vault2>` | Compare keys between two vaults |
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

## Touch ID

**v0.4.0** keeps Touch ID protection on by default and adds a boot-session grace period for protected reads. After the first successful unlock, `kc` caches the current macOS boot session in `$TMPDIR`, so subsequent protected reads skip the prompt until you log out or restart.

```bash
# Default: Touch ID required
kc set DB_PASSWORD "super-secret"

# Opt out per key
kc set PUBLIC_KEY "not-sensitive" --no-protect

# First protected read prompts Touch ID
kc get DB_PASSWORD

# Later protected reads skip the prompt until logout or restart
eval "$(kc env)"

# Export follows the same protected-read rule
kc export > .env
```

**Why this matters:**
- Physical presence required — no remote exfiltration
- Enterprise-grade audit trail (who touched what, when)
- If Touch ID is unavailable, falls back to system password prompt
- Zero friction in daily workflow — one fingerprint per boot session

## Vaults

Vaults are logical groups for your secrets. Under the hood, they map to Keychain "service" fields prefixed with `kc:`.

```bash
kc vault create prod
kc vault create staging
kc vault switch prod
kc set DB_PASS "super-secret"     # saved in vault "prod"
kc get DB_PASS --vault staging    # read from specific vault
```

### Search

Find secrets across all vaults instantly:

```bash
# Fuzzy search
kc search api

# Search with JSON output
kc search openai --json
```

## Secure Secret Injection

### kc run — process-scoped secrets (recommended)

```bash
# Secrets are injected only into the child process environment
kc run -- node server.js           # secrets only in this process
kc run --vault prod -- npm start   # vault-specific
kc run -- python manage.py runserver

# When the process exits, secrets are gone. Nothing in the parent shell.
# Other processes on the same machine can't read them.
```

### kc inject — single secret to stdout

```bash
# Print one secret — no trailing newline, no clipboard, no export
curl -H "Authorization: Bearer $(kc inject --key API_TOKEN)" https://api.example.com
kc inject --vault prod --key STRIPE_KEY | xargs -I{} some-tool --token {}
```

### Comparison: kc run vs eval

| | `kc run -- cmd` | `eval "$(kc env)"` |
|---|---|---|
| Secrets scope | Child process only | Entire shell + all child processes |
| Parent shell polluted | ✅ No | ❌ Yes |
| Leaked to other tools | ✅ No | ❌ Any subprocess sees them |
| Shell history risk | ✅ None | ⚠️ Values may appear |
| Good for | Services, scripts, CI | Interactive shell setup |

**Recommendation:** Use `kc run` for scripts and services. Use `eval "$(kc env)"` only in `.zshrc` for interactive shells.

---

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

> **Note:** For scripts, servers, and automation, prefer `kc run -- <command>` over `eval "$(kc env)"` to scope secrets to the child process only.

### One-command onboarding

If you already have plaintext secrets in `~/.zshrc`, `~/.bash_profile`, or fish config, run:

```bash
kc setup
```

`kc setup` detects your shell, shows the secrets it found, imports them into your active vault, comments the old lines with `#kc-migrated#`, and installs the correct init snippet.

### Per-vault loading

```bash
eval "$(kc env --vault prod)"
# or, more securely:
kc run --vault prod -- npm start
```

## Security

- **Offline only.** No network calls. Ever.
- **Keychain-native.** AES-256 encryption via macOS Secure Enclave.
- **Touch ID by default.** Physical presence required to read secrets.
- **Clipboard auto-clear.** After `kc get`, clipboard clears in 30s.
- **No plaintext config.** Vault list is inferred from Keychain — no files to leak.
- **Prefer interactive entry for secrets.** The TUI avoids putting secret values on the command line. Shells may record `kc set KEY VALUE` in history.
- **Audit logging.** Every access logged with timestamp and context.

## Data Model

```
macOS Keychain (AES-256 via Secure Enclave)
  └── Service = "kc:{vault_name}"
        └── Account = key_name
              └── Password = secret_value
              └── Access Control = Touch ID (default) | None (--no-protect)
```

## License

[MIT](LICENSE)

---

<p align="center">
  Built with 🏈 by <a href="https://github.com/v-gutierrez">Victor Gutierrez</a>
</p>
