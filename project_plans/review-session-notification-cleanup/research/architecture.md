# Architecture Research: review-session-notification-cleanup

**Date**: 2026-07-25 | **Agent**: 3 (Architecture), Phase 2

## Prior analysis incorporated (cited, not re-derived)

- `project_plans/review-gate-stale-session-rework/research/architecture.md` — establishes the
  three structurally distinct staleness mechanisms (Review Queue badge via `Determine()`,
  durable `StuckReasonStaleWork` via `backlog_lifecycle.go` reconcile, and the ad-hoc
  `notifyIfActiveWorkSessionStale` direct-publish). This item's `ReasonTaskComplete`/`ReasonIdle`/
  `ReasonStale` notifications flow through that doc's first mechanism only
  (`review_queue_determiner.go` → `OnItemAdded`) — the other two are out of scope here (they
  already carry `item_id` or are a separate in-progress item, not review/triage).
- `project_plans/review-queue-state-detection/research/03-architecture.md` — establishes the
  `ReviewQueueObserver` pattern and `ReviewQueuePollerConfig` threshold location, both reused
  below without modification.

## Q1 — Where should the Hidden/SessionRole suppression check live?

### There are three distinct call paths into `Determine()` / `queue.Add()`, only one of which is guarded today

| Call site | Hidden guard? | Feeds the shared queue (w/ observers)? |
|---|---|---|
| `ReviewQueuePoller.checkSession` (`session/review_queue_poller.go:640-661`) | **Yes** — `shouldSkipSession` (`:631-636`) checks `snap.Hidden` before ever calling `Determine()` | Yes — this is the steady-state poll loop |
| `StartupScanner.Scan` (`session/startup_scanner.go:32-56`) | **No** — only checks `!inst.Started() \|\| inst.Paused()` (`:36`), then calls `ss.determiner.Determine(inst, ...)` directly (`:44`) and `queue.Add(item)` (`:46`) unconditionally on `DetectionActionAdd` | Yes — `server/dependencies.go:722` calls `scanner.Scan(instances, reviewQueue)` where `reviewQueue := svc.ReviewQueue` (`:455`) is the **same** global queue `ReactiveQueueManager` subscribes to (`rqm.queue.Subscribe(rqm)`, `server/review_queue_manager.go:159`, called from `Start()` which `server/server.go:140` runs via `go deps.ReactiveQueueMgr.Start(serverCtx)`) |
| `GetReviewQueue` RPC (`server/services/review_queue_service.go:108-111`) | No check, but harmless | **No** — it builds a throwaway `queue := session.NewReviewQueue()` (`:108`) purely for response filtering; a fresh `ReviewQueue` has an empty `observers` slice (`session/queue/queue.go:206-211`), so `.Add()` there never reaches `OnItemAdded` |

**This is very likely the actual, reproducible root cause of the bug report.** `Instance.Hidden`
is persisted (`session/ent/schema/session.go:102`, `session/storage.go:117`), so a `review:<hash>`
session's Hidden Instance survives a service restart in storage. Given this repo's own operational
pattern of restarting the service with `--tmux-keep-server` (`.claude/rules/tmux-keep-server-on-restart.md`)
so tmux panes — including a still-running one-shot review session — survive the restart, a Hidden
review Instance can plausibly still be `Started()` and un-paused when `BuildRuntimeDeps` reloads
`instances := storage.LoadInstances()` (`server/dependencies.go:462`). 500ms later (`:719`)
`StartupScanner.Scan` runs `Determine()` on it with **zero Hidden check**, and if it returns
`DetectionActionAdd` (e.g. `ReasonIdle`/`ReasonStale`, `session/review_queue_determiner.go:174,243`),
`queue.Add(item)` fires `OnItemAdded` → an unsuppressed notification. This reproduces exactly what
AC1 describes ("siloed" Hidden check) and gives Phase 3 a concrete, testable defect instead of a
speculative one.

### Recommendation: fix both ends — (a) inside `Determine()` for defense-in-depth, and separately close the actual bypass in `StartupScanner`

- **`Determine()` (`session/review_queue_determiner.go:97`) already receives `*Instance` as its
  first argument**, and `Instance.Hidden` is a plain, always-populated field (`session/instance.go:220-223`)
  — no new dependency, no DB lookup, no interface change. Add an early return
  (`Action: DetectionActionSkip`) at the top of `Determine()` when `inst.Hidden`. This makes the
  pure-function guarantee AC1 asks for ("independent of the poller's existing `shouldSkipSession`
  guard") literally true — every current and future caller of `Determine()` is covered, including
  `StartupScanner` and any caller added later, closing the exact class of gap that caused this bug.
  This does not weaken `shouldSkipSession`'s existing, slightly different purpose (skipping
  `Stopped`/`Paused`/unstarted instances too) — that check stays as a cheaper pre-filter.
- **Also fix `StartupScanner.Scan` directly** (add `|| inst.Hidden` to its skip condition at
  `session/startup_scanner.go:36`) so the specific reproducible bypass is closed at its source, not
  only papered over by the deeper fix — both changes are cheap and the codebase's own stated
  intent (per `Determine()`'s doc comment: "pure function, no side effects") supports keeping the
  determiner authoritative rather than trusting every caller to pre-filter correctly.
- **`SessionRole` (review/triage) cannot be resolved this way** — it lives on `ItemSession`, a
  separate ent-backed row reached only via `session_uuid` (a loose FK, not an edge —
  `session/ent/schema/item_session.go`), so it requires a DB round trip `Determine()` does not
  have and should not gain (would violate its documented "pure function" contract and the
  interface-pollution guidance against widening a pure decision function with an I/O dependency
  used by only one caller class). This check belongs at **`OnItemAdded`**
  (`server/review_queue_manager.go:319`), which already has `rqm.storage` wired in
  (`NewReactiveQueueManager`'s 5th parameter, `:117`) and already does one adjacent lookup at this
  exact point — see Q2.

**Net answer: option (c), both — but asymmetric.** `Determine()` gets the free `Hidden` check
(zero new dependencies, closes the bug at its structural root for every caller). `OnItemAdded` gets
the `SessionRole` check (the only site with both the DB dependency and the per-event, not
per-poll-tick, cost budget for it).

## Q2 — Resolving `item_id` (and `SessionRole`) without a new N+1 pattern

`session/storage_backlog.go:185-197`'s `GetItemSessionBySessionUUID` is the existing, reusable,
single-query helper — already exposed on `*Storage` at `session/storage.go:932-938`:

```go
// GetItemSessionBySessionUUID looks up the most recent active ItemSession by session UUID alone.
// ... Loads the BacklogItem edge so BacklogItemID is populated in the returned summary.
func (r *EntRepository) GetItemSessionBySessionUUID(ctx context.Context, sessionUUID string) (ItemSessionSummary, error) {
	is, err := r.client.ItemSession.Query().
		Where(itemsession.SessionUUID(sessionUUID)).
		WithBacklogItem().
		Order(ent.Desc(itemsession.FieldCreatedAt)).
		First(ctx)
	...
```

`ItemSessionSummary` (`session/repository.go:285-308`) carries both `Role` and `BacklogItemID` —
**one query answers both Q1's SessionRole suppression check and Q2's item_id enrichment**. This
exact call is already used for precisely this purpose in `server/services/autonomous_orchestration_service.go:270`
(`is, err := concreteStorage.GetItemSessionBySessionUUID(ctx, sessionUUID)`, then
`item, itemErr := concreteStorage.GetBacklogItem(ctx, is.BacklogItemID)`), confirming this is
already the established pattern for "resolve backlog linkage from a session UUID" in this
codebase, not a new one being introduced.

**Key for the lookup**: `ItemSession.SessionUUID` is populated with the review Instance's real
`UUID`, not its title — confirmed at `session/review_gate.go:348-351`:
```go
if _, createErr := r.storage.CreateItemSession(ctx, ItemSessionData{
    ...
    SessionUUID: reviewInst.UUID,
    SessionRole: SessionRoleReview,
```
And `OnItemAdded` **already resolves exactly this UUID** two lines above where the suppression
check would go: `resolvedID := item.SessionID; if inst := rqm.poller.FindInstance(item.SessionID); inst != nil { resolvedID = inst.GetStableID() }`
(`server/review_queue_manager.go:348-353`), and `Instance.GetStableID()` returns `i.UUID` whenever
it's set (`session/instance_terminal.go:25-32`) — i.e. the same value `SessionUUID` was populated
with at spawn time. So the fix is: call `storage.GetItemSessionBySessionUUID(ctx, resolvedID)`
once, right after `resolvedID` is computed, reusing a value already being computed for a different
purpose in the same function — genuinely zero N+1 risk, since `OnItemAdded` fires once per queue
mutation event (not per poll tick, which happens every 2-8s regardless).

**Cost note**: `notifyIfActiveWorkSessionStale`-style call sites in `backlog_service_triage.go`
(lines 145, 180, 214, 239, 987, 2041, 2132) already construct `map[string]string{"item_id": item.ID}`
directly (they have `item.ID` in scope already, no lookup needed) — confirming `item_id` metadata
is an established, already-partially-implemented convention in this codebase, not a new concept
AC2 introduces. One live gap found in that same family: `server/services/autonomous_orchestration_service.go:305-316`
publishes a "Triage stuck" notification with `nil` metadata (last arg to `NewNotificationEvent`) —
this violates AC2's intent identically to the main bug, though via a currently-**unreachable** code
path (see Q4 note below on why `SessionRoleTriage` is dead in that switch today). Worth fixing in
the same PR since it's a one-line change and would silently regress AC2 the moment autonomous
triage sessions become real (a plausible near-term follow-up given the surrounding code already
anticipates it).

## Q3 — Pruning notifications whose session/instance no longer exists

**No existing sweep does this.** `server/notifications/store.go:437-456`'s `enforceRetention()`
(called from `NewNotificationHistoryStore` at `:98` and after every `Append` at `:153`) is strictly
age (`MaxNotificationAge` = 7d) and count (`MaxNotifications` = 500) based — it has no concept of
"does the referenced session still exist." `session/review_queue_poller.go:251`'s
`cleanupOrphanedItems` is a different thing entirely: it prunes the **live in-memory `ReviewQueue`**
(items with invalid `LastActivity` timestamps), not the **persisted `NotificationHistoryStore`** —
these are two separate stores with no shared lifecycle today.

**The reusable "does this session still exist" check already lives one hop away**:
`server/services/notification_service.go`'s `NotificationService` struct already holds **both**
dependencies needed — `notificationStore *notifications.NotificationHistoryStore` (`:31`) and
`reviewQueuePoller *session.ReviewQueuePoller` (`:32`, wired late via `SetReviewQueuePoller`,
`:56-58`) — and already uses exactly this existence check for a different purpose at `:91-93`:
```go
if ns.reviewQueuePoller != nil {
    if inst := ns.reviewQueuePoller.FindInstance(req.Msg.SessionId); inst != nil {
```
`FindInstance` (`session/review_queue_poller.go:897`) returns `nil` when no live `Instance`
matches — the canonical "session/instance is gone" signal this repo already relies on (also used
identically in `OnItemAdded`, Q2 above).

**Recommended shape — no new goroutine/ticker**: add a `PruneOrphaned(exists func(sessionID string) bool) int`
method to `NotificationHistoryStore` (takes a predicate, not a `*session.ReviewQueuePoller`, to
avoid an import cycle / new dependency in the `notifications` package — same pattern
`session/git/ops.go`'s hybrid-fallback style favors: inject the minimum capability needed, not the
whole owning type). Call it from `NotificationService`, which already has both pieces wired,
**piggybacked on the existing `enforceRetention()` call site inside `Append()`** (`:153`) — every
new notification append already triggers one retention pass; extending that pass to also drop
orphaned records adds no new scheduling primitive and keeps the "prune on write" cadence this store
already uses instead of a separate ticker goroutine.

**Scope carefully — do not delete legitimately-kept backlog-linked notifications**: a notification
whose `Instance` is gone but whose `metadata["item_id"]` is set is *not* orphaned in the sense this
AC means — its "View in Backlog" link is still valid and useful (this is the intended steady state
for *every* completed review/work session notification once AC2 ships). The prune predicate should
be: `record.Metadata["item_id"] == "" && !exists(record.SessionID)` — i.e. only records whose sole
navigation target was the now-dead "View Session" link. This also means AC1's suppression and
AC3's pruning are complementary, not overlapping: AC1 stops the notification from ever being
created for Hidden/review/triage sessions; AC3 cleans up the ones that already exist from before
the fix ships (or any that slip through some other minor gap), scoped to exactly the
no-item_id-and-dead-session case the bug report describes.

## Q4 — Full data flow, both cases

### Case A: `review:<hash>` — real, Hidden `Instance` (the confirmed-live bug path)

```
SpawnReviewSession (session_service.go:826)
  -> CreateDirectorySession(..., oneShot=true, hidden=true) (:839)
       -> Instance{Hidden: true, UUID: <uuid>} started, added to
          reviewQueuePoller.instances (:869) AND storage (:867)
       -> review_gate.go:348-351 creates ItemSession{SessionUUID: reviewInst.UUID,
          SessionRole: SessionRoleReview} — the item_id linkage now exists in the DB

  [steady state]                              [after a service restart]
  poller.checkSession (poller.go:640)          BuildRuntimeDeps reloads Hidden Instance
    -> shouldSkipSession sees Hidden=true         from storage (dependencies.go:462)
    -> RETURN, never reaches Determine()        -> StartupScanner.Scan (startup_scanner.go:32)
    -> NO notification (correct today)             -> NO Hidden check (BUG)
                                                    -> Determine() called directly (:44)
                                                    -> queue.Add(item) if Action==Add (:46)
                                                    -> OnItemAdded fires (review_queue_manager.go:319)
                                                    -> eventBus.Publish(NotificationEvent) (:365)
                                                    -> NotificationService receives via eventBus
                                                       subscription, store.Append (store.go:118)
                                                    -> persisted with metadata=nil (no item_id today)
                                                    -> NotificationsPage.tsx renders "View Session"
                                                       (:386-393, since metadata.item_id is falsy)
                                                       -> dead link (Instance/tmux session already
                                                          gone or about to be)
```

**Fix intercepts**: `Determine()`'s new `inst.Hidden` early-return (Q1) blocks this at the
structural root for both the poller and StartupScanner paths. The `StartupScanner.Scan` direct fix
(Q1) closes the specific reproducible bypass belt-and-suspenders. If a Hidden/review-role
notification is ever legitimately published anyway (future caller, or before this fix ships),
`OnItemAdded`'s new `GetItemSessionBySessionUUID` lookup (Q2) stamps `item_id` so the frontend link
is at worst "View in Backlog" instead of dead, and AC3's `PruneOrphaned` (Q3) cleans up any that
still lack `item_id` once the Instance is gone.

### Case B: headless-triage — synthetic UUID, **no `Instance` at all**

```
TriggerTriage (backlog_service_triage.go:1758)
  -> triageSessionUUID := "headless-triage-" + uuid.New()  (:1781, synthetic — no tmux, no Instance)
  -> storage.CreateItemSession(ItemSessionData{SessionUUID: triageSessionUUID,
       SessionRole: SessionRoleTriage}) (:1789)
  -> go func() { s.headlessPool.CallBlocking(...) }()  (:1824, in-process LLM call, 30m ctx timeout)
       on success: persists TriageResult, transitions item status, ends ItemSession (:1855-1922)
                   — NO notification published on the success path at all
       on error/timeout: only a log.ErrorLog line (:1855-1862) — NO notification published either
       on persistence failure: notifyTriagePersistFailure (:1911) — ALREADY item_id-enriched
```

**Finding: headless-triage sessions do not, and structurally cannot, produce a generic
TASK_COMPLETE/Idle/Stale notification today**, because that message family originates exclusively
from `Determine()` (`session/review_queue_determiner.go:174,243` — the only two call sites in the
whole repo constructing "Session idle - ready for next task"-style text), and `Determine()`
requires a `*Instance`. Headless triage never creates one — it is a bare in-process function call.
The requirements doc's "open question" (a headless-triage session producing this notification
class) resolves to: **it doesn't happen via any path found in this codebase today.**

The one adjacent, *reachable-looking* path that does construct a bare, non-generic notification
with `nil` metadata for a `SessionRoleTriage` item session
(`autonomous_orchestration_service.go:305-316`, "Triage stuck") is **currently dead code**: it fires
only when `GetItemSessionBySessionUUID` resolves a real `Instance`'s completion to an ItemSession
with `Role == SessionRoleTriage`, but the only code path that ever creates a `SessionRoleTriage`
ItemSession is `TriggerTriage` above, which never has a real `Instance` — confirmed by grepping
every `SessionRole:.*SessionRoleTriage` assignment in the repo (`backlog_service_triage.go:1789`
and a debug-seed handler, `backlog_debug_seed_handler.go:299`, neither Instance-backed). Recommend
fixing its `nil` metadata → `map[string]string{"item_id": item.ID}` anyway (one line, in-scope
family, cheap insurance against a near-future regression) but do **not** design a new suppression
mechanism around it — there is nothing live to suppress. AC1's `SessionRole` clause is fully
satisfied by the `OnItemAdded` check (Q1/Q2) alone, since **if** a real Instance-backed triage/review
session is ever added later, that check already covers it without further changes.

## Event-Command-Policy table

| Domain Event | Policy trigger | Command | Actor / System | Notes |
|---|---|---|---|---|
| `HiddenInstanceReachedIdleOrStale` | Whenever `Determine()` is called with `inst.Hidden == true` | `SuppressDetection` (NEW — early return `DetectionActionSkip`) | `Determine()` (`session/review_queue_determiner.go`) | Closes the structural gap regardless of caller (poller, StartupScanner, any future caller) |
| `SessionAddedToQueue` | Whenever `queue.Add()` triggers `OnItemAdded` for a non-`ApprovalPending` reason | `ResolveItemSessionLinkage` (NEW — `GetItemSessionBySessionUUID`) | `ReactiveQueueManager.OnItemAdded` (`server/review_queue_manager.go`) | Single query yields both `SessionRole` (suppress if review/triage) and `BacklogItemID` (enrich if not suppressed) |
| `BacklogLinkedSessionReachedIdleOrStale` | Whenever the linkage lookup returns a `BacklogItemID` and the item is NOT suppressed (e.g. a real work session) | `EnrichNotificationMetadata` (NEW — stamp `item_id`) | `ReactiveQueueManager.OnItemAdded` | Satisfies AC2 for the "legitimately fires" case out-of-scope items call out |
| `ReviewOrTriageSessionReachedIdleOrStale` | Whenever the linkage lookup returns `Role in {review, triage}` | `SuppressNotificationPublish` (NEW) | `ReactiveQueueManager.OnItemAdded` | The actual AC1 enforcement point for `SessionRole` (cannot live in `Determine()`, see Q1) |
| `NotificationAppended` | Every `Append()` call | `EnforceRetention` (existing) + `PruneOrphaned` (NEW, piggybacked) | `NotificationHistoryStore.Append` (`server/notifications/store.go:153`) | No new ticker/goroutine; reuses existing "prune on write" cadence |
| `NotificationReferencesDeadSession` | Whenever a stored record has no `item_id` AND `reviewQueuePoller.FindInstance(record.SessionID) == nil` | `DeleteNotificationRecord` (NEW) | `NotificationService` (bridges `notificationStore` + `reviewQueuePoller`, both already wired at `notification_service.go:31-32`) | Deliberately excludes item_id-linked records — those remain valid "View in Backlog" entries even after the session is gone |

## Integration points (files to touch)

- `session/review_queue_determiner.go` — `Determine()`: add `inst.Hidden` early-return.
- `session/startup_scanner.go` — `Scan()`: add `inst.Hidden` to the skip condition at `:36`.
- `server/review_queue_manager.go` — `OnItemAdded()` (`:319-373`): add the
  `GetItemSessionBySessionUUID` call, suppress for `SessionRoleReview`/`SessionRoleTriage`,
  stamp `item_id` into `item.Metadata` otherwise. Needs `ctx` — `OnItemAdded` currently has none;
  a bounded `context.WithTimeout(context.Background(), ...)` (matching the pattern at
  `session/review_queue_poller.go` reconcile call sites) will be needed since this is called from a
  synchronous observer callback, not a request-scoped handler.
- `session/storage.go` / `session/storage_backlog.go` — `GetItemSessionBySessionUUID` (existing,
  no change needed).
- `server/notifications/store.go` — add `PruneOrphaned(exists func(sessionID string) bool) int`,
  call from inside the existing retention pass triggered by `Append()`.
- `server/services/notification_service.go` — wire `PruneOrphaned` using
  `ns.reviewQueuePoller.FindInstance` as the `exists` predicate; both fields already present.
- `server/services/autonomous_orchestration_service.go:305-316` — fix `nil` → `{"item_id": item.ID}`
  (currently-dead branch, cheap insurance, same PR).
- `web-app/src/app/notifications/NotificationsPage.tsx` — no change needed (already prefers
  `item_id` per requirements.md's own citation, confirmed at `:377-393`).

## Consistency / concurrency notes

- `OnItemAdded` runs synchronously inside the observer-notify path of `ReviewQueue.Add()` (lock
  released before notify, per the existing `Clear()`/`Add()` comments in `session/queue/queue.go`)
  — adding a DB query here moves a previously fire-and-forget callback into one that can block on
  the DB. Existing code already accepts this shape for `maybeAutoCreatePR` (also called from
  `OnItemAdded`, `:372`) and for `rqm.storage`-backed lookups elsewhere in this file, so it's
  consistent with the current design, but the new query should use a short bounded timeout, not an
  unbounded context, to avoid stalling the queue's notify path on a slow DB.
- `PruneOrphaned`'s `exists` predicate depends on `reviewQueuePoller.FindInstance`, which only knows
  about instances currently loaded into the poller's in-memory list — this is the same freshness
  bound `notification_service.go:91-93`'s existing lookup already accepts, so no new consistency
  requirement is introduced.
