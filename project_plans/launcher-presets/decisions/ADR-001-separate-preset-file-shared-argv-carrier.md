# ADR-001: Launcher Presets as a Separate Hand-Edited File, With a Shared `extra_args` Argv Carrier on `CreateSessionRequest`

**Status**: Accepted
**Date**: 2026-08-06

## Context

Research for this feature (`project_plans/launcher-presets/research/features.md`,
`architecture.md`, `pitfalls.md`, `ux.md`) found that stapler-squad already ships two
overlapping "named launch shortcut" mechanisms:

- **`ProfileDefaults`** (`config/types.go:177`) — named defaults, editable via
  `UpsertProfile`/`DeleteProfile` RPCs, stored in `config.json`.
- **`AliasConfig`** (`config/types.go:234`) — doc comment literally reads "defines a named
  session preset invoked via `@name` in the omnibar," editable via `UpsertAlias`/`DeleteAlias`
  RPCs, stored in `config.json`, invoked via a `@name` `DetectorRegistry` entry
  (`AliasDetector`, priority 36).

Both `ProfileDefaults.CLIFlags` and `AliasConfig.CLIFlags` are a single string, naively
whitespace-split via `strings.Fields` at `session/instance_tmux.go:115` before being
shell-quoted per-token. This split is not quote-aware: a value like
`ssh -t host 'cd ~/repo && exec claude'` is shredded, corrupting exactly the remote-exec case
`requirements.md` calls out (Success Criterion 3).

Requirements.md explicitly scopes Launcher Presets as a **separate, hand-edited-only**
`~/.stapler-squad/launcher-presets.json` file (Out of Scope: "Editing presets through the
UI"), distinct from `config.json`. Without an explicit decision, this risks shipping a
**third**, unreconciled "named shortcut" concept alongside Profiles and Aliases.

## Decision

1. **Launcher Presets is a separate system**: its own JSON file
   (`~/.stapler-squad/launcher-presets.json`, resolved via `config.GetConfigDir()`), its own
   read-only `GetLauncherPresets` RPC, its own frontend hook (`useLauncherPresets.ts`) and UI
   component (`OmnibarPresetList.tsx`). It is **not** built by extending `AliasConfig`.
2. **The launch pipeline gap is fixed once, generically, and shared**: a new
   `repeated string extra_args = 28` field is added to `CreateSessionRequest` (and a parallel
   `ExtraArgs []string` field on `session.Instance`/`InstanceOptions`), carried through
   `buildLaunchCommand` (`session/instance_tmux.go`) via the existing `shellQuote` helper —
   the same per-token POSIX-quoting primitive `CLIFlags` already uses, just applied to a real
   `[]string` instead of a `strings.Fields`-split string. This carrier is preset-agnostic: any
   future caller (including a hypothetical future `AliasConfig.Argv` field) can populate
   `extra_args` without a second implementation of argv-safe launching.
3. **A preset's `argv` decomposes mechanically**: `argv[0]` → `CreateSessionRequest.program`;
   `argv[1:]` → `extra_args`. The preset's optional `program` field is presentation-only (UI
   label/badge), never sent to the backend — this keeps "what actually launches" unambiguous
   (always `argv`, never a value that could disagree with it).

## Alternatives Considered

| Alternative | Rejected because |
|---|---|
| Extend `AliasConfig` with an `argv []string` field, reuse `config.json` + `UpsertAlias`/`DeleteAlias` + `@name` detector | Violates requirements.md's explicit "hand-edited only, no UI editing" scope; would force presets through the alias RPC/storage identity, contradicting the "shareable dotfiles artifact" social job identified in `research/ux.md` §5 (a preset file is meant to be handed to a coworker or committed to a shared dotfiles repo, not stored in a per-machine `config.json`) |
| Build a fully separate, self-contained launch path for presets (own tmux command assembly, own quoting) | Would either reinvent `shellQuote` (duplicate implementation of the same shell-quoting boundary) or reintroduce the `strings.Fields` corruption bug if it reused `cli_flags` — both outcomes the requirements exist to prevent |
| New `SESSION_TYPE_PRESET` proto enum value | Out of scope per requirements.md; a preset has no distinct session lifecycle — it resolves to the existing `directory`/`new_worktree` types exactly like `autonomous` mode and `profile` already do (`.claude/rules/session-creation-registry.md`'s `autonomous` precedent) |

## Consequences

- Three "pick a saved thing" UI affordances now exist (Profile dropdown, `@alias` shorthand +
  browse palette, Presets list). This is an accepted, explicitly-flagged UX cost — see
  `research/features.md` §4 and the plan's Risk Control section — not an oversight.
- `AliasConfig`/`ProfileDefaults` are unchanged; existing `config.json` files remain fully
  backward compatible (no schema migration).
- If a future request asks for "presets, but UI-editable," the natural next step is teaching
  `AliasConfig` its own `Argv []string` field reusing the same `extra_args` carrier — this ADR
  does not preclude that; it only rejects doing so *instead of* the file-based v1.
