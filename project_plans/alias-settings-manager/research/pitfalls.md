# Research: Pitfalls — Alias Settings Manager

**Status**: Completed | **Phase**: 2 — Research
**Created**: 2026-06-21

---

## PITFALL-1: Concurrent Config.json Writes — No Mutex, Last Write Wins (HIGH)

### Risk

Every config-mutation RPC in `defaults_service.go` follows the pattern:

```go
cfg := config.LoadConfig()   // reads config.json from disk
// ... modify cfg in memory ...
config.SaveConfig(cfg)       // writes config.json atomically (temp-rename)
```

`LoadConfig` always reads from disk; there is **no in-memory mutex, no shared config singleton, and no optimistic lock**. If two `UpsertAlias` calls (or an `UpsertAlias` plus an `UpdateGlobalDefaults`) are inflight simultaneously, the second `LoadConfig` reads the pre-first-write state, and whichever `SaveConfig` lands last silently discards the other's changes.

This is the same race that already exists for `UpsertProfile`, `DeleteProfile`, `UpsertDirectoryRule`, and `UpdateGlobalDefaults`. It is **not introduced by this feature** — but the new RPCs amplify the exposure because the settings UI will now issue multiple rapid writes (user adds alias A, edits alias B, deletes alias C in quick succession before the page re-fetches).

### Evidence

`saveConfig()` in `config/config.go` (line 794–804) uses atomic temp-file rename — this is correct for filesystem atomicity (no partial writes). But it does not provide logical atomicity against concurrent readers/writers, because nothing prevents two goroutines from racing through the `LoadConfig → modify → SaveConfig` sequence simultaneously.

### Mitigation

**Option A (recommended)**: Add a `sync.Mutex` in the `DefaultsService` struct (or a package-level mutex in `config/`) guarding the load→modify→save sequence for all config-mutation handlers. This is the minimum viable fix and directly parallels how `ClaudeConfigManager` protects its writes via `mu sync.RWMutex` (see `config/claude.go` line 58).

**Option B**: Accept the existing race as a known limitation for the MVP — consistent with how `UpsertProfile` works today. The single-user, low-concurrency nature of the service means real data loss is unlikely, but rapid UI interactions (double-click Save, two browser tabs) can still trigger it.

**Recommendation**: Document the limitation in a code comment for `UpsertAlias` mirroring the existing pattern; add a package-level mutex as a follow-up cleanup ticket that covers all config writers.

---

## PITFALL-2: Alias Name Validation Gap — Server Side Has No Regex Guard (MEDIUM)

### Risk

`AliasConfig.Name` carries a comment in `config/config.go` (line 442):

```go
// Name is the unique alias identifier (e.g. "myproj"). Must match ^[\w-]+$.
```

The frontend `AliasDetector` (line 54 in `AliasDetector.ts`) correctly enforces `^@[\w-]+$` for omnibar detection. But the existing `UpsertProfile` handler in `defaults_service.go` (lines 97–99) only checks `Name == ""` — no regex validation of the actual characters. If `UpsertAlias` follows the same pattern, a name containing spaces, slashes, or special characters will be written to `config.json` and silently fail to trigger at the omnibar (the alias regex won't match it).

### Specific failure modes

- `"my alias"` (space): written to disk, never detectable via `@my alias`
- `"foo/bar"` (slash): written to disk, omnibar regex ignores it
- `""` (empty after trim): guard catches this, but `"  "` (only whitespace, not trimmed) would not be caught without an explicit `strings.TrimSpace` + empty check

### Mitigation

Add server-side validation in `UpsertAlias`:

```go
var aliasNameRE = regexp.MustCompile(`^[\w-]+$`)

if !aliasNameRE.MatchString(req.Msg.Alias.Name) {
    return nil, connect.NewError(connect.CodeInvalidArgument,
        fmt.Errorf("alias name %q must match ^[\\w-]+$ (letters, digits, hyphens, underscores only)", req.Msg.Alias.Name))
}
```

Add the same pattern as a `pattern` attribute on the name `<input>` in `AliasesManager.tsx` for immediate feedback.

---

## PITFALL-3: Live Reload — Config Is Read Per-Request, But In-Memory Session Resolution Uses Startup Config (MEDIUM)

### Risk

Two layers of "config" exist in the running server:

1. **RPC handlers** call `config.LoadConfig()` on every request — this always reads `config.json` from disk. So `UpsertAlias` and `ListAliases` see the latest file state immediately. **No restart required for the settings UI itself.**

2. **Session creation** in `session_service.go` also calls `config.LoadConfig()` per-request (verified at lines 580, 1023, 3851, 3906). Aliases are resolved at session-create time by loading config fresh. **No restart required for alias resolution either.**

So the good news is: **saving an alias via the API takes effect immediately for both the Settings UI and for the next `@alias` session creation**. The service restart described in the current workaround is only needed because users have been manually editing the JSON file (which doesn't trigger any in-process re-read at all).

### Remaining risk

The Monaco "Config Files" editor and the new `AliasesManager` can show stale data to each other **if both are open simultaneously in two browser tabs** — but this is not a correctness problem, just a UX staleness issue. The requirements document already notes this is acceptable for now.

### Mitigation

No restart banner is needed. Confirm the behavior in the requirements document: saves are live. Add a comment near `ListAliases` in `defaults_service.go` noting that `LoadConfig` is per-request and that no cache invalidation is needed.

---

## PITFALL-4: `env_vars` Map Editing — Key-Value UI Complexity vs. TEXT Area (MEDIUM)

### Risk

`AliasConfig.EnvVars` is `map[string]string`. The existing `ProfilesManager.tsx` silently omits `EnvVars` from the form (it hardcodes `envVars: {}` at line 144). This is acceptable for profiles but aliases are specifically designed to be invoked with pre-set env vars (e.g., per-project API keys, `CLAUDE_MODEL` overrides).

Building a full key-value pair editor (add row / edit key / edit value / delete row) is 3–5× more complex than the rest of the `AliasesManager` form. Known React pitfalls:

- **Stale closure on row delete**: using the index as the React key means removing row 2 of 3 causes row 3 to receive the key `"2"` — if key-based reconciliation is used without a stable row identifier, edits to the wrong row silently survive.
- **Duplicate key collision**: two rows with the same key are valid in the form state but invalid as a Go `map[string]string` — the second write silently wins on the backend.
- **Empty-key rows**: a row with an empty key field will be serialized as `{"": "value"}` — valid JSON, invalid env var, no error surfaced.

### Mitigation

For this iteration, use a `KEY=VALUE` textarea (one pair per line) parsed client-side into `Record<string, string>` before the RPC call. Validation rules:
- Skip blank lines
- Lines without `=` are an error (surface inline)
- Duplicate keys are an error or last-write-wins (document clearly)
- Empty key (line starts with `=`) is an error

This matches the `CLIFlags` field (already a plain text input) and avoids all the dynamic-list React pitfalls. The full key-value UI can be promoted in a follow-up once the simpler text approach validates user needs.

---

## PITFALL-5: Malformed `config.json` on Load — No Atomic Recovery Path (LOW)

### Risk

`LoadConfigFromPath` (line 819–822 of `config.go`) calls `json.Unmarshal` on the entire file. If `config.json` is malformed (partial write from a crash, hand-edit typo), `LoadConfig` returns `DefaultConfig()` (line 764–767) — this means any RPC handler that calls `cfg := config.LoadConfig()` then `SaveConfig(cfg)` will **silently replace the malformed file with a defaults-only config**, destroying all user settings.

This pre-existing risk is slightly amplified by the new alias RPCs because the Settings UI gives users a more prominent way to accidentally trigger saves while the file is in a bad state.

### Mitigation

The existing raw Monaco editor in "Config Files" tab mitigates this: the `UpdateClaudeConfig` RPC calls `mgr.UpdateConfigWithValidation` before writing. For alias saves specifically:

1. On `UpsertAlias`/`DeleteAlias`: check if the returned config has `ConfigVersion == 0` after `LoadConfig()` — this is the signal that defaults were used (because the file was missing or corrupted). Return a `CodeInternal` error instead of saving.
2. Alternatively, expose a `LoadConfigStrict` variant that returns an error (not defaults) when the file exists but is malformed.

This is a defense-in-depth improvement; mark it as P1 for a separate cleanup, not a blocker for this feature.

---

## PITFALL-6: Tag List Key Collision on Duplicate Values (LOW)

### Risk

The `ProfilesManager.tsx` tag implementation (lines 334–345) uses the tag value as the React `key`:

```tsx
{form.tags.map((t) => (
  <span key={t} className={tagClass}>
```

If a user adds the same tag twice (e.g., "work", "work"), React will throw a duplicate-key warning, and the `handleRemoveTag` function filters by value — removing either "work" instance removes both.

The `handleAddTag` guard at line 178 (`if (trimmed && !form.tags.includes(trimmed))`) prevents duplicates in `ProfilesManager`. The `AliasesManager` must replicate this guard or the same bug occurs.

### Mitigation

Copy the duplicate-prevention guard from `ProfilesManager`. Add a test case for the "add duplicate tag" scenario.

---

## PITFALL-7: `\w` in `^[\w-]+$` Includes Unicode Word Characters in Go but Not in Most JS Engines (LOW)

### Risk

Go's `regexp` package uses RE2 semantics where `\w` matches `[0-9A-Za-z_]` only (ASCII). JavaScript's `/^[\w-]+$/` also matches ASCII `\w` only in most engines (not Unicode by default). So they agree on valid characters. However, if the user enters a name like `café` (with Unicode), Go silently rejects it (regex fails), but a naive client-side regex `/^[\w-]+$/` without the `u` flag also rejects it — the behaviors match.

The risk is confusion, not a correctness bug: users may expect emoji or accented characters in alias names to work. The error message from the server should reference the exact allowed character set.

### Mitigation

The `CodeInvalidArgument` error message in PITFALL-2's mitigation already includes the character class. No additional action needed, but document the constraint in the form field's `title` or `placeholder` attribute.

---

## Summary of Risk Levels

| Pitfall | Severity | Action Required |
|---------|----------|-----------------|
| P-1: Concurrent write race — last write wins | HIGH | Add mutex or document limitation; file follow-up ticket |
| P-2: No server-side regex on alias name | MEDIUM | Add `^[\w-]+$` guard in `UpsertAlias` handler |
| P-3: Live reload confusion — restart NOT needed | MEDIUM | Update requirements doc; no restart banner needed |
| P-4: `env_vars` key-value UI complexity | MEDIUM | Use `KEY=VALUE` textarea for this iteration |
| P-5: Malformed config.json destroys all settings | LOW | Add ConfigVersion==0 guard; log warning; defer to follow-up |
| P-6: Duplicate tag key collision | LOW | Copy duplicate-prevention guard from ProfilesManager |
| P-7: `\w` Unicode ambiguity | LOW | Clear error message; document in form placeholder |

**Top 3 to address before coding**: P-2 (server-side name regex — prevents silent bad data in config.json), P-3 (confirm no restart required — shapes whether a banner is needed), P-4 (env_vars UX decision — determines form complexity before layout is built).
