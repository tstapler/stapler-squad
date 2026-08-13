# Requirements: Launcher Presets

Status: Draft | Phase: 1 - Ideation complete (non-interactive; migrated from GitHub issue #176)
Created: 2026-08-06
Complexity: 3 (full-stack feature: config loading + RPC + frontend UI + registry touchpoints)

## Problem Statement

Stapler-squad already supports configurable programs (`AvailablePrograms`, auto-detected CLI
binaries — see `config/config.go:736`) and a `program` string field on `CreateSessionRequest`
(`proto/session/v1/session.proto:486`), but there is no way to save a frequently-used, fully
specified launch command — program + flags + default working directory — as a named,
one-click shortcut. Users who repeatedly launch `codex --model gpt-5` or `ssh -t host 'cd
~/repo && exec claude'` must re-type or re-select these every time from the Omnibar.

`herdr-web` (`bridge/src/launcher_presets.rs`, `web/src/launcherPresets.ts`) solves this with a
JSON config file of named presets, each with an explicit `argv` array (not a shell string) to
avoid quoting ambiguity — especially for paths with spaces or nested quoting in remote-exec
commands.

## Success Criteria

1. A user can define a preset in a JSON config file and see it appear as a selectable shortcut
   in the Omnibar launch flow without restarting the server (or after an explicit reload).
2. Selecting a preset pre-fills the session-creation form (program, argv/flags, working
   directory) rather than silently launching — the user still reviews/submits.
3. Presets using multi-argument `argv` (including remote-exec forms like `ssh -t host '...'`)
   launch correctly with no shell-quoting corruption.
4. Malformed preset config (bad JSON, missing required fields, duplicate IDs) fails loudly
   (surfaced to the user/logs) rather than silently dropping presets or crashing the server.

## Scope

### Must Have (MoSCoW)

- **Preset config file** — load from `~/.stapler-squad/launcher-presets.json`, schema:
  `{ "version": 1, "presets": [{ "id", "label", "argv": [...], "program"?, "default_path"? }] }`.
- **Load + validation on every `GetLauncherPresets` call** — parse fresh on each RPC call (no
  startup-time load, no in-memory cache — see `implementation/plan.md`'s Pattern Decisions
  "Live reload" row: a fresh-per-call read satisfies this item's "without restarting the
  server" requirement for free, since every Omnibar-open *is* a reload); reject the whole file
  with a clear error on schema/JSON errors (do not partially apply); validate `id` uniqueness
  and non-empty `argv`. *(Revised from an earlier "parse at server startup" draft — a
  cross-artifact consistency check during Phase 4 validation found that wording directly
  contradicted the deliberate fresh-per-call design in plan.md; this bullet now matches the
  implemented decision.)*
- **`GetLauncherPresets` RPC** — read-only list endpoint returning loaded presets, alongside
  (not replacing) the existing `AvailablePrograms` list.
- **Omnibar surface** — a "Presets" section/list in the existing launch panel
  (`OmnibarCreationPanel.tsx`). Selecting a preset pre-fills the creation form fields it maps
  to (program, argv-derived flags, default path if present) using the existing 7-touchpoint
  session-creation flow — see Constraints below.
- **argv-based launch, not shell strings** — presets are stored and transmitted as string
  arrays; no shell interpolation of preset-supplied strings at any point in the pipeline.

### Nice to Have

- **Live reload** — watch the config file (`fsnotify`, already a project dependency per
  `session/unfinished/watcher.go`) and hot-reload presets without a server restart.
- **`preset:<id>` omnibar shorthand** — a new lowest/near-lowest priority `DetectorRegistry`
  entry (see `.claude/rules/feature-testing-registry.md`) that matches typed `preset:<id>` and
  resolves directly to that preset's prefill, skipping manual selection.
- **XDG fallback path** — also check `~/.config/stapler-squad/launcher-presets.json` if the
  primary path is absent.

### Out of Scope (this iteration)

- Editing presets through the UI (config file is hand-edited only, matching herdr-web's model).
- Per-workspace or per-project preset scoping (global list only).
- A brand-new `SessionType` enum value — a preset is a convenience prefill for the *existing*
  creation flow, not a structurally distinct session lifecycle (see Constraints).
- Preset-level secrets/credential injection.

## Constraints

**Tech stack:** Go backend (`config/`, `server/services/`), ConnectRPC/Protobuf
(`proto/session/v1/`), React/TypeScript frontend (`web-app/src/components/sessions/`,
`web-app/src/lib/omnibar/`).

**No raw `argv` field exists yet on `CreateSessionRequest`** (`proto/session/v1/session.proto:472-531`)
— only a single `program` string (field 5) and `prompt` (field 7). Supporting multi-argument
presets (e.g. `["codex", "--model", "gpt-5"]` or an `ssh -t host '...'` remote-exec form)
requires either a new `repeated string argv` field or decomposing preset argv into
`program` + a new flags/args mechanism at the plan stage — this is an open design question for
Phase 3 (Plan), not resolved here.

**Session-creation registry constraint** — per `.claude/rules/session-creation-registry.md`,
any new *session type* requires touching 7 places (proto enum, request fields, Go handler,
`session/instance.go`, `Omnibar.tsx`, `OmnibarCreationPanel.tsx`, `OmnibarContext.tsx` +
`useSessionService.ts`). Per the reference implementation pattern used for one-off sessions,
launcher presets should be modeled as **a prefill/shortcut on top of the existing directory/
worktree session types**, not a new `SessionType` enum value — confirm this decision explicitly
in Phase 3 rather than defaulting into a new enum value by accident.

**Feature registry** — per `.claude/rules/feature-registry.md`, the new RPC
(`GetLauncherPresets`) and the new Omnibar UI surface each need per-feature JSON entries under
`docs/registry/features/` and `make registry-generate` run before the PR is complete.

**CSS** — any new Omnibar UI (`Presets` section) must use vanilla-extract (`.css.ts`), per
`.claude/rules/css-architecture.md` — no new `.module.css` files.

## Context

### Existing Work

- `config/config.go:736` `GetAvailablePrograms()` — auto-detects CLI binaries on `PATH`; this
  is a *different* mechanism from user-authored presets and should not be conflated in the RPC
  response (presets are explicit and static; available programs are auto-discovered).
- `proto/session/v1/session.proto:472` `CreateSessionRequest` — has `program` (string, field 5),
  `working_dir` (field 3), `path` (field 2), `profile` (field 11, "named profile's defaults") —
  the existing `profile` field is the closest existing precedent for "apply a named preset's
  defaults" and should be reviewed in Phase 2 research for reuse vs. duplication.
- `web-app/src/lib/omnibar/detector.ts` — `DetectorRegistry`, priority-sorted; a `preset:<id>`
  detector (Nice to Have) would register here or dynamically in `OmnibarContext.tsx` (pattern:
  `WorkflowDetector`/`AliasDetector`, which are also data-driven and registered at runtime).
- `session/unfinished/watcher.go` — existing `fsnotify` usage in this codebase; reuse this
  pattern rather than introducing a new file-watching dependency.

### What's Missing

1. Any config file schema/loader for user-authored launch presets.
2. A backend RPC to expose presets to the frontend.
3. A raw multi-argument launch path (`argv`) on `CreateSessionRequest` — today only a single
   `program` string exists.
4. Frontend UI (Presets section) and prefill wiring in the Omnibar creation flow.

## Open Questions

- Does a preset's `argv` fully replace `program` + args, or does it need to compose with
  existing fields (`working_dir`, `branch`, `profile`)? *(for Phase 2/3 research + plan)*
- Should remote-exec presets (`ssh -t host '...'`) create a session backed by a *local* tmux
  pane running `ssh`, or is there a need for a distinct remote session concept? Assume local
  tmux pane running the argv as-is (no new session type) unless research finds otherwise.
- Live reload (fsnotify) — Must Have or Nice to Have? Marked Nice to Have here; confirm in
  Phase 2 if restart-to-reload is an acceptable v1 UX.
