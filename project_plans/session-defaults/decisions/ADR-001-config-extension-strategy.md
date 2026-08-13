# ADR-001: Config Extension Strategy for Session Defaults

Date: 2026-04-13
Status: Accepted

## Context

Session defaults need to persist across restarts. The app already has a `config.json`
file written by `config/config.go` with atomic write (`.tmp` + rename). No versioning
exists; missing fields silently zero-value on unmarshal. `state.json` uses `flock` for
concurrent writes but `config.json` does not.

Two options were considered:
1. Extend the existing `config.json` schema with a `SessionDefaults` nested struct
2. Create a separate `session_defaults.json` file

## Decision

Extend `config.json` with a `session_defaults omitempty` nested struct.

Add a `config_version: 1` integer field to enable future migration paths.

Apply `flock` to config reads/writes to match the `state.go` pattern and prevent
concurrent-write data loss (two browser tabs saving settings simultaneously).

Initialize all map/slice fields (`Profiles`, `DirectoryRules`) inside `LoadConfig()`
after unmarshal to eliminate nil-panic risk at call sites.

## Rationale

- No new file means no new path-resolution logic and no additional startup I/O
- Atomic write is already implemented and tested
- Additive-only schema change: existing configs load correctly (omitempty + zero values)
- `DefaultProgram` top-level field kept; `ResolveDefaults` falls back to it for
  backward compatibility so no migration required for existing users

## Consequences

- All callers of `config.LoadConfig()` get the initialized struct for free; no
  nil checks needed at call sites
- `config_version` enables future destructive migrations without silent data loss
- flock adds a small latency cost on every config save (negligible for UI-driven ops)
- Two concurrent writes will serialize rather than race; last writer still wins within
  a flock window — acceptable for settings UI (not high-frequency writes)
