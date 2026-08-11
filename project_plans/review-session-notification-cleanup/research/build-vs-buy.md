# Build vs. Buy — Notifications for Headless Review/Triage Sessions

Agent 6, Phase 2 research. Scope per prompt: this is a small internal
bug/gap fix (suppress a notification category, thread `item_id` metadata,
prune orphaned rows) inside stapler-squad's existing Go/React monorepo —
no SaaS/OSS notification framework is a plausible replacement for the
feature as a whole. This pass instead checks, piece by piece, whether
existing in-repo mechanisms should be extended rather than new bespoke
code/mechanisms written.

## 1. Existing "is this session headless/background" predicate

**Searched for:** a reusable `IsHeadless(inst)` / `IsBackgroundSession(inst)`
/ similar helper, before assuming AC1's suppression check needs new code.

**Finding: no such helper exists anywhere in the codebase.**

- `grep` across `session/` and `server/` for `.Hidden` usage (excluding
  generated `session/ent/*`) turns up exactly one inline predicate doing
  this kind of check: `session/review_queue_poller.go:635`
  (`shouldSkipSession`):
  ```go
  return snap.Hidden || snap.Status == Stopped || snap.Status == Paused || !inst.Started()
  ```
  This is a private, single-purpose function on `ReviewQueuePoller` — not a
  generic predicate other packages call.
- `SessionRole` (`session.SessionRoleWork` / `SessionRoleTriage` /
  `SessionRoleReview`, defined `session/backlog.go:30-32`) is compared
  inline at **30+ call sites** across `session/backlog_lifecycle.go`,
  `session/storage_backlog.go`, `server/services/backlog_service*.go`, and
  `server/services/autonomous_orchestration_service.go` — always as a
  direct `== ` / `SessionRoleIn(...)` comparison, never through a shared
  "is this a background/automation session" wrapper function.

**Recommendation:** Write a small new, narrowly-scoped predicate for AC1
(e.g. checking `inst.Hidden || itemSession.Role == SessionRoleReview ||
itemSession.Role == SessionRoleTriage` at the point notifications are
published). This is genuinely new code, not a duplicate of anything
existing — but keep it a single small function (define it where it's
consumed — `server/review_queue_manager.go` or
`session/review_queue_determiner.go`, per this repo's interface-pollution
checklist) rather than reinventing `shouldSkipSession`'s logic a third
time. Do not try to force-reuse `shouldSkipSession` itself since it's
private to `ReviewQueuePoller` and mixes in queue-visibility concerns
(`Status == Stopped`, `!inst.Started()`) that are out of scope here.

## 2. Pruning — extend `enforceRetention()` vs. new cron/reconcile loop

**`server/notifications/store.go:437` `enforceRetention()`** currently only
does age (`MaxNotificationAge = 7*24h`) and count (`MaxNotifications =
500`) trimming. It runs opportunistically — called from `Append()` (every
new notification write) and once at store construction — there is no
independent ticker/goroutine for retention today.

**Searched for prior art on "reconcile against live instance registry":**

- `session/review_queue_poller.go:251` `cleanupOrphanedItems()` — despite
  the name, this is **not** the "session no longer exists" pattern the
  requirements doc speculated it might be. It only removes review-queue
  items with a zero/invalid `LastActivity` timestamp (a one-time
  migration-cleanup for a pre-`LastMeaningfulOutput` bug), unrelated to
  instance existence.
- The real prior art is `reconcileSessions` (same file, ~line 420-480):
  it diffs the in-memory instance set against a `liveSessions` map built
  from actual tmux sessions, and transitions/removes queue entries when an
  `Active` instance's tmux session is gone. This *is* a "reconcile against
  live state" pattern, but it reconciles tmux process liveness into
  `Instance.Status`, not "does a notification's referenced session ID
  still exist in storage" — the shapes are adjacent, not directly reusable
  as a function call.
- No existing function does "given a session UUID, does *any* record of it
  still exist" as a standalone check (see also section 3).

**Recommendation:** Extend `enforceRetention()` with a new predicate rather
than introducing a separate cron/reconcile loop. Concretely:
- Add a small injected dependency to `NotificationHistoryStore` — a
  `func(sessionID string) bool` (or a one-method interface) checking
  whether the session/instance still exists — wired from `server.go` at
  construction time, following the existing precedent there of wiring
  late-bound dependencies onto sibling components (e.g.
  `approvalHandler.SetNotificationStamper(notifStore)` /
  `SetAutoApprovalLogger(notifStore)` right after `notifStore` is built,
  `server/server.go:465-467`). The natural backing implementation is
  `SessionService.GetInstances()` (`server/services/session_service.go:4371`)
  for live sessions, falling back to a `Storage` lookup if "existence" is
  meant to include ended-but-persisted sessions.
- `enforceRetention()` already runs on every `Append`, so the new check
  piggybacks on an existing, already-cheap opportunistic pass — no new
  goroutine, ticker, or scheduled job is needed. This is materially
  cheaper and safer than a parallel reconcile loop: one code path, one set
  of tests, no new concurrency to reason about.
- Package layering note: `server/notifications` currently has zero
  dependency on `session`; keep it that way by injecting a plain function
  value/narrow interface rather than importing `*session.Registry` or
  `*session.Storage` directly into the notifications package (mirrors the
  existing `Appender` interface already used there for testability,
  `server/notifications/subscriber.go:23`).

## 3. Bespoke lookups (AC2/AC3) vs. existing utility

**Searched for:** an existing "resolve `item_id` for a `session_uuid`"
utility, to avoid writing a duplicate.

**Finding: the exact utility already exists and is already in scope at the
call site that needs it.**

- `session/storage.go:933` `Storage.GetItemSessionBySessionUUID(ctx,
  sessionUUID) (ItemSessionSummary, error)` — looks up the `ItemSession`
  row by `session_uuid` and follows the `backlog_item` edge.
  `ItemSessionSummary` (`session/repository.go:285-305`) already carries
  **both** fields needed here in one call: `.BacklogItemID` (AC2's
  `item_id`) and `.Role` (AC1's `SessionRole` check) — one lookup serves
  both acceptance criteria simultaneously.
- This function is already called from exactly the struct that owns
  `OnItemAdded` (the notification-publishing chokepoint,
  `server/review_queue_manager.go:319`): `ReactiveQueueManager` has a
  `storage *session.Storage` field (`server/review_queue_manager.go:59`)
  and already calls `rqm.storage.GetItemSessionBySessionUUID(...)` for an
  unrelated feature (`maybeAutoCreatePR`, line 428). No new plumbing is
  needed to make this data available inside `OnItemAdded` — the
  dependency is already there.
- No existing function does "does this session/instance still exist"
  (AC3) as a standalone boolean check — see section 2's gap. This one is
  legitimately new, but it's a simple existence check (map lookup against
  `SessionService.GetInstances()` or a `Storage` query), not a complex
  algorithm — bespoke code is the right call, no framework/library
  warranted.

**Recommendation:** Reuse `Storage.GetItemSessionBySessionUUID` for both
AC1's role check and AC2's `item_id` metadata (single call in
`OnItemAdded`). Write a small new existence-check function for AC3 (per
section 2) — confirmed nothing duplicates it today.

## 4. Final recommendation

**Confirmed**: this should ship as a small, targeted diff extending three
existing mechanisms, with zero new dependencies, frameworks, or services:

1. **AC1 (suppress)** — a new small predicate function, called from the
   existing chokepoint `ReactiveQueueManager.OnItemAdded`
   (`server/review_queue_manager.go:319`), guarding the existing
   `eventBus.Publish(notifEvent)` call the same way the
   `item.Reason != session.ReasonApprovalPending` guard already does two
   lines above it.
2. **AC2 (item_id metadata)** — reuse
   `Storage.GetItemSessionBySessionUUID` (already called one function
   away in the same file) to populate `item.Metadata["item_id"]` before
   constructing `notifEvent`.
3. **AC3 (prune orphaned)** — extend `NotificationHistoryStore
   .enforceRetention()` (`server/notifications/store.go:437`) with an
   injected existence-check dependency, wired from `server.go` alongside
   the existing `notifStore` setter-wiring block
   (`server/server.go:460-467`), reusing `SessionService.GetInstances()`
   as the source of truth for "still exists" — no new cron/reconcile
   loop.
4. **AC4 (regression test)** — standard Go test against the modified
   `OnItemAdded` path; no new test infrastructure required.

No case was found across all four investigation angles where a new
dependency, generic abstraction, or parallel mechanism would be cheaper or
safer than extending the three call sites named above. The one place a
genuinely new predicate/check is required (AC1's suppression condition,
AC3's existence check) is exactly where the requirements doc already
pointed, and both are simple boolean logic over data the consuming struct
already has in scope.
