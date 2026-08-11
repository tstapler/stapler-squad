# ADR-001: `auto_approve` Is an Independent Field, Not a Rename/Absorption of `auto_yes`

**Status**: Accepted
**Date**: 2026-08-06
**Context**: `session-yolo-mode` feature

## Context

`requirements.md` asks for a first-class `auto_approve` boolean, surfaced as an Omnibar toggle and session-card badge, that injects the correct per-agent CLI flag (Claude's `--dangerously-skip-permissions`, Aider's `--yes-always`) into the session's launch command.

Phase 2 research found that a field doing *part* of this already exists: `auto_yes` (`proto/session/v1/session.proto:497`, `session/ent/schema/session.go:46-47`, `session/instance.go:129`). Today, when `AutoYes` is true, Claude sessions get `--permission-mode bypassPermissions` injected (`session/instance_tmux.go:154-156`) — functionally equivalent to `--dangerously-skip-permissions` per Anthropic's docs — and a separate keystroke-level fallback (`TapEnter()`, `session/instance_tmux.go:346-354`) fires literal Enter keypresses when a prompt is detected in the tmux pane. Non-Claude programs (including Aider) get neither: `classifyProgram` only distinguishes `claudeProgram`/`plainProgram`, so `AutoYes` is a silent no-op for Aider today.

Research proposed three options:
- **(a)** Rename/promote `auto_yes` into the new `auto_approve` concept, extending its flag injection to Aider, and keep `auto_yes`'s existing TapEnter/daemon-auto-respond role under its old name.
- **(b)** Add `auto_approve` as a fully parallel field, coexisting with `auto_yes`, both driving the same `--permission-mode` mechanism.
- **(c)** Keep `auto_yes` completely untouched and add `auto_approve` as a narrow, independent, purely flag-injection-scoped field.

Research leaned toward (a)/(c), explicitly flagging this as a judgment call for the plan to resolve rather than leave implicit.

## Decision

**Option (c): `auto_approve` is added as a new, independent field. `auto_yes` is not touched, renamed, or absorbed.**

The deciding factor, found during planning (not surfaced in the original research): `auto_yes` is not only a create-time checkbox. It is threaded through the session-defaults **preset system** — `web-app/src/components/settings/ProfilesManager.tsx`, `AliasesManager.tsx`, `DirectoryRulesManager.tsx`, `web-app/src/lib/hooks/useAliases.ts`, `useSessionDefaults.ts`, `web-app/src/lib/validation/sessionSchema.ts`, plus `SessionWizard.tsx` and `SessionDetailView.tsx` — 12+ frontend files in total. Renaming or absorbing it (option (a)) would require touching every one of those call sites to keep the preset system consistent with the new name, which is a substantially larger and riskier diff than "surface a per-session opt-in toggle," the actual scope of requirements.md.

Option (c) achieves every literal requirement:
- A field literally named `auto_approve`, end to end (proto → ent → Go → TS).
- Aider support via a new `yoloFlagByAgent` map, independent of `classifyProgram`'s existing Claude-only special-casing.
- A session-card badge and a post-creation toggle, both keyed off the new field alone.
- The exact flag literal requirements.md and the `maki` precedent name (`--dangerously-skip-permissions`), rather than `auto_yes`'s existing `--permission-mode bypassPermissions` substitute.

Option (c) is implemented so that command injection for the new field is appended in `buildLaunchCommand`, *after* the existing `classifyProgram` switch, rather than inside `buildClaudeCommand`'s existing `PermissionMode`/`AutoYes` block (`instance_tmux.go:151-156`). This means the new code path never touches, and cannot regress, any existing `AutoYes`/`PermissionMode` behavior.

## Alternatives Considered

**(a) Rename/absorb `auto_yes` into `auto_approve`.**
Rejected because of the 12+-file preset-system blast radius described above. A rename that only updates the RPC-facing name while leaving `auto_yes` untouched everywhere else would produce exactly the "two similarly-named things with an unclear relationship" outcome research warned against — worse, in fact, since one of them would now be misleadingly named relative to its own preset UI.

**(b) Fully parallel, same-mechanism field.**
Rejected because it directly compounds an existing latent bug: `session/instance_tmux.go:151-156` can already emit `--permission-mode` twice on the same command line if both `PermissionMode` and `AutoYes` are set (both non-empty/true independently). Routing a third field through that same `if`/`if` block would make a three-way version of that bug trivially reachable.

## Consequences

- Two conceptually-related booleans now exist on `Instance`: `AutoYes` (pre-existing, drives `--permission-mode bypassPermissions` + `TapEnter` + daemon auto-respond; owned by the preset system) and `AutoApprove` (new, drives a per-agent CLI flag via `yoloFlagFor`; owned by the Omnibar toggle, session-card badge, and post-creation action). Both fields carry cross-referencing doc comments (`session/instance.go`, `session/ent/schema/session.go`) so a future reader isn't left to rediscover the distinction from scratch.
- **Known, accepted edge case**: a session created from a Profile/Alias/DirectoryRules preset with `auto_yes=true`, if the user *also* enables the new `auto_approve` toggle, gets both `--permission-mode bypassPermissions` and `--dangerously-skip-permissions` on the same command line. Both point the same direction (bypass), so this is redundant but not a correctness hazard. Not engineered around — proportionate given this is a single-user tool (see `requirements.md`'s Stakeholders section).
- The preset system (Profiles/Aliases/DirectoryRules) does not gain `auto_approve` support in this pass. If a future need arises to let a saved preset also pre-set `auto_approve`, that is new, separately-scoped work — not a natural extension of this ADR's decision, since it would need its own UI/UX pass through those 12+ files.
- Backlog automation's headless review sessions (`session/backlog_review.go:418-419`) set `PermissionMode: PermissionModeBypassPermissions` directly, bypassing both `auto_yes` and `auto_approve`. Neither this decision nor the plan changes that — those sessions will not show the new badge despite functionally running unguarded. Named explicitly so it isn't silently rediscovered as a bug later.
