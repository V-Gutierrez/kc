# kc v0.5.0 — SPEC Addendum: Vault Management + Secret Versioning

> Survey requested by Victor (2026-09-21): "melhor gestão de vaults + versionamento dos secrets."
> Started as a levantamento (gap analysis + proposal). Greenlit in full the same day ("quero TUDO TUDO implementado") and **shipped**: Proposals A and B, items B1–B7 included. The sections below are kept as written for the reasoning; the status block records what actually landed.

**Autoria:** Victor Gutierrez + Consi

## Status — shipped 2026-09-21

| Scope | Shipped as |
|---|---|
| A — versioning | `kc history` · `kc get --version` · `kc diff --version` · `kc rollback` · `set --no-history` / `--keep-versions` · `~/.kc/config` (`history.retention`) · history purged with the vault |
| A — audit | `stale-rotation` rule in `internal/audit/rules.go` |
| B1 — protected vault | `kc vault protect` / `unprotect` |
| B2 — clone | `kc vault clone` |
| B3 — move/copy key | `kc mv` / `kc cp`, protection carried across |
| B4 — rename | `kc vault rename`, history included |
| B5 — metadata | `kc vault describe` / `info`, tab-separated `~/.kc/vaults` |
| B6 — soft delete | `kc vault delete` archives · `restore` · `purge` (`--expired`) |
| B7 — per-directory vault | `.kc-vault` marker resolved in-process · `kc vault use` / `unuse` / `which` · `kc hook <shell>` + `kc env --sync` for the shell environment |

Two departures from the proposal, both deliberate:

- **B7 needs no shell hook to work.** Vault resolution happens inside kc on every invocation, so `kc list` in a pinned directory is already correct with nothing installed. The hook exists only to keep *exported environment variables* in step, and is opt-in for that reason.
- **`kc env --sync` unsets before it reads.** Listing key names needs no authentication, so the previous vault's exports are dropped before the Touch ID prompt. A declined prompt therefore leaves the shell clean instead of holding secrets the new directory is not entitled to — found by driving the real zsh hook, not by reading the code.

Also fixed in the same pass, outside the original scope: the TUI edit form overwrote a secret with an empty string when the value had not been revealed first, and a rename in that form left the original key behind. See `internal/tui/edit_safety_test.go`.

## Current State (verified against source, 2026-09-21)

- **Vaults** (`internal/vault/vault.go`, `internal/cli/vault.go`): a vault is a name mapped 1:1 to a Keychain service (`kc:{name}`). The vault list is a flat newline file at `~/.kc/vaults` (name only, no metadata). CLI surface is exactly 4 verbs: `list`, `create`, `switch`, `delete --force`. No rename, no clone, no per-key move, no description/tag/owner, no protection default per vault.
- **Secret versioning**: does not exist. `kc set` calls `SecItemUpdate` semantics through `KC.SetWithProtection`, which overwrites the Keychain item in place — the previous value is gone, unrecoverable. `SPEC-v0.4.0.md` explicitly deferred this ("❌ Audit logging (use `kc history` separately)") but `kc history` was never built — confirmed by grep, no `history` command or package exists.
- **Reusable primitives already in the codebase** (relevant because "boring over clever" — reuse beats new machinery):
  - `keychain.ItemMetadata` already carries a `Modified` timestamp per item (used today only in `kc list` display).
  - `internal/diff.Compare(left, right map[string]string) []Entry` is a generic keyed diff — currently wired to vault-vs-vault/`.env` comparison, but its signature works unchanged for version-vs-version diff of a single key.
  - `internal/audit` already has a `Scan([]ScanInput) []Finding` pipeline with severity ranking (`stale`, `weak-secret`, `duplicate`, `suspicious-name` rules) — a natural home for a new "not rotated in N days" rule once versioning exists.
  - `Manager.GetAllKeys` + `Manager.BulkSetWithProtection` already exist and are sufficient to implement vault clone almost for free.
  - `keychain.Digest` (already used by the duplicate-value audit rule) is the right primitive for showing version history without printing raw secret values.

## Problem

Two gaps, independent but reinforcing:

1. **Vault management is CRUD-minimal.** Real usage (staging clones of prod, renaming a vault as a project evolves, moving one key between vaults, marking `prod` as "must be Touch-ID protected") requires manual multi-step workarounds today, several of which silently lose the protection flag.
2. **There is no undo for `kc set`.** Overwrite is unconditional and irreversible. There is also no rotation visibility — nothing tells you a secret hasn't changed in 400 days. Both are exactly the failure modes real secret managers (HashiCorp Vault KV v2, 1Password item history, `pass`+git) exist to prevent.

## Proposal A — Secret Versioning

### Data model

macOS Keychain has no native item history — `SecItemUpdate` overwrites in place. Versioning has to be built at the application layer, but it can reuse the exact same encrypted-item primitives kc already has (no new crypto, no new dependency — matches the "zero external dependencies" constraint in `SPEC.md`).

```
Live secret   → service "kc:{vault}"               account "{key}"          (unchanged, current behavior)
History entry → service "kc:{vault}:__history__"    account "{key}~{seq}"    seq = zero-padded monotonic counter
```

- On `kc set KEY` for a key that already exists: read the current live value first, write it to a new history slot (`{key}~{next_seq}`) with the **same protection level** it had live (no downgrade), *then* overwrite the live value. Order matters — never lose data if the second write fails.
- `seq` is derived by listing Keychain accounts under `kc:{vault}:__history__` filtered by the `{key}~` prefix and parsing the trailing integer — no separate index file, no second source of truth (same reasoning the codebase already applies: `~/.kc/vaults` is only materialized because Keychain can't enumerate services, not because a duplicate index is desirable).
- The version's timestamp comes from Keychain's own `Modified` attribute on the history item (already exposed via `ListMetadata`) — free, no hand-rolled clock.
- **Retention**: keep last N versions per key, default N=5, pruned oldest-first after each write. Configurable (see Config below).

### CLI surface

```bash
kc history KEY [--vault v] [--json]        # list versions: seq, timestamp, protection, value digest (never raw value)
kc get KEY --version N [--vault v]         # read a specific historical version (masked/clipboard, same UX as kc get)
kc diff KEY --version N1 --version N2      # reuse internal/diff.Compare({KEY: v1}, {KEY: v2}) — no new diff logic needed
kc rollback KEY --version N [--vault v]    # restore version N as the new live value
kc set KEY "value" --no-history            # opt out of snapshotting (high-churn secrets, explicit choice)
```

`rollback` is itself just a `set` under the hood — restoring version N first snapshots the *current* live value into a new history slot, so rollback is never destructive and is itself reversible. Append-only, no exceptions.

### Config

No config file exists yet (`~/.kc/config` does not exist — verified by grep). This is genuinely new infrastructure, smallest useful version:

```
~/.kc/config          # key=value, single file, same 0600 permission model as active_vault
history.retention=5
```

`--keep-versions N` flag on `set`/`import` overrides the config default per-call.

### Cleanup correctness (a real gap, not hidden)

`vault.DeleteVault` (`internal/vault/vault.go:181`) only knows about service `kc:{name}`. Once history exists, it must also purge `kc:{name}:__history__` — otherwise deleting a vault leaves orphaned encrypted blobs behind that `kc vault list` can no longer reach or account for. One-line fix, but must ship in the same PR as versioning, not after.

### Audit integration (follow-up, not blocking)

New rule in `internal/audit/rules.go`: `stale-rotation` — flag keys matching sensitive-name patterns (`*_KEY`, `*_TOKEN`, `*_SECRET`, `*_PASSWORD`) whose newest version (live `Modified` or latest history entry) is older than a configurable threshold (default 180 days). Slots directly into the existing `Scan()` pipeline and severity ranking — no new architecture.

## Proposal B — Vault Management

Ordered by leverage (safety first, then ergonomics that are cheap because primitives already exist, then the one item that needs new infrastructure):

1. **Protected-vault default** (`kc vault create prod --require-protection`, or `kc vault protect NAME`). Stores a flag alongside the vault name; `kc set` into a flagged vault without Touch ID is *rejected*, not silently allowed. Today the unsafe path is opt-out (`--no-protect`) with nothing stopping a mistake on `prod`; this inverts the default where it matters. Highest leverage relative to effort — a few lines in `resolveVault`/`SetWithProtection`.
2. **Vault clone** (`kc vault clone SRC DST [--protect|--no-protect]`). Implementation is close to free: `GetAllKeys(SRC)` + `Create(DST)` + `BulkSetWithProtection(entries, DST, ...)` — all three already exist on `Manager`. Covers "spin up a staging copy of prod" in one command instead of an export/import round-trip through a plaintext `.env` file (which is exactly the anti-pattern kc exists to remove — see `SPEC.md` "Why").
3. **Move/copy a single key across vaults** (`kc mv KEY --from A --to B [--copy]`). Today this is a manual `get` + `set` + `del`, and nothing preserves the protection flag automatically — a real, silent downgrade risk. New command reads `ListKeyMetadata` first to carry protection state across.
4. **Vault rename** (`kc vault rename OLD NEW`). Requires copying all keys to a new service, updating `~/.kc/vaults` and `active_vault` if it was active, then deleting the old service — same primitives as clone, plus the bookkeeping already in `Switch`/`writeVaults`.
5. **Vault metadata** (description, created-at, optional `env` tag like `prod`/`staging`). Extend the `~/.kc/vaults` format from a bare name list to `name\tdescription\tcreatedAt\ttags` (tab-separated, backward compatible: `parseVaultList` already trims and treats an unlabeled line as name-only, so old files keep working unmodified).
6. **Soft-delete on `vault delete`** — direct synergy with Proposal A: once per-key history exists, deleting a vault can snapshot each key into `kc:__archive__:{vault}:{key}` before purging, with `kc vault restore NAME` available until a separate GC step purges it for good (default 30 days). This turns today's irreversible `--force` delete into a safety net essentially for free once versioning ships — sequencing reason to do A before B6.
7. **Per-directory auto vault context** (`.kc-vault` marker file + shell hook, like `direnv`/`.nvmrc`). This is the biggest ergonomics gap versus tooling Victor already uses daily, but it's also the only item here that needs genuinely new infrastructure — a shell hook layered on `kc init` (which today only prints a static snippet) plus a lookup-on-cd. Flagging as separate/later phase, not bundled with the rest: it changes shell behavior, which deserves its own review pass independent of the vault CRUD work.

## NOT in this pass

- ❌ Multi-device / cloud sync of vaults or history (macOS Keychain iCloud sync is a user-level setting kc doesn't and shouldn't manage)
- ❌ Team/shared vaults (kc's threat model is single-user local Keychain; sharing implies a server component out of scope)
- ❌ Per-directory auto vault switching implementation (scoped above as B7, deliberately deferred)
- ❌ Changing the underlying storage backend away from Keychain

## Sequencing recommendation

**A before B**, because B6 (soft-delete) is materially cheaper once A's history machinery exists, and A is the piece Victor named explicitly ("versionamento"). Within B, items 1–4 are small and independent of each other and of A; item 7 is a separate decision (shell behavior change) that shouldn't block the rest.

Suggested slices if approved:
- **Slice 1 (S/M):** A — history data model + `kc history`/`get --version`/`diff --version`/`rollback`, plus the `DeleteVault` cleanup fix.
- **Slice 2 (S):** B1 + B2 — protected-vault default + vault clone (highest leverage, lowest effort, no dependency on Slice 1).
- **Slice 3 (S):** B3 + B4 — move/copy key, vault rename.
- **Slice 4 (S):** B5 + B6 — vault metadata, soft-delete (depends on Slice 1).
- **Slice 5 (M, separate review):** B7 — per-directory auto-switch shell hook.

Each slice is independently shippable and testable per the repo's existing TDD convention (every package above already carries a `_test.go` sibling).

## Open decision for Victor

Which slice(s) to greenlight first, and whether B7 (shell auto-switch) is in scope at all right now or stays backlog. Recommendation: Slice 1 + Slice 2 first — they cover both things named in the request ("versionamento" and the single highest-leverage vault-safety gap) with the least new surface area.
