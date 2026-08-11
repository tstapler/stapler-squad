# Architecture Review: review-session-notification-cleanup

**Date**: 2026-07-25 (iteration 1 — scoped re-review of prior BLOCKERs/Concern only)
**Verdict**: CONCERNS

## Constitution Violations
- None. `docs/adr/ADR-000-architecture-constitution.md` does not exist in this repository
  (confirmed via filesystem search); no constitution constraints apply.

## Blockers

None remain. Both blockers from the previous round are resolved.

- [x] **RESOLVED — Task 4.3.1a / Story 4.3 (`server/server.go`), `startedAt` scoping.** The plan
  now adds `startedAt time.Time` to the `Server` struct and sets it once in `newServerBase`
  (Story 4.3 / Task 4.3.1a, plan.md:936-991), which both `NewServer` and `NewServerWithDeps`
  call before either does anything server-specific. Verified directly against
  `server/server.go`: `newServerBase` (line 65) constructs `srv := &Server{...}` (lines 68-82)
  then `srv.addr.Store(&addr)` (line 83) — exactly the insertion point the plan names ("right
  after `srv := &Server{...}` is constructed... before `srv.addr.Store(&addr)`"). `NewServer`
  (line 107) and `NewServerWithDeps` (line 127) both call `newServerBase` first, then
  `wireDepsIntoServer(srv, deps, connCtx)` (lines 118, 129) — confirming both entry points
  genuinely converge on the shared base function. `wireDepsIntoServer` (line 138) is a function
  parameter `srv *Server`, so `srv.startedAt` is a valid, in-scope field reference inside the
  closure the plan adds at lines 949-970 (right after the real `notifStore, storeErr =
  notifications.NewNotificationHistoryStore(...)` success branch at server.go:203-214, which the
  plan's line citations match exactly). No undefined identifiers, correct receiver — this
  compiles as described. The old `NewServer`-local `startTime` (line 111) is explicitly called
  out in the plan as unrelated dependency-build timing instrumentation, untouched by this task —
  correctly distinguished from the new `srv.startedAt`.

- [x] **RESOLVED — Story 4.2 / Task 4.2.1a/4.2.1b (`server/notifications/store.go`), N+1-under-lock
  existence check.** The plan now injects a **batch** `existingSessionIDs func() map[string]struct{}`
  (renamed to `SetSessionExistenceLookup`, replacing the earlier per-record
  `SetSessionExistenceChecker`/`exists(sessionID string) bool` shape), called exactly **once** per
  prune pass inside `pruneOrphanedRecords` (plan.md:850-875), building an in-memory set checked via
  map membership per record — not a per-record `FindInstanceDataByID` call. Verified against the
  real `server/notifications/store.go`: `Append` (line 118) takes `s.mu.Lock()` and calls
  `s.enforceRetention()` (line 153) while holding it; `enforceRetention()` (line 437) is documented
  "must be called with the write lock held" and already computes `now := time.Now()` at its top —
  exactly what the plan's new gated block (plan.md:886-892) reuses, matching the real function's
  current structure. The plan also decouples the sweep from every single `Append()` via
  `lastOrphanPruneAt`/`orphanPruneInterval` (1 min default), gating the batch fetch inside
  `enforceRetention()`. Internal consistency verified: the setter name (`SetSessionExistenceLookup`),
  the struct field (`existenceChecker func() map[string]struct{}`), and both call sites
  (`enforceRetention()`'s `s.existenceChecker` and `PruneOrphaned`'s public `existingSessionIDs`
  parameter) all agree on the same batch function shape. `server/server.go`'s wiring in Task 4.3.1a
  calls `notifStore.SetSessionExistenceLookup(func() map[string]struct{} { ... })` — the new
  batch-shaped closure, not the old per-record predicate — and its body calls
  `storage.ListInstanceData()` (confirmed real method, `session/storage.go:381`, a thin wrapper over
  `s.repo.List(context.Background())`) exactly once per invocation, matching both `GetStableID()`
  and `Title` into the returned set, mirroring `InstanceData.MatchesID`'s existing two-way match
  (confirmed at `session/storage.go:388-403`). `PruneOrphaned` locks `s.mu.Lock()` and delegates to
  the (non-locking) `pruneOrphanedRecords`, matching the "assumes lock already held" contract used
  by both call paths — no double-lock or missing-lock defect.

## Concerns

- [x] **RESOLVED — magic-string metadata convention duplicated with no shared constant.** The plan
  now defines `events.SessionScopedMetadata(base map[string]string, itemID string) map[string]string`
  plus exported constants `MetadataKeySessionScoped`/`MetadataKeyItemID` in `pkg/events` (Task
  2.1.1c, plan.md:565-603), forwarded through `server/events/forward.go` alongside the existing
  `NewNotificationEvent`/`EventNotification` forwards. Verified `server/events/forward.go` already
  forwards `EventNotification` and `NewNotificationEvent` from `pkgevents` in exactly the pattern
  the plan describes, confirming the forwarding mechanism it extends is real, not hypothetical.
  All three original call sites now route through the one helper/constants:
  producer Task 2.2.1a (`server/review_queue_manager.go`) calls
  `events.SessionScopedMetadata(item.Metadata, linkedItemID)`; producer Task 3.2.1b
  (`server/services/autonomous_orchestration_service.go`) calls
  `events.SessionScopedMetadata(nil, linkedItemID)`; consumer Task 4.1.1b
  (`server/notifications/subscriber.go`'s `eventToRecord`) reads back via
  `event.NotificationMetadata[events.MetadataKeySessionScoped] == "true"` — the exported constant,
  not a raw string literal, at all three sites. No divergent literal remains.

The following Concerns from the previous round are **carried forward unaddressed** — the current
plan's text is unchanged on each of these points (confirmed by re-reading the corresponding
sections: Design Decision 6, the Risk Control table, and Story 2.1's acceptance criteria):

- [ ] **No enforcement ladder for a third, future producer forgetting to opt in.** Still no
  compile-time, lint-time, or test-time check that every session-identified
  `events.NewNotificationEvent(...)` call site sets the `session_scoped`/`item_id` metadata via
  `events.SessionScopedMetadata`. Documented trade-off in ADR-001, not blocking. Recommendation
  unchanged: add one test asserting the two known-current producer call sites include the metadata
  key.
- [ ] **Design Decision 6 still applies an inconsistent reachability standard.** The plan declines a
  `Hidden` gate on the "Triage stuck" call site (Story 3.1) as unreachable/untestable, but adds
  exactly that category of gate to the generic done/stuck notifier (Story 3.2) whose own example is
  explicitly "a hypothetical future Hidden autonomous-driver-run instance" — also unreachable today.
  No code change needed; just reconcile the write-up's reasoning across the two sites.
- [ ] **Story 2.1 — burst-transition latency still not analyzed.** `OnItemAdded`'s synchronous,
  2s-bounded `GetItemSessionBySessionUUID` lookup (Task 2.1.1b) still runs inline in the observer
  callback; the Risk Control table still only justifies "once per transition, not per tick," not
  the cumulative burst case (N simultaneous transitions after a restart serially blocking up to
  N×2s). No new text addresses this in the current revision.

## Nitpicks

Carried forward unchanged from the previous round (still applicable, not addressed by this
repair — none were in scope for this iteration):

- `NotificationRecord.SessionID` overloading two domain concepts (real session ID vs. backlog item
  ID) would be more cleanly modeled as two distinct optional fields or a small discriminated
  `NotificationSubject` type rather than a sibling `SessionScoped bool`. Proportionate trade-off
  given scope; flagging for awareness, not action.
- Parse-at-boundary is only half-applied: `SessionScoped bool` is a good typed field at the
  persisted-record boundary, but the upstream signal (`event.NotificationMetadata`) remains a raw
  `map[string]string` end-to-end. Correctly out of scope here; noted for a future, larger
  notification-metadata cleanup.
- Task 2.1.1b: when `rqm.poller.FindInstance(item.SessionID)` returns `nil`, `resolvedID` stays the
  raw title string, and `GetItemSessionBySessionUUID(lookupCtx, resolvedID)` is queried with a title
  rather than a UUID — it will simply miss (`ErrNotFound`), silently skipping `item_id` enrichment
  even for a genuinely backlog-linked session in that edge case. Low severity, worth a one-line
  comment at the call site.

## New Issues From This Round's Repair

None found. The repaired sections (Story 4.2, Story 4.3, Task 2.1.1c, and the three metadata-helper
call sites) were checked line-by-line against the real `server/server.go` and
`server/notifications/store.go` for naming/signature consistency (setter name vs. usage, closure
shape vs. field type, lock-holding contract between `Append`/`enforceRetention`/`PruneOrphaned`) —
no new compile error, race, or cross-section inconsistency was introduced.
