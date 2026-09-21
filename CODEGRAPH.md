# CODEGRAPH.md — kc

Local CodeGraph index for faster code exploration.

## Commands

```bash
codegraph status .
codegraph query -p . "symbolName"
codegraph context -p . "how does X work?"
codegraph sync .
```

Use `~/.local/bin/codegraph` if shell picks wrong Node. It forces Node 22 because CodeGraph blocks Node 26.

## Agent Rule

If `.codegraph/` exists, explore with CodeGraph first. Do not re-read files already returned by CodeGraph unless editing or verifying exact line numbers. Fallback to grep/read only when graph is stale, missing, or insufficient.

## Benchmark Questions

- How does the main request flow work end-to-end?
- What files are impacted by changing the primary runtime path?
- Where are external integrations configured?
- What tests cover the touched surface?
