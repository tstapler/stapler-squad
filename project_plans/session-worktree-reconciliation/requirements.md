# Requirements: session-worktree-reconciliation

**Date**: 2026-09-23
**Source**: Backlog item b8ccca59-a570-489b-b699-7e498623058b, "Backlog system leaves
orphaned/leaked sessions with broken worktree tracking instead of detecting and
self-cleaning"
**Type**: chore/reliability — background reconciliation + gap-filling on top of mostly
existing lifecycle infrastructure
**Complexity**: 2-3 — no new subsystem; extends the existing periodic-sweep pattern
already used four times in this codebase (see Prior Art below)

## Problem Statement

Discovered while shipping backlog item e2373931-802c-4ccd-b17c-fc5f36426d9c (PR #625):
the work session's `sessions` row had no matching `worktrees` row, so `repo_path`/branch
resolution silently broke for the review gate and `report_pr_created`. Nothing detected
or auto-repaired this; a human had to correct the DB by hand.

Two asks, of different sizes once checked against the current codebase:

1. **Detection/repair of broken worktree tracking** (genuinely missing today — see
   Research). Sessions whose `worktrees` row is missing, or whose `repo_path`,
   `worktree_path`, or `base_commit_sha` no longer resolve on disk/in git, should be
   found periodically and either repaired or flagged.
2. **Self-cleanup for sessions whose backlog item finished** (largely already built —
   see Research). Confirm the existing terminal-transition cleanup path
   (`docs/reference/backlog-completion-gate-and-cleanup.md`) actually covers this
   backlog item's ask, and scope only the delta, if any.

## Prior Art (read before planning — do not re-solve these)

The codebase already has four periodic/one-shot reconciliation passes with an
established shape (ticker loop → list candidates → best-effort fix/log, never a hard
failure of the caller):

- `session.ReconcileOrphanedTmuxSessions` (`session/orphan_sweep.go`) — tmux session
  with no DB record → kill, with a `minAge` grace period against a create/register
  race. Runs once at startup (`minAge=0`) and periodically via `OrphanedTmuxSweeper`.
- `session.ReconcileSuspendedProcesses` (`session/import_reconcile.go`) — startup-only,
  resumes a SIGSTOP'd process left behind by a crashed import-external-session commit
  if its target Instance never materialized.
- `reconcileStaleWorkSessions` (`session/backlog_lifecycle_stale.go`) — flags/remediates
  an `in_progress` item whose active work session has gone quiet
  (`maxWorkSessionStaleness = 2h`), via `StaleWorkRemediator`.
- `CleanupTerminalItem` / `cleanupTerminalItemSync` + `reconcileTerminalItemSessions`
  (60s sweep, per `docs/reference/backlog-completion-gate-and-cleanup.md`) + the hourly
  `SessionRetentionSweeper` (`server/services/session_retention_sweeper.go`) — this is
  ask #2. It already: runs `cleanupItemWorktrees` + `archiveItemWorkSessions`
  synchronously on every terminal transition (done/archived), with a 60s sweep as a
  safety net, and separately reclaims disk via `SessionRetentionSweeper` once a
  retention window + PR-terminal + clean-worktree + no-sibling-rework checks all pass.
  **Research must confirm** whether this already satisfies ask #2 end-to-end (tmux
  pane + worktree + DB row all reclaimed once an item is `done`/`pr_pending`) or leaves
  a specific gap (e.g. `pr_pending` isn't itself terminal enough to trigger it, or the
  60s/retention-window sweeps only run against the live deployed instance and never
  against sessions in workspace-mode/instance-isolated state dirs).

None of the four above check `sessions` ↔ `worktrees` row consistency. That is
confirmed-absent — this project's real net-new surface is ask #1.

## Data model (session/ent/schema)

- `Worktree` (`session/ent/schema/worktree.go`): `repo_path`, `worktree_path`,
  `session_name`, `branch_name`, `base_commit_sha`, all `NotEmpty()`. Back-edge to
  `Session` is `Required()` (a `Worktree` row cannot exist without owning a session).
- `Session` → `Worktree` edge (`session/ent/schema/session.go:181`) is **not**
  `Required()` — a session legitimately has no `Worktree` row when it's a non-worktree
  session (main-repo session, shell sibling, etc.), so "no worktree row" is not itself
  a defect signal. The defect signal from PR #625 is specifically: a session that *was
  created via the worktree path* (has a worktree-shaped `repo_path`/branch, or a
  `git_worktree_watcher`/worktree-creation record) but its `Worktree` row is missing or
  points at a `repo_path`/`worktree_path`/`base_commit_sha` that no longer resolves.
  Research needs to pin down the exact signal that distinguishes "no worktree by
  design" from "worktree row lost."

## Ask

1. A periodic reconciliation sweep, following the existing sweep pattern (ticker,
   best-effort, logged, never blocks the caller), that finds sessions whose worktree
   tracking is inconsistent:
   - Session was worktree-backed but has no matching `worktrees` row.
   - `repo_path` points at a directory that is not a git worktree (or doesn't exist).
   - `base_commit_sha` doesn't resolve in the tracked repo.
   Each finding is either auto-repaired (when the correct value can be derived
   unambiguously — e.g. from the live git worktree list) or flagged for operator
   attention (surfaced via the existing notification pipe, per
   `feedback_document_ai_decisions_in_edge_cases.md`: self-heal/auto-close actions post
   a visible comment + notify(), never act silently).
2. Confirm or close the gap in the self-cleanup path for finished work (ask #2 above) —
   scoped to whatever `sdd:2-research` finds is actually missing, not a rebuild of
   `CleanupTerminalItem`.

## Acceptance Criteria (draft — refined during planning)

- A reconciliation pass runs periodically (cadence TBD in planning) and detects, for
  every live session, whether its worktree tracking is internally consistent.
- Every detected inconsistency is either repaired automatically (with a recorded
  before/after value) or produces a visible, operator-facing flag — never a silent
  no-op.
- The exact PR #625 scenario (worktree-backed session, no `worktrees` row) is caught by
  a regression test that reproduces the missing-row state and asserts detection.
- Ask #2 (self-cleanup) is resolved by either (a) a written confirmation, with
  citations, that the existing terminal-transition cleanup already covers it, or (b) a
  scoped fix for the specific gap found.
- No change to this sweep can make a healthy, in-flight session's worktree resolution
  worse (i.e. this must not race the actor-setter/snapshot pattern documented in
  `.claude/rules/instance-lock-free-reads.md` — reads must go through `Snapshot()`).

## Related

- Backlog item 80ab6b37-d509-42fa-a758-95c672134075 — narrower "how does a session fix
  its OWN tracking" angle (self-correction from inside the session), distinct from this
  item's "how does the system detect and clean up across the board" angle. Do not
  duplicate; research should check whether that item shipped anything reusable.
- PR #625 (https://github.com/tstapler/stapler-squad/pull/625) — the report_duplicate
  review-gate fix whose shipping session hit this exact problem.
- `docs/reference/backlog-completion-gate-and-cleanup.md` — existing terminal-cleanup
  infra relevant to ask #2.
- `.claude/rules/instance-lock-free-reads.md` — lock-free read discipline any new code
  touching `Instance` fields must follow.
