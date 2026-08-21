# Adversarial Review: backlog-stuck-item-visibility

**Date**: 2026-07-14 (iteration 2; verdict updated iteration 3)
**Verdict (iteration 3): CLEAN** — the two iteration-2 concerns (C1, C2) are now both
resolved in `implementation/plan.md`. See the per-concern "Update (iteration 3)" notes below.

> **Update (iteration 3):** The iteration-2 verdict was CONCERNS solely because C1 and C2
> were open. Both are now closed by concrete plan changes:
> - **C1** (self-heal sweep's `reason → expected status` mapping under-specified) is resolved
>   by the **per-reason expected-status table added to Task 2.1.5d** (plan.md lines ~344–355):
>   the three status-anchored reasons map to a single status, `bouncing` maps to the set
>   `{in_progress, review}` and resolves only on `done`/PASS, and `rework_cap`/`push_failed`
>   are **excluded from the status sweep** (`continue`) and resolved only by their event-site
>   `ResolveStuck` (Tasks 2.1.5b / 2.1.6a). This matches the C1 regression tests named in
>   validation.md.
> - **C2** (poll-shaped conditions that clear without a status change have no resolve path)
>   is resolved by the **explicit "mark-OR-resolve every tick" else-branches** now specified
>   in Task 2.1.1a (`pr_ready_unmerged`), Task 2.1.3a (`abandoned_review`), and Task 2.1.3b
>   (`stale_work`) — each calls the atomic idempotent `ResolveStuck(reason)` directly when its
>   poll shows the condition no longer holds, exactly as this review recommended. This is also
>   tracked as pre-mortem F2 (P1), marked resolved.
> The verdict below (CONCERNS) and the two `[ ]` concern boxes reflect the iteration-2 state;
> they are retained for history and re-marked `[x]` with the resolution notes inline.

**Iteration-2 verdict (historical)**: CONCERNS

Scope of iteration 2: re-check ONLY the two iteration-1 blockers and the five iteration-1
concerns against the repaired `implementation/plan.md` and `decisions/ADR-001-*.md`. Both
blockers are resolved with concrete, implementable mechanics. The five concerns are all
addressed with real plan changes (specific stories/tasks), not just prose claims. Two new
concerns surface in the repair itself — both in the self-heal sweep (Task 2.1.5d), and both
about the *completeness* of the resolve model rather than a plan-breaking defect. The panic
vs. self-heal interaction flagged for scrutiny is handled **correctly** (see Verified clean).

## Blockers

None — all iteration 1 blockers resolved.

- **B1 (unique-index self-contradiction) — RESOLVED, concretely.** The design is now internally
  consistent end to end: Glossary (`BacklogStuckState`, `MarkStuck`), Pattern Decisions row 1,
  Migration Plan, Story 1.1.1 / Task 1.1.1a, ADR-001 Option B, and Explicit Non-Goals all state
  the same thing — **exactly one row per `(item_id, reason)`, resolve-in-place, plain 2-column
  unique index `index.Fields("item_id","reason").Unique()`** as both the `OnConflictColumns`
  target and the correctness guard. Episode history is explicitly "considered and rejected" in
  ADR-001 (with the SQLite-NULLs-distinct rationale) and again in Non-Goals. `OnConflictColumns`
  now targets a real non-partial index, so the "recurrence updates the resolved row and stays
  hidden" failure mode is gone. This is specified, not asserted.

- **B2 (TOCTOU + no self-heal) — RESOLVED, with a residual specification gap (see Concerns).**
  `MarkStuck` is now a single atomic `INSERT ... ON CONFLICT(item_id, reason)` upsert (Task
  1.1.3a); `ResolveStuck` is a single atomic idempotent `UPDATE ... WHERE resolved_at IS NULL`
  returning affected-row-count (Task 1.1.3b); the `ExpectedStatus` check is correctly downgraded
  to a "best-effort pre-filter" and the plan no longer overclaims "a tick can never win a race."
  The phantom-from-race recovery path that was entirely absent in iteration 1 now exists (Task
  2.1.5d self-heal sweep) and per-detector panic isolation is added (Task 2.1.5e). The specific
  iteration-1 leak — "a stale row written after the transition with no un-stick event to clear
  it" — is now closed because that row is status-inconsistent and the sweep resolves it within
  one tick. ADR-001 Consequences states the honest guarantee. The atomic-write mechanics are
  concrete; the sweep's *coverage* is not complete (C1/C2 below).

## Concerns

- [x] **C1 — RESOLVED (iteration 3).** The per-reason expected-status table was added to
  Task 2.1.5d (plan.md lines ~344–355): anchored reasons → one status; `bouncing` → set
  `{in_progress, review}`, resolving only on `done`/PASS; `rework_cap`/`push_failed` excluded
  from the status sweep and resolved by their event-site `ResolveStuck` (Tasks 2.1.5b/2.1.6a).
  The original concern text is retained below for history.
- [ ] **C1 (iteration-2, historical) — Self-heal sweep's `reason → expected status` mapping is under-specified for 3 of
  the 6 reasons.** Task 2.1.5d resolves a row when "the item's current status is inconsistent
  with its reason's expected status" (singular). That is well-defined for the three status-anchored
  reasons (`pr_ready_unmerged`→`pr_pending`, `abandoned_review`→`review`, `stale_work`→`in_progress`).
  It is NOT well-defined for the other three: `bouncing` legitimately spans a **set** {`in_progress`,
  `review`} (that is the definition of bouncing), and the two event-shaped reasons (`rework_cap`,
  `push_failed`) are written with `expectedStatus=<current>` (Tasks 2.1.2a, 2.1.6a) — they have no
  fixed anchor status for the sweep to compare against. As written, a naive implementer could make
  the sweep resolve a valid `bouncing` row the moment the item sits in `in_progress` (its healthy
  half-cycle), or resolve a valid `rework_cap`/`push_failed` row against an arbitrary status. —
  **Recommendation**: add an explicit per-reason expected-status(es) table to Task 2.1.5d: anchored
  reasons map to one status; `bouncing` maps to the set {`in_progress`,`review`} and resolves only
  on a terminal/`done`/PASS status; `rework_cap`/`push_failed` either map to their parked status
  (`review`) explicitly or are **excluded from the status-based sweep** and rely on their event-site
  `ResolveStuck` (Task 2.1.5b/2.1.6a) instead. Decide before Story 2.1.5.

- [x] **C2 — RESOLVED (iteration 3).** Explicit "mark-OR-resolve every tick" else-branches were
  added to Task 2.1.1a (`pr_ready_unmerged`), Task 2.1.3a (`abandoned_review`), and Task 2.1.3b
  (`stale_work`) — each calls the atomic idempotent `ResolveStuck(reason)` when its poll shows the
  condition no longer holds, closing the gap the status-based sweep structurally cannot see.
  Tracked as pre-mortem F2 (P1, resolved). The original concern text is retained below for history.
- [ ] **C2 (iteration-2, historical) — Poll-shaped conditions that clear WITHOUT a status change have no resolve path.** The
  self-heal sweep is deliberately *status-based* (correct for the panic interaction — see Verified
  clean), but that means it cannot detect a condition clearing while the item's status is unchanged.
  Two real cases: (a) `pr_ready_unmerged` — a PR that was `PRPriorityReady` gets a new commit / CI
  starts re-running / a review is requested, so `DerivePRPriority != Ready` while the item stays
  `pr_pending`; (b) `stale_work` — a stale session resumes reporting progress while the item stays
  `in_progress`. In both, the status is still consistent with the reason, so the sweep leaves the
  row open, and Task 2.1.1a / Task 2.1.3b specify only the positive (`MarkStuck`) branch — no
  `else ResolveStuck`. Result: the UI keeps showing "PR ready to merge, stuck 3d" or "work stale"
  after the condition has actually cleared — a false-positive that directly undermines the
  "trustworthy signal" value prop the feature exists to protect. `onSessionExited` backstops
  `stale_work` only when the session eventually exits (bounded but not immediate); the
  `pr_ready_unmerged` case has no backstop until merge. The fix is trivial and belongs in the
  detectors, not the sweep. — **Recommendation**: give each poll-shaped reconciler an explicit
  else-branch — when the poll shows the condition no longer holds, call the atomic idempotent
  `ResolveStuck(reason)`. State this in Task 2.1.1a (`pr_ready_unmerged`) and Task 2.1.3a/2.1.3b
  (`abandoned_review`/`stale_work`). This is the natural idempotent-reconcile shape (mark OR resolve
  every tick) and closes the gap the status-based sweep structurally cannot.

## Minors

All four iteration-1 minors are resolved and can be closed:
- **M1 (multi-reason fanout undocumented) — RESOLVED.** Observability Plan now has a "Multi-reason
  fanout (deliberate decision, pitfalls §3)" paragraph accepting the per-`(item_id,reason)` row →
  per-reason notification tradeoff explicitly.
- **M2 (backfill runs with feature flag disabled) — RESOLVED.** Task 1.1.4b now gates the backfill
  call so it runs "only when the backlog feature is enabled."
- **M3 (`onSessionExited` not in ResolveStuck list) — RESOLVED.** Now named explicitly in Story
  2.1.5 Files and Task 2.1.5a; the status-based self-heal sweep is the additional backstop.
- **M4 (in-memory map removal equivalence) — RESOLVED.** Task 2.1.3c deletes the map fields/mutexes;
  iteration-1 already confirmed the maps are the sole dedup gate, so the DB-marker swap is
  behavior-equivalent.

New minor:
- The event-shaped reasons written with `expectedStatus=<current>` (Tasks 2.1.2a, 2.1.6a) make the
  "best-effort pre-filter" a near no-op for those two reasons (current always equals current at
  write time). That is acceptable — their correctness rests on the event site being authoritative,
  not on the pre-filter — but worth a one-line note so a reader doesn't expect the pre-filter to
  guard them.

## Verified clean (no action needed)

- **The panic-vs-sweep interaction (the specific scrutiny target) is handled correctly.** A
  status-based self-heal sweep does NOT re-run detectors, so a panicking detector (Task 2.1.5e
  catches it) leaves the item's status unchanged — still consistent with that reason — and the
  sweep therefore leaves the reason's rows **alone** rather than wrongly resolving them as "condition
  no longer holds." The plan avoids the "detector failed to run ⇒ falsely auto-resolve" trap by
  keying the sweep on persisted item status rather than on a fresh condition re-evaluation. This is
  the right choice; note that it is the *same* property that creates C2 (status-based can't see a
  same-status clear), so C2's fix must live in the detectors, never by making the sweep
  condition-based (which would reintroduce the panic-false-resolve hazard).
- **All 5 iteration-1 concerns addressed with concrete plan changes**: C1 backfill excludes
  `pr_ready_unmerged` (Migration Plan backfill step + Task 1.1.4a + Story 1.1.4 AC, "does NOT call
  GetPRStatus/IsPRMerged"); C2 `allow_auto_merge` is a single per-repo TTL-cached fetch gated on
  `github.DefaultRateLimiter` (Unresolved Q2 + Story/Task 4.1.4); C3 per-detector panic recovery
  (Task 2.1.5e); C4 `pr_status_unknown` staleness = concrete 5 min (Glossary + Unresolved Q1 +
  Story 4.1.2 AC); C5 `push_failed` is a distinct 6th `StuckReason` with its own Story 2.1.6 /
  Task 2.1.6a and an explicit "literal push/PR-create failure, distinct from pr_ready_unmerged"
  decision (Glossary + ADR-001).
- **Atomic-write mechanics are specified, not asserted**: exact SQL shapes for `MarkStuck`
  (`INSERT ... ON CONFLICT(item_id, reason)`) and `ResolveStuck` (`UPDATE ... WHERE resolved_at IS
  NULL`, affected-row-count, idempotent) appear in Glossary, Pattern Decisions, Tasks 1.1.3a/1.1.3b,
  and ADR-001 Consequences — consistent across all four locations.
