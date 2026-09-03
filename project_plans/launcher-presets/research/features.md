# Research: Feature Landscape for Launcher Presets

## TL;DR

The single most important research finding: **`AliasConfig` (`config/types.go:234`) is already
almost exactly "Launcher Presets"** — a named, user-configured shortcut with `program`,
`cli_flags`, `path`, `tags`, `env_vars`, `session_type`, invoked via `@name` in the Omnibar
(`AliasDetector`, priority 36) and resolved server-side into a full `CreateSessionRequest` via
`config.ResolveAlias`. The literal doc comment on the struct reads: *"AliasConfig defines a
named session preset invoked via `@name` in the omnibar."* Phase 3 must explicitly decide
whether Launcher Presets is a **new parallel system** or an **extension of Aliases** (add an
`argv []string` field + a separate hand-edited-JSON storage mode), because building both
un-reconciled risks two overlapping "named shortcut" concepts with different storage, different
invocation syntax, and different editability models.

## 1. Existing overlapping features in this codebase

### 1a. `profile` (closest precedent named in requirements.md)

- Proto: `ProfileDefaultsProto` (`proto/session/v1/session.proto:1692`) — `name`, `description`,
  `program`, `auto_yes`, `tags`, `env_vars`, `cli_flags`.
- Go: `ProfileDefaults` struct, `config/types.go:177`.
- RPCs: `UpsertProfile` / `DeleteProfile` / `GetSessionDefaults` (`server/services/defaults_service.go`) —
  **editable via UI** (`SessionWizard.tsx:268` calls `client.upsertProfile(...)`), stored inside
  `config.json`'s `SessionDefaults.Profiles` map.
- Applied via `CreateSessionRequest.profile` (field 11) → `config.ResolveDefaults(cfg, workingDir, profile)`
  → merges global → directory-rule → profile → explicit-request-field layers
  (`server/services/session_service.go:1412`).
- **`cli_flags` is a single string, not argv.** At launch time it is naively whitespace-split:
  `session/instance_tmux.go:115` — `for _, f := range strings.Fields(i.CLIFlags) { ... }`. This
  is the exact shell-quoting-ambiguity problem the requirements doc (and herdr-web) call out —
  a preset value like `ssh -t host 'cd ~/repo && exec claude'` would be mis-split by
  `strings.Fields` (naive whitespace split has no quote-awareness), corrupting the remote-exec
  command. Profiles do not solve the argv problem; they only solve "here are some named
  defaults for program/tags/env/flags."

### 1b. `AliasConfig` (undocumented-in-requirements, but the real closest precedent)

- Go: `AliasConfig`, `config/types.go:234`, doc comment: *"defines a named session preset
  invoked via @name in the omnibar."*
- Proto: `AliasProto`, `proto/session/v1/session.proto:1819` — `name`, `group`, `path`,
  `description`, `profile`, `program`, `auto_yes`, `tags`, `env_vars`, `cli_flags`,
  `session_type`, `name_prefix`.
- RPCs: `ListAliases` / `UpsertAlias` / `DeleteAlias` — **editable via UI**, stored in
  `config.json`'s `SessionDefaults.Aliases` list.
- Frontend detection: `AliasDetector` (`web-app/src/lib/omnibar/detectors/AliasDetector.ts`),
  priority 36, dynamically registered/unregistered in `OmnibarContext.tsx` (same pattern
  `WorkflowDetector` uses) whenever the alias list changes (fetched via `useAliases` hook).
  Grammar: `@<name>[:<branch>][ <label text>][ --<extra-flags>]`. This is functionally
  identical to the requirement's Nice-to-Have `preset:<id>` shorthand — same priority tier,
  same "dynamic, data-driven detector registered at runtime" pattern.
- Backend resolution: `CreateSessionRequest.alias_name` (field 27) → `config.ResolveAlias(cfg,
  aliasName, branch, title, extraFlags)` (`config/defaults.go:235`) → `config.FindAlias`
  (`config/defaults.go:180`) — fully resolves path/profile/program/flags server-side before
  session creation, referenced at `server/services/session_service.go:1381-1404`.
- Same `cli_flags`-is-a-string limitation as profiles (same `strings.Fields` split site).
- Aliases even have `session_type` override and `name_prefix` — features launcher presets
  don't ask for, suggesting Aliases are a superset in some dimensions.

### 1c. `AvailablePrograms` (explicitly called out as distinct in requirements.md — confirmed)

- `config/config.go:736` `GetAvailablePrograms()` — shells out `which <candidate>` for a
  hardcoded candidate list (`proxy-claude`, `claude`, `claude-code`, `gemini`, `agy`) per-shell
  (zsh/bash sourcing profile files first). Auto-discovery only; no user authorship, no argv, no
  working directory. Correctly identified in requirements.md as orthogonal — confirmed, no
  overlap risk here.

### 1d. Net delta: what genuinely doesn't exist yet

1. **True `argv []string` launch path.** Neither `profile.cli_flags` nor `alias.cli_flags` is
   an array — both are single strings split with `strings.Fields` at
   `session/instance_tmux.go:115`, which is not quote-aware. This is the one real gap that
   matches the requirement's "no shell interpolation of preset-supplied strings" success
   criterion (#3). Any plan must either (a) add `repeated string argv` fields end-to-end
   (proto → Go → tmux launch) or (b) prove `strings.Fields` splitting is acceptable for v1 and
   defer argv properly — but note doing (b) would fail success criterion #3 as written.
2. **Standalone hand-edited JSON file, not `config.json`.** Both Profiles and Aliases live
   inside the monolithic `config.json` and are mutated via RPC (`Upsert*`). The requirement
   explicitly wants a separate `~/.stapler-squad/launcher-presets.json` that's *not* editable
   through the UI. This is a deliberate scoping choice in requirements.md ("Out of Scope:
   Editing presets through the UI") — but it means presets, if built as fully separate from
   Aliases, introduce a **third** named-shortcut storage location/model in the same product
   surface (Profiles in config.json+UI, Aliases in config.json+UI, Presets in a dedicated
   file+hand-edit-only). Phase 3 should weigh: is a third model justified by the "argv +
   hand-edited, restart/reload semantics" distinction, or should this instead become "Aliases
   gain an optional `argv` field," reusing the existing storage/RPC/detector machinery?
3. **A one-click UI list**, as opposed to the alias's typed-shorthand-only UX. Aliases have no
   equivalent of "browse a Presets panel and click one" in `OmnibarCreationPanel.tsx` today —
   `AliasBrowse` detection type exists for the `@`-typed palette, but there's no static
   always-visible list section in the creation panel itself. This part of the requirement is
   genuinely net-new UI regardless of which backend model is chosen.

## 2. Industry precedent: herdr-web (per requirements.md summary)

Per the requirements doc's own description (no direct repo access needed — summarized from
`bridge/src/launcher_presets.rs` / `web/src/launcherPresets.ts` references):

- JSON config file of named presets.
- Each preset stores an explicit `argv` array, not a shell string — specifically to avoid
  quoting ambiguity for paths with spaces or nested quoting in remote-exec commands
  (`ssh -t host '...'`).
- This is the direct design inspiration for the Must-Have "argv-based launch, not shell
  strings" requirement, and maps 1:1 onto stapler-squad's net gap identified in 1d.1 above.

No further external research was conducted per task instructions (design summarized from the
requirements doc, not fetched from the herdr-web repo directly).

## 3. Edge cases and failure modes the design should handle

| Edge case | Recommended handling | Precedent in codebase |
|---|---|---|
| Duplicate preset `id` in the config file | Reject the **whole file** at load with a clear error (already stated as Must-Have) | No direct precedent — `AliasConfig` has no analogous "reject whole list on one bad entry" behavior today; check `config/defaults.go` alias loading path in Phase 3 to confirm aliases don't already silently dedupe/drop, since presets should not repeat that gap |
| Malformed / non-JSON config file | Fail loudly at startup (log + surface via RPC error, not silent empty list) — already Must-Have | `NewWatchDirWatcher` (`session/unfinished/watcher.go:24`) models graceful *degradation* (fsnotify unavailable → falls back to polling) but that's for a different failure class (missing OS feature, not corrupt user input) — don't reuse that "degrade gracefully" pattern for schema/JSON errors, which should hard-fail per requirements |
| Empty `argv` on a preset | Validation error at load time (already Must-Have: "validate ... non-empty argv") | None directly; mirror the "reject whole file" behavior rather than skip-and-warn |
| Preset references a `program` not in `AvailablePrograms` | Requirements don't mandate cross-validation against `AvailablePrograms` — and shouldn't: `AvailablePrograms` is PATH-based auto-discovery on the *server host*, while presets may reference programs correctly present but not on the hardcoded candidate list (e.g. `codex`, which isn't even in `GetAvailablePrograms`'s candidate list at `config/config.go:747`). Recommend: do NOT hard-fail if `program` isn't in `AvailablePrograms`; that list is a UI convenience list, not an allowlist. At most, warn softly in the UI. |
| Config file deleted/moved after server startup (no live reload) | Must-Have scope only requires "without restart, or after an explicit reload" — so a "Reload presets" RPC/button is the minimum viable answer here; document that presets simply persist in memory from last successful load until explicitly reloaded, and a subsequent reload against a now-missing file should be treated as "0 presets" with a clear message, not an error that wipes previously-loaded state ambiguously |
| fsnotify live-reload (Nice to Have) races a mid-write partial file | Follow `WatchDirWatcher`'s existing fsnotify degrade-to-polling fallback (`session/unfinished/watcher.go:28-34`) for *availability*, but debounce reload events and re-validate full-file-atomically (reject-whole-file-on-error) so a half-written file during `fsnotify.Write` doesn't transiently wipe/corrupt the in-memory preset list |
| `argv[0]` conflicts with / duplicates the `program` field | Requirements.md's own Open Questions flags this ("does argv fully replace program+args, or compose with existing fields") — unresolved, must be decided in Phase 3, not this research doc |
| XDG fallback path also present alongside primary path (both exist) | Define explicit precedence (primary wins) and log a warning if both exist to avoid silent confusion about which file is authoritative |
| Preset `id` collides with an existing Alias `name` when a user types `preset:<id>` vs `@<id>` | Since these are different detector namespaces (`preset:` prefix vs `@` prefix) no runtime collision occurs, but document this explicitly since a user maintaining both Aliases and Presets for the same purpose is a symptom of the model 1d.2 overlap risk, not an edge case to code around |

## 4. Unstated user needs beyond explicit requirements

- **Preset ordering** — requirements.md's schema is a flat JSON array; users will likely expect
  the Omnibar list to preserve file order (or alphabetical) rather than arbitrary map iteration
  order. Since the schema already specifies `"presets": [...]` (array, not map-by-id), preserving
  declaration order through load → RPC → render is low-cost and should be an explicit design
  decision, not an accident of Go map iteration.
- **Preset grouping** — `AliasConfig` already has a `Group` field (`config/types.go:238`,
  "optional display group for palette organization") purely for UI organization. If the preset
  list grows past a handful of entries, an equivalent `group` field (or reuse of existing tag
  infra) is a predictable near-term ask; worth flagging even though out of the current Must/Nice
  scope so it's not accidentally precluded by the v1 schema (e.g. leave room in the JSON schema
  for a future optional field without a breaking `"version"` bump).
- **Keyboard-triggerable / typed shorthand** — already captured as the Nice-to-Have
  `preset:<id>` detector; per 1b above, this would essentially clone `AliasDetector`'s pattern
  wholesale, reinforcing the "should this be Aliases+argv" question.
- **Preset icons/colors** — no evidence of demand elsewhere in the codebase (Aliases have no
  icon/color field either); low priority, likely not worth adding to the v1 schema.
- **A "save current form as a preset" affordance** — `SessionWizard.tsx` already has this exact
  UX for Profiles ("Save the current configuration as a reusable profile",
  `SessionWizard.tsx:853`). Requirements.md explicitly scopes this Out of Scope for presets
  ("Editing presets through the UI... config file is hand-edited only"), but expect this to be
  the first follow-up request once users see the Profile equivalent exists — worth a one-line
  callout in the plan so the "hand-edited only" decision reads as deliberate, not an oversight.
- **Distinguishing three "named shortcut" concepts in the UI.** If Presets ship as a fully
  separate system from Profiles and Aliases, the Omnibar/creation panel will have three
  different "pick a saved thing" affordances (`profile` dropdown in `SessionWizard.tsx:519`,
  `@alias` typed shorthand + browse palette, and a new "Presets" list section). This is a
  concrete UX-clarity risk worth flagging to Phase 3/pm-triad-review even though it's a design
  question rather than a pure engineering one.

## Key files for Phase 3 planning

- `config/types.go:177` (`ProfileDefaults`), `config/types.go:234` (`AliasConfig`)
- `config/defaults.go:180` (`FindAlias`), `config/defaults.go:235` (`ResolveAlias`)
- `proto/session/v1/session.proto:472-531` (`CreateSessionRequest`), `:1692` (`ProfileDefaultsProto`), `:1819` (`AliasProto`)
- `server/services/session_service.go:1378-1436` (defaults/alias/cli_flags resolution + append order)
- `session/instance_tmux.go:115` (`strings.Fields(i.CLIFlags)` — the naive-split site any argv work must bypass)
- `web-app/src/lib/omnibar/detectors/AliasDetector.ts`, `web-app/src/lib/contexts/OmnibarContext.tsx:75-125` (dynamic detector registration pattern to reuse for `preset:<id>`)
- `session/unfinished/watcher.go:16-50` (fsnotify-with-polling-fallback pattern to reuse for live reload)
- `web-app/src/components/sessions/SessionWizard.tsx:268,853` (existing "save as profile" UX, for contrast with presets' hand-edit-only scope)
