# Build vs Buy: AliasesManager UI + Backend CRUD

**Date**: 2026-06-21
**Feature**: Alias session preset management in Settings UI

---

## Summary

Build everything from scratch by forking `ProfilesManager`. No external libraries needed for the form layer; a lightweight bespoke key-value editor is the right call for `env_vars`. The cost of pulling in react-hook-form or a third-party KV editor exceeds any benefit at this problem size.

---

## Option 1: OSS Form Library (react-hook-form / Formik)

### Verdict: Skip for this feature

`react-hook-form` (v7.63.0) and `@hookform/resolvers` (v5.2.2) are **already installed** in `web-app/package.json`. `zod` (v4.1.11) is also present. The combination is used in exactly one place: `SessionWizard.tsx` — a deprecated component scheduled for removal.

`ProfilesManager.tsx`, `DirectoryRulesManager.tsx`, and `GlobalDefaultsForm.tsx` all use raw `useState` form state. The pattern across settings components is deliberate: these forms have 5–11 fields each, no async field-level validation, and no multi-step logic. React-hook-form's value proposition (performance via uncontrolled inputs, field array abstractions, schema resolver integration) kicks in at higher complexity than the AliasesManager form requires.

Introducing react-hook-form here would:
- Break consistency with every other settings component
- Add abstraction complexity for a form that can be fully expressed in ~50 lines of `useState`
- Require justifying a pattern divergence in code review with no concrete benefit

**Decision**: Do not use react-hook-form for this feature. Follow the existing `ProfilesManager` state pattern.

---

## Option 2: SaaS / Managed API

### Verdict: Not applicable

Alias data lives exclusively in `~/.stapler-squad/config.json` on the user's local machine. The app is a single-user local Go server with no cloud backend. There is no managed-API angle here.

---

## Option 3: Key-Value Pair Editor — Library vs Bespoke

### Verdict: Build bespoke (< 50 lines)

The `env_vars` field is `map[string]string` in Go / `Record<string, string>` in TypeScript. Available OSS options for a React KV editor:

| Option | Notes |
|---|---|
| `react-editable-json-tree` | Full JSON tree editor — massive overkill; ~25 kB min+gzip |
| `react-json-view` (npmjs) / `@microlink/react-json-view` | Viewer first, editing is secondary; API not typed for Records |
| `react-keyvalue-editor` (various) | Tiny unmaintained packages; none have meaningful download counts or typed exports |
| Roll bespoke | ~40–50 lines: `[{key, value}]` array in local state, `+` / `✕` buttons, serialize to `Record<string,string>` on submit |

The `ProfilesManager` already implements the **exact same pattern** for the `tags` field: a local `string[]` state, an "Add" button, remove-per-item buttons, and serialization on save. The KV editor is that pattern extended to pairs. There is no library that fits the existing vanilla-extract + raw-useState approach better than a 50-line bespoke component.

An alternative fallback (per requirements rabbit holes) is a `KEY=VALUE`-per-line textarea parsed on submit. This is simpler still but worse UX than discrete rows. The bespoke row editor is recommended unless `env_vars` usage turns out to be rare.

**Decision**: Build a bespoke `EnvVarsEditor` sub-component inside `AliasesManager.tsx`. Model it on the existing tag editor in `ProfilesManager`.

---

## Option 4: Fork / Adapt ProfilesManager

### Verdict: Fork and adapt — this is the primary implementation strategy

`ProfilesManager.tsx` (390 lines) and `ProfilesManager.css.ts` (189 lines) map almost directly onto the `AliasesManager` requirements:

| `ProfilesManager` element | `AliasesManager` equivalent | Delta |
|---|---|---|
| `ProfileFormData` interface | `AliasFormData` | Add: `group`, `path`, `profile`, `cli_flags`; keep: `name`, `description`, `program`, `auto_yes`, `tags`; add: `env_vars` (new KV editor) |
| `upsertProfile` RPC call | `upsertAlias` RPC call | RPC not yet defined in proto — must add |
| `deleteProfile` RPC call | `deleteAlias` RPC call | Same pattern |
| `loadProfiles` (loads `getSessionDefaults`) | `loadAliases` (`listAliases` RPC already exists) | `ListAliases` RPC is implemented in `DefaultsService.ListAliases` |
| Tag list + add/remove UI | Same for tags | Reuse unchanged |
| Form card layout | Same layout | Reuse unchanged |
| CSS tokens | Same token set | Reuse `ProfilesManager.css.ts` wholesale, rename exports |

The backend already has:
- `ListAliases` RPC and `AliasProto` message (session.proto lines 381–1791)
- `aliasConfigToProto` helper (`defaults_service.go:224`)
- `config.AliasConfig` struct with all fields (`config.go:441–463`)
- `config.SaveConfig` / `config.LoadConfig` pattern used by `UpsertProfile` and `DeleteProfile`

What does **not** yet exist (must build):
1. `UpsertAlias` and `DeleteAlias` proto RPCs (add to session.proto)
2. `UpsertAlias` / `DeleteAlias` handler methods on `DefaultsService` (model on `UpsertProfile`/`DeleteProfile` — ~70 lines total)
3. `AliasesManager.tsx` + `AliasesManager.css.ts` (fork `ProfilesManager`, add group/path/profile/cli_flags fields + KV editor)
4. Proto bindings regeneration (`make generate-proto`)
5. Registry update (`make registry-generate`)

**Decision**: Fork `ProfilesManager.tsx` and `ProfilesManager.css.ts` as the starting point. The backend UpsertAlias/DeleteAlias handlers follow the `UpsertProfile`/`DeleteProfile` pattern verbatim (~70 lines). Total new code estimate: ~250 lines frontend, ~80 lines backend.

---

## Recommendation Summary

| Layer | Approach | Rationale |
|---|---|---|
| Form state | Raw `useState` (same as all other settings components) | Consistency; react-hook-form already installed but not used in settings |
| Backend CRUD | Fork `UpsertProfile`/`DeleteProfile` pattern in `DefaultsService` | Pattern is well-tested; config save is atomic via temp-file rename |
| `env_vars` editor | Bespoke `EnvVarsEditor` sub-component (~50 lines) | No library fits; tag editor in `ProfilesManager` is the direct analog |
| Component scaffolding | Fork `ProfilesManager.tsx` / `.css.ts` | 70–80% of the final component already exists |

---

## Key Files Referenced

- `/home/tstapler/Programming/stapler-squad/web-app/src/components/settings/ProfilesManager.tsx` — fork target
- `/home/tstapler/Programming/stapler-squad/web-app/src/components/settings/ProfilesManager.css.ts` — CSS fork target
- `/home/tstapler/Programming/stapler-squad/server/services/defaults_service.go` — backend fork target (UpsertProfile/DeleteProfile at lines 89–162)
- `/home/tstapler/Programming/stapler-squad/proto/session/v1/session.proto` — add UpsertAlias/DeleteAlias RPCs (AliasProto already at line 1773)
- `/home/tstapler/Programming/stapler-squad/config/config.go` — `AliasConfig` struct (line 441)
