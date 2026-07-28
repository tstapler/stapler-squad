# Adversarial Review: review-session-notification-cleanup

**Date**: 2026-07-25 (re-review, iteration 1 of repair loop — scoped to the 2 prior blockers)
**Verdict**: CONCERNS

## Blockers

Both previously-identified blockers are **RESOLVED**.

- [x] **RESOLVED — Data race / fatal-crash risk in `OnItemAdded`.** The updated plan (Story 2.1's
  "data-race note", Task 2.1.1a/2.1.1b/2.2.1a) no longer writes to `item.Metadata` anywhere.
  Task 2.1.1a introduces a **local** `linkedItemID string`; Task 2.1.1b's linkage lookup assigns
  only that local; Task 2.2.1a builds a **fresh** map via
  `metadata := events.SessionScopedMetadata(item.Metadata, linkedItemID)` and passes `metadata`
  (not `item.Metadata`) as `events.NewNotificationEvent(...)`'s trailing argument. The new shared
  helper `events.SessionScopedMetadata` (Task 2.1.1c, `pkg/events`) does
  `m := make(map[string]string, len(base)+2); for k, v := range base { m[k] = v }` — ranging over
  a `nil` `base` (the common case, since `item.Metadata` is usually unset) is a safe no-op in Go,
  so there is no nil-map panic risk either. Verified against the real source: `session/queue/queue.go`'s
  `Add()` (lines 217-273) does exactly what the plan claims — stores `item` into `rq.items` at
  line 230, unlocks at line 258, then calls `observer.OnItemAdded(item)` unlocked at line 263 — so
  the race premise is real. `server/adapters/review_queue_adapter.go`'s `ReviewItemToProto` (lines
  42-55) already builds its own independent copy of `item.Metadata` for exactly this reason ("Always
  produce an independent copy of Metadata so concurrent RPC calls cannot race on the same
  underlying map") — the plan's citation of this pattern as precedent is accurate, and the new
  `OnItemAdded` code now mirrors it instead of writing back onto the shared pointer. Prose and code
  snippets in the current plan.md contain no remaining `item.Metadata[...] = ...` or
  `item.Metadata = make(...)` assignment.

- [x] **RESOLVED — `PruneOrphaned` N+1-under-lock.** The updated plan (Story 4.2, Task 4.2.1a/
  4.2.1b, Task 4.3.1a) replaced the per-record `func(sessionID string) bool` predicate with a
  batch `existingSessionIDs func() map[string]struct{}`, renamed to
  `NotificationHistoryStore.SetSessionExistenceLookup`. `pruneOrphanedRecords` now calls
  `existingSessionIDs()` **exactly once** per invocation, then checks each candidate record via
  in-memory `map[string]struct{}` membership — no per-record ent query. This also addresses the
  "runs on every single `Append()`" half of the original finding: Task 4.2.1b adds a
  `lastOrphanPruneAt time.Time` / `orphanPruneInterval = 1 * time.Minute` cadence gate inside
  `enforceRetention()` (`if s.existenceChecker != nil && time.Since(s.lastOrphanPruneAt) >=
  orphanPruneInterval`), so the batch fetch runs on a coarse timer rather than on every append.
  Verified against the real `server/notifications/store.go`: `Append()` (lines 118-156) does hold
  `s.mu.Lock()` for its full body (line 119-120, `defer s.mu.Unlock()`) and calls
  `s.enforceRetention()` (line 153) while that write lock is held, and `enforceRetention()` (lines
  437-454) already computes `now := time.Now()` at its top — exactly the local the plan says
  Task 4.2.1b reuses rather than calling `time.Now()` a second time. The design is a genuine fix
  matching the reviews' own recommendation ("fetch once, filter in memory"), not a relabeling.

**New blocker-level issues introduged by the repair**: none found. Specific things checked and
ruled out:
- No nil-map panic in `events.SessionScopedMetadata` (ranging a nil map is safe; see above).
- No double-locking/deadlock between the exported `PruneOrphaned` (which itself takes `s.mu.Lock()`
  then calls `pruneOrphanedRecords`) and the direct `enforceRetention()` call path (which calls
  `pruneOrphanedRecords` directly, already holding the lock via `Append`) — `pruneOrphanedRecords`
  itself never locks, only its two callers do, each exactly once.
- The `nil`-vs-empty-set sentinel distinction (`pruneOrphanedRecords` returns `0` immediately if
  `existingSessionIDs()` returns `nil`, never treating "not ready" as "everything is gone") is
  preserved correctly through both the `pruneOrphanedMinUptime` startup gate and a `ListInstanceData`
  fetch-failure path.

## Concerns

Carried forward from the previous round — none of these were in scope for this repair pass and
none appear to have been addressed by the plan.md changes reviewed here.

- [ ] **No structural guard against a 4th suppression-decision path drifting out of sync.** The
  plan still documents 3 separate hand-applied `Hidden`-gating call sites (`Determine()`,
  `OnItemAdded`, one branch of `onAutonomousDriverComplete`) with no lint/test/registry enforcing
  that a future 5th `events.NewNotificationEvent(` call site also gets gated — the same failure
  class that produced the bug being fixed. Recommend at minimum a comment convention or a
  table-driven test enumerating all known producers, mirroring this repo's
  `session-creation-registry.md`/`feature-testing-registry.md` pattern.

- [ ] **Synchronous DB lookup added to `OnItemAdded`'s critical path is still mischaracterized as
  matching `maybeAutoCreatePR`'s pattern.** Task 2.1.1b's `itemSessionLookupTimeout` (2s) is still
  a synchronous, inline call inside `OnItemAdded`, itself running inside one of up to 5 concurrent
  `checkSession` goroutines — unlike `maybeAutoCreatePR`'s async-goroutine DB calls. `pitfalls.md`'s
  proposed alternative (cache the resolved `(item_id, sessionRole)` pair on `Instance` at
  session-creation time) is still not discussed/rejected in Step 0.5. Note: this alternative, if
  adopted, would also have sidestepped Blocker 1 entirely (nothing would need to build metadata at
  notification time) — but the plan instead fixed Blocker 1 directly, which is a valid resolution
  on its own terms; this Concern is about lookup latency/contention, not correctness.

- [ ] **Epic 3 still spends a full Story + dedicated regression test (Task 3.3.1a) on the
  `SessionRoleTriage` "stuck" branch that the plan's own research confirms is unreachable in
  production.** Design Decision 6 uses this same unreachability to justify *not* adding a
  `Hidden` gate to that branch, yet a dedicated unit test for the metadata fix remains. The
  one-line metadata fix itself is fine; the dedicated test is disproportionate maintenance surface
  for unreachable code.

- [ ] **AC2's "not a dead link" premise for a deleted backlog item still rests on a code trace, not
  an executed test.** No unit/integration/e2e test exercises a notification whose `item_id` points
  at an already-deleted backlog item, even though this scenario becomes materially more common
  once this fix ships (item_id-bearing notifications for completed sessions routinely outlive
  their backlog items after closure/deletion).

- [ ] **AC3's pruning is still bounded only by "whenever the next notification happens to be
  appended," not by any independent time trigger.** The new `orphanPruneInterval` cadence gate
  bounds how *often* a sweep can run, but the sweep is still only ever invoked from inside
  `Append()` (via `enforceRetention()`) — on a quiet system with no new notifications for days, an
  orphaned record can still sit well past the 7-day age-based retention AC3 is meant to improve on.
  This is an acceptable "opportunistic GC" trade-off if explicitly documented as such, but the plan
  still does not call it out as an accepted limitation.

## Minors

- ADR-001's `metadata["session_scoped"]` convention remains unenforced by any lint/test — a new
  `NewNotificationEvent` call site that forgets to set `session_scoped`/`item_id` fails silently
  (record just becomes un-prunable, not incorrectly deleted — a safe failure mode, but still
  tribal knowledge).
- Story 3.2's acceptance criteria still constructs "a hypothetical future Hidden
  autonomous-driver-run instance" to justify testing a `Hidden` gate that, per the plan's own
  research, is equally unreachable today (since `SessionRoleReview` always early-returns before the
  generic notifier is reached) — the same "currently dead code" situation Design Decision 6 uses to
  justify *skipping* a test for the Triage-stuck branch. Inconsistent internal standard, though
  harmless since the added gate itself is free.
