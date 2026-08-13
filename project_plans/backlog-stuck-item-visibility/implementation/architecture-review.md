# Architecture Review: backlog-stuck-item-visibility
**Date**: 2026-07-14 (iteration 2; re-confirmed iteration 3)
**Verdict**: CLEAN

> Scope note: iteration 2 re-checks only the items BLOCKED/raised in iteration 1 after a
> repair pass. Not a fresh full review. The iteration-1 BLOCKER and all 4 concerns are
> resolved; one low-severity nitpick is added.
>
> **Update (iteration 3):** Verdict remains CLEAN. The single iteration-2 nitpick — the
> `pr_ready_unmerged` resolve-completeness gap (a PR that stops being ready while staying
> `pr_pending`) — is now **RESOLVED** in the plan: Task 2.1.1a specifies the exact one-line
> fix this review recommended, an explicit `else ResolveStuck(StuckReasonPRReadyUnmerged)`
> branch on the not-ready case ("mark-OR-resolve every tick", plan.md lines ~246–254). This
> is the same fix adversarial-review C2 and pre-mortem F2 tracked; all three are now closed.
> No new architecture concerns in iteration 3.

## Constitution Violations
- N/A — no constitution file (`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repo). (Carried forward from iteration 1.)

## Blockers

**None — the iteration-1 blocker is resolved.**

- [x] **RESOLVED — Story 1.1.1/1.1.3 upsert vs. Story 2.1.5c episode-reopen contradiction.**
  The repair dropped episode history entirely and adopted a **resolve-in-place** model:
  exactly one row per `(item_id, reason)`, a **plain 2-column unique index**, `MarkStuck`
  as an atomic `INSERT ... ON CONFLICT(item_id, reason)` upsert, `ResolveStuck` as an
  atomic idempotent `UPDATE ... WHERE resolved_at IS NULL`. This removes the root of the
  contradiction: with only one row per pair, the `OnConflictColumns(item_id, reason)`
  target and the unique index columns are now identical, and both `item_id` and `reason`
  are non-nullable, so the SQLite "NULLs distinct in UNIQUE indexes" hazard no longer
  applies. The design as written works correctly on `mattn/go-sqlite3` — a plain 2-column
  unique index over two NOT-NULL columns is a valid `ON CONFLICT` target, and ent's
  `OnConflictColumns` emits the matching `ON CONFLICT (item_id, reason) DO UPDATE`. No new
  self-contradiction was introduced.
  - Verified no leftover contradiction: every remaining mention of episode history /
    append-only rows / 3-column or partial index (plan.md lines 53, 65, 143, 315, 489;
    ADR-001 lines 58–72, 90) is in **explicit-rejection** framing, not a live requirement.
    Task 2.1.5c (line 314–316), previously the source of the coexisting-rows requirement,
    now prescribes reopen-in-place on the single row and states "No 3-column or partial
    index." The `--feature sql/upsert` requirement is correctly retained (still needed for
    `OnConflictColumns`).
  - Confirmed the redesign does **not** silently drop a requirement: requirements.md
    Success Metric asks only for **current** stuck-state visibility, never historical
    episode tracking. Critically, root-cause-#4 (bouncing) detection is driven by
    `CountReviewCyclesSince` over the **existing `BacklogStatusEvent` table** (Task 2.1.4a),
    NOT by counting resolved rows in the stuck table — so episode history was never
    load-bearing for it. Dropping it loses nothing the requirements or detectors need.

## Concerns

**All 4 iteration-1 concerns resolved.**

- [x] **RESOLVED — canonical `DerivePRPriority` reuse.** Story 2.1.1 now keys the green-PR
  signal on `github.DerivePRPriority(info) == github.PRPriorityReady` (glossary line 34,
  Pattern Decisions line 57, AC lines 232–235, Task 2.1.1a line 239), with an explicit AC
  that a zero-review/unapproved PR passing the weaker `!HasBlockingReviews` is **not**
  flagged. The forked-readiness-definition risk is closed.
- [x] **RESOLVED — pure DB-independent decision core.** New Story 2.1.0 (lines 215–226)
  extracts `stuckPRReady`, `abandonedReview`, `isBouncing` as pure functions with
  table-driven unit tests and no ent import; the bouncing/PR-ready reconcilers now delegate
  to them (Tasks 2.1.1a, 2.1.4b) instead of inlining threshold arithmetic. The
  false-positive-risk logic (the flagged rabbit hole) is now unit-testable.
- [x] **RESOLVED — dual-write topology documented.** ADR-001 (lines 130–135) states the
  `MarkStuck`/`ResolveStuck`/`MarkStuckNotified` trio is the single storage seam both
  packages call, with detection triggers deliberately distributed by poll-vs-event shape.
- [x] **RESOLVED — timestamp semantics / non-idempotent resolve.** `ResolveStuck` is now
  atomic + idempotent (`WHERE resolved_at IS NULL`, affected-row-count, never overwrites an
  existing `resolved_at`) — glossary line 31, AC lines 174–175, ADR lines 118–122.
  `FindOpenStuckStates` parses rows into a proven open/un-snoozed projection at the
  repository boundary (parse-don't-validate), so callers never re-check nullable timestamp
  combinations — AC line 176, ADR lines 148–152.

### Crux re-check (first_detected_at under resolve-in-place) — PASS
The one thing worth double-checking in the redesign: does resolve-in-place still give the
UI a stable "stuck for N" duration? Yes. The design correctly distinguishes the two
`ON CONFLICT` cases: on conflict with an **open** row it updates `last_checked_at`/`context`
only and **leaves `first_detected_at` untouched** (Task 1.1.3a line 181; AC line 170–171
explicitly asserts "MarkStuck twice does not reset first_detected_at"); it resets
`first_detected_at` **only** when reopening a **resolved** row (AC line 172–173). So a
continuously-stuck item's duration counts from its true onset across all 15+ daily restarts,
and the 30-min `pr_ready_unmerged` gate (measured from `first_detected_at`) crosses as
intended — the Success Metric ("every item stuck beyond a threshold is visible") holds.
Resetting on genuine recurrence is the correct behavior, not a bug: an item that truly
un-stuck and re-stuck should show duration from the new onset.

## Nitpicks

- **RESOLVED (iteration 3, was NEW low) — `pr_ready_unmerged` resolve-completeness when a PR
  stops being ready while staying `pr_pending`.** Fixed exactly as recommended: Task 2.1.1a now
  carries the explicit `else ResolveStuck(StuckReasonPRReadyUnmerged)` branch on the not-ready
  case (plan.md lines ~246–254). The original nitpick text is retained below for history.
- **(historical, iteration-2 nitpick text) — `pr_ready_unmerged` resolve-completeness when a PR stops being ready while
  staying `pr_pending`.** `ResolveStuck` wiring (Story 2.1.5) covers status-transition-away
  paths and the self-heal sweep resolves rows whose **item status** no longer matches the
  reason's expected status. But if a PR was ready (open row, `first_detected_at=T0`) and
  then stops being `PRPriorityReady` (e.g. a new commit re-runs CI, or a fresh review is
  requested) **while the item stays in `pr_pending`**, neither trigger fires: the detection
  branch (Task 2.1.1a) only marks-stuck on the ready case, and self-heal only checks item
  status (still `pr_pending`, so it matches). The open row can linger and `ListStuckBacklogItems`
  would surface "PR ready to merge" for a PR that is momentarily not ready. Blast radius is
  small — a false operator *notification* is still gated on both `notified_at IS NULL` and
  the live ready check, so no false alert fires; only a stale UI row until the next real
  transition. Cheap fix at implementation time: in Task 2.1.1a's branch, call
  `ResolveStuck(StuckReasonPRReadyUnmerged)` on the not-ready `else`, symmetric to the
  mark-stuck path. Worth a one-line note in Story 2.1.1; not plan-blocking.
- **RESOLVED (was iteration-1 nitpick) — `StuckReason` "sum type" overclaim.** The plan and
  ADR-001 now consistently describe it as a "validated string-backed enum / validated
  primitive, not a truly-unrepresentable sum type" (glossary line 26, Pattern Decisions
  line 55, ADR lines 100–106). Language softened correctly; the type choice remains
  idiomatic for the house style.
- **Carried forward (unchanged, out of scope):** reconcilers depend on concrete
  `*EntRepository` (integration-testable only) — mitigated in practice by the new pure
  decision functions in Story 2.1.0 which ARE unit-testable. `BacklogLifecycleListener`
  SRP keeps growing (adds `reconcileBouncingItems`, `BackfillStuckStates`, self-heal sweep,
  per-detector `recover()`); pre-existing God-object trend, watch-item, not fixed here.
- **POSITIVE (carried forward, still accurate):** no speculative `StuckItemDetector`
  Strategy interface; new repository methods land on the existing `Storage` seam; reuses
  `notifyReworkCapHit`/`BacklogStatusEvent`; no new external dependency. The atomic-writes +
  self-heal + per-detector panic isolation story is coherent and honestly scoped ("stale
  writes possible but self-heal in ≤1 tick") rather than overclaiming race-freedom.
