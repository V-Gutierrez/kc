---
name: kc
description: >
  Manage macOS Keychain secrets via kc CLI. Store, read, list, search, version, and load
  secrets with Touch ID protection. Use when: (1) Reading API keys or tokens, (2) Storing new
  secrets, (3) Loading vault environments, (4) Searching across vaults, (5) Recovering a
  previous value of a secret, (6) Organising or pinning vaults per project.
---

# kc — macOS Keychain CLI

Manage secrets stored in macOS Keychain with Touch ID protection.

## Secure Secret Injection (Recommended)

### kc run — process-scoped secrets
```bash
kc run -- node server.js              # secrets only in this process
kc run --vault prod -- npm start      # vault-specific secrets
kc run -- python script.py
```
**When the process exits, secrets are gone. Nothing leaks to the parent shell.**

### kc inject — single secret to stdout
```bash
# Inline a single secret without exporting it to the shell
curl -H "Authorization: Bearer $(kc inject --key API_TOKEN)" https://api.example.com
kc inject --vault prod --key STRIPE_KEY
```

---

## All Commands

```bash
# Read a secret (copies to clipboard, auto-clears 30s)
kc get KEY_NAME

# Store a secret (Touch ID protected by default)
kc set KEY_NAME "value"

# Store without Touch ID
kc set KEY_NAME "value" --no-protect

# List all keys in active vault
kc list

# List as JSON
kc list --json

# Search across all vaults
kc search query

# Delete a secret
kc del KEY_NAME

# Print a single secret to stdout (no trailing newline)
kc inject --key KEY_NAME
kc inject --vault prod --key KEY_NAME

# Run a command with vault secrets injected (preferred over eval)
kc run -- <command> [args...]
kc run --vault prod -- <command>

# Export as env vars (legacy)
kc env
kc env --vault prod

# Recover a previous value — every kc set records the value it replaces
kc history KEY_NAME                          # seq, timestamp, digest (never plaintext)
kc get KEY_NAME --version 2
kc diff KEY_NAME --version 1 --version 2
kc rollback KEY_NAME --version 1             # itself reversible
kc set KEY_NAME "value" --no-history         # opt out for one write

# Settings (retention, rotation window)
kc config list
kc config set history.retention 10

# Switch active vault
kc vault switch prod

# List vaults
kc vault list

# Create a new vault
kc vault create staging

# Vault lifecycle
kc vault clone prod staging                  # no plaintext round-trip
kc vault rename staging qa                   # keys and history move with the name
kc vault protect prod                        # Touch ID required for every key
kc vault info prod
kc vault delete old                          # soft delete — restorable
kc vault restore old
kc vault purge old                           # destroy for good

# Move or copy one secret, protection carried across
kc mv STRIPE_KEY --to prod
kc cp STRIPE_KEY --to staging

# Pin a directory (and everything under it) to a vault
kc vault use acme                            # writes .kc-vault
kc vault which                               # explains the resolved vault and why
kc vault unuse

# Import from .env file
kc import .env

# Export to .env file
kc export -o .env

# Consi/OpenClaw batch resolution (exec provider protocol)
echo '{"protocolVersion":1,"provider":"kc","ids":["OPENAI_API_KEY","NVIDIA_API_KEY"]}' | kc resolve
echo '{"protocolVersion":1,"provider":"kc","ids":["OPENAI_API_KEY"]}' | kc resolve --no-touch-id
echo '{"protocolVersion":1,"provider":"kc","ids":["OPENAI_API_KEY"]}' | kc resolve --vault prod

# Interactive TUI
kc
```

## Legacy Shell Integration (less secure)

```bash
# Exports ALL secrets to shell — any child process can read them
eval "$(kc env)"
eval "$(kc env --vault prod)"
```

Prefer `kc run` for running processes. Use `eval "$(kc env)"` only for interactive shell setup (`.zshrc`).

To follow a directory's pinned vault in the shell environment, install the cd hook — opt-in, because it changes what `cd` does:

```bash
eval "$(kc hook zsh)"     # bash and fish also supported
```

It is a no-op while the resolved vault is unchanged. On a change it unsets the previous vault's exports **before** reading anything, so declining the Touch ID prompt leaves the shell clean rather than holding the old vault's secrets.

## When to use

| Pattern | Use case |
|---------|----------|
| `kc run -- <cmd>` | Running services, servers, scripts that need secrets |
| `kc inject --key K` | One-off inline injection (curl, psql, etc.) |
| `eval "$(kc env)"` | Interactive shell setup (`.zshrc`) or dev sessions |
| `kc get KEY` | Reading a secret for display/clipboard |
| `kc set KEY` | Storing new secrets interactively |
| `kc resolve` | Batch resolution via stdin JSON (Consi/OpenClaw protocol) |
| `kc history` / `kc rollback` | A secret was overwritten and you need the previous value |
| `kc vault use` | A project should always act on its own vault |
| `kc vault clone` | Spinning up a staging copy of a vault |

## Notes

- macOS only (uses native Keychain + Touch ID)
- First `kc get` per boot session prompts Touch ID, then cached at `/tmp/kc-session-<UID>`
- `kc resolve --no-touch-id` skips Touch ID for non-interactive callers (Consi gateway startup)
- Consi/OpenClaw exec provider requires `passEnv: ["HOME", "PATH"]` — without `HOME`, Keychain access fails
- Secrets never leave Apple's encryption stack
- `kc run` requires `--` separator before the command
- History is kept per key (last 5 by default) in `kc:{vault}:__history__` and is deleted with the vault
- Vault precedence: `KC_VAULT` → nearest `.kc-vault` → active vault → `default`
