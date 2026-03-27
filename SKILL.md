---
name: kc
description: >
  Manage macOS Keychain secrets via kc CLI. Store, read, list, search, and load secrets
  with Touch ID protection. Use when: (1) Reading API keys or tokens, (2) Storing new secrets,
  (3) Loading vault environments, (4) Searching across vaults.
---

# kc — macOS Keychain CLI

Manage secrets stored in macOS Keychain with Touch ID protection.

## Commands

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

# Load vault into current shell
kc load vault-name

# Load active vault
kc load

# Export as env vars
eval "$(kc env)"

# Export specific vault
eval "$(kc env --vault prod)"

# Switch active vault
kc vault switch prod

# List vaults
kc vault list

# Import from .env file
kc import .env

# Interactive TUI
kc
```

## When to use

- **Reading secrets:** `kc get NOTION_TOKEN` instead of hardcoding
- **Loading environments:** `kc load outbound-prod` before running services
- **Searching:** `kc search kafka` to find all Kafka-related keys
- **Storing new keys:** `kc set NEW_API_KEY` (interactive hidden input)

## Notes

- macOS only (uses native Keychain + Touch ID)
- First `kc get` per boot session prompts Touch ID, then cached
- Secrets never leave Apple's encryption stack
- `kc env` outputs `export KEY=VALUE` format ready for shell
