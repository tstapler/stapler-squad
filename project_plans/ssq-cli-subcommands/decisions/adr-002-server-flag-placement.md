# ADR-002: Per-command `--server` flag, not a root persistent flag

**Date**: 2026-10-18
**Status**: Accepted

## Context

All new CLI commands and the retrofitted `get-session` need a way to target a non-default server URL. Three placement options exist in Cobra:

1. **`rootCmd.PersistentFlags()`** — one global `--server` flag inherited by all subcommands.
2. **Per-command local flag** — each new command registers its own `--server` flag pointing at a package-level or closure variable.
3. **Environment variable only** — `SSQ_SERVER` env var; no `--server` flag.

## Decision

Use **per-command local flag** (option 2), read by `cmd/commands/ResolveServerURL(flagVal string)` with `SSQ_SERVER` env fallback.

## Rationale

**Against option 1 (PersistentFlags on rootCmd):**

`rootCmd` is the server-startup command. Its `--help` output is what an operator reads to learn how to start the daemon. Inserting `--server` as a persistent flag implies the server reads it at startup, which is not the case. The flag is consumed only by CLI sub-commands; it is meaningless in the context of `stapler-squad` (no arguments → start HTTP server). This is confusing for the primary use case.

**Against option 3 (env-only):**

Env-only removes the ability to target a non-default server from the command line without editing the shell environment. CI scripts that need to reference multiple servers in one job would require export/unset dance. The `--server` flag is important for composability.

**For option 2 (per-command flag):**

- Keeps `rootCmd --help` clean — server-client flags appear only on the commands that use them.
- `--server` and `SSQ_SERVER` both work, as required by AC-5.
- Consistent: `get-session` already lives in `cmd/commands/`; the new commands follow the same pattern.
- `ResolveServerURL(flagVal)` is defined once in `cmd/commands/client.go`; each command calls it with its own flag value. DRY at the logic level; only the flag registration is repeated (one line per command, acceptable).

## Consequences

- `ssq --help` does not show `--server`; it only appears in `ssq create-session --help`, `ssq list-sessions --help`, etc.
- If a future refactor wants to unify the flag, extracting it to a `PersistentFlags` on a new `ssq client` command group is straightforward.
- `SSQ_SERVER` is the ergonomic default for shell aliases and CI environments; `--server` is the explicit override for one-off use.
