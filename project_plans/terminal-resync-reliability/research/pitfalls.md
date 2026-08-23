# Pitfalls Research: terminal-resync-reliability

Agent 4 (Pitfalls), Phase 2 research. Scope: known failure modes for hardening a *live*,
already-shipped real-time feature behind a feature flag, plus stack-specific risks for this
repo's ConnectRPC/WebSocket + xterm.js + tmux control-mode + file-lock-semaphore combination.

## 1. General pitfalls: hardening a live streaming feature behind a flag

### 1.1 The flag itself is not what this repo's flag mechanism can do

The one flag this codebase already ships for client-side terminal behavior —
`NEXT_PUBLIC_RECONNECT_V2` — is a Next.js `NEXT_PUBLIC_*` env var
([`web-app/src/lib/hooks/useTerminalStream.ts:56-58,185,289,476,597`](../../../web-app/src/lib/hooks/useTerminalStream.ts)).
`web-app/next.config.ts` confirms this is a genuine Next.js app, which **inlines
`NEXT_PUBLIC_*` values at build time**. That means:

- This is not a runtime-toggleable flag (no LaunchDarkly/GrowthBook-style dynamic evaluation).
  "Flip the flag" means rebuild + redeploy the web app, full stop.
- There is no percentage rollout, no per-user targeting, and no way to flip it back off for a
  single affected user without a redeploy — the standard mid-incident "just kill the flag"
  playbook doesn't exist for this mechanism as currently implemented.
- **Consequence for this project**: if Phase 3/5 reuses this same pattern for the five
  in-scope fixes, "ship behind a feature flag" only buys revert-ability at redeploy
  granularity, not blast-radius control during rollout. If gradual/canary rollout is actually
  wanted (reasonable given "Large appetite" + "regression reintroduces the original corruption
  bug"), the flag needs a genuinely dynamic mechanism (server-evaluated, config-reloadable) —
  worth flagging back to the Constraints/Open Questions in requirements.md rather than
  assuming the existing pattern suffices.
- The server side has its own separate config-driven flag precedent —
  `TmuxExecGateConfig`/`config/types.go:84-103` (`defaultTmuxExecGateSlots = 8`, backed by
  `appconfig.LoadConfig()`). This *is* read fresh per-call (`LoadConfig()` inside
  `AcquireExecSlot`), so server-side flags in this repo can plausibly be hot-reloadable if
  `LoadConfig()`'s underlying config-file watch supports it — but client and server flag
  semantics would then differ (client: build-time constant; server: live-reloadable), which is
  itself a coherence risk if a single logical "resync hardening" flag is expected to gate both
  sides atomically. A client that thinks the flag is on (baked into its bundle) talking to a
  server that just had it flipped off (or vice versa) is a **skew window that always exists**
  under this architecture, not an edge case — every deploy has one until all open browser tabs
  reload.

### 1.2 Flag-flip-mid-session races

Even setting aside the build-time-vs-runtime distinction above: because `SessionDetailView.tsx`
keeps every terminal mounted for keep-alive, a single page load can hold N `TerminalOutput`
mounts alive for the session's entire lifetime — hours, potentially. If any part of the fix
set is gated by a flag read once at mount (a very likely implementation shape, matching how
`foreground`/`isVisible` are already threaded in as props rather than polled), then:

- A tab that's been open since before a flag flip runs the **old** resync behavior until next
  reload; a freshly opened tab in the same browser runs the **new** behavior. Two tabs on the
  *same* session, open concurrently, can therefore run different resync logic against the same
  backend session — this is exactly the kind of "partial rollout inconsistency" this question
  asks about, and it's not hypothetical here given the mount-once architecture.
- Any correlation-ID scheme (Scope item 2) must be robust to this: if tab A (old code, no
  correlation ID) and tab B (new code, correlation ID) both trigger resyncs against the same
  session's server-side handler, the server-side "skip expensive slow path" and "correlate
  response" logic must tolerate requests that never carry a correlation ID at all, not just
  requests carrying a *stale* one.

### 1.3 Telemetry that measures the wrong thing

- **Observer effect**: the Observability Requirements ask for a stall-watchdog counter tagged
  by visibility state, burst-size counter, exec-gate wait-time metric, and dimension-mismatch
  frequency metric. Every one of these, if implemented as a synchronous log line or metric
  emission inside the hot path (`handleVisibilityOrFocusResyncInner` in
  [`useVisibilityResync.ts:108-168`](../../../web-app/src/components/sessions/useVisibilityResync.ts),
  or inside `AcquireExecSlot`/`acquireSlot` in
  [`session/tmux/exec_gate.go:113-152`](../../../session/tmux/exec_gate.go)), adds latency to
  the exact code path whose latency is the thing being fixed. `acquireSlot` already does
  `rand.Perm` + `flock.TryLock` filesystem syscalls per slot per retry iteration under
  contention (up to `execGateAcquireBackoffMax` = 100ms backoff cap) — an added synchronous
  metric emission (e.g. writing to a file, or a network-bound OTel exporter without local
  buffering) inside that retry loop would change the very queue-depth/wait-time numbers being
  measured. Emit off the hot path (buffered counter incremented in-memory, flushed
  asynchronously) or the "improvement" numbers in Phase 6 verification will be partly an
  artifact of instrumentation overhead rather than the fix.
- **Console logging as telemetry**: the existing resync code already logs liberally via
  `console.info`/`console.warn`/`console.log` (see the six call sites in
  `useVisibilityResync.ts`). These are useful for local debugging but are not a metrics
  pipeline — "counter for stall-watchdog fires" implemented as "grep browser console logs"
  doesn't scale to the "many simultaneously mounted terminals, many sessions per user" NFR and
  won't produce an aggregate, cross-session number anyone can dashboard. If Phase 3 planning
  doesn't specify a real client-side metrics sink, the observability requirements risk being
  satisfied only in a way that's unusable for verifying the success metrics in production.
- **Sampling bias from the fix itself**: once resyncs are scoped to visible terminals only
  (Scope item 1), the *denominator* for "stall-watchdog fires for backgrounded terminals" drops
  to near-zero by construction (backgrounded terminals mostly stop requesting resyncs at all).
  A naive "fires per period" counter will look like a huge win even if the underlying
  queuing/capacity problem (exec-gate contention, dimension-mismatch slow path) is unchanged
  for the terminals that *do* still resync (visible-but-not-focused multi-monitor setups, or
  whatever visible-but-backgrounded semantics land on). Track fires *per resync attempt*
  (a rate), not just an absolute count, so the metric isn't gamed by the traffic reduction from
  fix #1 alone.
- **Tagging by visibility state is a snapshot, not a history**: `document.visibilityState` is
  read at the moment the counter increments. A terminal that was visible when the resync
  *started* but backgrounded by the time the watchdog fires 4 seconds later would be tagged
  by its *current* (backgrounded) state unless the tag is captured at request time and carried
  through — an easy off-by-moment bug that silently corrupts the "near-zero for backgrounded"
  success metric.

## 2. Stack-specific risks

### 2.1 ConnectRPC bidi streaming over WebSocket

- **No native request/response correlation.** ConnectRPC bidi streams are just message
  sequences over one WebSocket connection; there's no per-message request ID at the protocol
  layer analogous to gRPC's stream framing metadata being reused for this. The correlation ID
  this project wants (Scope item 2) has to be **application-level**, embedded in the proto
  message itself (`CurrentPaneRequest`/`CurrentPaneResponse` or equivalent) — this is a wire
  format change requiring `make proto-gen`, per CLAUDE.md's "New API Endpoints" section, and
  therefore touches every existing client build that hasn't picked up the new proto. A client
  running old-generated-code against a new server (or vice versa, during a rolling deploy) will
  either drop the new field silently (safe, if added as a new optional field) or panic on an
  unrecognized required field — favor an additive optional field, never repurpose an existing
  one.
- **One WebSocket serializes all message ordering for a session.** Because
  `connectrpc_websocket.go` streams over a single WebSocket per session-connection, a batched
  resync response (Scope item 5) sharing that same connection with ordinary PTY output means a
  large batched resync payload can head-of-line-block regular keystroke echo/output for that
  terminal while it's being sent/received — the reverse of the corruption fix's intent (visible
  jank on the terminal precisely when it's being made visible/focused).
- **Mid-connection dimension-mismatch slow path is already opaque to the client.** The current
  `CurrentPaneRequest` handler (`server/services/connectrpc_websocket.go`, e.g. the
  `streamViaControlMode`/`streamShellViaControlMode` handshake-dimension logic around lines
  598-720) does a resize-and-redraw nudge when client dimensions don't match server tmux pane
  size, with no signal back to the client about *why* it was slow. Any "skip slow path for
  backgrounded-stale dimensions" heuristic (Scope 3a) needs the client to tell the server
  "these dimensions might be stale" — new wire information, which is exactly what the Rabbit
  Holes section already flags as not existing today. Building this without very carefully
  scoping it risks scope creep into the exact rabbit hole requirements.md warns about.

### 2.2 xterm.js buffer/ANSI-state resets

- **A resync is not idempotent from xterm.js's point of view.** Whatever `requestFullResync`
  ultimately writes to the terminal (a fresh `tmux capture-pane` snapshot) is written into
  xterm.js's *live* buffer — if the previous resync's output is still being written
  (async/chunked) when a *new* resync starts (staggering/prioritization racing with the 300ms
  debounce and 4s watchdog), two full-screen snapshots interleaving into the terminal will
  produce visibly scrambled ANSI escape sequences (partial cursor-position codes, truncated SGR
  resets) — worse than the original corruption bug, and hard to reproduce because it depends on
  exact timing between two async writes.
- **Correlation ID cleanup must not leave xterm.js state stuck mid-reset.** If a resync's
  response never arrives (dropped connection, server crash mid-capture, orphaned server-side
  goroutine), the *terminal-side* state — not just a pending-request map entry — can also be
  left half-applied: e.g. a resize was issued (`SetWindowSize`) but the corresponding capture
  never rendered, leaving xterm.js's dimensions out of sync with its content. Cleanup on
  timeout/abandonment needs to consider both the correlation bookkeeping and the terminal's own
  render state, or the stall watchdog's "just force disconnect+reconnect" fallback (which does
  reset both) becomes load-bearing forever rather than a rare fallback.
- **Batching couples independent terminals' failure domains.** Scope item 5 ("batch updates...
  redesign the resync wire protocol") risks exactly the failure mode named in the question: if
  N terminals' resync requests get coalesced into one wire round trip/message for efficiency,
  a single malformed or oversized pane capture (e.g. one terminal has an enormous scrollback or
  a stuck ANSI sequence) can fail or delay the whole batch, turning one terminal's problem into
  N terminals' stalls — the opposite of the isolation the visibility-scoping fix (item 1) is
  trying to achieve. Any batching design needs per-item error isolation (partial-success
  responses), not one-fails-all-fail semantics.

### 2.3 tmux control-mode capture

- **tmux's server is single-threaded** (per the exec-gate's own doc comment,
  [`session/tmux/exec_gate.go:36`](../../../session/tmux/exec_gate.go)) and does an
  "unconditional O(n) scan of every connected client on each wakeup." Any staggering/
  prioritization scheme (Scope item 4) that spreads resyncs out in time still funnels through
  this same single-threaded server — staggering reduces peak concurrency of *exec-gate slot
  holders*, but every resync still ultimately issues tmux control-mode commands against a
  server whose per-operation cost scales with total connected clients. Raising
  `defaultTmuxExecGateSlots` (Scope 3b) increases how many subprocesses can be *in flight*
  concurrently but doesn't change that the tmux server itself processes them serially
  internally — past some slot count the bottleneck shifts from "waiting for a gate slot" to
  "tmux server itself is saturated," and the exec-gate's own comment already anticipates this
  ("gets measurably slower as concurrent load and client count rise"). A naive slots increase
  could reduce *queueing* latency while increasing *tmux-server-side* latency for everyone,
  net-negative for exactly the users this project is trying to help.
- **Control-mode capture is stateful per pane.** `streamViaControlMode`'s comments (lines
  ~284-353, ~664-708 in `connectrpc_websocket.go`) describe carefully sequenced nudge-then-
  capture logic to avoid serving stale-dimension content from cache. Any reordering introduced
  by staggering/batching resync requests (Scope items 4-5) risks breaking this sequencing
  assumption — e.g. if request A's nudge and request B's capture for the *same* pane
  interleave because they're now processed out of strict per-connection order, the capture can
  be taken at the wrong dimensions again, reintroducing a variant of the original corruption
  bug that shipped in `96990ce12`. This is the single highest-risk area to regression-test
  explicitly, since "restart ALL managed sessions" is called out in that same file (line ~353)
  as the alternative this code was written specifically to avoid.

### 2.4 Go file-lock-based semaphore (exec-gate)

- **`flock` slot files are host-local, not global.** `gateDir` keys by `serverSocket`
  (`session/tmux/exec_gate.go:104-121`), and slot files live under the config dir on local
  disk. If this project ever runs multiple stapler-squad server processes against the *same*
  tmux server (not currently how it's deployed, per CLAUDE.md's single systemd service model,
  but worth naming as an assumption this gate depends on), `flock` correctly serializes across
  processes on one host but provides no guarantee across hosts/containers — a "fast lane" gate
  key (Open Questions: "does exec-gate capacity increase need a new gate key... scoped to
  resync traffic only?") must still live under the same per-socket `gateDir`, or it silently
  stops being mutually exclusive with the main gate it's supposed to coexist with.
- **A raised slot count changes contention for *every* tmux subprocess caller, not just
  resync.** `AcquireExecSlot`/`runGated` (lines 40-95) are described as "the single call-site
  pattern every tmux subprocess spawn in this package should use" — meaning session creation,
  pane management, and other unrelated tmux operations share the same gate key
  (`"default"`, since `serverSocket` is almost always empty per the requirements doc). Raising
  `defaultTmuxExecGateSlots` to fix resync-burst contention necessarily raises the ceiling for
  *all* concurrent tmux operations system-wide — this is the exact "shared infra, blast radius
  beyond resync traffic" risk requirements.md's Rabbit Holes section already names, and it's
  the reason Scope item 3b explicitly offers "and/or add a fast lane" as an alternative. A
  fast lane needs its own slot pool/gate key so a resync burst can't starve or be starved by
  ordinary session-management tmux calls — reusing the same `"default"` pool defeats the
  purpose.
- **Retry backoff under `acquireSlot` is unbounded in wall-clock time when blocking.** The loop
  in `acquireSlot` (lines 128-152) retries with exponential backoff capped at
  `execGateAcquireBackoffMax` (100ms) *forever* until `ctx.Done()` — `AcquireExecSlot` itself
  imposes no deadline; callers must supply one via `ctx` (per `runGated`'s
  `execGateAcquireTimeout` = 5s wrapper, `exec_gate.go:76-83`). If a staggering/prioritization
  scheme (Scope item 4) issues resync requests with a longer or no per-request timeout (e.g. to
  let low-priority backgrounded resyncs "wait their turn" gracefully), those goroutines pile up
  blocked on `acquireSlot` — each one holding a WebSocket handler goroutine and its associated
  memory (buffers, any correlation-ID map entry, request context) open for the entire wait. A
  large resync burst with generous timeouts is a straightforward way to leak goroutines/memory
  under load even without any bug in the correlation-ID map itself.

## 3. Specific risks named in the research question, applied to this codebase

- **Thundering-herd retries if the watchdog still fires.** `useVisibilityResync.ts`'s
  stall-watchdog fallback (`disconnectRef.current().then(() => connectRef.current())`,
  line 143) has no jitter or backoff of its own — if the *new* design still allows the watchdog
  to fire simultaneously across many terminals (e.g. staggering reduces but doesn't eliminate
  simultaneous stalls under real contention spikes), every stalled terminal reconnects at
  effectively the same instant, regenerating the exact resync-storm-at-connect-time problem
  this project exists to fix, just shifted from "focus event" to "watchdog expiry." Any
  redesign must jitter the watchdog's own reconnect action, not just the initial resync
  request — the requirements' "stagger/prioritize resync bursts" (item 4) covers the *request*
  side but doesn't explicitly mention the *watchdog-triggered reconnect* side, which is a gap
  worth surfacing to Phase 3 planning.
- **Correlation-ID map memory leak on no-response.** A `Map<correlationId, PendingResync>`
  (or ref-based equivalent, following this file's existing ref-heavy pattern) needs an
  explicit TTL/cleanup tied to the *same* 4s stall watchdog, or an entry orphans forever if a
  response never arrives and the watchdog logic is refactored to not also clear the
  correlation map. Given `useVisibilityResync.ts` already tracks state across **two separate,
  documented-as-overlapping** hooks (`useTerminalFlowControl` and `useVisibilityResync` — see
  requirements.md's Rabbit Holes, and the existing session-switch cleanup effect at lines
  188-198 that already has to defensively clear multiple refs), a third piece of
  cross-hook-shared state (the correlation map) is a strong candidate for the same class of bug
  Story 2.1.6's cleanup effect was written to fix: state owned by one hook not torn down when
  the other hook's lifecycle (session switch, unmount) ends first.
- **Staggering introduces new perceived-latency complaints.** The existing `RESYNC_BANNER_DELAY_MS`
  (2000ms, `useVisibilityResync.ts:6-10`) already exists specifically because a resync can take
  long enough that users need a "reconnecting" affordance. Any staggering scheme that
  deliberately delays a *background* terminal's resync (correctly, per this project's goals)
  must ensure that when the user *then* switches focus to that terminal, its still-pending
  staggered resync doesn't leave it looking stale/frozen for the stagger interval — i.e.
  staggering needs a "promote to front of queue on focus" escape hatch, or the fix trades one
  user complaint (spurious disconnects) for another (terminals that look stuck until their
  staggered turn comes up), especially since `RESYNC_BANNER_DELAY_MS` was tuned against the
  *old* single-resync timing and may need re-tuning once queueing delay is a normal part of the
  critical path.

## Summary of most load-bearing findings

1. This repo's only existing client-side flag (`NEXT_PUBLIC_RECONNECT_V2`) is a Next.js
   build-time constant, not a dynamic flag — "ship behind a feature flag" needs a real decision
   about whether that's acceptable for a Large, corruption-risk-bearing project, since it means
   no live kill switch and an inherent client/server skew window on every deploy.
2. tmux's single-threaded server and the shared `"default"` exec-gate key mean naively raising
   `defaultTmuxExecGateSlots` (vs. a dedicated fast-lane gate key) risks trading resync-queue
   latency for tmux-server-side saturation, and risks starving/being starved by unrelated
   session-management tmux calls that share the same gate.
3. Batching (Scope 5) and control-mode capture's documented nudge-then-capture sequencing
   (`connectrpc_websocket.go`) are in tension: reordering or coalescing resync requests risks
   reintroducing dimension/timing bugs of the same shape as the original `96990ce12`
   corruption bug, making this the single highest-priority area for explicit regression tests
   in Phase 4/6.
