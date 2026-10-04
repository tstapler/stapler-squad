# Implementation Plan: session-worktree-reconciliation

**Feature**: Periodic sweep that detects and repairs (or flags) sessions whose `worktrees` ent row is missing/stale/unresolvable, plus a scoped fix for the one confirmed silent no-op in terminal-item worktree cleanup.
**Date**: 2026-09-23
**Status**: Ready for implementation
**ADRs**: ADR-001 (plain-function sweep, no new Set*-interface), ADR-002 (feature-flag-gated staged rollout, in-memory backoff)

---

## Step 0.5 — Alternatives Considered

**Epic A (reconciliation sweep):**
1. **Ticker-driven `session/`-package plain function** (chosen) — fits the 4 existing sweeps' established convention exactly (`session/orphan_sweep.go`, `session/import_reconcile.go`, `session/backlog_lifecycle_stale.go`, `server/services/session_retention_sweeper.go`); needs no new `Set*`-injected interface because repair only touches `EntRepository` + `session/git`, both already in-package (architecture.md §1). Weakness: one more standalone ticker loop, though pitfalls.md §5 confirms the shared skeleton isn't yet large enough to justify extraction.
2. **New `StuckReason` + route through `BacklogLifecycleListener.ReconcileStuck`'s registry** — reuses an existing dispatch mechanism familiar to maintainers. Rejected: `MarkStuck` requires a real `item_id` + single `expectedStatus` precondition (ux.md), but this defect is session-scoped and can occur on sessions with no linked backlog item at all — the registry's shape doesn't fit the problem.
3. **Ent schema write-time hooks** that validate on every Session/Worktree mutation — would catch the defect closer to the moment it's introduced. Rejected: adds overhead to every DB write for a rare race window, and does nothing for rows that are *already* broken today (PR #625's row was already missing by the time anyone looked) — a periodic sweep is required regardless, making the hook additive complexity with no scope reduction.

**Epic B (`cleanupItemWorktreesExcept` gap):**
1. **Guard clause: detect missing-but-expected row, log+notify, keep the existing `continue`** (chosen) — matches features.md §3's own recommendation exactly; minimal diff.
2. **Route detection through a helper shared with Epic A's predicate** (adopted as an implementation detail of choice 1, not a separate alternative) — avoids duplicating the "should this session have a worktree row" rule in two files (build-vs-buy.md's "minimize new-code surface for the dupl/jscpd gates").
3. **Rely on Epic A's sweep to eventually catch the same session on a later tick, change nothing here** — rejected: requirements.md explicitly asks for this confirmed gap to be closed, and the leaked on-disk directory (if any) currently has zero signal at the moment of archival — silently waiting for a future tick is exactly the "silent no-op" the requirements forbid.

---

## Domain Glossary
| Term | Definition | Notes |
|------|-----------|-------|
| `WorktreeConsistencyIssueKind` | Sum type naming what's wrong: `IssueMissingWorktreeRow`, `IssueRepoPathUnresolvable`, `IssueBaseCommitShaUnresolvable`. | New type, `session/worktree_consistency_sweep.go`. |
| `WorktreeConsistencyResolution` | Sum type: `ResolutionRepaired` (auto-fixed) or `ResolutionFlagged` (declined to guess, posted for a human). | Per architecture.md §3's repair/flag boundary table. |
| `WorktreeConsistencySeverity` | Sum type: `SeverityWarning` (detected/ambiguous) or `SeverityError` (repair attempted, derived value still doesn't resolve). | Mirrors ux.md's two-tier notification convention; no new proto `NotificationType` invented. |
| `WorktreeConsistencyFinding` | Struct: `SessionID`, `IssueKind`, `Resolution`, `Severity`, `Before`, `After`, `Detail` — the record that gets logged and posted to the Notifier. | Satisfies AC2's "recorded before/after value." |
| `SessionWorktreeCandidate` | Struct pairing one session's `InstanceData` (from `ListInstanceDataWithWorktree`) with its optional `*ent.Worktree` row — the unit examined once per sweep tick. | Built once per tick (pitfalls.md §1 "snapshot-once-per-cycle"). |
| `LiveWorktreeEntry` | Alias for `session/git.NativeWorktreeEntry` — the on-disk git-worktree-list truth used to verify or derive DB state. | No new struct; reused as-is. |
| `ExpectsWorktree(data)` | Predicate: does this session's `SessionType`/`IsWorktree`/`Branch` imply a worktree row should exist. | Shared by Epic A's candidate filter and Epic B's gap fix — single source of truth, avoids duplicated logic across two files. |
| `git.ListWorktrees(repoPath)` | New exported wrapper around the existing unexported `nativeListWorktrees`. | `session/git/native_worktree_list.go`. |
| `sweep(ctx, deps)` | Testable, ticker-free inner function performing one reconciliation pass. | Never called from a real ticker in tests (pitfalls.md §6). |
| `StartWorktreeConsistencySweeper(ctx, deps)` | Ticker-driven outer loop wrapping `sweep`. | Mirrors `SessionRetentionSweeper.Start`. |
| `worktreeConsistencySweepInterval` | Unexported `const time.Duration` — the ticker period. | Set to 15 minutes (between the 60s backlog reconcile and the hourly retention sweep — this is a best-effort correctness pass, not a hot path). |
| `creatingGracePeriod` | Unexported `const time.Duration` — exclusion window for `Status == Creating` sessions, measured from persisted `creation_progress_updated_at`. | Set to 5 minutes, mirroring `StaleCreationSweeper`'s persisted-timestamp approach (features.md §4.1). |
| `FeatureFlagWorktreeConsistencySweep` | `const string = "worktree_consistency_sweep"` — the `config.FeatureFlags` map key gating the sweep. | Live-settable via the existing `UpdateFeatureFlag` RPC; default `false` at ship (ADR-002). |
| `matchLiveWorktree(candidate, entries)` | Matches a candidate to zero, one, or many `LiveWorktreeEntry` values by branch ref / worktree path. | Ambiguity (0 or 2+ matches) forces `ResolutionFlagged`. |
| `RepairWorktreeRow(candidate, match)` | Repair action: creates/updates a `Worktree` ent row from a uniquely-matched `LiveWorktreeEntry`, **then always notifies** (`NOTIFICATION_TYPE_INFO`, "auto-repaired: \<field\> derived from live git worktree — before: \<X\>, after: \<Y\>") — repair is never silent. | Only reachable on a unique match and a non-isolated instance (Story 1.3.3). Resolves Adversarial-D1: research/architecture.md §3 and its EventStorming table's `SessionWorktreeRepaired → PostRepairCommentAndNotify` row require this. |
| `FlagSessionForOperator(candidate, finding)` | Declined-repair path: always posts a notification, and opportunistically `storage.MarkStuck` when the session has a live, non-terminal-status linked `BacklogItem`. Calls `Notifier.NotifySession` (not `Notify`) when no `BacklogItem` is linked, so a session UUID never lands in `metadata["item_id"]`. | Mirrors `notifyIfActiveWorkSessionStale`'s dual-write pattern (ux.md). Resolves Architecture-A1. |
| `Notifier.NotifySession(sessionID, title, message string, notificationType int32, urgent, important bool)` | New sibling method added to the existing `session.Notifier` interface (`session/backlog_lifecycle.go:39-41`). Implemented by `EventBusNotifier` (`server/services/backlog_notifier.go`) to publish via `events.NewNotificationEvent(sessionID, ...)` with `metadata: map[string]string{}` — no `item_id` key. | Resolves Architecture-A1: `Notify`'s single `itemID string` param is unconditionally written into both the event's `SessionID` and `metadata["item_id"]` (`backlog_notifier.go:18-36`) — every existing caller always has a real backlog-item ID, so that param can't safely double as a bare session ID for a session with no linked item. The web UI already has the fallback branch this needs: `NotificationItem.tsx`'s "View Session" link renders exactly when `metadata["item_id"]` is absent and `sessionId` is set. |
| `StuckReasonWorktreeInconsistent` | New `domain.StuckReason` value, added to `session/domain/backlog.go` and its `AllStuckReasons` slice. | Resolves Architecture-A2: `MarkStuck` requires a valid, exhaustively-enumerated `StuckReason`; the plan named no value for this feature's dual-write. |
| `cleanupItemWorktreesExcept` | Existing function (`server/services/backlog_service.go:1233`) gaining one new guard-clause branch in Epic B. | Not renamed, not extracted — scope is one branch. |

---

## Pattern Decisions
| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Sweep control flow | Transaction Script (`sweep(ctx)` per-tick procedure) | PoEAA | Domain Model (a rich `WorktreeConsistencyService` object) | Detection/repair is a short, linear per-candidate pipeline, same shape as the 4 existing sweeps; no persistent domain object needed once no new interface is required (architecture.md §1). |
| Issue classification | Sum type via typed consts (`WorktreeConsistencyIssueKind`) | Type-driven design | Loose `string`/`bool` flags | Makes an invalid/uncovered issue kind a compile-time impossibility; matches repo convention (`SessionType`, `NotificationType`). |
| Repair-vs-flag decision | Plain branching (switch on `IssueKind` + match count) | GoF (evaluated, rejected) | Strategy pattern, one repairer per `IssueKind` | Only 3 issue kinds, each with a small, non-reused rule (architecture.md §3's table); a Strategy interface adds indirection with no second implementation ever plugging in — YAGNI per pitfalls.md §5. |
| Git worktree listing | Gateway/Adapter (`git.ListWorktrees` wraps `nativeListWorktrees`) | PoEAA (Gateway) | New parser over `git worktree list --porcelain` | The risky text-parsing approach was already retired by `native_worktree_list.go` (build-vs-buy.md §2) — reuse, don't reinvent. |
| Sweeper lifecycle | `Start`/`sweep` ticker-loop template, hand-mirrored from `SessionRetentionSweeper` | Existing repo convention | New generic `PeriodicSweeper` abstraction | pitfalls.md §5: only the ~15-18 line skeleton is shared across 2 of 4 sweeps — not yet flagged by `dupl`; premature abstraction violates the repo's stated ratchet-not-zero-tolerance stance. |
| Notification | Observer (`Notifier.Notify` / `eventBus.Publish`) | GoF | New `StuckReason` + `MarkStuck`-centric flow | `MarkStuck` needs an `item_id` + single status precondition; this defect is session-scoped and can exist on sessions with no linked item (ux.md). |
| Ent anti-join query | Repository (new `EntRepository` method mirroring `FindReviewItemsWithoutGate`) | PoEAA | Raw SQL / new query builder | Established precedent at `session/storage_backlog.go:1002-1014`; ent's typed `session.Not(session.HasWorktree())` already expresses this. |
| Epic B fix shape | Guard clause replacing silent `continue` | Clean Code | Extract `cleanupItemWorktreesExcept` into a new service/type | Requirements scope Epic B to the one confirmed gap; extracting a working, otherwise-correct function would be an unrequested rebuild. |

---

## Tech Debt Disposition
| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `session/backlog_lifecycle.go` | 2298-line God object by size (confirmed via `wc -l`) | Isolate via seam — new sibling file `session/worktree_consistency_sweep.go` holds all sweep logic. **One small, intentional exception** (Architecture-A1): the `Notifier` interface itself lives at `session/backlog_lifecycle.go:39-41`, so adding the `NotifySession` sibling method (needed to stop a session-only flag from corrupting `metadata["item_id"]`) is a ~3-line addition to that file's interface declaration only — not a rewrite, and no existing method on it changes shape. | This reconciler is session-scoped, not `BacklogItem`/`StuckReason`-scoped, so it was never a `ReconcileStuck` registry candidate in the first place (architecture.md §4). A full refactor pass is out of scope — belongs to a separate `/sdd:fix-hotspot` effort. |
| `session/domain/backlog.go` | `StuckReason` is a closed, exhaustively-enumerated sum type (`AllStuckReasons`, count pinned by `session/domain/backlog_test.go`) | Second small, intentional touch (Architecture-A2): add `StuckReasonWorktreeInconsistent`, append it to `AllStuckReasons`, bump the exhaustive-count test's expected value 20 → 21 | A different file from `backlog_lifecycle.go` above — this doesn't contradict that row's "isolate via seam" disposition, but the plan states it explicitly so a reviewer doesn't have to infer a second touched file from silence. |
| `cleanupItemWorktreesExcept` silent no-op | Confirmed real gap: missing-row sessions silently `continue`, no log, no flag, no disk cleanup (features.md §3) | Scoped guard-clause fix only (Epic B) — no extraction, no on-disk cleanup attempt added | Requirements explicitly cap Epic B to this one confirmed delta; the rest of ask #2 (`pr_pending` exclusion, sync terminal cleanup, hourly retention sweep) is already correctly built (architecture.md §5). |
| Sweeper `Start`/ticker skeleton duplication | ~15-18 line pattern shared with 2 of 4 existing sweepers | Accept as-is, do not extract a shared helper | Not yet flagged by `make ready-complexity-gate`'s `dupl` gate (pitfalls.md §5); extracting now would be speculative, against the repo's stated YAGNI/ratchet convention. |

---

## Observability Plan
- **Logs**: one structured `slog` line per `WorktreeConsistencyFinding` — `Info` level for `ResolutionRepaired`, `Warn` for `ResolutionFlagged`/`SeverityError` — including `session_id`, `issue_kind`, `resolution`, `before`, `after`. Colocated with `logs/staplersquad.log` per the repo's standard `log.GetConfigDir()` resolution (mirrors `SessionRetentionSweeper`'s existing per-cycle summary log). One additional per-tick summary line: candidates scanned / issues found / repaired / flagged.
- **Metrics**: none new — the repo has no established metrics registry beyond notification history; rely on `get_notification_history` plus the structured logs above.
- **Alerts**: existing `NotificationPanel`/`NotificationsNavBadge` surfaces every `WARNING`/`ERROR` notification the sweep publishes — no new alerting channel needed (ux.md's "no new UI surface" conclusion).

## Risk Control
- **Feature flag**: `FeatureFlagWorktreeConsistencySweep` (`"worktree_consistency_sweep"`) gates both whether `StartWorktreeConsistencySweeper` performs any work and whether `sweep(ctx)` proceeds past its first check — re-evaluated every tick (stack.md §5), so it can be flipped off mid-run with no restart. **Default `false` at ship.**
- **Rollback**: flip the flag off via the existing `UpdateFeatureFlag` RPC (no redeploy, no `make install-service`). All repairs are additive ent writes with the pre-repair value captured on the `WorktreeConsistencyFinding` and logged, so a bad repair is diagnosable and hand-reversible from the log line alone.
- **Staged rollout, with a concrete flip-on decision rule (Product Triad Review; pre-mortem P1 #1)**: enable first against the maintainer's own primary (non-isolated) instance. **Exit criterion for this project is not "code merged" — it is the flag flipped to `true`.** Concretely: after a 14-day burn-in on the primary instance with the flag on, flip `FeatureFlagWorktreeConsistencySweep`'s default to `true` in `config/config.go` **unless** the burn-in surfaces a genuine operator-reported false positive (an auto-repair that was wrong, or a flag that misfired on a healthy session) or the sweep itself errors/panics — i.e. the rule is fail-safe-to-ship, not fail-safe-to-never-ship: any burn-in outcome (zero findings; only flagged/ambiguous findings; one or more clean repairs) that produces **no confirmed false positive and no sweep-level error** is sufficient to flip the default. This closes the gap a first draft of this rule left open (an exhaustive-outcomes review during Product Triad Review found that "flag-only, zero repairs" — a real, likely outcome, since repair only fires on an unambiguous git-worktree match — satisfied neither of that draft's two conditions, leaving the flag stuck at default-`false` indefinitely). **The 14 days means 14 days of the sweep actually ticking, not wall-clock time the flag happened to be set** (a second triad-review round correctly flagged that this repo's services restart routinely — `docs/explanation/service-restart-orphan-process.md` — and Task 1.2.4a's backoff state is explicitly in-memory/reset-on-restart, so "zero findings" could otherwise mean "barely ran" rather than "ran cleanly"): confirm via the per-tick summary log line (Observability Plan) that sweep ticks were logged on at least 12 of the 14 days before treating the window as complete. **Before flipping, actively review `get_notification_history` for the window — silence is not confirmation** (the rule requires a checked-and-clean burn-in, not merely an unwatched one). **Enforcement task (Task 1.3.4, added to Epic 1.3, not left as prose)**: file a dated backlog item titled "Flip `worktree_consistency_sweep` default to true — 14-day burn-in check," due 14 days out, in the same PR that ships this feature — see Task 1.3.4 below. Repair actions (not detection/flagging) are hard-restricted to non-isolated instances via `config.IsIsolatedInstance()` (Story 1.3.3), mirroring `OrphanedTmuxSweeper`'s existing guard against two instances racing over the same shared on-disk repo (pitfalls.md §4).

## Decided (previously Unresolved Questions — resolved during Phase 3 plan repair, no human owner available; see Adversarial-D4)
- **Cross-tick backoff duration (Story 1.2.4): 24 hours per `(SessionID, IssueKind)` pair, in-memory only (resets on process restart).** The sweep ticks every 15 minutes (`worktreeConsistencySweepInterval`); a 24h backoff caps a still-unresolved finding at one notification per day instead of up to 96 (once per tick), consistent with pitfalls.md's cited 14x-bounce-loop incident and this repo's general low tolerance for notification-panel spam. Chosen over "once per process lifetime" because a long-lived server process would otherwise never re-surface a finding an operator missed or dismissed weeks ago. Task 1.2.4a implements this directly (no further tuning knob deferred).
- **Durable-comment scope (Story 1.2.3): no new comment mechanism — decided out of scope.** For a session with a linked `BacklogItem`, `MarkStuck`'s own durable `BacklogStuckState` row is the record — no separate comment needed. For a session with no linked `BacklogItem`, `Notifier.NotifySession`'s toast is itself durable enough for v1: `EventBusNotifier.Notify`/`NotifySession` persist every event to `NotificationHistoryStore` (`server/notifications/store.go`), which is queryable via `get_notification_history` — not merely a transient UI toast. Building a new session-level, item-independent comment API (which architecture.md §3 flagged as having no existing precedent) is out of scope for this project.

## Dependency Visualization

```
server/dependencies.go (periodic-ticker block, :1351/:1792)
        │
        ▼
StartWorktreeConsistencySweeper(ctx, storage, notifier, cfgAccessor)   [session/worktree_consistency_sweep.go]
        │  time.NewTicker(worktreeConsistencySweepInterval) + immediate first run
        ▼
sweep(ctx, deps)  ── gated by FeatureFlagWorktreeConsistencySweep, re-checked every tick
        │
        ├─▶ listConsistencyCandidates(ctx, storage)
        │        └─▶ storage.ListInstanceDataWithWorktree()      (NEVER the LoadMinimal path — NOTE: real signature
        │             takes no ctx (session/storage.go:438), hardcodes context.Background() internally, so
        │             sweep(ctx)'s own cancellation cannot interrupt this specific query today)
        │        └─▶ ExpectsWorktree(data)                       (filters out legitimate non-worktree sessions)
        │        └─▶ excludes Status==Creating younger than creatingGracePeriod
        │
        ├─▶ per repo_path: git.ListWorktrees(repoPath)           [session/git/native_worktree_list.go]
        │
        ├─▶ classifyIssues(candidate, liveEntries)
        │        ├─▶ matchLiveWorktree(candidate, entries)  ──▶ IssueMissingWorktreeRow
        │        ├─▶ git.IsGitRepo / membership check        ──▶ IssueRepoPathUnresolvable  (skip if Status==Paused)
        │        └─▶ git.CommitInfo(repoPath, sha)            ──▶ IssueBaseCommitShaUnresolvable (always flag-only)
        │
        └─▶ resolveFinding(ctx, repo, notifier, finding)
                 ├─▶ [unique match + !IsIsolatedInstance] RepairWorktreeRow → EntRepository write (Worktree row)
                 │        └─▶ notifier.Notify/NotifySession(..., NOTIFICATION_TYPE_INFO, ...)   (ALWAYS — no silent repair, Adversarial-D1)
                 └─▶ [ambiguous / unresolvable / isolated instance] FlagSessionForOperator
                          ├─▶ [BacklogItem linked] notifier.Notify(itemID, ...)                  (EventBus → NotificationPanel; metadata carries real item_id)
                          ├─▶ [no BacklogItem linked] notifier.NotifySession(sessionID, ...)      (EventBus → NotificationPanel; metadata carries NO item_id, Architecture-A1)
                          └─▶ storage.MarkStuck(ctx, itemID, StuckReasonWorktreeInconsistent, ...)  [best-effort, only if a live non-terminal BacklogItem is linked]

──────────────────────────────────────────────────────────────────────────────

server/services/backlog_service.go: cleanupItemWorktreesExcept (Epic B)
        │
        ├─▶ storage.GetWorktreeDataBySessionUUID(ctx, sessionUUID)
        │        └─▶ wt.WorktreePath == "" ?
        │                 ├─▶ ExpectsWorktree(sessionData) == true  →  log.Warn + notifier.Notify (NEW, Story 2.1.1)
        │                 └─▶ ExpectsWorktree(sessionData) == false →  continue (unchanged, legitimate non-worktree session)
        └─▶ (existing on-disk cleanup for the found-path case, unchanged)
```

---

## Phase 1: Worktree Consistency Reconciliation Sweep (Epic A)

### Epic 1.1: Detection primitives (git + ent read paths)
**Goal**: Give the sweep a race-free way to read "what the DB says" and "what git actually has on disk," with zero live-`Instance` reads (satisfies requirements.md's AC5 / `.claude/rules/instance-lock-free-reads.md` by construction — the sweep never holds an `*Instance` pointer).

#### Story 1.1.1: Export a git worktree listing helper
**As a** reconciliation sweep, **I want** a public entry point onto the existing native worktree parser, **so that** I can read on-disk git truth without a second parsing implementation.
**Acceptance Criteria**:
- `git.ListWorktrees` returns the same entries `nativeListWorktrees` would for the same repo path.
  - *Given* a repo at `/home/tstapler/.stapler-squad/worktrees/sess-b8ccca59` with one registered linked worktree whose branch is `work/b8ccca59`, *When* `git.ListWorktrees("/home/tstapler/.stapler-squad/worktrees/sess-b8ccca59")` is called, *Then* it returns a `[]NativeWorktreeEntry` containing an entry with `BranchRef == "refs/heads/work/b8ccca59"`, identical to calling the unexported `nativeListWorktrees` directly.
**Files**: `session/git/native_worktree_list.go`, `session/git/native_worktree_list_test.go`

##### Task 1.1.1a: Add exported wrapper (~2 min)
- Add `func ListWorktrees(repoPath string) ([]NativeWorktreeEntry, error) { return nativeListWorktrees(repoPath) }` next to the existing unexported function.
- Files: `session/git/native_worktree_list.go`

##### Task 1.1.1b: Add a wrapper-parity test (~3 min)
- Add a test asserting `git.ListWorktrees(fixtureRepoPath)` returns entries equal to a direct `nativeListWorktrees(fixtureRepoPath)` call, reusing the existing test fixture already used by `native_worktree_list_test.go`.
- Files: `session/git/native_worktree_list_test.go`

#### Story 1.1.2: Candidate assembly with the `ExpectsWorktree` predicate
**As a** reconciliation sweep, **I want** a filtered, eager-loaded list of sessions that should have a worktree row, **so that** I never misflag a legitimately non-worktree session or a session that's still mid-creation.
**Acceptance Criteria**:
- Candidate listing uses the eager-loaded query, never the minimal loader.
  - *Given* a `Session` row with `SessionType == SessionTypeNewWorktree`, `Branch == "work/b8ccca59"`, and no `Worktree` edge set, *When* `listConsistencyCandidates(ctx, storage)` runs (internally calling `storage.ListInstanceDataWithWorktree()` — this method takes no `ctx` parameter, see Task 1.1.2b), *Then* the returned `[]SessionWorktreeCandidate` includes that session with `Worktree == nil` and `ExpectsWorktree(data) == true`.
- `Status == Creating` sessions inside the grace period are excluded.
  - *Given* a session with `Status == Creating` and `creation_progress_updated_at` set to 30 seconds ago, *When* `listConsistencyCandidates` runs with `creatingGracePeriod == 5m`, *Then* that session is excluded from the returned candidates.
- No raw `*Instance` field is ever read by this path, proven under `-race`, not just by inspection.
  - *Given* a concurrently-running `CreateSession` pipeline writing `i.Path` under `i.mu.Lock()` via `setGitHubResolutionLocked` (`session/instance_actor_setters.go`) for an unrelated healthy session, *When* `sweep(ctx)` runs at the same moment, *Then* it never dereferences that session's `*Instance` — its only reads are `storage.ListInstanceDataWithWorktree()` (ent) and `git.ListWorktrees` (on-disk), so `go test -race` (Task 1.1.2d) reports no race between the two.
**Files**: `session/worktree_consistency_sweep.go` (new), `session/worktree_consistency_sweep_test.go` (new)

##### Task 1.1.2a: Define types + `ExpectsWorktree` predicate (~4 min)
- Create `session/worktree_consistency_sweep.go`. Define `WorktreeConsistencyIssueKind`, `WorktreeConsistencyResolution`, `WorktreeConsistencySeverity` (typed string consts), `WorktreeConsistencyFinding`, `SessionWorktreeCandidate` structs, and `func ExpectsWorktree(data session.InstanceData) bool` checking `SessionType` (`SessionTypeNewWorktree`/`SessionTypeExistingWorktree`), the persisted `IsWorktree` flag, and non-empty `Branch`, per features.md §1/§4.5.
- Files: `session/worktree_consistency_sweep.go`

##### Task 1.1.2b: Implement `listConsistencyCandidates` (~5 min)
- Implement `func listConsistencyCandidates(ctx context.Context, storage *session.Storage) ([]SessionWorktreeCandidate, error)` calling `storage.ListInstanceDataWithWorktree()` — **no `ctx` argument**: the real method (`session/storage.go:438`) takes none and hardcodes `context.Background()` internally, so `sweep(ctx)`'s own cancellation cannot interrupt this specific call today (an accurate, pre-existing limitation, not something this plan fixes) — per stack.md §2 / pitfalls.md §1, never `ListInstanceData`. Filter to `ExpectsWorktree(data) == true`, and exclude `Status == Creating` sessions whose `creation_progress_updated_at` is younger than `creatingGracePeriod`.
- Define `const creatingGracePeriod = 5 * time.Minute`.
- Files: `session/worktree_consistency_sweep.go`

##### Task 1.1.2c: Unit tests for candidate listing (~5 min)
- Test: fresh `Creating` session excluded; stale `Creating` session (past grace period) included; `SessionTypeDirectory` session (no branch) excluded by `ExpectsWorktree`; `SessionTypeNewWorktree` with missing `Worktree` edge included with `Worktree == nil`.
- Files: `session/worktree_consistency_sweep_test.go`

##### Task 1.1.2d: Concurrent race-freedom proof under `-race` (~5 min)
- Add a test that starts a goroutine calling `setGitHubResolutionLocked` (or another actor-setter mutating `i.Path`/`i.Branch` under `i.mu.Lock()`, `session/instance_actor_setters.go`) in a tight loop against one live `*Instance`, while concurrently calling `sweep(ctx)` (or `listConsistencyCandidates` + `classifyIssues`) against a session set including that same instance's persisted row, for a fixed number of iterations. Run via `go test -race -run TestSweep_NoRaceWithConcurrentActorWrites ./session/...` and assert the run reports no race — this is the actual proof Story 1.1.2's third AC promises; the mere absence of a raw `i.Path` read in the sweep's source (by construction) is necessary but not sufficient, since a `-race` run is what requirements.md's AC5 actually requires (Adversarial-D3).
- Files: `session/worktree_consistency_sweep_test.go`

---

### Epic 1.2: Repair vs. flag decision logic
**Goal**: Turn a candidate + live git truth into a `WorktreeConsistencyFinding`, apply the auto-repair/flag boundary from architecture.md §3 exactly, and guarantee every finding produces a visible record (never a silent no-op).

#### Story 1.2.1: Match live git-worktree entries to a candidate
**As a** reconciliation sweep, **I want** to know whether zero, one, or many on-disk worktrees correspond to a candidate, **so that** I only auto-repair unambiguous cases.
**Acceptance Criteria**:
- Unique match by branch ref.
  - *Given* a `SessionWorktreeCandidate` with `Branch == "work/b8ccca59"` and `entries == []LiveWorktreeEntry{{BranchRef: "refs/heads/work/b8ccca59", WorktreePath: "/home/tstapler/.stapler-squad/worktrees/sess-b8ccca59"}}`, *When* `matchLiveWorktree(candidate, entries)` runs, *Then* it returns `(match, matchCount=1)` with `match.WorktreePath == "/home/tstapler/.stapler-squad/worktrees/sess-b8ccca59"`.
- Ambiguous match.
  - *Given* two entries both referencing `refs/heads/work/b8ccca59` (e.g. a stale prunable entry plus a live one), *When* `matchLiveWorktree` runs, *Then* it returns `(nil, matchCount=2)`.
**Files**: `session/worktree_consistency_sweep.go`, `session/worktree_consistency_sweep_test.go`

##### Task 1.2.1a: Implement `matchLiveWorktree` (~4 min)
- `func matchLiveWorktree(candidate SessionWorktreeCandidate, entries []git.NativeWorktreeEntry) (match *git.NativeWorktreeEntry, matchCount int)`, matching on `BranchRef` (normalized `refs/heads/<Branch>`) and falling back to `WorktreePath` equality, per architecture.md §3's "≥4 existing parsers already do this extraction."
- Files: `session/worktree_consistency_sweep.go`

##### Task 1.2.1b: Unit tests: zero/one/many matches (~4 min)
- Files: `session/worktree_consistency_sweep_test.go`

#### Story 1.2.2: Classify issues per candidate
**As a** reconciliation sweep, **I want** to detect the 3 specific inconsistency shapes without misflagging known-healthy states, **so that** paused sessions and mid-pipeline sessions are never treated as broken.
**Acceptance Criteria**:
- Missing row, unique live match → flagged for auto-repair.
  - *Given* a candidate with `Worktree == nil` and exactly one matching `LiveWorktreeEntry`, *When* `classifyIssues(candidate, entries)` runs, *Then* it returns one finding with `IssueKind == IssueMissingWorktreeRow` and `Resolution == ResolutionRepaired` (repair happens downstream in `resolveFinding`, Story 1.2.3).
- Missing row AND no live worktree to match — the harder, more historically accurate PR #625 shape — is still a finding, never a silent skip (pre-mortem P1 #2).
  - *Given* a candidate with `Worktree == nil`, `ExpectsWorktree(data) == true`, and **zero** matching `LiveWorktreeEntry` values (the on-disk worktree is also gone — e.g. the session's row was already missing *and* its worktree directory had already been cleaned up by the time anyone looked, per plan.md's own Epic A alternatives note on PR #625's actual state), *When* `classifyIssues(candidate, entries)` runs, *Then* it still returns one finding with `IssueKind == IssueMissingWorktreeRow` and `Resolution == ResolutionFlagged` (nothing to repair from — there is no live git-worktree entry to derive a row from) — **not** zero findings. Before this AC, the sweep would have detected nothing at all for this exact shape, silently violating requirements.md's "never a silent no-op" AC for precisely the incident that motivated this project.
- Paused sessions with a gone directory are healthy, not broken.
  - *Given* a candidate with `Worktree.WorktreePath == "/home/tstapler/.stapler-squad/worktrees/sess-old"` (directory no longer exists) and `Status == Paused`, *When* `classifyIssues` runs, *Then* it returns no finding (per `pauseLocked`'s documented directory removal, `session/instance.go:2144-2157`).
- The same directory-missing shape on an `Active` session is the real anomaly.
  - *Given* the same missing directory but `Status == Active`, *When* `classifyIssues` runs, *Then* it returns one finding with `IssueKind == IssueRepoPathUnresolvable`.
- Base commit SHA unresolvable is always a finding, regardless of ambiguity elsewhere — but only for a genuine resolution failure, not a transient error.
  - *Given* a candidate with `Worktree.BaseCommitSha == "0000000000000000000000000000000000dead"` which `git.CommitInfo` fails to resolve in the candidate's repo with a genuine "object not found" error, *When* `classifyIssues` runs, *Then* it returns one finding with `IssueKind == IssueBaseCommitShaUnresolvable`.
- A transient or ambiguous git/filesystem error produces zero findings this tick, not a false positive.
  - *Given* `git.CommitInfo` or the on-disk existence/`git.IsGitRepo` check returns an error that is **not** classifiable as "genuinely gone" (e.g. a permission error, lock contention, or `context.DeadlineExceeded` — anything other than `os.IsNotExist`-class "path/ref doesn't exist"), *When* `classifyIssues` runs, *Then* it produces **no** `WorktreeConsistencyFinding` for that candidate this tick, and logs the raw error at `Debug`/`Warn` so the next tick's `git.ListWorktrees`/`git.CommitInfo` call re-evaluates from scratch — per pitfalls.md §1's `SessionRetentionSweeper` lesson: "a transient stat/git error during concurrent activity is a race to retry next tick," not evidence of brokenness.
**Files**: `session/worktree_consistency_sweep.go`, `session/worktree_consistency_sweep_test.go`

##### Task 1.2.2a: Missing-row classification, including the zero-match case (~5 min)
- Implement the `IssueMissingWorktreeRow` branch of `classifyIssues`, using `matchLiveWorktree`. Per architecture.md §3's original repair/flag table (which this task must implement in full, not just the unique-match half): `matchCount == 1` → finding with `Resolution = ResolutionRepaired`; `matchCount == 0` → **still emit a finding**, with `Resolution = ResolutionFlagged` (there is nothing to repair from, but the inconsistency itself is real and must not be silently skipped — pre-mortem P1 #2: this is the exact shape PR #625's incident is most likely to have actually been in by the time a human looked, since the on-disk worktree can be cleaned up independently of the DB row); `matchCount >= 2` → finding with `Resolution = ResolutionFlagged` (ambiguous, per Story 1.2.1).
- Files: `session/worktree_consistency_sweep.go`

##### Task 1.2.2b: repo_path resolvability check, Paused-gated, transient-error-safe (~5 min)
- Implement the `IssueRepoPathUnresolvable` branch: check on-disk existence + `git.IsGitRepo`/`git.ListWorktrees` membership, explicitly skipped when `candidate.Status == Paused` (pitfalls.md §3). Discriminate the error: `os.IsNotExist(err)` (or an equivalent "path/ref genuinely absent" check on the wrapped error) → real finding; any other error (permission denied, lock contention, `context.DeadlineExceeded`, or any error type not confidently classified as "gone") → skip this candidate this tick, log at `Debug`/`Warn`, produce no finding (Adversarial-D2).
- Files: `session/worktree_consistency_sweep.go`

##### Task 1.2.2c: base_commit_sha resolvability check, transient-error-safe (~4 min)
- Implement the `IssueBaseCommitShaUnresolvable` branch via `git.CommitInfo(repoPath, sha)`. **Do not reuse Task 1.2.2b's `os.IsNotExist` check here** — `git.CommitInfo` (`session/git/ops.go:480-496`) surfaces go-git's own sentinel for an unresolvable SHA, `plumbing.ErrObjectNotFound` (`github.com/go-git/go-git/v5/plumbing`), which `os.IsNotExist` does not match (verified: it's a distinct sentinel error, not a filesystem error). Discriminate via `errors.Is(err, plumbing.ErrObjectNotFound)` → genuine "commit not found" → finding, always `ResolutionFlagged` (never auto-repaired, per architecture.md §3's table — this field is load-bearing for review-gate diff correctness); any other error (I/O error, lock contention, `context.DeadlineExceeded`, or an error not matching the sentinel) → skip, log at `Debug`/`Warn`, no finding this tick.
- Files: `session/worktree_consistency_sweep.go`

##### Task 1.2.2d: Unit tests for all classify branches (~5 min)
- Include the Paused-exclusion regression test from pitfalls.md §3 as its own named test case.
- Files: `session/worktree_consistency_sweep_test.go`

##### Task 1.2.2e: Unit test — transient error produces zero findings (~4 min)
- Two cases: (1) mock the repo-path existence check to return a non-`os.IsNotExist` error (e.g. `context.DeadlineExceeded`, or a plain `errors.New("permission denied")`); (2) mock `git.CommitInfo` to return an error that does **not** wrap `plumbing.ErrObjectNotFound` (e.g. the same `context.DeadlineExceeded`/permission-denied case — never construct a fake `plumbing.ErrObjectNotFound` here, since that IS the "genuinely gone" case Task 1.2.2c must still flag). Assert `classifyIssues` returns zero findings for that candidate in both cases, and that a log line was emitted at `Debug`/`Warn` (Adversarial-D2's required regression guard).
- Files: `session/worktree_consistency_sweep_test.go`

#### Story 1.2.3: Repair vs. flag execution + notification
**As an** operator, **I want** every detected inconsistency to either be safely auto-repaired with a recorded before/after, or surfaced as a visible flag, **so that** nothing is silently dropped.
**Acceptance Criteria**:
- Unambiguous missing-row repair, and repair always notifies.
  - *Given* a `WorktreeConsistencyFinding{IssueKind: IssueMissingWorktreeRow}` with a unique `matchLiveWorktree` result on a non-isolated instance, *When* `resolveFinding(ctx, repo, notifier, finding)` runs, *Then* it creates a new `Worktree` ent row from the matched entry, sets `finding.Before == nil`, `finding.After == <new row's RepoPath/WorktreePath/BranchName>`, `finding.Resolution == ResolutionRepaired`, logs at `Info`, **and also calls the notifier** (`Notify` when a `BacklogItem` is linked, else `NotifySession`) with `NOTIFICATION_TYPE_INFO` and a message of the shape "auto-repaired: \<field\> derived from live git worktree — before: \<X\>, after: \<Y\>" — repair is never silent (Adversarial-D1; research/architecture.md §3's EventStorming table marks `SessionWorktreeRepaired → PostRepairCommentAndNotify` mandatory, citing `feedback_document_ai_decisions_in_edge_cases.md`'s "no silent repair" instinct).
- Ambiguous case is flagged, never guessed, and the message body includes all 3 of ux.md's required fields — not just the first 2 (UX Triad Review gap: the original AC only tested "what's broken" and "what was tried," never "what to do," leaving it possible to ship a message the operator can't act on).
  - *Given* a finding with `matchCount == 2` (or `0`), *When* `resolveFinding` runs, *Then* it sets `Resolution == ResolutionFlagged`, `Severity == SeverityWarning`, and calls the notifier (see next AC for which method) with `NOTIFICATION_TYPE_WARNING` and a body containing all three of: (1) **what's broken** — the session title/UUID and the field (`repo_path`/`worktree_path`/`base_commit_sha`); (2) **what the sweep tried** — "N candidate worktrees matched, could not pick one unambiguously" (or the zero-match equivalent); (3) **what to do** — a concrete next step, e.g. "inspect the session's git worktree state manually, or see `docs/how-to/debug-with-logs.md` for this sweep's log output" (ux.md's full 3-field message shape, not 2 of 3).
- Notification always fires on flag, and the metadata shape depends on whether a `BacklogItem` is linked (Architecture-A1).
  - *Given* the flagged session has a linked `BacklogItem`, *When* `resolveFinding` runs the flag branch, *Then* it calls `notifier.Notify(itemID, ...)` and the resulting event's `metadata["item_id"]` equals the real item ID.
  - *Given* the flagged session has **no** linked `BacklogItem`, *When* `resolveFinding` runs the flag branch, *Then* it calls `notifier.NotifySession(sessionID, ...)` (the new sibling method on `Notifier`, `session/backlog_lifecycle.go`) instead of `Notify`, and the resulting event's `metadata` map does **not** contain an `item_id` key — a session UUID must never be written into `metadata["item_id"]` (this was the concrete bug: `EventBusNotifier.Notify` unconditionally sets `metadata["item_id"] = itemID`, and `NotificationItem.tsx:311-321`/`NotificationsPage.tsx:131` both key off that field's presence to render a backlog-item deep link and categorize the notification).
- Best-effort dual-write to `MarkStuck` only when a live linked item exists, using the new `StuckReasonWorktreeInconsistent`.
  - *Given* the flagged session has a linked `BacklogItem` in a non-terminal status, *When* `resolveFinding` runs the flag branch, *Then* it also calls `storage.MarkStuck(ctx, itemID, domain.StuckReasonWorktreeInconsistent, expectedStatus, stuckContext)` best-effort (ignoring `applied == false`), matching `notifyIfActiveWorkSessionStale`'s dual-write pattern; *given* no linked item, *then* `MarkStuck` is not called at all (the notify call above still fires via `NotifySession`).
**Files**: `session/worktree_consistency_sweep.go`, `session/worktree_consistency_sweep_test.go`, `session/backlog_lifecycle.go` (add `NotifySession` to the `Notifier` interface), `session/backlog_lifecycle_test.go` (add `NotifySession` to the existing `fakeNotifier` test double at line 2426 so it keeps satisfying `Notifier`), `server/services/backlog_notifier.go` (implement `NotifySession` on `EventBusNotifier`), `server/services/backlog_notifier_test.go` (new — no existing test file covers `backlog_notifier.go` today), `session/domain/backlog.go` (add `StuckReasonWorktreeInconsistent`), `session/domain/backlog_test.go` (bump exhaustive count 20 → 21)

##### Task 1.2.3a-prep: Add `NotifySession` to the `Notifier` interface (~4 min)
- Add `NotifySession(sessionID, title, message string, notificationType int32, urgent, important bool)` as a sibling method on `session.Notifier` (`session/backlog_lifecycle.go:39-41`), with a doc comment explaining it's for sessions with no linked `BacklogItem` — the existing `Notify` always writes its first argument into `metadata["item_id"]`, which is wrong for a bare session ID (Architecture-A1). Implement it on `EventBusNotifier` (`server/services/backlog_notifier.go`) as `Bus.Publish(events.NewNotificationEvent(sessionID, "", uuid.New().String(), notificationType, derivePriority(urgent, important), title, message, map[string]string{}))` — empty metadata, no `item_id` key. Add `NotifySession` to the `fakeNotifier` test double (`session/backlog_lifecycle_test.go:2426`) so existing tests keep compiling.
- Files: `session/backlog_lifecycle.go`, `server/services/backlog_notifier.go`, `session/backlog_lifecycle_test.go`

##### Task 1.2.3b-prep: Define `StuckReasonWorktreeInconsistent` (~3 min)
- Add `StuckReasonWorktreeInconsistent domain.StuckReason = "worktree_inconsistent"` to `session/domain/backlog.go` alongside the other `StuckReason*` consts, append it to `AllStuckReasons`, and update `TestAllStuckReasons_should_contain20Entries_When_Enumerated`'s expected count from 20 to 21 (Architecture-A2).
- Files: `session/domain/backlog.go`, `session/domain/backlog_test.go`

##### Task 1.2.3a: Implement repair-write path + repair notify (~5 min)
- `func resolveFinding(ctx context.Context, repo *session.EntRepository, notifier Notifier, finding *WorktreeConsistencyFinding) error` — repair branch creates/updates the `Worktree` row, mirroring `EntRepository.Update`'s existing write shape (`ent_repository.go:636-666`); populates `Before`/`After`; then calls `notifier.Notify`/`NotifySession` (per whether a `BacklogItem` is linked) with `NOTIFICATION_TYPE_INFO` describing the repair (Adversarial-D1 — repair is never silent).
- Files: `session/worktree_consistency_sweep.go`

##### Task 1.2.3b: Re-verify-before-write guard (~4 min)
- Immediately before the repair write, re-query the candidate's current `Worktree` row state and abort the repair (fall through to flag) if it no longer matches the state that justified repair — mirrors pitfalls.md §1's CAS-style guard.
- Files: `session/worktree_consistency_sweep.go`

##### Task 1.2.3c: Implement flag path + notify/MarkStuck (~5 min)
- Flag branch: build the 3-field message body (what's broken / what the sweep tried / what to do, ux.md); call `notifier.Notify(itemID, ...)` when a `BacklogItem` is linked, else `notifier.NotifySession(sessionID, ...)` — never call `Notify` with a session ID in the `itemID` slot; use `NOTIFICATION_TYPE_WARNING` (or `_ERROR` when `Severity == SeverityError`) and `derivePriority(urgent=true, important=true)`; opportunistically call `storage.MarkStuck(ctx, itemID, domain.StuckReasonWorktreeInconsistent, ...)` only when a live non-terminal `BacklogItem` is linked to the session.
- Files: `session/worktree_consistency_sweep.go`

##### Task 1.2.3d: Unit tests (~5 min)
- Cover: repair writes row with correct before/after AND fires a notify; flag calls notify with WARNING on ambiguity; flag calls notify with ERROR when a repaired repo_path's derived base_commit_sha still fails to resolve; MarkStuck called only when a live linked item exists, using `StuckReasonWorktreeInconsistent`.
- Files: `session/worktree_consistency_sweep_test.go`

##### Task 1.2.3e: Unit test — no-linked-item notification never carries `item_id` (~4 min)
- Assert that for a finding on a session with no linked `BacklogItem`, both the repair-notify path and the flag-notify path call `NotifySession` (not `Notify`), and the resulting `events.Event.NotificationMetadata` map has no `"item_id"` key set (Architecture-A1's required regression guard).
- Files: `session/worktree_consistency_sweep_test.go`

#### Story 1.2.4: Backoff-gate repeated notifications
**As an** operator, **I want** a still-unresolved finding to not re-notify on every 15-minute tick, **so that** a genuinely unfixable case doesn't spam the notification panel.
**Acceptance Criteria**:
- No duplicate notify within the backoff window.
  - *Given* `sweep(ctx)` already called `notifier.Notify` once for `(sessionID="sess-b8ccca59", IssueKind=IssueBaseCommitShaUnresolvable)` on the previous tick, and the same condition still holds, *When* the next `sweep(ctx)` call runs within the backoff window, *Then* `notifier.Notify` is not called again for that pair, but the finding is still logged at `Info`.
**Files**: `session/worktree_consistency_sweep.go`, `session/worktree_consistency_sweep_test.go`

##### Task 1.2.4a: In-memory backoff tracking (~4 min)
- Track `lastNotifiedAt map[(sessionID, issueKind)]time.Time` owned by the sweep's loop state (not persisted — a process restart re-notifies once, consistent with existing sweeper conventions). `const worktreeConsistencyBackoffWindow = 24 * time.Hour` (Decided, see "Decided" section above — bounds a still-unresolved finding to one notification per day against the 15-minute tick, consistent with pitfalls.md's 14x-bounce-loop lesson).
- Files: `session/worktree_consistency_sweep.go`

##### Task 1.2.4b: Unit test: two consecutive `sweep(ctx)` calls, one notify (~4 min)
- Files: `session/worktree_consistency_sweep_test.go`

---

### Epic 1.3: Sweeper wiring, config, and safety
**Goal**: Wire the sweep into server startup behind a live-settable, default-off feature flag, with the `Creating`-window and cross-instance safety guards from pitfalls.md applied.

#### Story 1.3.1: `Start`/`sweep` split with feature-flag gate
**As an** operator, **I want** the sweep to no-op entirely when its flag is off, **so that** I can ship it disabled and enable it deliberately.
**Acceptance Criteria**:
- Flag off → zero storage/git calls.
  - *Given* `config.FeatureFlags` has no `"worktree_consistency_sweep"` key (default `false`), *When* the sweeper's ticker fires and calls `sweep(ctx)`, *Then* `sweep` returns immediately without calling `storage.ListInstanceDataWithWorktree` or `git.ListWorktrees`.
**Files**: `session/worktree_consistency_sweep.go`, `session/worktree_consistency_sweep_test.go`

##### Task 1.3.1a: Implement `StartWorktreeConsistencySweeper` (~5 min)
- `time.NewTicker(worktreeConsistencySweepInterval)` + immediate first run + `select{ctx.Done / ticker.C: sweep(ctx)}`, mirroring `SessionRetentionSweeper.Start` (`server/services/session_retention_sweeper.go`). Define `const worktreeConsistencySweepInterval = 15 * time.Minute` and `const FeatureFlagWorktreeConsistencySweep = "worktree_consistency_sweep"`.
- Files: `session/worktree_consistency_sweep.go`

##### Task 1.3.1b: Gate `sweep(ctx)` on the flag, re-checked every call (~3 min)
- `if !config.GetFeatureFlagWithDefault(cfg(), FeatureFlagWorktreeConsistencySweep, false) { return }` at the top of `sweep`.
- Files: `session/worktree_consistency_sweep.go`

##### Task 1.3.1c: Unit tests: flag off no-ops, flag on proceeds (~4 min)
- Use a spy `*session.Storage`/counter to assert zero calls when the flag is off.
- Files: `session/worktree_consistency_sweep_test.go`

#### Story 1.3.2: Wire into `server/dependencies.go`
**As the** server, **I want** the sweeper launched alongside the other periodic tickers, **so that** it runs in every deployment without a separate opt-in wiring step.
**Acceptance Criteria**:
- Sweeper goroutine starts at server startup.
  - *Given* `server/dependencies.go`'s existing periodic-ticker block (60s backlog reconcile at `:1351`, 30-min pause reaper at `:1792`), *When* the server builds its dependencies, *Then* `go session.StartWorktreeConsistencySweeper(serverCtx, storage, notifierAdapter, cfgAccessor)` is also launched, reusing the same `EventBusNotifier` adapter already constructed there.
**Files**: `server/dependencies.go`

##### Task 1.3.2a: Add the `go session.StartWorktreeConsistencySweeper(...)` call (~3 min)
- Add next to the existing periodic-ticker block, reusing the already-constructed storage/notifier handles.
- Files: `server/dependencies.go`

##### Task 1.3.2b: Smoke-test dependency wiring (~5 min)
- Confirm the sweeper goroutine starts without panicking when dependencies are built via the narrow `ServerDependencies` test helper (not `server.BuildDependencies()`, which makes real network calls — `instinct_ci_hermetic_testing_gotchas.md`). Colocate with the existing coverage of the sibling 60s reconcile-ticker block (`server/dependencies.go:1350-1358`), e.g. `TestReconcileTicker_should_KeepRunningReconcileStuck_When_QuotaGateReconcilePanics`.
- Files: `server/dependencies_test.go` (confirmed via grep — this file already covers `server/dependencies.go`'s periodic-ticker wiring block; no new file needed).

#### Story 1.3.3: Restrict repair actions to non-isolated instances
**As an** operator running a manual dev instance against the same on-disk repo as the live deployed instance, **I want** repair writes restricted to the primary instance, **so that** two instances never race a `git worktree` write on the same shared repo.
**Acceptance Criteria**:
- Isolated instance downgrades repair to flag.
  - *Given* `config.IsIsolatedInstance()` returns `true` and a finding would otherwise be `ResolutionRepaired` on a unique match, *When* `resolveFinding` runs, *Then* it instead sets `Resolution == ResolutionFlagged` with `Detail == "repair skipped: running on isolated instance"`, mirroring `OrphanedTmuxSweeper`'s existing `config.IsIsolatedInstance()` guard (`orphan_tmux_sweeper.go:56-67`).
**Files**: `session/worktree_consistency_sweep.go`, `session/worktree_consistency_sweep_test.go`

##### Task 1.3.3a: Add the isolated-instance guard (~3 min)
- In `resolveFinding`'s repair branch, check `config.IsIsolatedInstance()` before writing; downgrade to flag with the detail message above.
- Files: `session/worktree_consistency_sweep.go`

##### Task 1.3.3b: Unit test: isolated instance + unique match → flagged (~3 min)
- Files: `session/worktree_consistency_sweep_test.go`

#### Story 1.3.4: File the burn-in flip-on tracking item at ship time
**As a** maintainer, **I want** the 14-day flip-on decision (Risk Control) tracked as an actual dated item, **so that** the commitment to eventually enable this feature by default doesn't silently rot as unactioned prose (Product Triad Review — this Risk Control section exists specifically because a first-draft version of this commitment had no enforcement mechanism).
**Acceptance Criteria**:
- A dated tracking item exists before this feature's PR merges.
  - *Given* this feature's implementation is complete and its PR is ready to open, *When* the PR is created, *Then* a backlog item titled "Flip `worktree_consistency_sweep` default to true — 14-day burn-in check" exists (via `mcp__stapler-squad__create_backlog_item` or the repo's standard tracking mechanism), due 14 days from the flag being enabled on the primary instance, referencing this plan's Risk Control section's exhaustive flip-on rule (any burn-in outcome with no confirmed false positive and no sweep-level error, confirmed via an active `get_notification_history` review, is sufficient to flip).
**Files**: None (process task — not a code change)

##### Task 1.3.4a: File the tracking item (~3 min)
- Not a code task: create the backlog item described above at the same time this feature's PR is opened, so the 14-day clock and its due date exist as a trackable artifact rather than a sentence in this plan.
- Files: None

---

### Epic 1.4: Regression tests for the PR #625 scenario
**Goal**: Prove the exact reported defect (worktree-backed session, no `worktrees` row) is now caught — in both the shape where the on-disk worktree still exists (Story 1.4.1, repairable) and the harder, more historically accurate shape where it's also gone by the time the sweep runs (Story 1.4.2, flag-only; pre-mortem P1 #2).

#### Story 1.4.1: Reproduce and detect the exact missing-row state
**As a** maintainer, **I want** a test that reproduces PR #625's exact defect shape, **so that** a future regression is caught automatically.
**Acceptance Criteria**:
- Detection + repair on the reproduced fixture.
  - *Given* a `Session` row persisted with `SessionType == SessionTypeNewWorktree`, `Branch == "work/b8ccca59"`, `Status == Active`, no corresponding `Worktree` ent row (reproducing the exact non-atomic window at `session_creation_pipeline.go:277-281` per features.md §1), and a real git worktree registered on disk at the session's expected path/branch, *When* `sweep(ctx)` runs against this fixture, *Then* it produces exactly one `WorktreeConsistencyFinding{IssueKind: IssueMissingWorktreeRow, Resolution: ResolutionRepaired}`, and a subsequent `storage.GetWorktreeDataBySessionUUID(ctx, sessionUUID)` call returns the newly-created row's `WorktreePath` matching the on-disk path.
**Files**: `session/worktree_consistency_sweep_test.go`

##### Task 1.4.1a: Build the reproduction fixture (~5 min)
- Create the Session row + a real git worktree on disk (via existing worktree-creation test helpers), deliberately skipping the `storage.UpdateInstance` call that would normally create the `Worktree` row — reproducing the exact gap named in features.md §1.
- Files: `session/worktree_consistency_sweep_test.go`

##### Task 1.4.1b: Assert detection + repair + row correctness (~4 min)
- Assert the single finding, its `Resolution`, and that the resulting DB row's fields match the on-disk git-worktree entry.
- Files: `session/worktree_consistency_sweep_test.go`

##### Task 1.4.1c: Name the test as a PR #625 regression test (~2 min)
- One-line doc comment on the test function citing "regression test for PR #625" (per requirements.md's AC #3) so a future reader knows why the fixture exists without re-deriving it.
- Files: `session/worktree_consistency_sweep_test.go`

#### Story 1.4.2: Reproduce and flag the harder "row AND worktree both gone" state
**As a** maintainer, **I want** a second regression fixture covering the case where the on-disk worktree is also gone (not just the DB row), **so that** the sweep is proven against the historically-accurate shape of the incident, not just the easier one (pre-mortem P1 #2: `Worktree == nil` with a still-present on-disk worktree is the easy case Story 1.4.1 covers; a session whose worktree directory was independently cleaned up by the time the sweep runs — leaving zero live `git.ListWorktrees` matches — is the harder case that Task 1.2.2a's original single-branch design would have silently skipped).
**Acceptance Criteria**:
- Detection (flag-only, no repair possible) on the reproduced double-missing fixture.
  - *Given* a `Session` row persisted with `SessionType == SessionTypeNewWorktree`, `Branch == "work/b8ccca59-gone"`, `Status == Active`, no corresponding `Worktree` ent row, and **no** git worktree registered on disk anywhere in the tracked repo for that branch (both the row and the on-disk worktree are gone — reproducing the harder, more historically accurate variant per plan.md's own Epic A alternatives note), *When* `sweep(ctx)` runs against this fixture, *Then* it produces exactly one `WorktreeConsistencyFinding{IssueKind: IssueMissingWorktreeRow, Resolution: ResolutionFlagged}` — not zero findings — and the notifier is called (per Story 1.2.3's flag path).
**Files**: `session/worktree_consistency_sweep_test.go`

##### Task 1.4.2a: Build the double-missing reproduction fixture (~4 min)
- Create the Session row with no `Worktree` ent row and, unlike Task 1.4.1a, no matching git worktree anywhere on disk for its branch (skip the worktree-creation test helper entirely, or create then remove it).
- Files: `session/worktree_consistency_sweep_test.go`

##### Task 1.4.2b: Assert flagged-not-silent detection (~3 min)
- Assert the single finding, `Resolution == ResolutionFlagged`, and that `notifier.Notify`/`NotifySession` was called — this is the concrete proof that Task 1.2.2a's zero-match branch (added per pre-mortem P1 #2) actually fires, not just that the code compiles.
- Files: `session/worktree_consistency_sweep_test.go`

---

## Phase 2: Terminal-Cleanup Gap Closure (Epic B)

### Epic 2.1: Detect and flag missing-row sessions during item archival
**Goal**: Close the one confirmed gap in `cleanupItemWorktreesExcept` (`server/services/backlog_service.go:1233-1265`), where a session with a missing `Worktree` row silently `continue`s with no log, no flag, and no on-disk directory cleanup (features.md §3). The rest of ask #2 — `pr_pending` status exclusion (by design, `backlog_lifecycle_archive.go:110-116`/`terminal_status.go:28-44`), synchronous terminal-transition cleanup, and the hourly retention sweep — is already correctly built; **confirmed via architecture.md §5 and features.md §3, no further changes are made to those paths.**

**This notification is not a one-shot, easily-ignored WARNING (UX Triad Review blocker; pre-mortem P2 #5, citing this repo's own documented history of non-durable notify-once failures, `project_backlog_stuck_review_investigation.md`): Epic A's sweep (Story 1.1.2) lists candidates via an unfiltered `ListInstanceDataWithWorktree()` scan with no status/archived exclusion, so the exact same missing-row session this archival-time notification fires for remains an Epic A candidate afterward, and — per this plan's own zero-live-match fix (Story 1.2.2, pre-mortem P1 #2) — will be re-flagged (`IssueMissingWorktreeRow`, `ResolutionFlagged`) on every subsequent 15-minute tick, subject only to the 24h backoff, until someone fixes it or the session is deleted. The archival-time notification here is the earliest possible signal, not the only one; Epic A's periodic sweep is the recurring backstop that keeps re-surfacing it if the first notification is missed — this connection is stated explicitly here rather than left as an unstated side effect.**

#### Story 2.1.1: Replace silent continue with detection + notify
**As an** operator, **I want** to be told when an archived item's session had a missing worktree row instead of it silently vanishing, **so that** I'm not left guessing why a worktree directory leaked.
**Acceptance Criteria**:
- Missing-but-expected row is logged and notified, not silently skipped.
  - *Given* a `done`-status `BacklogItem` whose linked session has `SessionType == SessionTypeNewWorktree`, `Branch == "work/b8ccca59"`, but no `Worktree` ent row — so `storage.GetWorktreeDataBySessionUUID` returns `GitWorktreeData{}, nil` — *When* `cleanupItemWorktreesExcept(ctx, sessions, exceptPath)` (real signature, `server/services/backlog_service.go:1233` — takes `sessions []session.ItemSessionSummary`, not an `item` param) processes that session (`is session.ItemSessionSummary`), *Then* it calls `notifier.Notify(is.BacklogItemID, ..., NOTIFICATION_TYPE_WARNING, ...)` — using the real linked item ID already available on `ItemSessionSummary`, **never** `is.SessionUUID` in the `itemID` slot (this file was one call-site away from reintroducing the exact same `metadata["item_id"]` corruption bug as Architecture-A1, since every `ItemSessionSummary` here does carry a real `BacklogItemID` — no `NotifySession` variant is needed in this call path) — describing the missing row and naming `is.SessionUUID` in the message body (not the metadata key), logs at `Warn`, and then continues to the next session (no on-disk cleanup attempted — there is no known path to remove without the row).
- Legitimately non-worktree sessions are unaffected.
  - *Given* the same `done`-status item but a session with `SessionType == SessionTypeDirectory` (no branch, `ExpectsWorktree(data) == false`), *When* `cleanupItemWorktreesExcept` processes it, *Then* it silently `continue`s exactly as before — no notify, no log, no behavior change for the healthy case.
**Files**: `server/services/backlog_service.go`, `server/services/backlog_service_triage_test.go` (confirmed via grep — this file already contains `cleanupItemWorktreesExcept`'s existing call-site tests, e.g. around line 2793/2829/2873/2915; add the new branch's tests alongside them, not in a new file)

##### Task 2.1.1a: Call `session.ExpectsWorktree` before the existing empty-path check (~3 min)
- In `cleanupItemWorktreesExcept`, when `wt.WorktreePath == ""`, branch on `session.ExpectsWorktree(sessionData)` before the existing `continue`.
- Files: `server/services/backlog_service.go`

##### Task 2.1.1b: Log + notify on the missing-but-expected branch (~4 min)
- On `ExpectsWorktree == true`, call the existing injected `Notifier.Notify(is.BacklogItemID, ...)` (same interface Epic A uses — real item ID from `ItemSessionSummary.BacklogItemID`, not the session UUID) with a WARNING plus `log.Warn(...)` naming the session UUID and item ID; then `continue` (control flow otherwise unchanged).
- Files: `server/services/backlog_service.go`

##### Task 2.1.1c: Unit tests for both branches (~5 min)
- Test 1: missing-but-expected session → exactly one notify + one warn log, archival flow completes without error. Test 2: `SessionTypeDirectory` session → no notify, no log, unchanged `continue` behavior (regression guard against changing the healthy path).
- Files: `server/services/backlog_service_triage_test.go`
