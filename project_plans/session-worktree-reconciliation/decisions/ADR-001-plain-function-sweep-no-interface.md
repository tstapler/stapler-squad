# ADR-001: Session-Package Plain-Function Sweep, No New `Set*`-Injected Interface

**Status**: Accepted
**Date**: 2026-09-23

## Context

The repo has 4 existing periodic-sweep implementations. Two of them (`reconcileStaleWorkSessions` and its `StaleWorkRemediator`) push repair behind a `Set*`-injected interface implemented in `server/services/`, because their repair action needs the live Instance/tmux registry that only `server/services.BacklogService` owns — and `session/` cannot import `server/services/` (package-cycle constraint, documented on `WorktreeCleaner`, `session/backlog_lifecycle_archive.go:36-47`).

This feature's repair actions are: write/update a `Worktree` ent row derived from `git worktree list` output, and re-derive `repo_path`/`base_commit_sha` via `session/git`'s existing parsers. Both are already available inside `session/` (`EntRepository`, `session/git`'s `nativeListWorktrees`/`CommitInfo`) — no live Instance/tmux registry is required.

## Decision

Implement the sweep as a plain periodic function in a new file, `session/worktree_consistency_sweep.go`, taking `*session.Storage` and a `Notifier` as direct arguments — no new `Set*`-injected interface, no `server/services/` struct. This is closer in shape to `ReconcileOrphanedTmuxSessions` (self-contained `session/`-package function) than to `StaleWorkRemediator`.

## Consequences

- Simpler than 2 of its 4 precedents: no interface to define, no adapter to wire in `server/services/`.
- If a future requirement needs the sweep to touch the live Instance/tmux registry (not anticipated by current research), it will need the same `Set*`-interface treatment as `reconcileStaleWorkSessions` — this ADR's decision does not preclude that later.
- Wiring happens from `server/dependencies.go`'s existing periodic-ticker block (alongside the 60s backlog reconcile and 30-min pause reaper), not from `server/server.go`'s `server/services/` sweeper construction block — a different wiring point than `SessionRetentionSweeper`, but the correct one for a `session/`-package-only dependency set.
