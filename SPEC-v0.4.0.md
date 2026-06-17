# kc v0.4.0 — SPEC Addendum: `kc resolve`

> Native batch secret resolution for Consi/OpenClaw exec provider protocol.

## Motivation

Consi's secrets subsystem supports an `exec` provider: a binary that receives a JSON request via stdin with secret IDs and returns values via stdout. Today this is served by a Python wrapper script that calls `kc inject` once per key — functional but inefficient (N subprocess calls for N keys, all hitting Keychain independently).

`kc resolve` implements the protocol natively in Go with zero external dependencies, single Keychain session, Touch ID batched into one prompt, and proper error handling per key.

## Protocol

### Input (stdin)
```json
{
  "protocolVersion": 1,
  "provider": "kc",
  "ids": ["OPENAI_API_KEY", "TELEGRAM_BOT_TOKEN", "NONEXISTENT_KEY"]
}
```

### Output (stdout)
```json
{
  "protocolVersion": 1,
  "values": {
    "OPENAI_API_KEY": "sk-proj-abc123...",
    "TELEGRAM_BOT_TOKEN": "123:abc...",
    "NONEXISTENT_KEY": null
  }
}
```

### Rules
- **Unknown keys → `null` value** (not error objects, not skipped). Consi handles null values gracefully.
- **Timestamp field in request is ignored** (kc has no TTL/rotation).
- **Errors are terminal**: if stdin is invalid JSON or the vault is unreachable, exit non-zero with error message on stderr. No partial results.
- **Protected keys**: if ANY requested key requires Touch ID, a single Touch ID prompt is triggered before resolving any values. Session caching applies (same as `kc get` / `kc inject` / `kc run`).
- **No trailing newline issues**: output must be exactly one line of valid JSON.
- **stdin → stdout only**: no interactive prompts, no clipboard, no TUI. Pure pipe.

## CLI

```bash
kc resolve < request.json > response.json

# Pipe directly from Consi
echo '{"protocolVersion":1,"provider":"kc","ids":["OPENAI_API_KEY"]}' | kc resolve

# From a script
kc resolve <<< '{"protocolVersion":1,"provider":"kc","ids":["KEY1","KEY2"]}'
```

### Flags
| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--vault` | string | active vault | Override vault for resolution |

### Exit Codes
| Code | Meaning |
|------|---------|
| 0 | Success (even if some keys returned null) |
| 1 | Input/parse error, vault access error, or any fatal failure |
| 2 | Stdin was empty (no input provided) |

## Architecture

```
internal/
  resolve/
    resolve.go           # Protocol types + resolution engine
    resolve_test.go      # Unit tests with mock keychain
  cli/
    resolve.go           # Cobra command (newResolveCmd)
```

### `resolve.go` — Protocol types

```go
package resolve

// Request is the stdin payload from Consi exec provider.
type Request struct {
    ProtocolVersion int      `json:"protocolVersion"`
    Provider        string   `json:"provider"`
    IDs             []string `json:"ids"`
}

// Response is the stdout payload.
type Response struct {
    ProtocolVersion int                `json:"protocolVersion"`
    Values          map[string]*string `json:"values"` // null for missing keys
}
```

### Resolution engine

```go
// Resolver resolves multiple secret IDs against a vault.
type Resolver struct {
    Store KeychainStore  // existing interface
    Auth  Authorizer     // existing auth.Authorizer interface
}

// Resolve reads all requested IDs from the vault.
// If any protected key is requested, triggers Touch ID once before resolving.
// Missing keys → nil pointer in result.
func (r *Resolver) Resolve(vault string, ids []string) (*Response, error)
```

### KeychainStore interface (reuses existing)

Already defined in `internal/cli/types.go`:
```go
type KeychainStore interface {
    Get(vault, key string) (string, error)
    ListMetadata(vault string) ([]SecretMetadata, error)
}
```

We only need `Get` and `ListMetadata` (for protection check). No new interface needed.

## Implementation Plan

1. **`internal/cli/resolve.go`** — Cobra command: parse stdin JSON, wire Resolver, print stdout JSON
2. **Wire into `root.go`** — add `newResolveCmd(app)` to subcommands
3. **Tests** — mock KeychainStore, test: happy path, missing keys, protected keys, empty stdin, invalid JSON, vault flag
4. **Man page** — `man/kc-resolve.1` (optional, can defer)

## Performance

- Single Keychain session for all keys (vs N calls in Python wrapper)
- Touch ID triggered once, not per-key
- Expected: <50ms for 20 keys (excluding Touch ID prompt time)

## NOT in v0.4.0

- ❌ TTL/rotation support (Consi doesn't use it yet)
- ❌ Multi-vault resolution in single call
- ❌ Value masking in output (the whole point is to return values)
- ❌ Audit logging (use `kc history` separately)

## Success Metrics

- `kc resolve` resolves 20 keys in <100ms (excluding Touch ID)
- Returns valid JSON parseable by `jq`
- Missing keys → `null` values, not errors
- Protected keys → single Touch ID prompt
- Consi `secrets audit --check` passes after migration
