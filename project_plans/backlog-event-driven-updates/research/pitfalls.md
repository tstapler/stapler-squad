# Research: Pitfalls & Risks — backlog-event-driven-updates

Scope: server-push event streams over ConnectRPC server-streaming, backed by the in-memory
`pkg/events.EventBus`, feeding live UI state for `/backlog`, `/backlog/board`, and
`BacklogItemDetail`. All findings below are grounded in the current code, not general theory.

## 1. Event bus durability — purely in-memory / ephemeral

`pkg/events/bus.go` has no persistence layer at all:

- `nextSeq` is an in-process `atomic.Uint64`, starting at 0 on every process start.
- The replay buffer (`eb.buf`) is a plain Go slice held in memory, capped at `eventBufTTL = 1
  hour` and `eventBufMaxLen = 10_000` (`pkg/events/bus.go:10-15`).
- `EventsSince(afterSeq)` only ever looks at that in-memory slice.

**Consequence for `after_seq` replay:** it works fine across a client reconnect *within the same
server process lifetime* (network blip, tab backgrounded then foregrounded, etc.) — the buffer
still has the missed events. It does **not** work across a full server restart: `nextSeq` resets
to 0, so a reconnecting client's remembered `afterSeq` (e.g. `500`) is now larger than every
`Seq` in the fresh (empty-or-small) buffer. `EventsSince` binary-searches for `Seq > afterSeq` and
returns `nil` — silently, not an error. A naive client reading "no events" as "nothing missed"
would show a permanently stale view after any server restart/deploy, invisibly.

**This is already anticipated and handled once, for `WatchSessions`, on the frontend — not the
backend.** `web-app/src/lib/hooks/useSessionService.ts:730-742` detects a *backwards* seq jump
(`event.seq < prevSeq`, only possible after a counter reset) on the first event of a fresh
stream, logs a warning, resets `lastSeqRef` to 0, and sets `needsFullResyncRef` so the next
reconnect does a full `listSessions()` fetch instead of trusting the buffer. **This detection
lives client-side only; the server gives no signal that a restart happened.** The new
`BacklogItemEvent`/`WatchBacklogItems` stream and its shared frontend hook must replicate this
exact backwards-jump-triggers-full-refetch pattern — do not assume "mirror `ReviewQueueEvent`"
covers it, because `ReviewQueueEvent` has no seq/replay concept at all (see §3).

**Design against:** treat `after_seq` as a best-effort optimization, never a correctness
guarantee. Every reconnect of the new hook (not just a detected backwards jump) should already
be doing a full itemList RPC fetch first (mirroring `useSessionService.ts`'s
`listSessions()`-before-`watchSessions()` sequencing at lines 825-831), so replay is purely an
incremental optimization on top of a resync that would recover correctness anyway.

## 2. Ordering — a real, currently-untested race, independent of this feature

Two things are worth separating: cross-event ordering at the bus level, and the frontend's
guard against applying a stale event.

**Bus-level race:** `EventBus.Publish` (`pkg/events/bus.go:72-94`) is *not* atomic across seq
assignment and fan-out:

```go
event.Seq = eb.nextSeq.Add(1)          // (1) seq assigned
eb.bufMu.Lock(); eb.buf = append(...); eb.bufMu.Unlock()  // (2) buffered
eb.mu.RLock(); for _, ch := range eb.subscribers { ch <- event }  // (3) fanned out, RLock only
```

Step (3) uses `RLock`, so two goroutines calling `Publish` concurrently can both be inside the
fan-out loop at once. If goroutine A gets `Seq=5` and goroutine B gets `Seq=6` a moment later,
there is no guarantee A's `ch <- event` executes before B's — the Go scheduler can deliver
`Seq=6` to a subscriber's channel before `Seq=5`. **This means a fast bounce (e.g.
`in_progress→review→in_progress` within milliseconds, triggered from two different goroutines —
plausible here since reconcilers, triage, and RPC handlers can all call
`TransitionBacklogItemStatus` concurrently for unrelated reasons) can arrive at the frontend
out of order despite monotonic `Seq` numbers.**

This is not hypothetical or new to this project — it's a latent bug in the shared `EventBus`
today. `pkg/events/bus_test.go`'s `TestEventBusConcurrentPublish` (line ~107) publishes 100
events concurrently but explicitly does not assert ordering (comment: "exact count may vary due
to timing" — it only checks *some* events arrived). There is no existing test that would catch
an ordering violation.

**Frontend guard — exists, but only as a no-op dedup, not a staleness guard.**
`web-app/src/lib/store/sessionsSlice.ts`'s `upsertSession` (lines 42-77) only special-cases the
*equal* case:

```ts
if (existing && existing.updatedAt !== undefined && incoming.updatedAt !== undefined &&
    existing.updatedAt.seconds === incoming.updatedAt.seconds &&
    existing.updatedAt.nanos === incoming.updatedAt.nanos) {
  return; // skip identical no-op
}
sessionsAdapter.upsertOne(state, action.payload); // otherwise: always overwrite
```

It does **not** compare `incoming.updatedAt < existing.updatedAt` and skip the write. If an
older event is delivered after a newer one (via the race above, or via retry/redelivery), the
existing `WatchSessions` pattern will happily regress the UI to the older state. **Copying this
pattern verbatim for backlog items inherits the bug.** Given backlog items are explicitly
autonomous/multi-writer (reconcilers + triage + RPC handlers + auto-ship, per the requirements'
own baseline), and given the session's own investigation this cycle already found two
"looks-fresh-but-is-stale" bugs (`LastCommitSha` seeded-once staleness;
`AttachSessionToItem` silently-wrong-workdir isolation), this project should **not** just copy
`upsertSession`'s guard — it should add a real `incoming.updatedAt < existing.updatedAt ⇒ drop`
check (or compare `Seq` if threaded through to the reducer) for the backlog item upsert reducer,
closing the gap rather than propagating it.

## 3. "Many call sites" risk — the two existing patterns fail differently, and neither fails loudly

Concretely, `storage.TransitionBacklogItemStatus` / `storage.UpdateBacklogItem` are called from
~30 distinct sites across `server/services/backlog_service_triage.go`,
`backlog_service_lifecycle.go`, `backlog_service_sync.go`, `autonomous_orchestration_service.go`,
`server/mcp/tools_backlog.go`, and `session/backlog_lifecycle.go`/`backlog_sync.go` — i.e. the
storage layer itself (`session/storage.go:706-722`) is a thin passthrough to
`session/ent_repository_backlog.go`, with **zero built-in event publication**. Every caller is
independently responsible for remembering to publish. That is the mechanism by which a missed
call site happens, and it is exactly the risk the requirements name explicitly.

**Failure mode if a call site is missed: silent, permanent staleness for that one code path —
not a loud failure.** Two existing safety nets exist elsewhere in this codebase that *would*
paper over a missed call site, but neither is automatic today for backlog:

- **Reconnect-triggered full refetch** (`useSessionService.ts`): every `watchSessions()` call —
  initial mount *and every reconnect* — does `listSessions()` before opening the stream (lines
  825-831). A missed publish call site means the UI is stale *only until the next reconnect*
  (network blip, backoff cycle, or the 30s/15s staleness backstop below), not forever, as long
  as reconnects happen. If the stream stays open indefinitely (e.g. a very stable connection to
  a rarely-restarted server), staleness for that one code path could persist for a long time.
- **Backstop staleness detector**: `useSessionService.ts:944-980` runs a periodic check — if no
  event has been received in 30s (always-visible tab) or 15s (tab visibility change), it marks
  `connectionState = "stale"` and forces a reconnect (which triggers the full refetch above).
  This is a *time-since-any-event* heuristic, not a *this-specific-item-is-wrong* heuristic — it
  won't catch "item X's status silently never updated after minute 1 while other items keep
  streaming fine," because other items' events keep resetting the global staleness clock.

**Recommendation, since the requirement explicitly asks whether a missed call site can be made
to fail loudly/detectably:** the safest structural fix is *not* "add a test enumerating every
`TransitionBacklogItemStatus` call site" (that test rots the moment someone adds a case inline
without updating the enumeration, silently, which is the same failure mode one level up).
Prefer collapsing the publish call into the mutation itself: have
`storage.TransitionBacklogItemStatus` / `storage.UpdateBacklogItem` publish the
`BacklogItemEvent` directly (storage already knows the old/new state at the point of the CAS),
so **no caller can forget** — the call site risk is eliminated at the layer boundary instead of
enumerated. This matches the requirements' own instinct ("critically the storage layer, not
just the RPC handler") and is stronger than a per-call-site test. A periodic reconciliation
fetch (mirroring the backstop above, scoped per-item rather than per-connection) is still worth
adding as defense-in-depth for the case a bug ships anyway, but should not be the *only* line of
defense.

## 4. Backpressure / memory — inherited "drop oldest for slow subscriber" is the wrong default for backlog

`EventBus.Publish` (`pkg/events/bus.go:86-93`) fans out non-blocking per subscriber:

```go
select {
case ch <- event:
default:
    // Subscriber is slow; drop to prevent blocking others.
}
```

Each subscriber's channel is sized `bufferSize` (constructor default 100,
`NewEventBus(bufferSize)`). `TestEventBusBufferOverflow` (`pkg/events/bus_test.go:210-247`)
confirms this concretely: publishing `2×bufferSize` events to a non-consuming subscriber results
in only `bufferSize` events ever being received — the rest are silently dropped, no error, no
notification to the subscriber that anything was lost.

For **session events** this is a reasonable trade-off — losing an intermediate terminal-output
tick or a mid-stream status flicker rarely changes what the user needs to know, since the latest
state supersedes it. For **backlog item events**, this is a materially different risk:
- The requirements describe an operator "relying on precise state history" — a fast bounce
  through `in_progress → review → in_progress` is exactly the kind of transient the requirements
  care about surfacing (it's literally the example given for the ordering question), and drop-
  oldest silently erases it if the client is a backgrounded tab.
- A backgrounded browser tab is the single most likely trigger: browsers throttle/suspend
  `requestAnimationFrame`/timers but a ConnectRPC stream (fetch + ReadableStream) may keep
  receiving into the channel while JS execution is throttled, making buffer exhaustion on a
  backgrounded tab a real, not edge-case, scenario — which is precisely when an operator tabs
  back in expecting to trust what they see.

**Inheriting the same 100-slot drop-oldest channel is safe capacity-wise (10k-event ring buffer
handles bursts fine) but the *silent* part is the problem, not the *bounded* part.** Recommend:
keep the same non-blocking bounded-channel mechanism (don't reinvent bus internals), but have
the new frontend hook use the `Seq` gap it can already detect (received event's `Seq` not equal
to `lastSeq + 1`) as a signal to trigger the same full-refetch path used for the backwards-jump
case in §1 — turning a silent drop into a self-healing resync, without needing new bus-level
plumbing. Today's `WatchSessions` client code does not check for forward gaps at all (only
backwards jumps), so this would be new logic, not a copy of an existing solved pattern.

## 5. Testing pitfalls — streaming RPCs are tested by bypassing the stream

Neither `WatchSessions` nor `WatchReviewQueue` has a test that drives the actual
`connect.ServerStream`/HTTP handler end-to-end. Confirmed by search — no test file calls
`svc.WatchSessions(...)` or exercises `connect.ServerStream[...]` directly; there is no mock
`ServerStream` implementation in the repo.

**The actual pattern used** (`server/services/session_service_test.go`'s
`TestDeleteSession_PublishesDeletedEvent`, `server/review_queue_manager_test.go`'s
`TestReactiveQueueManagerIntegration`/`TestOnItemAdded_EventBusBehavior_BUG001`): subscribe
directly to the `*events.EventBus` (or the manager's internal `eventCh`) in the test, call the
*mutating* method under test (e.g. `svc.DeleteSession(...)`), then assert on what comes out of
the channel with a timeout select:

```go
ch, _ := eventBus.Subscribe(ctx)
resp, err := svc.DeleteSession(ctx, req)
select {
case evt := <-ch:
    assert.Equal(t, events.EventSessionDeleted, evt.Type, ...)
case <-time.After(2 * time.Second):
    t.Fatal("timed out waiting for event")
}
```

This is the right pattern to copy — it tests "does the mutation actually publish the right
event," which is the highest-value assertion, and avoids the awkwardness of mocking a gRPC
stream. **What it does not cover, and what this project's tests should not assume is covered
just because it looks similar to `WatchReviewQueue`:**
- The RPC handler's own branching logic — initial-snapshot vs. `AfterSeq`-replay path, `Hidden`
  filtering, category/status filter application inside `WatchSessions`
  (`session_service.go:1991-2044`) — is untested directly today. If the new
  `WatchBacklogItems` handler has equivalent filter/replay branching, plan for at least one test
  that exercises the handler function itself (even with a hand-rolled fake
  `connect.ServerStream` implementing `Send`/`Conn`, or via `connect.NewClient` against an
  `httptest.Server` if a heavier integration test is preferred) — don't assume the
  subscribe-and-assert pattern above substitutes for it, since it only ever tests the bus
  fan-out, never the handler's own logic.
- **`ReviewQueueEvent`/`WatchReviewQueue` has no `Seq`/`after_seq`/replay concept whatsoever** —
  it's a second, independent, simpler event mechanism (`server/review_queue_manager.go`'s
  `ReactiveQueueManager`, its own `map[string]*reviewQueueStreamClient`, its own per-client
  `chan *sessionv1.ReviewQueueEvent`), not built on `pkg/events.EventBus`'s `Seq`/`EventsSince`
  machinery at all — despite living in the same event-bus-adjacent code and despite the
  requirements text pairing it with `WatchSessions` as "prior art for this exact pattern." Only
  `WatchSessions` has the seq/replay machinery this project explicitly wants
  (`after_seq`-based reconnect/replay is in scope). **The new `BacklogItemEvent`/
  `WatchBacklogItems` should structurally follow `WatchSessions` + `pkg/events.EventBus`, not
  `ReviewQueueEvent`/`ReactiveQueueManager`** — the latter would need the replay capability
  built from scratch, and copying its filter/dispatch shape without its (missing) replay
  semantics would quietly ship a stream with no reconnect story at all.

## Summary of concrete design directives

1. `after_seq` is a same-process-lifetime optimization only; server restarts silently reset it.
   Always pair the stream with a full-list refetch on every reconnect (not just detected
   backwards jumps), matching `useSessionService.ts`'s existing `listSessions()`-before-
   `watchSessions()` sequencing.
2. Build on `pkg/events.EventBus` + `WatchSessions`'s shape, not `ReviewQueueEvent`/
   `ReactiveQueueManager`'s shape — the latter has no seq/replay at all despite looking like
   parallel prior art.
3. Do not copy `sessionsSlice.upsertSession`'s guard verbatim — it only dedupes *identical*
   updates, it does not reject an older event arriving after a newer one. Add a real
   `updatedAt`/`seq` regression check for the backlog item reducer.
4. Eliminate the "many call sites" risk structurally by publishing from inside
   `storage.TransitionBacklogItemStatus`/`UpdateBacklogItem` themselves, not by asking every one
   of the ~30 current call sites (and every future one) to remember to publish.
5. Inherit the bus's bounded/drop-oldest channel as-is, but add forward-gap detection
   (`event.Seq != lastSeq + 1`) client-side to trigger a resync — this doesn't exist yet even
   for `WatchSessions` (which only detects backwards jumps), so treat it as new work, not reuse.
6. Plan at least one direct test of the `WatchBacklogItems` handler's branching logic (snapshot
   vs. replay, filters) — the existing subscribe-and-assert pattern used for `WatchSessions`/
   `WatchReviewQueue` tests only the bus fan-out, never the handler itself, and no test file in
   the repo drives the handler directly today.
