# Requirements: alias-settings-manager

**Date**: 2026-06-21
**Type**: feature addition
**Complexity**: 2 — focused feature

## Problem Statement

Users cannot manage alias session presets from the Settings UI. Aliases (named session presets invoked via `@name` in the omnibar) must be configured by manually editing `~/.stapler-squad/config.json` and restarting the service. This is error-prone and requires knowledge of the JSON schema.

## Baseline

Current workaround: open `~/.stapler-squad/config.json` in a text editor, add/edit/delete entries in the `aliases` array following the schema, save, then restart the service with `make install-service` or `systemctl --user restart stapler-squad`.

The "Config Files" tab in Settings provides a Monaco JSON editor for raw config editing, but offers no structural guidance, validation of alias-specific fields, or live reload.

## Users / Consumers

End users (the person running their own stapler-squad instance) who want to manage alias presets without touching raw JSON.

## Success Metrics

- A user can create, edit, and delete alias presets entirely from the Settings UI with no file editing or manual service restarts required.
- The alias list in Settings reflects the current state of `config.json` on load.
- Saves write through to `config.json` and take effect immediately (or surface a clear reload prompt if live reload is not feasible in this iteration).

## Appetite

Not set — let planning determine scope. Expected Small–Medium (1–3 days given the established `ProfilesManager` pattern to follow).

## Constraints

- Must follow the existing `ProfilesManager` / `DirectoryRulesManager` component pattern (form card, list row, upsert + delete RPCs).
- CSS must use vanilla-extract (`.css.ts` files, `vars` token references) per ADR-009.
- No new `SessionType` enum values — aliases are a config concept only.

## Non-functional Requirements

- **Performance SLO**: not specified — config reads/writes are infrequent
- **Scalability**: not applicable (single-user local service)
- **Security classification**: internal
- **Data residency**: local only (`~/.stapler-squad/config.json`)

## Scope

### In Scope

1. `UpsertAlias` RPC — create or update an alias by name; writes to `config.json`
2. `DeleteAlias` RPC — remove an alias by name from `config.json`
3. `AliasesManager` React component — list + inline form (add/edit/delete), placed in Settings > General tab alongside `ProfilesManager` and `DirectoryRulesManager`
4. Form fields covering all `AliasConfig` fields: `name`, `description`, `group`, `path`, `profile`, `program`, `auto_yes`, `tags`, `env_vars`, `cli_flags`
5. Proto generation (`make generate-proto`) and registry update (`make registry-generate`)

### Out of Scope

- Live config reload without service restart (investigate in rabbit holes; if complex, surface a "restart required" notice instead)
- Bulk import/export of aliases
- Alias reordering UI
- Alias usage analytics

## Rabbit Holes

- **Live reload**: does saving an alias via the API require a service restart, or does the Go config layer hot-reload? If the latter requires significant work, scope to "save + show restart banner" for this iteration.
- **`env_vars` form UX**: map editing (key→value pairs) is more complex than a plain text field. Could fall back to a raw text input (`KEY=VALUE` per line) parsed on save if the full key-value UI is too costly.

## Alternatives Considered

- Raw JSON editing via existing "Config Files" tab — already possible but no structural guidance; rejected as the primary UX for aliases.
- Dedicated `/aliases` page — rejected in favor of adding to the existing Settings General tab to keep config concerns co-located with Profiles and Directory Rules.

## Feasibility Risks

- `UpsertAlias` / `DeleteAlias` backend implementation must serialize/deserialize `config.json` safely under concurrent access (same concern as all other config-write RPCs — follow the existing pattern in `UpsertProfile`).
- Monaco editor in "Config Files" tab and the new structured editor could get out of sync if a user edits raw JSON while the settings tab is open — acceptable for now; both read from the server on mount.

## Observability Requirements

Standard request logging sufficient. No oncall alert needed.

## Risk Control

Not needed — low risk. The feature is purely additive (new UI + new RPCs). Worst case: a bad write corrupts `config.json`; mitigated by the existing JSON validation in the backend config loader and the raw editor fallback in "Config Files".

## Open Questions

- Does the Go config layer support live reload on write, or does `config.json` only load at startup? (Determines whether a "restart required" notice is needed.)
- Should `env_vars` be a key-value pair editor or a `KEY=VALUE` textarea for this iteration?
