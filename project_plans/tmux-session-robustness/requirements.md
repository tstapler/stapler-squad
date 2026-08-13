# Requirements: TMux Session Robustness & API Controllability

Status: Draft | Phase: 1 - Ideation complete
Created: 2026-04-16

## Problem Statement

The current tmux session management has two compounding problems that make debugging and
automation hard:

1. **Silent exits** — when a session's program (e.g. claude) exits inside tmux, there is no
   exit detection or logging of the exit reason. The system continues to treat the session as
   `IsStarted=true` (zombie state) because the controller is still registered. The operator
   has no way to know why a session stopped from the logs or UI.

2. **Poor API controllability** — the server layer reaches directly into `TmuxSession` internals.
   There is no clean lifecycle interface for start/stop/restart/status, no event hooks, and no
   way to react to session lifecycle events (start, exit, restart) from outside the session package.

The combination causes compounding failures: a session exits → no one is notified → the review
queue keeps polling it as if running → the Logs tab shows nothing useful → operator has to dig
through global logs to find anything.

Observed in production: `staplersquad_stapler-squad-testing-and-refactoring` exits after being
recreated, review queue stays stuck showing it as idle (lastActivity frozen), session log is empty.

## Success Criteria

The refactor is done when:

1. **Exit logging** — any session that exits writes a log entry to its session-scoped log file
   (via `log.ForSession()`) with the exit reason (EOF, PTY closed, control mode `%exit`, etc.)
2. **Zombie detection** — within one review queue poll cycle (≤10s) of a session dying, the
   system detects the discrepancy (`IsStarted=true` but tmux session gone) and updates state
3. **Clean API** — the server layer can start, stop, restart, and subscribe to lifecycle events
   via a defined interface without reaching into `TmuxSession` fields directly
4. **Lifecycle hooks** — `onStart`, `onExit`, `onRestart` callbacks are available for the review
   queue and server layer to subscribe to

## Scope

### Must Have (MoSCoW)
- Session exit detection in `ResponseStream.streamLoop()` (PTY EOF path) — logs to `ForSession()`
- Session exit detection via control mode `%exit` notification — propagated to session lifecycle
- Zombie reconciliation: background check or review-queue integration that detects IsStarted=true
  but tmux session doesn't exist → transitions session to appropriate state + logs warning
- `SessionController` or equivalent interface that server layer uses for lifecycle operations
- `onStart`, `onExit`, `onRestart` lifecycle hook callbacks

### Out of Scope
- No session data migration — existing JSON sessions must load without changes
- No new external dependencies (no new packages beyond what already exists)
- No changes to the tmux multiplexer protocol or external session discovery
- No changes to the web UI or protobuf definitions

## Constraints

Tech stack: Go, existing `session/`, `log/`, `session/detection/`, `session/tmux/` packages only
Timeline: Incremental — each piece (exit logging, zombie detection, API) can ship independently
Dependencies: Builds on `log.ForSession()` API added in this session
Backward compatibility: All current tests in `session/` must pass; no JSON schema changes

## Context

### Existing Work
- `log.ForSession(sessionID)` API just added — exit events should use this
- `ResponseStream.streamLoop()` already detects PTY EOF; exit logging just added (this session)
- `control_mode.go` `processControlModeLine` already handles `%exit` notification — not yet
  propagated to session lifecycle
- `ReviewQueuePoller.getContent()` uses a stale content cache (based on `lastActivity`) that
  never refreshes when a session's controller is frozen — zombie sessions stay "cached" forever
- `InstanceStatusManager.GetStatus()` returns `IsControllerActive=true` without checking if
  the tmux session actually exists

### Stakeholders
- Solo practitioner (Tyler) — primary user debugging why sessions die
- The review queue system — needs accurate session state to avoid false "running" signals
- Future: any server-layer code that starts/stops sessions programmatically

### Open Questions (Scope-Defining)

The following strategic questions need research before committing to scope:

1. **Direct tmux protocol implementation** — Rather than using `tmux attach -CC` PTY, could we
   directly implement the tmux control mode protocol over the unix socket? This would give us
   direct lifecycle events (pane exit codes, window close notifications) without the PTY
   attach/detach complexity. Cost: significant implementation work.

2. **Alternative process manager** — Could we replace tmux with a different persistence layer
   (e.g. a supervisor daemon, a named pipe/socket approach, or direct PTY management with a
   separate process) that survives server restarts without the tmux session reattach dance?
   Key requirement: sessions must survive a `stapler-squad` process restart.

3. **Hybrid: tmux for persistence + direct socket for events** — Keep tmux for process isolation
   and persistence, but connect to the tmux server's control socket directly (not via attach)
   to get reliable lifecycle events.

The answer to these questions determines whether this is a "polish the existing tmux wrapper"
refactor or a "replace the transport layer" redesign.

## Research Dimensions Needed

[ ] Stack - evaluate patterns for lifecycle management in Go PTY/tmux wrappers; assess direct
    tmux protocol implementation vs. current `attach -CC` approach; survey alternative process
    persistence layers (supervisord, s6, custom daemon)
[ ] Features - survey how other terminal multiplexers / agent managers handle session exit/restart
    and survive process restarts (Zellij, screen, tmuxinator, etc.)
[ ] Architecture - design patterns for event hooks, zombie detection, clean lifecycle API;
    evaluate cost/benefit of direct tmux socket protocol vs. current approach
[ ] Pitfalls - known failure modes: race conditions in exit detection, double-close, goroutine
    leaks; tmux protocol compatibility risks; persistence-layer restart scenarios
