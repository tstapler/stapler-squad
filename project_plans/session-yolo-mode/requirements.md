# Requirements: Surface Yolo/Auto-Approve Mode as a Per-Session Setting

**Status**: Draft | **Phase**: 1 — Ideation complete (non-interactive, derived from backlog item)
**Created**: 2026-08-06
**Source**: Backlog item `83a9d351-28a5-491e-8966-e22e6a35e509`, migrated from
`TylerStaplerAtFanatics/stapler-squad#180`

## Problem Statement

Claude Code, Aider, and other agents stapler-squad launches support a flag that bypasses
tool-approval prompts (`--dangerously-skip-permissions` for Claude Code, `--yes`/similar for
Aider). Today, a user who wants this has to manually type the flag into a custom command
string when creating a session. stapler-squad has no first-class concept of "this session runs
unguarded" — it isn't visible on the session card, isn't toggleable after creation, and isn't
tracked in session state at all.

## Success Criteria

- A user can opt a session into auto-approve/yolo mode at creation time via a UI toggle,
  without hand-typing CLI flags.
- The correct flag is appended to the launch command based on the detected agent binary
  (Claude Code vs Aider vs others), not hardcoded to one agent.
- Sessions running in auto-approve mode are visually distinguishable from normal sessions
  (badge on the session card) so a user scanning the session list can immediately tell which
  sessions are unguarded.
- The setting is persisted (survives reload) and readable back via the session API.

## Scope

### Must Have (MoSCoW)
- `auto_approve` boolean field on session creation (proto request + persisted session state).
- Omnibar creation panel toggle to set it at creation time.
- Session card badge (e.g. "⚡ Auto") shown when active.
- Command injection: the correct per-agent flag is appended when launching the tmux command,
  based on the already-detected agent binary/program for that session.

### Should Have
- Toggle after creation (an explicit session action) that changes the setting and takes
  effect on next launch/restart of that session's agent process — not a live in-place flag
  swap on an already-running process, since most agent CLIs don't support changing this mid-run.

### Out of Scope (this pass)
- This is a new session *attribute*, not a new session *creation mode* — it does NOT require
  the full 7-touchpoint session-creation-mode registry (`.claude/rules/session-creation-registry.md`).
  No new `SessionType` enum value, no new proto enum, no new `SessionType.IsValid()` case.
- Per-agent flag mapping beyond Claude Code and Aider (extensible later; only these two are
  in the backlog item's proposed change).
- Any change to the actual permission-prompt/approval logic itself — this only controls
  whether the flag is passed, not how the underlying agent enforces (or doesn't enforce)
  approval.
- Retroactively changing already-running sessions' live process flags without a
  restart/relaunch.

## Constraints

- **Tech stack**: Go backend (ConnectRPC + ent ORM), React/TypeScript frontend
  (vanilla-extract for any new CSS per `.claude/rules/css-architecture.md`).
- **Schema changes**: New proto field requires `make proto-gen`; new ent column requires
  `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
  per `.claude/rules/ent-schema-generation.md`.
- **Registry**: Any new frontend feature/marker needs `make registry-generate` per
  `.claude/rules/feature-registry.md`; needs at least one Playwright e2e test.
- **Safety**: The feature deliberately exposes an unsafe/unguarded mode — must be clearly
  labeled and opt-in only, never a silent default.

## Context

### Existing Work
- Pattern source: `maki` (`src/cmd/mod.rs:45`) exposes `--yolo` as a top-level CLI flag
  passed into `acp::run(model, yolo)`; `maki-acp/src/permissions.rs` gates prompts on it.
- stapler-squad's closest existing precedent for a boolean flag threaded end-to-end through
  session creation is `autonomous_mode` (reuses `SESSION_TYPE_DIRECTORY` rather than adding a
  new enum value) — see `.claude/rules/session-creation-registry.md`'s documented exception.
  This item should follow the same "flag on existing type" pattern, not the full new-mode
  pattern.
- `one_off` (`server/services/session_service.go` ~lines 510–615) is cited as the canonical
  "flag on existing type" reference implementation for how a new boolean field flows through
  proto → service → frontend.

### Stakeholders
- User (Self) — primary and only stakeholder; power-user convenience feature.

## Research Dimensions Needed

- [ ] Stack — exact current shape of `CreateSessionRequest`/session proto messages, ent
  schema, and the command-string builder that launches the tmux session, so the new field
  and flag injection point can be added with minimal new surface area.
- [ ] Features — how `autonomous_mode` and `one_off` flow end-to-end (proto → ent → service →
  OmnibarContext → OmnibarCreationPanel) as the template to replicate.
- [ ] Architecture — where per-agent flag mapping belongs (a small lookup table keyed by
  detected agent binary, not a new abstraction/interface layer — see
  `.claude/rules/interface-pollution-checklist.md`).
- [ ] Pitfalls — toggling after creation: what "restart with flag appended" safely means for
  an already-running tmux/agent process; whether `UpdateSession` already supports arbitrary
  field updates or needs extension.
