# ADR-003: Per-Session, Sticky-at-First-Attach Feature Flag Scoping

**Date**: 2026-08-20
**Status**: Accepted
**Project**: terminal-multi-connection-streaming

## Context

`STAPLER_SQUAD_USE_CONTROL_MODE` is read fresh, per call, inside `streamViaControlMode`/its
caller (`server/services/connectrpc_websocket.go:571,601`). If the new hub/transport flag
followed the same per-connection-re-read pattern, a flag flip mid-rollout could route
connection A to the legacy path and connection B (same tmux session) to the hub path
simultaneously — the exact two-owner race this project exists to eliminate, now reintroduced
between two *architecturally different* code paths instead of two copies of the same one
(`research/architecture.md` §6, failure mode 1; `research/pitfalls.md` §3a).

## Decision

`STAPLER_SQUAD_USE_STREAM_HUB` (default `false`) is resolved into a `StreamPath` exactly once
per tmux session, at the first connection's attach time, via `StreamOwnershipLock.Resolve`.
Every subsequent connection to the same session reuses that sticky resolution, regardless of
later flag changes, until the session's hub/legacy-owner fully tears down. `StreamOwnershipLock`
generalizes the existing `TmuxSession.controlModeStartMu`-class lock so that legacy
`StartControlMode` calls and new hub creation acquire the *same* primitive before either
proceeds — mutual exclusion by construction, not by convention. Concretely: `StreamOwnershipLock`
and its package-level `xsync.Map[string, *StreamOwnershipLock]` (keyed by tmux session name) live
in package `session/streamhub`, exposed as `streamhub.AcquireOwnershipLock(sessionName string)
*StreamOwnershipLock`; `session/instance_tmux.go`'s `Instance.StartControlMode` imports
`session/streamhub` and calls this function to acquire the same lock instance hub creation uses
— a one-way `session` → `session/streamhub` dependency (see ADR-001's Dependency Inversion note
for why this doesn't cycle). A per-session override
(Phase 3, Story 3.3.1) sits on top of the global default for realistic single-operator
canarying (flip one disposable session, not all traffic).

## Alternatives Considered

| Alternative | Rejected because |
|---|---|
| Re-read the env var per connection (today's pattern) | Directly reintroduces the two-owner race at the old/new path boundary during any flip window |
| Process-lifetime-fixed flag (changed only via restart) | Restarting to change the flag force-disconnects every open session at once — the exact blast radius this repo's own `CLAUDE.md`/`tmux-keep-server-on-restart.md` already flags as dangerous, for an unrelated reason, on this same live instance |
| Flag scoped to the flag-read call site only, no shared lock with legacy `StartControlMode` | Leaves a window where hub creation and legacy start can race independently even if each individually reads a sticky value, because the stickiness lives in two different caches that were never unified |

## Consequences

- Rollback ("flip the flag back") only ever affects **new** sessions' resolution — existing
  hubs/legacy owners finish their lifecycle under whichever path they started with. This is
  the mechanism that makes "flip the flag back, no code revert" a true statement rather than
  an aspiration.
- The rollback path must be exercised at least once against a real, disposable session
  (Phase 3, Story 3.3.2) before being relied on during an actual incident, since there is no
  staging environment to have exercised it earlier.
- The three pre-existing "how many connections" trackers (`activeControlModeStreams`,
  `TmuxSession.controlModeSubscribers`, `ExternalStreamer.consumers`) are not deleted; for
  `PathHubOwned` sessions, the lower two are demoted to single-entry-per-hub internal plumbing,
  and `activeControlModeStreams`/its WARN remain exclusively the legacy path's own safety net
  until the trial period (14 days, zero violations) in the plan's Risk Control section elapses.
