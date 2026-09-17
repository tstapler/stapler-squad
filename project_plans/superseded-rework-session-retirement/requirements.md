# Requirements: superseded-rework-session-retirement

**Date**: 2026-09-14
**Type**: Bug fix (systemic — a missing lifecycle transition, not a one-off defect)
**Source**: Backlog item `0e383248-f137-4945-9706-26a4c8a9b93f`, priority 2

## Problem Statement

When a backlog item goes through multiple rework rounds (r1, r2, r3, ...), each round
creates its own `Instance` record. Superseded rounds are not reliably retired, so the
server's startup cold-restore path resurrects every one of them at once, each spawning a
real `claude` process.

> **CORRECTION (Phase 2 research, agent 2).** The backlog item states that spawning is
> "purely additive" with "zero references to `ArchiveSession`/`SetArchivedAt`" in the
> spawn path. **This is refuted.** `spawnSessionAfterGates` already calls
> `archiveItemWorkSessions` (`server/services/backlog_service_triage.go:1089` →
> `backlog_service.go:1187-1202`), which archives and kills prior work/review sessions.
> It also tombstones dead work rows and prunes their worktrees (`:905`) and kills ended
> rounds' panes (`:914`).
>
> The actual defect is narrower: that archive step is **gated behind `isReopen`** and is
> **best-effort** (failures are logged, not retried), and several spawn entry points
> bypass `spawnSessionAfterGates` entirely. The fix is therefore to close the gaps around
> an existing, already-correct primitive — not to introduce retirement where none exists.
> Planning must target the gap, not rebuild the mechanism.

Observed 2026-09-14: a `make install-service` redeploy resurrected ~17 stale sessions
across 3 backlog items in seconds:

| Backlog item | Rounds resurrected | Genuinely current |
|---|---|---|
| `stapler-squad-add-durable-guidance-request` | r3, r4, r6, r7, r8, r9, r10 | r10 only |
| `stapler-squad-fix-fork-pressure-flap-and-status-banner` | r3, r4, r5, r6, r7, r9 | (latest only) |
| `stapler-squad-gate-request-review-on-ac-completion` | r5, r6, r7 | (latest only) |

Real cost: N concurrent Claude processes competing for the same API budget on every
restart; plausibly contributed to a weekly rate-limit exhaustion the same day. It recurs
on every restart until fixed.

**Distinct from** PRs #791 / #799 / #804, which fixed *single-session* crash-recovery
correctness (stale `HasSession()` pointer, `RestoreWithWorkDir` orphan guard, liveness
consolidation). Those make one session resume its own process correctly. This item is
about there being *more than one* session that each correctly resume, when only the
newest should exist at all.

## Users / Consumers

- The operator (Tyler) — bears the API-budget cost and the UI noise of 17 zombie sessions.
- The backlog automation pipeline — reads/acts on work sessions per item; extra live
  sessions for an item are ambiguity it is not designed for.
- The server startup path (`BuildRuntimeDeps` restore loop) — the proximate trigger.

## Success Metrics

1. After a server restart, at most **one** live work session per (backlog item, session
   role) is cold-restored — the current round — not every historical round.
2. Superseded rounds are retired at *spawn time* (proactive), not only when the pipeline
   happens to reprocess the item (today's reactive `tombstoneOrphanWorkSessions`).
3. A second line of defense exists so that a missed retirement (new spawn path added
   later, or pre-existing historical data) still does not cause a mass respawn.
4. **Regression proof**: a test that reconstructs the 2026-09-14 data shape (one backlog
   item, N `Active` rework-round instances) and asserts only the newest is restored.

## Constraints

- **Must not destroy live work.** This is a bug report *about* careless session handling
  destroying live work. The fix must never archive, skip-restore, or kill a session that
  is genuinely the current round, and must never delete a worktree or its uncommitted
  contents.
- **HARD SAFETY CONSTRAINT — shared worktree.** All `-rN` rounds for an item share **one
  worktree and one branch**. Retirement of a superseded round must therefore use
  `KillTmuxPaneOnly`, **never** `StopSessionByUUID`, which runs `CleanupWorktree` and
  would delete the worktree out from under the still-current round. Existing code is
  already careful here (`backlog_service_triage.go:3087-3093`,
  `backlog_service.go:1175-1177`) — follow that precedent exactly. Any plan that reaches
  for `StopSessionByUUID` on a superseded round is a BLOCKER.
- **Do not key retirement off the `-rN` title suffix.** Round number is not persisted;
  it is formatted by `buildRevisionTitle` (`backlog_service_triage.go:1580-1591`) as
  `"%s-r%d"` from a spawn-time recount. Parsing it back is unreliable: round 1 has no
  suffix, review sessions have no round marker, `AttachSessionToItem` creates work rows
  with arbitrary user titles, and naive string ordering puts `r10` before `r9`. Order by
  `created_at` via the `item_sessions` link table instead.
- **Must respect session roles.** `SessionRoleWork`, `SessionRoleReview`,
  `SessionRoleJulesWork` (and any others) are different roles for the *same* item.
  Retirement logic must be scoped per-role — retiring a work round must not touch the
  review session for the same item.
- **Must build on PR #804's `OtherLiveSessionInsideWorktree` guard**, which the live
  deployed service is already running. Do not reimplement or bypass it.
- **Reuse existing primitives** — `ArchiveSessionByUUID` / `SetArchivedAtIfNilAndStop`
  and the existing status-transition machinery — rather than inventing a parallel
  "superseded" concept, unless research shows the existing primitives cannot express it.
- Follow `.claude/rules/instance-lock-free-reads.md` for any `*Instance` field read.
- No rebase in this shared checkout; `git merge` / `git pull --no-rebase` only.
- Must pass `make build`, `session/...` + `server/services/...` tests, `make lint`
  (incl. `lint-custom`), and `make registry-diff` if any RPC is added.

## Scope

### In Scope
- Retiring the previous round's `Instance` when a new rework round is spawned, for every
  spawn entry point (`spawnSessionAfterGates`, `AutoReopenAfterFailedReview`,
  `AutoRespawnAutonomousWork`, `AutoReopenForPRFix`).
- A defensive filter or sweeper so startup cold-restore does not mass-resurrect
  superseded rounds even when retirement was missed (covers historical data written
  before this fix).
- Regression tests reproducing the observed incident shape.

### Out of Scope
- Redesigning the rework-round model itself (e.g. reusing one Instance across rounds).
- Worktree/disk cleanup for retired rounds (separate concern; the
  `worktree-disk-cleanup` skill and PR #804's deletion guards own that).
- Any change to review-session or Jules-session lifecycle beyond not breaking them.
- UI work beyond what falls out naturally from a status change.

## Resolved by research (agent 2)

- **Linkage**: the `item_sessions` link table / ent `ItemSession`
  (`session/ent/schema/item_session.go:19-101`). Session side is a loose string FK
  (`session_uuid` → `Instance.UUID`), not an edge. There is **no `backlog_item_id` on
  Instance**. Query via `ListItemSessions(ctx, itemID)`
  (`session/storage_backlog.go:294`) — one indexed query, all roles, ordered by
  `created_at` ascending. No full scan needed.
- **Roles** (`session/backlog.go:48-54`, untyped string consts): `work`, `triage`,
  `review`, `jules_work`. `IsTmuxBackedSessionRole` (`:76`) = work + review only.
  `work` and `review` legitimately coexist for one item — the reopen flow deliberately
  keeps the work session alive, so role conflation would destroy live work.
- **No existing `currentSessionFor(item, role)` helper.** Nearest reusable shape is
  `latestTriageSession` (`session/backlog_lifecycle_triage.go:73`) — "max CreatedAt for
  role" — which should be generalized rather than re-invented. The `findActive*` family
  answers a different question (still-open, not latest).
- The second `BacklogItem ↔ Session` many-to-many ent edge (`backlog_item.go:197` /
  `session.go:192`) is **vestigial** — zero non-generated, non-test uses. Do not build
  on it.

## Open Questions (still open)

1. Where exactly does the "restore every `Active` instance" decision live —
   `LoadInstances()`, `s.repo.List()`, or the async Step 6 loop in `BuildRuntimeDeps`?
   (Agent 1 pending.) The item names `storage.go:335`, but that function has no status
   filter, so the predicate is downstream.
2. Are `ArchiveSessionByUUID` / `SetArchivedAtIfNilAndStop` destructive (kill process /
   delete worktree) or state-only? (Agent 1 pending — safety-critical.)
3. How many (item, role) groups are currently multi-live in the real dataset, and does
   this need a one-time backfill for pre-existing rows? (Agent 3 pending.)
4. Which of the missed spawn entry points must be fixed now vs. deferred?
   Known bypasses of `spawnSessionAfterGates`: `AttachSessionToItem`
   (`backlog_service_sync.go:103` — arbitrary user title, no `-rN`, no archive; the
   biggest blind spot), review-gate spawn (`session/review_gate.go:444`),
   `TriggerReReview` (`:2989`, `:2860`, `:2896`), triage
   (`backlog_service_trigger_triage.go:321`), manual verdict
   (`backlog_service_lifecycle.go:1212`), Jules reservation
   (`jules_dispatch_service.go:324`). Also non-spawn mutators
   `RemediateStaleWorkSession` (`:2010`) and `forceResetItem` (`:1122`).
   → This is the main argument for the defensive startup filter carrying the load,
   rather than trying to patch every spawn site.
