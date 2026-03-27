---
name: kc
description: >
  Manage macOS Keychain secrets via kc CLI. Store, read, list, search, and load secrets
  with Touch ID protection. Use when: (1) Reading API keys or tokens, (2) Storing new secrets,
  (3) Loading vault environments, (4) Searching across vaults.
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

# Switch active vault
kc vault switch prod

# List vaults
kc vault list

# Create a new vault
kc vault create staging

# Import from .env file
kc import .env

# Export to .env file
kc export -o .env

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

## When to use

| Pattern | Use case |
|---------|----------|
| `kc run -- <cmd>` | Running services, servers, scripts that need secrets |
| `kc inject --key K` | One-off inline injection (curl, psql, etc.) |
| `eval "$(kc env)"` | Interactive shell setup (`.zshrc`) or dev sessions |
| `kc get KEY` | Reading a secret for display/clipboard |
| `kc set KEY` | Storing new secrets interactively |

## Notes

- macOS only (uses native Keychain + Touch ID)
- First `kc get` per boot session prompts Touch ID, then cached
- Secrets never leave Apple's encryption stack
- `kc run` requires `--` separator before the command
