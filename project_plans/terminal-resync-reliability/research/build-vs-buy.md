# Build vs. Buy — terminal-resync-reliability

Agent 6, Phase 2 research. All findings below are VERIFIED by reading the cited files unless
marked INFERRED.

## 1. Existing OSS library or framework

### 1a. Wire-level compression (ConnectRPC / WebSocket)

**Finding: the compression bit already exists in this repo's wire format and is unused — this
is "flip a flag," not "add a library."**

The terminal stream does not use connect-go's built-in duplex-stream transport. It hand-rolls
the Connect envelope format directly over a `gorilla/websocket.Conn`
(`server/services/connectrpc_websocket.go:18`, `server/protocol/envelope.go`). The envelope is
`[1 byte flags][4 bytes length][N bytes data]`, and flag bit 0 is literally named
`CompressedFlag` (`server/protocol/envelope.go:19`) with an `IsCompressed()` reader
(`server/protocol/envelope.go:95`) — this mirrors the real Connect protocol spec's compressed-flag
semantics. But `grep -n "IsCompressed\|CompressedFlag\|gzip\|Compress" server/services/connectrpc_websocket.go`
returns **zero matches**: nothing ever sets the flag on write or checks it on read. The
capability was designed into the wire format and never wired up.

Separately, `server/middleware/gzip.go:11` already imports `github.com/klauspost/compress/gzip`
(faster than stdlib gzip, already a `go.mod` dependency at `github.com/klauspost/compress v1.18.0`)
for HTTP response compression (`server/server.go:979,1162`), and `session/scrollback/storage.go`
and `session/scrollback/fork.go` also depend on it. This is not a new library to add — it's
already linked into the binary and has an established call pattern in this codebase.

- **Pros**: Zero new dependencies. The flag byte, the length-prefixed framing, and a
  battle-tested gzip implementation are all already present; per-envelope compression is
  "set the flag, gzip the payload before `CreateEnvelope`, ungzip on `IsCompressed()`" — no new
  wire-format version, no protocol negotiation to design.
- **Cons**: Needs care to only compress payloads above some size (small `CurrentPaneRequest`
  envelopes at ~20 bytes overhead from gzip framing can end up *larger* compressed) — but that's
  a threshold constant, not new infrastructure. Per-message gzip (vs. a persistent
  permessage-deflate stream context) forgoes cross-message dictionary reuse, so gains are
  bounded to each payload's own redundancy — fine for capturing bursty full-pane text blobs
  (requirement #5's target), less impactful for tiny control messages.
- **Verdict: Recommended.** This is squarely a "finish wiring up what's already there" task, not
  a build-vs-buy decision — the buy decision was already made when this envelope format was
  designed to mirror Connect's spec.

### 1b. Request correlation / idempotency-key pattern

**Finding: no existing request/response correlation-ID convention exists anywhere in this
repo's streaming protocol — this needs to be built, but the shape is a one-line proto change,
not new infrastructure.**

Searched `proto/session/v1/events.proto` and `server/services/{approval_handler,session_service,
connectrpc_websocket}.go` for `correlation`/`request_id`/`RequestId`. The only real hits are (a)
`proto/session/v1/import.proto`'s `CorrelationResultProto`, which is history-file-matching
correlation (import candidate ↔ JSONL transcript) — an unrelated domain concept that happens to
share the English word "correlation," not a request/response ID pattern; and (b) a code comment
in `approval_handler.go:545` noting the approval ID is reused as the notification ID — an
adjacent but different pattern (reusing a domain entity's own ID, not a synthetic per-request
correlation token).

The actual message types on the bidirectional terminal stream —
`CurrentPaneRequest`/`CurrentPaneResponse` (`proto/session/v1/events.proto:195-228`) and
`ScrollbackRequest`/`ScrollbackResponse` (`proto/session/v1/events.proto:172-184`) — have **no
id field at all**. `TerminalData` only carries `session_id` and, since Story on custom shells,
`shell_id` (`events.proto:65,97`). This confirms the requirement's premise: today's
"any output message satisfies the pending resync" heuristic (`useVisibilityResync.ts:47-51`) is
the *only* completion signal, because there's nothing to correlate against.

- **Pros of building it (no alternative to "buy")**: The fix is additive and small —
  `optional uint64 correlation_id = N;` on `CurrentPaneRequest` and the same field echoed back
  on `CurrentPaneResponse`, plus a `map[uint64]chan *CurrentPaneResponse`-shaped pending-request
  table on both sides. This is protobuf's own idiom (compare gRPC's `google.rpc.RequestInfo` or
  JSON-RPC's `id` field) — there's no gap where an external library adds value; a correlation ID
  is a field, not a subsystem.
- **Cons**: The pending-request map itself (client and server side) is exactly the kind of
  hand-rolled state machine flagged in requirement question 3 below — TTL/cleanup on
  disconnect, no double-registration, no leak on out-of-order or dropped responses.
- **Verdict: Recommended to build** (there is no meaningful "buy" option for a two-field proto
  addition), **but pair the proto change with a well-tested primitive for the pending-map half**
  — see §3.

### 1c. Client-side event coalescing / debouncing

**Finding: `useDebounce`/`useDebouncedCallback` (`web-app/src/lib/hooks/useDebounce.ts`) solve a
different problem than the one this feature has, and reaching for a generic task-queue library
would be over-engineering either way.**

Read `web-app/src/lib/hooks/useDebounce.ts` in full (58 lines). Both exports are **per-hook-
instance** timers: `useDebounce(value, delay)` re-arms a single `setTimeout` in a `useEffect`
keyed on that value; `useDebouncedCallback` holds one `timeoutIdRef` per component instance. This
is the right tool for debouncing repeated calls *within one* mounted terminal (e.g. rapid resize
events), but `SessionDetailView.tsx` mounts one `TerminalOutput`/`useVisibilityResync` instance
**per terminal**, and the requirement is to stagger/coordinate *across* those independent
instances when a single `visibilitychange` event fires — a cross-instance concern that a
per-instance debounce hook cannot express no matter how it's configured, because each instance's
timer knows nothing about the others.

The sibling `terminal-input-batching` project (`project_plans/terminal-input-batching/
implementation/plan.md:245-333`) is a closer precedent in spirit — its `pendingBytesRef` /
`flushTimerRef` pattern coalesces multiple `sendInput()` calls into one flush — but it is also
per-hook-instance (one input buffer per terminal's own keystrokes), so it's a good template for
*how to write a coalescing ref+timer pair idiomatically in this codebase's style*, not a
directly reusable module for the *cross-instance* staggering problem.

- **Pros of the existing hooks**: Proven, tested, in the exact style this codebase already uses;
  zero new dependencies; correct model for any *single-terminal* coalescing needed alongside the
  cross-terminal fix (e.g. debouncing a terminal's own rapid resize-triggered resyncs).
  `terminal-input-batching`'s ref/timer idiom is directly reusable as a *pattern* (not a shared
  module) for whatever local buffering the correlation-ID pending-map needs.
- **Cons**: Neither solves the actual in-scope problem (staggering across N mounted terminals on
  one `visibilitychange` event) as-is. That requires a small piece of new, purpose-built state
  shared above the per-terminal hooks — most naturally a module-level (or `SessionDetailView`-
  lifted) queue that each `useVisibilityResync` instance registers into instead of firing
  `requestFullResync` synchronously, so the queue can drain by visibility/recency order. This is
  a novel but small addition (a few dozen lines: an ordered array + a drain loop with a fixed
  inter-item delay), not something a generic task-batching/queue library should be pulled in for.
- **Verdict**: **Viable but insufficient as-is** — `useDebounce` stays for its existing job;
  building a small dedicated cross-instance stagger queue is **Recommended** over adopting any
  external generic queue/batching library (e.g. `p-queue`-style npm packages), because the
  actual logic needed (sort N pending requests by a visibility/recency key, drain with a fixed
  delay) is a few dozen lines and pulling in a general-purpose task scheduler for that trades a
  known, auditable amount of code for an opaque dependency with configuration surface
  (concurrency, priority, retry) this feature doesn't need.

### 1d. Server-side rate-limiting / fair-queuing primitives

**Finding: `golang.org/x/time/rate` is already a direct dependency AND already has an in-repo
usage pattern to copy — this is the strongest "reuse, don't invent" finding in this report.**

`go.mod:46` lists `golang.org/x/time v0.15.0` directly (not just transitively — confirmed by
`grep -n "golang.org/x/time" go.mod`). `server/services/rate_limiter.go` (75 lines) already
wraps it: a `map[string]*rate.Limiter` keyed by string, each entry a `rate.NewLimiter(rl.rate,
rl.burst)` (`rate_limiter.go:13,15,24,36`). `server/services/trigger_rate_limiter.go` (55 lines)
is a second, adjacent per-key limiter. `golang.org/x/sync v0.21.0` (`go.mod:193`) is also already
a direct dependency, making `golang.org/x/sync/semaphore` available with no new dependency for a
weighted/counting-semaphore alternative to the exec-gate's file-lock scheme if that's ever
revisited (out of scope here per the requirement, but worth noting for §4).

The existing exec-gate (`session/tmux/exec_gate.go`, read in full) is **not** a rate limiter — it's
a cross-*process* mutual-exclusion gate implemented via `flock` on `n` slot files
(`acquireSlot`, `exec_gate.go:128-155`), because tmux subprocess concurrency must be bounded
across the whole machine (multiple `stapler-squad` processes, test runners, etc.), not just
within one Go process's memory. `golang.org/x/time/rate` operates in-process only and can't
replace that cross-process coordination — but it's exactly the right tool for a purely
in-process concern like "cap how many resync requests are in flight in the client's `useTerminal
Stream` hook, or in the server handler goroutine pool, before they even reach the exec-gate."

- **Pros**: Already vendored, already has two working call sites in this exact codebase to
  copy the idiom from (per-key limiter map). `rate.Limiter` correctly handles the requirement's
  "prioritize resync bursts" language via its burst-size parameter, and is a few-line addition
  on top of code already proven in production here.
  `golang.org/x/sync/semaphore.NewWeighted` is likewise already available with zero new
  dependency, if a fast-lane needs a distinct in-process concurrency cap rather than a rate
  limiter (these solve different problems: `rate.Limiter` throttles *frequency over time*,
  `semaphore.Weighted` caps *concurrent in-flight count* — the requirement's "fast lane for
  resync traffic" reads more like the latter).
  Either is a battle-tested library with correct edge-case handling (context cancellation,
  no goroutine leaks) that a hand-rolled token bucket would have to re-derive.
- **Cons**: Only replaces the *in-process* half of any capacity fix. If the design lands on
  literally raising `defaultTmuxExecGateSlots` or adding a resync-scoped key to the existing
  `gateDir`-keyed flock scheme, that stays hand-rolled — it's solving cross-process contention,
  a problem `x/time/rate`/`x/sync/semaphore` don't address.
- **Verdict: Recommended** for any purely in-process fast-lane/prioritization the design needs
  (reuse `x/time/rate` following `rate_limiter.go`'s exact pattern, or `x/sync/semaphore` for a
  concurrency cap) — **not recommended** to attempt replacing the exec-gate's flock-based
  cross-process design with either package; that's solving a different problem (see §4).

## 2. SaaS / managed API

Not applicable, as anticipated in the requirement. This is internal wire-protocol hardening
between a self-hosted Go server and its own React client over an already-authenticated
WebSocket — there is no external service boundary (rate limiting, compression, or request
correlation as a service) that a SaaS product addresses. No SaaS option was found or would fit;
noting explicitly per the requirement's ask.

## 3. LLM-generated implementation vs. battle-tested library

Two genuinely bespoke pieces are needed regardless of any library choice above, because neither
has an off-the-shelf equivalent scoped to this exact problem:

**(a) The correlation-ID pending-request map** (§1b). This is inherently custom — no library
provides "a map from your own synthetic ID to a Go channel, scoped to a live WebSocket
connection's lifetime, in a protobuf oneof-based protocol." The risk isn't "should we use a
library instead," it's *how the hand-rolled map is written*: it must be cleaned up on
disconnect/reconnect (todo: the existing connection-lifecycle teardown in `useTerminalStream.ts`
is the natural place to drain it), must not leak an entry when a response never arrives (needs a
timeout tied to `RESYNC_STALL_TIMEOUT_MS`, which already exists at
`useVisibilityResync.ts:6`), and must handle the map being touched from both a WebSocket
message-received callback and a timeout callback without a data race (client-side: single-
threaded JS event loop, so this is much lower risk than the Go server side; server-side: needs a
`sync.Mutex` or `sync.Map`, standard library, not a reason to add a dependency). **Recommendation:
build it, but explicitly model it on `useVisibilityResync.ts`'s existing ref-based
timer-cleanup pattern (already reviewed and tested for exactly this class of bug — see its
extensive doc comments on distinct-refs-per-timer at `useVisibilityResync.ts:41-45`) rather than
writing a new pattern from scratch.** This is a "copy the reviewed local idiom" recommendation,
not a "write a novel implementation" one.

**(b) Any staggering/prioritization queue** (§1c). Same reasoning: this is custom application
logic (sort pending resyncs by visibility/recency, drain with a delay) with no meaningful library
surface to replace. The correctness risk here is much lower than (a) — it's pure client-side
JS state with no concurrency hazard (single-threaded), and unlike the pending-request map, a bug
in the stagger queue fails safe (worst case: resyncs still burst, which is today's baseline, not
a regression below it).

**On the server-side capacity fixes** (§1d): here the "battle-tested library over hand-rolled"
argument is strongest, precisely because this touches a **live, traffic-bearing production
path** shared by every session, and `x/time/rate`/`x/sync/semaphore` are stdlib-adjacent,
extremely well-exercised primitives (used inside Kubernetes, Docker, and much of the Go
ecosystem) vs. a hand-rolled token bucket or counting scheme that would need its own tests to
reach the same confidence — for no benefit, since a correct in-process primitive already exists
and is already proven inside this exact repo (`rate_limiter.go`).

## 4. Fork or adapt

**`terminal-input-batching`** (`project_plans/terminal-input-batching/implementation/plan.md`):
useful as a **pattern to imitate**, not a **module to import** — see §1c. Its `pendingBytesRef`/
`flushTimerRef`/cleanup-flushes-not-cancels discipline is exactly the level of care this
feature's pending-request map and stagger queue need, and Phase 3 planning should point at it as
the style reference for whoever implements the timer/ref plumbing.

**`TmuxExecGateConfig`/`exec_gate.go`**: adapt, don't fork. `session/tmux/exec_gate.go`'s
`gateDir()` already keys lock directories per `serverSocket`
(`exec_gate.go:118-124`, "Keyed per socket (not global) so isolated servers... never contend
with the production gate or each other") — the same keying mechanism the requirement's "new
gate key / fast lane scoped to resync only" open question is asking for. Extending `gateDir`'s
`key` derivation (currently hardcoded to `"default"` when `serverSocket == ""`,
`exec_gate.go:120`) to accept a second dimension (e.g. `"default/resync"`) is a natural, minimal
adaptation of existing code — not a fork, and not new infrastructure. `TmuxExecGateConfig` in
`config/types.go:84-103` already has the `SlotsOrDefault()` fallback idiom
(`defaultTmuxExecGateSlots = 8`) that a resync-specific slot count would follow directly (e.g. a
new `TmuxExecGateConfig.ResyncSlots` field with its own default, mirroring the existing struct
shape) — Phase 3 should treat this as "add a field, not a new config type."

## Summary of Verdicts

| Area | Verdict | Why |
|---|---|---|
| Wire compression | **Recommended** (wire it up, don't add a library) | `CompressedFlag` + `klauspost/compress/gzip` already exist and are unused |
| Correlation ID (proto) | **Recommended** (build — no buy option exists) | 2-field proto addition; no existing pattern to reuse, none needed |
| Pending-request map (impl) | **Recommended** (build, but copy `useVisibilityResync.ts`'s ref/timer idiom) | No library fits; correctness risk managed by imitating already-reviewed local pattern |
| Client-side coalescing/debounce | **Viable but insufficient alone** | `useDebounce` solves a different (per-instance) problem than cross-terminal staggering |
| Cross-terminal stagger queue | **Recommended** (build small, don't adopt a generic queue lib) | Few dozen lines; external task-queue libs bring unneeded config surface |
| Server-side rate limiting | **Recommended** (`x/time/rate` / `x/sync/semaphore`, already-vendored) | Already a direct dependency with a working in-repo call-site to copy |
| Exec-gate capacity/fast-lane | **Adapt existing code** | `gateDir` keying + `TmuxExecGateConfig` already generalize to this; don't fork or rebuild |
| SaaS/managed API | **Not applicable** | No external service boundary exists for this internal wire-protocol work |
