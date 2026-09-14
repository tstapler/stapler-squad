# Phase 2 Synthesis — superseded-rework-session-retirement

**Date**: 2026-09-14
**Inputs**: research/stack.md (agent 1), features.md (agent 2), architecture.md (agent 3),
pitfalls.md (agent 4). All four converged independently.

## The backlog item's diagnosis is wrong in its central claim

| Item claims | Reality (verified) |
|---|---|
| Spawn path is "purely additive"; "zero references to `ArchiveSession`/`SetArchivedAt`" | **Refuted.** `spawnSessionAfterGates` step 12c (`backlog_service_triage.go:1081-1090`) calls `archiveItemWorkSessions` (`backlog_service.go:1187`), which archives **and** `KillTmuxPaneOnly`s every prior round. Present since PR #191 (2026-07-20). |
| `LoadInstances()` (`storage.go:335`) unconditionally restores every `Active` instance | **Wrong function.** It has no status filter and defers `Start()`. `EntRepository.List` returns every row unfiltered. |
| Fix = archive-on-supersede and/or a new sweeper | Both already exist in effect. A second one is a fix without a root cause **and** a `dupl` gate failure. |

**Archival is not broken. It ran. The 14 stale rows all carry `archived_at`.**
The restore path ignores `archived_at` and resurrects them anyway.

## Actual root cause — a guard gap in two restore sites

1. **`session/instance_serialization.go:455-604`** — if/else chain setting `started`.
   The final `else` (`:576`, `:596-600`) catches `Creating`/`Active`/`Restoring`/
   `PermanentlyFailed`/`Failed` and has **no `ArchivedAt` check**.
   The only `ArchivedAt` guard (`:509`) is buried inside the `Status == Stopped` branch,
   added as a fork-pressure perf fix (`c08513d47`), not as a safety guard — load-bearing
   for safety **by accident**.

2. **`server/dependencies.go:881-892`** (Step 6b) — `IsHotRestoreRecoverable() &&
   TmuxSessionExists()` → `RecoverFromStopped()` + `Start(false)`. No `ArchivedAt` check.
   **This is the ratchet**: it flips an archived row `Stopped` → `Active`, after which it
   resurrects on every boot forever. Explains the `status=Active AND archived_at NOT NULL`
   contradiction (14 rows) that archival alone could never produce.

Restore trigger itself: `server/dependencies.go:854` `if !inst.Started()` → `inst.Start(false)`.

## Verified incident data

Live DB is **`~/.stapler-squad/workspaces/d685c4b1a423cca3/sessions.db`** (workspace mode).
`~/.stapler-squad/sessions.db` is stale since 09-04 — validating against it would be
meaningless.

- 14 rows `status=1 (Active)` **with `archived_at` set**, across exactly 3 items, all
  `work` role, all `-rN` rework rounds.
- Plus 7 rows `PermanentlyFailed + archived`, including 5 rounds of a **fourth** item
  (`fix-tmux-stale-session-resume-bypass` r4,r5,r7,r8,r9) — **already armed** to
  resurrect on the next restart.
- Restart: `2026-09-14T06:52:53 UTC`. 13 instances restored in a 27 ms burst.
- Restore log line (`server/dependencies.go:883`/`:888`):
  `"Reconcile: session is terminal in DB but tmux is alive — restoring"`.
- `gate-request-review-on-ac-completion` has **no current round at all** — fully wedged.
- Blast radius: 3 of 611 (item, role) groups (0.5%).

## Counterfactual against the real incident (the decisive comparison)

| Candidate | Result on the 13 restored |
|---|---|
| **Skip restore when `archived_at != nil`** | Skips **11/13** (every rework round); keeps **2/2** legitimate `Knowledge Maintenance`. **Zero false positives.** |
| Skip when a newer instance exists for (item, role) | 11 → 1, but `gate-request-...-r7` resurrects **forever**: it is the newest round *and* a zombie with no successor. Elects the zombie. |

The `archived_at` predicate is simpler **and** strictly more correct. `ArchivedAt` is
already on `InstanceSnapshot` (`instance_snapshot.go:139`) — lock-free read, no plumbing.

## Safety verdicts on primitives

| Primitive | Verdict |
|---|---|
| `ArchiveSessionByUUID` (`session_service.go:1072-1110`) | **Non-destructive.** No tmux, no worktree, no kill. Documented safe to call unconditionally from a sweep. |
| `SetArchivedAtIfNilAndStop` (`instance_actor_setters.go:281-297`) | **Non-destructive.** Structural proof: `session/state_machine.go:48-70` has no `After` hook on any `X → Stopped` edge. |
| `KillTmuxPaneOnly` | Kills the process. Pane only — safe for a superseded round. |
| `StopSessionByUUID` | **FATAL HERE** — runs `CleanupWorktree`; rounds share one worktree. |
| `SessionRetentionSweeper` | The one **irreversible** consequence of setting `ArchivedAt`: permanently deletes archived sessions past 14 days via `Destroy()` → worktree removal. |

## Hard constraints carried into Phase 3

1. **Never group instances by backlog item on the `Instance` side.** `Instance`/
   `InstanceData` have **no** backlog-item field. `GetItemSessionBySessionUUID` returns a
   zero value with a **nil error** when unlinked (BUG-045 shape). Grouping buckets every
   manual/workflow/MCP session — including the operator's own live session — under key
   `""` and archives all but one. If any grouping is used at all: `continue` on empty
   `BacklogItemID` **and** empty `SessionUUID`, with a test.
2. **Never parse the `-rN` suffix.** Nothing in the repo reads it back; only
   `buildRevisionTitle` writes it. Round 1 has no suffix, review sessions have no marker,
   `AttachSessionToItem` uses arbitrary titles, and `r10 < r9` lexicographically.
3. **Never widen `KillTmuxPaneOnly` to `StopSessionByUUID`** (shared worktree).
4. **Skip-restore must reuse `ArchivedAt`, not a bespoke flag.** Skipping `Start()`
   without `ArchivedAt` leaves the instance `Active`-but-`!Started()`: `Resume()` refuses
   anything not `Paused` (`instance.go:2001`) → visible in UI, looks live, can never be
   started. `ArchivedAt` is already honored by `fromInstanceData:497-511`, Step 6/6b, the
   review-queue poller and `ListSessions`, and `UnarchiveSession` reverses it.
5. **Liveness must go through `IsSessionLive`/`findConfirmedLiveInstance`** — a raw
   `FindLiveInstance(...) != nil` fails the `tools/lint/noliveinstanceraw` analyzer added
   by PR #804. Its shadow instance is **not** valid for worktree decisions.
6. **Do not re-query `ListItemSessions` at archive time.** Existing code is safe only
   because it archives the *pre-spawn snapshot* read at step 8. Preserve that.
7. **BUG-064 ordering:** `UpdateItemSessionEnded` must run **before** `KillTmuxPaneOnly`.
8. **Timestamp tiebreak is fail-dangerous:** `CreatedAt` is a non-pointer `time.Time`;
   NULL → zero time → sorts oldest → gets archived. Strictly-`>` semantics, never archive
   a zero `CreatedAt`.

## Open discrepancy for the planner to settle

Agent 1: `review_queue_poller.go:771` `shouldSkipSession` **already** excludes
`ArchivedAt != nil`. Agent 3: `review_queue_poller.go:548` logged
`"stopped session found alive, reviving to Active"` **1383 times** on 09-14, near-paired
with 1381 transitions back to `Stopped`.
→ Either those revivals are non-archived sessions (a separate steady-state flap, likely
out of scope) or `:548` bypasses `shouldSkipSession`. **Settle before scoping.**

## Gates that will bite

`dupl` (150 tokens, new-code-only vs `origin/main`) — four existing near-identical
`[]ItemSessionSummary` loops. `gocyclo` 25 / `gocognit` 40 / `funlen` 150 apply to
*modified* functions too, and `spawnSessionAfterGates` is already ~260 lines.
`revive` file-length-limit 1000 — prefer a new file. `make lint-custom`:
`noliveinstanceraw`, `silenttransition` (`UpdateItemSessionEnded` errors need
`//nolint:silenttransition` + reason), `entfullscan`, `tmuxsocketscope`. Plus
`actor-field-guard` and `-race`.

## Test constraints

`session.NewTestEntRepository(t)` (in-memory SQLite); `envtest.NewIsolatedStateDir(t)`
before `t.Parallel()`; `executor.Executor` doubles for liveness/pane-cwd (copy
`server/services/liveness_consolidation_test.go`); no sleeps; make the decision a **pure
function** of `(sessions, now, bootTime)` mirroring `shouldSkipWorkTombstoneForRestartGrace`.
**Prove the test red pre-fix.** Never use `make install-service` to try this by hand.
