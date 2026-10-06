# Research: Pitfalls & Risks — backlog-event-subscribe

Scope: adapting the already-shipped `WatchBacklogItems` ConnectRPC server-streaming RPC
(`server/services/backlog_service_events.go`) into a bounded, synchronous, timeout-driven MCP
tool (`wait_for_backlog_event`). All findings below are grounded in the current checkout, not
general theory. The sibling project `backlog-event-driven-updates` already produced its own
`research/pitfalls.md` covering the stream/bus itself (durability, ordering, backpressure,
testing gaps) — that document is not repeated here except where it directly changes this
project's design; read it alongside this one.

## 1. Goroutine/subscription leaks — the bus already has a cleanup mechanism; the new tool must not bypass it

`pkg/events.EventBus.Subscribe` (`pkg/events/bus.go:51-66`) already builds in leak prevention:

```go
func (eb *EventBus) Subscribe(ctx context.Context) (<-chan *Event, string) {
	ch := make(chan *Event, eb.bufferSize)
	id := generateSubscriberID()
	eb.mu.Lock()
	eb.subscribers[id] = ch
	eb.mu.Unlock()

	// Cleanup subscription on context cancellation
	go func() {
		<-ctx.Done()
		eb.Unsubscribe(id)
	}()

	return ch, id
}
```

Two things matter here:

- **The cleanup is driven entirely by `ctx.Done()`.** If the tool handler derives its wait
  context from `context.WithTimeout(reqCtx, timeout)` and reliably calls the returned `cancel()`
  (via `defer`) on every exit path, the spawned goroutine fires `Unsubscribe(id)` regardless of
  *how* the handler returns (timeout fired, event matched early, panic recovered upstream,
  caller/session disconnected and canceled `reqCtx`). This is the same pattern
  `watchBacklogItems` already uses (`server/services/backlog_service_events.go:83-84`):
  `eventCh, subID := s.eventBus.Subscribe(ctx); defer s.eventBus.Unsubscribe(subID)` — belt and
  suspenders, since `Unsubscribe` is idempotent (`pkg/events/bus.go:146-154`, guarded by
  `if ch, exists := ...`).
- **The footgun is a missing or short-circuited `defer cancel()`.** If the new tool constructs
  `context.WithTimeout` but returns early on some error path *before* `defer cancel()` is
  registered (e.g. an early `return errResult(...)` above the `WithTimeout` call — a realistic
  slip given `wait_for_output`'s handler has several early-return validation checks before its
  own timeout logic, `server/mcp/tools_terminal.go:390-408`), the subscription goroutine leaks
  until the *parent* context (`reqCtx`, the MCP request context) is itself canceled — which, per
  finding 4 below, may never happen for an untimed-out request. **Concrete rule for planning:**
  call `Subscribe(ctx)` only *after* `ctx, cancel := context.WithTimeout(...)` and
  `defer cancel()` are both already in effect, mirroring the RPC handler's own ordering.
- **No new leak-prevention pattern is needed** — the existing `ctx.Done()`-driven cleanup plus
  `defer eb.Unsubscribe(subID)` is sufficient and is exactly what `WatchBacklogItems` already
  relies on. There's no evidence of a WeakRef/finalizer-style pattern anywhere in this codebase;
  don't invent one.
- **Test precedent to extend, not reinvent:** `pkg/events/bus_test.go:208`
  `TestEventBusContextCancellation` already asserts that canceling the subscribe context results
  in cleanup. The new tool's tests should add an equivalent goroutine-count assertion
  (`runtime.NumGoroutine()` before/after, or `go.uber.org/goleak`, already a project dependency —
  see `server/services/analytics_store_test.go:11,349,353` and `session/actor_test.go:6-22` for
  the two existing usage patterns, `goleak.VerifyNone(t, baseline)` after taking
  `goleak.IgnoreCurrent()` as a baseline) rather than trusting the bus's own test coverage to
  transitively prove the new tool has no leak — the tool's own timeout/early-return branches are
  new code the bus's tests don't exercise.

## 2. Missed-event race — order of operations, not `after_seq`, is what matters for a one-shot caller

The requirements' own framing ("does `after_seq` replay solve this cleanly for a one-shot
caller") is slightly off: `after_seq` solves the *reconnect* problem (a client that already knows
the last `Seq` it saw), not the *first-call* problem this tool actually has (a session that has
never seen any event for this item and doesn't know what `Seq` to pass). For a one-shot waiter,
the pattern that matters is **subscribe-before-read**, exactly as `watchBacklogItems` already
does it (`server/services/backlog_service_events.go:79-83`, comment: *"Subscribe before building
the snapshot/replay batch so no events are lost between the two phases"*):

1. `eventCh, subID := eventBus.Subscribe(ctx)` — start listening first.
2. *Then* read current item state (e.g. via `storage.GetBacklogItem` or equivalent) to check
   whether the condition the caller is waiting for (a verdict already recorded, a status already
   changed) is *already true* — this covers the case where the event fired between the caller's
   last poll and this tool call, before the subscription existed.
3. If not already satisfied, block on `eventCh` (with `select` against the timeout context) for a
   live event.

**Reversing steps 1 and 2 (check-then-subscribe) is the exact race the requirements name, and
this project must not reintroduce it** — an event landing in the gap between the read and the
subscribe call would be missed entirely (not even recoverable via `after_seq`, since the tool
has no prior `Seq` to replay from).

**Edge case even with the correct subscribe-then-check order:** because `Subscribe` fully
completes (channel registered under `eb.mu.Lock()`) before it returns, and `Publish` takes
`eb.mu.RLock()` to iterate subscribers (`pkg/events/bus.go:83-93`), there is no window where an
event published *after* `Subscribe()` returns can be missed by the channel — it will land in
`eventCh` (subject to the buffer-drop caveat in finding 4 below, not a missed-registration
issue). The only real race is the "read snapshot state" step landing *before* an event that was
actually published *before* `Subscribe()` was called — ordinary "state is stale by definition of
when you looked" behavior, not a bug; the tool's own read of current state after subscribing
already accounts for this by construction (step 2 above always sees state at-or-after the
subscribe point).

**A second-order race worth flagging for planning, not fixing here:** `EventBus.Publish` assigns
`Seq` via `eb.nextSeq.Add(1)` and then fans out under only an `RLock` (`pkg/events/bus.go:72-94`)
— two concurrent `Publish` calls are not guaranteed to deliver in `Seq` order (documented at
length in the sibling project's `research/pitfalls.md` §2, "Ordering — a real, currently-untested
race"). This is pre-existing and out of scope to fix here, but it means a caller waiting on
"verdict recorded" while a rapid double-transition happens concurrently (e.g. a reconciler
retriggers the same transition) could theoretically observe an out-of-order delivery. Low
practical likelihood for the `/backlog/review` verdict-wait use case (single writer per item in
practice) — call out as an accepted, pre-existing risk in the plan rather than building new
ordering guarantees into this tool.

## 3. Timeout/cancellation correctness

`.claude/rules/go-double-checked-locking.md` doesn't directly apply here (there's no
compute-then-conditionally-store-then-reread pattern in a subscribe-and-wait tool — it's a single
read of live state, not a cache), but its underlying principle — **return the value you actually
observed, not a value re-derived from shared state after the fact** — matters for one thing: when
the tool's `select` receives an event off `eventCh`, return *that event* directly, don't re-fetch
"current state" afterward and return the fetched copy (a second read could itself race against a
*third*, unrelated event and hand back state newer than what the caller was told to expect).

**`context.WithTimeout` + `select` on a channel is already used correctly elsewhere in this
codebase** — `server/mcp/tools_terminal.go`'s `writeToSession` (lines 291-306), `sendControl`
(358-371), and `steerSession` (686-699) all follow the same shape:

```go
errCh := make(chan error, 1)   // buffered — critical, see below
go func() { errCh <- inst.SendKeys(text) }()

ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()

select {
case err := <-errCh:
    ...
case <-ctx.Done():
    return errResult("PTY_WRITE_TIMEOUT", ...), nil
}
```

**The footgun these examples already defend against, and the new tool must too: the producer
channel must be buffered (`make(chan T, 1)`), not unbuffered.** If `errCh`/`eventCh` were
unbuffered and the `select` exits via the timeout branch, the producer goroutine's send
(`errCh <- ...`) blocks forever with no receiver — a goroutine leak on every single timeout, which
given `wait_for_backlog_event`'s entire purpose is to *often* time out (empty polling loops are
exactly what it replaces) would leak on a hot path, not an edge case.

**This is not actually a footgun for the new tool specifically**, because `EventBus.Subscribe`
already hands back a buffered channel (`make(chan *Event, eb.bufferSize)`,
`pkg/events/bus.go:52`, default `bufferSize=100`) and `Publish`'s send into it is already
non-blocking (`select { case ch <- event: default: }`, lines 87-92) — so the bus side can never
block waiting for this tool to receive. The tool's own `select` only needs `case evt, ok :=
<-eventCh` and `case <-ctx.Done()`, both non-blocking reads/receives — no new unbuffered-channel
risk to introduce, *as long as the tool subscribes via `eventBus.Subscribe` rather than building
any ad hoc `chan *Event` of its own for this feature.*

**`defer cancel()` placement:** all four precedent call sites above create the timeout context
immediately before the `select` and `defer cancel()` immediately after, with no branching in
between — copy that shape exactly. Don't hoist `context.WithTimeout` above validation checks
(session/item-not-found guards) the way `waitForOutput`'s own handler does *not* do (it validates
`session_id`/`pattern` args, then verifies the session exists, all *before* computing its
`deadline` via `time.Now().Add(...)` rather than a context — `wait_for_output` uses a manual
`deadline`/`ticker` loop, not `context.WithTimeout`, precisely because it's polling scrollback on
an interval rather than blocking on a channel). The new tool is closer in shape to
`writeToSession`/`sendControl` (context+select) than to `waitForOutput` (deadline+ticker) because
it has a real blocking channel to select on instead of something to poll — **do not copy
`wait_for_output`'s ticker-polling shape "because it's the closest existing tool for
`timeout_seconds`"**; the requirements' own baseline note (`requirements.md` line 45-48) calls
`wait_for_output` "the same shape" for the timeout *contract*, not the internal implementation —
internally this tool should look like `writeToSession`'s context+select, not `waitForOutput`'s
polling loop, since a real channel to block on is available and polling it would reintroduce the
avoid-polling core value proposition.

## 4. MCP tool call lifecycle — no server-imposed hard timeout, confirmed

`server/mcp/proxy.go:39-43`:

```go
// connectTimeout bounds the initial handshake only (transport start,
// initialize, tools/list). It is intentionally NOT applied to the HTTP
// client used for the lifetime of the connection, since individual tool
// calls (e.g. wait_for_output, run_command) may legitimately run long.
const connectTimeout = 3 * time.Second
```

This confirms: once the MCP handshake completes, an individual `CallTool` request (which is what
`wait_for_backlog_event` would be) is **not** subject to any additional server-side deadline
beyond what the tool's own handler chooses to impose via its `timeout_seconds` parameter. Grepped
`server/mcp/server.go` and `proxy.go` for any `http.Server{ReadTimeout,WriteTimeout}` or
per-request `context.WithTimeout` wrapping — none found; `mcpserver.NewStreamableHTTPServer`/
`NewStdioServer` are used with no additional timeout options in `NewCore`'s callers
(`server/mcp/server.go:85,114`). **Practical consequence:** the tool's own
`timeout_seconds`-derived `context.WithTimeout` is the *only* bound on how long the call can run
— get it right (finding 3), because nothing upstream will save a caller from a bug that causes
the tool to hang past the requested timeout. Cap `timeout_seconds` the same way `wait_for_output`
and `run_command` do (`mcpgo.Min(1)`, `mcpgo.Max(60)` / `Max(120)` — `server/mcp/tools_terminal.go
lines 130-131, 166-167`) so a caller can't request an unbounded wait that then has no upstream
backstop at all.

**One layer this research did not chase down and should be named as a gap:** whether the Claude
Code *harness* itself (outside this repo) imposes its own client-side timeout on an MCP tool call
that would fire before the server-side `timeout_seconds` elapses — e.g. if the harness times out
a stdio tool call at some fixed ceiling below `timeout_seconds`'s max. This is unverifiable from
inside this repo (the harness is external) — planning should pick a conservative default/max
(e.g. matching `wait_for_output`'s 30s default / 60s max, not `run_command`'s 120s max) rather
than assuming an arbitrarily long wait is safe end-to-end.

## 5. Multi-instance/workspace isolation — a genuinely NEW pitfall, confirmed, specific to the MCP path

The requirements assert prior isolation work "confirmed EventBus scoping is correct per-process"
and ask whether a new pitfall could be introduced because MCP tool calls route through a
different process. **Confirmed: yes, and it's more severe than "different process" — the
fallback path uses a `nil` EventBus, not merely a different instance of one.**

`main.go`'s MCP entrypoint (`main.go:70-104`) has two paths for every MCP tool call:

1. **Thin-client proxy (the common case):** `mcpserver.RunProxyServer(ctx, proxyURL, ...)`
   forwards the stdio `CallTool` request over HTTP to the already-running daemon's `/mcp`
   endpoint (`server/mcp/proxy.go`). In this path, the tool executes *inside the daemon process*,
   using the daemon's real `*events.EventBus` — the same instance `BacklogService.WatchBacklogItems`
   publishes/subscribes through. This path is safe and is what finding 1-4 above assume.
2. **Fully-local fallback (only when the daemon's HTTP endpoint isn't reachable — e.g. before the
   daemon has started, or it crashed):** `buildMCPDeps()` builds a fresh, independent
   `server.BuildCoreDeps()` stack, and `RunServer` is called with the `eventBus` parameter
   **hardcoded to `nil`**:

   ```go
   // main.go:104
   return mcpserver.RunServer(ctx, store, svc, sbMgr, storage, nil, nil, backlogEnabled, nil, nil)
   //                                                          ^^^ eventBus = nil
   ```

   `registerBacklogTools` wires this straight into `backlogHandlers.eventBus`
   (`server/mcp/tools_backlog.go:64,121`, doc comment: *"eventBus *events.EventBus // optional;
   nil means notifications are disabled"*). **A `wait_for_backlog_event` tool built on this
   struct will have `bh.eventBus == nil` on the fallback path — not a disconnected-but-live bus
   that just never sees the daemon's events, but a literal nil pointer.**

**Why this is worse than "different process, same-shaped bus":** `get_backlog_item` and other
existing point-in-time-read tools are unaffected by this fallback because they read directly from
`storage` (SQLite, shared on disk regardless of process) — reads still work correctly on the
fallback path, just without the daemon's live auto-triage side effects (already documented at
`main.go:99-103`: *"No BacklogService on this stdio fallback path... submit_review_verdict's
eager review->in_progress transition is skipped here"*). An event *subscription*, by contrast, has
no on-disk fallback — `pkg/events.EventBus` is purely in-memory (confirmed in the sibling
project's pitfalls doc §1) — so there is no data source to fall back to at all. **If the new tool
doesn't explicitly guard `eventBus == nil`, it will either nil-pointer-panic on `.Subscribe(ctx)`,
or (if written defensively but not deliberately) silently block until `timeout_seconds` expires
every single time it runs on this path — indistinguishable from "no event happened" to the
calling session, when the real cause is "this tool call cannot see events at all."**

**Required design response (concrete, for planning):** mirror
`watchBacklogItems`'s own existing guard (`server/services/backlog_service_events.go:75-77`):

```go
if s.eventBus == nil {
    return connect.NewError(connect.CodeUnimplemented, fmt.Errorf("backlog event stream is not available"))
}
```

`wait_for_backlog_event`'s handler must perform the equivalent nil check up front and return a
distinct, clearly-labeled MCP error (not a timeout result) — e.g. an `ErrFeatureDisabled`-style
code or a new `ErrEventStreamUnavailable`, distinguishable from `WAIT_TIMEOUT` — so a session sees
"this tool isn't usable right now, fall back to polling" rather than a false-negative "timed out,
no new event" that looks identical to the ordinary empty-wait outcome and would train the session
to (wrongly) trust a fast-empty response as "definitely no event yet."

**When does the fallback path actually trigger in practice?** `main.go:80`,
`mcpProxyURL(cfg.ListenAddress)` plus the daemon's `/mcp` endpoint being unreachable — i.e. before
the daemon has fully started, or after it has crashed/is mid-restart (note
`.claude/rules/tmux-keep-server-on-restart.md` and
`.claude/rules/service-restart-orphan-process.md` both describe daemon-restart scenarios already
known to be imperfect in this codebase). A session's very first MCP call in a freshly-created
worktree/session, spawned in the same instant the daemon is restarting, is a realistic trigger —
not a purely theoretical one.

## 6. Test-flakiness precedent for this feature class

`.claude/rules/fix-flaky-tests-dont-defer.md` names three recurring flakes in this codebase, none
of which are event-bus/streaming-specific (`TestEnsureServerRunning_NoOp`,
`TestKillOrphanedControlModeClients` in `session/tmux`, and
`TestRemoveHooksConfig_should_StripOnlyTheNamedHook_When_MultipleHooksPresent` in
`server/services`) — so there is no *named, tracked* recurring flake in exactly this feature
class today. However, the *shape* of risk is already anticipated structurally:

- `server/services/backlog_service_events_test.go`'s `testAfterSubscribeHook` seam
  (`server/services/backlog_service_events.go:43-53`) exists specifically because "reproducing
  [the Subscribe/EventsSince race window] depends on non-deterministic goroutine scheduling,
  which cannot be turned into a reliable regression test" without a deterministic hook. **This is
  the single clearest piece of evidence in the codebase that this exact feature class
  (subscribe-then-read races) is recognized as flake-prone enough to need a purpose-built
  determinism seam, not just `time.Sleep`-based test synchronization.** Planning should budget for
  an equivalent seam (or reuse this one, if the new tool's core logic is added as a method on
  `BacklogService` or a package that can import it) rather than relying on `time.After`/timing
  assertions in the new tool's own tests, which is exactly the anti-pattern the existing hook was
  built to avoid.
- `pkg/events/bus_test.go`'s own tests (`TestEventBusConcurrentPublish`,
  `TestEventBusBufferOverflow`) use timeout-based `select` assertions
  (`case <-time.After(...)`) as the general pattern for "wait for an async event or fail" — this
  is the accepted idiom in this codebase for testing bus consumers and is fine to copy, but keep
  timeouts generous (existing tests use 1-2s) to avoid CI-environment flakiness under load, per
  the general spirit of the fix-flaky-tests rule (a test that occasionally times out under CI
  contention is exactly the kind of "known flake" the rule says to root-cause rather than
  re-excuse — prefer a hook/synchronization primitive over a race against wall-clock time
  wherever the code under test can expose one, as `testAfterSubscribeHook` already does).
- No `t.Skip`/flaky-marker comments exist today in `server/services/*_test.go` or
  `server/mcp/*_test.go` for anything event/timing-related (confirmed via grep) — this feature
  would be the first of its kind in `server/mcp/`, so there's no existing pattern in that specific
  package to inherit bugs from, but also no local precedent to lean on beyond the
  `server/services` seam described above.

## Summary of concrete design directives for planning

1. Subscribe via `eventBus.Subscribe(ctx)` where `ctx` already has `context.WithTimeout` +
   `defer cancel()` applied *before* the subscribe call — don't validate-then-subscribe with the
   timeout context constructed later; construct the timeout context first, subscribe immediately,
   validate/read state second (finding 1, 2).
2. Order of operations inside the handler: **subscribe → read current state (check
   already-satisfied) → select on `{eventCh, ctx.Done()}`** — never read-then-subscribe (finding 2).
3. Use `select { case evt := <-eventCh: ...; case <-ctx.Done(): ... }`, not a `waitForOutput`-style
   polling ticker — a real channel exists to block on; polling it would defeat the point (finding 3).
4. Return the event received off the channel directly; don't re-fetch state after receiving it
   (finding 3, double-checked-locking principle).
5. Cap `timeout_seconds` conservatively (mirror `wait_for_output`'s 30s default / 60s max) since
   nothing upstream (MCP server, proxy) imposes its own ceiling — confirm the harness's own
   client-side timeout behavior separately if it becomes available to check (finding 4, named gap).
6. **Mandatory: guard `eventBus == nil` at the top of the handler** (mirroring
   `watchBacklogItems`'s existing guard) and return a distinct error code, not a timeout result —
   this is the one genuinely new pitfall this project introduces relative to the already-shipped
   RPC, caused by the MCP stdio fallback path's `nil` eventBus (finding 5). Add a test that drives
   the handler with `eventBus: nil` and asserts it returns the distinct error, not a false
   "timed out" result.
7. Add a goroutine-leak test (goleak or `runtime.NumGoroutine()` diff) covering both the
   timeout-exit and matched-event-exit paths, not just context-cancellation — reuse
   `go.uber.org/goleak`, already a project dependency (finding 1).
8. If the handler's core logic needs a deterministic-race test (subscribe/read-state race), add
   a test-only hook analogous to `testAfterSubscribeHook` rather than a `time.Sleep`-based test —
   this codebase has already had to solve this exact problem once and left a documented seam
   (finding 6).
