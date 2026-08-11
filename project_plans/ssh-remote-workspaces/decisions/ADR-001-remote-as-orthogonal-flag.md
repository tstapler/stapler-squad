# ADR-001: Remote Target as an Orthogonal Field, Not a New `SessionType`

**Status**: Accepted
**Date**: 2026-08-06
**Project**: ssh-remote-workspaces

## Context

`SessionType` (`proto/session/v1/types.proto:354-366`, mirrored in `config/types.go:202-216`
and aliased in `session/instance.go:434-448`) currently has five values —
`SESSION_TYPE_DIRECTORY`, `SESSION_TYPE_NEW_WORKTREE`, `SESSION_TYPE_EXISTING_WORKTREE`,
`SESSION_TYPE_NEW_PROJECT`, `SESSION_TYPE_ONE_OFF` — each answering "what does the
worktree/directory setup look like." `resolveSessionType` (`server/services/session_service.go:1651`)
switches on this enum to pick the Go-side `session.SessionType` constant.

"Remote" answers a different question — *on which host does that setup run* — and every
existing `SessionType` value is meaningful on a remote host too: a user can create a new
worktree remotely, attach to an existing remote worktree, run a one-off remotely, etc. This
codebase already has a documented precedent for exactly this shape of decision: the
`autonomous` mode reuses `SESSION_TYPE_DIRECTORY` and layers a `bool autonomous_mode = 23`
field (`proto/session/v1/session.proto:557`) onto whichever session type was chosen, rather
than forking the enum — see `.claude/rules/session-creation-registry.md`'s "autonomous
exception."

## Decision

Add `RemoteTarget` as a new field on `CreateSessionRequest` (field 28, the next available
number after `alias_name = 27`), not a parallel set of `SESSION_TYPE_REMOTE_*` enum values.
`resolveSessionType` is untouched; a new `resolveRemoteTarget(msg) *session.RemoteTarget`
resolution step runs alongside it, and a new mode-specific block in `CreateSession` composes
with whichever `session.SessionType` was already resolved — mirroring the existing
`if req.Msg.AutonomousMode { ... }` / `if req.Msg.OneOff { ... }` blocks
(`server/services/session_service.go:1625,1361`). No new `session.SessionType` Go constant is
added (mirrors the autonomous exception: the lifecycle doesn't structurally differ, only
where it executes).

## Alternatives Considered

- **New `SESSION_TYPE_REMOTE_*` enum values (one per existing mode, or a single
  `SESSION_TYPE_REMOTE` requiring a separate sub-selector).** Rejected: doubles (or 5x's) the
  enum and the `resolveSessionType` switch for no semantic gain — every existing mode is
  still meaningful remotely, so this is a combinatorial product being modeled as a flat list.
  It would also violate the session-creation registry's own stated intent for the
  `autonomous` exception, which exists precisely to avoid this pattern when "the backend
  session type is shared but behavior is driven by additional request parameters."
- **A parallel `RemoteSessionType` enum consumed instead of `SessionType` when a remote is
  set.** Rejected: forces every consumer of `SessionType` (frontend radio group, Go handler
  switch, worktree lifecycle code) to branch on "which enum am I looking at," doubling the
  registry's touchpoint surface instead of composing with it.

## Consequences

- The 7-touchpoint session-creation registry (`.claude/rules/session-creation-registry.md`)
  gets a `RemoteTarget` field threaded through touchpoints 2 (proto request), 3 (Go handler
  mode-specific block), 5-7 (frontend `Omnibar.tsx`/`OmnibarCreationPanel.tsx`/
  `OmnibarContext.tsx`+`useSessionService.ts`), but does **not** touch touchpoints 1 (proto
  enum) or 4 (`SessionType` constants) — a smaller footprint than a new mode would require.
- Every existing and future `SessionType` value automatically gains remote capability for
  free once `RemoteTarget` resolution is wired in — no per-mode remote variant to maintain.
- `OmnibarCreationPanel.tsx`'s `SESSION_TYPES` radio group is unchanged; the remote selector
  is a new, separate composable control (see plan.md Epic 4.3), following the same pattern
  already used for the "Autonomous mode" checkbox.
