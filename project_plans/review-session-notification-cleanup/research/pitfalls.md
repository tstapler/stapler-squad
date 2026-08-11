# Pitfalls & Risks — Notifications for Headless Review/Triage Sessions

Research for `project_plans/review-session-notification-cleanup/requirements.md`.
Covers: over-suppression, concurrency, pruning correctness, test flakiness, and
regression-test strategy.

## 1. Over-suppression risk

**All confirmed `Hidden: true` call sites (only two in the whole repo):**

- `server/services/session_service.go:827` — `SpawnReviewSession` →
  `CreateDirectorySession(..., oneShot=true, hidden=true)`. Title `"review:"+item.ID[:8]`.
- `server/services/backlog_service_triage.go:2351-2352` — `TriggerReReview`'s
  re-review session → `CreateDirectorySession(..., !useAutonomous /*oneShot*/, true /*hidden*/)`.
  Title `"re-review:"+slug`.

Both are `session.SessionRoleReview`-tagged, backlog-owned, ephemeral sessions.
**No other call site sets `Hidden: true`.** In particular:

- The real interactive backlog **work** session
  (`backlog_service_triage.go:703-708`, `SessionRoleWork`, tags
  `TagBacklogWork`) is spawned with `hidden=false` explicitly — confirmed side
  by side with the review session at the same call site pattern. This is the
  session a user actually watches for TASK_COMPLETE/Idle, so it must keep
  generating notifications; the two-call-site audit shows it does not
  accidentally inherit `Hidden`.
- User-initiated **one-off** sessions (`SessionTypeOneOff` /
  `req.Msg.OneOff`, per `.claude/rules/session-creation-registry.md`) go
  through the general `CreateSession` handler
  (`server/services/session_service.go` ~1390-1457), which builds
  `session.InstanceOptions` directly from `req.Msg` fields. There is no
  `Hidden` field on `CreateSessionRequest` and `InstanceOptions.Hidden` is
  never set in that path — it defaults to the zero value (`false`). So a
  user-created one-off session cannot accidentally get `Hidden=true` and does
  not risk suppression under AC1.
- Autonomous-mode work sessions (`AutonomousMode` flag, reusing
  `SessionTypeDirectory` per the registry doc's documented exception) also do
  not set `Hidden` anywhere in `autonomous_orchestration_service.go` or
  `backlog_lifecycle.go`.

**Conclusion:** gating on `Instance.Hidden` alone is *not* over-broad today —
it is exactly and only the review/re-review sessions. The larger
over-suppression risk is on the **`SessionRole` half** of AC1's OR condition:
`ItemSession.SessionRole == "review"` is checked via `is.Role`, but
`SessionRoleReview` is also used for the **legitimate work session that
becomes the review session on reopen** in some flows (see
`backlog_service.go:732,773,810` and `backlog_lifecycle.go:3141`, which treat
`SessionRoleWork` and `SessionRoleReview` as a pair in several places). Any
suppression keyed purely on `SessionRole == review` (without also confirming
`Hidden == true` on the same session) should be double-checked against
whether a *non-hidden* review-role session ever exists — if it does, an
`OR`-based check (`Hidden == true` **OR** `SessionRole == review`) would
silently swallow a real, visible session's notifications. Prefer requiring
both signals to agree, or scope the `SessionRole` check specifically to rows
whose `SessionUUID` prefix/lookup also resolves to a `Hidden` instance,
rather than trusting `SessionRole` in isolation.

## 2. Race conditions — concurrent poll loop + synchronous DB lookups

`session/review_queue_poller.go:checkSessions()` (line 503) fans out to up to
`checkSessionsConcurrency = 5` (line 500) concurrent goroutines via a
semaphore, once per poll tick. `PollInterval` defaults to **2 seconds**
(`DefaultReviewQueuePollerConfig`, line 45) — i.e. every active/idle session
in the poller's in-memory `rqp.instances` slice gets re-evaluated every 2s.

Today `shouldSkipSession` (line 631-636) and everything else in
`checkSession` (line 640+) is a **lock-free, in-memory snapshot read**
(`inst.Snapshot()`, `xsync.Map` cache, no I/O) — this is explicitly called
out in comments ("Lock-free read... across sessions", "no I/O inside").

**Risk:** if the suppression check requires resolving `ItemSession` by
`session_uuid` (a DB/ent query, per requirements' note that `session_uuid` is
a "loose FK, not an ent edge"), and that lookup is added directly inside
`checkSession`'s hot path, it turns a currently zero-I/O per-tick loop into up
to 5 concurrent synchronous DB round-trips every 2 seconds, scaling linearly
with active session count. This is the same class of problem
`.claude/docs/concurrency-patterns.md` and the double-checked-locking rule
warn about: adding synchronous I/O under what was previously a lock-free
read path changes its performance envelope silently.

**Mitigation options found in the existing codebase to point the plan at:**

- `OnItemAdded` (`server/review_queue_manager.go:319`) only fires **once per
  actual queue transition** (`ReviewQueue.Add`'s `exists==false` branch), not
  once per poll tick — several orders of magnitude less frequent than
  `checkSession`. It's already the place that does one on-demand instance
  lookup per notification (`rqm.poller.FindInstance(item.SessionID)` at line
  350, to resolve `GetStableID()`). Doing the `ItemSession`/`SessionRole`
  lookup **here** (or even better, caching the resolved `item_id` on the
  `Instance` at session-creation time so no lookup is needed at
  notification time at all) avoids adding I/O to the 2s poll loop entirely.
- `Instance.Hidden` itself needs **no** DB lookup — it's already an in-memory
  snapshot field, so the `Hidden` half of AC1 is cheap to check anywhere,
  including inside `checkSession`/`shouldSkipSession` if wanted for
  defense-in-depth. It's specifically the `SessionRole` half (which requires
  the `ItemSession` row) that carries the I/O risk.
- If `SessionRole`/`item_id` truly need to gate `Determine()` or
  `checkSession()` directly (per the "independent of the poller's existing
  `shouldSkipSession`" requirement), consider caching the resolved
  `(item_id, sessionRole)` pair on the `Instance` struct itself at session
  creation (it's immutable for the life of the session — an `ItemSession`
  row is created once per session and its `SessionRole`/`backlog_item` edge
  don't change), so the hot loop reads an in-memory field instead of
  querying on every tick.

## 3. Pruning correctness (AC3) — "not in memory" ≠ "gone forever"

Two confirmed facts push against a naive "session id not found in some
in-memory registry ⇒ prune" implementation:

**(a) Post-restart reload window.** `server/dependencies.go:448`
(`BuildRuntimeDeps`) explicitly documents and depends on ordering:
`storage.LoadInstances()` (line 462) must run, and — separately — that
function's own comment notes instance `Start()` calls are **deferred to an
async Step 6 loop** ("so a bulk load (server startup) doesn't block on
cold-restoring every dead session before the HTTP server can bind"). The
`BuildRuntimeDeps` doc comment itself warns: *"requires a TmuxServerReady
token to enforce that tmux.EnsureServerRunning was called before sessions
are loaded. Without this ordering, DoesSessionExist() may trigger
recoverFromServerFailure... which considers all sessions non-existent and
cold-restores them."* This is precedent for exactly the failure mode AC3
must avoid: any check that treats "not yet visible in some in-memory
collection at this exact moment" as "doesn't exist" can be wrong for a
window after every server restart, before the async reload/registration
finishes — which would mass-prune every notification for every still-real
session right after a restart.
- **Correct signal:** `session/storage.go:DeleteInstance(title)` (line 458)
  is the actual, persisted "genuinely gone" event — a hard delete from the
  repository. `SessionService.DeleteSession` (line 1939) is the only path
  that calls it, driven by an explicit user/API delete action, not a
  transient in-memory registry gap. Pruning should check *storage*
  (`storage.ListInstanceData()` / ent by UUID) for existence, not the
  poller's `rqp.instances` slice or `FindInstance`, and even then should
  likely be skipped/deferred until the async reload step at startup has
  completed, mirroring the `TmuxServerReady`-gating precedent.

**(b) `NotificationRecord.SessionID` is overloaded — not always a real
session ID.** `NotificationRecord` (`server/notifications/store.go:34-52`)
has one `SessionID string` field. For review-queue-originated notifications
it's `item.SessionID` resolved to `inst.GetStableID()` (a real
session UUID/title) — but for a whole family of operator-facing
notifications, `SessionID` is deliberately set to a **backlog item ID
instead**. `server/services/backlog_notifier.go`'s `EventBusNotifier.Notify`
comment states this explicitly: *"itemID is passed as sessionID (not just
metadata) so the notification subscriber's coalescing key... differentiates
between different backlog items."* This pattern repeats at
`backlog_service_triage.go:145,180,214,239,987,2041,2132` (rework-cap-hit,
repeated-failure, spawn-and-rollback-failed, triage-persist-failure,
branch-drift-blocked, codebase-missing notifications) — all pass `itemID` as
the event's `sessionID`. Both real session UUIDs and backlog item IDs are
plain `uuid.New().String()` values (see `GetStableID()`,
`session/instance_terminal.go:28`) — **format-indistinguishable**. A naive
"look up `SessionID` in the session registry; not found ⇒ prune" would
immediately and incorrectly delete every one of these item-scoped
notifications on the very first prune pass, since an item ID will never
match a session UUID. Pruning logic needs a **positive signal that a record
is session-scoped** before applying an existence check — e.g. only prune
records whose producer is known to be session-linked (the
`review-queue-<sessionID>-<timestamp>` ID prefix used at
`server/review_queue_manager.go:339` is one such signal already in the
data, though other session-linked producers may use different ID schemes and
would need auditing before relying on prefix-matching as the sole
discriminator).

**(c) Package layering / N+1 risk.** `server/notifications` currently has
zero dependency on `session` (confirmed: its only imports are
`encoding/json`, `os`, `path/filepath`, `sync`, `time`, `uuid`, `log`).
Wiring in an existence check means either injecting a lookup interface into
the store (consumer-defined per
`.claude/rules/interface-pollution-checklist.md`) or doing the prune pass in
a higher-level caller that already imports both. Either way, prune passes
should fetch the *set* of currently-known session UUIDs/titles (and
backlog item IDs) once per pass and filter in memory, not do one DB/registry
lookup per stored notification record (up to `MaxNotifications = 500`
records) each time `enforceRetention()`-equivalent logic runs.

## 4. Test flakiness — existing deterministic patterns to reuse

`review_queue_poller.go` defines two **consumer-side interfaces** specifically
so tests can avoid tmux/subprocess calls (per
`.claude/rules/interface-pollution-checklist.md`'s "define at the consumption
point" pattern, already followed here):

```go
type StatusProvider interface {
    GetStatus(inst *Instance) InstanceStatusInfo
    GetController(instanceTitle string) (*ClaudeController, bool)
}
type ContentProvider interface {
    GetContent(inst *Instance, statusInfo InstanceStatusInfo, paneActivity map[string]time.Time) string
    EvictInstance(title string)
}
```

`session/review_queue_reactive_test.go` shows the established fake pattern:
a hand-rolled `reactiveTestStatusProvider{statusByTitle map[string]InstanceStatusInfo}`
satisfying `StatusProvider`, injected via
`NewReviewQueuePollerWithConfig(queue, statusProvider, nil, cfg)`. Tests then:

- construct a bare `&Instance{Title: ..., Status: Active}` and manually set
  `inst.started.Store(true)` / `inst.LastMeaningfulOutput` instead of
  spawning a real tmux session,
- call the **exported** `poller.CheckSession(inst)` directly (bypasses
  `checkSessions()`'s goroutine fan-out and the `batchPaneActivity("")`
  subprocess call entirely — single-threaded, deterministic),
- use `poller.injectCachedContent(...)` to seed terminal content without a
  real pane,
- assert via `testutil/wait.WaitForCondition(...)` (polling with timeout)
  rather than a fixed `time.Sleep`, for the short async gap between
  `CheckSession` and queue mutation.

This is the pattern any new `Determine()`/`checkSession` suppression test
should reuse — no new test infrastructure needed.

## 5. Regression test strategy for AC4 — integration-style beats unit-only

**Multiple independent notification-origin paths exist** (confirmed by
grepping every `NewNotificationEvent`/`eventBus.Publish` call site):
`server/review_queue_manager.go` (`OnItemAdded`, the review-queue path this
item's Current-State section documents), plus at least
`backlog_service_triage.go` (6 separate direct `eventBus.Publish` call sites
for item-scoped operator notifications), `approval_service.go`,
`approval_handler.go`, `capacity_monitor.go`, `unfinished_work_service.go`,
`checkpoint_service.go`, `session/unfinished/scanner.go`, and others. Per the
requirements' own "open question," the mechanism producing a notification
for a **synthetic headless-triage session** (no real `Instance`/tmux session
at all — `backlog_service_triage.go:1781` spawns it with a
`headlessTriageUUIDPrefix` UUID through `headlessPool`, never through
`Instance`/`Determine()`) is not yet located and is explicitly flagged as a
separate research thread. **This means unit-testing `Determine()` alone
cannot prove AC4** for the headless-triage case even if it's sufficient for
the `review:<hash>` `Hidden=true` case — `Determine()` only ever sees
`*Instance`, and headless-triage sessions may have none.

**Recommended test shape**, following the precedent already in this codebase
(`server/review_queue_manager_test.go:TestOnItemAdded_EventBusBehavior_BUG001`,
line 154): subscribe directly to the `*events.EventBus`
(`eventBus.Subscribe(ctx)`), drive the code path under test (either
`mgr.OnItemAdded(&session.ReviewItem{...})` for the review-queue path, or
whatever call backlog_service_triage.go's headless-triage completion routes
through once Agent 2 locates it), and assert **no** `events.EventNotification`
arrives within a bounded `select`/`time.After` window — exactly
`TestOnItemAdded_EventBusBehavior_BUG001`'s existing shape for
`ReasonApprovalPending`. This is more robust than asserting on
`Determine()`'s return value alone because:

1. It proves the actual observable contract (AC4 says "does not produce a
   Notifications-page entry," which is a statement about the EventBus/store,
   not about `Determine()`'s internal result), and
2. It stays valid regardless of *where* the suppression is implemented —
   whether in `Determine()`, in `OnItemAdded`, or in whatever separate path
   handles headless-triage — so it won't need rewriting once Agent 2's
   research pins down the headless-triage mechanism.

A second, narrower unit test on `Determine()`/`shouldSkipSession` for the
`Hidden=true` `Instance` case is still worth keeping (fast, no EventBus setup
needed) as a first line of defense, but should not be the *only* regression
test claimed against AC4 — it cannot cover the headless-triage path.
